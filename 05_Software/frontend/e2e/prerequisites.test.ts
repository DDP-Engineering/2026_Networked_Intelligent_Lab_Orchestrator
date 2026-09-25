import { describe, expect, it } from 'vitest';

import {
  PrerequisiteError,
  requireApplianceStage,
  requireBrowserRuntime,
  requireBuiltInterface,
} from './prerequisites';

/**
 * A cross-component test that quietly skips itself is worse than no test at
 * all, because a green gate then claims the browser reached an appliance when
 * nothing was observed. Every missing prerequisite must stop the run and say
 * which repository command supplies it.
 */

const PRESENT = (): boolean => true;
const ABSENT = (): boolean => false;

const STAGE_ENVIRONMENT = {
  NILO_APPLIANCE_STAGE: '1',
  NILO_NILO_BINARY: '/tmp/nilo',
  NILO_NETHELPER_BINARY: '/tmp/nilo-nethelper',
  NILO_APPLIANCE_BROWSER_NAMESPACE: 'nilo-browser',
  NILO_APPLIANCE_MANAGEMENT_INTERFACE: 'nilo-browser0',
  NILO_APPLIANCE_MANAGEMENT_HOST: '198.18.0.1',
};

describe('a run started outside the appliance verification stage', () => {
  it('fails the run and names the stage that provides an appliance', () => {
    const failure = capture(() => requireApplianceStage({}, PRESENT));

    expect(failure).toBeInstanceOf(PrerequisiteError);
    expect(failure?.prerequisite).toContain('appliance');
    expect(failure?.message).toContain('02_Tools/tests/appliance/check.sh');
  });

  it('explains why the suite cannot run anywhere else', () => {
    const failure = capture(() => requireApplianceStage({}, PRESENT));

    expect(failure?.message).toContain('privileged helper');
    expect(failure?.message).toContain('NetworkManager');
  });
});

describe('the binaries the run drives', () => {
  it('fails when the stage named none', () => {
    const failure = capture(() =>
      requireApplianceStage({ NILO_APPLIANCE_STAGE: '1' }, PRESENT),
    );

    expect(failure).toBeInstanceOf(PrerequisiteError);
    expect(failure?.message).toContain('NILO_NILO_BINARY');
  });

  it('fails when a named binary does not exist', () => {
    const failure = capture(() => requireApplianceStage(STAGE_ENVIRONMENT, ABSENT));

    expect(failure).toBeInstanceOf(PrerequisiteError);
    expect(failure?.message).toContain('/tmp/nilo');
  });

  it('returns the binaries and isolated browser ingress the stage supplied', () => {
    expect(requireApplianceStage(STAGE_ENVIRONMENT, PRESENT)).toEqual({
      binary: '/tmp/nilo',
      helperBinary: '/tmp/nilo-nethelper',
      browserNamespace: 'nilo-browser',
      managementInterface: 'nilo-browser0',
      managementHost: '198.18.0.1',
    });
  });

  it('fails when the stage did not name its isolated browser ingress', () => {
    const failure = capture(() =>
      requireApplianceStage(
        {
          NILO_APPLIANCE_STAGE: '1',
          NILO_NILO_BINARY: '/tmp/nilo',
          NILO_NETHELPER_BINARY: '/tmp/nilo-nethelper',
        },
        PRESENT,
      ),
    );

    expect(failure).toBeInstanceOf(PrerequisiteError);
    expect(failure?.message).toContain('NILO_APPLIANCE_BROWSER_NAMESPACE');
  });
});

describe('the built interface the appliance serves', () => {
  it('fails when nothing was built, rather than falling back to a dev server', () => {
    const failure = capture(() => requireBuiltInterface({}, '/gui', ABSENT));

    expect(failure).toBeInstanceOf(PrerequisiteError);
    expect(failure?.message).toContain('/gui/dist');
    expect(failure?.message).toContain('development server');
  });

  it('fails when the build carries no entry document', () => {
    const failure = capture(() =>
      requireBuiltInterface({}, '/gui', (path) => !path.endsWith('index.html')),
    );

    expect(failure).toBeInstanceOf(PrerequisiteError);
    expect(failure?.message).toContain('index.html');
  });

  it('prefers the directory the stage named', () => {
    expect(requireBuiltInterface({ NILO_GUI_DIR: '/built' }, '/gui', PRESENT)).toBe(
      '/built',
    );
  });

  it('defaults to the interface project build output', () => {
    expect(requireBuiltInterface({}, '/gui', PRESENT)).toBe('/gui/dist');
  });
});

describe('a missing browser engine', () => {
  it('fails the run when no directory is configured', () => {
    const failure = capture(() => requireBrowserRuntime(undefined, PRESENT));

    expect(failure).toBeInstanceOf(PrerequisiteError);
    expect(failure?.message).toContain('PLAYWRIGHT_BROWSERS_PATH');
  });

  it('fails the run when the configured directory is empty', () => {
    const failure = capture(() =>
      requireBrowserRuntime('/opt/nilo/playwright', ABSENT),
    );

    expect(failure).toBeInstanceOf(PrerequisiteError);
    expect(failure?.message).toContain('/opt/nilo/playwright');
  });

  it('returns the directory when a browser is installed', () => {
    expect(requireBrowserRuntime('/opt/nilo/playwright', PRESENT)).toBe(
      '/opt/nilo/playwright',
    );
  });
});

function capture(action: () => unknown): PrerequisiteError | null {
  try {
    action();
    return null;
  } catch (failure) {
    return failure as PrerequisiteError;
  }
}
