import { BenchCoreError, type Credential } from './client';
import { CSRF_HEADER_NAME, resourceUrl } from './contract';
import type { InstrumentJob } from './bench';

export interface SerialSettings {
  baud_rate: number;
  data_bits: number;
  parity: string;
  stop_bits: number;
  flow_control: string;
  rts: boolean;
  dtr: boolean;
}
export const defaultSerialSettings: SerialSettings = {
  baud_rate: 115200,
  data_bits: 8,
  parity: 'none',
  stop_bits: 1,
  flow_control: 'none',
  rts: true,
  dtr: true,
};
export interface SerialStatus {
  device_id: string;
  connected: boolean;
  connection_id?: string;
  owner_id?: string;
  expires_at?: string;
  settings?: SerialSettings;
  logging: boolean;
  log_id?: string;
  rx_bytes: number;
  tx_bytes: number;
  warning?: string;
  can_control: boolean;
  signals?: Record<string, boolean | null>;
}
export interface SerialEvent {
  sequence: number;
  timestamp: string;
  elapsed_ms: number;
  direction: 'rx' | 'tx' | 'state' | 'gap' | 'error';
  data_base64: string;
  byte_count: number;
  message?: string;
}
export interface SerialRead {
  events: SerialEvent[];
  next_cursor: number;
  gap: boolean;
}
export interface SerialSend {
  connection_id: string;
  idempotency_key: string;
  encoding: 'ascii' | 'utf8' | 'hex';
  data: string;
  ending: 'none' | 'cr' | 'lf' | 'crlf';
}
export interface SerialSendResult {
  status: string;
  bytes_written: number;
  error?: string;
}
export interface SerialLog {
  id: string;
  device_id: string;
  connection_id: string;
  owner_id: string;
  started_at: string;
  ended_at?: string;
  status: string;
  rx_bytes: number;
  tx_bytes: number;
  size_bytes: number;
}
export interface SerialLimits {
  files: number;
  total_bytes: number;
  file_bytes: number;
}
export interface SerialRetention {
  revision: number;
  limits: SerialLimits;
  usage_bytes: number;
}

/** All serial requests use the application's authenticated origin and CSRF boundary. */
export function serialAPI(baseUrl: string, credential: Credential) {
  const device = (id: string, suffix: string) =>
    `/api/v1/serial-devices/${encodeURIComponent(id)}/${suffix}`;
  const log = (id: string, suffix = '') =>
    `/api/v1/serial-logs/${encodeURIComponent(id)}${suffix}`;
  async function exchange(
    path: string,
    method = 'GET',
    body?: unknown,
    signal?: AbortSignal,
  ) {
    const headers: Record<string, string> = { Accept: 'application/json' };
    if (credential.kind === 'bearer')
      headers.Authorization = `Bearer ${credential.token}`;
    else if (method !== 'GET') headers[CSRF_HEADER_NAME] = credential.csrfToken;
    if (body !== undefined) headers['Content-Type'] = 'application/json';
    const response = await fetch(resourceUrl(baseUrl, path), {
      method,
      headers,
      credentials: 'include',
      signal: signal
        ? AbortSignal.any([signal, AbortSignal.timeout(20000)])
        : AbortSignal.timeout(20000),
      body: body === undefined ? undefined : JSON.stringify(body),
    });
    if (!response.ok) {
      const error = await response.json().catch(() => ({}));
      throw new BenchCoreError(
        response.status === 401
          ? 'unauthorized'
          : response.status === 403
            ? 'forbidden'
            : 'rejected',
        'Serial request failed',
        error.error?.code ?? error.code ?? `HTTP ${response.status}`,
      );
    }
    return response;
  }
  async function request<T>(
    path: string,
    method = 'GET',
    body?: unknown,
    signal?: AbortSignal,
  ): Promise<T> {
    return (await exchange(path, method, body, signal)).json() as Promise<T>;
  }
  return {
    status: (id: string, signal?: AbortSignal) =>
      request<SerialStatus>(device(id, 'workspace'), 'GET', undefined, signal),
    connect: (id: string, revision: number, settings: SerialSettings, key: string) =>
      request<InstrumentJob>(device(id, 'connect'), 'POST', {
        revision,
        settings,
        idempotency_key: key,
        timeout_seconds: 60,
      }),
    read: (id: string, connection: string, after: number, signal?: AbortSignal) =>
      request<SerialRead>(
        device(
          id,
          `read?connection_id=${encodeURIComponent(connection)}&after=${after}&wait_ms=1000`,
        ),
        'GET',
        undefined,
        signal,
      ),
    send: (id: string, input: SerialSend) =>
      request<SerialSendResult>(device(id, 'send'), 'POST', input),
    disconnect: (id: string, connection_id: string) =>
      request(device(id, 'disconnect'), 'POST', { connection_id }),
    signals: (
      id: string,
      connection_id: string,
      values: Partial<Pick<SerialSettings, 'rts' | 'dtr'>>,
    ) =>
      request<SerialStatus>(device(id, 'signals'), 'POST', {
        connection_id,
        ...values,
      }),
    logging: (id: string, connection_id: string, action: 'start' | 'stop') =>
      request<SerialStatus>(device(id, 'logging'), 'POST', { connection_id, action }),
    logs: (id: string, offset: number) =>
      request<{ logs: SerialLog[]; total: number; offset: number }>(
        device(id, `logs?offset=${offset}`),
      ),
    retention: (id: string) => request<SerialRetention>(device(id, 'retention')),
    saveRetention: (id: string, revision: number, limits: SerialLimits) =>
      request<SerialRetention>(device(id, 'retention'), 'PUT', { revision, limits }),
    events: (id: string, after: number) =>
      request<SerialRead>(log(id, `/events?after=${after}`)),
    remove: (id: string) => request(log(id), 'DELETE'),
    // Stream downloads through the browser for cookie sessions, avoiding large in-memory blobs.
    downloadURL: (id: string, format: string) =>
      resourceUrl(baseUrl, log(id, `/content?format=${encodeURIComponent(format)}`)),
  };
}
