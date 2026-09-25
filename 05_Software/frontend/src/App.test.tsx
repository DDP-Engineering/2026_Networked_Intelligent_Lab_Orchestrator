import packageJson from '../package.json';
import { fireEvent, screen, waitFor, within } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import App from './App';
import { BenchCoreError } from './api/client';
import type { BenchCoreClient, BenchCoreFailure, Credential } from './api/client';
import { PREFERENCE_KEYS } from './preferences/store';
import { renderInShell } from './test/render';
import type {
  BenchCoreSession,
  BenchCoreStatus,
  DeviceInventory,
  IntegrationInventory,
} from './api/contract';

// The interface reports the project version; a bump must not silently
// break these assertions, so they read the same source the app does.
const APP_VERSION: string = packageJson.version;

const healthyStatus: BenchCoreStatus = {
  apiVersion: 'nilo.http/1',
  status: 'healthy',
  profile: 'simulation',
  simulation: true,
  build: { name: 'nilo', version: '0.1.0', integrationProtocol: 'nilo.integration/1' },
};

const readyIntegrations: IntegrationInventory = {
  apiVersion: 'nilo.http/1',
  scenario: 'healthy',
  items: [
    {
      id: 'simulation.bench',
      displayName: 'SIMULATION - Demo Integration',
      status: 'healthy',
      simulation: true,
      operations: ['artifact', 'crash', 'echo', 'fail', 'sleep'],
    },
  ],
};

const readyDevices: DeviceInventory = {
  apiVersion: 'nilo.http/1',
  scenario: 'healthy',
  items: [
    {
      id: 'simulation.bench.device.stable-001',
      integrationId: 'simulation.bench',
      displayName: 'SIMULATION - Electronics Bench 001',
      kind: 'simulated-electronics-bench',
      status: 'ready',
      simulation: true,
      capabilities: ['artifact', 'crash', 'echo', 'fail', 'sleep'],
    },
  ],
};

const unavailableIntegrations: IntegrationInventory = {
  ...readyIntegrations,
  scenario: 'unavailable',
  items: readyIntegrations.items.map((item) => ({ ...item, status: 'unavailable' })),
};

const unavailableDevices: DeviceInventory = {
  ...readyDevices,
  scenario: 'unavailable',
  items: readyDevices.items.map((item) => ({ ...item, status: 'unavailable' })),
};

const administratorSession: BenchCoreSession = {
  apiVersion: 'nilo.http/1',
  username: 'admin',
  role: 'administrator',
  permissions: [
    'status.read',
    'inventory.read',
    'users.read',
    'users.manage',
    'tokens.read.all',
    'tokens.manage.all',
  ],
  credential: 'session',
  csrfToken: 'csrf-token-fixture',
};

const viewerSession: BenchCoreSession = {
  apiVersion: 'nilo.http/1',
  username: 'bench-viewer',
  role: 'viewer',
  permissions: ['status.read', 'inventory.read'],
  credential: 'session',
  csrfToken: 'csrf-token-fixture',
};

/** The identity a development bearer token resolves to: a least-privileged Viewer. */
const tokenSession: BenchCoreSession = {
  ...viewerSession,
  username: 'development-identity',
  credential: 'token',
  csrfToken: null,
};

const ENDPOINT = 'http://127.0.0.1:8090';

const BOUNDARY = /never represent evidence from physical equipment/i;

const noSession = () =>
  Promise.reject(new BenchCoreError('unauthorized', 'no session', 'unauthorized'));

/**
 * A client that answers everything healthily unless an override says otherwise.
 *
 * By default the browser holds no session cookie, so restoration is refused and
 * the interface presents the sign-in screen; a bearer credential resolves to the
 * Viewer identity a development token really carries.
 */
function stubClient(overrides: Partial<BenchCoreClient> = {}): BenchCoreClient {
  return {
    baseUrl: '',
    requestPower: () => Promise.reject(new Error('not part of this fixture')),
    listPlugins: async () => [],
    listInstrumentJobs: async () => [],
    getInstrumentJob: async () => {
      throw new Error('unavailable');
    },
    pluginAction: async () => {
      throw new Error('unavailable');
    },
    discoverInstruments: async () => [],
    submitInstrumentJob: async () => {
      throw new Error('unavailable');
    },
    instrumentJobAction: async () => {
      throw new Error('unavailable');
    },
    downloadInstrumentArtifact: async () => {
      throw new Error('unavailable');
    },
    listNetworkOperations: () => Promise.resolve([]),
    previewInterfaceSettings: () =>
      Promise.reject(new Error('not part of this fixture')),
    scanWiFi: () => Promise.reject(new Error('not part of this fixture')),
    previewWiFiJoin: () => Promise.reject(new Error('not part of this fixture')),
    listDeviceNetworks: () => Promise.resolve([]),
    previewDeviceNetworkChange: () =>
      Promise.reject(new Error('not part of this fixture')),
    checkDeviceNetwork: () => Promise.reject(new Error('not part of this fixture')),
    getAccessInstructions: () => Promise.reject(new Error('not part of this fixture')),
    listActivity: () => Promise.reject(new Error('not part of this fixture')),
    exportActivity: () => Promise.reject(new Error('not part of this fixture')),
    getActivityRetention: () => Promise.reject(new Error('not part of this fixture')),
    setActivityRetention: () => Promise.reject(new Error('not part of this fixture')),
    getPasswordPolicy: () =>
      Promise.resolve({
        minimumLength: 12,
        requireLowercase: false,
        requireUppercase: false,
        requireDigit: false,
        requireSpecial: false,
      }),
    setPasswordPolicy: (_credential, policy) => Promise.resolve(policy),
    getStatus: () => Promise.resolve(healthyStatus),
    getIntegrations: () => Promise.resolve(readyIntegrations),
    getDevices: () => Promise.resolve(readyDevices),
    getWireGuardStatus: () => Promise.reject(new Error('no network')),
    previewWireGuardChange: () => Promise.reject(new Error('no network')),
    getNetworkOperation: () => Promise.reject(new Error('no network')),
    getNetworkStatus: () =>
      Promise.reject(new BenchCoreError('unavailable', 'no network')),
    getManagementAccess: () => Promise.reject(new Error('no management access')),
    setManagementAccess: () => Promise.reject(new Error('no management access')),
    confirmManagementAccess: () => Promise.reject(new Error('no management access')),
    recoverManagementAccess: () => Promise.reject(new Error('no management access')),
    completeCommissioning: () => Promise.reject(new Error('not part of this fixture')),
    getNetworkStatistics: () =>
      Promise.reject(new BenchCoreError('unavailable', 'no network')),
    previewInterfaceChange: () =>
      Promise.reject(new BenchCoreError('unavailable', 'no network')),
    acknowledgeNetworkOperation: () =>
      Promise.reject(new BenchCoreError('unavailable', 'no network')),
    confirmNetworkOperation: () =>
      Promise.reject(new BenchCoreError('unavailable', 'no network')),
    rollbackNetworkOperation: () =>
      Promise.reject(new BenchCoreError('unavailable', 'no network')),
    signIn: () => noSession(),
    currentSession: (credential: Credential) =>
      credential.kind === 'bearer' ? Promise.resolve(tokenSession) : noSession(),
    signOut: () => Promise.resolve(),
    listIdentities: () => Promise.resolve({ apiVersion: 'nilo.http/1', items: [] }),
    createIdentity: () =>
      Promise.reject(new BenchCoreError('forbidden', 'not in this fixture')),
    setIdentityRole: () => Promise.resolve(),
    setIdentityEnabled: () => Promise.resolve(),
    resetIdentityPassword: () => Promise.resolve(),
    listTokens: () => Promise.resolve({ apiVersion: 'nilo.http/1', items: [] }),
    createToken: () =>
      Promise.reject(new BenchCoreError('forbidden', 'not in this fixture')),
    revokeToken: () => Promise.resolve(),
    deleteIdentity: () => Promise.resolve(),
    deleteToken: () => Promise.resolve(),
    ...overrides,
  };
}

/** A client whose browser already holds a usable session for this identity. */
function signedInClient(
  session: BenchCoreSession,
  overrides: Partial<BenchCoreClient> = {},
): BenchCoreClient {
  return stubClient({ currentSession: () => Promise.resolve(session), ...overrides });
}

function rejecting(failure: BenchCoreFailure, detail = `simulated ${failure}`) {
  return () => Promise.reject(new BenchCoreError(failure, detail));
}

/** Every request fails the same way, as a stopped backend or a bad token does. */
function failing(failure: BenchCoreFailure): BenchCoreClient {
  return stubClient({
    currentSession: rejecting(failure),
    getStatus: rejecting(failure),
    getIntegrations: rejecting(failure),
    getDevices: rejecting(failure),
  });
}

function renderApp(client: BenchCoreClient, apiMode: 'live' | 'mock' = 'live') {
  return renderInShell(<App client={client} apiMode={apiMode} endpoint={ENDPOINT} />, {
    theme: 'light',
    language: 'en',
  });
}

/** The Bench page heading, which is what "connected" now looks like. */
function benchPage(): Promise<HTMLElement> {
  return screen.findByRole('heading', { name: 'Bench', level: 2 });
}

/** Open the account menu, where sign-out and the role now live. */
function openAccountMenu(): void {
  fireEvent.click(screen.getByRole('button', { name: 'Account' }));
}

/**
 * Render a page load that arrives with the cookies a signed-in browser holds.
 *
 * The session secret itself is `HttpOnly` and therefore invisible to both this
 * page and this test; the readable cross-site request forgery cookie is the one
 * thing restoration can and must pick up for itself.
 */
function renderRestoredApp(client: BenchCoreClient) {
  document.cookie = 'nilo_csrf=csrf-token-fixture';
  return renderApp(client);
}

/** Wait for the start-up session probe to settle on the sign-in screen. */
function signInScreen(): Promise<HTMLElement> {
  return screen.findByRole('heading', { name: /sign in to nilo/i });
}

async function connectWith(client: BenchCoreClient): Promise<void> {
  client.signIn = () => Promise.resolve(viewerSession);
  await signInAs(client, viewerSession.username, 'viewer-secret');
}

async function signInAs(
  client: BenchCoreClient,
  username = 'admin',
  password = 'administrator-secret',
): Promise<void> {
  renderApp(client);
  await signInScreen();
  fireEvent.change(screen.getByLabelText('Username'), { target: { value: username } });
  fireEvent.change(screen.getByLabelText('Password'), { target: { value: password } });
  fireEvent.click(screen.getByRole('button', { name: /^sign in$/i }));
}

function integrationsRegion(): HTMLElement {
  return screen.getByRole('region', { name: /^integrations$/i });
}

function devicesRegion(): HTMLElement {
  return screen.getByRole('region', { name: /^devices$/i });
}

function expectSimulationIdentity(): void {
  if (document.querySelector('.shell')) {
    expect(screen.getByRole('status', { name: /runtime profile/i })).toHaveTextContent(
      'SIM',
    );
    expect(screen.queryByText(BOUNDARY)).toBeNull();
  } else {
    expect(screen.queryByRole('status', { name: /runtime profile/i })).toBeNull();
    expect(screen.queryByText(BOUNDARY)).toBeNull();
  }
}

afterEach(() => {
  vi.restoreAllMocks();
  localStorage.clear();
  sessionStorage.clear();
  document.cookie = 'nilo_csrf=; expires=Thu, 01 Jan 1970 00:00:00 GMT';
});

describe('sign-in screen', () => {
  it('presents the final product login and canonical version as the only credential path', async () => {
    const client = stubClient();
    const getStatus = vi.spyOn(client, 'getStatus');
    renderApp(client);

    await signInScreen();
    expect(screen.getByText('NILO')).toBeVisible();
    expect(screen.getByText(`Version ${APP_VERSION}`)).toBeVisible();
    expect(document.querySelectorAll('form')).toHaveLength(1);
    expect(screen.getByLabelText('Username')).toBeVisible();
    expect(screen.getByLabelText('Password')).toHaveAttribute('type', 'password');
    expect(screen.getByRole('button', { name: /^sign in$/i })).toBeVisible();
    expect(screen.queryByText(/development interface/i)).toBeNull();
    expect(screen.queryByLabelText(/development bearer token/i)).toBeNull();
    expect(screen.queryByText(ENDPOINT)).toBeNull();
    expect(screen.queryByRole('status', { name: /runtime profile/i })).toBeNull();
    expect(getStatus).not.toHaveBeenCalled();
  });

  it('reveals and hides the entered password without changing the submitted value', async () => {
    const client = stubClient({ signIn: () => Promise.resolve(administratorSession) });
    const signIn = vi.spyOn(client, 'signIn');
    renderApp(client);
    await signInScreen();

    const password = screen.getByLabelText('Password');
    fireEvent.change(screen.getByLabelText('Username'), { target: { value: 'admin' } });
    fireEvent.change(password, { target: { value: 'administrator-secret' } });

    const show = screen.getByRole('button', { name: 'Show password' });
    expect(show).toHaveAttribute('aria-pressed', 'false');
    fireEvent.click(show);
    expect(password).toHaveAttribute('type', 'text');
    expect(password).toHaveValue('administrator-secret');

    const hide = screen.getByRole('button', { name: 'Hide password' });
    expect(hide).toHaveAttribute('aria-pressed', 'true');
    fireEvent.click(hide);
    expect(password).toHaveAttribute('type', 'password');
    expect(password).toHaveValue('administrator-secret');

    fireEvent.click(screen.getByRole('button', { name: /^sign in$/i }));
    expect(signIn).toHaveBeenCalledExactlyOnceWith('admin', 'administrator-secret');
  });

  it('refuses an incomplete sign-in without contacting the backend', async () => {
    const client = stubClient();
    const signIn = vi.spyOn(client, 'signIn');
    renderApp(client);
    await signInScreen();

    fireEvent.click(screen.getByRole('button', { name: /^sign in$/i }));

    expect(
      await screen.findByText(/enter both a username and a password/i),
    ).toBeVisible();
    expect(signIn).not.toHaveBeenCalled();
  });
});

describe('human sign-in and session', () => {
  it('signs in with a username and password and shows who is signed in', async () => {
    const client = stubClient({ signIn: () => Promise.resolve(administratorSession) });
    const signIn = vi.spyOn(client, 'signIn');

    await signInAs(client, 'admin', 'administrator-secret');

    expect(await benchPage()).toBeVisible();
    expect(signIn).toHaveBeenCalledExactlyOnceWith('admin', 'administrator-secret');
    expect(screen.getByRole('button', { name: 'Account' })).toHaveTextContent('admin');
    expectSimulationIdentity();
  });

  it('uses the session credential and its CSRF token for every later request', async () => {
    const client = stubClient({ signIn: () => Promise.resolve(administratorSession) });
    const getStatus = vi.spyOn(client, 'getStatus');

    await signInAs(client);

    await waitFor(() => {
      expect(getStatus).toHaveBeenCalledWith({
        kind: 'session',
        csrfToken: 'csrf-token-fixture',
      });
    });
  });

  it('stays on the sign-in screen when the credentials are refused', async () => {
    await signInAs(stubClient({ signIn: rejecting('unauthorized') }));

    expect(
      await screen.findByText(/that username and password were not accepted/i),
    ).toBeVisible();
    expect(screen.getByRole('heading', { name: /sign in to nilo/i })).toBeVisible();
  });

  it('says a throttled username has to wait rather than be retyped', async () => {
    await signInAs(
      stubClient({
        signIn: () =>
          Promise.reject(
            new BenchCoreError('rejected', 'too many', 'too_many_attempts'),
          ),
      }),
    );

    expect(await screen.findByText(/wait for the lock-out window/i)).toBeVisible();
  });

  it('restores the signed-in state on a fresh page without asking again', async () => {
    const client = signedInClient(administratorSession);
    const signIn = vi.spyOn(client, 'signIn');

    renderRestoredApp(client);

    expect(await benchPage()).toBeVisible();
    expect(screen.getByRole('button', { name: 'Account' })).toHaveTextContent('admin');
    expect(screen.queryByRole('heading', { name: /sign in to nilo/i })).toBeNull();
    expect(signIn).not.toHaveBeenCalled();
  });

  it('presents the sign-in screen when no session can be restored', async () => {
    renderApp(stubClient());

    expect(await signInScreen()).toBeVisible();
    expect(screen.queryByRole('heading', { name: 'Bench', level: 2 })).toBeNull();
  });

  it('ends the session server-side when the human signs out', async () => {
    const client = signedInClient(administratorSession);
    const signOut = vi.spyOn(client, 'signOut');
    renderRestoredApp(client);
    await benchPage();

    openAccountMenu();
    fireEvent.click(screen.getByRole('menuitem', { name: /sign out/i }));

    expect(await signInScreen()).toBeVisible();
    expect(screen.getByText(/you are signed out/i)).toBeVisible();
    await waitFor(() => {
      expect(signOut).toHaveBeenCalledWith({
        kind: 'session',
        csrfToken: 'csrf-token-fixture',
      });
    });
  });

  it('returns to the sign-in screen when the session has ended', async () => {
    const client = signedInClient(administratorSession, {
      getStatus: rejecting('unauthorized'),
    });

    renderRestoredApp(client);

    expect(await screen.findByText(/your session ended/i)).toBeVisible();
    expect(screen.getByRole('heading', { name: /sign in to nilo/i })).toBeVisible();
  });

  it('keeps a forbidden identity signed in rather than sending it back to sign in', async () => {
    const client = signedInClient(viewerSession, {
      getStatus: rejecting('forbidden', 'inventory.read is missing'),
    });

    renderRestoredApp(client);

    expect(await screen.findByText(/may not read the bench/i)).toBeVisible();
    expect(screen.queryByRole('heading', { name: /sign in to nilo/i })).toBeNull();
  });
});

describe('main navigation', () => {
  it('offers the Authentication page to an administrator', async () => {
    renderRestoredApp(signedInClient(administratorSession));
    await benchPage();

    fireEvent.click(
      within(screen.getByRole('navigation', { name: 'Main' })).getByRole('button', {
        name: 'Authentication',
      }),
    );

    expect(await screen.findByRole('region', { name: 'Users' })).toBeVisible();
    expect(screen.queryByRole('region', { name: 'API Tokens' })).toBeNull();
    expectSimulationIdentity();
  });

  it('omits the Authentication entry for an identity that may not administer', async () => {
    renderRestoredApp(signedInClient(viewerSession));
    await benchPage();

    const navigation = within(screen.getByRole('navigation', { name: 'Main' }));
    expect(navigation.getByRole('button', { name: /bench/i })).toBeVisible();
    expect(navigation.queryByRole('button', { name: /authentication/i })).toBeNull();
  });
});

describe('connection states', () => {
  it('shows a distinct in-progress state while the requests are pending', async () => {
    await connectWith(
      stubClient({ getStatus: () => new Promise<BenchCoreStatus>(() => {}) }),
    );

    expect(await screen.findByText(/contacting bench core/i)).toBeVisible();
    expect(screen.queryByRole('heading', { name: 'Bench', level: 2 })).toBeNull();
    expectSimulationIdentity();
  });

  it('shows the reported status once Bench Core answers', async () => {
    await connectWith(stubClient());

    expect(await benchPage()).toBeVisible();
    const facts = within(document.querySelector('.fact-grid') as HTMLElement);
    expect(facts.getByText('simulation')).toBeVisible();
    expect(facts.getByText('healthy')).toBeVisible();
    expect(facts.getByText('0.1.0')).toBeVisible();
    expect(screen.getByRole('button', { name: /refresh/i })).toBeVisible();
    expectSimulationIdentity();
  });

  it('shows a distinct unreachable-backend state', async () => {
    await connectWith(failing('unavailable'));

    expect(await screen.findByText(/is not reachable/i)).toBeVisible();
    expect(screen.queryByText(/rejected this token/i)).toBeNull();
    expect(screen.getByRole('button', { name: /try again/i })).toBeVisible();
    expectSimulationIdentity();
  });

  it('shows a distinct unsupported-status state', async () => {
    await connectWith(failing('malformed'));

    expect(await screen.findByText(/unsupported status response/i)).toBeVisible();
    expect(screen.queryByText(/is not reachable/i)).toBeNull();
    expect(screen.getByRole('button', { name: /try again/i })).toBeVisible();
    expectSimulationIdentity();
  });

  it('retries every resource with the same session when the backend was unreachable', async () => {
    let attempts = 0;
    const client = stubClient({
      getStatus: () => {
        attempts += 1;
        return attempts === 1
          ? Promise.reject(new BenchCoreError('unavailable', 'simulated outage'))
          : Promise.resolve(healthyStatus);
      },
    });
    const getDevices = vi.spyOn(client, 'getDevices');
    await connectWith(client);

    fireEvent.click(await screen.findByRole('button', { name: /try again/i }));

    expect(await benchPage()).toBeVisible();
    // Initial failure, explicit retry, then the shell's reachability refresh.
    expect(attempts).toBe(3);
    expect(getDevices).toHaveBeenLastCalledWith({
      kind: 'session',
      csrfToken: viewerSession.csrfToken,
    });
  });

  it('keeps the signed-out surface focused on product sign-in', async () => {
    renderApp(stubClient());

    await signInScreen();
    expectSimulationIdentity();
    expect(screen.getByRole('heading', { name: /sign in to nilo/i })).toBeVisible();
    expect(screen.getByText(`Version ${APP_VERSION}`)).toBeVisible();
  });
});

describe('simulated inventory', () => {
  it('shows every integration the backend reports, with its state', async () => {
    await connectWith(stubClient());

    const integrations = within(
      await screen.findByRole('region', { name: 'Integrations' }),
    );
    expect(integrations.getByText('SIMULATION - Demo Integration')).toBeVisible();
    expect(integrations.getByText('simulation.bench')).toBeVisible();
    expect(integrations.getByText('healthy')).toBeVisible();
    expectSimulationIdentity();
  });

  it('shows every device with the integration that owns it and its availability', async () => {
    await connectWith(stubClient());

    const devices = within(await screen.findByRole('region', { name: 'Devices' }));
    expect(devices.getByText('SIMULATION - Electronics Bench 001')).toBeVisible();
    expect(devices.getByText('simulation.bench.device.stable-001')).toBeVisible();
    expect(devices.getByText('simulation.bench')).toBeVisible();
    expect(devices.getByText('ready')).toBeVisible();
    expect(devices.getByText('simulated-electronics-bench')).toBeVisible();
  });

  it('shows the unavailable scenario states rather than the ready ones', async () => {
    await connectWith(
      stubClient({
        getIntegrations: () => Promise.resolve(unavailableIntegrations),
        getDevices: () => Promise.resolve(unavailableDevices),
      }),
    );

    await screen.findByRole('region', { name: 'Integrations' });
    expect(within(integrationsRegion()).getByText('unavailable')).toBeVisible();
    expect(within(devicesRegion()).getByText('unavailable')).toBeVisible();
    expect(within(devicesRegion()).queryByText('ready')).toBeNull();
    expect(
      within(document.querySelector('.fact-grid') as HTMLElement).getByText('healthy'),
    ).toBeVisible();
    expectSimulationIdentity();
  });

  it('states that an inventory is empty instead of treating it as a failure', async () => {
    await connectWith(
      stubClient({
        getIntegrations: () => Promise.resolve({ ...readyIntegrations, items: [] }),
        getDevices: () => Promise.resolve({ ...readyDevices, items: [] }),
      }),
    );

    await screen.findByRole('region', { name: 'Integrations' });
    expect(within(integrationsRegion()).getByText(/no integrations/i)).toBeVisible();
    expect(within(devicesRegion()).getByText(/no devices/i)).toBeVisible();
    expect(screen.queryByRole('alert')).toBeNull();
    expect(screen.queryByText(/could not be read/i)).toBeNull();
    expect(
      within(document.querySelector('.fact-grid') as HTMLElement).getByText('healthy'),
    ).toBeVisible();
    expect(screen.getByRole('button', { name: /refresh/i })).toBeVisible();
    expectSimulationIdentity();
  });

  it('counts the entries it is showing', async () => {
    await connectWith(stubClient());

    await screen.findByRole('region', { name: 'Integrations' });
    expect(within(integrationsRegion()).getByText(/1 entry/i)).toBeVisible();
    expect(within(devicesRegion()).getByText(/1 entry/i)).toBeVisible();
  });
});

describe('unusable inventory responses', () => {
  it('names the integration inventory and keeps the rest of the screen', async () => {
    await connectWith(
      stubClient({ getIntegrations: rejecting('malformed', 'items is not a list') }),
    );

    await screen.findByRole('region', { name: 'Integrations' });
    const integrations = within(integrationsRegion());
    expect(
      integrations.getByText(/integration inventory could not be read/i),
    ).toBeVisible();
    expect(integrations.getByText(/items is not a list/i)).toBeVisible();
    expect(
      within(document.querySelector('.fact-grid') as HTMLElement).getByText('healthy'),
    ).toBeVisible();
    expect(
      within(devicesRegion()).getByText('SIMULATION - Electronics Bench 001'),
    ).toBeVisible();
    expect(within(devicesRegion()).queryByText(/could not be read/i)).toBeNull();
    expectSimulationIdentity();
  });

  it('names the device inventory and keeps the rest of the screen', async () => {
    await connectWith(
      stubClient({ getDevices: rejecting('malformed', 'entry 0 is missing kind') }),
    );

    await screen.findByRole('region', { name: 'Devices' });
    const devices = within(devicesRegion());
    expect(devices.getByText(/device inventory could not be read/i)).toBeVisible();
    expect(devices.getByText(/entry 0 is missing kind/i)).toBeVisible();
    expect(
      within(document.querySelector('.fact-grid') as HTMLElement).getByText('healthy'),
    ).toBeVisible();
    expect(
      within(integrationsRegion()).getByText('SIMULATION - Demo Integration'),
    ).toBeVisible();
    expect(within(integrationsRegion()).queryByText(/could not be read/i)).toBeNull();
  });

  it('tells the two unusable inventories apart', async () => {
    await connectWith(
      stubClient({ getIntegrations: rejecting('malformed', 'items is not a list') }),
    );
    expect(
      await screen.findByText(/integration inventory could not be read/i),
    ).toBeVisible();
    expect(screen.queryByText(/device inventory could not be read/i)).toBeNull();
  });

  it('reports an inventory the backend could not serve without claiming it is empty', async () => {
    await connectWith(
      stubClient({
        getDevices: rejecting('unavailable', 'Bench Core answered HTTP 503'),
      }),
    );

    await screen.findByRole('region', { name: 'Devices' });
    expect(
      within(devicesRegion()).getByText(/device inventory is not reachable/i),
    ).toBeVisible();
    expect(within(devicesRegion()).queryByText(/no devices/i)).toBeNull();
  });
});

describe('credential retention', () => {
  it('never writes a password or a CSRF token to a durable store or the console', async () => {
    const stored = vi.spyOn(Storage.prototype, 'setItem');
    const cookie = vi.spyOn(Document.prototype, 'cookie', 'set');
    const console = [
      vi.spyOn(globalThis.console, 'log').mockImplementation(() => {}),
      vi.spyOn(globalThis.console, 'info').mockImplementation(() => {}),
      vi.spyOn(globalThis.console, 'warn').mockImplementation(() => {}),
      vi.spyOn(globalThis.console, 'debug').mockImplementation(() => {}),
      vi.spyOn(globalThis.console, 'error').mockImplementation(() => {}),
    ];
    const client = stubClient({ signIn: () => Promise.resolve(administratorSession) });

    await signInAs(client, 'admin', 'administrator-secret');
    await benchPage();

    // Preferences are the only durable writes, and a credential is never one.
    for (const [key] of stored.mock.calls) {
      expect(Object.values(PREFERENCE_KEYS)).toContain(key);
    }
    expect(cookie).not.toHaveBeenCalled();
    const written = console.flatMap((method) =>
      method.mock.calls.map((call) => call.map(String).join(' ')),
    );
    for (const secret of ['administrator-secret', 'csrf-token-fixture']) {
      expect(JSON.stringify(localStorage)).not.toContain(secret);
      expect(JSON.stringify(sessionStorage)).not.toContain(secret);
      expect(document.body.innerHTML).not.toContain(secret);
      expect(written.join('\n')).not.toContain(secret);
    }
  });

  it('clears an unsubmitted password when the application is remounted', async () => {
    const { unmount } = renderApp(stubClient());
    await signInScreen();
    fireEvent.change(screen.getByLabelText('Password'), {
      target: { value: 'unsubmitted-secret' },
    });
    unmount();

    renderApp(stubClient());

    expect(await signInScreen()).toBeVisible();
    expect(screen.getByLabelText('Password')).toHaveValue('');
    expect(screen.queryByRole('heading', { name: 'Bench', level: 2 })).toBeNull();
  });

  it('discards the inventory when the user signs out', async () => {
    await connectWith(stubClient());
    await screen.findByRole('region', { name: 'Devices' });

    openAccountMenu();
    fireEvent.click(screen.getByRole('menuitem', { name: /sign out/i }));

    expect(await signInScreen()).toBeVisible();
    expect(screen.getByLabelText('Password')).toHaveValue('');
    expect(screen.queryByRole('region', { name: 'Devices' })).toBeNull();
    expect(screen.queryByText('SIMULATION - Electronics Bench 001')).toBeNull();
  });
});

describe('mock mode', () => {
  it('presents the same product login in mock mode', async () => {
    renderApp(stubClient(), 'mock');

    await signInScreen();
    expect(screen.getByText(`Version ${APP_VERSION}`)).toBeVisible();
    expect(screen.queryByText(/contract mock/i)).toBeNull();
    expectSimulationIdentity();
  });
});
