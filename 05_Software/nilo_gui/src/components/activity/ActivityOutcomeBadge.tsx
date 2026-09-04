import { useTranslate } from '../../i18n/languages';
import { OUTCOME_KEYS, OUTCOME_MARKS, SEVERITY_KEYS, outcomeTone } from './vocabulary';

export interface ActivityOutcomeBadgeProps {
  /** The outcome exactly as the appliance recorded it. */
  outcome: string;
  /** The severity beside it; only shown when it is not the ordinary `info`. */
  severity?: string;
}

/**
 * One outcome, as a badge that can be read without seeing its colour.
 *
 * The word is always present and always first in the accessible name; the
 * tone and the mark beside it are reinforcement. An outcome this build does
 * not know is shown as the appliance wrote it, in the neutral tone, because
 * inventing a label for an unrecognised state would be describing a history
 * that was never recorded.
 */
export function ActivityOutcomeBadge({ outcome, severity }: ActivityOutcomeBadgeProps) {
  const t = useTranslate();
  const tone = outcomeTone(outcome);
  const key = OUTCOME_KEYS[outcome];
  const label =
    key !== undefined
      ? t(key)
      : outcome === ''
        ? t('activity.outcome.unknown')
        : outcome;
  const severityKey = severity === undefined ? undefined : SEVERITY_KEYS[severity];

  return (
    <>
      <span className={`status-badge status-badge-${tone}`}>
        <span className="status-badge-mark" aria-hidden="true">
          {OUTCOME_MARKS[tone]}
        </span>
        {label}
      </span>
      {severity !== undefined && severity !== '' && severity !== 'info' && (
        <small className="network-secondary">
          {severityKey === undefined ? severity : t(severityKey)}
        </small>
      )}
    </>
  );
}
