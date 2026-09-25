import { BENCH_CORE_API_VERSION, type ParseResult } from './contract';

export const APPLIANCE_PATH = '/api/v1/appliance';

export interface ApplianceMetadata {
  apiVersion?: string;
  id: string;
  serialName: string;
  friendlyName: string;
  state: 'provisioning' | 'commissioned';
  version: string;
  managementUrl: string;
  managementAddresses: string[];
  managementMacs: string[];
}

export type IdentifyResult =
  { supported: true; activeUntil: string } | { supported: false; reason: string };

function record(value: unknown): Record<string, unknown> | null {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : null;
}

function text(body: Record<string, unknown>, key: string): string | null {
  return typeof body[key] === 'string' ? body[key] : null;
}

function strings(body: Record<string, unknown>, key: string): string[] | null {
  const value = body[key];
  return Array.isArray(value) && value.every((item) => typeof item === 'string')
    ? value
    : null;
}

export function parseApplianceMetadata(value: unknown): ParseResult<ApplianceMetadata> {
  const body = record(value);
  if (body === null || text(body, 'api_version') !== BENCH_CORE_API_VERSION) {
    return {
      ok: false,
      violation: 'appliance response has an unsupported API version',
    };
  }
  const state = text(body, 'state');
  const addresses = strings(body, 'management_addresses');
  const macs = strings(body, 'management_macs');
  const required = ['id', 'serial_name', 'version', 'management_url'] as const;
  if (
    !required.every((key) => text(body, key) !== null) ||
    (state !== 'provisioning' && state !== 'commissioned') ||
    addresses === null ||
    macs === null
  ) {
    return { ok: false, violation: 'appliance response is malformed' };
  }
  return {
    ok: true,
    value: {
      apiVersion: BENCH_CORE_API_VERSION,
      id: text(body, 'id')!,
      serialName: text(body, 'serial_name')!,
      friendlyName: text(body, 'friendly_name') ?? '',
      state,
      version: text(body, 'version')!,
      managementUrl: text(body, 'management_url')!,
      managementAddresses: addresses,
      managementMacs: macs,
    },
  };
}

export function parseIdentifyResult(value: unknown): ParseResult<IdentifyResult> {
  const body = record(value);
  if (body === null || text(body, 'api_version') !== BENCH_CORE_API_VERSION) {
    return { ok: false, violation: 'identify response is malformed' };
  }
  if (body.supported === false && text(body, 'reason') !== null) {
    return { ok: true, value: { supported: false, reason: text(body, 'reason')! } };
  }
  const activeUntil = text(body, 'active_until');
  if (
    body.supported !== true ||
    activeUntil === null ||
    Number.isNaN(Date.parse(activeUntil))
  ) {
    return { ok: false, violation: 'identify response is malformed' };
  }
  return { ok: true, value: { supported: true, activeUntil } };
}
