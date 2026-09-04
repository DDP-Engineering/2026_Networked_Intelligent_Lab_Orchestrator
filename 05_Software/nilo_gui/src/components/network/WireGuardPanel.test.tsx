import { fireEvent, screen } from '@testing-library/react';
import { expect, it, vi } from 'vitest';
import { renderInShell } from '../../test/render';
import type { WireGuardStatus } from '../../api/wireguard';
import { WireGuardPanel } from './WireGuardPanel';

const status: WireGuardStatus = {
  reported: true,
  measuredBy: 'simulation',
  networkManagerAvailable: true,
  networkManagerVersion: 'simulated',
  kernelSupported: true,
  helperAvailable: true,
  canInstall: false,
  tools: [{ name: 'wg', installed: true, version: 'simulated' }],
  detail: '',
  profiles: [
    {
      id: 'office',
      name: 'Office',
      interface: 'sim-wg',
      niloOwned: true,
      active: false,
      autoconnect: false,
      lastHandshake: null,
      addresses: ['10.7.0.2/24'],
      dns: ['10.7.0.1'],
      peers: [{ endpoint: 'vpn.example.test:51820', allowedIPs: ['0.0.0.0/0'] }],
    },
  ],
};

it('keeps profile data visible and opens import only on request', () => {
  renderInShell(
    <WireGuardPanel status={status} canManage canInstall={false} propose={vi.fn()} />,
  );
  expect(screen.getByText('10.7.0.2/24')).toBeVisible();
  expect(screen.getByText('vpn.example.test:51820')).toBeVisible();
  expect(screen.queryByLabelText('Profile name')).not.toBeInTheDocument();
  expect(screen.queryByText(/simulated environment/i)).not.toBeInTheDocument();
  fireEvent.click(screen.getByRole('button', { name: 'Import profile' }));
  expect(screen.getByLabelText('Profile name')).toBeVisible();
});

it('shows readiness without claiming an empty profile inventory or offering management', () => {
  renderInShell(
    <WireGuardPanel
      status={{ ...status, readinessOnly: true, profiles: [], measuredBy: 'test-host' }}
      canManage
      canInstall
      propose={vi.fn()}
    />,
  );
  expect(screen.getByText('Diagnostics').closest('details')).not.toHaveAttribute(
    'open',
  );
  expect(
    screen.getByText('WireGuard profiles have not been reported.'),
  ).toBeInTheDocument();
  expect(screen.queryAllByRole('button')).toHaveLength(0);
  expect(screen.queryByLabelText('Profile name')).not.toBeInTheDocument();
});

it('proposes activation and explicit startup selection without applying either', async () => {
  const propose = vi.fn();
  renderInShell(
    <WireGuardPanel status={status} canManage canInstall={false} propose={propose} />,
  );
  fireEvent.click(screen.getByRole('button', { name: 'Enable Office' }));
  expect(propose).toHaveBeenCalledWith({ action: 'activate', target: 'office' });
  fireEvent.click(screen.getByRole('checkbox', { name: 'Connect at startup: Office' }));
  expect(propose).toHaveBeenCalledWith({
    action: 'autostart',
    target: 'office',
    enabled: true,
  });
});

it('shows telemetry and no mutation controls for viewers', () => {
  renderInShell(
    <WireGuardPanel
      status={status}
      canManage={false}
      canInstall={false}
      propose={vi.fn()}
    />,
  );
  expect(screen.getByText('Office')).toBeInTheDocument();
  expect(
    screen.queryByRole('button', { name: /enable|disable|remove|import/i }),
  ).toBeNull();
  fireEvent.click(screen.getByRole('button', { name: 'Details Office' }));
  expect(screen.getByText('10.7.0.1')).toBeVisible();
  expect(screen.getByText('0.0.0.0/0')).toBeVisible();
  expect(screen.queryAllByRole('checkbox')).toHaveLength(0);
});

it('offers acknowledged installation when tools can be installed even if kernel evidence is unknown', () => {
  const propose = vi.fn();
  renderInShell(
    <WireGuardPanel
      status={{
        ...status,
        reported: false,
        readinessOnly: true,
        canInstall: true,
        profiles: [],
      }}
      canManage
      canInstall
      propose={propose}
    />,
  );
  fireEvent.click(screen.getByRole('button', { name: 'Install WireGuard tools' }));
  expect(propose).toHaveBeenCalledWith({ action: 'install' });
  expect(screen.queryByLabelText('Profile name')).not.toBeInTheDocument();
});

it('disables another install while a network operation is pending', () => {
  renderInShell(
    <WireGuardPanel
      status={{ ...status, canInstall: true }}
      canManage
      canInstall
      busy
      propose={vi.fn()}
    />,
  );
  expect(
    screen.getByRole('button', { name: 'Install WireGuard tools' }),
  ).toBeDisabled();
});
