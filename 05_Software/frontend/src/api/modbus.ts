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
  owner_id?: string;
  expires_at?: string;
  connected?: boolean;
  reason?: string;
  workspace_id?: string;
  workspace_revision?: number;
  poll_paused?: boolean;
  display_paused?: boolean;
  can_control?: boolean;
  group_stats?: Record<string, ModbusGroupStats>;
  recording_id?: string;
  logging?: boolean;
  logging_mode?: string;
  logging_sampling?: string;
  recording_warning?: string;
}

export interface ModbusGroupStats {
  paused: boolean;
  requested_interval_ms: number;
  actual_interval_ms: number;
  latency_ms: number;
  missed_polls: number;
  errors: number;
  samples: number;
  last_sample_at?: string;
}

export interface ModbusRequest {
  function: number;
  unit: number;
  address: number;
  count: number;
  values: number[];
}

export interface ModbusPollGroup {
  id: string;
  interval_ms: number;
  timeout_ms?: number;
  stop_on_error?: boolean;
  requests: ModbusRequest[];
}

export interface ModbusPoint {
  id: string;
  name: string;
  area: 'coil' | 'discrete' | 'holding' | 'input';
  address: number;
  type: string;
  width: number;
  unit: string;
  access: 'read' | 'write' | 'read_write';
  write_min: number | string | null;
  write_max: number | string | null;
}

export interface ModbusMap {
  id: string;
  revision: number;
  name: string;
  points: ModbusPoint[];
}

export interface ModbusWorkspace {
  schema_version: number;
  id: string;
  revision: number;
  owner_id: string;
  name: string;
  endpoint_id: string;
  profile_ref: { id: string; revision: number };
  map_refs: { id: string; revision: number }[];
  poll_groups: {
    id: string;
    name: string;
    enabled: boolean;
    unit_id: number;
    function: number;
    address: number;
    quantity: number;
    interval_ms: number;
    map_ref: { id: string; revision: number };
    timeout_ms?: number;
    stop_on_error?: boolean;
  }[];
  layout: {
    columns: string[];
    address_base: string;
    series: string[];
    display_paused: boolean;
  };
}

export interface ModbusResult {
  request_id: string;
  outcome: string;
  values: (number | boolean)[];
  attempts: number;
  exception_code?: number;
  frames?: { direction: 'tx' | 'rx'; data_hex: string }[];
}

export interface ModbusRecordingOptions {
  mode: 'values' | 'traffic' | 'both';
  sampling: 'every' | 'interval' | 'change';
  interval_ms?: number;
  name?: string;
  tags?: string[];
}

export interface ModbusRecording {
  id: string;
  session_id: string;
  endpoint_id: string;
  owner_id: string;
  name: string;
  tags: string[];
  mode: ModbusRecordingOptions['mode'];
  sampling: ModbusRecordingOptions['sampling'];
  interval_ms: number;
  status: 'active' | 'completed' | 'incomplete';
  reason?: string;
  started_at: string;
  ended_at?: string;
  last_sequence: number;
  size_bytes: number;
  part_count: number;
}

export interface ModbusRecordingPage {
  recordings: ModbusRecording[];
  next_cursor?: string;
  count?: number;
  no_match?: boolean;
}

export interface ModbusRecordingSearch {
  before?: string;
  name?: string;
  tag?: string;
  endpoint_id?: string;
  started_from?: string;
  started_through?: string;
}

export interface ModbusRecordedEvent {
  sequence: number;
  timestamp: string;
  elapsed_ns: number;
  kind: 'sample' | 'traffic' | 'write';
  group_id?: string;
  request: ModbusRequest;
  result: ModbusResult;
  point_id?: string;
  quality?: string;
  raw?: unknown;
  typed?: unknown;
  request_hex?: string;
  reply_hex?: string;
  command_id?: string;
}

export interface ModbusRecordingEventPage {
  events: ModbusRecordedEvent[];
  next_cursor: number;
  count: number;
  scanned: number;
  no_match: boolean;
  truncated: boolean;
}

export function parseModbusRecording(value: unknown): ParseResult<ModbusRecording> {
  return record(value) &&
    typeof value.id === 'string' &&
    typeof value.session_id === 'string' &&
    typeof value.endpoint_id === 'string' &&
    typeof value.owner_id === 'string' &&
    typeof value.name === 'string' &&
    Array.isArray(value.tags) && value.tags.every((tag) => typeof tag === 'string') &&
    ['active', 'completed', 'incomplete'].includes(String(value.status)) &&
    typeof value.started_at === 'string' &&
    integer(value.last_sequence) && integer(value.size_bytes) && integer(value.part_count)
    ? { ok: true, value: value as unknown as ModbusRecording }
    : { ok: false, violation: 'Invalid Modbus recording' };
}

export function parseModbusRecordingPage(value: unknown): ParseResult<ModbusRecordingPage> {
  if (!record(value) || !Array.isArray(value.recordings) ||
      value.recordings.some((entry) => !parseModbusRecording(entry).ok) ||
      (value.next_cursor !== undefined && typeof value.next_cursor !== 'string'))
    return { ok: false, violation: 'Invalid Modbus recording page' };
  return { ok: true, value: value as unknown as ModbusRecordingPage };
}

export function parseModbusRecordingEvents(value: unknown): ParseResult<ModbusRecordingEventPage> {
  if (!record(value) || !Array.isArray(value.events) || !integer(value.next_cursor) ||
      !integer(value.count) || !integer(value.scanned) ||
      typeof value.no_match !== 'boolean' || typeof value.truncated !== 'boolean' ||
      value.events.some((event) => !record(event) || !integer(event.sequence) ||
        typeof event.timestamp !== 'string' || !integer(event.elapsed_ns) ||
        !['sample', 'traffic', 'write'].includes(String(event.kind)) ||
        !validRequest(event.request) || !validResult(event.result)))
    return { ok: false, violation: 'Invalid Modbus recording event page' };
  return { ok: true, value: value as unknown as ModbusRecordingEventPage };
}

export interface ModbusEvent {
  sequence: number;
  timestamp: string;
  group_id: string;
  request: ModbusRequest;
  result: ModbusResult;
}

export interface ModbusEventPage {
  events: ModbusEvent[];
  next_cursor: number;
  gap: boolean;
}

export interface ModbusDecodedPoint {
  type: string;
  value: unknown;
  raw_words: number[];
  quality: string;
  label?: string;
  set_bits?: string[];
}

export interface ModbusEncodedPoint {
  point_id: string;
  area: string;
  address: number;
  words: number[];
}

export interface ModbusWritePreview {
  session_id: string;
  endpoint_revision: number;
  profile_revision: number;
  request: ModbusRequest;
  pdu_hex: string;
  digest: string;
}

export interface ModbusCommand {
  command_id: string;
  outcome: string;
  result: ModbusResult;
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

const validRequest = (value: unknown): value is ModbusRequest =>
  record(value) &&
  ['function', 'unit', 'address', 'count'].every((key) => integer(value[key])) &&
  Array.isArray(value.values) &&
  value.values.every(integer);
const validResult = (value: unknown): value is ModbusResult =>
  record(value) &&
  typeof value.request_id === 'string' &&
  typeof value.outcome === 'string' &&
  integer(value.attempts) &&
  Array.isArray(value.values) &&
  value.values.every((item) => typeof item === 'boolean' || integer(item));

export function parseModbusWorkspace(value: unknown): ParseResult<ModbusWorkspace> {
  return record(value) &&
    value.schema_version === 1 &&
    typeof value.owner_id === 'string' &&
    typeof value.id === 'string' &&
    integer(value.revision) &&
    typeof value.name === 'string' &&
    typeof value.endpoint_id === 'string' &&
    record(value.profile_ref) &&
    typeof value.profile_ref.id === 'string' &&
    integer(value.profile_ref.revision) &&
    Array.isArray(value.map_refs) &&
    value.map_refs.every(
      (item) => record(item) && typeof item.id === 'string' && integer(item.revision),
    ) &&
    Array.isArray(value.poll_groups) &&
    value.poll_groups.every(
      (item) =>
        record(item) &&
        typeof item.id === 'string' &&
        typeof item.name === 'string' &&
        typeof item.enabled === 'boolean' &&
        ['unit_id', 'function', 'address', 'quantity', 'interval_ms'].every((key) =>
          integer(item[key]),
        ) &&
        record(item.map_ref) &&
        typeof item.map_ref.id === 'string' &&
        integer(item.map_ref.revision),
    ) &&
    record(value.layout) &&
    Array.isArray(value.layout.series)
    ? { ok: true, value: value as unknown as ModbusWorkspace }
    : { ok: false, violation: 'Invalid Modbus workspace' };
}

export function parseModbusMap(value: unknown): ParseResult<ModbusMap> {
  return record(value) &&
    typeof value.id === 'string' &&
    integer(value.revision) &&
    typeof value.name === 'string' &&
    Array.isArray(value.points) &&
    value.points.every(
      (point) =>
        record(point) &&
        typeof point.id === 'string' &&
        typeof point.name === 'string' &&
        typeof point.area === 'string' &&
        integer(point.address) &&
        integer(point.width) &&
        typeof point.type === 'string' &&
        typeof point.access === 'string',
    )
    ? { ok: true, value: value as unknown as ModbusMap }
    : { ok: false, violation: 'Invalid Modbus map' };
}

export function parseModbusEvents(value: unknown): ParseResult<ModbusEventPage> {
  return record(value) &&
    integer(value.next_cursor) &&
    typeof value.gap === 'boolean' &&
    Array.isArray(value.events) &&
    value.events.every(
      (event) =>
        record(event) &&
        integer(event.sequence) &&
        typeof event.timestamp === 'string' &&
        typeof event.group_id === 'string' &&
        validRequest(event.request) &&
        validResult(event.result),
    )
    ? { ok: true, value: value as unknown as ModbusEventPage }
    : { ok: false, violation: 'Invalid Modbus event page' };
}

export function parseModbusResult(value: unknown): ParseResult<ModbusResult> {
  return validResult(value)
    ? { ok: true, value }
    : { ok: false, violation: 'Invalid Modbus result' };
}

export function parseModbusDecodedPoint(
  value: unknown,
): ParseResult<ModbusDecodedPoint> {
  return record(value) &&
    typeof value.type === 'string' &&
    typeof value.quality === 'string' &&
    Array.isArray(value.raw_words) &&
    value.raw_words.every(integer) &&
    'value' in value
    ? { ok: true, value: value as unknown as ModbusDecodedPoint }
    : { ok: false, violation: 'Invalid Modbus decoded point' };
}

export function parseModbusEncodedPoint(
  value: unknown,
): ParseResult<ModbusEncodedPoint> {
  return record(value) &&
    typeof value.point_id === 'string' &&
    typeof value.area === 'string' &&
    integer(value.address) &&
    Array.isArray(value.words) &&
    value.words.every(integer)
    ? { ok: true, value: value as unknown as ModbusEncodedPoint }
    : { ok: false, violation: 'Invalid Modbus encoded point' };
}

export function parseModbusWritePreview(
  value: unknown,
): ParseResult<ModbusWritePreview> {
  return record(value) &&
    typeof value.session_id === 'string' &&
    integer(value.endpoint_revision) &&
    integer(value.profile_revision) &&
    validRequest(value.request) &&
    typeof value.pdu_hex === 'string' &&
    typeof value.digest === 'string'
    ? { ok: true, value: value as unknown as ModbusWritePreview }
    : { ok: false, violation: 'Invalid Modbus write preview' };
}

export function parseModbusCommand(value: unknown): ParseResult<ModbusCommand> {
  return record(value) &&
    typeof value.command_id === 'string' &&
    typeof value.outcome === 'string' &&
    validResult(value.result)
    ? { ok: true, value: value as unknown as ModbusCommand }
    : { ok: false, violation: 'Invalid Modbus command' };
}

export function parseModbusAck(value: unknown): ParseResult<void> {
  return record(value) && value.ok === true
    ? { ok: true, value: undefined }
    : { ok: false, violation: 'Invalid Modbus acknowledgement' };
}
