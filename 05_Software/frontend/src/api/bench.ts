import type { ParseResult } from './contract';

export interface InstrumentDevice {
  id: string;
  connection: string;
  display_name: string;
  driver: string;
  channels: string[];
  samplerates: number[];
  maximum_samples: number;
}
export interface InstrumentPlugin {
  id: string;
  display_name: string;
  version: string;
  installed: boolean;
  enabled: boolean;
  mode: 'ask' | 'auto';
  status: string;
  simulation: boolean;
  operations: string[];
  devices: InstrumentDevice[];
  error: string;
  digest: string;
}
export interface InstrumentSubmission {
  plugin_id: string;
  operation: 'capture' | 'decode_uart';
  device_id: string;
  parameters: Record<string, unknown>;
  idempotency_key: string;
  timeout_seconds: number;
}
export interface InstrumentReply {
  sequence: number;
  transaction_id: string;
  time: string;
  payload: Record<string, unknown>;
}
export interface InstrumentJob {
  transaction_id: string;
  session_id: string;
  request_id: string;
  plugin_id: string;
  owner_id: string;
  state: string;
  submission: InstrumentSubmission;
  created_at: string;
  deadline: string;
  result: Record<string, unknown>;
  replies: InstrumentReply[];
  error: string;
  artifact_id: string;
}
export type PluginAction = 'install' | 'enable' | 'disable' | 'restart' | 'uninstall';
export const isJobTerminal = (job: InstrumentJob) =>
  ['completed', 'failed', 'cancelled', 'incomplete'].includes(job.state);

const record = (value: unknown): value is Record<string, unknown> =>
  typeof value === 'object' && value !== null && !Array.isArray(value);
const strings = (value: unknown): value is string[] =>
  Array.isArray(value) && value.every((item) => typeof item === 'string');
const device = (value: unknown): value is InstrumentDevice =>
  record(value) &&
  ['id', 'connection', 'display_name', 'driver'].every(
    (key) => typeof value[key] === 'string',
  ) &&
  strings(value.channels) &&
  value.channels.every((channel) => /^D([0-9]|1[0-5])$/.test(channel)) &&
  Array.isArray(value.samplerates) &&
  value.samplerates.every(Number.isSafeInteger) &&
  typeof value.maximum_samples === 'number';
const plugin = (value: unknown): value is InstrumentPlugin =>
  record(value) &&
  ['id', 'display_name', 'version', 'status', 'error', 'digest'].every(
    (key) => typeof value[key] === 'string',
  ) &&
  ['installed', 'enabled', 'simulation'].every(
    (key) => typeof value[key] === 'boolean',
  ) &&
  (value.mode === 'ask' || value.mode === 'auto') &&
  strings(value.operations) &&
  Array.isArray(value.devices) &&
  value.devices.every(device);
const job = (value: unknown): value is InstrumentJob =>
  record(value) &&
  [
    'transaction_id',
    'session_id',
    'request_id',
    'plugin_id',
    'owner_id',
    'state',
    'created_at',
    'deadline',
    'error',
    'artifact_id',
  ].every((key) => typeof value[key] === 'string') &&
  record(value.submission) &&
  record(value.result) &&
  Array.isArray(value.replies) &&
  value.replies.every(
    (reply: unknown) =>
      record(reply) &&
      Number.isSafeInteger(reply.sequence) &&
      reply.transaction_id === value.transaction_id &&
      typeof reply.time === 'string' &&
      record(reply.payload),
  );

const parsed = <T>(
  value: unknown,
  check: (value: unknown) => value is T,
): ParseResult<T> =>
  check(value)
    ? { ok: true, value }
    : { ok: false, violation: 'Invalid instrument response' };
export const parsePlugin = (value: unknown) => parsed(value, plugin);
export const parseInstrumentJob = (value: unknown) => parsed(value, job);
export function parsePlugins(value: unknown): ParseResult<InstrumentPlugin[]> {
  return record(value)
    ? parsed(
        value.plugins,
        (v): v is InstrumentPlugin[] => Array.isArray(v) && v.every(plugin),
      )
    : { ok: false, violation: 'Invalid plugin inventory' };
}
export function parseInstrumentDevices(
  value: unknown,
): ParseResult<InstrumentDevice[]> {
  return record(value)
    ? parsed(
        value.devices,
        (v): v is InstrumentDevice[] => Array.isArray(v) && v.every(device),
      )
    : { ok: false, violation: 'Invalid instrument inventory' };
}
export function parseInstrumentJobs(value: unknown): ParseResult<InstrumentJob[]> {
  return record(value)
    ? parsed(value.jobs, (v): v is InstrumentJob[] => Array.isArray(v) && v.every(job))
    : { ok: false, violation: 'Invalid transaction list' };
}
