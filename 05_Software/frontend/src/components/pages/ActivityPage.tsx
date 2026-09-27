import { MessageBanner } from '../MessageBanner';
import { Fragment, useCallback, useEffect, useRef, useState } from 'react';

import type {
  ActivityFilter,
  ActivityPage as ActivityPageBody,
  ActivityPageSize,
  ActivityRetentionSettings,
  ActivitySort,
} from '../../api/activity';
import { ACTIVITY_CATEGORIES } from '../../api/activity';
import { BenchCoreError } from '../../api/client';
import type { MessageKey } from '../../i18n/en';
import { useTranslate } from '../../i18n/languages';
import { ActivityEventTable } from '../activity/ActivityEventTable';
import { ActivityHealthBanner } from '../activity/ActivityHealthBanner';
import {
  MORE_FILTER_FIELDS,
  PRIMARY_FILTER_FIELDS,
  activeFilterCount,
  appliedOf,
  draftOf,
  withoutPanelFilters,
  type PanelFilterField,
} from '../activity/filters';
import { CATEGORY_KEYS, COMPONENT_FILTER_OPTIONS } from '../activity/vocabulary';
import { Menu, MenuAction } from '../shell/Menu';
import { useViewportClass } from '../shell/useViewportClass';

export interface ActivityPageProps {
  /** The filter the page opens with, for example from a network table. */
  initialFilter?: ActivityFilter;
  readActivity: (filter: ActivityFilter) => Promise<ActivityPageBody>;
  exportActivity?: (filter: ActivityFilter, format: 'jsonl' | 'csv') => Promise<string>;
  canExport: boolean;
  canManageRetention: boolean;
  readRetention?: () => Promise<ActivityRetentionSettings>;
  saveRetention?: (
    settings: ActivityRetentionSettings,
  ) => Promise<ActivityRetentionSettings>;
}

const FILTER_LABELS: Record<PanelFilterField, MessageKey> = {
  from: 'activity.filter.from',
  to: 'activity.filter.to',
  actor: 'activity.filter.actor',
  target: 'activity.filter.target',
  action: 'activity.filter.action',
  outcome: 'activity.filter.outcome',
  component: 'activity.filter.component',
  severity: 'activity.filter.severity',
  operation: 'activity.filter.operation',
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

const DEFAULT_ACTIVITY_QUERY: Pick<
  ActivityFilter,
  'sort' | 'order' | 'limit' | 'page' | 'cursor'
> = {
  sort: 'time',
  order: 'desc',
  limit: 10,
  page: 1,
  cursor: '',
};

/** The page sizes the listing offers, smallest first; `all` follows them. */
const ACTIVITY_PAGE_SIZES = [10, 25, 50, 100] as const;

interface ActivityPaginationProps {
  total: number;
  page: number;
  size: ActivityPageSize;
  onPage: (page: number) => void;
  onSize: (size: ActivityPageSize) => void;
}

function ActivityPagination({
  total,
  page,
  size,
  onPage,
  onSize,
}: ActivityPaginationProps) {
  const t = useTranslate();
  const pageCount = size === 'all' ? 1 : Math.max(Math.ceil(total / size), 1);
  const selectedPage = Math.min(page, pageCount);
  const pageNumbers = [
    ...new Set([
      1,
      selectedPage - 2,
      selectedPage - 1,
      selectedPage,
      selectedPage + 1,
      selectedPage + 2,
      pageCount,
    ]),
  ]
    .filter((number) => number >= 1 && number <= pageCount)
    .sort((left, right) => left - right);

  return (
    <nav className="activity-pagination" aria-label={t('activity.pagination')}>
      <div className="field activity-page-size">
        <label htmlFor="activity-page-size">{t('activity.pagination.size')}</label>
        <select
          id="activity-page-size"
          value={String(size)}
          onChange={(event) => {
            const value = event.target.value;
            onSize(value === 'all' ? 'all' : (Number(value) as 10 | 25 | 50 | 100));
          }}
        >
          {ACTIVITY_PAGE_SIZES.map((value) => (
            <option key={value} value={value}>
              {value}
            </option>
          ))}
          <option value="all">{t('activity.pagination.all')}</option>
        </select>
      </div>
      <button
        type="button"
        className="button"
        disabled={selectedPage <= 1}
        onClick={() => onPage(selectedPage - 1)}
      >
        {t('activity.pagination.previous')}
      </button>
      <div
        className="activity-page-numbers"
        role="group"
        aria-label={t('activity.pagination.page')}
      >
        {pageNumbers.map((number, index) => (
          <Fragment key={number}>
            {index > 0 && number - pageNumbers[index - 1] > 1 && (
              <span aria-hidden="true">…</span>
            )}
            <button
              type="button"
              className={`button ${number === selectedPage ? 'button-primary' : ''}`}
              aria-label={t('activity.pagination.select', { page: number })}
              aria-current={number === selectedPage ? 'page' : undefined}
              onClick={() => {
                if (number !== selectedPage) onPage(number);
              }}
            >
              {number}
            </button>
          </Fragment>
        ))}
      </div>
      <span className="activity-page-total">
        {t('activity.pagination.total', { pages: pageCount, records: total })}
      </span>
      <button
        type="button"
        className="button"
        disabled={selectedPage >= pageCount}
        onClick={() => onPage(selectedPage + 1)}
      >
        {t('activity.pagination.next')}
      </button>
    </nav>
  );
}

/**
 * The shared Activity page: audit and operational events read through one
 * filtered, server-paginated query, with export and retention controls beside
 * it. Nothing here is inferred: every row is what
 * the appliance recorded, and the retained window says what it still holds.
 *
 * The page is organised around the records rather than around its controls, and
 * its furniture is two lines: the title with Export, and one wrapping line of
 * scope, filters and the disclosure the three least used filters sit behind.
 * Category is part of that line because it is the one filter always in play -
 * except where the viewport collapses the panel, when scope stays outside the
 * drawer so a bench panel keeps it a tap away. Retention is administration of
 * the log rather than the log itself, so it stays collapsed, above the filters
 * where an administrator reaches it without scrolling past the rows it governs.
 */
export function ActivityPage({
  initialFilter = {},
  readActivity,
  exportActivity,
  canExport,
  canManageRetention,
  readRetention,
  saveRetention,
}: ActivityPageProps) {
  const t = useTranslate();
  // The draft renders the caller's instants in the local control; the applied
  // filter keeps them exactly as they arrived, because a round trip through a
  // control that has no seconds would move the window the caller asked for.
  const [draft, setDraft] = useState<ActivityFilter>(() =>
    draftOf({ ...DEFAULT_ACTIVITY_QUERY, ...initialFilter }),
  );
  const [filter, setFilter] = useState<ActivityFilter>(() => ({
    ...DEFAULT_ACTIVITY_QUERY,
    ...initialFilter,
    cursor: '',
  }));
  const [page, setPage] = useState<Reading<ActivityPageBody>>({ state: 'reading' });
  const [exportNote, setExportNote] = useState('');
  const [exportFailed, setExportFailed] = useState(false);
  // The disclosure opens on its own whenever one of the filters behind it is
  // in effect, so a count of one is never a count of something hidden.
  const [moreOpen, setMoreOpen] = useState(() =>
    MORE_FILTER_FIELDS.some((field) => String(initialFilter[field] ?? '') !== ''),
  );
  const [settings, setSettings] = useState<ActivityRetentionSettings | null>(null);
  const [retentionNote, setRetentionNote] = useState('');
  const [retentionFailed, setRetentionFailed] = useState(false);

  // On a field display the records come first: the whole filter panel sits
  // behind a disclosure that opens only when it is wanted.
  const viewport = useViewportClass();
  const collapsible = viewport !== 'wide';
  const [filtersOpen, setFiltersOpen] = useState(false);

  // A page load in flight must not be able to append to a listing that has
  // since been re-queried under different filters.
  const generation = useRef(0);

  /**
   * The one way the applied filter changes.
   *
   * The previous page is taken down in the same event as the new query is
   * chosen: rows read under one filter must never stand beneath the count and
   * the export of another, however briefly, on a screen that is read as
   * evidence. The generation moves here too, synchronously, so an older page
   * still in flight has already been disowned by the time the effect asks for
   * the new one — and its loading state is reset with everything else, rather
   * than left to a `finally` that no longer recognises the listing.
   */
  const applyQuery = (next: ActivityFilter) => {
    generation.current += 1;
    setFilter(next);
    setPage({ state: 'reading' });
  };

  const applyFilter = (next: ActivityFilter) =>
    applyQuery({ ...next, page: 1, cursor: '' });

  const reload = useCallback(
    async (abandoned: () => boolean) => {
      try {
        const value = await readActivity({ ...filter, cursor: '' });
        if (abandoned()) return;
        setPage({ state: 'read', value });
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

  useEffect(() => {
    if (!canManageRetention || readRetention === undefined) return;
    let abandoned = false;
    void (async () => {
      try {
        const value = await readRetention();
        if (!abandoned) setSettings(value);
      } catch {
        if (!abandoned) setSettings(null);
      }
    })();
    return () => {
      abandoned = true;
    };
  }, [canManageRetention, readRetention]);

  const rows = page.state === 'read' ? page.value.items : [];

  const runExport = async (format: 'jsonl' | 'csv') => {
    if (exportActivity === undefined) return;
    setExportNote('');
    try {
      // The applied filter, never the draft: an export must contain what the
      // listing above it contains.
      const text = await exportActivity(filter, format);
      const rowCount =
        format === 'jsonl'
          ? Math.max(text.trim().split('\n').length - 2, 0)
          : text.split('\n').filter((line) => line !== '' && !line.startsWith('#'))
              .length - 1;
      setExportFailed(false);
      setExportNote(t('activity.export.done', { rows: rowCount }));
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
      setExportFailed(true);
      setExportNote(t('activity.export.failed'));
    }
  };

  const save = async () => {
    if (saveRetention === undefined || settings === null) return;
    setRetentionNote('');
    try {
      setSettings(await saveRetention(settings));
      setRetentionFailed(false);
      setRetentionNote(t('activity.retention.saved'));
    } catch (error) {
      setRetentionFailed(true);
      setRetentionNote(t('activity.retention.failed', { detail: describe(error) }));
    }
  };

  /**
   * Select a category chip. The listing re-queries at once, and only the
   * category moves: a filter that has been typed but not applied stays a draft,
   * because a chip is not an Apply control in disguise.
   */
  const selectCategory = (category: string) => {
    setDraft({ ...draft, category });
    applyFilter({ ...filter, category, cursor: '' });
  };

  /**
   * Narrow to one operation. Every other filter goes, the category included:
   * the stages of one change are not all audit records, and a chip left in
   * place would hide the half of the operation being asked for.
   */
  const showOperation = (operationId: string) => {
    const next = {
      ...DEFAULT_ACTIVITY_QUERY,
      category: '',
      operation: operationId,
    };
    setDraft(draftOf(next));
    applyFilter(next);
    setMoreOpen(true);
  };

  const sortBy = (field: ActivitySort) => {
    const order = filter.sort === field && filter.order === 'asc' ? 'desc' : 'asc';
    const next: ActivityFilter = { ...filter, sort: field, order };
    setDraft({ ...draft, sort: field, order, page: 1, cursor: '' });
    applyFilter(next);
  };

  const filterField = (field: PanelFilterField) => (
    <div className="field" key={field}>
      <label htmlFor={`activity-${field}`}>{t(FILTER_LABELS[field])}</label>
      {field === 'component' ? (
        <select
          id="activity-component"
          value={String(draft.component ?? '')}
          onChange={(event) => setDraft({ ...draft, component: event.target.value })}
        >
          {!COMPONENT_FILTER_OPTIONS.some(
            (option) => option.value === String(draft.component ?? ''),
          ) && <option value={draft.component}>{draft.component}</option>}
          {COMPONENT_FILTER_OPTIONS.map((option) => (
            <option key={option.value || 'all'} value={option.value}>
              {t(option.label)}
            </option>
          ))}
        </select>
      ) : (
        <input
          id={`activity-${field}`}
          type={field === 'from' || field === 'to' ? 'datetime-local' : 'text'}
          value={String(draft[field] ?? '')}
          onChange={(event) => setDraft({ ...draft, [field]: event.target.value })}
        />
      )}
    </div>
  );

  const active = activeFilterCount(filter);
  const category = draft.category ?? '';
  const countText =
    active === 0
      ? t('activity.filter.none')
      : t('activity.filter.active', { count: active });

  const chips = (
    <div
      className="activity-chips"
      role="group"
      aria-label={t('activity.filter.category')}
    >
      {(['', ...ACTIVITY_CATEGORIES] as const).map((candidate) => (
        <button
          key={candidate === '' ? 'all' : candidate}
          type="button"
          className={`activity-chip ${category === candidate ? 'activity-chip-on' : ''}`}
          aria-pressed={category === candidate}
          onClick={() => selectCategory(candidate)}
        >
          {t(candidate === '' ? 'activity.category.all' : CATEGORY_KEYS[candidate])}
        </button>
      ))}
    </div>
  );

  const filters = (
    <form
      className="form activity-filters"
      aria-label={t('activity.filters')}
      onSubmit={(event) => {
        event.preventDefault();
        applyFilter(appliedOf(draft));
        if (collapsible) setFiltersOpen(false);
      }}
    >
      {/* Scope leads the one filter line where the panel is shown inline; where
          the viewport collapses the panel it stays outside, above the drawer. */}
      {!collapsible && chips}
      {PRIMARY_FILTER_FIELDS.map((field) => filterField(field))}
      {/* A disclosure button rather than a details element: the three fields it
          reveals join the same wrapping line as the six above, which an
          expanding details box could only do by taking a row to itself. */}
      <button
        type="button"
        className="button activity-filters-more"
        aria-expanded={moreOpen}
        aria-controls="activity-more-filters"
        onClick={() => setMoreOpen(!moreOpen)}
      >
        {t('activity.filter.more')}
      </button>
      <div
        id="activity-more-filters"
        className="activity-filters-more-fields"
        hidden={!moreOpen}
      >
        {MORE_FILTER_FIELDS.map((field) => filterField(field))}
      </div>
      <div className="form-actions">
        <button type="submit" className="button button-primary">
          {t('activity.apply')}
        </button>
        <button
          type="button"
          className="button"
          onClick={() => {
            setDraft(withoutPanelFilters(draft));
            applyFilter(withoutPanelFilters(filter));
            if (collapsible) setFiltersOpen(false);
          }}
        >
          {t('activity.clear')}
        </button>
        <p className="activity-filter-count" role="status">
          {countText}
        </p>
      </div>
    </form>
  );

  const exportMenu = canExport && exportActivity !== undefined && (
    <Menu label={t('activity.export')} triggerClass="activity-export">
      {(close) => (
        <>
          <MenuAction
            onSelect={() => {
              close();
              void runExport('csv');
            }}
          >
            {t('activity.export.csv')}
          </MenuAction>
          <MenuAction
            onSelect={() => {
              close();
              void runExport('jsonl');
            }}
          >
            {t('activity.export.jsonl')}
          </MenuAction>
          <p className="menu-foot">{t('activity.export.scope')}</p>
        </>
      )}
    </Menu>
  );

  /**
   * Retention is administration of the log rather than the log itself, so it
   * stays a disclosure - but it belongs above the listing, where an
   * administrator reaches it without scrolling past every row it governs.
   */
  const retention = canManageRetention &&
    settings !== null &&
    saveRetention !== undefined && (
      <details className="form-group activity-retention">
        <summary>{t('activity.retention')}</summary>
        <section aria-label={t('activity.retention')}>
          <h3>{t('activity.retention.title')}</h3>
          <p className="page-foot">{t('activity.retention.intro')}</p>
          <form
            className="form activity-retention-form"
            aria-label={t('activity.retention.title')}
            onSubmit={(event) => {
              event.preventDefault();
              void save();
            }}
          >
            {ACTIVITY_CATEGORIES.map((candidate) => (
              <Fragment key={candidate}>
                <div className="field">
                  <label htmlFor={`retention-${candidate}`}>
                    {t('activity.retention.days', {
                      category: t(CATEGORY_KEYS[candidate]),
                    })}
                  </label>
                  <input
                    id={`retention-${candidate}`}
                    inputMode="numeric"
                    value={String(settings.retentionDays[candidate] ?? '')}
                    onChange={(event) =>
                      setSettings({
                        ...settings,
                        retentionDays: {
                          ...settings.retentionDays,
                          [candidate]: Number(event.target.value),
                        },
                      })
                    }
                  />
                </div>
                <div className="field">
                  <label htmlFor={`quota-${candidate}`}>
                    {t('activity.retention.quota', {
                      category: t(CATEGORY_KEYS[candidate]),
                    })}
                  </label>
                  <input
                    id={`quota-${candidate}`}
                    inputMode="numeric"
                    value={String(
                      Math.round((settings.quotaBytes[candidate] ?? 0) / 1048576),
                    )}
                    onChange={(event) =>
                      setSettings({
                        ...settings,
                        quotaBytes: {
                          ...settings.quotaBytes,
                          [candidate]: Number(event.target.value) * 1048576,
                        },
                      })
                    }
                  />
                </div>
              </Fragment>
            ))}
            <div className="form-actions">
              <button type="submit" className="button button-primary">
                {t('activity.retention.save')}
              </button>
            </div>
          </form>
          {retentionNote !== '' && (
            <MessageBanner tone={retentionFailed ? 'error' : 'info'}>
              {retentionNote}
            </MessageBanner>
          )}
        </section>
      </details>
    );

  return (
    <div className="page activity-page">
      <header className="page-head activity-head">
        <h2>{t('activity.title')}</h2>
        {exportMenu}
      </header>
      <div className="page-body">
        {page.state === 'read' && (
          <ActivityHealthBanner
            loggingHealth={page.value.loggingHealth}
            retainedWindow={page.value.retainedWindow}
          />
        )}

        {exportNote !== '' && (
          <MessageBanner tone={exportFailed ? 'error' : 'info'}>
            {exportNote}
          </MessageBanner>
        )}

        {retention}

        {collapsible && chips}

        {collapsible ? (
          <details
            className="form-group activity-filter-drawer"
            open={filtersOpen}
            onToggle={(event) => setFiltersOpen(event.currentTarget.open)}
          >
            <summary>
              {t('activity.filters')}
              <span className="activity-filter-drawer-count">{countText}</span>
            </summary>
            {filters}
          </details>
        ) : (
          filters
        )}

        {page.state === 'reading' && <p>{t('activity.reading')}</p>}
        {page.state === 'failed' && (
          <MessageBanner tone="warning">{t('activity.unreadable')}</MessageBanner>
        )}
        {page.state === 'read' && (
          <>
            {rows.length === 0 ? (
              <p>{t('activity.empty')}</p>
            ) : (
              <ActivityEventTable
                events={rows}
                sort={filter.sort}
                order={filter.order}
                onSort={sortBy}
                onShowOperation={showOperation}
              />
            )}
            <ActivityPagination
              total={page.value.total}
              page={filter.page ?? 1}
              size={(filter.limit ?? 10) as ActivityPageSize}
              onPage={(selected) => applyQuery({ ...filter, page: selected })}
              onSize={(size) => {
                const next = { ...filter, limit: size, page: 1, cursor: '' };
                setDraft({ ...draft, limit: size, page: 1, cursor: '' });
                applyQuery(next);
              }}
            />
          </>
        )}
      </div>
    </div>
  );
}
