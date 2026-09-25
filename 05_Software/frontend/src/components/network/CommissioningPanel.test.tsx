import { fireEvent, screen, waitFor } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import { BenchCoreError } from '../../api/client';
import type { ManagementAccessStatus } from '../../api/management-access';
import { renderInShell } from '../../test/render';
import { CommissioningPanel } from './CommissioningPanel';

/** A first-boot Mini PC: two eligible adapters and one container interface. */
const provisioning: ManagementAccessStatus = {
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
    {
      name: 'wlp2s0',
      adapterId: '02:00:00:00:00:02',
      kind: 'wifi',
      state: 'connected',
      ipv4: ['192.168.20.15/24'],
      ipv6: [],
      enabled: true,
      current: false,
      usable: true,
      role: 'management',
      roleAvailable: true,
      roleDetail: '',
      conflictingResourceId: '',
      conflictingResourceName: '',
    },
    {
      name: 'veth9f2a41',
      adapterId: '02:00:00:00:00:03',
      kind: 'ethernet',
      state: 'connected',
      ipv4: ['10.88.0.2/16'],
      ipv6: [],
      enabled: false,
      current: false,
      usable: false,
      role: '',
      roleAvailable: false,
      roleDetail: 'not managed by NetworkManager',
      conflictingResourceId: '',
      conflictingResourceName: '',
    },
  ],
  configuredInterfaces: ['enp1s0', 'wlp2s0'],
  urls: [{ url: 'https://ddp-remote.local:8443', kind: 'local' }],
  tlsNames: ['ddp-remote.local'],
  tlsMissing: [],
  confirmBy: null,
  detail: '',
  outcome: { code: '', interface: '', resourceName: '' },
  currentInterface: 'enp1s0',
  provisioning: true,
};

function panel(overrides: Partial<Parameters<typeof CommissioningPanel>[0]> = {}) {
  return renderInShell(
    <CommissioningPanel
      status={provisioning}
      canManage
      busy={false}
      complete={vi.fn(async () => ({ ...provisioning, provisioning: false }))}
      {...overrides}
    />,
  );
}

describe('CommissioningPanel', () => {
  it('warns that every eligible adapter is a temporary management path', () => {
    panel();

    expect(screen.getByRole('alert')).toHaveTextContent(/temporary/i);
  });

  it('lists each eligible adapter with its identity, type and addresses', () => {
    panel();

    for (const shown of [
      'enp1s0',
      '02:00:00:00:00:01',
      '192.168.10.24/24',
      'wlp2s0',
      '02:00:00:00:00:02',
      '192.168.20.15/24',
    ]) {
      expect(
        screen.getByText(new RegExp(shown.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'))),
      ).toBeTruthy();
    }
  });

  it('marks the interface this browser session arrived on', () => {
    panel();

    const current = screen.getByTestId('commissioning-interface-enp1s0');
    expect(current.textContent).toMatch(/current/i);
    const other = screen.getByTestId('commissioning-interface-wlp2s0');
    expect(other.textContent).not.toMatch(/current/i);
  });

  it('explains that the local name works only on the local network', () => {
    panel();

    expect(screen.getByText(/ddp-remote\.local/)).toBeTruthy();
    expect(screen.getByTestId('commissioning-urls').textContent).toMatch(
      /local network/i,
    );
  });

  it('leaves an interface that cannot carry management access unselectable and says why', () => {
    panel();

    const ineligible = screen.queryByRole('checkbox', { name: /veth9f2a41/i });
    expect(ineligible).toBeNull();
  });

  it('refuses to complete with nothing selected', async () => {
    const complete = vi.fn();
    panel({ complete });

    fireEvent.click(screen.getByRole('checkbox', { name: /enp1s0/i }));
    fireEvent.click(screen.getByRole('checkbox', { name: /wlp2s0/i }));
    fireEvent.click(screen.getByRole('button', { name: /complete commissioning/i }));

    await waitFor(() => {
      expect(screen.getByTestId('commissioning-failure')).toBeTruthy();
    });
    expect(complete).not.toHaveBeenCalled();
  });

  it('asks for confirmation before giving up the current browser path', async () => {
    const complete = vi.fn(async () => ({ ...provisioning, provisioning: false }));
    panel({ complete });

    // Drop the adapter this session arrived on, keeping the other.
    fireEvent.click(screen.getByRole('checkbox', { name: /enp1s0/i }));
    fireEvent.click(screen.getByRole('button', { name: /complete commissioning/i }));

    await waitFor(() => {
      expect(screen.getByTestId('commissioning-current-warning')).toBeTruthy();
    });
    expect(complete).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole('button', { name: /give up .* and complete/i }));
    await waitFor(() => {
      expect(complete).toHaveBeenCalledWith(['wlp2s0'], true);
    });
  });

  it('completes onto the selected interfaces', async () => {
    const complete = vi.fn(async () => ({ ...provisioning, provisioning: false }));
    panel({ complete });

    fireEvent.click(screen.getByRole('checkbox', { name: /wlp2s0/i }));
    fireEvent.click(screen.getByRole('button', { name: /complete commissioning/i }));

    await waitFor(() => {
      expect(complete).toHaveBeenCalledWith(['enp1s0'], false);
    });
  });

  it('reports a refusal from the appliance without losing the selection', async () => {
    const complete = vi.fn(async () => {
      throw new BenchCoreError(
        'rejected',
        'Keep at least one adapter the appliance can be reached through.',
        'management_access_required',
      );
    });
    panel({ complete });

    fireEvent.click(screen.getByRole('button', { name: /complete commissioning/i }));

    await waitFor(() => {
      expect(screen.getByTestId('commissioning-failure')).toBeTruthy();
    });
    expect(screen.getByRole('checkbox', { name: /enp1s0/i })).toBeChecked();
  });

  it('offers no commissioning control to an identity that may not manage', () => {
    panel({ canManage: false });

    expect(
      screen.queryByRole('button', { name: /complete commissioning/i }),
    ).toBeNull();
  });

  it('shows no secret-shaped value anywhere', () => {
    const { container } = panel();

    for (const secret of ['password', 'private_key', 'preshared', 'psk']) {
      expect(container.textContent?.toLowerCase()).not.toContain(secret);
    }
  });
});
