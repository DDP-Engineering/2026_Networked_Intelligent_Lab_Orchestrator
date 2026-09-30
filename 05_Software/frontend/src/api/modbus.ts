import type { ParseResult } from './contract';

/** A Core-owned Modbus endpoint, including saved targets that are offline. */
export interface ModbusEndpoint {
  schema_version: number;
  id: string;
  revision: number;
  name: string;
  transport: 'rtu' | 'tcp' | 'rtu_tcp';
  resource_id: string;
  enabled: boolean;
  presence: string;
  readiness: string;
  network?: { host: string; port: number; unit_min: number; unit_max: number };
  serial_identity?: {
    stable_id: string;
    vendor_id: string;
    product_id: string;
    serial_number: string | null;
    bus_path: string;
  };
}

export interface ModbusProfile {
  schema_version: number;
  id: string;
  revision: number;
  owner_id: string;
  name: string;
  endpoint_id: string;
  settings: {
    response_timeout_ms: number;
    connect_timeout_ms: number;
    inter_request_ms: number;
    read_retries: number;
  };
}

export interface ModbusSettingsValues {
  max_sessions: number;
  session_lease_seconds: number;
  idle_seconds: number;
  retention_count: number;
  retention_bytes: number;
  part_bytes: number;
}

export interface ModbusSettingsState {
  revision: number;
  settings: ModbusSettingsValues;
}

export interface ModbusDefinitionSummary {
  id: string;
  name: string;
  revision: number;
  endpoint_id?: string;
}

export interface ModbusSessionStatus {
  session_id?: string;
  state: string;
  endpoint_id: string;
  profile_id?: string;
  connected?: boolean;
  reason?: string;
}

const record = (value: unknown): value is Record<string, unknown> =>
  typeof value === 'object' && value !== null && !Array.isArray(value);
const integer = (value: unknown): value is number => Number.isSafeInteger(value);

const endpoint = (value: unknown): value is ModbusEndpoint => {
  if (!record(value)) return false;
  if (
    value.schema_version !== 1 ||
    typeof value.id !== 'string' ||
    !integer(value.revision) ||
    typeof value.name !== 'string' ||
    !['rtu', 'tcp', 'rtu_tcp'].includes(String(value.transport)) ||
    typeof value.resource_id !== 'string' ||
    typeof value.enabled !== 'boolean' ||
    typeof value.presence !== 'string' ||
    typeof value.readiness !== 'string'
  )
    return false;
  if (value.network !== undefined) {
    if (
      !record(value.network) ||
      typeof value.network.host !== 'string' ||
      !integer(value.network.port) ||
      !integer(value.network.unit_min) ||
      !integer(value.network.unit_max)
    )
      return false;
  }
  if (value.serial_identity !== undefined) {
    if (
      !record(value.serial_identity) ||
      !['stable_id', 'vendor_id', 'product_id', 'bus_path'].every(
        (key) =>
          typeof (value.serial_identity as Record<string, unknown>)[key] === 'string',
      ) ||
      !(
        typeof value.serial_identity.serial_number === 'string' ||
        value.serial_identity.serial_number === null
      )
    )
      return false;
  }
  return true;
};

export function parseModbusEndpoints(value: unknown): ParseResult<ModbusEndpoint[]> {
  const endpoints = record(value) ? value.endpoints : undefined;
  return Array.isArray(endpoints) && endpoints.every(endpoint)
    ? { ok: true, value: endpoints }
    : { ok: false, violation: 'Invalid Modbus endpoint inventory' };
}

const profile = (value: unknown): value is ModbusProfile =>
  record(value) &&
  value.schema_version === 1 &&
  typeof value.id === 'string' &&
  integer(value.revision) &&
  typeof value.owner_id === 'string' &&
  typeof value.name === 'string' &&
  typeof value.endpoint_id === 'string' &&
  record(value.settings) &&
  [
    'response_timeout_ms',
    'connect_timeout_ms',
    'inter_request_ms',
    'read_retries',
  ].every((key) => integer((value.settings as Record<string, unknown>)[key]));

export function parseModbusProfiles(value: unknown): ParseResult<ModbusProfile[]> {
  const profiles = record(value) ? value.profiles : undefined;
  return Array.isArray(profiles) && profiles.every(profile)
    ? { ok: true, value: profiles }
    : { ok: false, violation: 'Invalid Modbus profile inventory' };
}

export function parseModbusEndpoint(value: unknown): ParseResult<ModbusEndpoint> {
  return endpoint(value)
    ? { ok: true, value }
    : { ok: false, violation: 'Invalid Modbus endpoint' };
}

export function parseModbusProfile(value: unknown): ParseResult<ModbusProfile> {
  return profile(value)
    ? { ok: true, value }
    : { ok: false, violation: 'Invalid Modbus profile' };
}

export function parseModbusSettings(value: unknown): ParseResult<ModbusSettingsState> {
  if (
    !record(value) ||
    !integer(value.revision) ||
    !record(value.settings) ||
    ![
      'max_sessions',
      'session_lease_seconds',
      'idle_seconds',
      'retention_count',
      'retention_bytes',
      'part_bytes',
    ].every((key) => integer((value.settings as Record<string, unknown>)[key]))
  ) {
    return { ok: false, violation: 'Invalid Modbus settings' };
  }
  return { ok: true, value: value as unknown as ModbusSettingsState };
}

export function parseModbusDefinitions(field: 'maps' | 'workspaces') {
  return (value: unknown): ParseResult<ModbusDefinitionSummary[]> => {
    const items = record(value) ? value[field] : undefined;
    return Array.isArray(items) &&
      items.every(
        (item) =>
          record(item) &&
          typeof item.id === 'string' &&
          typeof item.name === 'string' &&
          integer(item.revision) &&
          (item.endpoint_id === undefined || typeof item.endpoint_id === 'string'),
      )
      ? { ok: true, value: items as ModbusDefinitionSummary[] }
      : { ok: false, violation: `Invalid Modbus ${field}` };
  };
}

export function parseModbusSession(value: unknown): ParseResult<ModbusSessionStatus> {
  return record(value) &&
    typeof value.state === 'string' &&
    typeof value.endpoint_id === 'string'
    ? { ok: true, value: value as unknown as ModbusSessionStatus }
    : { ok: false, violation: 'Invalid Modbus session status' };
}
