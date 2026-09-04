import { fireEvent, screen, within } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import type { NetworkStatistics, NetworkStatisticsPoint } from '../../api/contract';
import { renderInShell } from '../../test/render';
import { TrafficChart } from './TrafficChart';

function point(
  minute: number,
  rxBytes: number,
  txBytes: number,
): NetworkStatisticsPoint {
  return {
    minuteStart: new Date(`2026-09-04T10:0${minute}:00Z`),
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

    const chart = screen.getByRole('group', { name: /traffic for enp1s0/i });
    const samples = within(chart).getAllByRole('button');
    expect(samples).toHaveLength(2);
    expect(samples[0]).toHaveAccessibleName(/RX 1\.0 kB.*TX 512 B.*total 1\.5 kB/i);
    expect(samples[1]).toHaveAccessibleName(/RX 4\.0 kB.*TX 2\.0 kB.*total 6\.0 kB/i);
    expect(samples[0]).toHaveAttribute('tabindex', '-1');
    expect(samples[1]).toHaveAttribute('tabindex', '0');

    const bars = chart.querySelectorAll('[data-minute]');
    expect(
      Number(bars[1].getAttribute('x')) - Number(bars[0].getAttribute('x')),
    ).toBeCloseTo(2 * (Number(bars[0].getAttribute('width')) / 0.7), 8);

    fireEvent.focus(samples[0]);
    expect(screen.getByRole('status', { name: 'Selected minute' })).toHaveTextContent(
      /RX 1\.0 kB.*TX 512 B.*Total 1\.5 kB/i,
    );
    fireEvent.pointerDown(samples[1]);
    expect(screen.getByRole('status', { name: 'Selected minute' })).toHaveTextContent(
      /RX 4\.0 kB.*TX 2\.0 kB.*Total 6\.0 kB/i,
    );
    fireEvent.keyDown(samples[1], { key: 'ArrowLeft' });
    expect(samples[0]).toHaveFocus();
    expect(screen.getByRole('status', { name: 'Selected minute' })).toHaveTextContent(
      /RX 1\.0 kB.*TX 512 B.*Total 1\.5 kB/i,
    );
  });
});
