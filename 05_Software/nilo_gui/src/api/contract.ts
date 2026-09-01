/**
 * The versioned Bench Core HTTP contract, in one place.
 *
 * Everything the GUI knows about the wire format lives here: the production
 * client and the development mock both build on these constants and this
 * parser, so a backend contract change cannot leave the two disagreeing.
 */

/** The only API version this GUI understands. */
export const BENCH_CORE_API_VERSION = 'nilo.http/1';

/** Path of the authenticated status resource, relative to the base URL. */
export const STATUS_PATH = '/api/v1/status';

/** Build identity as reported by Bench Core. */
export interface BenchCoreBuild {
  name: string;
  version: string;
  integrationProtocol: string;
}

/** Authenticated status as reported by Bench Core. */
export interface BenchCoreStatus {
  apiVersion: string;
  status: string;
  profile: string;
  simulation: boolean;
  build: BenchCoreBuild;
}

/** The wire shape of a status response body. */
export interface StatusResponseBody {
  api_version: string;
  status: string;
  profile: string;
  simulation: boolean;
  build: {
    name: string;
    version: string;
    integration_protocol: string;
  };
}

/** A parsed status, or the reason the body could not be trusted. */
export type StatusParseResult =
  { ok: true; status: BenchCoreStatus } | { ok: false; violation: string };

/** Join a base URL and the status path without doubling the separator. */
export function statusUrl(baseUrl: string): string {
  return `${normalizeBaseUrl(baseUrl)}${STATUS_PATH}`;
}

/** Drop a trailing separator so joined paths stay canonical. */
export function normalizeBaseUrl(baseUrl: string): string {
  return baseUrl.replace(/\/+$/, '');
}

/**
 * Validate a decoded status body against the supported contract.
 *
 * Returns the violation rather than throwing, so the caller decides how an
 * unusable response is surfaced.
 */
export function parseStatusBody(body: unknown): StatusParseResult {
  if (typeof body !== 'object' || body === null || Array.isArray(body)) {
    return { ok: false, violation: 'the status response body is not a JSON object' };
  }
  const candidate = body as Record<string, unknown>;

  const apiVersion = candidate.api_version;
  if (typeof apiVersion !== 'string') {
    return { ok: false, violation: 'the status response is missing api_version' };
  }
  if (apiVersion !== BENCH_CORE_API_VERSION) {
    return {
      ok: false,
      violation: `the status response declares unsupported API version ${apiVersion}, but this interface understands ${BENCH_CORE_API_VERSION}`,
    };
  }

  for (const field of ['status', 'profile'] as const) {
    if (typeof candidate[field] !== 'string') {
      return { ok: false, violation: `the status response is missing ${field}` };
    }
  }
  if (typeof candidate.simulation !== 'boolean') {
    return { ok: false, violation: 'the status response is missing simulation' };
  }

  const build = candidate.build;
  if (typeof build !== 'object' || build === null || Array.isArray(build)) {
    return { ok: false, violation: 'the status response is missing build' };
  }
  const buildCandidate = build as Record<string, unknown>;
  for (const field of ['name', 'version', 'integration_protocol'] as const) {
    if (typeof buildCandidate[field] !== 'string') {
      return { ok: false, violation: `the status response is missing build.${field}` };
    }
  }

  return {
    ok: true,
    status: {
      apiVersion,
      status: candidate.status as string,
      profile: candidate.profile as string,
      simulation: candidate.simulation,
      build: {
        name: buildCandidate.name as string,
        version: buildCandidate.version as string,
        integrationProtocol: buildCandidate.integration_protocol as string,
      },
    },
  };
}
