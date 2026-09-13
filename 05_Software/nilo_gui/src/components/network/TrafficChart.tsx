import { useId, useState } from 'react';

import type { NetworkStatistics } from '../../api/contract';
import { useTranslate } from '../../i18n/languages';

export interface TrafficChartProps {
  statistics: NetworkStatistics;
}

const WIDTH = 720;
const HEIGHT = 190;
const PLOT_LEFT = 64;
const PLOT_RIGHT = 12;
const PLOT_TOP = 10;
const PLOT_BOTTOM = 34;
const MIN_BAR = 1;

/** Compact byte totals for chart scales and selected-minute values. */
function formatBytes(value: number): string {
  if (value < 1024) return `${Math.round(value)} B`;
  const units = ['kB', 'MB', 'GB', 'TB'];
  let amount = value / 1024;
  let unit = units[0];
  for (let index = 1; index < units.length && amount >= 1024; index += 1) {
    amount /= 1024;
    unit = units[index];
  }
  return `${amount.toFixed(amount >= 10 ? 0 : 1)} ${unit}`;
}

function formatTime(at: Date, period: NetworkStatistics['period']): string {
  return new Intl.DateTimeFormat(undefined, {
    ...(period === '24h' ? { day: '2-digit', month: 'short' } : {}),
    hour: '2-digit',
    minute: '2-digit',
  }).format(at);
}

/**
 * One interface's measured traffic over the selected window.
 *
 * Bars are stacked RX and TX byte totals for a measured minute. Their horizontal
 * position comes from time, so an unmeasured minute remains a visible gap. The
 * axes, legend and selected-minute readout make the same graphic usable by
 * sight, touch and keyboard without changing the statistics contract.
 */
export function TrafficChart({ statistics }: TrafficChartProps) {
  const t = useTranslate();
  const chartId = useId();
  const lastPoint = statistics.points.at(-1);
  const [selectedAt, setSelectedAt] = useState(
    lastPoint?.minuteStart.toISOString() ?? '',
  );

  const span = statistics.to.getTime() - statistics.from.getTime();
  const minutes = Math.max(1, Math.round(span / 60_000));
  const plotWidth = WIDTH - PLOT_LEFT - PLOT_RIGHT;
  const plotHeight = HEIGHT - PLOT_TOP - PLOT_BOTTOM;
  const step = plotWidth / minutes;
  const barWidth = Math.max(MIN_BAR, step * 0.7);
  const peak = statistics.points.reduce(
    (highest, point) => Math.max(highest, point.rxBytes + point.txBytes),
    0,
  );
  const scale = peak > 0 ? plotHeight / peak : 0;
  const selected =
    statistics.points.find((point) => point.minuteStart.toISOString() === selectedAt) ??
    lastPoint;
  const selectedIso = selected?.minuteStart.toISOString() ?? '';

  const chartLabel = t('network.chart.label')
    .replace('{count}', String(statistics.points.length))
    .replace('{interface}', statistics.interfaceId)
    .replace('{period}', statistics.period);
  const describePoint = (point: (typeof statistics.points)[number]) =>
    t('network.chart.sample')
      .replace('{time}', formatTime(point.minuteStart, statistics.period))
      .replace('{rx}', formatBytes(point.rxBytes))
      .replace('{tx}', formatBytes(point.txBytes))
      .replace('{total}', formatBytes(point.rxBytes + point.txBytes));
  const yTicks = peak === 0 ? [0] : [0, 0.5, 1];
  const middle = new Date((statistics.from.getTime() + statistics.to.getTime()) / 2);
  const xTicks = [statistics.from, middle, statistics.to];
  const selectIndex = (index: number) => {
    const point = statistics.points[index];
    if (point !== undefined) setSelectedAt(point.minuteStart.toISOString());
  };
  const selectAtClientX = (clientX: number, chart: SVGSVGElement) => {
    if (statistics.points.length === 0) return;
    const bounds = chart.getBoundingClientRect();
    if (bounds.width <= 0) return;
    const logicalX = ((clientX - bounds.left) / bounds.width) * WIDTH;
    const nearest = statistics.points.reduce((nearestIndex, point, index) => {
      const offset = (point.minuteStart.getTime() - statistics.from.getTime()) / 60_000;
      const center = PLOT_LEFT + offset * step + barWidth / 2;
      const nearestPoint = statistics.points[nearestIndex];
      const nearestOffset =
        (nearestPoint.minuteStart.getTime() - statistics.from.getTime()) / 60_000;
      const nearestCenter = PLOT_LEFT + nearestOffset * step + barWidth / 2;
      return Math.abs(center - logicalX) < Math.abs(nearestCenter - logicalX)
        ? index
        : nearestIndex;
    }, 0);
    selectIndex(nearest);
  };

  return (
    <figure
      className={`traffic-figure${statistics.stale ? ' traffic-chart-stale' : ''}`}
    >
      <figcaption className="traffic-chart-head">
        <span className="traffic-axis-title">{t('network.chart.volumeAxis')}</span>
        <span className="traffic-legend" aria-label={t('network.chart.legend')}>
          <span>
            <span className="traffic-swatch traffic-rx" aria-hidden="true" />
            {t('network.chart.rxLegend')}
          </span>
          <span>
            <span className="traffic-swatch traffic-tx" aria-hidden="true" />
            {t('network.chart.txLegend')}
          </span>
        </span>
      </figcaption>
      <svg
        role="listbox"
        tabIndex={0}
        aria-label={chartLabel}
        aria-activedescendant={
          selected === undefined
            ? undefined
            : `${chartId}-sample-${statistics.points.indexOf(selected)}`
        }
        className="traffic-chart"
        viewBox={`0 0 ${WIDTH} ${HEIGHT}`}
        onPointerDown={(event) => selectAtClientX(event.clientX, event.currentTarget)}
        onPointerMove={(event) => selectAtClientX(event.clientX, event.currentTarget)}
        onKeyDown={(event) => {
          if (statistics.points.length === 0) return;
          const selectedIndex = Math.max(
            0,
            statistics.points.findIndex(
              (point) => point.minuteStart.toISOString() === selectedIso,
            ),
          );
          let nextIndex: number;
          if (event.key === 'ArrowLeft' || event.key === 'ArrowUp') {
            nextIndex = Math.max(0, selectedIndex - 1);
          } else if (event.key === 'ArrowRight' || event.key === 'ArrowDown') {
            nextIndex = Math.min(statistics.points.length - 1, selectedIndex + 1);
          } else if (event.key === 'Home') {
            nextIndex = 0;
          } else if (event.key === 'End') {
            nextIndex = statistics.points.length - 1;
          } else {
            return;
          }
          event.preventDefault();
          selectIndex(nextIndex);
        }}
      >
        {yTicks.map((ratio) => {
          const y = PLOT_TOP + plotHeight * (1 - ratio);
          return (
            <g className="traffic-grid" key={ratio} aria-hidden="true">
              <line x1={PLOT_LEFT} x2={WIDTH - PLOT_RIGHT} y1={y} y2={y} />
              <text x={PLOT_LEFT - 8} y={y + 4} textAnchor="end">
                {formatBytes(peak * ratio)}
              </text>
            </g>
          );
        })}
        {statistics.points.map((point, index) => {
          const iso = point.minuteStart.toISOString();
          const offset =
            (point.minuteStart.getTime() - statistics.from.getTime()) / 60_000;
          const x = PLOT_LEFT + offset * step;
          const received = point.rxBytes * scale;
          const transmitted = point.txBytes * scale;
          const description = describePoint(point);
          return (
            <g
              id={`${chartId}-sample-${index}`}
              key={iso}
              role="option"
              aria-label={description}
              aria-selected={selectedIso === iso}
              className={`traffic-sample${selectedIso === iso ? ' traffic-sample-selected' : ''}`}
            >
              <title>{description}</title>
              <rect
                data-minute={iso}
                className="traffic-rx"
                x={x}
                y={PLOT_TOP + plotHeight - received}
                width={barWidth}
                height={received}
              />
              <rect
                className="traffic-tx"
                x={x}
                y={PLOT_TOP + plotHeight - received - transmitted}
                width={barWidth}
                height={transmitted}
              />
              {received + transmitted === 0 ? (
                <circle
                  className="traffic-zero"
                  cx={x + barWidth / 2}
                  cy={PLOT_TOP + plotHeight}
                  r={3}
                />
              ) : null}
            </g>
          );
        })}
        {xTicks.map((at, index) => (
          <text
            aria-hidden="true"
            className="traffic-time-tick"
            key={at.toISOString()}
            x={PLOT_LEFT + (plotWidth * index) / 2}
            y={HEIGHT - 15}
            textAnchor={index === 0 ? 'start' : index === 2 ? 'end' : 'middle'}
          >
            {formatTime(at, statistics.period)}
          </text>
        ))}
        <text
          className="traffic-time-title"
          x={PLOT_LEFT + plotWidth / 2}
          y={HEIGHT - 2}
          textAnchor="middle"
        >
          {t('network.chart.timeAxis')}
        </text>
      </svg>
      {selected === undefined ? null : (
        <p
          className="traffic-selected"
          role="status"
          aria-label={t('network.chart.selected')}
          aria-live="polite"
        >
          {describePoint(selected)}
        </p>
      )}
    </figure>
  );
}
