import type { ReactNode } from 'react';

import type { ActivityEvent } from '../../api/activity';
import type { MessageKey } from '../../i18n/en';
import { useTranslate } from '../../i18n/languages';
import { Modal } from '../Modal';
import { ActivityOutcomeBadge } from './ActivityOutcomeBadge';
import {
  ACTOR_KEYS,
  CATEGORY_KEYS,
  SEVERITY_KEYS,
  TIME_SOURCE_KEYS,
} from './vocabulary';

/** How deep the structured reader goes before it prints the value as written. */
const MAX_DEPTH = 4;

export interface ActivityEventDetailsModalProps {
  event: ActivityEvent;
  onDismiss: () => void;
  /**
   * Narrow the listing to every stage of this event's operation. Omitted where
   * the surrounding screen has no listing to narrow.
   */
  onShowOperation?: (operationId: string) => void;
}

function plain(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === 'object' && !Array.isArray(value);
}

/**
 * Safe structured details, rendered as labelled values rather than as the JSON
 * the appliance happened to encode them in.
 *
 * Keys are printed as the appliance wrote them: they are the record's own field
 * names, and translating them would claim a meaning this interface cannot know.
 * Nothing is added — a value that was not recorded is absent here too.
 */
function StructuredValue({
  value,
  depth = 0,
}: {
  value: unknown;
  depth?: number;
}): ReactNode {
  const t = useTranslate();

  if (value === null || value === undefined) {
    return <span className="cell-unknown">{t('activity.value.none')}</span>;
  }
  if (typeof value === 'boolean') {
    return <span>{t(value ? 'activity.value.yes' : 'activity.value.no')}</span>;
  }
  if (typeof value === 'number' || typeof value === 'string') {
    const text = String(value);
    return text === '' ? (
      <span className="cell-unknown">{t('activity.value.none')}</span>
    ) : (
      <span className="activity-value">{text}</span>
    );
  }
  if (depth >= MAX_DEPTH) {
    return <pre className="activity-details">{JSON.stringify(value, null, 2)}</pre>;
  }
  if (Array.isArray(value)) {
    if (value.length === 0) {
      return <span className="cell-unknown">{t('activity.value.none')}</span>;
    }
    return (
      <ul className="activity-value-list">
        {value.map((item, index) => (
          <li key={index}>
            <StructuredValue value={item} depth={depth + 1} />
          </li>
        ))}
      </ul>
    );
  }
  if (plain(value)) {
    const entries = Object.entries(value);
    if (entries.length === 0) {
      return <span className="cell-unknown">{t('activity.value.none')}</span>;
    }
    return (
      <dl className="record-facts activity-structured">
        {entries.map(([key, entry]) => (
          <div key={key}>
            <dt>{key}</dt>
            <dd>
              <StructuredValue value={entry} depth={depth + 1} />
            </dd>
          </div>
        ))}
      </dl>
    );
  }
  return <pre className="activity-details">{JSON.stringify(value, null, 2)}</pre>;
}

/** One labelled fact of the record. */
function Fact({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div>
      <dt>{label}</dt>
      <dd>{children}</dd>
    </div>
  );
}

/**
 * A recorded value: the readable label first, the stable code beside it as
 * secondary technical detail, and an explicit statement when the appliance
 * recorded nothing at all.
 */
function Value({ text, code }: { text?: string; code?: string }) {
  const t = useTranslate();
  const readable = text !== undefined && text !== '' ? text : (code ?? '');
  if (readable === '') {
    return <span className="cell-unknown">{t('activity.value.none')}</span>;
  }
  return (
    <>
      <span className="activity-value">{readable}</span>
      {code !== undefined && code !== '' && code !== readable && (
        <small className="network-secondary">{code}</small>
      )}
    </>
  );
}

/**
 * One activity record in full: the envelope as labelled fields, and whatever
 * safe structured detail the appliance attached to it.
 *
 * A record that carries a `before` and an `after` is shown as two sections,
 * because the question an audit answers is what changed. Anything else is shown
 * as one structured section. Neither path adds a field that was not recorded,
 * and neither reads a value the appliance chose not to export.
 */
export function ActivityEventDetailsModal({
  event,
  onDismiss,
  onShowOperation,
}: ActivityEventDetailsModalProps) {
  const t = useTranslate();
  const categoryKey: MessageKey | undefined = CATEGORY_KEYS[event.category];
  const actorKey: MessageKey | undefined = ACTOR_KEYS[event.actorKind];
  const severityKey: MessageKey | undefined = SEVERITY_KEYS[event.severity];
  const timeSourceKey: MessageKey | undefined = TIME_SOURCE_KEYS[event.timeSource];

  const details = event.details;
  const changed = 'before' in details || 'after' in details;
  const rest = Object.fromEntries(
    Object.entries(details).filter(([key]) => key !== 'before' && key !== 'after'),
  );

  return (
    <Modal
      title={t('activity.details.title', { sequence: event.sequence })}
      onDismiss={onDismiss}
    >
      <dl className="record-facts">
        <Fact label={t('activity.field.id')}>
          <Value code={event.id} />
        </Fact>
        <Fact label={t('activity.field.sequence')}>
          <Value code={String(event.sequence)} />
        </Fact>
        <Fact label={t('activity.field.recorded')}>
          <Value text={event.createdAt.toLocaleString()} />
        </Fact>
        <Fact label={t('activity.col.category')}>
          <Value
            text={categoryKey === undefined ? undefined : t(categoryKey)}
            code={event.category}
          />
        </Fact>
        <Fact label={t('activity.col.component')}>
          <Value code={event.component} />
        </Fact>
        <Fact label={t('activity.col.action')}>
          <Value code={event.action} />
        </Fact>
        <Fact label={t('activity.col.outcome')}>
          <ActivityOutcomeBadge outcome={event.outcome} />
          <small className="network-secondary">{event.outcome}</small>
        </Fact>
        <Fact label={t('activity.field.severity')}>
          <Value
            text={severityKey === undefined ? undefined : t(severityKey)}
            code={event.severity}
          />
        </Fact>
        <Fact label={t('activity.field.actorKind')}>
          <Value
            text={actorKey === undefined ? undefined : t(actorKey)}
            code={event.actorKind}
          />
        </Fact>
        <Fact label={t('activity.field.actorId')}>
          <Value code={event.actorId} />
        </Fact>
        <Fact label={t('activity.field.subjectType')}>
          <Value code={event.subjectType} />
        </Fact>
        <Fact label={t('activity.field.subjectId')}>
          <Value code={event.subjectId} />
        </Fact>
        <Fact label={t('activity.field.targetLabel')}>
          <Value text={event.targetLabel} />
        </Fact>
        <Fact label={t('activity.filter.operation')}>
          <Value code={event.operationId} />
        </Fact>
        <Fact label={t('activity.field.appliance')}>
          <Value code={event.applianceId} />
        </Fact>
        <Fact label={t('activity.field.boot')}>
          <Value code={event.bootId} />
        </Fact>
        <Fact label={t('activity.field.timeSource')}>
          <Value
            text={timeSourceKey === undefined ? undefined : t(timeSourceKey)}
            code={event.timeSource}
          />
        </Fact>
      </dl>

      {event.operationId !== '' && onShowOperation !== undefined && (
        <div className="form-actions">
          <button
            type="button"
            className="button"
            onClick={() => onShowOperation(event.operationId)}
          >
            {t('activity.operation', { id: event.operationId })}
          </button>
        </div>
      )}

      {changed && (
        <>
          <section className="activity-detail-section">
            <h4>{t('activity.details.before')}</h4>
            <StructuredValue value={details.before} />
          </section>
          <section className="activity-detail-section">
            <h4>{t('activity.details.after')}</h4>
            <StructuredValue value={details.after} />
          </section>
        </>
      )}

      {Object.keys(rest).length > 0 && (
        <section className="activity-detail-section">
          <h4>{t('activity.details.recorded')}</h4>
          <StructuredValue value={rest} />
        </section>
      )}

      {!changed && Object.keys(rest).length === 0 && (
        <p className="activity-detail-section">{t('activity.details.none')}</p>
      )}
    </Modal>
  );
}
