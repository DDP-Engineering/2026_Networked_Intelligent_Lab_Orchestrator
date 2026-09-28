import packageJson from '../../../package.json';
import { act, fireEvent, screen, waitFor, within } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import App from '../../App';
import { BenchCoreError } from '../../api/client';
import type { BenchCoreClient } from '../../api/client';
import type {
  BenchCoreSession,
  BenchCoreStatus,
  DeviceInventory,
  IntegrationInventory,
} from '../../api/contract';
import { PREFERENCE_KEYS } from '../../preferences/store';
import {
  installColorScheme,
  installViewportClass,
  renderInShell,
  restoreMatchMedia,
} from '../../test/render';

// The interface reports the project version; a bump must not silently
// break these assertions, so they read the same source the app does.
const APP_VERSION: string = packageJson.version;

const status: BenchCoreStatus = {
  apiVersion: 'nilo.http/1',
  status: 'healthy',
  profile: 'simulation',
  simulation: true,
  build: { name: 'nilo', version: '0.1.0', integrationProtocol: 'nilo.integration/1' },
};

const integrations: IntegrationInventory = {
  apiVersion: 'nilo.http/1',
  scenario: 'healthy',
  items: [
    {
      id: 'simulation.bench',
      displayName: 'SIMULATION - Demo Integration',
      status: 'healthy',
      simulation: true,
      operations: ['echo', 'sleep'],
    },
  ],
};

const devices: DeviceInventory = {
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
      capabilities: ['echo'],
    },
  ],
};

const administrator: BenchCoreSession = {
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
    'activity.read',
    'system.power.manage',
  ],
  credential: 'session',
  csrfToken: 'csrf-token-fixture',
};

const viewer: BenchCoreSession = {
  ...administrator,
  username: 'bench-viewer',
  role: 'viewer',
  permissions: ['status.read', 'inventory.read'],
};

function client(session: BenchCoreSession = administrator): BenchCoreClient {
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
    getOperatingSettings: () => Promise.reject(new Error('not part of this fixture')),
    setOperatingSettings: () => Promise.reject(new Error('not part of this fixture')),
    getStatus: () => Promise.resolve(status),
    getIntegrations: () => Promise.resolve(integrations),
    getDevices: () => Promise.resolve(devices),
    getWireGuardStatus: () => Promise.reject(new Error('no network')),
    previewWireGuardChange: () => Promise.reject(new Error('no network')),
    getNetworkOperation: () => Promise.reject(new Error('no network')),
    getNetworkStatus: () => Promise.reject(new BenchCoreError('unavailable', 'no')),
    getManagementAccess: () => Promise.reject(new Error('no management access')),
    setManagementAccess: () => Promise.reject(new Error('no management access')),
    confirmManagementAccess: () => Promise.reject(new Error('no management access')),
    recoverManagementAccess: () => Promise.reject(new Error('no management access')),
    completeCommissioning: () => Promise.reject(new Error('not part of this fixture')),
    getNetworkStatistics: () => Promise.reject(new BenchCoreError('unavailable', 'no')),
    previewInterfaceChange: () =>
      Promise.reject(new BenchCoreError('unavailable', 'no')),
    acknowledgeNetworkOperation: () =>
      Promise.reject(new BenchCoreError('unavailable', 'no')),
    confirmNetworkOperation: () =>
      Promise.reject(new BenchCoreError('unavailable', 'no')),
    rollbackNetworkOperation: () =>
      Promise.reject(new BenchCoreError('unavailable', 'no')),
    signIn: () => Promise.reject(new BenchCoreError('unauthorized', 'no')),
    currentSession: () => Promise.resolve(session),
    signOut: () => Promise.resolve(),
    listIdentities: () => Promise.resolve({ apiVersion: 'nilo.http/1', items: [] }),
    createIdentity: () => Promise.reject(new BenchCoreError('forbidden', 'no')),
    setIdentityRole: () => Promise.resolve(),
    setIdentityEnabled: () => Promise.resolve(),
    resetIdentityPassword: () => Promise.resolve(),
    listTokens: () => Promise.resolve({ apiVersion: 'nilo.http/1', items: [] }),
    createToken: () => Promise.reject(new BenchCoreError('forbidden', 'no')),
    revokeToken: () => Promise.resolve(),
    deleteIdentity: () => Promise.resolve(),
    deleteToken: () => Promise.resolve(),
  };
}

function renderShell(session: BenchCoreSession = administrator, options = {}) {
  document.cookie = 'nilo_csrf=csrf-token-fixture';
  return renderInShell(
    <App client={client(session)} apiMode="live" endpoint="http://127.0.0.1:8090" />,
    { theme: 'light', language: 'en', ...options },
  );
}

const benchPage = () => screen.findByRole('heading', { name: 'Bench', level: 2 });
/** The navigation, found by role: its accessible name is itself translated. */
const nav = () => within(screen.getByRole('navigation'));
const tabs = () => within(screen.getByRole('tablist', { name: 'Open pages' }));

function openFromNav(name: RegExp | string): void {
  fireEvent.click(nav().getByRole('button', { name }));
}

/** The labels of the bottom-anchored core cluster, top to bottom. */
function coreEntries(container: HTMLElement): (string | undefined)[] {
  return Array.from(
    container.querySelectorAll(
      '.nav-lower > .nav-list > li > .nav-entry, .nav-lower > .nav-list > li > .nav-group-row > .nav-entry',
    ),
  ).map((entry) => entry.textContent?.trim());
}

afterEach(() => {
  vi.restoreAllMocks();
  restoreMatchMedia();
  localStorage.clear();
  document.cookie = 'nilo_csrf=; expires=Thu, 01 Jan 1970 00:00:00 GMT';
  document.documentElement.removeAttribute('data-theme');
});

describe('the one compact top row', () => {
  it('hides stale instrument navigation while keeping open tabs until inventory recovers', async () => {
    const intervals: (() => void)[] = [];
    vi.spyOn(window, 'setInterval').mockImplementation((handler) => {
      if (typeof handler === 'function') intervals.push(() => handler());
      return intervals.length as unknown as ReturnType<typeof window.setInterval>;
    });
    const api = client();
    let unavailable = false;
    api.listPlugins = async () => {
      if (unavailable) throw new Error('Inventory unavailable');
      return [];
    };
    document.cookie = 'nilo_csrf=csrf-token-fixture';
    renderInShell(<App client={api} apiMode="live" endpoint="http://127.0.0.1:8090" />);
    await benchPage();
    await nav().findByRole('button', { name: 'SIMULATION - Demo Integration' });
    openFromNav('SIMULATION - Demo Integration');
    expect(
      tabs().getByRole('tab', { name: /SIMULATION - Demo Integration/ }),
    ).toBeVisible();
    unavailable = true;
    await act(async () => {
      intervals.forEach((refresh) => refresh());
    });
    await waitFor(() =>
      expect(
        nav().queryByRole('button', { name: 'SIMULATION - Demo Integration' }),
      ).toBeNull(),
    );
    expect(
      tabs().getByRole('tab', { name: /SIMULATION - Demo Integration/ }),
    ).toBeVisible();
    unavailable = false;
    await act(async () => {
      intervals.forEach((refresh) => refresh());
    });
    await waitFor(() =>
      expect(
        nav().getByRole('button', { name: 'SIMULATION - Demo Integration' }),
      ).toBeVisible(),
    );
  });
  it('offers a width splitter only when navigation is beside the workspace', async () => {
    installViewportClass('compact');
    const shell = renderShell();
    await benchPage();
    expect(screen.getByRole('separator', { name: 'Navigation width' })).toBeVisible();
    shell.unmount();
    installViewportClass('constrained');
    renderShell();
    await benchPage();
    expect(screen.queryByRole('separator', { name: 'Navigation width' })).toBeNull();
    expect(screen.getByRole('button', { name: 'Menu' })).toBeVisible();
  });

  it('offers appliance power actions only to permitted browser sessions', async () => {
    const api = client(administrator);
    api.getStatus = () =>
      Promise.resolve({ ...status, profile: 'appliance', simulation: false });
    api.requestPower = vi.fn(() =>
      Promise.resolve({
        apiVersion: 'nilo.http/1',
        action: 'restart' as const,
        status: 'accepted' as const,
      }),
    );
    document.cookie = 'nilo_csrf=csrf-token-fixture';
    renderInShell(
      <App client={api} apiMode="live" endpoint="https://127.0.0.1:8443" />,
    );
    await benchPage();

    fireEvent.click(screen.getByRole('button', { name: 'Account' }));
    expect(screen.getByRole('menuitem', { name: 'Restart NILO' })).toBeVisible();
    expect(screen.getByRole('menuitem', { name: 'Power off NILO' })).toBeVisible();

    fireEvent.click(screen.getByRole('menuitem', { name: 'Restart NILO' }));
    fireEvent.click(screen.getByRole('button', { name: 'Restart NILO' }));
    await waitFor(() => expect(api.requestPower).toHaveBeenCalledTimes(1));
    expect(api.requestPower).toHaveBeenCalledWith(
      { kind: 'session', csrfToken: 'csrf-token-fixture' },
      'restart',
    );
  });

  it('does not offer power actions in simulation, even with permission', async () => {
    renderShell(administrator);
    await benchPage();
    fireEvent.click(screen.getByRole('button', { name: 'Account' }));
    expect(screen.queryByRole('menuitem', { name: /Restart NILO/i })).toBeNull();
    expect(screen.queryByRole('menuitem', { name: /Power off NILO/i })).toBeNull();
  });

  it('does not offer appliance power actions to a viewer', async () => {
    const api = client(viewer);
    api.getStatus = () =>
      Promise.resolve({ ...status, profile: 'appliance', simulation: false });
    document.cookie = 'nilo_csrf=csrf-token-fixture';
    renderInShell(
      <App client={api} apiMode="live" endpoint="https://127.0.0.1:8443" />,
    );
    await benchPage();
    fireEvent.click(screen.getByRole('button', { name: 'Account' }));
    expect(screen.queryByRole('menuitem', { name: /Restart NILO/i })).toBeNull();
    expect(screen.queryByRole('menuitem', { name: /Power off NILO/i })).toBeNull();
  });
  it('uses the same product sign-in surface before appliance authentication', async () => {
    const api = client();
    api.currentSession = () =>
      Promise.reject(new BenchCoreError('unauthorized', 'sign in'));
    const { container } = renderInShell(
      <App
        client={api}
        apiMode="live"
        endpoint="https://127.0.0.1:8443"
        runtimeProfile="appliance"
      />,
    );
    await screen.findByRole('button', { name: 'Sign in' });
    expect(container.querySelector('.sim-badge')).toBeNull();
    expect(container.querySelector('.card-boundary')).toBeNull();
    expect(screen.getByText(`Version ${APP_VERSION}`)).toBeVisible();
  });
  it('labels an appliance without simulation claims', async () => {
    const api = client();
    api.getStatus = () =>
      Promise.resolve({ ...status, profile: 'appliance', simulation: false });
    api.getIntegrations = () =>
      Promise.resolve({ ...integrations, scenario: '', items: [] });
    api.getDevices = () => Promise.resolve({ ...devices, scenario: '', items: [] });
    document.cookie = 'nilo_csrf=csrf-token-fixture';
    const { container } = renderInShell(
      <App client={api} apiMode="live" endpoint="https://127.0.0.1:8443" />,
    );
    await benchPage();
    expect(container.querySelector('.sim-badge')).toBeNull();
    expect(container.querySelector('.shell-boundary')).toBeNull();
    expect(
      screen.getByRole('status', { name: 'Live appliance networking' }),
    ).toBeVisible();
  });
  it('updates browser reachability when the next status request fails', async () => {
    const api = client();
    api.getStatus = vi
      .fn()
      .mockResolvedValueOnce(status)
      .mockRejectedValue(new BenchCoreError('unavailable', 'offline'));
    document.cookie = 'nilo_csrf=csrf-token-fixture';
    renderInShell(<App client={api} apiMode="live" endpoint="http://127.0.0.1:8090" />);
    await benchPage();
    openFromNav('Network');
    expect(
      await screen.findByText(/Bench Core did not answer this interface/i),
    ).toBeVisible();
  });

  it('carries the wordmark, the tabs and every control on one row', async () => {
    const { container } = renderShell();
    await benchPage();

    const row = container.querySelector('.toprow') as HTMLElement;
    const brand = container.querySelector('.shell-brand') as HTMLElement;

    expect(brand).toHaveTextContent('NILO');
    expect(within(row).getByRole('tablist', { name: 'Open pages' })).toBeVisible();
    expect(within(row).getByRole('button', { name: 'Network' })).toBeVisible();
    expect(within(row).getByRole('status', { name: /runtime profile/i })).toBeVisible();
    expect(within(row).getByRole('button', { name: 'Appearance' })).toBeVisible();
    expect(within(row).getByRole('button', { name: 'Language' })).toBeVisible();
    expect(within(row).getByRole('button', { name: 'Account' })).toBeVisible();
  });

  it('adds no second tab strip and no page title above the workspace', async () => {
    const { container } = renderShell();
    await benchPage();

    expect(screen.getAllByRole('tablist')).toHaveLength(1);
    // The only heading naming the page is the page's own, inside the workspace.
    const workspace = container.querySelector('.shell-workspace') as HTMLElement;
    expect(
      within(workspace).getByRole('heading', { name: 'Bench', level: 2 }),
    ).toBeVisible();
    expect(container.querySelectorAll('.page-head')).toHaveLength(1);
  });
});

describe('the constrained shell', () => {
  // REQ-UX-2687f591: below the compact rail, navigation is a labeled drawer
  // and the lower-priority selections move into the account menu.
  it('opens a labeled navigation drawer from a Menu control and returns focus', async () => {
    installViewportClass('constrained');
    const { container } = renderShell();
    await benchPage();

    const menu = screen.getByRole('button', { name: 'Menu' });
    expect(menu).toBeVisible();
    fireEvent.click(menu);

    const drawer = screen.getByRole('dialog', { name: 'Navigation' });
    const network = within(drawer).getByRole('button', { name: 'Network' });
    expect(network).toHaveTextContent('Network');
    fireEvent.click(network);

    expect(screen.queryByRole('dialog', { name: 'Navigation' })).toBeNull();
    expect(
      screen.getByRole('heading', { name: 'Network / Overview', level: 2 }),
    ).toBeVisible();
    expect(document.activeElement).toBe(screen.getByRole('button', { name: 'Menu' }));

    // The runtime indicator and the network summary stay; the theme and
    // language menus fold into the account menu.
    const row = container.querySelector('.toprow') as HTMLElement;
    expect(within(row).getByRole('status', { name: /runtime profile/i })).toBeVisible();
    expect(within(row).getByRole('button', { name: 'Network' })).toBeVisible();
    expect(within(row).queryByRole('button', { name: 'Appearance' })).toBeNull();
    expect(within(row).queryByRole('button', { name: 'Language' })).toBeNull();
    fireEvent.click(within(row).getByRole('button', { name: 'Account' }));
    const account = screen.getByRole('menu');
    expect(within(account).getByRole('menuitemradio', { name: 'Dark' })).toBeVisible();
    expect(
      within(account).getByRole('menuitemradio', { name: 'Deutsch' }),
    ).toBeVisible();
    expect(screen.getAllByRole('status', { name: /runtime profile/i })).toHaveLength(1);
  });

  it('closes the drawer with Escape', async () => {
    installViewportClass('constrained');
    renderShell();
    await benchPage();

    fireEvent.click(screen.getByRole('button', { name: 'Menu' }));
    expect(screen.getByRole('dialog', { name: 'Navigation' })).toBeVisible();
    fireEvent.keyDown(document, { key: 'Escape' });
    expect(screen.queryByRole('dialog', { name: 'Navigation' })).toBeNull();
  });

  it('keeps the labeled rail on a compact field viewport', async () => {
    installViewportClass('compact');
    renderShell();
    await benchPage();

    expect(screen.queryByRole('button', { name: 'Menu' })).toBeNull();
    expect(nav().getByRole('button', { name: 'Network' })).toHaveTextContent('Network');
  });
});

describe('the tabs', () => {
  it('keeps the open tabs when the bench is re-read', async () => {
    renderShell();
    await benchPage();

    openFromNav('Network');
    openFromNav('Authentication');
    fireEvent.click(tabs().getByRole('tab', { name: /Bench/ }));
    await benchPage();

    fireEvent.click(screen.getByRole('button', { name: 'Refresh' }));
    await benchPage();

    // A refresh passes through the connecting phase, which used to unmount the
    // shell and take every tab but Bench with it.
    expect(tabs().getAllByRole('tab')).toHaveLength(3);
    expect(tabs().getByRole('tab', { name: /Network/ })).toBeVisible();
    expect(tabs().getByRole('tab', { name: /Authentication/ })).toBeVisible();
  });

  it('opens a page once, however many times it is chosen', async () => {
    renderShell();
    await benchPage();

    openFromNav('Network');
    openFromNav('Network');

    expect(tabs().getAllByRole('tab', { name: /network/i })).toHaveLength(1);
    expect(tabs().getAllByRole('tab')).toHaveLength(2);
  });

  it('activates the tab that is selected', async () => {
    renderShell();
    await benchPage();
    openFromNav('Network');

    fireEvent.click(tabs().getByRole('tab', { name: /bench/i }));

    expect(await benchPage()).toBeVisible();
    expect(tabs().getByRole('tab', { name: /bench/i })).toHaveAttribute(
      'aria-selected',
      'true',
    );
  });

  it('moves the selection with the arrow, Home and End keys', async () => {
    renderShell();
    await benchPage();
    openFromNav('Network');
    openFromNav('About');

    const about = tabs().getByRole('tab', { name: /about/i });
    fireEvent.keyDown(about, { key: 'ArrowLeft' });
    expect(tabs().getByRole('tab', { name: /network/i })).toHaveAttribute(
      'aria-selected',
      'true',
    );

    fireEvent.keyDown(tabs().getByRole('tab', { name: /network/i }), { key: 'Home' });
    expect(tabs().getByRole('tab', { name: /bench/i })).toHaveAttribute(
      'aria-selected',
      'true',
    );

    fireEvent.keyDown(tabs().getByRole('tab', { name: /bench/i }), { key: 'End' });
    expect(tabs().getByRole('tab', { name: /about/i })).toHaveAttribute(
      'aria-selected',
      'true',
    );

    fireEvent.keyDown(tabs().getByRole('tab', { name: /about/i }), {
      key: 'ArrowRight',
    });
    expect(tabs().getByRole('tab', { name: /about/i })).toHaveAttribute(
      'aria-selected',
      'true',
    );
  });

  it('leaves the active page alone when another tab closes', async () => {
    renderShell();
    await benchPage();
    openFromNav('Network');

    fireEvent.click(screen.getByRole('button', { name: 'Close Bench' }));

    expect(
      screen.getByRole('heading', { name: 'Network / Overview', level: 2 }),
    ).toBeVisible();
    expect(tabs().queryByRole('tab', { name: /bench/i })).toBeNull();
  });

  it('falls back to the left neighbour when the active tab closes', async () => {
    renderShell();
    await benchPage();
    openFromNav('Network');
    openFromNav('About');

    fireEvent.click(screen.getByRole('button', { name: 'Close About' }));

    expect(
      screen.getByRole('heading', { name: 'Network / Overview', level: 2 }),
    ).toBeVisible();
  });

  it('falls back to the right neighbour when the first tab closes', async () => {
    renderShell();
    await benchPage();
    openFromNav('Network');
    fireEvent.click(tabs().getByRole('tab', { name: /bench/i }));

    fireEvent.click(screen.getByRole('button', { name: 'Close Bench' }));

    expect(
      screen.getByRole('heading', { name: 'Network / Overview', level: 2 }),
    ).toBeVisible();
  });

  it('states that no page is open when the last tab closes', async () => {
    renderShell();
    await benchPage();

    fireEvent.click(screen.getByRole('button', { name: 'Close Bench' }));

    expect(screen.getByText(/no page is open/i)).toBeVisible();
    expect(tabs().queryAllByRole('tab')).toHaveLength(0);
  });
});

describe('the navigation', () => {
  // REQ-UX-93e1b747, REQ-UX-7c42d7ba: Authentication owns focused destinations.
  it('expands Authentication into distinct Users and API tokens workspaces', async () => {
    renderShell();
    await benchPage();

    expect(nav().getByRole('button', { name: 'Users' })).toBeVisible();
    expect(nav().getByRole('button', { name: 'API tokens' })).toBeVisible();

    openFromNav('Users');
    expect(
      await screen.findByRole('heading', { name: 'Authentication / Users', level: 2 }),
    ).toBeVisible();
    openFromNav('API tokens');
    expect(
      await screen.findByRole('heading', {
        name: 'Authentication / API tokens',
        level: 2,
      }),
    ).toBeVisible();
    expect(tabs().getByRole('tab', { name: 'Authentication / Users' })).toBeVisible();
    expect(
      tabs().getByRole('tab', { name: 'Authentication / API tokens' }),
    ).toBeVisible();

    fireEvent.click(nav().getByRole('button', { name: 'Collapse Authentication' }));
    expect(nav().queryByRole('button', { name: 'Users' })).toBeNull();
    fireEvent.click(nav().getByRole('button', { name: 'Expand Authentication' }));
    expect(nav().getByRole('button', { name: 'Users' })).toBeVisible();
  });

  it('opens Activity from Authentication with the identity component applied', async () => {
    renderShell();
    await benchPage();
    openFromNav('Users');
    await screen.findByRole('region', { name: 'Users' });

    fireEvent.click(screen.getByRole('button', { name: 'View activity' }));

    expect(await screen.findByLabelText('Component')).toHaveValue('identity');
  });

  it('opens audited Nilo Settings activity with its settings filter', async () => {
    const session = {
      ...administrator,
      permissions: [...administrator.permissions, 'platform.settings.manage'],
    };
    const api = client(session);
    api.getOperatingSettings = () =>
      Promise.resolve({
        revision: 1,
        settings: {
          executing_jobs: 2,
          outstanding_jobs: 32,
          serial_connections: 4,
          camera_streams: 2,
          default_job_timeout_seconds: 60,
          max_job_timeout_seconds: 300,
          serial_lifetime_minutes: 60,
          serial_idle_minutes: 1,
          camera_lifetime_minutes: 55,
        },
        usage: {
          executing_jobs: 0,
          outstanding_jobs: 0,
          serial_connections: 0,
          camera_streams: 0,
        },
      });
    document.cookie = 'nilo_csrf=csrf-token-fixture';
    renderInShell(
      <App client={api} apiMode="live" endpoint="http://127.0.0.1:8090" />,
      { theme: 'light', language: 'en' },
    );
    await benchPage();
    openFromNav('Nilo Settings');
    await screen.findByRole('heading', { name: 'Nilo Settings' });

    fireEvent.click(screen.getByRole('button', { name: 'View activity' }));

    expect(
      within(await screen.findByRole('group', { name: 'Category' })).getByRole(
        'button',
        {
          name: 'Audit',
        },
      ),
    ).toHaveAttribute('aria-pressed', 'true');
    expect(screen.getByLabelText('Component')).toHaveValue('platform');
    expect(screen.getByLabelText('Action')).toHaveValue('operating_settings.changed');
  });

  it('hides Nilo Settings Activity when the session lacks activity.read', async () => {
    const session = {
      ...administrator,
      permissions: [
        ...administrator.permissions.filter(
          (permission) => permission !== 'activity.read',
        ),
        'platform.settings.manage',
      ],
    };
    renderShell(session);
    await benchPage();
    openFromNav('Nilo Settings');
    await screen.findByRole('heading', { name: 'Nilo Settings' });

    expect(screen.queryByRole('button', { name: 'View activity' })).toBeNull();
  });

  // REQ-UX-b718864e: one nested Network group, four routable destinations.
  it('expands Network into four focused pages without duplicating their tabs', async () => {
    renderShell();
    await benchPage();

    for (const name of ['Overview', 'Device access', 'VPN profiles', 'Diagnostics']) {
      expect(nav().getByRole('button', { name })).toBeVisible();
    }
    expect(nav().queryByRole('button', { name: 'Network adapters' })).toBeNull();

    openFromNav('Device access');
    expect(
      await screen.findByRole('heading', {
        name: 'Network / Device access',
        level: 2,
      }),
    ).toBeVisible();
    openFromNav('Device access');
    expect(tabs().getAllByRole('tab', { name: 'Device access' })).toHaveLength(1);

    fireEvent.click(nav().getByRole('button', { name: 'Collapse Network' }));
    expect(nav().queryByRole('button', { name: 'Device access' })).toBeNull();
    expect(tabs().getByRole('tab', { name: 'Device access' })).toBeVisible();
    fireEvent.click(nav().getByRole('button', { name: 'Expand Network' }));
    expect(nav().getByRole('button', { name: 'Device access' })).toBeVisible();
  });

  it('puts Network in the lower core cluster, not among the instruments', async () => {
    const { container } = renderShell();
    await benchPage();

    expect(coreEntries(container)).toEqual([
      'Plugins',
      'Network',
      'Authentication',
      'Activity',
      'About',
    ]);

    const upper = within(container.querySelector('.nav-upper') as HTMLElement);
    expect(upper.queryByRole('button', { name: 'Network' })).toBeNull();
    expect(nav().queryByRole('button', { name: 'Settings' })).toBeNull();
  });

  it('lists the integrations the backend actually reported', async () => {
    const { container } = renderShell();
    await benchPage();

    const upper = within(container.querySelector('.nav-upper') as HTMLElement);
    expect(upper.getByRole('button', { name: 'Bench' })).toBeVisible();
    expect(
      await upper.findByRole('button', { name: 'SIMULATION - Demo Integration' }),
    ).toBeVisible();
    // Nothing the appliance does not have is advertised.
    expect(upper.queryByRole('button', { name: /logic analyzer/i })).toBeNull();
    expect(upper.queryByRole('button', { name: /usb camera/i })).toBeNull();
  });

  it('hides the Authentication entry from an identity without the permission', async () => {
    const { container } = renderShell(viewer);
    await benchPage();

    expect(nav().queryByRole('button', { name: 'Authentication' })).toBeNull();
    expect(nav().getByRole('button', { name: 'Network' })).toBeVisible();
    // The entries a viewer does keep stay in the order an administrator sees.
    expect(coreEntries(container)).toEqual(['Plugins', 'Network', 'About']);
  });

  it('shows only the Authentication destination that the session may read', async () => {
    const { unmount } = renderShell({
      ...administrator,
      permissions: ['users.read'],
    });
    await benchPage();
    expect(nav().getByRole('button', { name: 'Users' })).toBeVisible();
    expect(nav().queryByRole('button', { name: 'API tokens' })).toBeNull();
    openFromNav('Users');
    expect(await screen.findByRole('button', { name: 'New user' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'New user' })).toHaveAttribute(
      'title',
      'Your session does not include users.manage.',
    );

    unmount();
    renderShell({ ...administrator, permissions: ['tokens.read.all'] });
    await benchPage();
    expect(nav().queryByRole('button', { name: 'Users' })).toBeNull();
    expect(nav().getByRole('button', { name: 'API tokens' })).toBeVisible();
    openFromNav('API tokens');
    expect(await screen.findByRole('button', { name: 'New token' })).toBeDisabled();
  });

  it('opens an integration page showing what the backend reported', async () => {
    renderShell();
    await benchPage();

    await nav().findByRole('button', { name: 'SIMULATION - Demo Integration' });
    openFromNav('SIMULATION - Demo Integration');

    expect(screen.getAllByText('simulation.bench').length).toBeGreaterThan(0);
    expect(screen.getByText('SIMULATION - Electronics Bench 001')).toBeVisible();
  });

  it('says an unreadable device inventory is unreadable, not that there are none', async () => {
    const unreadable = client();
    unreadable.getDevices = () =>
      Promise.reject(new BenchCoreError('malformed', 'device inventory unusable'));
    document.cookie = 'nilo_csrf=csrf-token-fixture';
    renderInShell(
      <App client={unreadable} apiMode="live" endpoint="http://127.0.0.1:8090" />,
      { theme: 'light', language: 'en' },
    );
    await benchPage();

    expect(
      nav().queryByRole('button', { name: 'SIMULATION - Demo Integration' }),
    ).toBeNull();

    expect(screen.getByText(/device inventory could not be read/i)).toBeVisible();
    expect(screen.queryByText(/reports no devices/i)).toBeNull();
  });

  it('names an instrument on its tab only, not in a page-title row as well', async () => {
    const { container } = renderShell();
    await benchPage();

    await nav().findByRole('button', { name: 'SIMULATION - Demo Integration' });
    openFromNav('SIMULATION - Demo Integration');

    const name = 'SIMULATION - Demo Integration';
    expect(screen.getByRole('tab', { name: new RegExp(name) })).toBeVisible();
    const workspace = container.querySelector('.shell-workspace') as HTMLElement;
    expect(within(workspace).queryByRole('heading', { name })).toBeNull();
    expect(workspace.querySelector('.page-head')).toBeNull();
  });
});

describe('themes', () => {
  it('stamps the selected theme on the document root and keeps it', async () => {
    const { unmount } = renderShell();
    await benchPage();

    fireEvent.click(screen.getByRole('button', { name: 'Appearance' }));
    fireEvent.click(screen.getByRole('menuitemradio', { name: 'Dark' }));

    await waitFor(() =>
      expect(document.documentElement.getAttribute('data-theme')).toBe('dark'),
    );
    expect(localStorage.getItem(PREFERENCE_KEYS.theme)).toBe('dark');

    unmount();
    // A fresh application reads the persisted preference rather than the default.
    renderInShell(
      <App client={client()} apiMode="live" endpoint="http://127.0.0.1:8090" />,
      { language: 'en' },
    );
    await waitFor(() =>
      expect(document.documentElement.getAttribute('data-theme')).toBe('dark'),
    );
  });

  it('follows the device preference while System is selected, and reacts to a change', async () => {
    const setScheme = installColorScheme('dark');
    renderShell(administrator, { theme: 'system' });
    await benchPage();

    fireEvent.click(screen.getByRole('button', { name: 'Appearance' }));
    expect(screen.getByText(/follows your device/i)).toHaveTextContent('Dark');
    fireEvent.keyDown(document, { key: 'Escape' });

    setScheme('light');

    fireEvent.click(screen.getByRole('button', { name: 'Appearance' }));
    await waitFor(() =>
      expect(screen.getByText(/follows your device/i)).toHaveTextContent('Light'),
    );
    // The root attribute stays `system`: the palette follows the device in CSS.
    expect(document.documentElement.getAttribute('data-theme')).toBe('system');
  });
});

describe('languages', () => {
  it('offers exactly the languages with a catalogue, and translates the shell', async () => {
    renderShell();
    await benchPage();

    fireEvent.click(screen.getByRole('button', { name: 'Language' }));
    expect(
      screen.getAllByRole('menuitemradio').map((entry) => entry.textContent),
    ).toEqual(['English', 'Deutsch', 'Română']);

    fireEvent.click(screen.getByRole('menuitemradio', { name: 'Deutsch' }));

    await waitFor(() =>
      expect(nav().getByRole('button', { name: 'Netzwerk' })).toBeVisible(),
    );
    expect(localStorage.getItem(PREFERENCE_KEYS.language)).toBe('de');
    expect(document.documentElement.getAttribute('lang')).toBe('de');
  });

  it('leaves identifiers, protocol names and reported values untranslated', async () => {
    renderShell();
    await benchPage();

    fireEvent.click(screen.getByRole('button', { name: 'Language' }));
    fireEvent.click(screen.getByRole('menuitemradio', { name: 'Română' }));
    await waitFor(() =>
      expect(nav().getByRole('button', { name: 'Rețea' })).toBeVisible(),
    );

    const facts = within(document.querySelector('.fact-grid') as HTMLElement);
    expect(facts.getByText('nilo.http/1')).toBeVisible();
    expect(facts.getByText('nilo.integration/1')).toBeVisible();
    expect(facts.getByText('simulation')).toBeVisible();
    expect(facts.getByText('healthy')).toBeVisible();
    expect(screen.getAllByText('simulation.bench').length).toBeGreaterThan(0);
  });
});

describe('the network presentation', () => {
  const ILLUSTRATIVE = ['192.168.10.42', '10.8.0.12', '1 Gbps', '2.4 Mbps', '0.3 Mbps'];

  it('keeps the four network facts separate in the summary', async () => {
    renderShell();
    await benchPage();

    // The top-row control, not the navigation entry of the same name.
    const row = document.querySelector('.toprow') as HTMLElement;
    fireEvent.click(within(row).getByRole('button', { name: 'Network' }));
    const menu = screen.getByRole('menu');

    for (const label of [
      'Interface connection',
      'Tunnel',
      'Last handshake',
      'Bench Core reachability',
    ]) {
      expect(within(menu).getByText(label)).toBeVisible();
    }
    // The top row reads none of the appliance's own networking, so three of the
    // four say so rather than being guessed from the one fact it does have.
    expect(within(menu).getAllByText('Not reported')).toHaveLength(3);

    for (const value of ILLUSTRATIVE) {
      expect(screen.queryByText(value)).toBeNull();
    }
  });

  it('says the network state is unreadable, and still reports HTTP reachability', async () => {
    // This fixture's client refuses the network reads, which is what a backend
    // that cannot answer looks like. The page has to say so rather than present
    // an appliance with no interfaces.
    const { container } = renderShell();
    await benchPage();

    openFromNav('Network');

    expect(
      screen.getByRole('heading', { name: 'Network / Overview', level: 2 }),
    ).toBeVisible();
    expect(
      await screen.findByText(/appliance network state could not be read/i),
    ).toBeVisible();

    // Reachability remains in the top-row menu instead of repeated on the page.
    fireEvent.click(
      within(container.querySelector('.toprow') as HTMLElement).getByRole('button', {
        name: 'Network',
      }),
    );
    expect(
      within(screen.getByRole('menu')).getByText(/answered this interface over HTTP/i),
    ).toBeVisible();

    for (const value of ILLUSTRATIVE) {
      expect(container.textContent).not.toContain(value);
    }
  });
});

describe('core pages', () => {
  it('shows the build the backend actually reported on About', async () => {
    renderShell();
    await benchPage();

    openFromNav('About');

    expect(screen.getByRole('heading', { name: 'About', level: 2 })).toBeVisible();
    expect(screen.getByText('0.1.0')).toBeVisible();
    expect(screen.getByText('nilo.integration/1')).toBeVisible();
  });
});
