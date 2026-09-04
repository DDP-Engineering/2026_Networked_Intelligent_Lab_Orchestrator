import {
  createBenchCoreClient,
  type BenchCoreClient,
  type BenchCoreTransport,
} from '../api/client';
import { normalizeBaseUrl } from '../api/contract';
import { MOCK_SCENARIOS, createMockTransport, parseMockScenario } from '../api/mock';
import type { MockScenario } from '../api/mock';

/**
 * Build-time configuration of the Bench Core endpoint.
 *
 * The reader takes a plain record rather than `import.meta.env` so it can be
 * exercised directly, and so the same source serves a Windows browser talking
 * to a WSL2 backend and an isolated frontend session with no backend at all.
 *
 * The browser never addresses Bench Core directly. A bearer credential is not
 * a CORS-safelisted header, so a cross-origin call would be preflighted, and
 * Bench Core answers no preflight. The client therefore requests origin-
 * relative paths: in development the Vite server forwards them to the endpoint
 * below, and on an appliance Bench Core serves the built assets itself.
 */

/** Whether the GUI calls a real backend or its built-in mock. */
export type ApiMode = 'live' | 'mock';

/** Server-supplied presentation hint; it grants no API permissions. */
export type RuntimeProfile = 'simulation' | 'appliance' | 'unknown';

/** The loopback endpoint a WSL2-hosted development backend binds to. */
export const DEFAULT_API_BASE_URL = 'http://127.0.0.1:8090';

/** Requests are addressed relative to the origin serving the interface. */
export const SAME_ORIGIN_BASE_URL = '';

/** Resolved Bench Core configuration for this build. */
export interface GuiEnvironment {
  apiMode: ApiMode;
  /**
   * The Bench Core endpoint: what the development server forwards to and what
   * the interface displays. It is never fetched from the browser directly.
   */
  apiBaseUrl: string;
  mockScenario: MockScenario;
}

/** The environment shape the reader accepts. */
export type EnvironmentRecord = Record<string, string | undefined>;

/** A declared environment variable that cannot be used as written. */
export class GuiConfigurationError extends Error {
  readonly variable: string;

  constructor(variable: string, detail: string) {
    super(`${variable} is not usable: ${detail}`);
    this.name = 'GuiConfigurationError';
    this.variable = variable;
  }
}

/** Resolve the Bench Core configuration, applying the documented defaults. */
export function readGuiEnvironment(environment: EnvironmentRecord): GuiEnvironment {
  return {
    apiMode: readMode(environment.VITE_NILO_API_MODE),
    apiBaseUrl: readBaseUrl(environment.VITE_NILO_API_BASE_URL),
    mockScenario: readScenario(environment.VITE_NILO_MOCK_SCENARIO),
  };
}

/**
 * Build the client this environment describes.
 *
 * `transport` is an override for tests; production callers omit it and get
 * either the global `fetch` or the contract mock.
 */
export function createConfiguredClient(
  environment: EnvironmentRecord,
  transport?: BenchCoreTransport,
): BenchCoreClient {
  const resolved = readGuiEnvironment(environment);
  return createBenchCoreClient({
    baseUrl: SAME_ORIGIN_BASE_URL,
    transport:
      transport ??
      (resolved.apiMode === 'mock'
        ? createMockTransport(resolved.mockScenario)
        : undefined),
  });
}

function readMode(value: string | undefined): ApiMode {
  const declared = (value ?? '').trim();
  if (declared === '' || declared === 'live') {
    return 'live';
  }
  if (declared === 'mock') {
    return 'mock';
  }
  throw new GuiConfigurationError(
    'VITE_NILO_API_MODE',
    `expected "live" or "mock", but found "${declared}"`,
  );
}

function readBaseUrl(value: string | undefined): string {
  const declared = (value ?? '').trim();
  if (declared === '') {
    return DEFAULT_API_BASE_URL;
  }
  let parsed: URL;
  try {
    parsed = new URL(declared);
  } catch {
    throw new GuiConfigurationError(
      'VITE_NILO_API_BASE_URL',
      `expected an absolute http or https URL, but found "${declared}"`,
    );
  }
  if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') {
    throw new GuiConfigurationError(
      'VITE_NILO_API_BASE_URL',
      `expected an absolute http or https URL, but found "${declared}"`,
    );
  }
  return normalizeBaseUrl(declared);
}

function readScenario(value: string | undefined): MockScenario {
  const declared = (value ?? '').trim();
  if (declared === '') {
    return 'healthy';
  }
  const scenario = parseMockScenario(declared);
  if (scenario === null) {
    throw new GuiConfigurationError(
      'VITE_NILO_MOCK_SCENARIO',
      `expected one of ${MOCK_SCENARIOS.join(', ')}, but found "${declared}"`,
    );
  }
  return scenario;
}
