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
      within(region).getByRole('button', { name: 'Apply interface allowlist' }),
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
    expect(within(region).getByText(/device-side adapter for PLC line/i)).toBeVisible();
    expect(
      within(region).getByRole('checkbox', {
        name: 'Allow management access on wlan0',
      }),
    ).toBeEnabled();

    fireEvent.click(
      within(region).getByRole('button', { name: 'How to repurpose an adapter' }),
    );
    const guide = screen.getByRole('dialog', { name: 'Safe adapter repurposing' });
    expect(within(guide).getByText(/^1\. Establish/)).toBeVisible();
    expect(within(guide).getByText(/^2\. Remove/)).toBeVisible();
    expect(within(guide).getByText(/^3\. Assign/)).toBeVisible();
    expect(within(guide).queryByRole('button', { name: /swap/i })).toBeNull();
  });

  it('keeps a recovered outcome visible until refresh supersedes it', async () => {
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
    renderInShell(
      <ManagementAccessPanel
        read={read}
        update={() => Promise.resolve(applied)}
        confirm={() => Promise.resolve(applied)}
        recover={() => Promise.resolve(applied)}
      />,
      { theme: 'light', language: 'en' },
    );

    expect(await screen.findByText(/Last known-good access recovered/i)).toBeVisible();
    expect(
      screen.getByText(/Management access was removed from eth0 because PLC line/i),
    ).toBeVisible();
    expect(screen.queryByText(/backend wording/)).toBeNull();
    fireEvent.click(screen.getByRole('button', { name: 'Refresh interfaces' }));
    await waitFor(() =>
      expect(screen.queryByText(/Management access was removed from eth0/i)).toBeNull(),
    );
    expect(screen.getByText(/Applied and confirmed/i)).toBeVisible();
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
      within(region).getByRole('button', { name: 'Apply interface allowlist' }),
    );
    expect(await within(region).findByRole('alert')).toHaveTextContent(
      /device-side adapter for PLC line/i,
    );

    fireEvent.click(within(region).getByRole('button', { name: 'Refresh interfaces' }));
    await waitFor(() => expect(read).toHaveBeenCalledTimes(2));
    expect(within(region).getByRole('alert')).toHaveTextContent(
      /device-side adapter for PLC line/i,
    );

    fireEvent.click(within(region).getByRole('button', { name: 'Refresh interfaces' }));
    await waitFor(() => expect(read).toHaveBeenCalledTimes(3));
    await waitFor(() => expect(within(region).queryByRole('alert')).toBeNull());
    expect(within(region).queryByText('untrusted backend role wording')).toBeNull();
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
      within(region).getByRole('button', { name: 'Apply interface allowlist' }),
    );
    expect(
      await within(region).findByText(/adapter role conflict could not be read/i),
    ).toBeVisible();
    expect(within(region).queryByText('untrusted backend role wording')).toBeNull();
  });
});
