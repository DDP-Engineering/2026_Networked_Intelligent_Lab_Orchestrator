import { fireEvent, screen, waitFor, within } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import { BenchCoreError } from '../../api/client';
import type {
  NetworkOperation,
  NetworkStatistics,
  NetworkStatus,
  StatisticsPeriod,
} from '../../api/contract';
import type { DeviceNetwork } from '../../api/devicenetworks';
import type { DeviceNetworkChange } from '../../api/devicenetworks';
import type { ManagementAccessStatus } from '../../api/management-access';
import type { WireGuardStatus } from '../../api/wireguard';
import { renderInShell } from '../../test/render';
import { NetworkPage } from './NetworkPage';

const lineThree: DeviceNetwork = {
  id: 'dn-1',
  name: 'Line 3',
  location: 'Hall B',
  deviceInterface: '02:00:00:00:00:01',
  applianceAddress: '192.168.0.1/24',
  deviceSubnet: '192.168.0.0/24',
  scope: 'host',
  target: '192.168.0.130',
  addressing: 'original',
  exposed: '192.168.0.130',
  access: 'local',
  localInterface: '02:00:00:00:00:01',
  localSources: ['192.168.10.0/24'],
  vpnProfileId: '',
  vpnSources: [],
  services: {
    protocols: ['tcp'],
    tcpPorts: [504],
    udpPorts: [],
    preserveSource: false,
  },
  enabled: true,
  revision: 'r1',
  state: 'active',
  detail: '',
  applied: true,
  observedAt: new Date('2026-09-04T10:00:00Z'),
};

const officeActive: WireGuardStatus = {
  reported: true,
  measuredBy: 'simulation',
  networkManagerAvailable: true,
  networkManagerVersion: 'simulated',
  kernelSupported: true,
  helperAvailable: true,
  canInstall: false,
  tools: [],
  detail: '',
  profiles: [
    {
      id: 'office',
      name: 'Office',
      interface: 'sim-wg',
      niloOwned: true,
      active: true,
      autoconnect: true,
      lastHandshake: null,
      addresses: ['10.7.0.2/24'],
      dns: [],
      peers: [{ endpoint: 'vpn.example.test:51820', allowedIPs: ['10.7.0.0/24'] }],
    },
  ],
};

const status: NetworkStatus = {
  measuredBy: 'simulation',
  interfaces: [
    {
      id: 'sim-enp1s0',
      name: 'enp1s0',
      kind: 'ethernet',
      state: 'connected',
      ipv4: ['192.168.10.24/24'],
      ipv6: ['fd00:10::24/64'],
      gateway4: '192.168.10.1',
      gateway6: 'fd00:10::1',
      linkCapacityMbps: 1000,
      rxBytesPerSecond: 12480,
      txBytesPerSecond: 3120,
      controllable: true,
      notControllableReason: '',
      adapterId: '02:00:00:00:00:01',
      ssid: '',
      settings: {
        saved: true,
        applied: true,
        revision: 'r1',
        method: 'static',
        managementProtected: true,
        neverDefault: false,
        ssid: '',
      },
    },
    {
      id: 'sim-lo',
      name: 'lo',
      kind: 'loopback',
      state: 'connected',
      ipv4: ['127.0.0.1/8'],
      ipv6: ['::1/128'],
      linkCapacityMbps: 0,
      rxBytesPerSecond: 0,
      txBytesPerSecond: 0,
      controllable: false,
      notControllableReason: 'the loopback device is not an appliance connection',
      adapterId: '',
      ssid: '',
      settings: null,
    },
  ],
};

const managementAccess: ManagementAccessStatus = {
  state: 'applied',
  port: 8443,
  interfaces: [
    {
      name: 'enp1s0',
      adapterId: '02:00:00:00:00:01',
      kind: 'ethernet',
      state: 'connected',
      ipv4: ['192.168.10.24/24'],
      ipv6: [],
      enabled: true,
      current: true,
      usable: true,
      role: 'management',
      roleAvailable: true,
      roleDetail: '',
      conflictingResourceId: '',
      conflictingResourceName: '',
    },
  ],
  configuredInterfaces: ['enp1s0'],
  urls: [],
  tlsNames: [],
  tlsMissing: [],
  confirmBy: null,
  detail: '',
  outcome: { code: '', interface: '', resourceName: '' },
  currentInterface: 'enp1s0',
  provisioning: false,
};

const statistics: NetworkStatistics = {
  interfaceId: 'sim-enp1s0',
  period: '15m',
  from: new Date('2026-09-03T09:45:00Z'),
  to: new Date('2026-09-03T10:00:00Z'),
  measuredBy: 'simulation',
  stale: false,
  points: [
    {
      minuteStart: new Date('2026-09-03T09:45:00Z'),
      rxBytes: 4096,
      txBytes: 1024,
      rxErrors: 0,
      txErrors: 0,
      rxDropped: 0,
      txDropped: 0,
      samples: 11,
      stale: false,
    },
  ],
};

const previewed: NetworkOperation = {
  id: 'operation-1',
  action: 'interface.disable',
  target: 'sim-enp1s0',
  state: 'previewed',
  revision: 'revision-9f2c',
  createdAt: new Date('2026-09-03T10:00:00Z'),
  confirmDeadline: null,
  failure: '',
};

const awaitingConfirmation: NetworkOperation = {
  ...previewed,
  state: 'awaiting_confirmation',
  confirmDeadline: new Date('2026-09-03T10:02:00Z'),
};

interface PageOverrides {
  getNetworkStatus?: () => Promise<NetworkStatus>;
  // Mirrors the real prop, so a test can assert which interface was asked for.
  getNetworkStatistics?: (
    interfaceId: string,
    period: StatisticsPeriod,
  ) => Promise<NetworkStatistics>;
  previewInterfaceChange?: (
    interfaceId: string,
    action: 'enable' | 'disable',
  ) => Promise<NetworkOperation>;
  acknowledgeOperation?: (id: string, revision: string) => Promise<NetworkOperation>;
  confirmOperation?: (id: string) => Promise<NetworkOperation>;
  rollbackOperation?: (id: string) => Promise<NetworkOperation>;
  listOperations?: () => Promise<NetworkOperation[]>;
  canManageInterfaces?: boolean;
  readDeviceNetworks?: () => Promise<DeviceNetwork[]>;
  readManagementAccess?: () => Promise<ManagementAccessStatus>;
  previewDeviceNetworkChange?: (
    change: DeviceNetworkChange,
  ) => Promise<NetworkOperation>;
  wireGuardStatus?: WireGuardStatus | null;
  previewInterfaceSettings?: () => Promise<NetworkOperation>;
}

type NetworkView = 'overview' | 'adapters' | 'device-access' | 'vpn' | 'diagnostics';

/** Open the row's compact Actions dialog and choose one action by its full name. */
function chooseAction(rowName: string, action: RegExp | string): void {
  fireEvent.click(screen.getByRole('button', { name: `Actions for ${rowName}` }));
  fireEvent.click(
    within(screen.getByRole('dialog', { name: `Actions for ${rowName}` })).getByRole(
      'button',
      { name: action },
    ),
  );
}

function renderPage(overrides: PageOverrides = {}, view: NetworkView = 'overview') {
  return renderInShell(
    <NetworkPage
      view={view}
      readNetworkStatus={overrides.getNetworkStatus ?? (() => Promise.resolve(status))}
      readNetworkStatistics={
        overrides.getNetworkStatistics ?? (() => Promise.resolve(statistics))
      }
      previewInterfaceChange={
        overrides.previewInterfaceChange ?? (() => Promise.resolve(previewed))
      }
      acknowledgeOperation={
        overrides.acknowledgeOperation ?? (() => Promise.resolve(awaitingConfirmation))
      }
      confirmOperation={
        overrides.confirmOperation ??
        (() => Promise.resolve({ ...awaitingConfirmation, state: 'confirmed' }))
      }
      rollbackOperation={
        overrides.rollbackOperation ??
        (() => Promise.resolve({ ...awaitingConfirmation, state: 'rolled_back' }))
      }
      listOperations={overrides.listOperations}
      canManageInterfaces={overrides.canManageInterfaces ?? false}
      benchCoreReachable
      readDeviceNetworks={overrides.readDeviceNetworks}
      readManagementAccess={overrides.readManagementAccess}
      previewDeviceNetworkChange={overrides.previewDeviceNetworkChange}
      wireGuardStatus={overrides.wireGuardStatus ?? null}
      previewInterfaceSettings={overrides.previewInterfaceSettings}
    />,
    { theme: 'light', language: 'en' },
  );
}

describe('NetworkPage', () => {
  it('shares one management-role observation with the device selector', async () => {
    const readManagementAccess = vi.fn(() => Promise.resolve(managementAccess));
    renderPage(
      {
        canManageInterfaces: true,
        readDeviceNetworks: () => Promise.resolve([lineThree]),
        readManagementAccess,
        previewDeviceNetworkChange: () => Promise.resolve(previewed),
      },
      'device-access',
    );

    fireEvent.click(await screen.findByRole('button', { name: 'Add device network' }));
    const editor = screen.getByRole('dialog', { name: 'New device network' });
    expect(
      within(within(editor).getByLabelText('Device-side adapter')).getByRole('option', {
        name: /enp1s0/,
      }),
    ).toBeDisabled();
    expect(readManagementAccess).toHaveBeenCalledTimes(1);
  });

  it.each([
    [
      'reading',
      () => new Promise<ManagementAccessStatus>(() => undefined),
      'Management adapter roles are still loading.',
    ],
    [
      'failed',
      () => Promise.reject(new Error('unavailable')),
      'Management adapter roles are unavailable.',
    ],
  ] as const)(
    'fails the device editor closed while management roles are %s',
    async (_state, readManagementAccess, message) => {
      renderPage(
        {
          canManageInterfaces: true,
          readDeviceNetworks: () => Promise.resolve([lineThree]),
          readManagementAccess,
          previewDeviceNetworkChange: () => Promise.resolve(previewed),
        },
        'device-access',
      );

      fireEvent.click(
        await screen.findByRole('button', { name: 'Add device network' }),
      );
      const editor = screen.getByRole('dialog', { name: 'New device network' });
      expect(await within(editor).findByText(message)).toBeVisible();
      expect(within(editor).getByLabelText('Device-side adapter')).toBeDisabled();
      expect(within(editor).getByRole('button', { name: 'Preview' })).toBeDisabled();
    },
  );

  it('retains a structured role conflict until observed state supersedes it', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const disabled = { ...lineThree, enabled: false, state: 'disabled' as const };
    const readManagementAccess = vi
      .fn()
      .mockResolvedValueOnce(managementAccess)
      .mockResolvedValueOnce(managementAccess)
      .mockResolvedValue({
        ...managementAccess,
        interfaces: managementAccess.interfaces.map((entry) => ({
          ...entry,
          role: '' as const,
          enabled: false,
        })),
        configuredInterfaces: [],
      });
    try {
      renderPage(
        {
          canManageInterfaces: true,
          readDeviceNetworks: () => Promise.resolve([disabled]),
          readManagementAccess,
          previewDeviceNetworkChange: () =>
            Promise.reject(
              new BenchCoreError(
                'rejected',
                'backend wording',
                'network_interface_role_conflict',
                {
                  adapterId: '02:00:00:00:00:01',
                  interface: 'enp1s0',
                  existingRole: 'management',
                  requestedRole: 'device',
                  conflictingResourceId: '',
                  conflictingResourceName: '',
                },
              ),
            ),
        },
        'device-access',
      );

      await screen.findByText('Line 3');
      chooseAction('Line 3', 'Enable Line 3');

      expect(
        await screen.findByText(/enp1s0 is reserved for management access/i),
      ).toBeVisible();
      expect(screen.queryByText('backend wording')).toBeNull();
      await vi.advanceTimersByTimeAsync(5000);
      expect(
        screen.getByText(/enp1s0 is reserved for management access/i),
      ).toBeVisible();
      await vi.advanceTimersByTimeAsync(5000);
      await waitFor(() =>
        expect(
          screen.queryByText(/enp1s0 is reserved for management access/i),
        ).toBeNull(),
      );
    } finally {
      vi.useRealTimers();
    }
  });

  it('never renders backend prose for malformed structured role conflicts', async () => {
    renderPage(
      {
        canManageInterfaces: true,
        readDeviceNetworks: () =>
          Promise.resolve([
            { ...lineThree, enabled: false, state: 'disabled' as const },
          ]),
        previewDeviceNetworkChange: () =>
          Promise.reject(
            new BenchCoreError(
              'rejected',
              'untrusted backend role wording',
              'network_interface_role_conflict',
            ),
          ),
      },
      'device-access',
    );

    await screen.findByText('Line 3');
    chooseAction('Line 3', 'Enable Line 3');
    expect(
      await screen.findByText(/adapter role conflict could not be read/i),
    ).toBeVisible();
    expect(screen.queryByText('untrusted backend role wording')).toBeNull();
  });

  it('preserves typed adapter detail when acknowledgement finds a new role conflict', async () => {
    const disabled = { ...lineThree, enabled: false, state: 'disabled' as const };
    const acknowledgeOperation = vi.fn(() =>
      Promise.reject(
        new BenchCoreError(
          'rejected',
          'untrusted acknowledgement wording',
          'network_interface_role_conflict',
          {
            adapterId: '02:00:00:00:00:01',
            interface: 'enp1s0',
            existingRole: 'management',
            requestedRole: 'device',
            conflictingResourceId: '',
            conflictingResourceName: '',
          },
        ),
      ),
    );
    renderPage(
      {
        canManageInterfaces: true,
        readDeviceNetworks: () => Promise.resolve([disabled]),
        previewDeviceNetworkChange: () => Promise.resolve(previewed),
        acknowledgeOperation,
      },
      'device-access',
    );

    await screen.findByText('Line 3');
    chooseAction('Line 3', 'Enable Line 3');
    fireEvent.click(
      within(await screen.findByRole('dialog')).getByRole('button', {
        name: /^apply$/i,
      }),
    );

    expect(
      await screen.findByText(/enp1s0 is reserved for management access/i),
    ).toBeVisible();
    expect(screen.queryByText('untrusted acknowledgement wording')).toBeNull();
  });

  // REQ-UX-b555166c: each destination carries only its own concern.
  it.each([
    [
      'overview',
      'Network / Overview',
      'Overview',
      ['Interfaces', 'WireGuard profiles', 'Statistics'],
    ],
    [
      'adapters',
      'Network / Network adapters',
      'Interfaces',
      ['Overview', 'WireGuard profiles', 'Statistics'],
    ],
    [
      'device-access',
      'Network / Device access',
      'Device networks',
      ['Overview', 'Interfaces', 'WireGuard profiles', 'Statistics'],
    ],
    [
      'vpn',
      'Network / VPN profiles',
      'WireGuard profiles',
      ['Overview', 'Interfaces', 'Device networks', 'Statistics'],
    ],
    [
      'diagnostics',
      'Network / Diagnostics',
      'Statistics',
      ['Overview', 'Interfaces', 'Device networks', 'WireGuard profiles'],
    ],
  ] as const)(
    'renders only the %s destination',
    async (view, pageTitle, present, absent) => {
      renderPage(
        {
          readDeviceNetworks: () => Promise.resolve([lineThree]),
          wireGuardStatus: officeActive,
        },
        view,
      );

      expect(
        await screen.findByRole('heading', { name: pageTitle, level: 2 }),
      ).toBeVisible();
      if (present === 'Overview') {
        expect(await screen.findByRole('region', { name: present })).toBeVisible();
      } else {
        expect(await screen.findByRole('heading', { name: present })).toBeVisible();
      }
      for (const heading of absent) {
        expect(screen.queryByRole('heading', { name: heading })).toBeNull();
      }
    },
  );

  it('requests traffic history only from Diagnostics', async () => {
    const getNetworkStatistics = vi.fn(() => Promise.resolve(statistics));
    renderPage({ getNetworkStatistics }, 'adapters');
    await screen.findAllByText('enp1s0');
    await Promise.resolve();
    expect(getNetworkStatistics).not.toHaveBeenCalled();
  });

  it('lists addresses and gateways without repeating the shell simulation indicator', async () => {
    renderPage({}, 'adapters');

    expect((await screen.findAllByText('enp1s0')).length).toBeGreaterThan(0);
    expect(screen.getByText('192.168.10.24/24')).toBeInTheDocument();
    expect(screen.getByText('fd00:10::24/64')).toBeInTheDocument();
    expect(screen.getAllByText('lo').length).toBeGreaterThan(0);

    expect(screen.getByRole('columnheader', { name: 'Gateway' })).toBeVisible();
    expect(screen.getByText('192.168.10.1')).toBeVisible();
    expect(screen.getByText('fd00:10::1')).toBeVisible();
    expect(screen.queryByText(/simulated environment/i)).not.toBeInTheDocument();
  });

  it('states the reason for a read-only device in its details, not in a tooltip', async () => {
    const { container } = renderPage({}, 'adapters');
    await screen.findAllByText('enp1s0');
    expect(container.querySelector('[title]')).toBeNull();
    fireEvent.click(screen.getByRole('button', { name: 'Details lo' }));
    expect(
      screen.getByText(/the loopback device is not an appliance connection/i),
    ).toBeVisible();
  });

  // REQ-UX-ff989933: the essential columns stay, everything else is in the
  // explicit details, and several actions are one compact dialog.
  it('keeps the essential columns and puts the secondary values behind Details', async () => {
    renderPage({ canManageInterfaces: true }, 'adapters');
    await screen.findAllByText('enp1s0');

    for (const column of ['Interface', 'State', 'IP address', 'Gateway', 'Actions']) {
      expect(screen.getByRole('columnheader', { name: column })).toBeVisible();
    }
    expect(screen.queryByRole('columnheader', { name: /link/i })).toBeNull();
    expect(screen.queryByText('1000 Mbit/s')).toBeNull();

    const details = screen.getByRole('button', { name: 'Details enp1s0' });
    expect(details).toHaveAttribute('aria-expanded', 'false');
    fireEvent.click(details);
    expect(details).toHaveAttribute('aria-expanded', 'true');
    const facts = screen.getByRole('region', { name: 'Details enp1s0' });
    expect(within(facts).getByText('1000 Mbit/s')).toBeVisible();
    expect(within(facts).getByText(/12\.5 kB\/s/)).toBeVisible();
    expect(within(facts).getByText(/3\.1 kB\/s/)).toBeVisible();
    expect(within(facts).getByText('02:00:00:00:00:01')).toBeVisible();
    expect(within(facts).getByText(/protected as a management path/i)).toBeVisible();
    expect(within(facts).getByText(/static/)).toBeVisible();
    expect(screen.getByRole('table').closest('.table-scroll')).not.toBeNull();
  });

  it('offers every interface action from one dialog, with visible object context', async () => {
    renderPage(
      {
        canManageInterfaces: true,
        previewInterfaceSettings: () => Promise.resolve(previewed),
      },
      'adapters',
    );
    await screen.findAllByText('enp1s0');

    fireEvent.click(screen.getByRole('button', { name: 'Actions for enp1s0' }));
    const dialog = screen.getByRole('dialog', { name: 'Actions for enp1s0' });
    expect(within(dialog).getByText('Network interface')).toBeVisible();
    expect(within(dialog).getByText('Connected')).toBeVisible();
    expect(
      within(dialog).getByRole('button', { name: 'Disable enp1s0' }),
    ).toBeEnabled();
    expect(
      within(dialog).getByRole('button', { name: /Configure enp1s0/ }),
    ).toBeEnabled();
    fireEvent.keyDown(document, { key: 'Escape' });
    expect(screen.queryByRole('dialog', { name: 'Actions for enp1s0' })).toBeNull();

    // Loopback offers no action, so it has no Actions control either.
    expect(screen.queryByRole('button', { name: 'Actions for lo' })).toBeNull();
    // The details stay explicit and separate from the action dialog.
    expect(screen.getByRole('button', { name: 'Details lo' })).toBeVisible();
  });

  // REQ-UX-cde0b9e2: one overview, and only the facts that decide the next step.
  it('summarizes the uplink, the device side, the active profile and what is pending', async () => {
    renderPage({
      readDeviceNetworks: () => Promise.resolve([lineThree]),
      wireGuardStatus: officeActive,
    });
    const overview = await screen.findByRole('region', { name: 'Overview' });
    expect(within(overview).getAllByText(/enp1s0/).length).toBeGreaterThan(0);
    expect(within(overview).getByText(/192\.168\.10\.24\/24/)).toBeVisible();
    expect(within(overview).getByText(/192\.168\.0\.1\/24/)).toBeVisible();
    expect(within(overview).getByText('Office')).toBeVisible();
    expect(within(overview).getByText(/nothing pending/i)).toBeVisible();
    expect(within(overview).queryByText(/simulated environment/i)).toBeNull();
  });

  it('says none where there is no device network and no active profile', async () => {
    renderPage({ readDeviceNetworks: () => Promise.resolve([]) });
    const overview = await screen.findByRole('region', { name: 'Overview' });
    expect(within(overview).getByText(/enp1s0/)).toBeVisible();
    expect(within(overview).getAllByText('None')).toHaveLength(2);
  });

  it('names a change awaiting confirmation in the overview', async () => {
    renderPage({ listOperations: () => Promise.resolve([awaitingConfirmation]) });
    const overview = await screen.findByRole('region', { name: 'Overview' });
    expect(within(overview).getByText(/interface\.disable/)).toBeVisible();
  });

  // REQ-UX-4570c081: the rollback window is in the confirmation, never only in help.
  it('states the two-minute rollback window in the confirmation itself', async () => {
    renderPage({ canManageInterfaces: true }, 'adapters');
    await screen.findAllByText('enp1s0');
    chooseAction('enp1s0', 'Disable enp1s0');
    const dialog = await screen.findByRole('dialog');
    expect(within(dialog).getByText(/within two minutes/i)).toBeVisible();
    expect(within(dialog).getByText(/restored automatically/i)).toBeVisible();
  });

  it('asks for a different window when a period is chosen', async () => {
    const getNetworkStatistics = vi.fn(() => Promise.resolve(statistics));
    renderPage({ getNetworkStatistics }, 'diagnostics');

    await waitFor(() => expect(getNetworkStatistics).toHaveBeenCalled());
    const before = getNetworkStatistics.mock.calls.length;

    screen.getByRole('button', { name: '24 hours' }).click();
    await waitFor(() =>
      expect(getNetworkStatistics.mock.calls.length).toBeGreaterThan(before),
    );
  });

  it('marks a series that stopped refreshing rather than letting it look current', async () => {
    renderPage(
      {
        getNetworkStatistics: () =>
          Promise.resolve({
            ...statistics,
            stale: true,
            points: statistics.points.map((point) => ({ ...point, stale: true })),
          }),
      },
      'diagnostics',
    );
    expect(await screen.findByText(/no longer being refreshed/i)).toBeInTheDocument();
  });

  it('states that nothing was measured rather than drawing an empty window as zero', async () => {
    renderPage(
      {
        getNetworkStatistics: () =>
          Promise.resolve({ ...statistics, stale: true, points: [] }),
      },
      'diagnostics',
    );
    expect(await screen.findByText(/no traffic was measured/i)).toBeInTheDocument();
  });

  // Bench Core authorizes every request on its own, so hiding a control is
  // presentation. It still must not be offered to somebody who cannot use it.
  it('offers no interface controls to an identity that cannot manage them', async () => {
    renderPage({ canManageInterfaces: false }, 'adapters');
    await screen.findAllByText('enp1s0');
    expect(screen.queryByRole('button', { name: /disable/i })).toBeNull();
  });

  it('offers a control only for a device that is actually controllable', async () => {
    renderPage({ canManageInterfaces: true }, 'adapters');
    await screen.findAllByText('enp1s0');

    // The wired device can be disabled; loopback carries no control at all.
    fireEvent.click(screen.getByRole('button', { name: 'Actions for enp1s0' }));
    expect(
      within(screen.getByRole('dialog', { name: 'Actions for enp1s0' })).getAllByRole(
        'button',
        {
          name: /disable enp1s0/i,
        },
      ),
    ).toHaveLength(1);
    fireEvent.keyDown(document, { key: 'Escape' });
    expect(screen.queryByRole('button', { name: /actions for lo/i })).toBeNull();
    expect(screen.queryByRole('button', { name: /disable lo/i })).toBeNull();
  });

  it('shows the exact change before it asks for agreement, and changes nothing until then', async () => {
    const previewInterfaceChange = vi.fn(() => Promise.resolve(previewed));
    const acknowledgeOperation = vi.fn(() => Promise.resolve(awaitingConfirmation));
    renderPage(
      {
        canManageInterfaces: true,
        previewInterfaceChange,
        acknowledgeOperation,
      },
      'adapters',
    );
    await screen.findAllByText('enp1s0');

    chooseAction('enp1s0', /disable enp1s0/i);

    const dialog = await screen.findByRole('dialog');
    expect(within(dialog).getByText(/sim-enp1s0/)).toBeInTheDocument();
    expect(within(dialog).getByText(/interface.disable/)).toBeInTheDocument();
    expect(previewInterfaceChange).toHaveBeenCalledWith('sim-enp1s0', 'disable');
    expect(acknowledgeOperation).not.toHaveBeenCalled();

    fireEvent.click(within(dialog).getByRole('button', { name: /^apply$/i }));
    await waitFor(() =>
      expect(acknowledgeOperation).toHaveBeenCalledWith('operation-1', 'revision-9f2c'),
    );
  });

  it('abandons a previewed change without acknowledging it', async () => {
    const acknowledgeOperation = vi.fn(() => Promise.resolve(awaitingConfirmation));
    renderPage({ canManageInterfaces: true, acknowledgeOperation }, 'adapters');
    await screen.findAllByText('enp1s0');

    chooseAction('enp1s0', /disable enp1s0/i);
    const dialog = await screen.findByRole('dialog');
    fireEvent.click(within(dialog).getByRole('button', { name: /cancel/i }));

    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(acknowledgeOperation).not.toHaveBeenCalled();
  });

  it('counts the confirmation window down and offers both ways out', async () => {
    renderPage({ canManageInterfaces: true }, 'adapters');
    await screen.findAllByText('enp1s0');

    chooseAction('enp1s0', /disable enp1s0/i);
    fireEvent.click(
      within(await screen.findByRole('dialog')).getByRole('button', {
        name: /^apply$/i,
      }),
    );

    // A countdown, and both exits, reachable without hunting for them.
    expect(await screen.findByRole('timer')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /keep change/i })).toBeInTheDocument();
    expect(
      screen.getByRole('button', { name: /restore previous state/i }),
    ).toBeInTheDocument();
  });

  it('keeps the change when the administrator confirms inside the window', async () => {
    const confirmOperation = vi.fn(() =>
      Promise.resolve({ ...awaitingConfirmation, state: 'confirmed' as const }),
    );
    renderPage({ canManageInterfaces: true, confirmOperation }, 'adapters');
    await screen.findAllByText('enp1s0');

    chooseAction('enp1s0', /disable enp1s0/i);
    fireEvent.click(
      within(await screen.findByRole('dialog')).getByRole('button', {
        name: /^apply$/i,
      }),
    );
    fireEvent.click(await screen.findByRole('button', { name: /keep change/i }));

    await waitFor(() => expect(confirmOperation).toHaveBeenCalledWith('operation-1'));
    await waitFor(() => expect(screen.queryByRole('timer')).toBeNull());
    fireEvent.click(screen.getByRole('button', { name: 'Actions for enp1s0' }));
    expect(
      within(screen.getByRole('dialog', { name: 'Actions for enp1s0' })).getByRole(
        'button',
        {
          name: /disable enp1s0/i,
        },
      ),
    ).toBeEnabled();
  });

  it('restores the previous state when the administrator asks for it', async () => {
    const rollbackOperation = vi.fn(() =>
      Promise.resolve({ ...awaitingConfirmation, state: 'rolled_back' as const }),
    );
    renderPage({ canManageInterfaces: true, rollbackOperation }, 'adapters');
    await screen.findAllByText('enp1s0');

    chooseAction('enp1s0', /disable enp1s0/i);
    fireEvent.click(
      within(await screen.findByRole('dialog')).getByRole('button', {
        name: /^apply$/i,
      }),
    );
    fireEvent.click(
      await screen.findByRole('button', { name: /restore previous state/i }),
    );

    await waitFor(() => expect(rollbackOperation).toHaveBeenCalledWith('operation-1'));
    expect(await screen.findByText(/previous state was restored/i)).toBeInTheDocument();
  });

  // A rollback that could not be verified is the one outcome that must not be
  // reported quietly, because the appliance may be in neither state.
  it('reports a rollback that was not verified prominently', async () => {
    renderPage(
      {
        canManageInterfaces: true,
        rollbackOperation: () =>
          Promise.resolve({
            ...awaitingConfirmation,
            state: 'rollback_failed',
            failure: 'the previous connection was not observed after restoring',
          }),
      },
      'adapters',
    );
    await screen.findAllByText('enp1s0');

    chooseAction('enp1s0', /disable enp1s0/i);
    fireEvent.click(
      within(await screen.findByRole('dialog')).getByRole('button', {
        name: /^apply$/i,
      }),
    );
    fireEvent.click(
      await screen.findByRole('button', { name: /restore previous state/i }),
    );

    const alert = await screen.findByRole('alert');
    expect(alert).toHaveTextContent(/not observed after restoring/i);
  });

  // A count of minutes is not a chart. The plan asks for a traffic chart, and a
  // chart has to place a measured minute where it happened.
  it('draws one mark per measured minute, positioned by time', async () => {
    const from = new Date('2026-09-03T09:45:00Z');
    const at = (minute: number) => new Date(from.getTime() + minute * 60_000);
    renderPage(
      {
        getNetworkStatistics: () =>
          Promise.resolve({
            ...statistics,
            from,
            to: new Date(from.getTime() + 15 * 60_000),
            // Minutes 0, 1 and 3 measured; minute 2 was never observed.
            points: [0, 1, 3].map((minute) => ({
              ...statistics.points[0],
              minuteStart: at(minute),
              rxBytes: 1000 * (minute + 1),
            })),
          }),
      },
      'diagnostics',
    );

    const chart = await screen.findByRole('group', { name: /traffic/i });
    const marks = chart.querySelectorAll('[data-minute]');
    expect(marks).toHaveLength(3);

    const drawn = Array.from(marks).map((mark) => mark.getAttribute('data-minute'));
    expect(drawn).toEqual([
      at(0).toISOString(),
      at(1).toISOString(),
      at(3).toISOString(),
    ]);

    // The unmeasured minute leaves a hole rather than the next mark sliding
    // left into it: minute 3 sits three steps along, not two.
    const x = Array.from(marks).map((mark) => Number(mark.getAttribute('x')));
    expect(x[1] - x[0]).toBeGreaterThan(0);
    expect(x[2] - x[1]).toBeCloseTo(2 * (x[1] - x[0]), 1);
  });

  it('says in words what the chart shows, for a reader who cannot see it', async () => {
    renderPage({}, 'diagnostics');
    const chart = await screen.findByRole('group', { name: /traffic/i });
    expect(chart.getAttribute('aria-label')).toMatch(/1 measured minute/i);
  });

  it('draws no chart when nothing was measured, and says so instead', async () => {
    renderPage(
      {
        getNetworkStatistics: () =>
          Promise.resolve({ ...statistics, stale: true, points: [] }),
      },
      'diagnostics',
    );
    expect(await screen.findByText(/no traffic was measured/i)).toBeInTheDocument();
    expect(screen.queryByRole('group', { name: /traffic/i })).toBeNull();
  });

  it('charts the interface the operator chose', async () => {
    // Typed parameters so the assertion below can read which interface was
    // asked for, rather than indexing an untyped empty tuple.
    const getNetworkStatistics = vi.fn((interfaceId: string) =>
      Promise.resolve({ ...statistics, interfaceId }),
    );
    renderPage({ getNetworkStatistics }, 'diagnostics');
    await screen.findAllByText('enp1s0');
    await waitFor(() => expect(getNetworkStatistics).toHaveBeenCalled());

    const chooser = screen.getByRole('combobox', { name: 'Interface' });
    fireEvent.change(chooser, { target: { value: 'sim-lo' } });

    await waitFor(() => {
      const last = getNetworkStatistics.mock.calls.at(-1);
      expect(last?.[0]).toBe('sim-lo');
    });
  });

  it('shows which period is selected', async () => {
    renderPage({}, 'diagnostics');
    await screen.findAllByText('enp1s0');

    expect(screen.getByRole('button', { name: '15 minutes' })).toHaveAttribute(
      'aria-pressed',
      'true',
    );
    fireEvent.click(screen.getByRole('button', { name: '1 hour' }));
    await waitFor(() =>
      expect(screen.getByRole('button', { name: '1 hour' })).toHaveAttribute(
        'aria-pressed',
        'true',
      ),
    );
    expect(screen.getByRole('button', { name: '15 minutes' })).toHaveAttribute(
      'aria-pressed',
      'false',
    );
  });

  it('reports an unreadable status as unavailable instead of as an empty appliance', async () => {
    renderPage({
      getNetworkStatus: () =>
        Promise.reject(new BenchCoreError('unavailable', 'backend stopped')),
    });
    expect(await screen.findByText(/could not be read/i)).toBeInTheDocument();
    expect(screen.queryByText('enp1s0')).not.toBeInTheDocument();
  });
});
