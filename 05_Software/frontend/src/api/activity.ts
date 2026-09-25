import { BENCH_CORE_API_VERSION, type ParseResult } from './contract';

/**
 * The shared history and its retention policy.
 *
 * An activity page is what the appliance answered: the items in sequence
 * order, the cursor for the next page, the retained window the answer was
 * drawn from and whether the appliance can still record what it does. Nothing
 * here rewrites an event; the stable codes are rendered by the page.
 */

export const ACTIVITY_PATH = '/api/v1/activity';
export const ACTIVITY_EXPORT_PATH = '/api/v1/activity/export';
export const ACTIVITY_RETENTION_PATH = '/api/v1/activity/retention';

export const ACTIVITY_CATEGORIES = ['audit', 'event'] as const;
export type ActivityCategory = (typeof ACTIVITY_CATEGORIES)[number];

export interface ActivityEvent {
  id: string;
  sequence: number;
  createdAt: Date;
  category: string;
  component: string;
  action: string;
  outcome: string;
  severity: string;
  actorKind: string;
  actorId: string;
  subjectType: string;
  subjectId: string;
  targetLabel: string;
  operationId: string;
  applianceId: string;
  bootId: string;
  timeSource: string;
  /** Safe structured details, as the appliance wrote them. */
  details: Record<string, unknown>;
}

export interface RetainedWindow {
  oldest: Date | null;
  newest: Date | null;
  count: number;
  truncated: boolean;
  purgedBefore: Date | null;
  purgeReason: string;
}

export interface LoggingHealth {
  auditWritable: boolean;
  failures: number;
  lastFailure: string;
}

export interface ActivityPage {
  items: ActivityEvent[];
  nextCursor: string;
  total: number;
  retainedWindow: Record<string, RetainedWindow>;
  loggingHealth: LoggingHealth;
}

export const ACTIVITY_SORT_FIELDS = [
  'time',
  'activity',
  'outcome',
  'actor',
  'target',
] as const;
export type ActivitySort = (typeof ACTIVITY_SORT_FIELDS)[number];
export type ActivitySortOrder = 'asc' | 'desc';
export type ActivityPageSize = 10 | 25 | 50 | 100 | 'all';

/** The filters a query may carry; empty values are not sent. */
export interface ActivityFilter {
  category?: string;
  component?: string;
  target?: string;
  actor?: string;
  severity?: string;
  outcome?: string;
  operation?: string;
  action?: string;
  from?: string;
  to?: string;
  cursor?: string;
  limit?: number | 'all';
  page?: number;
  sort?: ActivitySort;
  order?: ActivitySortOrder;
}

export function activityQuery(
  filter: ActivityFilter,
  format?: 'jsonl' | 'csv',
): string {
  const query = new URLSearchParams();
  for (const [key, value] of Object.entries(filter)) {
    if (value !== undefined && value !== '' && value !== 0) {
      query.set(key, String(value));
    }
  }
  if (format !== undefined) {
    query.set('format', format);
  }
  const encoded = query.toString().replace(/\+/g, '%20');
  return encoded === '' ? '' : `?${encoded}`;
}

export interface ActivityRetentionSettings {
  retentionDays: Record<string, number>;
  quotaBytes: Record<string, number>;
}

function object(value: unknown, where: string): Record<string, unknown> {
  if (value === null || typeof value !== 'object' || Array.isArray(value)) {
    throw new Error(`the ${where} is not a JSON object`);
  }
  return value as Record<string, unknown>;
}
function text(body: Record<string, unknown>, key: string): string {
  const value = body[key];
  if (value === undefined || value === null) return '';
  if (typeof value !== 'string') throw new Error(`bad ${key}`);
  return value;
}
function number(body: Record<string, unknown>, key: string): number {
  const value = body[key];
  if (value === undefined || value === null) return 0;
  if (typeof value !== 'number' || !Number.isFinite(value))
    throw new Error(`bad ${key}`);
  return value;
}
function optionalInstant(body: Record<string, unknown>, key: string): Date | null {
  const value = body[key];
  if (value === undefined || value === null || value === '') return null;
  const parsed = new Date(String(value));
  if (Number.isNaN(parsed.getTime())) throw new Error(`bad ${key}`);
  return parsed;
}
function instant(body: Record<string, unknown>, key: string): Date {
  const value = optionalInstant(body, key);
  if (value === null) throw new Error(`missing ${key}`);
  return value;
}
function version(body: Record<string, unknown>, where: string): void {
  if (body.api_version !== BENCH_CORE_API_VERSION) {
    throw new Error(`the ${where} declares an unsupported API version`);
  }
}
function attempt<T>(read: () => T): ParseResult<T> {
  try {
    return { ok: true, value: read() };
  } catch (error) {
    return {
      ok: false,
      violation: error instanceof Error ? error.message : String(error),
    };
  }
}

function readEvent(value: unknown, where: string): ActivityEvent {
  const body = object(value, where);
  const details = body.details;
  return {
    id: text(body, 'id'),
    sequence: number(body, 'sequence'),
    createdAt: instant(body, 'created_at'),
    category: text(body, 'category'),
    component: text(body, 'component'),
    action: text(body, 'action'),
    outcome: text(body, 'outcome'),
    severity: text(body, 'severity'),
    actorKind: text(body, 'actor_kind'),
    actorId: text(body, 'actor_id'),
    subjectType: text(body, 'subject_type'),
    subjectId: text(body, 'subject_id'),
    targetLabel: text(body, 'target_label'),
    operationId: text(body, 'operation_id'),
    applianceId: text(body, 'appliance_id'),
    bootId: text(body, 'boot_id'),
    timeSource: text(body, 'time_source'),
    details:
      details !== null && typeof details === 'object' && !Array.isArray(details)
        ? (details as Record<string, unknown>)
        : {},
  };
}

export function parseActivityPageBody(value: unknown): ParseResult<ActivityPage> {
  return attempt(() => {
    const body = object(value, 'activity page');
    version(body, 'activity page');
    if (!Array.isArray(body.items)) throw new Error('the activity page has no items');
    const windows = object(body.retained_window ?? {}, 'retained window');
    const retainedWindow: Record<string, RetainedWindow> = {};
    for (const [category, raw] of Object.entries(windows)) {
      const window = object(raw, `retained window ${category}`);
      retainedWindow[category] = {
        oldest: optionalInstant(window, 'oldest'),
        newest: optionalInstant(window, 'newest'),
        count: number(window, 'count'),
        truncated: window.truncated === true,
        purgedBefore: optionalInstant(window, 'purged_before'),
        purgeReason: text(window, 'purge_reason'),
      };
    }
    const health = object(body.logging_health ?? {}, 'logging health');
    return {
      items: body.items.map((item, index) =>
        readEvent(item, `activity item ${index + 1}`),
      ),
      nextCursor: text(body, 'next_cursor'),
      total: number(body, 'total'),
      retainedWindow,
      loggingHealth: {
        auditWritable: health.audit_writable !== false,
        failures: number(health, 'failures'),
        lastFailure: text(health, 'last_failure'),
      },
    };
  });
}

export function parseActivityRetentionSettingsBody(
  value: unknown,
): ParseResult<ActivityRetentionSettings> {
  return attempt(() => {
    const body = object(value, 'activity retention settings');
    version(body, 'activity retention settings');
    const days = object(body.retention_days ?? {}, 'retention days');
    const bytes = object(body.quota_bytes ?? {}, 'quota bytes');
    const retentionDays: Record<string, number> = {};
    const quotaBytes: Record<string, number> = {};
    for (const [category, value] of Object.entries(days)) {
      if (typeof value !== 'number') throw new Error('bad retention');
      retentionDays[category] = value;
    }
    for (const [category, value] of Object.entries(bytes)) {
      if (typeof value !== 'number') throw new Error('bad quota');
      quotaBytes[category] = value;
    }
    return { retentionDays, quotaBytes };
  });
}
