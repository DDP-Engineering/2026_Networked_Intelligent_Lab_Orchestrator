import { fireEvent, screen, waitFor, within } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import { BenchCoreError } from '../../api/client';
import type { ManagementAccessStatus } from '../../api/management-access';
import { renderInShell } from '../../test/render';
import { ManagementAccessPanel } from './ManagementAccessPanel';

const applied: ManagementAccessStatus = {
  state: 'applied',
  port: 8443,
  interfaces: [
    {
      name: 'wlan0',
      adapterId: 'wifi-a',
      kind: 'wifi',
      state: 'connected',
      ipv4: ['192.168.1.103/24'],
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
    {
      name: 'nilo0',
      adapterId: '',
      kind: 'wireguard',
      state: 'connected',
      ipv4: ['10.205.148.11/32'],
      ipv6: [],
      enabled: true,
      current: false,
      usable: true,
      role: '',
      roleAvailable: true,
      roleDetail: '',
      conflictingResourceId: '',
      conflictingResourceName: '',
    },
    {
      name: 'eth0',
      adapterId: 'ethernet-a',
      kind: 'ethernet',
      state: 'connected',
      ipv4: ['192.168.0.1/24'],
      ipv6: [],
      enabled: false,
      current: false,
      usable: true,
      role: '',
      roleAvailable: true,
      roleDetail: '',
      conflictingResourceId: '',
      conflictingResourceName: '',
    },
  ],
  configuredInterfaces: ['wlan0', 'nilo0'],
  urls: [
    { url: 'https://ddp-remote.local:8443', kind: 'local' },
    { url: 'https://10.205.148.11:8443', kind: 'vpn' },
  ],
  tlsNames: ['ddp-remote.local', '10.205.148.11'],
  tlsMissing: [],
  confirmBy: null,
  detail: '',
  outcome: { code: '', interface: '', resourceName: '' },
  currentInterface: 'wlan0',
  provisioning: false,
};

describe('ManagementAccessPanel', () => {
  it('distinguishes an absent saved adapter from its same-name replacement', async () => {
    const replacement: ManagementAccessStatus = {
      ...applied,
      configuredInterfaces: ['nilo0'],
      currentInterface: 'nilo0',
      interfaces: [
        {
          ...applied.interfaces[0],
          adapterId: 'wifi-new',
          enabled: false,
          current: false,
          role: '',
        },
        {
          ...applied.interfaces[0],
          state: 'unavailable',
          enabled: true,
          current: false,
          usable: false,
          roleAvailable: false,
          ipv4: [],
        },
        applied.interfaces[1],
      ],
    };
    const observedReplacement = {
      ...replacement.interfaces[0],
      id: 'observed-wifi-new',
      linkCapacityMbps: 100,
      rxBytesPerSecond: 0,
      txBytesPerSecond: 0,
      controllable: true,
      notControllableReason: '',
      ssid: 'DDP',
      settings: null,
    };
    const toggle = vi.fn();
    renderInShell(
      <ManagementAccessPanel
        read={() => Promise.resolve(replacement)}
        interfaces={[observedReplacement]}
        onToggle={toggle}
        update={() => Promise.resolve(replacement)}
        confirm={() => Promise.resolve(replacement)}
        recover={() => Promise.resolve(replacement)}
      />,
      { theme: 'light', language: 'en' },
    );
    const saved = await screen.findByText('wifi-a');
    const absentRow = saved.closest('tr')!;
    const absent = within(absentRow).getByRole('checkbox');
    expect(absent).toBeChecked();
    expect(absent).toBeDisabled();
    expect(within(absentRow).queryByRole('button')).toBeNull();
    expect(screen.getAllByRole('table')).toHaveLength(1);
    expect(screen.getAllByRole('checkbox', { name: /wlan0/ })).toHaveLength(2);
    fireEvent.click(screen.getByRole('button', { name: 'Actions for wlan0' }));
    fireEvent.click(screen.getByRole('button', { name: /^Disable wlan0/ }));
    expect(toggle).toHaveBeenCalledWith(observedReplacement);
    const present = screen
      .getAllByRole('checkbox')
      .find(
        (checkbox) =>
          checkbox !== absent && checkbox.getAttribute('aria-label')?.includes('wlan0'),
      )!;
    fireEvent.click(present);
    expect(present).toBeChecked();
    expect(absent).toBeChecked();
  });

  it('retains both absent identities sharing a name while another selection changes', async () => {
    const observation: ManagementAccessStatus = {
      ...applied,
      configuredInterfaces: ['nilo0'],
      currentInterface: 'nilo0',
      interfaces: [
        ...['wifi-a', 'wifi-b'].map((adapterId) => ({
          ...applied.interfaces[0],
          adapterId,
          state: 'unavailable' as const,
          current: false,
          usable: false,
          roleAvailable: false,
          ipv4: [],
        })),
        applied.interfaces[1],
        applied.interfaces[2],
      ],
    };
    const update = vi.fn(() => Promise.resolve(observation));
    renderInShell(
      <ManagementAccessPanel
        read={() => Promise.resolve(observation)}
        update={update}
        confirm={() => Promise.resolve(observation)}
        recover={() => Promise.resolve(observation)}
      />,
      { theme: 'light', language: 'en' },
    );
    const absent = [];
    for (const identity of ['wifi-a', 'wifi-b']) {
      const row = (await screen.findByText(identity)).closest('tr')!;
      const checkbox = within(row).getByRole('checkbox');
      expect(checkbox).toBeChecked();
      expect(checkbox).toBeDisabled();
      absent.push(checkbox);
    }
    fireEvent.click(screen.getByRole('checkbox', { name: /eth0/ }));
    for (const checkbox of absent) expect(checkbox).toBeChecked();
    fireEvent.click(screen.getByRole('button', { name: 'Apply management access' }));
    await waitFor(() => expect(update).toHaveBeenCalledWith(['nilo0', 'eth0'], false));
  });

  it('shows one reading line while the management observation is pending', () => {
    renderInShell(
      <ManagementAccessPanel
        read={() => new Promise<ManagementAccessStatus>(() => undefined)}
        update={() => Promise.resolve(applied)}
        confirm={() => Promise.resolve(applied)}
        recover={() => Promise.resolve(applied)}
      />,
      { theme: 'light', language: 'en' },
    );

    expect(screen.getByRole('status')).toHaveTextContent(
      'Reading the appliance network state…',
    );
    expect(screen.queryByRole('alert')).toBeNull();
  });

  it('keeps pending confirmation actions beside one management status line', async () => {
    const pending: ManagementAccessStatus = {
      ...applied,
      state: 'pending',
      confirmBy: new Date(Date.now() + 90_000),
    };
    renderInShell(
      <ManagementAccessPanel
        read={() => Promise.resolve(pending)}
        update={() => Promise.resolve(pending)}
        confirm={() => Promise.resolve(applied)}
        recover={() => Promise.resolve(applied)}
      />,
      { theme: 'light', language: 'en' },
    );

    const region = await screen.findByRole('region', {
      name: 'Management access interfaces',
    });
    expect(within(region).getAllByRole('status')).toHaveLength(1);
    expect(within(region).getByRole('status')).toHaveTextContent(
      'Pending confirmation',
    );
    expect(within(region).getByRole('button', { name: 'Keep change' })).toBeVisible();
    expect(
      within(region).getByRole('button', { name: 'Restore previous state' }),
    ).toBeVisible();
    expect(within(region).queryByText(/confirm it here or restore/i)).toBeNull();
  });

  it('requires an explicit second confirmation before disabling the current session interface', async () => {
    const update = vi.fn().mockResolvedValue({
      ...applied,
      state: 'pending',
      configuredInterfaces: ['nilo0'],
      confirmBy: new Date(Date.now() + 90_000),
    });
    renderInShell(
      <ManagementAccessPanel
        read={() => Promise.resolve(applied)}
        update={update}
        confirm={() => Promise.resolve(applied)}
        recover={() => Promise.resolve(applied)}
      />,
      { theme: 'light', language: 'en' },
    );
    const region = await screen.findByRole('region', {
      name: 'Management access interfaces',
    });
    fireEvent.click(
      within(region).getByRole('checkbox', {
        name: 'Allow management access on wlan0',
      }),
    );
    fireEvent.click(
      within(region).getByRole('button', { name: 'Apply management access' }),
    );
    expect(update).not.toHaveBeenCalled();
    const warning = within(region).getByRole('alertdialog');
    expect(warning).toHaveTextContent('wlan0');
    fireEvent.click(
      within(warning).getByRole('button', {
        name: 'Disable current interface and apply',
      }),
    );
    await waitFor(() => expect(update).toHaveBeenCalledWith(['nilo0'], true));
  });

  it('shows stable local and VPN URLs without promoting the DHCP address', async () => {
    renderInShell(
      <ManagementAccessPanel
        read={() => Promise.resolve(applied)}
        update={() => Promise.resolve(applied)}
        confirm={() => Promise.resolve(applied)}
        recover={() => Promise.resolve(applied)}
      />,
      { theme: 'light', language: 'en' },
    );
    await screen.findByText('https://ddp-remote.local:8443');
    expect(screen.getByText('https://10.205.148.11:8443')).toBeInTheDocument();
    expect(
      screen.getByText(/Wi-Fi DHCP addresses are current observations/i),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole('link', { name: /192\.168\.1\.103/ }),
    ).not.toBeInTheDocument();
  });

  it('keeps a device-owned adapter visible, disabled, and explained', async () => {
    const claimed: ManagementAccessStatus = {
      ...applied,
      interfaces: applied.interfaces.map((entry) =>
        entry.name === 'eth0'
          ? {
              ...entry,
              role: 'device',
              roleAvailable: false,
              roleDetail: 'assigned to the device role by PLC line',
              conflictingResourceId: 'dn-plc',
              conflictingResourceName: 'PLC line',
            }
          : entry,
      ),
    };
    renderInShell(
      <ManagementAccessPanel
        read={() => Promise.resolve(claimed)}
        update={() => Promise.resolve(claimed)}
        confirm={() => Promise.resolve(claimed)}
        recover={() => Promise.resolve(claimed)}
      />,
      { theme: 'light', language: 'en' },
    );

    const region = await screen.findByRole('region', {
      name: 'Management access interfaces',
    });
    expect(
      within(region).getByRole('checkbox', {
        name: 'Allow management access on eth0',
      }),
    ).toBeDisabled();
    expect(within(region).getByText(/Device side: PLC line/i)).toBeVisible();
    expect(
      within(region).getByRole('checkbox', {
        name: 'Allow management access on wlan0',
      }),
    ).toBeEnabled();
  });

  it('keeps a recovered outcome visible until refresh supersedes it', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const read = vi
      .fn()
      .mockResolvedValueOnce({
        ...applied,
        state: 'recovered',
        detail: 'backend wording must not be rendered',
        outcome: {
          code: 'role_conflict_recovered',
          interface: 'eth0',
          resourceName: 'PLC line',
        },
      })
      .mockResolvedValue(applied);
    try {
      renderInShell(
        <ManagementAccessPanel
          read={read}
          update={() => Promise.resolve(applied)}
          confirm={() => Promise.resolve(applied)}
          recover={() => Promise.resolve(applied)}
        />,
        { theme: 'light', language: 'en' },
      );

      expect(
        await screen.findByText(/Management access was removed from eth0/i),
      ).toBeVisible();
      expect(
        screen.getByText(/Management access was removed from eth0 because PLC line/i),
      ).toBeVisible();
      expect(screen.queryByText(/backend wording/)).toBeNull();
      await vi.advanceTimersByTimeAsync(5000);
      await waitFor(() =>
        expect(
          screen.queryByText(/Management access was removed from eth0/i),
        ).toBeNull(),
      );
      expect(screen.getByText(/Applied and confirmed/i)).toBeVisible();
    } finally {
      vi.useRealTimers();
    }
  });

  it.each([
    ['de', /Verwaltungszugriff wurde von eth0 entfernt/],
    ['ro', /Accesul de administrare a fost eliminat de pe eth0/],
  ] as const)(
    'localizes a recovered role conflict in %s',
    async (language, message) => {
      const recovered: ManagementAccessStatus = {
        ...applied,
        state: 'recovered',
        detail: 'English backend detail',
        outcome: {
          code: 'role_conflict_recovered',
          interface: 'eth0',
          resourceName: 'PLC line',
        },
      };
      renderInShell(
        <ManagementAccessPanel
          read={() => Promise.resolve(recovered)}
          update={() => Promise.resolve(recovered)}
          confirm={() => Promise.resolve(recovered)}
          recover={() => Promise.resolve(recovered)}
        />,
        { theme: 'light', language },
      );

      expect(await screen.findByText(message)).toBeVisible();
      expect(screen.queryByText(/English backend detail/)).toBeNull();
    },
  );

  it('keeps a management-update conflict until observed inventory supersedes its device claim', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const claimed: ManagementAccessStatus = {
      ...applied,
      interfaces: applied.interfaces.map((entry) =>
        entry.name === 'eth0'
          ? {
              ...entry,
              role: 'device',
              roleAvailable: false,
              roleDetail: 'assigned to the device role by PLC line',
              conflictingResourceId: 'dn-plc',
              conflictingResourceName: 'PLC line',
            }
          : entry,
      ),
    };
    const read = vi
      .fn()
      .mockResolvedValueOnce(applied)
      .mockResolvedValueOnce(claimed)
      .mockResolvedValue(applied);
    try {
      renderInShell(
        <ManagementAccessPanel
          read={read}
          update={() =>
            Promise.reject(
              new BenchCoreError(
                'rejected',
                'untrusted backend role wording',
                'network_interface_role_conflict',
                {
                  adapterId: 'ethernet-a',
                  interface: 'eth0',
                  existingRole: 'device',
                  requestedRole: 'management',
                  conflictingResourceId: 'dn-plc',
                  conflictingResourceName: 'PLC line',
                },
              ),
            )
          }
          confirm={() => Promise.resolve(applied)}
          recover={() => Promise.resolve(applied)}
        />,
        { theme: 'light', language: 'en' },
      );

      const region = await screen.findByRole('region', {
        name: 'Management access interfaces',
      });
      fireEvent.click(
        within(region).getByRole('checkbox', {
          name: 'Allow management access on eth0',
        }),
      );
      fireEvent.click(
        within(region).getByRole('button', { name: 'Apply management access' }),
      );
      expect(await within(region).findByRole('alert')).toHaveTextContent(
        /device-side adapter for PLC line/i,
      );

      await vi.advanceTimersByTimeAsync(5000);
      await waitFor(() => expect(read).toHaveBeenCalledTimes(2));
      expect(within(region).getByRole('alert')).toHaveTextContent(
        /device-side adapter for PLC line/i,
      );

      await vi.advanceTimersByTimeAsync(5000);
      await waitFor(() => expect(read).toHaveBeenCalledTimes(3));
      await waitFor(() => expect(within(region).queryByRole('alert')).toBeNull());
      expect(within(region).queryByText('untrusted backend role wording')).toBeNull();
    } finally {
      vi.useRealTimers();
    }
  });

  it('shows one primary line when the management observation fails', async () => {
    renderInShell(
      <ManagementAccessPanel
        read={() =>
          Promise.reject(new Error('management access lists an invalid interface'))
        }
        update={() => Promise.resolve(applied)}
        confirm={() => Promise.resolve(applied)}
        recover={() => Promise.resolve(applied)}
      />,
      { theme: 'light', language: 'en' },
    );

    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Management access is unavailable.',
    );
    expect(screen.queryByText('Reading the appliance network state…')).toBeNull();
    expect(
      screen.queryByText('management access lists an invalid interface'),
    ).toBeNull();
  });

  it('uses an accessible region name without repeating a heading or instructions', async () => {
    renderInShell(
      <ManagementAccessPanel
        read={() => Promise.resolve(applied)}
        update={() => Promise.resolve(applied)}
        confirm={() => Promise.resolve(applied)}
        recover={() => Promise.resolve(applied)}
      />,
      { theme: 'light', language: 'en' },
    );

    const region = await screen.findByRole('region', {
      name: 'Management access interfaces',
    });
    expect(
      within(region).queryByRole('heading', { name: 'Management access interfaces' }),
    ).toBeNull();
    expect(within(region).queryByText(/Choose the interface names/i)).toBeNull();
    expect(
      within(region).queryByRole('button', { name: 'Refresh interfaces' }),
    ).toBeNull();
    expect(
      within(region).queryByRole('button', { name: 'How to repurpose an adapter' }),
    ).toBeNull();
  });

  it('never renders backend prose for a malformed structured role conflict', async () => {
    renderInShell(
      <ManagementAccessPanel
        read={() => Promise.resolve(applied)}
        update={() =>
          Promise.reject(
            new BenchCoreError(
              'rejected',
              'untrusted backend role wording',
              'network_interface_role_conflict',
            ),
          )
        }
        confirm={() => Promise.resolve(applied)}
        recover={() => Promise.resolve(applied)}
      />,
      { theme: 'light', language: 'en' },
    );

    const region = await screen.findByRole('region', {
      name: 'Management access interfaces',
    });
    fireEvent.click(
      within(region).getByRole('checkbox', {
        name: 'Allow management access on eth0',
      }),
    );
    fireEvent.click(
      within(region).getByRole('button', { name: 'Apply management access' }),
    );
    expect(
      await within(region).findByText(/adapter role conflict could not be read/i),
    ).toBeVisible();
    expect(within(region).queryByText('untrusted backend role wording')).toBeNull();
  });
});
