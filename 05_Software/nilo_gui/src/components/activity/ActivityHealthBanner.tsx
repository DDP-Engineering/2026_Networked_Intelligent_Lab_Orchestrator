import type { LoggingHealth, RetainedWindow } from '../../api/activity';
import { useTranslate } from '../../i18n/languages';
import { CATEGORY_KEYS } from './vocabulary';

export interface ActivityHealthBannerProps {
  loggingHealth: LoggingHealth;
  retainedWindow: Record<string, RetainedWindow>;
}

/**
 * What the listing can and cannot be trusted to contain, in one compact strip.
 *
 * Two facts belong above every audit listing and nowhere else: whether the
 * appliance can still write its history, and where retention has already taken
 * entries out of it. The first is a refusal condition and is stated as one, in
 * its own notice; the second is a quiet line of counts, because a gap is normal
 * on a bounded appliance and must not read like a fault.
 */
export function ActivityHealthBanner({
  loggingHealth,
  retainedWindow,
}: ActivityHealthBannerProps) {
  const t = useTranslate();
  const gaps = Object.entries(retainedWindow).filter(
    ([, window]) => window.truncated && window.purgedBefore !== null,
  );

  return (
    <div className="activity-banner">
      {loggingHealth.auditWritable ? (
        <p className="activity-banner-state" role="status">
          <span className="state-dot state-ok" aria-hidden="true" />
          {t('activity.health.ok')}
        </p>
      ) : (
        <p className="notice notice-danger" role="status">
          {t('activity.health.degraded', { failure: loggingHealth.lastFailure })}
        </p>
      )}
      <p className="activity-banner-retention">
        {Object.entries(retainedWindow).map(([category, window]) => (
          <span key={category} className="activity-banner-count">
            {t('activity.retained', {
              count: window.count,
              category: t(CATEGORY_KEYS[category] ?? 'activity.category.all'),
            })}
          </span>
        ))}
      </p>
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
