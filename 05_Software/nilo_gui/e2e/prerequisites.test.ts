import { describe, expect, it } from 'vitest';

import {
  PrerequisiteError,
  requireBrowserRuntime,
  requireCheckoutInDistribution,
  selectDistribution,
} from './prerequisites';

/**
 * A cross-component test that quietly skips itself is worse than no test at
 * all, because a green gate then claims the browser reached WSL2 when nothing
 * was observed. Every missing prerequisite must stop the run and say which
 * repository command supplies it.
 */

describe('a missing WSL2 distribution', () => {
  it('fails the run and names the bootstrap that installs one', () => {
    const failure = capture(() => selectDistribution([], undefined));

    expect(failure).toBeInstanceOf(PrerequisiteError);
    expect(failure?.prerequisite).toContain('WSL2');
    expect(failure?.message).toContain('bootstrap-development.ps1');
  });

  it('fails the run rather than guessing between several distributions', () => {
    const failure = capture(() =>
      selectDistribution(['Ubuntu-24.04', 'Debian'], undefined),
    );

    expect(failure).toBeInstanceOf(PrerequisiteError);
    expect(failure?.message).toContain('Ubuntu-24.04');
    expect(failure?.message).toContain('Debian');
    expect(failure?.message).toContain('NILO_E2E_DISTRIBUTION');
  });

  it('uses the only installed distribution, or the one explicitly requested', () => {
    expect(selectDistribution(['Ubuntu-24.04'], undefined)).toBe('Ubuntu-24.04');
    expect(selectDistribution(['Ubuntu-24.04', 'Debian'], 'Debian')).toBe('Debian');
  });

  it('fails when the requested distribution is not installed', () => {
    const failure = capture(() => selectDistribution(['Ubuntu-24.04'], 'Missing'));

    expect(failure).toBeInstanceOf(PrerequisiteError);
    expect(failure?.message).toContain('Missing');
  });
});

describe('a checkout the distribution cannot see', () => {
  it('fails the run and names the diagnostic that proves the mapping', () => {
    const failure = capture(() =>
      requireCheckoutInDistribution({ code: 1, stdout: '', stderr: 'no such path' }),
    );

    expect(failure).toBeInstanceOf(PrerequisiteError);
    expect(failure?.prerequisite).toContain('checkout');
    expect(failure?.message).toContain('doctor-wsl.ps1');
  });

  it('returns the translated path when the distribution can see the checkout', () => {
    expect(
      requireCheckoutInDistribution({
        code: 0,
        stdout: '/mnt/f/2026_Networked_Intelligent_Lab_Orchestrator\n',
        stderr: '',
      }),
    ).toBe('/mnt/f/2026_Networked_Intelligent_Lab_Orchestrator');
  });
});

describe('a browser that was never installed', () => {
  it('fails the run and names the bootstrap that installs it', () => {
    const failure = capture(() =>
      requireBrowserRuntime(
        'F:/repo/02_Tools/runtime/windows-x86_64/playwright',
        () => false,
      ),
    );

    expect(failure).toBeInstanceOf(PrerequisiteError);
    expect(failure?.prerequisite).toContain('browser');
    expect(failure?.message).toContain('bootstrap-browser.ps1');
  });

  it('fails when no repository-local browser directory was configured at all', () => {
    const failure = capture(() => requireBrowserRuntime(undefined, () => true));

    expect(failure).toBeInstanceOf(PrerequisiteError);
    expect(failure?.message).toContain('PLAYWRIGHT_BROWSERS_PATH');
  });

  it('returns the repository-local directory when a browser is present', () => {
    const directory = 'F:/repo/02_Tools/runtime/windows-x86_64/playwright';

    expect(requireBrowserRuntime(directory, (path) => path === directory)).toBe(
      directory,
    );
  });
});

describe('the prerequisite report as a whole', () => {
  it('never resolves to a skip', () => {
    for (const failing of [
      () => selectDistribution([], undefined),
      () => requireCheckoutInDistribution({ code: 1, stdout: '', stderr: '' }),
      () => requireBrowserRuntime(undefined, () => true),
    ]) {
      expect(failing).toThrowError(PrerequisiteError);
    }
  });
});

function capture(action: () => unknown): PrerequisiteError | null {
  try {
    action();
    return null;
  } catch (error) {
    return error as PrerequisiteError;
  }
}
