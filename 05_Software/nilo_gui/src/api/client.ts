import {
  DEVICES_PATH,
  INTEGRATIONS_PATH,
  STATUS_PATH,
  normalizeBaseUrl,
  parseDevicesBody,
  parseIntegrationsBody,
  parseStatusBody,
  resourceUrl,
  type BenchCorePath,
  type BenchCoreStatus,
  type DeviceInventory,
  type IntegrationInventory,
  type ParseResult,
} from './contract';

/** The subset of a fetch response this client depends on. */
export interface TransportResponse {
  readonly status: number;
  readonly ok: boolean;
  text(): Promise<string>;
}

/** The subset of a fetch request this client sends. */
export interface TransportRequest {
  headers: Record<string, string>;
}

/**
 * A fetch-shaped function. The global `fetch` satisfies it, and so does the
 * development mock, which is what lets the GUI run with no backend.
 */
export type BenchCoreTransport = (
  url: string,
  request: TransportRequest,
) => Promise<TransportResponse>;

/** Why a Bench Core request could not be completed. */
export type BenchCoreFailure = 'unauthorized' | 'unavailable' | 'malformed';

/** A Bench Core request failure with a machine-readable cause. */
export class BenchCoreError extends Error {
  readonly failure: BenchCoreFailure;
  readonly detail: string;

  constructor(failure: BenchCoreFailure, detail: string) {
    super(`bench core request failed: ${failure}: ${detail}`);
    this.name = 'BenchCoreError';
    this.failure = failure;
    this.detail = detail;
  }
}

/** The authenticated Bench Core operations the GUI can perform. */
export interface BenchCoreClient {
  readonly baseUrl: string;
  getStatus(token: string): Promise<BenchCoreStatus>;
  getIntegrations(token: string): Promise<IntegrationInventory>;
  getDevices(token: string): Promise<DeviceInventory>;
}

export interface BenchCoreClientOptions {
  baseUrl: string;
  transport?: BenchCoreTransport;
}

/** Build a client for one Bench Core base URL. */
export function createBenchCoreClient(
  options: BenchCoreClientOptions,
): BenchCoreClient {
  const baseUrl = normalizeBaseUrl(options.baseUrl);
  const transport = options.transport ?? defaultTransport;

  /**
   * One request path for every resource, so a rejected token, an unreachable
   * backend, and a body outside the contract are classified identically
   * whichever resource was asked for. The token is only ever an Authorization
   * header value; it never reaches the URL.
   */
  async function request<T>(
    path: BenchCorePath,
    token: string,
    parse: (body: unknown) => ParseResult<T>,
  ): Promise<T> {
    const url = resourceUrl(baseUrl, path);

    let response: TransportResponse;
    try {
      response = await transport(url, {
        headers: {
          Accept: 'application/json',
          Authorization: `Bearer ${token}`,
        },
      });
    } catch (error) {
      throw new BenchCoreError('unavailable', describe(error));
    }

    if (response.status === 401) {
      throw new BenchCoreError(
        'unauthorized',
        'Bench Core rejected the bearer token (HTTP 401)',
      );
    }
    if (!response.ok) {
      throw new BenchCoreError(
        'unavailable',
        `Bench Core answered with HTTP ${response.status}`,
      );
    }

    let body: unknown;
    try {
      body = JSON.parse(await response.text());
    } catch (error) {
      throw new BenchCoreError(
        'malformed',
        `the response to ${path} is not valid JSON: ${describe(error)}`,
      );
    }

    const parsed = parse(body);
    if (!parsed.ok) {
      throw new BenchCoreError('malformed', parsed.violation);
    }
    return parsed.value;
  }

  return {
    baseUrl,
    getStatus: (token) => request(STATUS_PATH, token, parseStatusBody),
    getIntegrations: (token) =>
      request(INTEGRATIONS_PATH, token, parseIntegrationsBody),
    getDevices: (token) => request(DEVICES_PATH, token, parseDevicesBody),
  };
}

function defaultTransport(
  url: string,
  request: TransportRequest,
): Promise<TransportResponse> {
  return fetch(url, request);
}

function describe(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}
