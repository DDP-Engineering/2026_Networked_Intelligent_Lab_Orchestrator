import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { describe, expect, it } from 'vitest';

import {
  CleanupError,
  EndpointTimeoutError,
  REDACTED_TOKEN,
  TokenIssueError,
  buildReleaseScript,
  parseIssuedToken,
  releaseOwned,
  requireLinuxStateRoot,
  reserveLoopbackPorts,
  waitForEndpoint,
  withRedactedSecret,
  type CommandResult,
  type ProcessHandle,
} from './topology';

/**
 * The harness runs on a developer's own workstation, beside their editors,
 * servers and shells. These tests hold it to the two properties that keep it
 * safe there: it waits for a bounded time, and it releases only what it made.
 */

const source = readFileSync(resolve(process.cwd(), 'e2e', 'topology.ts'), 'utf8');

function recordingHandle(name: string): ProcessHandle & { killed: number } {
  let killed = 0;
  return {
    name,
    get killed() {
      return killed;
    },
    kill() {
      killed += 1;
    },
    exited: Promise.resolve(),
  };
}

describe('loopback port reservation', () => {
  it('takes ports the operating system says are free, not constants', async () => {
    const ports = await reserveLoopbackPorts(2);

    expect(ports).toHaveLength(2);
    expect(ports[0]).not.toBe(ports[1]);
    for (const port of ports) {
      expect(port).toBeGreaterThan(0);
      expect(port).toBeLessThan(65536);
    }
    expect(source).not.toMatch(/\b8090\b|\b5173\b/);
  });
});

describe('bounded readiness wait', () => {
  it('gives up within the supplied deadline and names the silent endpoint', async () => {
    let elapsed = 0;

    await expect(
      waitForEndpoint({
        url: 'http://127.0.0.1:1/api/v1/status',
        accept: (status) => status === 401,
        timeoutMs: 250,
        intervalMs: 10,
        probe: async () => null,
        sleep: async (milliseconds) => {
          elapsed += milliseconds;
        },
        now: () => elapsed,
      }),
    ).rejects.toThrowError(EndpointTimeoutError);

    expect(elapsed).toBeLessThanOrEqual(300);
  });

  it('carries the endpoint and the deadline on the error it raises', async () => {
    let elapsed = 0;
    const failure = await waitForEndpoint({
      url: 'http://127.0.0.1:1/',
      accept: (status) => status === 200,
      timeoutMs: 100,
      intervalMs: 10,
      probe: async () => 502,
      sleep: async (milliseconds) => {
        elapsed += milliseconds;
      },
      now: () => elapsed,
    }).then(
      () => null,
      (error: unknown) => error as EndpointTimeoutError,
    );

    expect(failure).toBeInstanceOf(EndpointTimeoutError);
    expect(failure?.url).toBe('http://127.0.0.1:1/');
    expect(failure?.timeoutMs).toBe(100);
    expect(failure?.message).toContain('http://127.0.0.1:1/');
  });

  it('returns as soon as the endpoint answers acceptably', async () => {
    let attempts = 0;

    await expect(
      waitForEndpoint({
        url: 'http://127.0.0.1:1/api/v1/status',
        accept: (status) => status === 401,
        timeoutMs: 1_000,
        intervalMs: 10,
        probe: async () => (++attempts < 3 ? null : 401),
        sleep: async () => {},
        now: () => 0,
      }),
    ).resolves.toBeUndefined();

    expect(attempts).toBe(3);
  });
});

describe('WSL-native runtime state', () => {
  it('places its temporary state below the distribution home directory', () => {
    expect(requireLinuxStateRoot('/home/dev')).toBe('/home/dev/.local/state/nilo-e2e');
  });

  it('refuses a home directory that resolves to a Windows drive mount', () => {
    expect(() => requireLinuxStateRoot('/mnt/f/home/dev')).toThrowError(/\/mnt\//);
    expect(() => requireLinuxStateRoot('')).toThrowError(/home directory/i);
  });
});

describe('token handling', () => {
  it('reads the issued token out of the backend output', () => {
    const output = '{"token_id":"t-1","user_id":"u-1","token":"issued-value"}\n';

    expect(parseIssuedToken(output)).toBe('issued-value');
  });

  it('never repeats the backend output in the error it raises', () => {
    const output = 'panic: something failed after printing plaintext-abc123';

    const failure = (() => {
      try {
        parseIssuedToken(output);
        return null;
      } catch (error) {
        return error as TokenIssueError;
      }
    })();

    expect(failure).toBeInstanceOf(TokenIssueError);
    expect(failure?.message ?? '').not.toContain('plaintext-abc123');
  });

  it('is never handed to console output anywhere in the harness', () => {
    expect(source).not.toMatch(/console\.[a-z]+\([^)]*token/i);
  });
});

describe('redacting the token out of a failed action', () => {
  // A failing Playwright fill() repeats its argument in the call log it puts on
  // error.message, but only once the locator has resolved - which the token
  // field always does. Turning off trace, video and screenshot does not help,
  // because that message goes to the console reporter and to the error-context
  // note. Every action carrying the token is therefore wrapped.
  const secret = 'DUMMY-MARKER-9f3a2b7c';

  it('returns the value untouched when the action succeeds', async () => {
    await expect(withRedactedSecret(secret, async () => 'done')).resolves.toBe('done');
  });

  it('strips the secret from the message of a failing action', async () => {
    const failure = await withRedactedSecret(secret, () => {
      throw new Error(`locator.fill: Timeout\nCall log:\n  - fill("${secret}")`);
    }).then(
      () => null,
      (error: unknown) => error as Error,
    );

    expect(failure?.message ?? '').not.toContain(secret);
    expect(failure?.message ?? '').toContain(REDACTED_TOKEN);
    expect(failure?.message ?? '').toContain('locator.fill: Timeout');
  });

  it('strips the secret from the stack of a failing action', async () => {
    const failure = await withRedactedSecret(secret, () => {
      const error = new Error('plain message');
      error.stack = `Error: plain message\n    at fill("${secret}")`;
      throw error;
    }).then(
      () => null,
      (error: unknown) => error as Error,
    );

    expect(failure?.stack ?? '').not.toContain(secret);
    expect(String(failure?.stack ?? '')).toContain(REDACTED_TOKEN);
  });

  it('strips the secret from a rejection that is not an Error', async () => {
    const failure = await withRedactedSecret(secret, () =>
      Promise.reject(`raw rejection carrying ${secret}`),
    ).then(
      () => null,
      (error: unknown) => error as Error,
    );

    expect(failure?.message ?? '').not.toContain(secret);
    expect(failure?.message ?? '').toContain(REDACTED_TOKEN);
  });

  it('leaves an empty secret alone rather than redacting everything', async () => {
    const failure = await withRedactedSecret('', () => {
      throw new Error('unrelated failure');
    }).then(
      () => null,
      (error: unknown) => error as Error,
    );

    expect(failure?.message).toBe('unrelated failure');
  });

  it('is the only way the harness types the token into a field', () => {
    // The field is handed in, and the value never leaves this module.
    expect(source).toContain('enterTokenInto');
    expect(source).toMatch(/withRedactedSecret\(this\.#token/);
    expect(source).not.toMatch(/get token\(\)/);
  });
});

describe('releasing what a run created', () => {
  it('signals only the processes the harness itself started', async () => {
    const backend = recordingHandle('backend');
    const frontend = recordingHandle('frontend');
    const calls: Array<{ file: string; args: readonly string[] }> = [];
    const run = async (
      file: string,
      args: readonly string[],
    ): Promise<CommandResult> => {
      calls.push({ file, args });
      return { code: 0, stdout: '', stderr: '' };
    };

    await releaseOwned(
      {
        distribution: 'Ubuntu-24.04',
        dataDirectory: '/home/dev/.local/state/nilo-e2e/run-1',
        children: [frontend, backend],
      },
      run,
    );

    expect(frontend.killed).toBe(1);
    expect(backend.killed).toBe(1);
    expect(calls).toHaveLength(1);
    expect(calls[0].args).toContain('Ubuntu-24.04');
    expect(calls[0].args.join(' ')).toContain('/home/dev/.local/state/nilo-e2e/run-1');
  });

  it('removes nothing when the run created no directory', async () => {
    const calls: string[] = [];
    const run = async (file: string): Promise<CommandResult> => {
      calls.push(file);
      return { code: 0, stdout: '', stderr: '' };
    };

    await releaseOwned(
      { distribution: 'Ubuntu-24.04', dataDirectory: null, children: [] },
      run,
    );

    expect(calls).toEqual([]);
  });

  it('attempts every step when one fails, then reports the failure', async () => {
    const angry: ProcessHandle = {
      name: 'backend',
      kill() {
        throw new Error('already gone');
      },
      exited: Promise.resolve(),
    };
    const frontend = recordingHandle('frontend');
    let removed = false;
    const run = async (): Promise<CommandResult> => {
      removed = true;
      return { code: 0, stdout: '', stderr: '' };
    };

    const failure = await releaseOwned(
      {
        distribution: 'Ubuntu-24.04',
        dataDirectory: '/home/dev/.local/state/nilo-e2e/run-2',
        children: [angry, frontend],
      },
      run,
    ).then(
      () => null,
      (error: unknown) => error as CleanupError,
    );

    // The remaining work still happened; the failure is reported, not swallowed.
    expect(frontend.killed).toBe(1);
    expect(removed).toBe(true);
    expect(failure).toBeInstanceOf(CleanupError);
    expect(failure?.message).toContain('backend');
  });

  it('fails when the release command exits non-zero', async () => {
    // A distribution that could not remove the directory has not removed it,
    // and a run that leaves a database behind has not cleaned up.
    const failure = await releaseOwned(
      {
        distribution: 'Ubuntu-24.04',
        dataDirectory: '/home/dev/.local/state/nilo-e2e/run-4',
        children: [],
      },
      async () => ({ code: 1, stdout: '', stderr: 'rm: permission denied' }),
    ).then(
      () => null,
      (error: unknown) => error as CleanupError,
    );

    expect(failure).toBeInstanceOf(CleanupError);
    expect(failure?.message).toContain('rm: permission denied');
    expect(failure?.dataDirectory).toBe('/home/dev/.local/state/nilo-e2e/run-4');
  });

  it('fails when the release command cannot be run at all', async () => {
    const failure = await releaseOwned(
      {
        distribution: 'Ubuntu-24.04',
        dataDirectory: '/home/dev/.local/state/nilo-e2e/run-5',
        children: [],
      },
      async () => {
        throw new Error('the distribution is not running');
      },
    ).then(
      () => null,
      (error: unknown) => error as CleanupError,
    );

    expect(failure).toBeInstanceOf(CleanupError);
    expect(failure?.message).toContain('the distribution is not running');
  });

  it('verifies the recorded process still belongs to this run before signalling it', () => {
    const script = buildReleaseScript('/home/dev/.local/state/nilo-e2e/run-3');

    expect(script).toContain('/proc/');
    expect(script).toContain('cmdline');
    expect(script).toContain('/home/dev/.local/state/nilo-e2e/run-3');
    expect(script).toMatch(/rm -rf/);
    expect(script.indexOf('kill')).toBeLessThan(script.indexOf('rm -rf'));
  });

  it('proves the directory is gone rather than trusting the removal', () => {
    // `rm -rf` succeeding is not the same as the directory being absent, so the
    // script checks and exits non-zero when anything survived.
    const script = buildReleaseScript('/home/dev/.local/state/nilo-e2e/run-3');
    const afterRemoval = script.slice(script.indexOf('rm -rf'));

    expect(afterRemoval).toMatch(/if \[ -e /);
    expect(afterRemoval).toMatch(/exit 1/);
  });

  it('never frees a port or matches a process by name', () => {
    expect(source).not.toMatch(/taskkill|pkill|killall|fuser|Stop-Process/i);
  });
});
