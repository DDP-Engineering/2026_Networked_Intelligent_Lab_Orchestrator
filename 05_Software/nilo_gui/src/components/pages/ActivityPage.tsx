import { Fragment, useCallback, useEffect, useState } from 'react';

import type {
  ActivityEvent,
  ActivityFilter,
  ActivityPage as ActivityPageBody,
  DiagnosticSession,
  DiagnosticSessions,
  DiagnosticSettings,
} from '../../api/activity';
import { ACTIVITY_CATEGORIES } from '../../api/activity';
import { BenchCoreError } from '../../api/client';
import type { MessageKey } from '../../i18n/en';
import { useTranslate } from '../../i18n/languages';

export interface ActivityPageProps {
  /** The filter the page opens with, for example from a network table. */
  initialFilter?: ActivityFilter;
  readActivity: (filter: ActivityFilter) => Promise<ActivityPageBody>;
  exportActivity?: (filter: ActivityFilter, format: 'jsonl' | 'csv') => Promise<string>;
  canExport: boolean;
  canManageDiagnostics: boolean;
  readSessions?: () => Promise<DiagnosticSessions>;
  startSession?: (
    scope: string,
    target: string,
    minutes: number,
  ) => Promise<DiagnosticSession>;
  stopSession?: (id: string) => Promise<DiagnosticSession>;
  readSettings?: () => Promise<DiagnosticSettings>;
  saveSettings?: (settings: DiagnosticSettings) => Promise<DiagnosticSettings>;
}

const CATEGORY_KEYS: Record<string, MessageKey> = {
  audit: 'activity.category.audit',
  event: 'activity.category.event',
  debug: 'activity.category.debug',
};

const ACTOR_KEYS: Record<string, MessageKey> = {
  system: 'activity.actor.system',
  anonymous: 'activity.actor.anonymous',
  token: 'activity.actor.token',
  user: 'activity.actor.user',
};

const SESSION_STATE_KEYS: Record<DiagnosticSession['state'], MessageKey> = {
  active: 'diagnostics.state.active',
  stopped: 'diagnostics.state.stopped',
  expired: 'diagnostics.state.expired',
  replaced: 'diagnostics.state.replaced',
  interrupted: 'diagnostics.state.interrupted',
};

type Reading<T> =
  { state: 'reading' } | { state: 'read'; value: T } | { state: 'failed' };

function describe(error: unknown): string {
  return error instanceof BenchCoreError
    ? error.detail
    : error instanceof Error
      ? error.message
      : String(error);
}

/**
 * The shared Activity page: audit, operational events and debug detail read
 * through one filtered, server-paginated query, with export and the timed
 * diagnostics controls beside it. Nothing here is inferred: every row is what
 * the appliance recorded, and the retained window says what it still holds.
 */
export function ActivityPage({
  initialFilter = {},
  readActivity,
  exportActivity,
  canExport,
  canManageDiagnostics,
  readSessions,
  startSession,
  stopSession,
  readSettings,
  saveSettings,
}: ActivityPageProps) {
  const t = useTranslate();
  const [draft, setDraft] = useState<ActivityFilter>(initialFilter);
  const [filter, setFilter] = useState<ActivityFilter>(initialFilter);
  const [page, setPage] = useState<Reading<ActivityPageBody>>({ state: 'reading' });
  const [older, setOlder] = useState<ActivityEvent[]>([]);
  const [expanded, setExpanded] = useState<string | null>(null);
  const [exportNote, setExportNote] = useState('');
  const [sessions, setSessions] = useState<Reading<DiagnosticSessions>>({
    state: 'reading',
  });
  const [scope, setScope] = useState('network');
  const [target, setTarget] = useState('');
  const [minutes, setMinutes] = useState(15);
  const [settings, setSettings] = useState<DiagnosticSettings | null>(null);
  const [diagnosticsNote, setDiagnosticsNote] = useState('');

  const reload = useCallback(
    async (abandoned: () => boolean) => {
      try {
        const value = await readActivity({ ...filter, cursor: '' });
        if (abandoned()) return;
        setPage({ state: 'read', value });
        setOlder([]);
      } catch {
        if (!abandoned()) setPage({ state: 'failed' });
      }
    },
    [readActivity, filter],
  );

  useEffect(() => {
    let abandoned = false;
    void (async () => {
      await reload(() => abandoned);
    })();
    return () => {
      abandoned = true;
    };
  }, [reload]);

  const reloadDiagnostics = useCallback(async () => {
    if (readSessions === undefined) return;
    try {
      setSessions({ state: 'read', value: await readSessions() });
    } catch {
      setSessions({ state: 'failed' });
    }
    if (readSettings !== undefined) {
      try {
        setSettings(await readSettings());
      } catch {
        setSettings(null);
      }
    }
  }, [readSessions, readSettings]);

  useEffect(() => {
    if (!canManageDiagnostics) return;
    // Started from inside an async wrapper, matching the other pages: a
    // microtask separates the effect from the first state change.
    void (async () => {
      await reloadDiagnostics();
    })();
  }, [canManageDiagnostics, reloadDiagnostics]);

  const loadOlder = async () => {
    if (page.state !== 'read' || page.value.nextCursor === '') return;
    const last =
      older.length > 0
        ? older[older.length - 1]
        : page.value.items[page.value.items.length - 1];
    try {
      const next = await readActivity({
        ...filter,
        cursor: String(last?.sequence ?? ''),
      });
      setOlder([...older, ...next.items]);
      setPage({ state: 'read', value: { ...page.value, nextCursor: next.nextCursor } });
    } catch {
      setPage({ state: 'failed' });
    }
  };

  const runExport = async (format: 'jsonl' | 'csv') => {
    if (exportActivity === undefined) return;
    try {
      const text = await exportActivity(filter, format);
      const rows =
        format === 'jsonl'
          ? Math.max(text.trim().split('\n').length - 2, 0)
          : text.split('\n').filter((line) => line !== '' && !line.startsWith('#'))
              .length - 1;
      setExportNote(t('activity.export.done').replace('{rows}', String(rows)));
      const blob = new Blob([text], {
        type: format === 'csv' ? 'text/csv' : 'application/x-ndjson',
      });
      const url = URL.createObjectURL(blob);
      const anchor = document.createElement('a');
      anchor.href = url;
      anchor.download = `nilo-activity.${format}`;
      anchor.click();
      URL.revokeObjectURL(url);
    } catch {
      setExportNote(t('activity.export.failed'));
    }
  };

  const start = async () => {
    if (startSession === undefined) return;
    try {
      await startSession(scope, target.trim(), minutes);
      setDiagnosticsNote('');
      await reloadDiagnostics();
    } catch (error) {
      setDiagnosticsNote(t('diagnostics.failed').replace('{detail}', describe(error)));
    }
  };

  const stop = async (id: string) => {
    if (stopSession === undefined) return;
    try {
      await stopSession(id);
      setDiagnosticsNote('');
      await reloadDiagnostics();
    } catch (error) {
      setDiagnosticsNote(t('diagnostics.failed').replace('{detail}', describe(error)));
    }
  };

  const save = async () => {
    if (saveSettings === undefined || settings === null) return;
    try {
      setSettings(await saveSettings(settings));
      setDiagnosticsNote(t('diagnostics.saved'));
    } catch (error) {
      setDiagnosticsNote(t('diagnostics.failed').replace('{detail}', describe(error)));
    }
  };

  const filterField = (key: keyof ActivityFilter, label: MessageKey, type = 'text') => (
    <div className="field">
      <label htmlFor={`activity-${key}`}>{t(label)}</label>
      <input
        id={`activity-${key}`}
        type={type}
        value={String(draft[key] ?? '')}
        onChange={(event) => setDraft({ ...draft, [key]: event.target.value })}
      />
    </div>
  );

  const rows = page.state === 'read' ? [...page.value.items, ...older] : [];

  return (
    <div className="page activity-page">
      <header className="page-head">
        <h2>{t('activity.title')}</h2>
      </header>
      <div className="page-body">
        <form
          className="form activity-filters"
          aria-label={t('activity.apply')}
          onSubmit={(event) => {
            event.preventDefault();
            setFilter({ ...draft, cursor: '' });
          }}
        >
          <div className="field">
            <label htmlFor="activity-category">{t('activity.filter.category')}</label>
            <select
              id="activity-category"
              value={draft.category ?? ''}
              onChange={(event) => setDraft({ ...draft, category: event.target.value })}
            >
              <option value="">{t('activity.category.all')}</option>
              {ACTIVITY_CATEGORIES.map((category) => (
                <option key={category} value={category}>
                  {t(CATEGORY_KEYS[category])}
                </option>
              ))}
            </select>
          </div>
          {filterField('component', 'activity.filter.component')}
          {filterField('target', 'activity.filter.target')}
          {filterField('actor', 'activity.filter.actor')}
          {filterField('severity', 'activity.filter.severity')}
          {filterField('outcome', 'activity.filter.outcome')}
          {filterField('operation', 'activity.filter.operation')}
          {filterField('from', 'activity.filter.from', 'datetime-local')}
          {filterField('to', 'activity.filter.to', 'datetime-local')}
          <div className="form-actions">
            <button type="submit">{t('activity.apply')}</button>
            <button
              type="button"
              onClick={() => {
                setDraft({});
                setFilter({});
              }}
            >
              {t('activity.clear')}
            </button>
            {canExport && exportActivity !== undefined && (
              <>
                <button type="button" onClick={() => void runExport('jsonl')}>
                  {t('activity.export.jsonl')}
                </button>
                <button type="button" onClick={() => void runExport('csv')}>
                  {t('activity.export.csv')}
                </button>
              </>
            )}
          </div>
          {exportNote !== '' && <p role="status">{exportNote}</p>}
        </form>

        {page.state === 'reading' && <p>{t('activity.reading')}</p>}
        {page.state === 'failed' && (
          <p className="notice notice-unknown">{t('activity.unreadable')}</p>
        )}
        {page.state === 'read' && (
          <>
            <p className="network-secondary" role="status">
              {page.value.loggingHealth.auditWritable
                ? t('activity.health.ok')
                : t('activity.health.degraded').replace(
                    '{failure}',
                    page.value.loggingHealth.lastFailure,
                  )}
              {page.value.loggingHealth.droppedDebug > 0 &&
                ` ${t('activity.health.dropped').replace('{count}', String(page.value.loggingHealth.droppedDebug))}`}
            </p>
            <p className="network-secondary">
              {Object.entries(page.value.retainedWindow).map(([category, window]) => (
                <span key={category}>
                  {t('activity.retained')
                    .replace('{count}', String(window.count))
                    .replace(
                      '{category}',
                      t(CATEGORY_KEYS[category] ?? 'activity.category.all'),
                    )}
                  {window.truncated && window.purgedBefore !== null && (
                    <>
                      {' '}
                      {t('activity.gap')
                        .replace('{time}', window.purgedBefore.toLocaleString())
                        .replace('{reason}', window.purgeReason)}
                    </>
                  )}
                  {'  '}
                </span>
              ))}
            </p>
            {rows.length === 0 ? (
              <p>{t('activity.empty')}</p>
            ) : (
              <div className="table-scroll">
                <table className="data-table">
                  <thead>
                    <tr>
                      <th scope="col">{t('activity.col.time')}</th>
                      <th scope="col">{t('activity.col.category')}</th>
                      <th scope="col">{t('activity.col.component')}</th>
                      <th scope="col">{t('activity.col.action')}</th>
                      <th scope="col">{t('activity.col.target')}</th>
                      <th scope="col">{t('activity.col.actor')}</th>
                      <th scope="col">{t('activity.col.outcome')}</th>
                      <th scope="col">{t('activity.details')}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {rows.map((event) => (
                      <Fragment key={event.id}>
                        <tr>
                          <td>{event.createdAt.toLocaleString()}</td>
                          <td>
                            {t(
                              CATEGORY_KEYS[event.category] ?? 'activity.category.all',
                            )}
                          </td>
                          <td>{event.component}</td>
                          <td>
                            <code>{event.action}</code>
                          </td>
                          <td>
                            {event.targetLabel !== '' &&
                            event.targetLabel !== event.subjectId ? (
                              <>
                                {event.targetLabel}
                                <small className="network-secondary">
                                  {event.subjectId}
                                </small>
                              </>
                            ) : (
                              event.subjectId
                            )}
                          </td>
                          <td title={event.actorId}>
                            {t(ACTOR_KEYS[event.actorKind] ?? 'activity.actor.user')}
                          </td>
                          <td>
                            {event.outcome}
                            {event.severity !== 'info' && (
                              <small className="network-secondary">
                                {event.severity}
                              </small>
                            )}
                          </td>
                          <td>
                            <button
                              type="button"
                              className="network-detail-toggle"
                              aria-expanded={expanded === event.id}
                              aria-label={`${t('activity.details')} ${event.sequence}`}
                              onClick={() =>
                                setExpanded(expanded === event.id ? null : event.id)
                              }
                            >
                              {t('activity.details')}
                            </button>
                          </td>
                        </tr>
                        {expanded === event.id && (
                          <tr>
                            <td colSpan={8}>
                              {event.operationId !== '' && (
                                <p>
                                  <button
                                    type="button"
                                    onClick={() => {
                                      setDraft({ operation: event.operationId });
                                      setFilter({ operation: event.operationId });
                                    }}
                                  >
                                    {t('activity.operation').replace(
                                      '{id}',
                                      event.operationId,
                                    )}
                                  </button>
                                </p>
                              )}
                              <pre className="activity-details">
                                {JSON.stringify(event.details, null, 2)}
                              </pre>
                            </td>
                          </tr>
                        )}
                      </Fragment>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
            {page.value.nextCursor !== '' && (
              <button type="button" onClick={() => void loadOlder()}>
                {t('activity.more')}
              </button>
            )}
          </>
        )}

        {canManageDiagnostics && readSessions !== undefined && (
          <section aria-labelledby="activity-diagnostics">
            <h3 id="activity-diagnostics">{t('diagnostics.title')}</h3>
            <form
              className="form"
              aria-label={t('diagnostics.start')}
              onSubmit={(event) => {
                event.preventDefault();
                void start();
              }}
            >
              <div className="field">
                <label htmlFor="diagnostics-scope">{t('diagnostics.scope')}</label>
                <select
                  id="diagnostics-scope"
                  value={scope}
                  onChange={(event) => setScope(event.target.value)}
                >
                  {(sessions.state === 'read'
                    ? sessions.value.scopes
                    : ['network']
                  ).map((candidate) => (
                    <option key={candidate} value={candidate}>
                      {candidate}
                    </option>
                  ))}
                </select>
              </div>
              <div className="field">
                <label htmlFor="diagnostics-target">{t('diagnostics.target')}</label>
                <input
                  id="diagnostics-target"
                  value={target}
                  onChange={(event) => setTarget(event.target.value)}
                />
              </div>
              <div className="field">
                <label htmlFor="diagnostics-minutes">{t('diagnostics.minutes')}</label>
                <select
                  id="diagnostics-minutes"
                  value={minutes}
                  onChange={(event) => setMinutes(Number(event.target.value))}
                >
                  {(sessions.state === 'read'
                    ? sessions.value.durations
                    : [5, 15, 60]
                  ).map((candidate) => (
                    <option key={candidate} value={candidate}>
                      {t('diagnostics.minutesValue').replace(
                        '{minutes}',
                        String(candidate),
                      )}
                    </option>
                  ))}
                </select>
              </div>
              <div className="form-actions">
                <button type="submit">{t('diagnostics.start')}</button>
              </div>
            </form>
            {diagnosticsNote !== '' && <p role="status">{diagnosticsNote}</p>}
            {sessions.state === 'failed' && (
              <p className="notice notice-unknown">{t('diagnostics.unreadable')}</p>
            )}
            {sessions.state === 'read' && sessions.value.items.length === 0 && (
              <p>{t('diagnostics.none')}</p>
            )}
            {sessions.state === 'read' && sessions.value.items.length > 0 && (
              <div className="table-scroll">
                <table className="data-table">
                  <thead>
                    <tr>
                      <th scope="col">{t('diagnostics.scope')}</th>
                      <th scope="col">{t('diagnostics.target')}</th>
                      <th scope="col">{t('network.table.state')}</th>
                      <th scope="col">{t('diagnostics.minutes')}</th>
                      <th scope="col">{t('network.table.actions')}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {sessions.value.items.map((session) => (
                      <tr key={session.id}>
                        <th scope="row">{session.scope}</th>
                        <td>{session.target || '—'}</td>
                        <td>
                          {t(SESSION_STATE_KEYS[session.state])}
                          {session.state === 'active' && (
                            <small className="network-secondary">
                              {t('diagnostics.expires').replace(
                                '{time}',
                                session.expiresAt.toLocaleTimeString(),
                              )}
                            </small>
                          )}
                        </td>
                        <td>
                          {t('diagnostics.minutesValue').replace(
                            '{minutes}',
                            String(session.minutes),
                          )}
                        </td>
                        <td>
                          {session.state === 'active' && stopSession !== undefined && (
                            <button
                              type="button"
                              aria-label={`${t('diagnostics.stop')} ${session.scope}`}
                              onClick={() => void stop(session.id)}
                            >
                              {t('diagnostics.stop')}
                            </button>
                          )}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
            {settings !== null && saveSettings !== undefined && (
              <form
                className="form"
                aria-label={t('diagnostics.settings')}
                onSubmit={(event) => {
                  event.preventDefault();
                  void save();
                }}
              >
                <h4>{t('diagnostics.settings')}</h4>
                {ACTIVITY_CATEGORIES.map((category) => (
                  <Fragment key={category}>
                    <div className="field">
                      <label htmlFor={`retention-${category}`}>
                        {t('diagnostics.retention').replace(
                          '{category}',
                          t(CATEGORY_KEYS[category]),
                        )}
                      </label>
                      <input
                        id={`retention-${category}`}
                        inputMode="numeric"
                        value={String(settings.retentionDays[category] ?? '')}
                        onChange={(event) =>
                          setSettings({
                            ...settings,
                            retentionDays: {
                              ...settings.retentionDays,
                              [category]: Number(event.target.value),
                            },
                          })
                        }
                      />
                    </div>
                    <div className="field">
                      <label htmlFor={`quota-${category}`}>
                        {t('diagnostics.quota').replace(
                          '{category}',
                          t(CATEGORY_KEYS[category]),
                        )}
                      </label>
                      <input
                        id={`quota-${category}`}
                        inputMode="numeric"
                        value={String(
                          Math.round((settings.quotaBytes[category] ?? 0) / 1048576),
                        )}
                        onChange={(event) =>
                          setSettings({
                            ...settings,
                            quotaBytes: {
                              ...settings.quotaBytes,
                              [category]: Number(event.target.value) * 1048576,
                            },
                          })
                        }
                      />
                    </div>
                  </Fragment>
                ))}
                <div className="form-actions">
                  <button type="submit">{t('diagnostics.save')}</button>
                </div>
              </form>
            )}
          </section>
        )}
      </div>
    </div>
  );
}
