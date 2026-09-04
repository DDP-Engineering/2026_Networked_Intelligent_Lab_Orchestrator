import { fireEvent, screen, waitFor, within } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import { BenchCoreError } from '../../api/client';
import type {
  NetworkOperation,
  NetworkStatistics,
  NetworkStatus,
  StatisticsPeriod,
} from '../../api/contract';
import { renderInShell } from '../../test/render';
import { NetworkPage } from './NetworkPage';

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
      settings: null,
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
  canManageInterfaces?: boolean;
}

function renderPage(overrides: PageOverrides = {}) {
  return renderInShell(
    <NetworkPage
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
      canManageInterfaces={overrides.canManageInterfaces ?? false}
      benchCoreReachable
    />,
    { theme: 'light', language: 'en' },
  );
}

describe('NetworkPage', () => {
  it('lists addresses and gateways without repeating the shell simulation indicator', async () => {
    renderPage();

    expect((await screen.findAllByText('enp1s0')).length).toBeGreaterThan(0);
    expect(screen.getByText('192.168.10.24/24')).toBeInTheDocument();
    expect(screen.getByText('fd00:10::24/64')).toBeInTheDocument();
    expect(screen.getAllByText('lo').length).toBeGreaterThan(0);

    expect(screen.getByRole('columnheader', { name: 'Gateway' })).toBeVisible();
    expect(screen.getByText('192.168.10.1')).toBeVisible();
    expect(screen.getByText('fd00:10::1')).toBeVisible();
    expect(screen.queryByText(/simulated environment/i)).not.toBeInTheDocument();
  });

  it('makes the reason for a read-only device available on its name', async () => {
    renderPage();
    await screen.findAllByText('enp1s0');
    expect(
      screen.getByTitle(/the loopback device is not an appliance connection/i),
    ).toBeInTheDocument();
  });

  it('presents the three areas the plan asks for', async () => {
    renderPage();
    for (const heading of [/interfaces/i, /wireguard/i, /statistics/i]) {
      expect(await screen.findByRole('heading', { name: heading })).toBeInTheDocument();
    }
  });

  it('asks for a different window when a period is chosen', async () => {
    const getNetworkStatistics = vi.fn(() => Promise.resolve(statistics));
    renderPage({ getNetworkStatistics });

    await waitFor(() => expect(getNetworkStatistics).toHaveBeenCalled());
    const before = getNetworkStatistics.mock.calls.length;

    screen.getByRole('button', { name: '24 hours' }).click();
    await waitFor(() =>
      expect(getNetworkStatistics.mock.calls.length).toBeGreaterThan(before),
    );
  });

  it('marks a series that stopped refreshing rather than letting it look current', async () => {
    renderPage({
      getNetworkStatistics: () =>
        Promise.resolve({
          ...statistics,
          stale: true,
          points: statistics.points.map((point) => ({ ...point, stale: true })),
        }),
    });
    expect(await screen.findByText(/no longer being refreshed/i)).toBeInTheDocument();
  });

  it('states that nothing was measured rather than drawing an empty window as zero', async () => {
    renderPage({
      getNetworkStatistics: () =>
        Promise.resolve({ ...statistics, stale: true, points: [] }),
    });
    expect(await screen.findByText(/no traffic was measured/i)).toBeInTheDocument();
  });

  // Bench Core authorizes every request on its own, so hiding a control is
  // presentation. It still must not be offered to somebody who cannot use it.
  it('offers no interface controls to an identity that cannot manage them', async () => {
    renderPage({ canManageInterfaces: false });
    await screen.findAllByText('enp1s0');
    expect(screen.queryByRole('button', { name: /disable/i })).toBeNull();
  });

  it('offers a control only for a device that is actually controllable', async () => {
    renderPage({ canManageInterfaces: true });
    await screen.findAllByText('enp1s0');

    // The wired device can be disabled; loopback carries no control at all.
    expect(screen.getAllByRole('button', { name: /disable enp1s0/i })).toHaveLength(1);
    expect(screen.queryByRole('button', { name: /disable lo/i })).toBeNull();
  });

  it('shows the exact change before it asks for agreement, and changes nothing until then', async () => {
    const previewInterfaceChange = vi.fn(() => Promise.resolve(previewed));
    const acknowledgeOperation = vi.fn(() => Promise.resolve(awaitingConfirmation));
    renderPage({
      canManageInterfaces: true,
      previewInterfaceChange,
      acknowledgeOperation,
    });
    await screen.findAllByText('enp1s0');

    fireEvent.click(screen.getByRole('button', { name: /disable enp1s0/i }));

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
    renderPage({ canManageInterfaces: true, acknowledgeOperation });
    await screen.findAllByText('enp1s0');

    fireEvent.click(screen.getByRole('button', { name: /disable enp1s0/i }));
    const dialog = await screen.findByRole('dialog');
    fireEvent.click(within(dialog).getByRole('button', { name: /cancel/i }));

    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(acknowledgeOperation).not.toHaveBeenCalled();
  });

  it('counts the confirmation window down and offers both ways out', async () => {
    renderPage({ canManageInterfaces: true });
    await screen.findAllByText('enp1s0');

    fireEvent.click(screen.getByRole('button', { name: /disable enp1s0/i }));
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
    renderPage({ canManageInterfaces: true, confirmOperation });
    await screen.findAllByText('enp1s0');

    fireEvent.click(screen.getByRole('button', { name: /disable enp1s0/i }));
    fireEvent.click(
      within(await screen.findByRole('dialog')).getByRole('button', {
        name: /^apply$/i,
      }),
    );
    fireEvent.click(await screen.findByRole('button', { name: /keep change/i }));

    await waitFor(() => expect(confirmOperation).toHaveBeenCalledWith('operation-1'));
    await waitFor(() => expect(screen.queryByRole('timer')).toBeNull());
    expect(screen.getByRole('button', { name: /disable enp1s0/i })).toBeEnabled();
  });

  it('restores the previous state when the administrator asks for it', async () => {
    const rollbackOperation = vi.fn(() =>
      Promise.resolve({ ...awaitingConfirmation, state: 'rolled_back' as const }),
    );
    renderPage({ canManageInterfaces: true, rollbackOperation });
    await screen.findAllByText('enp1s0');

    fireEvent.click(screen.getByRole('button', { name: /disable enp1s0/i }));
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
    renderPage({
      canManageInterfaces: true,
      rollbackOperation: () =>
        Promise.resolve({
          ...awaitingConfirmation,
          state: 'rollback_failed',
          failure: 'the previous connection was not observed after restoring',
        }),
    });
    await screen.findAllByText('enp1s0');

    fireEvent.click(screen.getByRole('button', { name: /disable enp1s0/i }));
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
    renderPage({
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
    });

    const chart = await screen.findByRole('img', { name: /traffic/i });
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
    renderPage();
    const chart = await screen.findByRole('img', { name: /traffic/i });
    expect(chart.getAttribute('aria-label')).toMatch(/1 measured minute/i);
  });

  it('draws no chart when nothing was measured, and says so instead', async () => {
    renderPage({
      getNetworkStatistics: () =>
        Promise.resolve({ ...statistics, stale: true, points: [] }),
    });
    expect(await screen.findByText(/no traffic was measured/i)).toBeInTheDocument();
    expect(screen.queryByRole('img', { name: /traffic/i })).toBeNull();
  });

  it('charts the interface the operator chose', async () => {
    // Typed parameters so the assertion below can read which interface was
    // asked for, rather than indexing an untyped empty tuple.
    const getNetworkStatistics = vi.fn((interfaceId: string) =>
      Promise.resolve({ ...statistics, interfaceId }),
    );
    renderPage({ getNetworkStatistics });
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
    renderPage();
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
