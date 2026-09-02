/**
 * The prerequisites the integrated browser test cannot supply for itself.
 *
 * Every check here fails the run. None of them skips it: a browser test that
 * quietly excuses itself turns a green gate into a claim that the browser
 * reached Bench Core under WSL2 when nothing at all was observed. Each failure
 * names the prerequisite and the repository command that provides it, so the
 * developer never has to work out what the harness wanted.
 */

/** A command result the checks read without caring how it was produced. */
export interface CommandOutcome {
  readonly code: number;
  readonly stdout: string;
  readonly stderr: string;
}

/** A prerequisite of the integrated browser test that is not satisfied. */
export class PrerequisiteError extends Error {
  /** The missing thing, named the way the developer would name it. */
  readonly prerequisite: string;
  /** The repository command that supplies it. */
  readonly remedy: string;

  constructor(prerequisite: string, detail: string, remedy: string) {
    super(`${prerequisite} is not available: ${detail}\nProvide it with: ${remedy}`);
    this.name = 'PrerequisiteError';
    this.prerequisite = prerequisite;
    this.remedy = remedy;
  }
}

const DEVELOPMENT_BOOTSTRAP = '.\\02_Tools\\bootstrap\\bootstrap-development.ps1';
const BROWSER_BOOTSTRAP = '.\\02_Tools\\bootstrap\\bootstrap-browser.ps1';
const WSL_DOCTOR = '.\\02_Tools\\scripts\\doctor-wsl.ps1';

/**
 * Choose the distribution that will run Bench Core.
 *
 * With several installed and none requested the run stops rather than guessing:
 * the wrong distribution would build against the wrong toolchain and report a
 * failure that has nothing to do with the change under test.
 */
export function selectDistribution(
  available: readonly string[],
  requested: string | undefined,
): string {
  const installed = available.map((name) => name.trim()).filter((name) => name !== '');
  const wanted = (requested ?? '').trim();

  if (wanted !== '') {
    if (!installed.includes(wanted)) {
      throw new PrerequisiteError(
        'The requested WSL2 distribution',
        `NILO_E2E_DISTRIBUTION names "${wanted}", but the installed distributions are ${describe(installed)}`,
        `${DEVELOPMENT_BOOTSTRAP} -Distribution ${wanted} -InstallWSL`,
      );
    }
    return wanted;
  }

  if (installed.length === 0) {
    throw new PrerequisiteError(
      'A WSL2 distribution to run Bench Core',
      'no WSL distribution is installed on this host',
      `${DEVELOPMENT_BOOTSTRAP} -InstallWSL`,
    );
  }
  if (installed.length > 1) {
    throw new PrerequisiteError(
      'One WSL2 distribution to run Bench Core',
      `several are installed (${installed.join(', ')}); name one in NILO_E2E_DISTRIBUTION`,
      `$env:NILO_E2E_DISTRIBUTION = '${installed[0]}'`,
    );
  }
  return installed[0];
}

/** Confirm the distribution can see this checkout, and return its Linux path. */
export function requireCheckoutInDistribution(outcome: CommandOutcome): string {
  const translated = outcome.stdout.trim();
  if (outcome.code !== 0 || translated === '') {
    throw new PrerequisiteError(
      'This checkout, reachable from the WSL2 distribution',
      `the distribution could not translate the repository path${detail(outcome.stderr)}`,
      WSL_DOCTOR,
    );
  }
  return translated;
}

/** Confirm the repository-local browser was installed, and return its directory. */
export function requireBrowserRuntime(
  browsersPath: string | undefined,
  exists: (path: string) => boolean,
): string {
  const directory = (browsersPath ?? '').trim();
  if (directory === '') {
    throw new PrerequisiteError(
      'The repository-local browser directory',
      'PLAYWRIGHT_BROWSERS_PATH is not set, so a browser would be taken from or written to a per-user cache outside this repository; run this command through .\\02_Tools\\env\\with-tools.ps1',
      BROWSER_BOOTSTRAP,
    );
  }
  if (!exists(directory)) {
    throw new PrerequisiteError(
      'The repository-local browser',
      `nothing is installed at ${directory}`,
      BROWSER_BOOTSTRAP,
    );
  }
  return directory;
}

function describe(installed: readonly string[]): string {
  return installed.length === 0 ? 'none' : installed.join(', ');
}

function detail(stderr: string): string {
  const trimmed = stderr.trim();
  return trimmed === '' ? '' : `: ${trimmed}`;
}
