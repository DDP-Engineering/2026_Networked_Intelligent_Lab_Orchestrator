import { fireEvent, screen, waitFor, within } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import type { NetworkInterface, NetworkOperation } from '../../api/contract';
import type { DeviceNetwork, DeviceNetworkChange } from '../../api/devicenetworks';
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

describe('DeviceNetworksPanel', () => {
  // REQ-UX-1c5eab92 and REQ-FUNC-9b17ce40: intent and observation are two
  // columns, the actions go through a preview, and the activity link is
  // narrowed to the network.
  it('separates the operator intent from the observed state and links to activity', () => {
    const openActivity = vi.fn();
    renderInShell(
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
        openActivity={openActivity}
      />,
      { theme: 'light', language: 'en' },
    );
    const row = screen.getByRole('row', { name: /Line 3/ });
    expect(within(row).getByText('Waiting')).toBeVisible();
    expect(within(row).getByText('Enabled, not applied')).toBeVisible();
    expect(within(row).getAllByText('192.168.0.130').length).toBeGreaterThan(0);
    fireEvent.click(within(row).getByRole('button', { name: 'View activity Line 3' }));
    expect(openActivity).toHaveBeenCalledWith('dn-1');
  });

  it('previews a new device network from the editor and refuses an incomplete one', async () => {
    const propose = vi.fn<(change: DeviceNetworkChange) => Promise<NetworkOperation>>(
      () => Promise.resolve(previewed),
    );
    renderInShell(
      <DeviceNetworksPanel
        networks={[]}
        interfaces={interfaces}
        profiles={[]}
        canManage
        busy={false}
        unreadable={false}
        propose={propose}
        check={() => Promise.reject(new Error('unused'))}
        instructions={() => Promise.reject(new Error('unused'))}
      />,
      { theme: 'light', language: 'en' },
    );
    fireEvent.click(screen.getByRole('button', { name: 'Add device network' }));
    const dialog = screen.getByRole('dialog', { name: 'New device network' });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Preview' }));
    expect(within(dialog).getByRole('alert')).toHaveTextContent(/name/i);
    expect(propose).not.toHaveBeenCalled();

    fireEvent.change(within(dialog).getByLabelText('Name'), {
      target: { value: 'Line 3' },
    });
    fireEvent.change(within(dialog).getByLabelText('Device address'), {
      target: { value: '192.168.0.130' },
    });
    fireEvent.change(within(dialog).getByLabelText(/TCP ports/), {
      target: { value: '502' },
    });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Preview' }));
    await waitFor(() => expect(propose).toHaveBeenCalledTimes(1));
    const change = propose.mock.calls[0][0];
    expect(change.action).toBe('create');
    if (change.action !== 'create') throw new Error('unreachable');
    expect(change.device_network.target).toBe('192.168.0.130');
    expect(change.device_network.services.tcp_ports).toEqual([502]);
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
  });

  it('shows the separate facts of a connection check and the access steps', async () => {
    renderInShell(
      <DeviceNetworksPanel
        networks={[line]}
        interfaces={interfaces}
        profiles={[]}
        canManage
        busy={false}
        unreadable={false}
        propose={() => Promise.resolve(previewed)}
        check={() =>
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
          })
        }
        instructions={() =>
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
          })
        }
      />,
      { theme: 'light', language: 'en' },
    );
    fireEvent.click(screen.getByRole('button', { name: 'Check connection Line 3' }));
    fireEvent.click(screen.getByRole('button', { name: 'Run check' }));
    expect(await screen.findByText(/Failed: not installed/)).toBeVisible();
    expect(screen.getByText(/Unknown: the probe did not complete/)).toBeVisible();
    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));

    fireEvent.click(screen.getByRole('button', { name: 'Access instructions Line 3' }));
    expect(await screen.findByText(/Add a route to 192.168.0.0\/24/)).toBeVisible();
    expect(screen.getByText(/Not usable yet:/)).toHaveTextContent(/not connected/);
  });
});
