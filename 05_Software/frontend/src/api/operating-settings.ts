import type { ParseResult } from './contract';

export const OPERATING_SETTINGS_PATH = '/api/v1/platform/operating-settings';

export const OPERATING_FIELDS = [
  'executing_jobs',
  'outstanding_jobs',
  'serial_connections',
  'camera_streams',
  'default_job_timeout_seconds',
  'max_job_timeout_seconds',
  'serial_lifetime_minutes',
  'serial_idle_minutes',
  'camera_lifetime_minutes',
] as const;

export type OperatingField = (typeof OPERATING_FIELDS)[number];
export type OperatingSettingsValues = Record<OperatingField, number>;
export type OperatingUsage = Pick<
  OperatingSettingsValues,
  'executing_jobs' | 'outstanding_jobs' | 'serial_connections' | 'camera_streams'
>;
export interface OperatingSettingsState {
  revision: number;
  settings: OperatingSettingsValues;
  usage: OperatingUsage;
}

export const OPERATING_BOUNDS: Record<
  OperatingField,
  {
    min: number;
    max: number;
    unit: 'jobs' | 'connections' | 'streams' | 'seconds' | 'minutes';
  }
> = {
  executing_jobs: { min: 1, max: 8, unit: 'jobs' },
  outstanding_jobs: { min: 1, max: 256, unit: 'jobs' },
  serial_connections: { min: 1, max: 16, unit: 'connections' },
  camera_streams: { min: 1, max: 8, unit: 'streams' },
  default_job_timeout_seconds: { min: 1, max: 3600, unit: 'seconds' },
  max_job_timeout_seconds: { min: 1, max: 3600, unit: 'seconds' },
  serial_lifetime_minutes: { min: 1, max: 1440, unit: 'minutes' },
  serial_idle_minutes: { min: 1, max: 60, unit: 'minutes' },
  camera_lifetime_minutes: { min: 1, max: 240, unit: 'minutes' },
};

function record(value: unknown): Record<string, unknown> | null {
  return value !== null && typeof value === 'object' && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : null;
}

export function parseOperatingSettingsBody(
  body: unknown,
): ParseResult<OperatingSettingsState> {
  const envelope = record(body);
  const settings = record(envelope?.settings);
  const usage = record(envelope?.usage);
  if (
    !envelope ||
    !settings ||
    !usage ||
    !Number.isSafeInteger(envelope.revision) ||
    Number(envelope.revision) < 1
  ) {
    return { ok: false, violation: 'invalid operating settings envelope' };
  }
  const parsed = {} as OperatingSettingsValues;
  for (const field of OPERATING_FIELDS) {
    const value = settings[field];
    if (!Number.isSafeInteger(value)) {
      return { ok: false, violation: `invalid operating settings field ${field}` };
    }
    parsed[field] = value as number;
  }
  const active = {} as OperatingUsage;
  for (const field of OPERATING_FIELDS.slice(0, 4) as (keyof OperatingUsage)[]) {
    const value = usage[field];
    if (!Number.isSafeInteger(value) || Number(value) < 0) {
      return { ok: false, violation: `invalid operating usage field ${field}` };
    }
    active[field] = value as number;
  }
  return {
    ok: true,
    value: { revision: envelope.revision as number, settings: parsed, usage: active },
  };
}
