import type { LoggingHealth, RetainedWindow } from '../../api/activity';
import { useTranslate } from '../../i18n/languages';
import { CATEGORY_KEYS } from './vocabulary';

export interface ActivityHealthBannerProps {
  loggingHealth: LoggingHealth;
  retainedWindow: Record<string, RetainedWindow>;
}

/**
 * What qualifies the listing below it, and nothing else.
 *
 * Two conditions belong above an audit listing: that the appliance can no
 * longer write its history, and that retention has already taken entries out of
 * a window being read. The first is a refusal condition and is stated as one;
 * the second is a quiet line, because a gap is normal on a bounded appliance
 * and must not read like a fault. While neither holds, the strip says nothing:
 * a healthy appliance announcing its health on every load, and a count the
 * listing's own pagination already reports, are furniture on a screen whose
 * first line should be a record.
 */
export function ActivityHealthBanner({
  loggingHealth,
  retainedWindow,
}: ActivityHealthBannerProps) {
  const t = useTranslate();
  const gaps = Object.entries(retainedWindow).filter(
    ([, window]) => window.truncated && window.purgedBefore !== null,
  );

  if (loggingHealth.auditWritable && gaps.length === 0) {
    return null;
  }

  return (
    <div className="activity-banner">
      {!loggingHealth.auditWritable && (
        <p className="notice notice-danger" role="status">
          {t('activity.health.degraded', { failure: loggingHealth.lastFailure })}
        </p>
      )}
      {gaps.map(([category, window]) => (
        <p key={category} className="activity-banner-gap">
          <span className="activity-banner-category">
            {t(CATEGORY_KEYS[category] ?? 'activity.category.all')}
          </span>{' '}
          {t('activity.gap', {
            time: window.purgedBefore?.toLocaleString() ?? '',
            reason: window.purgeReason,
          })}
        </p>
      ))}
    </div>
  );
}
