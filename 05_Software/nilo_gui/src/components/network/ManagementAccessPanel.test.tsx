import { fireEvent, screen, waitFor, within } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import type { ManagementAccessStatus } from '../../api/management-access';
import { renderInShell } from '../../test/render';
import { ManagementAccessPanel } from './ManagementAccessPanel';

const applied: ManagementAccessStatus = {
  state: 'applied',
  port: 8443,
  interfaces: [
    {
      name: 'wlan0',
      kind: 'wifi',
      state: 'connected',
      ipv4: ['192.168.1.103/24'],
      ipv6: [],
      enabled: true,
      current: true,
      usable: true,
    },
    {
      name: 'nilo0',
      kind: 'wireguard',
      state: 'connected',
      ipv4: ['10.205.148.11/32'],
      ipv6: [],
      enabled: true,
      current: false,
      usable: true,
    },
    {
      name: 'eth0',
      kind: 'ethernet',
      state: 'connected',
      ipv4: ['192.168.0.1/24'],
      ipv6: [],
      enabled: false,
      current: false,
      usable: true,
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
  currentInterface: 'wlan0',
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
});
