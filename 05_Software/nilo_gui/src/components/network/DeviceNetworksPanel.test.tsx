import { fireEvent, screen, waitFor, within } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import type { NetworkInterface, NetworkOperation } from '../../api/contract';
import type { DeviceNetwork, DeviceNetworkChange } from '../../api/devicenetworks';
import type { ManagementAccessInterface } from '../../api/management-access';
import { renderInShell } from '../../test/render';
import { DeviceNetworksPanel } from './DeviceNetworksPanel';

const interfaces: NetworkInterface[] = [
  {
    id: 'sim-enp1s0',
    name: 'enp1s0',
    kind: 'ethernet',
    state: 'connected',
    ipv4: ['192.168.10.24/24'],
    ipv6: [],
    gateway4: '192.168.10.1',
    linkCapacityMbps: 1000,
    rxBytesPerSecond: 0,
    txBytesPerSecond: 0,
    controllable: true,
    notControllableReason: '',
    adapterId: '02:00:00:00:00:01',
    ssid: '',
    settings: null,
  },
  {
    id: 'sim-enp2s0',
    name: 'enp2s0',
    kind: 'ethernet',
    state: 'connected',
    ipv4: ['192.168.0.1/24'],
    ipv6: [],
    linkCapacityMbps: 1000,
    rxBytesPerSecond: 0,
    txBytesPerSecond: 0,
    controllable: true,
    notControllableReason: '',
    adapterId: '02:00:00:00:00:02',
    ssid: '',
    settings: null,
  },
];

const managementInterfaces: ManagementAccessInterface[] = [
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
];

const line: DeviceNetwork = {
  id: 'dn-1',
  name: 'Line 3',
  location: 'Hall B',
  deviceInterface: '02:00:00:00:00:02',
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
    tcpPorts: [502],
    udpPorts: [],
    preserveSource: false,
  },
  enabled: true,
  revision: 'r1',
  state: 'waiting',
  detail: 'the selected WireGuard profile is not connected',
  applied: false,
  observedAt: new Date('2026-09-04T10:00:00Z'),
};

const previewed: NetworkOperation = {
  id: 'operation-1',
  action: 'device_network.create',
  target: 'new',
  state: 'previewed',
  revision: 'revision-1',
  createdAt: new Date('2026-09-04T10:00:00Z'),
  confirmDeadline: null,
  failure: '',
};

function renderPanel(
  overrides: Partial<Parameters<typeof DeviceNetworksPanel>[0]> = {},
) {
  return renderInShell(
    <DeviceNetworksPanel
      networks={[line]}
      interfaces={interfaces}
      profiles={[]}
      canManage
      busy={false}
      unreadable={false}
      propose={() => Promise.resolve(previewed)}
      check={() => Promise.reject(new Error('unused'))}
      instructions={() => Promise.reject(new Error('unused'))}
      {...overrides}
    />,
    { theme: 'light', language: 'en' },
  );
}

/** Open the network's compact Actions dialog and choose one action by name. */
function chooseAction(network: string, action: RegExp | string): void {
  fireEvent.click(screen.getByRole('button', { name: `Actions for ${network}` }));
  fireEvent.click(
    within(screen.getByRole('dialog', { name: `Actions for ${network}` })).getByRole(
      'button',
      { name: action },
    ),
  );
}

function openEditor(): HTMLElement {
  fireEvent.click(screen.getByRole('button', { name: 'Add device network' }));
  return screen.getByRole('dialog', { name: 'New device network' });
}

describe('DeviceNetworksPanel', () => {
  // REQ-UX-1c5eab92 and REQ-FUNC-9b17ce40: intent and observation are two
  // columns, the actions go through a preview, and the activity link is
  // narrowed to the network.
  it('separates the operator intent from the observed state and links to activity', () => {
    const openActivity = vi.fn();
    renderPanel({ openActivity });
    const row = screen.getByRole('row', { name: /Line 3/ });
    expect(within(row).getByText('Waiting')).toBeVisible();
    expect(within(row).getByText('Enabled, not applied')).toBeVisible();
    expect(within(row).getAllByText('192.168.0.130').length).toBeGreaterThan(0);
    // A waiting network says why, in the row, not in a tooltip.
    expect(within(row).getByText(/profile is not connected/)).toBeVisible();
    expect(row.querySelector('[title]')).toBeNull();
    chooseAction('Line 3', 'View activity Line 3');
    expect(openActivity).toHaveBeenCalledWith('dn-1');
  });

  it('disables a management adapter only for the device-facing selection', () => {
    renderInShell(
      <DeviceNetworksPanel
        networks={[line]}
        interfaces={interfaces}
        profiles={[]}
        managementInterfaces={managementInterfaces}
        canManage
        busy={false}
        unreadable={false}
        propose={() => Promise.resolve(previewed)}
        check={() => Promise.reject(new Error('unused'))}
        instructions={() => Promise.reject(new Error('unused'))}
      />,
      { theme: 'light', language: 'en' },
    );

    const editor = openEditor();
    const deviceSide = within(editor).getByLabelText('Device-side adapter');
    expect(within(deviceSide).getByRole('option', { name: /enp1s0/ })).toBeDisabled();
    expect(within(editor).getByText(/reserved for management access/i)).toBeVisible();

    const localIngress = within(editor).getByLabelText('Local-access adapter');
    expect(within(localIngress).getByRole('option', { name: /enp1s0/ })).toBeEnabled();
    expect(within(deviceSide).getByRole('option', { name: /enp2s0/ })).toBeEnabled();

    fireEvent.click(
      within(editor).getByRole('button', { name: 'How to repurpose an adapter' }),
    );
    const guide = screen.getByRole('dialog', { name: 'Safe adapter repurposing' });
    expect(within(guide).getByText(/^1\. Establish/)).toBeVisible();
    expect(within(guide).getByText(/^2\. Remove/)).toBeVisible();
    expect(within(guide).getByText(/^3\. Assign/)).toBeVisible();
  });

  it('blocks preview when a refreshed management claim owns the edited adapter', () => {
    const claimedDeviceAdapter = managementInterfaces.map((entry) => ({
      ...entry,
      name: 'enp2s0',
      adapterId: '02:00:00:00:00:02',
    }));
    renderPanel({ managementInterfaces: claimedDeviceAdapter });

    chooseAction('Line 3', 'Edit Line 3');
    const editor = screen.getByRole('dialog', { name: 'Edit Line 3' });
    expect(within(editor).getByLabelText('Device-side adapter')).toHaveValue(
      '02:00:00:00:00:02',
    );
    expect(within(editor).getByRole('button', { name: 'Preview' })).toBeDisabled();
  });

  it.each([
    ['reading', 'Management adapter roles are still loading.'],
    ['failed', 'Management adapter roles are unavailable.'],
  ] as const)(
    'fails closed while management adapter roles are %s',
    (managementRoleState, message) => {
      const propose = vi.fn(() => Promise.resolve(previewed));
      renderPanel({ managementRoleState, propose });

      const editor = openEditor();
      expect(within(editor).getByText(message)).toBeVisible();
      expect(within(editor).getByLabelText('Device-side adapter')).toBeDisabled();
      expect(within(editor).getByRole('button', { name: 'Preview' })).toBeDisabled();
      expect(propose).not.toHaveBeenCalled();
    },
  );

  // REQ-UX-ff989933: compact columns, details and a viewport-level action dialog.
  it('shows the essential columns, keeps the rest in Details and every action in one dialog', () => {
    renderPanel({ openActivity: vi.fn() });
    for (const column of [
      'Name',
      'Device-side adapter',
      'Device address',
      'Access',
      'Status',
      'Actions',
    ]) {
      expect(screen.getByRole('columnheader', { name: column })).toBeVisible();
    }
    const row = screen.getByRole('row', { name: /Line 3/ });
    expect(within(row).getByText('enp2s0')).toBeVisible();
    expect(within(row).getByText('192.168.0.1/24')).toBeVisible();
    expect(within(row).getByText('Local')).toBeVisible();
    expect(screen.queryByText('192.168.10.0/24')).toBeNull();

    fireEvent.click(screen.getByRole('button', { name: 'Details Line 3' }));
    const details = screen.getByRole('region', { name: 'Details Line 3' });
    expect(within(details).getByText('192.168.10.0/24')).toBeVisible();
    expect(within(details).getByText('enp1s0')).toBeVisible();
    expect(within(details).getByText(/tcp/)).toBeVisible();
    expect(within(details).getByText('502')).toBeVisible();
    expect(within(details).getByText(/source NAT/i)).toBeVisible();
    expect(within(details).getByText(/Hall B/)).toBeVisible();

    fireEvent.click(screen.getByRole('button', { name: 'Actions for Line 3' }));
    const dialog = screen.getByRole('dialog', { name: 'Actions for Line 3' });
    expect(within(dialog).getByText('Device network')).toBeVisible();
    expect(within(dialog).getByText('Waiting')).toBeVisible();
    for (const action of [
      'Edit Line 3',
      'Disable Line 3',
      'Delete Line 3',
      'Check connection Line 3',
      'Access instructions Line 3',
      'View activity Line 3',
    ]) {
      expect(within(dialog).getByRole('button', { name: action })).toBeVisible();
    }
  });

  it('withholds a change while an operation is in flight, and says so', () => {
    renderPanel({ busy: true });
    fireEvent.click(screen.getByRole('button', { name: 'Actions for Line 3' }));
    const dialog = screen.getByRole('dialog', { name: 'Actions for Line 3' });
    const edit = within(dialog).getByRole('button', { name: /Edit Line 3/ });
    expect(edit).toBeDisabled();
    expect(
      within(dialog).getAllByText(/another change is in progress/i).length,
    ).toBeGreaterThan(0);
    expect(
      within(dialog).getByRole('button', { name: 'Access instructions Line 3' }),
    ).toBeEnabled();
  });

  it('previews a new device network from the editor and refuses an incomplete one', async () => {
    const propose = vi.fn<(change: DeviceNetworkChange) => Promise<NetworkOperation>>(
      () => Promise.resolve(previewed),
    );
    renderPanel({ networks: [], propose });
    const dialog = openEditor();
    fireEvent.click(within(dialog).getByRole('button', { name: 'Preview' }));
    expect(within(dialog).getAllByRole('alert')[0]).toHaveTextContent(/name/i);
    expect(propose).not.toHaveBeenCalled();

    fireEvent.change(within(dialog).getByLabelText('Name'), {
      target: { value: 'Line 3' },
    });
    fireEvent.change(within(dialog).getByLabelText('Device address'), {
      target: { value: '192.168.0.130' },
    });
    fireEvent.change(
      within(dialog).getByLabelText(/TCP ports/, { selector: 'input' }),
      {
        target: { value: '502' },
      },
    );
    fireEvent.click(within(dialog).getByRole('button', { name: 'Preview' }));
    await waitFor(() => expect(propose).toHaveBeenCalledTimes(1));
    const change = propose.mock.calls[0][0];
    expect(change.action).toBe('create');
    if (change.action !== 'create') throw new Error('unreachable');
    expect(change.device_network.target).toBe('192.168.0.130');
    expect(change.device_network.services.tcp_ports).toEqual([502]);
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
  });

  // REQ-UX-8859ce12: the groups, the collapsed Advanced, validation beside the
  // field, an editable port, and the plain-language review.
  it('groups the editor by task, validates beside the field and reviews in plain language', async () => {
    const propose = vi.fn<(change: DeviceNetworkChange) => Promise<NetworkOperation>>(
      () => Promise.resolve(previewed),
    );
    renderPanel({ networks: [], propose });
    const dialog = openEditor();

    const groups = within(dialog)
      .getAllByRole('group')
      .map(
        (group) =>
          group.getAttribute('aria-label') ?? group.textContent?.split('\n')[0],
      );
    expect(groups.slice(0, 5)).toEqual([
      'Identity',
      'Device side',
      'Access path',
      'Addressing',
      'Services',
    ]);
    const advanced = within(dialog).getByText('Advanced').closest('details');
    expect(advanced).not.toBeNull();
    expect(advanced).not.toHaveAttribute('open');
    expect(within(dialog).getByRole('heading', { name: 'Review' })).toBeVisible();

    // Persistent guidance is visible with every help popover closed.
    expect(within(dialog).queryByRole('dialog')).toBeNull();
    expect(within(dialog).getByText(/same subnet as the device/i)).toBeVisible();
    expect(within(dialog).getByText(/502 is an example/i)).toBeVisible();

    fireEvent.change(within(dialog).getByLabelText('Name'), {
      target: { value: 'Line 3' },
    });
    fireEvent.change(
      within(dialog).getByLabelText(/Appliance address/, { selector: 'input' }),
      {
        target: { value: '192.168.0.1' },
      },
    );
    fireEvent.change(within(dialog).getByLabelText('Device address'), {
      target: { value: '192.168.0.130' },
    });
    fireEvent.change(
      within(dialog).getByLabelText(/TCP ports/, { selector: 'input' }),
      {
        target: { value: '70000' },
      },
    );
    fireEvent.click(within(dialog).getByRole('button', { name: 'Preview' }));
    expect(propose).not.toHaveBeenCalled();
    expect(
      within(dialog).getByLabelText(/Appliance address/, { selector: 'input' }),
    ).toHaveAccessibleDescription(/prefix length/i);
    expect(
      within(dialog).getByLabelText(/TCP ports/, { selector: 'input' }),
    ).toHaveAccessibleDescription(/between 1 and 65535/i);

    fireEvent.change(
      within(dialog).getByLabelText(/Appliance address/, { selector: 'input' }),
      {
        target: { value: '192.168.0.1/24' },
      },
    );
    fireEvent.change(
      within(dialog).getByLabelText(/TCP ports/, { selector: 'input' }),
      {
        target: { value: '504' },
      },
    );
    fireEvent.change(
      within(dialog).getByLabelText(/Authorized local sources/, { selector: 'input' }),
      {
        target: { value: '192.168.10.0/24' },
      },
    );
    const review = within(dialog).getByRole('region', { name: 'Review' });
    expect(review).toHaveTextContent(/192\.168\.0\.1\/24/);
    expect(review).toHaveTextContent(/192\.168\.0\.130/);
    expect(review).toHaveTextContent(/192\.168\.10\.0\/24/);
    expect(review).toHaveTextContent(/source NAT/i);
    expect(review).toHaveTextContent(/WireGuard is not involved/i);
    expect(review).toHaveTextContent(/management/i);

    fireEvent.click(within(dialog).getByRole('button', { name: 'Preview' }));
    await waitFor(() => expect(propose).toHaveBeenCalledTimes(1));
    const change = propose.mock.calls[0][0];
    if (change.action !== 'create') throw new Error('unreachable');
    expect(change.device_network.services.tcp_ports).toEqual([504]);
  });

  // REQ-UX-1860069d: the how-to-configure help, with its example and its
  // warnings, opened on request and closed with focus returned.
  it('explains how to configure a device network on request', () => {
    renderPanel({ networks: [] });
    const dialog = openEditor();
    const howTo = within(dialog).getByRole('button', { name: 'How to configure' });
    fireEvent.click(howTo);
    const help = screen.getByRole('dialog', {
      name: /how to configure a device network/i,
    });
    for (const section of [
      'Goal',
      'What you need before you start',
      'Addressing example',
      'What NILO will change',
      'What must already be configured elsewhere',
      'Verify and recover',
    ]) {
      expect(within(help).getByRole('heading', { name: section })).toBeVisible();
    }
    expect(help).toHaveTextContent(/192\.168\.0\.1\/24/);
    expect(help).toHaveTextContent(/192\.168\.0\.130/);
    expect(help).toHaveTextContent(/192\.168\.1\.13/);
    expect(help).toHaveTextContent(/route/i);
    expect(help).toHaveTextContent(/does not prove/i);
    expect(help).toHaveTextContent(/restored/i);
    fireEvent.keyDown(document, { key: 'Escape' });
    expect(screen.queryByRole('dialog', { name: /how to configure/i })).toBeNull();
    expect(screen.getByRole('dialog', { name: 'New device network' })).toBeVisible();
  });

  // REQ-UX-2b0b6d59 on a real field: help beside the label, by click and by key.
  it('offers settings help beside the label that opens by click and closes with Escape', () => {
    renderPanel({ networks: [] });
    const dialog = openEditor();
    const help = within(dialog).getByRole('button', {
      name: /Help for Appliance address/,
    });
    fireEvent.click(help);
    const popover = screen.getByRole('dialog', { name: /Help for Appliance address/ });
    expect(popover).toHaveTextContent(/does not replace/i);
    fireEvent.keyDown(document, { key: 'Escape' });
    expect(screen.queryByRole('dialog', { name: /Help for/ })).toBeNull();
    expect(document.activeElement).toBe(help);
    // The editor underneath is still open: Escape closed only the top layer.
    expect(screen.getByRole('dialog', { name: 'New device network' })).toBeVisible();
  });

  it('shows the separate facts of a connection check and the access steps', async () => {
    renderPanel({
      check: () =>
        Promise.resolve({
          deviceNetworkId: 'dn-1',
          observedAt: new Date(),
          facts: [
            { name: 'forwarding', result: 'failed', detail: 'not installed' },
            {
              name: 'target',
              result: 'unknown',
              detail: 'the probe did not complete',
            },
          ],
        }),
      instructions: () =>
        Promise.resolve({
          deviceNetworkId: 'dn-1',
          targetAddress: '192.168.0.130',
          steps: [
            {
              machine: 'client',
              text: 'Add a route to 192.168.0.0/24 via 192.168.10.24.',
            },
          ],
          unmetPrerequisites: ['the selected WireGuard profile is not connected'],
        }),
    });
    chooseAction('Line 3', 'Check connection Line 3');
    fireEvent.click(screen.getByRole('button', { name: 'Run check' }));
    expect(await screen.findByText(/Failed: not installed/)).toBeVisible();
    expect(screen.getByText(/Unknown: the probe did not complete/)).toBeVisible();
    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));

    chooseAction('Line 3', 'Access instructions Line 3');
    expect(await screen.findByText(/Add a route to 192.168.0.0\/24/)).toBeVisible();
    expect(screen.getByText(/Not usable yet:/)).toHaveTextContent(/not connected/);
  });
});
