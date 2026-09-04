import { useState } from 'react';

import type {
  ActivityEvent,
  ActivitySort,
  ActivitySortOrder,
} from '../../api/activity';
import { useTranslate } from '../../i18n/languages';
import { ActivityEventDetailsModal } from './ActivityEventDetailsModal';
import { ActivityOutcomeBadge } from './ActivityOutcomeBadge';
import { ACTOR_KEYS } from './vocabulary';

export interface ActivityEventTableProps {
  events: readonly ActivityEvent[];
  /**
   * Narrow the listing to every stage of one operation. Omitted where the
   * surrounding screen has no listing to narrow, which also hides the control.
   */
  onShowOperation?: (operationId: string) => void;
  /** The listing's accessible name, for a screen that shows more than one. */
  label?: string;
  sort?: ActivitySort;
  order?: ActivitySortOrder;
  onSort?: (field: ActivitySort) => void;
}

/**
 * The activity listing: one markup that is a table on a wide display and a
 * stack of labelled records below 56rem, so a seven-inch panel never scrolls
 * the page sideways.
 *
 * Six columns, and no more: time, what happened, how it ended, who did it, what
 * it was done to, and the way into the rest. Everything else the appliance
 * recorded is one control away in the details dialog rather than in a column
 * nobody can read at 800 pixels. The action leads its cell and the function
 * follows it as secondary text, because an operator scanning a history is
 * looking for the change, not for the subsystem that made it.
 *
 * It owns only which row's details are open, so the Users workspace can render
 * the same listing without repeating the dialog.
 */
export function ActivityEventTable({
  events,
  onShowOperation,
  label,
  sort,
  order,
  onSort,
}: ActivityEventTableProps) {
  const t = useTranslate();
  const [opened, setOpened] = useState<string | null>(null);
  const open = events.find((event) => event.id === opened) ?? null;
  const heading = (field: ActivitySort, message: string) => {
    const active = sort === field;
    const ariaSort = active ? (order === 'desc' ? 'descending' : 'ascending') : 'none';
    return (
      <th scope="col" aria-sort={ariaSort}>
        <button
          type="button"
          className="activity-sort"
          aria-label={t('activity.sort.by', { column: message })}
          onClick={() => onSort?.(field)}
        >
          <span>{message}</span>
          <span className="activity-sort-mark" aria-hidden="true">
            {active ? (order === 'desc' ? '↓' : '↑') : '↕'}
          </span>
        </button>
      </th>
    );
  };

  return (
    <>
      <div className="table-scroll">
        <table
          className="data-table record-table activity-table"
          aria-label={label ?? t('activity.title')}
        >
          <thead>
            <tr>
              {onSort === undefined ? (
                <th scope="col">{t('activity.col.time')}</th>
              ) : (
                heading('time', t('activity.col.time'))
              )}
              {onSort === undefined ? (
                <th scope="col">{t('activity.col.activity')}</th>
              ) : (
                heading('activity', t('activity.col.activity'))
              )}
              {onSort === undefined ? (
                <th scope="col">{t('activity.col.outcome')}</th>
              ) : (
                heading('outcome', t('activity.col.outcome'))
              )}
              {onSort === undefined ? (
                <th scope="col">{t('activity.col.actor')}</th>
              ) : (
                heading('actor', t('activity.col.actor'))
              )}
              {onSort === undefined ? (
                <th scope="col">{t('activity.col.target')}</th>
              ) : (
                heading('target', t('activity.col.target'))
              )}
              <th scope="col">{t('activity.details')}</th>
            </tr>
          </thead>
          <tbody>
            {events.map((event) => (
              <tr key={event.id}>
                <td data-label={t('activity.col.time')}>
                  {event.createdAt.toLocaleString()}
                </td>
                <td data-label={t('activity.col.activity')}>
                  <span className="activity-action">{event.action}</span>
                  {event.component !== '' && (
                    <small className="network-secondary">{event.component}</small>
                  )}
                </td>
                <td data-label={t('activity.col.outcome')}>
                  <ActivityOutcomeBadge
                    outcome={event.outcome}
                    severity={event.severity}
                  />
                </td>
                <td data-label={t('activity.col.actor')}>
                  {ACTOR_KEYS[event.actorKind] !== undefined ? (
                    t(ACTOR_KEYS[event.actorKind])
                  ) : event.actorKind !== '' ? (
                    // A kind this build does not know is shown as written,
                    // never as the nearest one it does.
                    <span className="activity-value">{event.actorKind}</span>
                  ) : (
                    <span className="cell-unknown">{t('activity.value.none')}</span>
                  )}
                  {event.actorId !== '' && (
                    <small className="network-secondary">{event.actorId}</small>
                  )}
                </td>
                <td data-label={t('activity.col.target')}>
                  {event.targetLabel !== '' ? (
                    <>
                      <span className="activity-value">{event.targetLabel}</span>
                      {event.subjectId !== '' &&
                        event.subjectId !== event.targetLabel && (
                          <small className="network-secondary">
                            {event.subjectType === ''
                              ? event.subjectId
                              : `${event.subjectType} · ${event.subjectId}`}
                          </small>
                        )}
                    </>
                  ) : event.subjectId !== '' ? (
                    <>
                      <span className="activity-value">{event.subjectId}</span>
                      {event.subjectType !== '' && (
                        <small className="network-secondary">{event.subjectType}</small>
                      )}
                    </>
                  ) : (
                    <span className="cell-unknown">{t('activity.value.none')}</span>
                  )}
                </td>
                <td className="record-cell-actions">
                  <button
                    type="button"
                    className="button"
                    aria-label={t('activity.details.for', { sequence: event.sequence })}
                    aria-haspopup="dialog"
                    onClick={() => setOpened(event.id)}
                  >
                    {t('activity.details')}
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {open !== null && (
        <ActivityEventDetailsModal
          event={open}
          onDismiss={() => setOpened(null)}
          onShowOperation={
            onShowOperation === undefined
              ? undefined
              : (operationId) => {
                  setOpened(null);
                  onShowOperation(operationId);
                }
          }
        />
      )}
    </>
  );
}
