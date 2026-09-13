import { execFile, execFileSync, spawn, type ChildProcess } from 'node:child_process';
import {
  chmodSync,
  chownSync,
  existsSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  rmSync,
  writeFileSync,
} from 'node:fs';
import { request as httpsRequest } from 'node:https';
import { createServer, type AddressInfo, type Server } from 'node:net';
import { tmpdir } from 'node:os';
import { join } from 'node:path';

import { requireApplianceStage, requireBuiltInterface } from './prerequisites';

/**
 * The appliance one integrated browser run needs, and nothing else.
 *
 * The browser drives Bench Core in the configuration an appliance ships: the
 * `appliance` runtime profile, TLS material minted for this run, the built
 * interface served from Bench Core's own origin, and the real privileged helper
 * beside it. Nothing here is simulated - the adapters the interface shows are
 * the ones NetworkManager detected on the machine this runs on.
 *
 * It runs inside the disposable container created by
 * `02_Tools/tests/appliance/check.sh`, where D-Bus, NetworkManager and this
 * container's own adapter are already up. Two properties keep it safe there and
 * both are tested: every wait is bounded, and every teardown touches only what
 * this run created. It never clears a port, never matches a process by name,
 * and never signals anything it did not start itself.
 */

/** A long-lived process this run started, and therefore owns. */
export interface ProcessHandle {
  readonly name: string;
  /** Ask it to stop. Safe to call on a process that has already gone. */
  kill(signal?: NodeJS.Signals): void;
  /** Settles when it is gone. */
  readonly exited: Promise<void>;
}

/** Everything one run created and is therefore allowed to destroy. */
export interface OwnedResources {
  /** This run's own temporary directory, or none yet. */
  readonly dataDirectory: string | null;
  readonly children: readonly ProcessHandle[];
}

/** The unprivileged account Bench Core runs as, exactly as an appliance does. */
export const SERVICE_UID = 65534;
export const SERVICE_GID = 65534;

/** An endpoint that did not answer acceptably before its deadline. */
export class EndpointTimeoutError extends Error {
  readonly url: string;
  readonly timeoutMs: number;

  constructor(url: string, timeoutMs: number) {
    super(`${url} did not answer as expected within ${timeoutMs} ms`);
    this.name = 'EndpointTimeoutError';
    this.url = url;
    this.timeoutMs = timeoutMs;
  }
}

/**
 * Bench Core never reported the address it was listening on.
 *
 * Its message never repeats the process output, because a startup failure can
 * carry configuration the run would rather not echo.
 */
export class ApplianceStartError extends Error {
  constructor(detail: string) {
    super(`the appliance did not report a listening address: ${detail}`);
    this.name = 'ApplianceStartError';
  }
}

/** What stands in for a credential wherever a failure would otherwise print it. */
export const REDACTED_SECRET = '<redacted appliance credential>';

/**
 * Run an action that carries a secret, and let no failure repeat it.
 *
 * A failing browser action puts a call log on the error it throws, and that log
 * repeats the argument once the locator has resolved - which a password field
 * always does. Disabling trace, video and screenshot does not prevent it,
 * because the message itself reaches the console reporter and the error-context
 * note. Redacting here is structural: it holds whatever the runner threw and
 * whatever shape a future version gives it.
 */
export async function withRedactedSecret<T>(
  secret: string,
  action: () => Promise<T> | T,
): Promise<T> {
  try {
    return await action();
  } catch (failure) {
    if (secret === '') {
      throw failure;
    }
    const redacted = new Error(redact(describeError(failure), secret));
    redacted.name = failure instanceof Error ? failure.name : 'Error';
    redacted.stack =
      failure instanceof Error && typeof failure.stack === 'string'
        ? redact(failure.stack, secret)
        : redacted.stack;
    throw redacted;
  }
}

function redact(text: string, secret: string): string {
  return text.split(secret).join(REDACTED_SECRET);
}

function describeError(failure: unknown): string {
  return failure instanceof Error ? failure.message : String(failure);
}

/** A run that could not release everything it created. */
export class CleanupError extends Error {
  /** The directory that may still exist. */
  readonly dataDirectory: string | null;

  constructor(detail: string, dataDirectory: string | null) {
    super(
      `the browser end-to-end run could not release what it created: ${detail}` +
        (dataDirectory === null ? '' : `; ${dataDirectory} may still exist`),
    );
    this.name = 'CleanupError';
    this.dataDirectory = dataDirectory;
  }
}

export interface WaitForEndpointOptions {
  url: string;
  /** Whether an answer, or the absence of one, is what the caller waited for. */
  accept: (status: number | null) => boolean;
  timeoutMs: number;
  intervalMs?: number;
  probe?: (url: string) => Promise<number | null>;
  sleep?: (milliseconds: number) => Promise<void>;
  now?: () => number;
}

/** Wait until an endpoint answers acceptably, or the deadline passes. */
export async function waitForEndpoint(options: WaitForEndpointOptions): Promise<void> {
  const {
    url,
    accept,
    timeoutMs,
    intervalMs = 250,
    probe = probeHttpStatus,
    sleep = delay,
    now = () => Date.now(),
  } = options;
  const started = now();

  for (;;) {
    if (accept(await probe(url))) {
      return;
    }
    if (now() - started >= timeoutMs) {
      throw new EndpointTimeoutError(url, timeoutMs);
    }
    await sleep(intervalMs);
  }
}

/**
 * Reserve loopback ports the operating system says are free right now.
 *
 * Every reservation is held open until the last one is made, so the ports are
 * always distinct. Fixed ports would fight with anything else on the machine,
 * and this harness has no right to take a port from another process.
 */
export async function reserveLoopbackPorts(count: number): Promise<number[]> {
  const held: Server[] = [];
  try {
    for (let index = 0; index < count; index += 1) {
      held.push(await listenOnFreePort());
    }
    return held.map((server) => (server.address() as AddressInfo).port);
  } finally {
    await Promise.all(
      held.map((server) => new Promise<void>((settle) => server.close(() => settle()))),
    );
  }
}

/**
 * Read the address Bench Core reported, and confirm the profile it serves.
 *
 * The appliance prints one JSON record on standard output when its listener is
 * open. A record naming any other profile fails the run: the whole point of
 * this suite is that the browser reached an appliance.
 */
export function parseListeningRecord(output: string): {
  address: string;
  profile: string;
} {
  let document: unknown;
  try {
    document = JSON.parse(output);
  } catch {
    throw new ApplianceStartError('it did not answer with JSON');
  }
  const record =
    typeof document === 'object' && document !== null
      ? (document as { address?: unknown; profile?: unknown })
      : {};
  if (typeof record.address !== 'string' || record.address === '') {
    throw new ApplianceStartError('the record carries no address');
  }
  if (record.profile !== 'appliance') {
    throw new ApplianceStartError(
      `the record names the ${String(record.profile)} profile, but this suite proves the appliance`,
    );
  }
  return { address: record.address, profile: record.profile };
}

/** Wait for Bench Core's listening record without trusting the child to speak. */
export async function waitForListeningRecord(
  listening: Promise<string>,
  timeoutMs: number,
): Promise<string> {
  let timer: NodeJS.Timeout | undefined;
  try {
    return await Promise.race([
      listening,
      new Promise<string>((_settle, fail) => {
        timer = setTimeout(
          () => fail(new ApplianceStartError(`it stayed silent for ${timeoutMs} ms`)),
          timeoutMs,
        );
      }),
    ]);
  } finally {
    if (timer !== undefined) {
      clearTimeout(timer);
    }
  }
}

export interface ReleaseOptions {
  /** Grace period after SIGTERM before escalating. */
  terminateTimeoutMs?: number;
  /** Final period after SIGKILL before cleanup is reported incomplete. */
  killTimeoutMs?: number;
}

/**
 * Release everything one run created.
 *
 * Called on the success path and on every failure path, including a failure
 * part-way through start-up, so a half-built appliance is never left behind.
 *
 * Every step is attempted even after an earlier one fails, and then the failures
 * are raised together. A release that did not succeed is a failure of the run:
 * a directory that could not be removed has not been removed, and reporting
 * that as a clean pass would be the opposite of the guarantee this makes.
 */
export async function releaseOwned(
  owned: OwnedResources,
  remove: (path: string) => void = defaultRemove,
  present: (path: string) => boolean = existsSync,
  options: ReleaseOptions = {},
): Promise<void> {
  const failures: string[] = [];
  const exited = new Set<ProcessHandle>();
  const allExited = Promise.all(
    owned.children.map((child) =>
      child.exited.then(
        () => exited.add(child),
        () => exited.add(child),
      ),
    ),
  ).then(() => undefined);

  for (const child of owned.children) {
    try {
      child.kill('SIGTERM');
    } catch (failure) {
      failures.push(`could not signal ${child.name}: ${describeError(failure)}`);
    }
  }
  if (!(await settlesWithin(allExited, options.terminateTimeoutMs ?? 10_000))) {
    for (const child of owned.children) {
      if (exited.has(child)) {
        continue;
      }
      try {
        child.kill('SIGKILL');
      } catch (failure) {
        failures.push(
          `could not force ${child.name} to stop: ${describeError(failure)}`,
        );
      }
    }
    await settlesWithin(allExited, options.killTimeoutMs ?? 5_000);
  }
  for (const child of owned.children) {
    if (!exited.has(child)) {
      failures.push(`${child.name} did not exit after SIGTERM and SIGKILL`);
    }
  }

  if (owned.dataDirectory !== null) {
    try {
      remove(owned.dataDirectory);
    } catch (failure) {
      failures.push(`could not remove the run directory: ${describeError(failure)}`);
    }
    if (present(owned.dataDirectory)) {
      failures.push('the run directory still exists after removal');
    }
  }

  if (failures.length > 0) {
    throw new CleanupError(failures.join('; '), owned.dataDirectory);
  }
}

async function settlesWithin(
  settlement: Promise<void>,
  timeoutMs: number,
): Promise<boolean> {
  let timer: NodeJS.Timeout | undefined;
  try {
    return await Promise.race([
      settlement.then(() => true),
      new Promise<boolean>((settle) => {
        timer = setTimeout(() => settle(false), timeoutMs);
      }),
    ]);
  } finally {
    if (timer !== undefined) {
      clearTimeout(timer);
    }
  }
}

function defaultRemove(path: string): void {
  rmSync(path, { recursive: true, force: true });
}

export interface TopologyOptions {
  /** The path of this checkout. */
  repositoryRoot: string;
  /** The interface project directory. */
  guiRoot: string;
  /** How long each endpoint gets to answer. */
  readyTimeoutMs?: number;
}

/** The directory layout an appliance expects, with the ownership it expects. */
export interface ApplianceLayout {
  readonly root: string;
  readonly state: string;
  readonly run: string;
  readonly data: string;
  readonly tls: string;
  readonly socket: string;
  readonly certificate: string;
  readonly key: string;
}

/** Where each part of one run's appliance lives, derived from its root. */
export function applianceLayout(root: string): ApplianceLayout {
  return {
    root,
    state: join(root, 'state'),
    run: join(root, 'run'),
    data: join(root, 'data'),
    tls: join(root, 'tls'),
    socket: join(root, 'run', 'nethelper.sock'),
    certificate: join(root, 'tls', 'server.crt'),
    key: join(root, 'tls', 'server.key'),
  };
}

/**
 * One integrated run's appliance: a root privileged helper and an unprivileged
 * Bench Core serving the built interface over HTTPS.
 *
 * Administrator bootstrap accepts a password only for that call, sends it to
 * the child over standard input, redacts failures and retains no credential
 * field on the topology.
 */
export class DevelopmentTopology {
  readonly #layout: ApplianceLayout;
  readonly #readyTimeoutMs: number;
  readonly #binary: string;
  readonly #guiDir: string;
  readonly #browserNamespace: string;
  readonly #managementInterface: string;
  #base: string;
  #helper: ProcessHandle | null;
  #core: ProcessHandle | null;
  #released: boolean;

  private constructor(state: {
    layout: ApplianceLayout;
    readyTimeoutMs: number;
    binary: string;
    guiDir: string;
    browserNamespace: string;
    managementInterface: string;
    base: string;
    helper: ProcessHandle;
    core: ProcessHandle;
  }) {
    this.#layout = state.layout;
    this.#readyTimeoutMs = state.readyTimeoutMs;
    this.#binary = state.binary;
    this.#guiDir = state.guiDir;
    this.#browserNamespace = state.browserNamespace;
    this.#managementInterface = state.managementInterface;
    this.#base = state.base;
    this.#helper = state.helper;
    this.#core = state.core;
    this.#released = false;
  }

  /** Bring the appliance up, or leave nothing behind trying. */
  static async start(options: TopologyOptions): Promise<DevelopmentTopology> {
    const readyTimeoutMs = options.readyTimeoutMs ?? 90_000;

    const {
      binary,
      helperBinary,
      browserNamespace,
      managementInterface,
      managementHost,
    } = requireApplianceStage(process.env, existsSync);
    const guiDir = requireBuiltInterface(process.env, options.guiRoot, existsSync);

    const layout = applianceLayout(mkdtempSync(join(tmpdir(), 'nilo-e2e-')));
    const owned: { children: ProcessHandle[]; dataDirectory: string | null } = {
      children: [],
      dataDirectory: layout.root,
    };

    try {
      prepareApplianceRoot(layout, managementHost);

      const helper = adopt(
        'helper',
        spawn(
          helperBinary,
          [
            '--socket',
            layout.socket,
            '--state-dir',
            layout.state,
            '--allow-uid',
            String(SERVICE_UID),
            '--socket-gid',
            String(SERVICE_GID),
          ],
          { stdio: ['ignore', 'ignore', 'inherit'] },
        ),
      );
      owned.children.push(helper);
      await waitForPath(layout.socket, readyTimeoutMs);

      const [port] = await reserveLoopbackPorts(1);
      const core = startCoreProcess({
        binary,
        layout,
        guiDir,
        port,
        managementInterface,
        managementHost,
      });
      owned.children.push(core.handle);
      const base = await waitForListeningRecord(core.listening, readyTimeoutMs);
      await waitForEndpoint({
        url: `${base}/api/v1/status`,
        accept: (status) => status === 401,
        timeoutMs: readyTimeoutMs,
        probe: (url) => probeHttpStatus(url, browserNamespace),
      });

      return new DevelopmentTopology({
        layout,
        readyTimeoutMs,
        binary,
        guiDir,
        browserNamespace,
        managementInterface,
        base,
        helper,
        core: core.handle,
      });
    } catch (failure) {
      // The start-up failure is what the developer needs to read, so a cleanup
      // failure is appended to it rather than replacing it.
      const cleanup = await releaseOwned(owned).then(
        () => null,
        (error: unknown) => error as Error,
      );
      if (cleanup !== null) {
        throw new Error(`${describeError(failure)}\n${cleanup.message}`, {
          cause: failure,
        });
      }
      throw failure;
    }
  }

  /** Where the browser opens the interface: the appliance's own origin. */
  get frontendUrl(): string {
    return `${this.#base}/`;
  }

  /**
   * Create this run's first administrator through the real command line.
   *
   * First-administrator bootstrap is deliberately a local operation with no
   * HTTP route, so a browser test that needs a human identity has to make one
   * the same way an appliance owner does. The password is written to the
   * command's standard input rather than passed as an argument, and no failure
   * here repeats it.
   */
  async bootstrapAdministrator(username: string, password: string): Promise<void> {
    await withRedactedSecret(password, async () => {
      const created = await runWithInput(
        this.#binary,
        ['admin', 'bootstrap', '--data-dir', this.#layout.data, '--username', username],
        `${password}
${password}
`,
      );
      if (created.code !== 0) {
        // Only the diagnostic stream is repeated: the other one is not a place
        // a credential should ever reach, and this keeps it so by contract.
        throw new Error(
          `bootstrapping the administrator exited with ${created.code}${describeStderr(created.stderr)}`,
        );
      }
    });
  }

  /** Stop this run's Bench Core and wait until its endpoint stops answering. */
  async stopBackend(): Promise<void> {
    if (this.#core !== null) {
      await releaseOwned({ dataDirectory: null, children: [this.#core] });
      this.#core = null;
    }
    await waitForEndpoint({
      url: `${this.#base}/api/v1/status`,
      accept: (status) => status === null,
      timeoutMs: this.#readyTimeoutMs,
      probe: (url) => probeHttpStatus(url, this.#browserNamespace),
    });
  }

  /** Start Bench Core again on the same database, identities and address. */
  async startBackend(): Promise<void> {
    const core = startCoreProcess({
      binary: this.#binary,
      layout: this.#layout,
      guiDir: this.#guiDir,
      port: Number(new URL(this.#base).port),
      managementInterface: this.#managementInterface,
      managementHost: new URL(this.#base).hostname,
    });
    this.#core = core.handle;
    this.#base = await waitForListeningRecord(core.listening, this.#readyTimeoutMs);
    await waitForEndpoint({
      url: `${this.#base}/api/v1/status`,
      accept: (status) => status === 401,
      timeoutMs: this.#readyTimeoutMs,
      probe: (url) => probeHttpStatus(url, this.#browserNamespace),
    });
  }

  /** Release this run's processes and temporary state. Safe to call twice. */
  async dispose(): Promise<void> {
    if (this.#released) {
      return;
    }
    this.#released = true;
    const children = [this.#core, this.#helper].filter(
      (child): child is ProcessHandle => child !== null,
    );
    this.#core = null;
    this.#helper = null;
    await releaseOwned({ dataDirectory: this.#layout.root, children });
  }
}

/**
 * Lay the run out with the ownership an appliance has.
 *
 * The privileged helper refuses to start unless its state directory is
 * root-owned and private and its socket directory is root-owned and group
 * readable, and Bench Core cannot write a data directory it does not own. These
 * are the real deployment's permissions, not test scaffolding: relaxing them
 * here would prove a pairing no appliance runs.
 */
function prepareApplianceRoot(layout: ApplianceLayout, managementHost: string): void {
  chmodSync(layout.root, 0o755);
  for (const directory of [layout.state, layout.run, layout.data, layout.tls]) {
    mkdirSync(directory, { recursive: true });
  }
  chmodSync(layout.state, 0o700);
  chmodSync(layout.run, 0o750);
  chownSync(layout.run, 0, SERVICE_GID);
  chownSync(layout.data, SERVICE_UID, SERVICE_GID);

  const material = generateListenerMaterial(managementHost);
  writeFileSync(layout.certificate, material.certificate, { mode: 0o644 });
  writeFileSync(layout.key, material.key, { mode: 0o640 });
  // The service account reads the key it serves with; it never writes it.
  chownSync(layout.key, 0, SERVICE_GID);
}

/**
 * TLS material for one run, generated here and discarded with the run.
 *
 * No certificate is checked in and none outlives the container: a fixture
 * private key in a repository is a liability with no purpose.
 */
function generateListenerMaterial(managementHost: string): {
  certificate: string;
  key: string;
} {
  const directory = mkdtempSync(join(tmpdir(), 'nilo-tls-'));
  const certificate = join(directory, 'crt');
  const key = join(directory, 'key');
  try {
    execFileSync(
      'openssl',
      [
        'req',
        '-x509',
        '-newkey',
        'rsa:2048',
        '-nodes',
        '-keyout',
        key,
        '-out',
        certificate,
        '-days',
        '1',
        '-subj',
        `/CN=${managementHost}`,
        '-addext',
        `subjectAltName=IP:${managementHost}`,
      ],
      { stdio: 'ignore' },
    );
    return {
      certificate: readFileSync(certificate, 'utf8'),
      key: readFileSync(key, 'utf8'),
    };
  } finally {
    rmSync(directory, { recursive: true, force: true });
  }
}

function startCoreProcess(options: {
  binary: string;
  layout: ApplianceLayout;
  guiDir: string;
  port: number;
  managementInterface: string;
  managementHost: string;
}): { handle: ProcessHandle; listening: Promise<string> } {
  const child = spawn(
    options.binary,
    [
      'serve',
      '--profile',
      'appliance',
      '--data-dir',
      options.layout.data,
      '--management-interface',
      options.managementInterface,
      '--management-port',
      String(options.port),
      '--management-tls-name',
      options.managementHost,
      '--gui-dir',
      options.guiDir,
      '--tls-cert',
      options.layout.certificate,
      '--tls-key',
      options.layout.key,
      '--nethelper-socket',
      options.layout.socket,
    ],
    { stdio: ['ignore', 'pipe', 'inherit'], uid: SERVICE_UID, gid: SERVICE_GID },
  );

  const listening = new Promise<string>((settle, fail) => {
    let buffered = '';
    const onData = (chunk: Buffer): void => {
      buffered += chunk.toString('utf8');
      const newline = buffered.indexOf('\n');
      if (newline === -1) {
        return;
      }
      child.stdout?.off('data', onData);
      try {
        parseListeningRecord(buffered.slice(0, newline));
        settle(`https://${options.managementHost}:${options.port}`);
      } catch (failure) {
        fail(failure);
      }
    };
    child.stdout?.on('data', onData);
    child.once('exit', (code) =>
      fail(new ApplianceStartError(`it exited with ${String(code)} before listening`)),
    );
    child.once('error', (error) => fail(new ApplianceStartError(describeError(error))));
  });

  return { handle: adopt('appliance', child), listening };
}

function runWithInput(
  file: string,
  args: readonly string[],
  input: string,
): Promise<{ code: number; stderr: string }> {
  return new Promise((settle) => {
    const child = spawn(file, [...args], {
      stdio: ['pipe', 'ignore', 'pipe'],
      uid: SERVICE_UID,
      gid: SERVICE_GID,
    });
    let stderr = '';
    child.stderr?.on('data', (chunk: Buffer) => {
      stderr += chunk.toString('utf8');
    });
    child.once('error', () => settle({ code: 1, stderr }));
    child.once('exit', (code) => settle({ code: code ?? 1, stderr }));
    child.stdin?.end(input);
  });
}

function describeStderr(stderr: string): string {
  const trimmed = stderr.trim();
  return trimmed === '' ? '' : `: ${trimmed}`;
}

function adopt(name: string, child: ChildProcess): ProcessHandle {
  const exited = new Promise<void>((settle) => {
    child.once('exit', () => settle());
    child.once('error', () => settle());
  });
  return {
    name,
    kill: (signal = 'SIGTERM') => {
      child.kill(signal);
    },
    exited,
  };
}

async function waitForPath(path: string, timeoutMs: number): Promise<void> {
  const started = Date.now();
  for (;;) {
    if (existsSync(path)) {
      return;
    }
    if (Date.now() - started >= timeoutMs) {
      throw new EndpointTimeoutError(path, timeoutMs);
    }
    await delay(100);
  }
}

function listenOnFreePort(): Promise<Server> {
  return new Promise((settle, fail) => {
    const server = createServer();
    server.once('error', fail);
    server.listen(0, '127.0.0.1', () => settle(server));
  });
}

/**
 * Ask an endpoint for its status without verifying the certificate.
 *
 * The listener's certificate is generated for this run and trusted by nobody,
 * so the readiness probe cannot verify it. This is a liveness check only: no
 * credential is ever sent over it, and the browser's own navigation uses
 * Playwright's explicit `ignoreHTTPSErrors` instead.
 */
async function probeHttpStatus(
  url: string,
  networkNamespace?: string,
): Promise<number | null> {
  if (networkNamespace !== undefined) {
    return probeHttpStatusInNamespace(url, networkNamespace);
  }
  const target = new URL(url);
  return new Promise((settle) => {
    const attempt = httpsRequest(
      {
        hostname: target.hostname,
        port: target.port,
        path: target.pathname + target.search,
        method: 'GET',
        rejectUnauthorized: false,
        timeout: 5_000,
      },
      (response) => {
        response.resume();
        settle(response.statusCode ?? null);
      },
    );
    attempt.once('error', () => settle(null));
    attempt.once('timeout', () => {
      attempt.destroy();
      settle(null);
    });
    attempt.end();
  });
}

/** Run the same Node HTTPS probe from the namespace that contains Chromium. */
function probeHttpStatusInNamespace(
  url: string,
  networkNamespace: string,
): Promise<number | null> {
  const source = [
    "const https = require('node:https');",
    'const request = https.get(process.argv[1], { rejectUnauthorized: false }, response => {',
    "  response.resume(); process.stdout.write(String(response.statusCode ?? ''));",
    '});',
    "request.on('error', () => process.exit(1));",
    'request.setTimeout(4000, () => request.destroy());',
  ].join('\n');
  return new Promise((settle) => {
    execFile(
      'ip',
      ['netns', 'exec', networkNamespace, process.execPath, '-e', source, url],
      { timeout: 5_000 },
      (error, stdout) => {
        if (error !== null) {
          settle(null);
          return;
        }
        const status = Number.parseInt(stdout.trim(), 10);
        settle(Number.isInteger(status) ? status : null);
      },
    );
  });
}

function delay(milliseconds: number): Promise<void> {
  return new Promise((settle) => setTimeout(settle, milliseconds));
}
