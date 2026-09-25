import { fireEvent, screen, within } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import type { NetworkStatistics, NetworkStatisticsPoint } from '../../api/contract';
import { renderInShell } from '../../test/render';
import { TrafficChart } from './TrafficChart';

function point(
  minute: number,
  rxBytes: number,
  txBytes: number,
): NetworkStatisticsPoint {
  return {
    minuteStart: new Date(Date.parse('2026-09-04T10:00:00Z') + minute * 60_000),
    rxBytes,
    txBytes,
    rxErrors: 0,
    txErrors: 0,
    rxDropped: 0,
    txDropped: 0,
    samples: 12,
    stale: false,
  };
}

const statistics: NetworkStatistics = {
  interfaceId: 'enp1s0',
  period: '15m',
  from: new Date('2026-09-04T10:00:00Z'),
  to: new Date('2026-09-04T10:15:00Z'),
  measuredBy: 'simulation',
  stale: false,
  // 10:01 is deliberately absent: the second bar must stay at minute two.
  points: [point(0, 1024, 512), point(2, 4096, 2048)],
};

describe('TrafficChart', () => {
  // REQ-UX-cb91fedb: the graphic explains direction, axes, gaps and exact values.
  it('labels the traffic directions, time and byte scale and exposes each minute', () => {
    renderInShell(<TrafficChart statistics={statistics} />, { language: 'en' });

    expect(screen.getByText('RX · Received')).toBeVisible();
    expect(screen.getByText('TX · Sent')).toBeVisible();
    expect(screen.getByText('Traffic per measured minute')).toBeVisible();
    expect(screen.getByText('Time')).toBeVisible();
    expect(screen.getByText('6.0 kB')).toBeVisible();

    const chart = screen.getByRole('listbox', { name: /traffic for enp1s0/i });
    const samples = within(chart).getAllByRole('option');
    expect(samples).toHaveLength(2);
    expect(samples[0]).toHaveAccessibleName(/RX 1\.0 kB.*TX 512 B.*total 1\.5 kB/i);
    expect(samples[1]).toHaveAccessibleName(/RX 4\.0 kB.*TX 2\.0 kB.*total 6\.0 kB/i);
    expect(samples[0]).toHaveAttribute('aria-selected', 'false');
    expect(samples[1]).toHaveAttribute('aria-selected', 'true');

    const bars = chart.querySelectorAll('[data-minute]');
    expect(
      Number(bars[1].getAttribute('x')) - Number(bars[0].getAttribute('x')),
    ).toBeCloseTo(2 * (Number(bars[0].getAttribute('width')) / 0.7), 8);

    fireEvent.focus(chart);
    fireEvent.keyDown(chart, { key: 'ArrowLeft' });
    expect(screen.getByRole('status', { name: 'Selected minute' })).toHaveTextContent(
      /RX 1\.0 kB.*TX 512 B.*Total 1\.5 kB/i,
    );
  });

  it('selects adjacent dense samples by unambiguous pointer coordinates', () => {
    const densePoints = Array.from({ length: 60 }, (_, minute) =>
      point(minute, minute === 1 ? 0 : minute + 1, 0),
    );
    renderInShell(
      <TrafficChart
        statistics={{
          ...statistics,
          period: '1h',
          to: new Date('2026-09-04T11:00:00Z'),
          points: densePoints,
        }}
      />,
      { language: 'en' },
    );

    const chart = screen.getByRole('listbox', { name: /traffic for enp1s0/i });
    const samples = within(chart).getAllByRole('option');
    expect(samples).toHaveLength(60);
    vi.spyOn(chart, 'getBoundingClientRect').mockReturnValue({
      x: 0,
      y: 0,
      top: 0,
      left: 0,
      right: 720,
      bottom: 190,
      width: 720,
      height: 190,
      toJSON: () => ({}),
    });

    const plotStep = (720 - 64 - 12) / 60;
    fireEvent.pointerDown(chart, { clientX: 64 + plotStep * 1.35, clientY: 100 });
    expect(screen.getByRole('status', { name: 'Selected minute' })).toHaveTextContent(
      /RX 0 B.*TX 0 B.*Total 0 B/i,
    );
    fireEvent.pointerDown(chart, { clientX: 64 + plotStep * 2.35, clientY: 100 });
    expect(screen.getByRole('status', { name: 'Selected minute' })).toHaveTextContent(
      /RX 3 B.*TX 0 B.*Total 3 B/i,
    );

    fireEvent.pointerMove(chart, { clientX: 64 + plotStep * 1.35, clientY: 100 });
    expect(screen.getByRole('status', { name: 'Selected minute' })).toHaveTextContent(
      /RX 0 B.*TX 0 B.*Total 0 B/i,
    );
  });
});
