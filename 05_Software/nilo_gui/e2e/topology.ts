import { spawn, type ChildProcess } from 'node:child_process';
import { existsSync } from 'node:fs';
import { createServer, type AddressInfo, type Server } from 'node:net';
import { resolve } from 'node:path';

import {
  requireBrowserRuntime,
  requireCheckoutInDistribution,
  selectDistribution,
  type CommandOutcome,
} from './prerequisites';

/**
 * The development topology one integrated browser run needs, and nothing else.
 *
 * This harness runs on a developer's own workstation, beside their editors,
 * shells and their own running development topology. Two properties keep it
 * safe there and both are tested: every wait is bounded, and every teardown
 * touches only what this run created. It never clears a port, never matches a
 * process by name, and never signals anything it did not start itself.
 *
 * It is deliberately not built on 02_Tools/scripts/nilo-dev.ps1. That entry
 * point owns no process and signals nothing by design, which is right for a
 * visible manual development window and wrong for a test that has to stop the
 * backend mid-run and prove it cleaned up after itself.
 */

/** The deterministic simulation scenarios Bench Core serves. */
export type BenchScenario = 'healthy' | 'unavailable';

/** The result of one short-lived command the harness ran to completion. */
export type CommandResult = CommandOutcome;

/** How the harness runs a short-lived command. */
export type RunCommand = (
  file: string,
  args: readonly string[],
) => Promise<CommandResult>;

/** A long-lived process this run started, and therefore owns. */
export interface ProcessHandle {
  readonly name: string;
  /** Ask it to stop. Safe to call on a process that has already gone. */
  kill(): void;
  /** Settles when it is gone. */
  readonly exited: Promise<void>;
}

/** Everything one run created and is therefore allowed to destroy. */
export interface OwnedResources {
  readonly distribution: string;
  /** This run's own temporary directory inside the distribution, or none yet. */
  readonly dataDirectory: string | null;
  readonly children: readonly ProcessHandle[];
}

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
 * The development token could not be read.
 *
 * Its message never repeats the command output, because on the successful path
 * that output is the plaintext credential.
 */
export class TokenIssueError extends Error {
  constructor(detail: string) {
    super(`the development token could not be read: ${detail}`);
    this.name = 'TokenIssueError';
  }
}

/** What stands in for the token wherever a failure would otherwise print it. */
export const REDACTED_TOKEN = '<redacted development token>';

/**
 * Run an action that carries the token, and let no failure repeat it.
 *
 * A failing browser action puts a call log on the error it throws, and that log
 * repeats the argument once the locator has resolved - which the token field
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
  return text.split(secret).join(REDACTED_TOKEN);
}

function describeError(failure: unknown): string {
  return failure instanceof Error ? failure.message : String(failure);
}

/** A run that could not release everything it created. */
export class CleanupError extends Error {
  /** The directory that may still exist inside the distribution. */
  readonly dataDirectory: string | null;

  constructor(detail: string, dataDirectory: string | null) {
    super(
      `the browser end-to-end run could not release what it created: ${detail}` +
        (dataDirectory === null
          ? ''
          : `; ${dataDirectory} may still exist inside the distribution`),
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
 * always distinct. Fixed ports would fight with the developer's own topology,
 * and this harness has no right to take a port from anything else.
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
 * The directory this run's temporary state goes under, inside the distribution.
 *
 * A home directory on a Windows drive mount is refused rather than used: a live
 * SQLite database on that filesystem is exactly what the development flow
 * forbids.
 */
export function requireLinuxStateRoot(home: string): string {
  const trimmed = home.trim();
  if (trimmed === '') {
    throw new Error('the distribution reported no home directory for this run');
  }
  if (trimmed.startsWith('/mnt/')) {
    throw new Error(
      `the distribution home directory ${trimmed} resolves under /mnt/, which is a Windows drive mount; Bench Core state must stay on the WSL Linux filesystem`,
    );
  }
  return `${trimmed}/.local/state/nilo-e2e`;
}

/** Read the issued token out of the backend's output, quoting nothing back. */
export function parseIssuedToken(output: string): string {
  let document: unknown;
  try {
    document = JSON.parse(output);
  } catch {
    throw new TokenIssueError('the issuing command did not answer with JSON');
  }
  const value =
    typeof document === 'object' && document !== null
      ? (document as { token?: unknown }).token
      : undefined;
  if (typeof value !== 'string' || value === '') {
    throw new TokenIssueError('the issuing command answered without a token field');
  }
  return value;
}

/**
 * Stop this run's backend, and only this run's backend.
 *
 * The recorded identifier is checked against the process's own command line
 * before it is signalled, so a reused identifier now belonging to unrelated
 * work is left alone.
 */
export function buildStopScript(dataDirectory: string): string {
  const directory = shellQuote(dataDirectory);
  const pidFile = shellQuote(`${dataDirectory}/backend.pid`);
  return [
    `recorded=$(cat ${pidFile} 2>/dev/null || true)`,
    `if [ -n "$recorded" ] && [ -r "/proc/$recorded/cmdline" ] &&`,
    `   tr '\\0' ' ' < "/proc/$recorded/cmdline" | grep -qF -- ${directory}; then`,
    `  kill "$recorded" 2>/dev/null || true`,
    `fi`,
  ].join('\n');
}

/**
 * Stop this run's backend and remove this run's own directory, in that order.
 *
 * The removal is checked rather than trusted: `rm -rf` reporting success is not
 * the same as the directory being gone, and a run that left its database behind
 * has not cleaned up, however the command exited.
 */
export function buildReleaseScript(dataDirectory: string): string {
  const directory = shellQuote(dataDirectory);
  return [
    buildStopScript(dataDirectory),
    `rm -rf ${directory}`,
    `if [ -e ${directory} ]; then`,
    `  echo "the run directory still exists after removal" >&2`,
    `  exit 1`,
    `fi`,
    '',
  ].join('\n');
}

/**
 * Release everything one run created.
 *
 * Called on the success path and on every failure path, including a failure
 * part-way through start-up, so a half-built topology is never left behind.
 *
 * Every step is attempted even after an earlier one fails, and then the failures
 * are raised together. A release that did not succeed is a failure of the run:
 * a distribution that could not remove the directory has not removed it, and
 * reporting that as a clean pass would be the opposite of the guarantee this
 * function exists to make.
 */
export async function releaseOwned(
  owned: OwnedResources,
  run: RunCommand,
): Promise<void> {
  const failures: string[] = [];

  for (const child of owned.children) {
    try {
      child.kill();
    } catch (failure) {
      failures.push(`could not signal ${child.name}: ${describeError(failure)}`);
    }
  }
  await Promise.all(owned.children.map((child) => child.exited.catch(() => undefined)));

  if (owned.dataDirectory !== null) {
    let released: CommandResult | null = null;
    try {
      released = await run('wsl.exe', [
        '-d',
        owned.distribution,
        '--',
        'sh',
        '-c',
        buildReleaseScript(owned.dataDirectory),
      ]);
    } catch (failure) {
      failures.push(`could not run the release script: ${describeError(failure)}`);
    }
    if (released !== null && released.code !== 0) {
      failures.push(
        `the release script exited with ${released.code}${describeStderr(released.stderr)}`,
      );
    }
  }

  if (failures.length > 0) {
    throw new CleanupError(failures.join('; '), owned.dataDirectory);
  }
}

export interface TopologyOptions {
  /** The Windows path of this checkout. */
  repositoryRoot: string;
  /** The Windows path of the interface project. */
  guiRoot: string;
  scenario?: BenchScenario;
  distribution?: string;
  /** How long each endpoint gets to answer. */
  readyTimeoutMs?: number;
  /** How long the backend gets to compile before the run is abandoned. */
  buildTimeoutMs?: number;
}

/**
 * One integrated run's Bench Core under WSL2 and interface server on Windows.
 *
 * The token is held in one private field for the lifetime of the run and is
 * never written, printed, or put into another process's environment.
 */
export class DevelopmentTopology {
  readonly #distribution: string;
  readonly #dataDirectory: string;
  readonly #backendPort: number;
  readonly #frontendPort: number;
  readonly #scenario: BenchScenario;
  readonly #readyTimeoutMs: number;
  readonly #token: string;
  #backend: ProcessHandle | null;
  #frontend: ProcessHandle | null;
  #released: boolean;

  private constructor(state: {
    distribution: string;
    dataDirectory: string;
    backendPort: number;
    frontendPort: number;
    scenario: BenchScenario;
    readyTimeoutMs: number;
    token: string;
    backend: ProcessHandle;
    frontend: ProcessHandle;
  }) {
    this.#distribution = state.distribution;
    this.#dataDirectory = state.dataDirectory;
    this.#backendPort = state.backendPort;
    this.#frontendPort = state.frontendPort;
    this.#scenario = state.scenario;
    this.#readyTimeoutMs = state.readyTimeoutMs;
    this.#token = state.token;
    this.#backend = state.backend;
    this.#frontend = state.frontend;
    this.#released = false;
  }

  /** Bring the whole topology up, or leave nothing behind trying. */
  static async start(options: TopologyOptions): Promise<DevelopmentTopology> {
    const scenario = options.scenario ?? 'healthy';
    const readyTimeoutMs = options.readyTimeoutMs ?? 90_000;
    const buildTimeoutMs = options.buildTimeoutMs ?? 600_000;

    const distribution = selectDistribution(
      await listDistributions(),
      process.env.NILO_E2E_DISTRIBUTION ?? options.distribution,
    );
    requireBrowserRuntime(process.env.PLAYWRIGHT_BROWSERS_PATH, (path) =>
      existsSync(path),
    );

    // Forward slashes and shell quoting, both deliberate: a Windows path handed
    // to wsl.exe as a bare argument loses its backslashes on the way across, so
    // `F:\checkout` arrives as `F:checkout`. wslpath accepts either separator.
    const checkout = requireCheckoutInDistribution(
      await runInDistribution(
        distribution,
        `wslpath -a ${shellQuote(options.repositoryRoot.replace(/\\/g, '/'))}`,
      ),
    );
    const home = expectSuccess(
      await runInDistribution(distribution, 'printf %s "$HOME"'),
      'read the distribution home directory',
    ).stdout;
    const stateRoot = requireLinuxStateRoot(home);

    const dataDirectory = expectSuccess(
      await runInDistribution(
        distribution,
        `mkdir -p ${shellQuote(stateRoot)} && mktemp -d ${shellQuote(`${stateRoot}/run-XXXXXXXX`)}`,
      ),
      'create a temporary runtime directory inside the distribution',
    ).stdout.trim();

    const owned: { children: ProcessHandle[]; dataDirectory: string | null } = {
      children: [],
      dataDirectory,
    };

    try {
      const [backendPort, frontendPort] = await reserveLoopbackPorts(2);

      expectSuccess(
        await runInDistribution(
          distribution,
          `cd ${shellQuote(`${checkout}/05_Software/nilo_assistant`)} && ../../02_Tools/env/with-tools.sh go build -o ${shellQuote(`${dataDirectory}/bin/nilo`)} ./cmd/nilo`,
          buildTimeoutMs,
        ),
        'build Bench Core inside the distribution',
      );

      const issued = await runCommand('wsl.exe', [
        '-d',
        distribution,
        '--',
        `${dataDirectory}/bin/nilo`,
        'development',
        'issue-token',
        '--data-dir',
        dataDirectory,
      ]);
      if (issued.code !== 0) {
        // Only the diagnostic stream is repeated: the other one carries a token.
        throw new TokenIssueError(
          `the issuing command exited with ${issued.code}${describeStderr(issued.stderr)}`,
        );
      }
      const token = parseIssuedToken(issued.stdout);

      const backend = startBackendProcess({
        distribution,
        dataDirectory,
        scenario,
        port: backendPort,
      });
      owned.children.push(backend);
      await waitForBackend(backendPort, readyTimeoutMs);

      const frontend = startFrontendProcess({
        guiRoot: options.guiRoot,
        port: frontendPort,
        backendPort,
      });
      owned.children.push(frontend);
      await waitForFrontend(frontendPort, readyTimeoutMs);

      return new DevelopmentTopology({
        distribution,
        dataDirectory,
        backendPort,
        frontendPort,
        scenario,
        readyTimeoutMs,
        token,
        backend,
        frontend,
      });
    } catch (failure) {
      // The start-up failure is what the developer needs to read, so a cleanup
      // failure is appended to it rather than replacing it.
      const cleanup = await releaseOwned({ distribution, ...owned }, runCommand).then(
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

  /** Where the browser opens the interface. */
  get frontendUrl(): string {
    return `http://127.0.0.1:${this.#frontendPort}/`;
  }

  /**
   * Type the development token into a field, and let no failure repeat it.
   *
   * There is deliberately no accessor for the token: the value is held in one
   * private field for the lifetime of the run and leaves this object only into
   * the browser, through an action whose failures are redacted first.
   */
  async enterTokenInto(field: { fill(value: string): Promise<void> }): Promise<void> {
    await withRedactedSecret(this.#token, () => field.fill(this.#token));
  }

  /** Stop this run's Bench Core and wait until its endpoint stops answering. */
  async stopBackend(): Promise<void> {
    if (this.#backend !== null) {
      try {
        this.#backend.kill();
      } catch {
        // Already gone.
      }
      await this.#backend.exited.catch(() => undefined);
      this.#backend = null;
    }
    const stopped = await runCommand('wsl.exe', [
      '-d',
      this.#distribution,
      '--',
      'sh',
      '-c',
      buildStopScript(this.#dataDirectory),
    ]);
    if (stopped.code !== 0) {
      throw new CleanupError(
        `the stop script exited with ${stopped.code}${describeStderr(stopped.stderr)}`,
        this.#dataDirectory,
      );
    }
    await waitForEndpoint({
      url: this.#backendStatusUrl,
      accept: (status) => status === null,
      timeoutMs: this.#readyTimeoutMs,
    });
  }

  /** Start Bench Core again on the same port, database and identities. */
  async startBackend(): Promise<void> {
    this.#backend = startBackendProcess({
      distribution: this.#distribution,
      dataDirectory: this.#dataDirectory,
      scenario: this.#scenario,
      port: this.#backendPort,
    });
    await waitForBackend(this.#backendPort, this.#readyTimeoutMs);
  }

  /** Release this run's processes and temporary state. Safe to call twice. */
  async dispose(): Promise<void> {
    if (this.#released) {
      return;
    }
    this.#released = true;
    const children = [this.#frontend, this.#backend].filter(
      (child): child is ProcessHandle => child !== null,
    );
    this.#frontend = null;
    this.#backend = null;
    await releaseOwned(
      {
        distribution: this.#distribution,
        dataDirectory: this.#dataDirectory,
        children,
      },
      runCommand,
    );
  }

  get #backendStatusUrl(): string {
    return `http://127.0.0.1:${this.#backendPort}/api/v1/status`;
  }
}

/** Bench Core answering an unauthenticated status request is what readiness means. */
function waitForBackend(port: number, timeoutMs: number): Promise<void> {
  return waitForEndpoint({
    url: `http://127.0.0.1:${port}/api/v1/status`,
    accept: (status) => status === 401,
    timeoutMs,
  });
}

function waitForFrontend(port: number, timeoutMs: number): Promise<void> {
  return waitForEndpoint({
    url: `http://127.0.0.1:${port}/`,
    accept: (status) => status === 200,
    timeoutMs,
  });
}

function startBackendProcess(options: {
  distribution: string;
  dataDirectory: string;
  scenario: BenchScenario;
  port: number;
}): ProcessHandle {
  const directory = shellQuote(options.dataDirectory);
  const binary = shellQuote(`${options.dataDirectory}/bin/nilo`);
  // The shell records its own identifier and then becomes the server, so the
  // recorded identifier is the server's and the teardown can verify it.
  const script = [
    `echo $$ > ${shellQuote(`${options.dataDirectory}/backend.pid`)}`,
    `exec ${binary} serve --profile simulation --scenario ${options.scenario} --data-dir ${directory} --bind 127.0.0.1:${options.port}`,
  ].join('\n');

  return adopt(
    'backend',
    spawn('wsl.exe', ['-d', options.distribution, '--', 'sh', '-c', script], {
      stdio: 'ignore',
      windowsHide: true,
    }),
  );
}

function startFrontendProcess(options: {
  guiRoot: string;
  port: number;
  backendPort: number;
}): ProcessHandle {
  // Vite itself, not the package manager: one process to own means one process
  // to stop, with no shell or launcher tree in between.
  const vite = resolve(options.guiRoot, 'node_modules', 'vite', 'bin', 'vite.js');
  if (!existsSync(vite)) {
    throw new Error(
      `the interface development server is missing at ${vite}; run .\\02_Tools\\bootstrap\\bootstrap-frontend.ps1`,
    );
  }

  return adopt(
    'frontend',
    spawn(
      process.execPath,
      [vite, '--host', '127.0.0.1', '--port', String(options.port), '--strictPort'],
      {
        cwd: options.guiRoot,
        env: {
          ...process.env,
          VITE_NILO_API_MODE: 'live',
          VITE_NILO_API_BASE_URL: `http://127.0.0.1:${options.backendPort}`,
        },
        stdio: 'ignore',
        windowsHide: true,
      },
    ),
  );
}

function adopt(name: string, child: ChildProcess): ProcessHandle {
  const exited = new Promise<void>((settle) => {
    child.once('exit', () => settle());
    child.once('error', () => settle());
  });
  return {
    name,
    kill: () => {
      child.kill();
    },
    exited,
  };
}

async function listDistributions(): Promise<string[]> {
  // wsl.exe writes its listing as UTF-16, which is why this one command is
  // decoded differently from every other command the harness runs.
  const listing = await runCommand('wsl.exe', ['--list', '--quiet'], {
    encoding: 'utf16le',
  });
  if (listing.code !== 0) {
    return [];
  }
  return listing.stdout
    .split(/\r?\n/)
    .map((line) => line.replace(/\0/g, '').trim())
    .filter((line) => line !== '');
}

function runInDistribution(
  distribution: string,
  script: string,
  timeoutMs?: number,
): Promise<CommandResult> {
  return runCommand('wsl.exe', ['-d', distribution, '--', 'sh', '-c', script], {
    timeoutMs,
  });
}

function expectSuccess(result: CommandResult, what: string): CommandResult {
  if (result.code !== 0) {
    throw new Error(
      `could not ${what} (exit ${result.code})${describeStderr(result.stderr)}`,
    );
  }
  return result;
}

async function runCommand(
  file: string,
  args: readonly string[],
  options: { encoding?: BufferEncoding; timeoutMs?: number } = {},
): Promise<CommandResult> {
  const encoding = options.encoding ?? 'utf8';
  return new Promise((settle) => {
    const child = spawn(file, [...args], { windowsHide: true });
    const out: Buffer[] = [];
    const err: Buffer[] = [];
    let timer: NodeJS.Timeout | undefined;

    child.stdout?.on('data', (chunk: Buffer) => out.push(chunk));
    child.stderr?.on('data', (chunk: Buffer) => err.push(chunk));
    if (options.timeoutMs !== undefined) {
      timer = setTimeout(() => child.kill(), options.timeoutMs);
    }
    const finish = (code: number) => {
      if (timer !== undefined) {
        clearTimeout(timer);
      }
      settle({
        code,
        stdout: Buffer.concat(out).toString(encoding),
        stderr: Buffer.concat(err).toString(encoding),
      });
    };
    child.once('error', (error) => finish(errorCode(error)));
    child.once('close', (code) => finish(code ?? 1));
  });
}

function errorCode(error: Error): number {
  return (error as NodeJS.ErrnoException).code === 'ENOENT' ? 127 : 1;
}

async function probeHttpStatus(url: string): Promise<number | null> {
  try {
    const response = await fetch(url, { signal: AbortSignal.timeout(2_000) });
    await response.body?.cancel();
    return response.status;
  } catch {
    return null;
  }
}

function listenOnFreePort(): Promise<Server> {
  return new Promise((settle, fail) => {
    const server = createServer();
    server.once('error', fail);
    server.listen(0, '127.0.0.1', () => settle(server));
  });
}

function delay(milliseconds: number): Promise<void> {
  return new Promise((settle) => setTimeout(settle, milliseconds));
}

/** Quote a value so a path holding a space or an apostrophe cannot break out. */
function shellQuote(value: string): string {
  return `'${value.replace(/'/g, `'\\''`)}'`;
}

function describeStderr(stderr: string): string {
  const trimmed = stderr.trim();
  return trimmed === '' ? '' : `: ${trimmed}`;
}
