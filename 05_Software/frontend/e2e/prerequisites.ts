/**
 * The prerequisites the integrated browser test cannot supply for itself.
 *
 * Every check here fails the run. None of them skips it: a browser test that
 * quietly excuses itself turns a green gate into a claim that the browser
 * reached an appliance when nothing at all was observed. Each failure names the
 * prerequisite and the repository command that provides it, so the developer
 * never has to work out what the harness wanted.
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

/** The one command that runs this suite, named in every refusal. */
export const APPLIANCE_STAGE = './02_Tools/tests/appliance/check.sh';

/** The binaries an appliance run needs, as the stage builds them. */
export interface ApplianceBinaries {
  readonly binary: string;
  readonly helperBinary: string;
  /** Network namespace from which browser traffic enters the appliance. */
  readonly browserNamespace: string;
  /** Interface where the isolated browser enters the appliance namespace. */
  readonly managementInterface: string;
  /** Address on that interface, covered by the run's temporary certificate. */
  readonly managementHost: string;
}

/**
 * Confirm this run is inside the appliance verification stage.
 *
 * The suite drives Bench Core on the appliance profile, which needs the real
 * privileged helper and a real NetworkManager. Those exist in the stage
 * container and nowhere else, so a run started anywhere else is refused rather
 * than allowed to fail obscurely half a minute later.
 */
export function requireApplianceStage(
  environment: Record<string, string | undefined>,
  exists: (path: string) => boolean,
): ApplianceBinaries {
  if ((environment.NILO_APPLIANCE_STAGE ?? '').trim() !== '1') {
    throw new PrerequisiteError(
      'The appliance verification stage',
      'this suite drives Bench Core on the appliance profile, which needs the privileged helper and a real NetworkManager; it runs inside the stage container and nowhere else',
      APPLIANCE_STAGE,
    );
  }
  const binary = (environment.NILO_NILO_BINARY ?? '').trim();
  const helperBinary = (environment.NILO_NETHELPER_BINARY ?? '').trim();
  for (const [name, value] of [
    ['NILO_NILO_BINARY', binary],
    ['NILO_NETHELPER_BINARY', helperBinary],
  ] as const) {
    if (value === '') {
      throw new PrerequisiteError(
        'The Bench Core binaries this run drives',
        `${name} is not set, so the run has nothing to start`,
        APPLIANCE_STAGE,
      );
    }
    if (!exists(value)) {
      throw new PrerequisiteError(
        'The Bench Core binaries this run drives',
        `${name} names ${value}, which does not exist`,
        APPLIANCE_STAGE,
      );
    }
  }
  const managementInterface = (
    environment.NILO_APPLIANCE_MANAGEMENT_INTERFACE ?? ''
  ).trim();
  const managementHost = (environment.NILO_APPLIANCE_MANAGEMENT_HOST ?? '').trim();
  const browserNamespace = (environment.NILO_APPLIANCE_BROWSER_NAMESPACE ?? '').trim();
  if (!/^[A-Za-z0-9][A-Za-z0-9_.-]{0,31}$/.test(browserNamespace)) {
    throw new PrerequisiteError(
      'The appliance browser namespace',
      'NILO_APPLIANCE_BROWSER_NAMESPACE does not name the stage namespace that contains Chromium',
      APPLIANCE_STAGE,
    );
  }
  if (!/^[A-Za-z0-9][A-Za-z0-9_.-]{0,14}$/.test(managementInterface)) {
    throw new PrerequisiteError(
      'The appliance browser ingress',
      'NILO_APPLIANCE_MANAGEMENT_INTERFACE does not name the stage interface where browser traffic enters',
      APPLIANCE_STAGE,
    );
  }
  if (!/^\d{1,3}(?:\.\d{1,3}){3}$/.test(managementHost)) {
    throw new PrerequisiteError(
      'The appliance browser ingress',
      'NILO_APPLIANCE_MANAGEMENT_HOST does not name the stage IPv4 address the browser reaches',
      APPLIANCE_STAGE,
    );
  }
  return {
    binary,
    helperBinary,
    browserNamespace,
    managementInterface,
    managementHost,
  };
}

/**
 * Confirm the interface was built, and return the directory Bench Core serves.
 *
 * The appliance serves its own assets; there is no development server in this
 * proof. An unbuilt interface is a missing prerequisite, not a reason to fall
 * back to something else.
 */
export function requireBuiltInterface(
  environment: Record<string, string | undefined>,
  guiRoot: string,
  exists: (path: string) => boolean,
): string {
  const declared = (environment.NILO_GUI_DIR ?? '').trim();
  const directory = declared === '' ? `${guiRoot}/dist` : declared;
  if (!exists(directory)) {
    throw new PrerequisiteError(
      'The built interface the appliance serves',
      `nothing is built at ${directory}, and this suite drives the assets an appliance installs rather than a development server`,
      APPLIANCE_STAGE,
    );
  }
  if (!exists(`${directory}/index.html`)) {
    throw new PrerequisiteError(
      'The built interface the appliance serves',
      `${directory} carries no index.html, so Bench Core has no entry document to serve`,
      APPLIANCE_STAGE,
    );
  }
  return directory;
}

/** Confirm a browser engine is installed, and return its directory. */
export function requireBrowserRuntime(
  browsersPath: string | undefined,
  exists: (path: string) => boolean,
): string {
  const directory = (browsersPath ?? '').trim();
  if (directory === '') {
    throw new PrerequisiteError(
      'The browser engine directory',
      'PLAYWRIGHT_BROWSERS_PATH is not set, so a browser would be taken from or written to a per-user cache outside this run',
      APPLIANCE_STAGE,
    );
  }
  if (!exists(directory)) {
    throw new PrerequisiteError(
      'The browser engine',
      `nothing is installed at ${directory}`,
      APPLIANCE_STAGE,
    );
  }
  return directory;
}
