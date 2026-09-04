import type { NetworkStatistics } from '../../api/contract';
import { useTranslate } from '../../i18n/languages';

export interface TrafficChartProps {
  statistics: NetworkStatistics;
}

const WIDTH = 640;
const HEIGHT = 120;
/** The narrowest a bar may be drawn, so a 24-hour series stays visible. */
const MIN_BAR = 1;

/**
 * One interface's traffic over the chosen window.
 *
 * Every mark is placed by the time it was measured, not by its index in the
 * list. That is the whole point: a minute nobody measured leaves a hole, and the
 * mark after it does not slide left to fill the space. A chart that closed the
 * gap would draw an unbroken series over a period the appliance was not
 * watching, which is the same lie as filling the minute with a zero.
 *
 * Each mark stacks received on transmitted, so the total height is the traffic
 * that crossed the interface in that minute and the split stays readable.
 */
export function TrafficChart({ statistics }: TrafficChartProps) {
  const t = useTranslate();

  const span = statistics.to.getTime() - statistics.from.getTime();
  const minutes = Math.max(1, Math.round(span / 60_000));
  const step = WIDTH / minutes;
  const barWidth = Math.max(MIN_BAR, step * 0.7);

  const peak = statistics.points.reduce(
    (highest, point) => Math.max(highest, point.rxBytes + point.txBytes),
    0,
  );
  // A window whose every measured minute carried nothing is still a measurement,
  // so it draws flat rather than dividing by zero.
  const scale = peak > 0 ? (HEIGHT - 2) / peak : 0;

  const label = t('network.chart.label')
    .replace('{count}', String(statistics.points.length))
    .replace('{interface}', statistics.interfaceId)
    .replace('{period}', statistics.period);

  return (
    <svg
      role="img"
      aria-label={label}
      className={`traffic-chart${statistics.stale ? ' traffic-chart-stale' : ''}`}
      viewBox={`0 0 ${WIDTH} ${HEIGHT}`}
      preserveAspectRatio="none"
    >
      {statistics.points.map((point) => {
        const offset =
          (point.minuteStart.getTime() - statistics.from.getTime()) / 60_000;
        const x = offset * step;
        const received = point.rxBytes * scale;
        const transmitted = point.txBytes * scale;
        return (
          <g key={point.minuteStart.toISOString()}>
            <rect
              data-minute={point.minuteStart.toISOString()}
              className="traffic-rx"
              x={x}
              y={HEIGHT - received}
              width={barWidth}
              height={received}
            />
            <rect
              className="traffic-tx"
              x={x}
              y={HEIGHT - received - transmitted}
              width={barWidth}
              height={transmitted}
            />
          </g>
        );
      })}
    </svg>
  );
}
