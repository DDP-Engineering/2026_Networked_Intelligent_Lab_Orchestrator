import type { ActivityFilter } from '../../api/activity';

/**
 * The filter panel's fields, and the conversion the appliance's time format
 * needs.
 *
 * Bench Core parses `from` and `to` as RFC 3339 and refuses anything else. A
 * `datetime-local` control produces neither a zone nor seconds, so the two
 * formats are converted here rather than in the page: the operator types a
 * local wall-clock time, the appliance is asked in UTC, and a filter that
 * arrived in RFC 3339 renders back in the control it came from.
 */

/** The filters that stay visible, in the order the panel lays them out. */
export const PRIMARY_FILTER_FIELDS = [
  'from',
  'to',
  'actor',
  'target',
  'action',
  'outcome',
] as const;

/** The filters kept behind the More filters disclosure. */
export const MORE_FILTER_FIELDS = ['component', 'severity', 'operation'] as const;

/** Every field the panel owns. Category is a chip, and cursor is not a filter. */
export const PANEL_FILTER_FIELDS = [
  ...PRIMARY_FILTER_FIELDS,
  ...MORE_FILTER_FIELDS,
] as const;

export type PanelFilterField = (typeof PANEL_FILTER_FIELDS)[number];

const MILLISECONDS = /\.\d{3}Z$/;

/**
 * A complete `datetime-local` value, or the RFC 3339 instant one converts to.
 *
 * The shape is checked rather than left to `Date`, which happily reads a
 * half-typed `2026-09-` as a date and would send the appliance a filter the
 * operator never asked for.
 */
const COMPLETE_INSTANT =
  /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}(:\d{2}(\.\d+)?)?(Z|[+-]\d{2}:\d{2})?$/;

/**
 * A `datetime-local` value as the UTC instant it names.
 *
 * The empty string and anything the browser could not complete convert to the
 * empty string, which the query builder then omits: an unfinished date must
 * narrow nothing rather than be sent as a filter the appliance will reject.
 */
export function localInputToRfc3339(value: string): string {
  const trimmed = value.trim();
  if (!COMPLETE_INSTANT.test(trimmed)) {
    return '';
  }
  const at = new Date(trimmed);
  if (Number.isNaN(at.getTime())) {
    return '';
  }
  // Seconds are kept and milliseconds dropped: RFC 3339 requires the former
  // and the control never offers the latter.
  return at.toISOString().replace(MILLISECONDS, 'Z');
}

/** An RFC 3339 instant as the local wall-clock value the control renders. */
export function rfc3339ToLocalInput(value: string): string {
  const trimmed = value.trim();
  if (trimmed === '') {
    return '';
  }
  const at = new Date(trimmed);
  if (Number.isNaN(at.getTime())) {
    return '';
  }
  const pad = (part: number) => String(part).padStart(2, '0');
  return (
    `${String(at.getFullYear()).padStart(4, '0')}-${pad(at.getMonth() + 1)}-${pad(at.getDate())}` +
    `T${pad(at.getHours())}:${pad(at.getMinutes())}`
  );
}

/** The draft a filter opens the panel with: its instants in local form. */
export function draftOf(filter: ActivityFilter): ActivityFilter {
  return {
    ...filter,
    from: rfc3339ToLocalInput(filter.from ?? ''),
    to: rfc3339ToLocalInput(filter.to ?? ''),
  };
}

/** The query a draft becomes: its instants in UTC, and no cursor. */
export function appliedOf(draft: ActivityFilter): ActivityFilter {
  return {
    ...draft,
    from: localInputToRfc3339(draft.from ?? ''),
    to: localInputToRfc3339(draft.to ?? ''),
    cursor: '',
  };
}

/** How many of the panel's fields are narrowing the listing. */
export function activeFilterCount(filter: ActivityFilter): number {
  return PANEL_FILTER_FIELDS.filter(
    (field) => String(filter[field] ?? '').trim() !== '',
  ).length;
}

/** The same filter with every panel field cleared; the category is a chip. */
export function withoutPanelFilters(filter: ActivityFilter): ActivityFilter {
  const cleared: ActivityFilter = { category: filter.category, cursor: '' };
  if (filter.sort !== undefined) cleared.sort = filter.sort;
  if (filter.order !== undefined) cleared.order = filter.order;
  if (filter.limit !== undefined) cleared.limit = filter.limit;
  if (filter.page !== undefined) cleared.page = 1;
  return cleared;
}
