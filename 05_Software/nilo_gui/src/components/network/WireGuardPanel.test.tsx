import { fireEvent, screen, within } from '@testing-library/react';
import { expect, it, vi } from 'vitest';
import { renderInShell } from '../../test/render';
import type { WireGuardStatus } from '../../api/wireguard';
import { WireGuardDiagnostics, WireGuardPanel } from './WireGuardPanel';

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

/** Open the profile's compact Actions dialog and choose one action. */
function chooseAction(profile: string, action: string): void {
  fireEvent.click(screen.getByRole('button', { name: `Actions for ${profile}` }));
  fireEvent.click(
    within(screen.getByRole('dialog', { name: `Actions for ${profile}` })).getByRole(
      'button',
      { name: action },
    ),
  );
}

// REQ-UX-ff989933: name, state, tunnel address and startup stay in the row;
// endpoint, handshake, DNS and routes are one Details away.
it('keeps the essential profile data in the row and the rest behind Details', () => {
  renderInShell(
    <WireGuardPanel status={status} canManage canInstall={false} propose={vi.fn()} />,
  );
  expect(screen.getByText('10.7.0.2/24')).toBeVisible();
  expect(
    screen.getByRole('checkbox', { name: 'Connect at startup: Office' }),
  ).toBeVisible();
  expect(screen.queryByText('vpn.example.test:51820')).toBeNull();
  expect(screen.queryByLabelText('Profile name')).not.toBeInTheDocument();
  expect(screen.queryByText(/simulated environment/i)).not.toBeInTheDocument();

  fireEvent.click(screen.getByRole('button', { name: 'Details Office' }));
  const details = screen.getByRole('region', { name: 'Details Office' });
  expect(within(details).getByText('vpn.example.test:51820')).toBeVisible();
  expect(within(details).getByText('10.7.0.1')).toBeVisible();
  expect(within(details).getByText('0.0.0.0/0')).toBeVisible();
  expect(within(details).getByText('sim-wg')).toBeVisible();
  expect(within(details).getByLabelText('New name')).toBeVisible();

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

// REQ-UX-823c3e13: every readiness value belongs to one compact fact so the
// browser can place the label on the left and its value on the right.
it('groups every expanded readiness label with its value', () => {
  renderInShell(<WireGuardDiagnostics status={status} collapsible={false} />);

  const readiness = screen.getByRole('region', { name: 'WireGuard readiness' });
  const facts = readiness.querySelectorAll<HTMLElement>('dl > div');
  expect(facts).toHaveLength(4);
  expect(within(facts[0]).getByText('Network manager')).toBeVisible();
  expect(within(facts[0]).getByText('simulated')).toBeVisible();
  expect(within(facts[1]).getByText('Kernel support')).toBeVisible();
  expect(within(facts[1]).getByText('Available')).toBeVisible();
  expect(within(facts[2]).getByText('Network helper')).toBeVisible();
  expect(within(facts[3]).getByText('wg')).toBeVisible();
});

it('proposes activation and explicit startup selection without applying either', async () => {
  const propose = vi.fn();
  renderInShell(
    <WireGuardPanel status={status} canManage canInstall={false} propose={propose} />,
  );
  chooseAction('Office', 'Enable Office');
  expect(propose).toHaveBeenCalledWith({ action: 'activate', target: 'office' });
  fireEvent.click(screen.getByRole('checkbox', { name: 'Connect at startup: Office' }));
  expect(propose).toHaveBeenCalledWith({
    action: 'autostart',
    target: 'office',
    enabled: true,
  });
});

it('offers every permitted profile action from one dialog and says why one is withheld', () => {
  renderInShell(
    <WireGuardPanel
      status={{ ...status, profiles: [{ ...status.profiles[0], active: true }] }}
      canManage
      canInstall={false}
      propose={vi.fn()}
    />,
  );
  fireEvent.click(screen.getByRole('button', { name: 'Actions for Office' }));
  const dialog = screen.getByRole('dialog', { name: 'Actions for Office' });
  expect(within(dialog).getByText('WireGuard profile')).toBeVisible();
  expect(within(dialog).getByText('Active')).toBeVisible();
  expect(within(dialog).getByRole('button', { name: 'Disable Office' })).toBeEnabled();
  const remove = within(dialog).getByRole('button', { name: /Remove Office/ });
  expect(remove).toBeDisabled();
  expect(within(dialog).getByText(/active profile cannot be removed/i)).toBeVisible();
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
    screen.queryByRole('button', { name: /enable|disable|remove|import|actions/i }),
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
