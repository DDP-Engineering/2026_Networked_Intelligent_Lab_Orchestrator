/**
 * The versioned Bench Core HTTP contract, in one place.
 *
 * Everything the GUI knows about the wire format lives here: the production
 * client and the development mock both build on these constants and these
 * parsers, so a backend contract change cannot leave the two disagreeing.
 *
 * Every path is origin-relative. A bearer credential is not CORS-safelisted, so
 * an absolute URL here would turn each authenticated request into a preflight
 * Bench Core does not answer.
 */

/** The only API version this GUI understands. */
export const BENCH_CORE_API_VERSION = 'nilo.http/1';

/** Path of the authenticated status resource, relative to the base URL. */
export const STATUS_PATH = '/api/v1/status';

/** Path of the authenticated integration inventory, relative to the base URL. */
export const INTEGRATIONS_PATH = '/api/v1/integrations';

/** Path of the authenticated device inventory, relative to the base URL. */
export const DEVICES_PATH = '/api/v1/devices';

/** Every resource path this interface requests, in documentation order. */
export const BENCH_CORE_PATHS = [STATUS_PATH, INTEGRATIONS_PATH, DEVICES_PATH] as const;

/** One of the paths above. */
export type BenchCorePath = (typeof BENCH_CORE_PATHS)[number];

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

/** One integration the simulation backend offers. */
export interface BenchCoreIntegration {
  id: string;
  displayName: string;
  status: string;
  simulation: boolean;
  operations: string[];
}

/** One device the simulation backend offers, and the integration owning it. */
export interface BenchCoreDevice {
  id: string;
  integrationId: string;
  displayName: string;
  kind: string;
  status: string;
  simulation: boolean;
  capabilities: string[];
}

/** The integration inventory for one deterministic simulation scenario. */
export interface IntegrationInventory {
  apiVersion: string;
  scenario: string;
  items: BenchCoreIntegration[];
}

/** The device inventory for one deterministic simulation scenario. */
export interface DeviceInventory {
  apiVersion: string;
  scenario: string;
  items: BenchCoreDevice[];
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

/** The wire shape of an integration inventory response body. */
export interface IntegrationsResponseBody {
  api_version: string;
  scenario: string;
  items: {
    id: string;
    display_name: string;
    status: string;
    simulation: boolean;
    operations: string[];
  }[];
}

/** The wire shape of a device inventory response body. */
export interface DevicesResponseBody {
  api_version: string;
  scenario: string;
  items: {
    id: string;
    integration_id: string;
    display_name: string;
    kind: string;
    status: string;
    simulation: boolean;
    capabilities: string[];
  }[];
}

/** A parsed value, or the reason the body could not be trusted. */
export type ParseResult<T> = { ok: true; value: T } | { ok: false; violation: string };

/** Join a base URL and a resource path without doubling the separator. */
export function resourceUrl(baseUrl: string, path: BenchCorePath): string {
  return `${normalizeBaseUrl(baseUrl)}${path}`;
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
export function parseStatusBody(body: unknown): ParseResult<BenchCoreStatus> {
  const envelope = readObject(body, 'status response');
  if (!envelope.ok) {
    return envelope;
  }
  const candidate = envelope.value;

  const apiVersion = readApiVersion(candidate, 'status response');
  if (!apiVersion.ok) {
    return apiVersion;
  }

  const status = readString(candidate, 'status', 'status response');
  if (!status.ok) {
    return status;
  }
  const profile = readString(candidate, 'profile', 'status response');
  if (!profile.ok) {
    return profile;
  }
  const simulation = readBoolean(candidate, 'simulation', 'status response');
  if (!simulation.ok) {
    return simulation;
  }

  const build = readObject(candidate.build, 'status response build');
  if (!build.ok) {
    return { ok: false, violation: 'the status response is missing build' };
  }
  const name = readString(build.value, 'name', 'status response build');
  if (!name.ok) {
    return name;
  }
  const version = readString(build.value, 'version', 'status response build');
  if (!version.ok) {
    return version;
  }
  const protocol = readString(
    build.value,
    'integration_protocol',
    'status response build',
  );
  if (!protocol.ok) {
    return protocol;
  }

  return {
    ok: true,
    value: {
      apiVersion: apiVersion.value,
      status: status.value,
      profile: profile.value,
      simulation: simulation.value,
      build: {
        name: name.value,
        version: version.value,
        integrationProtocol: protocol.value,
      },
    },
  };
}

/** Validate a decoded integration inventory against the supported contract. */
export function parseIntegrationsBody(
  body: unknown,
): ParseResult<IntegrationInventory> {
  return parseInventory(body, 'integration inventory', (entry, where) => {
    const id = readString(entry, 'id', where);
    if (!id.ok) {
      return id;
    }
    const displayName = readString(entry, 'display_name', where);
    if (!displayName.ok) {
      return displayName;
    }
    const status = readString(entry, 'status', where);
    if (!status.ok) {
      return status;
    }
    const simulation = readBoolean(entry, 'simulation', where);
    if (!simulation.ok) {
      return simulation;
    }
    const operations = readStringArray(entry, 'operations', where);
    if (!operations.ok) {
      return operations;
    }
    return {
      ok: true,
      value: {
        id: id.value,
        displayName: displayName.value,
        status: status.value,
        simulation: simulation.value,
        operations: operations.value,
      },
    };
  });
}

/** Validate a decoded device inventory against the supported contract. */
export function parseDevicesBody(body: unknown): ParseResult<DeviceInventory> {
  return parseInventory(body, 'device inventory', (entry, where) => {
    const id = readString(entry, 'id', where);
    if (!id.ok) {
      return id;
    }
    const integrationId = readString(entry, 'integration_id', where);
    if (!integrationId.ok) {
      return integrationId;
    }
    const displayName = readString(entry, 'display_name', where);
    if (!displayName.ok) {
      return displayName;
    }
    const kind = readString(entry, 'kind', where);
    if (!kind.ok) {
      return kind;
    }
    const status = readString(entry, 'status', where);
    if (!status.ok) {
      return status;
    }
    const simulation = readBoolean(entry, 'simulation', where);
    if (!simulation.ok) {
      return simulation;
    }
    const capabilities = readStringArray(entry, 'capabilities', where);
    if (!capabilities.ok) {
      return capabilities;
    }
    return {
      ok: true,
      value: {
        id: id.value,
        integrationId: integrationId.value,
        displayName: displayName.value,
        kind: kind.value,
        status: status.value,
        simulation: simulation.value,
        capabilities: capabilities.value,
      },
    };
  });
}

/**
 * Both inventories share one envelope — version, scenario, and an ordered item
 * array — so only the per-entry reader differs between them.
 */
function parseInventory<T>(
  body: unknown,
  resource: string,
  readEntry: (entry: Record<string, unknown>, where: string) => ParseResult<T>,
): ParseResult<{ apiVersion: string; scenario: string; items: T[] }> {
  const envelope = readObject(body, `${resource} response`);
  if (!envelope.ok) {
    return envelope;
  }
  const candidate = envelope.value;

  const apiVersion = readApiVersion(candidate, `${resource} response`);
  if (!apiVersion.ok) {
    return apiVersion;
  }
  const scenario = readString(candidate, 'scenario', `${resource} response`);
  if (!scenario.ok) {
    return scenario;
  }
  if (!Array.isArray(candidate.items)) {
    return {
      ok: false,
      violation: `the ${resource} response is missing items, or items is not a list`,
    };
  }

  const items: T[] = [];
  for (const [index, raw] of candidate.items.entries()) {
    const where = `${resource} entry ${index}`;
    const entry = readObject(raw, where);
    if (!entry.ok) {
      return entry;
    }
    const parsed = readEntry(entry.value, where);
    if (!parsed.ok) {
      return parsed;
    }
    items.push(parsed.value);
  }

  return {
    ok: true,
    value: { apiVersion: apiVersion.value, scenario: scenario.value, items },
  };
}

function readApiVersion(
  candidate: Record<string, unknown>,
  where: string,
): ParseResult<string> {
  const apiVersion = readString(candidate, 'api_version', where);
  if (!apiVersion.ok) {
    return apiVersion;
  }
  if (apiVersion.value !== BENCH_CORE_API_VERSION) {
    return {
      ok: false,
      violation: `the ${where} declares unsupported API version ${apiVersion.value}, but this interface understands ${BENCH_CORE_API_VERSION}`,
    };
  }
  return apiVersion;
}

function readObject(
  body: unknown,
  where: string,
): ParseResult<Record<string, unknown>> {
  if (typeof body !== 'object' || body === null || Array.isArray(body)) {
    return { ok: false, violation: `the ${where} is not a JSON object` };
  }
  return { ok: true, value: body as Record<string, unknown> };
}

function readString(
  candidate: Record<string, unknown>,
  field: string,
  where: string,
): ParseResult<string> {
  const value = candidate[field];
  if (typeof value !== 'string') {
    return { ok: false, violation: `the ${where} is missing ${field}` };
  }
  return { ok: true, value };
}

function readBoolean(
  candidate: Record<string, unknown>,
  field: string,
  where: string,
): ParseResult<boolean> {
  const value = candidate[field];
  if (typeof value !== 'boolean') {
    return { ok: false, violation: `the ${where} is missing ${field}` };
  }
  return { ok: true, value };
}

function readStringArray(
  candidate: Record<string, unknown>,
  field: string,
  where: string,
): ParseResult<string[]> {
  const value = candidate[field];
  if (!Array.isArray(value) || value.some((entry) => typeof entry !== 'string')) {
    return {
      ok: false,
      violation: `the ${where} is missing ${field}, or ${field} is not a list of names`,
    };
  }
  return { ok: true, value: value as string[] };
}
