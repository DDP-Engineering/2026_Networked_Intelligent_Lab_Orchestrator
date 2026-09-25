import { readFileSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { describe, expect, it } from 'vitest';

import {
  ApplianceStartError,
  CleanupError,
  EndpointTimeoutError,
  REDACTED_SECRET,
  SERVICE_GID,
  SERVICE_UID,
  applianceLayout,
  parseListeningRecord,
  releaseOwned,
  reserveLoopbackPorts,
  waitForListeningRecord,
  waitForEndpoint,
  withRedactedSecret,
  type ProcessHandle,
} from './topology';

/**
 * The harness starts a real appliance inside a disposable container. These
 * tests hold it to the properties that keep that safe and honest: it waits for
 * a bounded time, it releases only what it made, it refuses any runtime profile
 * but the appliance, and it never lets a credential reach a failure message.
 */

const source = readFileSync(resolve(process.cwd(), 'e2e', 'topology.ts'), 'utf8');

function recordingHandle(name: string): ProcessHandle & { killed: number } {
  let killed = 0;
  const handle = {
    name,
    kill: () => {
      killed += 1;
    },
    exited: Promise.resolve(),
    get killed() {
      return killed;
    },
  };
  return handle as ProcessHandle & { killed: number };
}

describe('the listening record the appliance prints', () => {
  it('returns the address when the appliance profile is the one serving', () => {
    expect(
      parseListeningRecord(
        '{"status":"listening","address":"127.0.0.1:8443","profile":"appliance"}',
      ),
    ).toEqual({ address: '127.0.0.1:8443', profile: 'appliance' });
  });

  it('refuses any other runtime profile, because this suite proves the appliance', () => {
    const failure = capture(() =>
      parseListeningRecord('{"address":"127.0.0.1:8443","profile":"simulation"}'),
    );

    expect(failure).toBeInstanceOf(ApplianceStartError);
    expect(failure?.message).toContain('simulation');
    expect(failure?.message).toContain('appliance');
  });

  it('fails on a record that carries no address', () => {
    expect(
      capture(() => parseListeningRecord('{"profile":"appliance"}')),
    ).toBeInstanceOf(ApplianceStartError);
  });

  it('fails on output that is not the record at all', () => {
    expect(capture(() => parseListeningRecord('starting...'))).toBeInstanceOf(
      ApplianceStartError,
    );
  });
});

describe('waiting for an endpoint', () => {
  it('returns as soon as the answer is the one waited for', async () => {
    const answers = [null, null, 401];
    let slept = 0;

    await waitForEndpoint({
      url: 'https://127.0.0.1:8443/api/v1/status',
      accept: (status) => status === 401,
      timeoutMs: 10_000,
      probe: () => Promise.resolve(answers.shift() ?? null),
      sleep: () => {
        slept += 1;
        return Promise.resolve();
      },
      now: () => 0,
    });

    expect(slept).toBe(2);
  });

  it('gives up at the deadline rather than waiting forever', async () => {
    let clock = 0;

    await expect(
      waitForEndpoint({
        url: 'https://127.0.0.1:8443/api/v1/status',
        accept: (status) => status === 401,
        timeoutMs: 500,
        probe: () => Promise.resolve(null),
        sleep: () => Promise.resolve(),
        now: () => (clock += 250),
      }),
    ).rejects.toBeInstanceOf(EndpointTimeoutError);
  });

  it('can wait for an endpoint to stop answering', async () => {
    const answers = [401, null];

    await waitForEndpoint({
      url: 'https://127.0.0.1:8443/api/v1/status',
      accept: (status) => status === null,
      timeoutMs: 10_000,
      probe: () => Promise.resolve(answers.shift() ?? null),
      sleep: () => Promise.resolve(),
      now: () => 0,
    });
  });
});

describe('waiting for the core listening record', () => {
  it('fails at the configured deadline when the core stays alive and silent', async () => {
    await expect(
      waitForListeningRecord(new Promise<string>(() => undefined), 5),
    ).rejects.toBeInstanceOf(ApplianceStartError);
  });
});

describe('releasing what a run created', () => {
  it('signals every child and removes the run directory', async () => {
    const core = recordingHandle('appliance');
    const helper = recordingHandle('helper');
    const removed: string[] = [];

    await releaseOwned(
      { dataDirectory: '/tmp/nilo-e2e-1', children: [core, helper] },
      (path) => removed.push(path),
      () => false,
    );

    expect(core.killed).toBe(1);
    expect(helper.killed).toBe(1);
    expect(removed).toEqual(['/tmp/nilo-e2e-1']);
  });

  it('fails when the directory still exists after removal', async () => {
    await expect(
      releaseOwned(
        { dataDirectory: '/tmp/nilo-e2e-2', children: [] },
        () => undefined,
        () => true,
      ),
    ).rejects.toBeInstanceOf(CleanupError);
  });

  it('still removes the directory after a child could not be signalled', async () => {
    const removed: string[] = [];
    const stubborn: ProcessHandle = {
      name: 'appliance',
      kill: () => {
        throw new Error('gone');
      },
      exited: Promise.resolve(),
    };

    const failure = await releaseOwned(
      { dataDirectory: '/tmp/nilo-e2e-3', children: [stubborn] },
      (path) => removed.push(path),
      () => false,
    ).catch((error: unknown) => error as CleanupError);

    expect(removed).toEqual(['/tmp/nilo-e2e-3']);
    expect(failure).toBeInstanceOf(CleanupError);
    expect((failure as CleanupError).message).toContain('appliance');
  });

  it('names the directory that may still exist', async () => {
    const failure = await releaseOwned(
      { dataDirectory: '/tmp/nilo-e2e-4', children: [] },
      () => {
        throw new Error('permission denied');
      },
      () => true,
    ).catch((error: unknown) => error as CleanupError);

    expect((failure as CleanupError).dataDirectory).toBe('/tmp/nilo-e2e-4');
  });

  it('escalates an owned child that ignores the first termination signal', async () => {
    const signals: Array<NodeJS.Signals | undefined> = [];
    let exited!: () => void;
    const stubborn: ProcessHandle = {
      name: 'appliance',
      kill: (signal) => {
        signals.push(signal);
        if (signal === 'SIGKILL') exited();
      },
      exited: new Promise<void>((settle) => {
        exited = settle;
      }),
    };

    await releaseOwned(
      { dataDirectory: null, children: [stubborn] },
      () => undefined,
      () => false,
      { terminateTimeoutMs: 1, killTimeoutMs: 20 },
    );

    expect(signals).toEqual(['SIGTERM', 'SIGKILL']);
  });

  it('fails within the configured bounds when an owned child never exits', async () => {
    const stubborn: ProcessHandle = {
      name: 'helper',
      kill: () => undefined,
      exited: new Promise<void>(() => undefined),
    };

    await expect(
      releaseOwned(
        { dataDirectory: null, children: [stubborn] },
        () => undefined,
        () => false,
        { terminateTimeoutMs: 1, killTimeoutMs: 1 },
      ),
    ).rejects.toThrow(/helper.*did not exit/);
  });
});

describe('a credential carried into the browser', () => {
  it('is redacted out of whatever the action threw', async () => {
    const failure = await withRedactedSecret('super-secret', () => {
      throw new Error('fill("super-secret") failed');
    }).catch((error: unknown) => error as Error);

    expect(failure.message).not.toContain('super-secret');
    expect(failure.message).toContain(REDACTED_SECRET);
  });

  it('is redacted out of the stack as well as the message', async () => {
    const thrown = new Error('failed');
    thrown.stack = 'Error: failed\n  at fill("super-secret")';

    const failure = await withRedactedSecret('super-secret', () => {
      throw thrown;
    }).catch((error: unknown) => error as Error);

    expect(failure.stack).not.toContain('super-secret');
  });

  it('passes a successful result through untouched', async () => {
    await expect(withRedactedSecret('secret', () => 'result')).resolves.toBe('result');
  });
});

describe('the appliance layout', () => {
  it('keeps helper state, runtime data and TLS material apart', () => {
    const layout = applianceLayout(join('/tmp', 'run'));

    expect(layout.state).toBe(join('/tmp', 'run', 'state'));
    expect(layout.data).toBe(join('/tmp', 'run', 'data'));
    expect(layout.socket).toBe(join('/tmp', 'run', 'run', 'nethelper.sock'));
    expect(layout.certificate).toBe(join('/tmp', 'run', 'tls', 'server.crt'));
  });
});

describe('reserving loopback ports', () => {
  it('hands back distinct ports', async () => {
    const [first, second] = await reserveLoopbackPorts(2);

    expect(first).not.toBe(second);
  });
});

describe('the harness source', () => {
  it('starts Bench Core on the appliance profile and no other', () => {
    expect(source).toContain("'--profile',\n      'appliance',");
    expect(source).not.toContain('simulation');
    expect(source).not.toContain('--scenario');
  });

  it('runs Bench Core as the unprivileged service account, never as root', () => {
    expect(SERVICE_UID).toBe(65534);
    expect(SERVICE_GID).toBe(65534);
    expect(source).toContain('uid: SERVICE_UID, gid: SERVICE_GID');
  });

  it('serves the built interface rather than starting a development server', () => {
    expect(source).toContain("'--gui-dir'");
    expect(source).not.toContain('vite');
  });

  it('never passes a credential as a command argument', () => {
    expect(source).not.toContain('--password');
    expect(source).toContain('child.stdin?.end(input)');
  });

  it('offers no accessor that would hand the password out', () => {
    expect(source).not.toMatch(/get\s+password/);
  });
});

function capture(action: () => unknown): Error | null {
  try {
    action();
    return null;
  } catch (failure) {
    return failure as Error;
  }
}
