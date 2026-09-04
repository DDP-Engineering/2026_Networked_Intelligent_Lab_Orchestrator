import { fireEvent, screen, waitFor, within } from '@testing-library/react';
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
import { installColorScheme, renderInShell } from '../../test/render';

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
      id: 'simulation.fake-python',
      displayName: 'SIMULATION - Fake Python Integration',
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
      id: 'simulation.fake-python.device.stable-001',
      integrationId: 'simulation.fake-python',
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
    listDiagnosticSessions: () => Promise.reject(new Error('not part of this fixture')),
    startDiagnosticSession: () => Promise.reject(new Error('not part of this fixture')),
    stopDiagnosticSession: () => Promise.reject(new Error('not part of this fixture')),
    getDiagnosticSettings: () => Promise.reject(new Error('not part of this fixture')),
    setDiagnosticSettings: () => Promise.reject(new Error('not part of this fixture')),
    getStatus: () => Promise.resolve(status),
    getIntegrations: () => Promise.resolve(integrations),
    getDevices: () => Promise.resolve(devices),
    getWireGuardStatus: () => Promise.reject(new Error('no network')),
    previewWireGuardChange: () => Promise.reject(new Error('no network')),
    getNetworkOperation: () => Promise.reject(new Error('no network')),
    getNetworkStatus: () => Promise.reject(new BenchCoreError('unavailable', 'no')),
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

afterEach(() => {
  vi.restoreAllMocks();
  localStorage.clear();
  document.cookie = 'nilo_csrf=; expires=Thu, 01 Jan 1970 00:00:00 GMT';
  document.documentElement.removeAttribute('data-theme');
});

describe('the one compact top row', () => {
  it('uses the appliance runtime on the sign-in screen before authentication', async () => {
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
    expect(container.querySelector('.card-boundary')).toHaveTextContent(
      'Live appliance networking',
    );
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

    expect(screen.getByRole('heading', { name: 'Network', level: 2 })).toBeVisible();
    expect(tabs().queryByRole('tab', { name: /bench/i })).toBeNull();
  });

  it('falls back to the left neighbour when the active tab closes', async () => {
    renderShell();
    await benchPage();
    openFromNav('Network');
    openFromNav('About');

    fireEvent.click(screen.getByRole('button', { name: 'Close About' }));

    expect(screen.getByRole('heading', { name: 'Network', level: 2 })).toBeVisible();
  });

  it('falls back to the right neighbour when the first tab closes', async () => {
    renderShell();
    await benchPage();
    openFromNav('Network');
    fireEvent.click(tabs().getByRole('tab', { name: /bench/i }));

    fireEvent.click(screen.getByRole('button', { name: 'Close Bench' }));

    expect(screen.getByRole('heading', { name: 'Network', level: 2 })).toBeVisible();
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
  it('puts Network in the lower core cluster, not among the instruments', async () => {
    const { container } = renderShell();
    await benchPage();

    const lower = within(container.querySelector('.nav-lower') as HTMLElement);
    const names = lower
      .getAllByRole('button')
      .map((entry) => entry.textContent?.trim());

    expect(names).toEqual([
      'Network',
      'Activity',
      'Plugins',
      'Authentication',
      'Settings',
      'About',
    ]);

    const upper = within(container.querySelector('.nav-upper') as HTMLElement);
    expect(upper.queryByRole('button', { name: 'Network' })).toBeNull();
  });

  it('lists the integrations the backend actually reported', async () => {
    const { container } = renderShell();
    await benchPage();

    const upper = within(container.querySelector('.nav-upper') as HTMLElement);
    expect(upper.getByRole('button', { name: 'Bench' })).toBeVisible();
    expect(
      upper.getByRole('button', { name: 'SIMULATION - Fake Python Integration' }),
    ).toBeVisible();
    // Nothing the appliance does not have is advertised.
    expect(upper.queryByRole('button', { name: /logic analyzer/i })).toBeNull();
    expect(upper.queryByRole('button', { name: /usb camera/i })).toBeNull();
  });

  it('hides the Authentication entry from an identity without the permission', async () => {
    renderShell(viewer);
    await benchPage();

    expect(nav().queryByRole('button', { name: 'Authentication' })).toBeNull();
    expect(nav().getByRole('button', { name: 'Network' })).toBeVisible();
  });

  it('opens an integration page showing what the backend reported', async () => {
    renderShell();
    await benchPage();

    openFromNav('SIMULATION - Fake Python Integration');

    expect(screen.getAllByText('simulation.fake-python').length).toBeGreaterThan(0);
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

    openFromNav('SIMULATION - Fake Python Integration');

    expect(screen.getByText(/device inventory could not be read/i)).toBeVisible();
    expect(screen.queryByText(/reports no devices/i)).toBeNull();
  });

  it('names an instrument on its tab only, not in a page-title row as well', async () => {
    const { container } = renderShell();
    await benchPage();

    openFromNav('SIMULATION - Fake Python Integration');

    const name = 'SIMULATION - Fake Python Integration';
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
    expect(screen.getAllByText('simulation.fake-python').length).toBeGreaterThan(0);
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

    expect(screen.getByRole('heading', { name: 'Network', level: 2 })).toBeVisible();
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

describe('pages that are not built yet', () => {
  it.each(['Plugins', 'Settings'])(
    'says plainly that %s is not available',
    async (name) => {
      renderShell();
      await benchPage();

      openFromNav(name);

      expect(screen.getByText(`${name} is not available yet`)).toBeVisible();
    },
  );

  it('shows the build the backend actually reported on About', async () => {
    renderShell();
    await benchPage();

    openFromNav('About');

    expect(screen.getByRole('heading', { name: 'About', level: 2 })).toBeVisible();
    expect(screen.getByText('0.1.0')).toBeVisible();
    expect(screen.getByText('nilo.integration/1')).toBeVisible();
  });
});
