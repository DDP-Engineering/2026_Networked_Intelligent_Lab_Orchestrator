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

/** Every bench resource path this interface requests, in documentation order. */
export const BENCH_CORE_PATHS = [STATUS_PATH, INTEGRATIONS_PATH, DEVICES_PATH] as const;

/** One of the bench resource paths above. */
export type BenchCorePath = (typeof BENCH_CORE_PATHS)[number];

/** Path that exchanges a username and password for a browser session. */
export const SIGN_IN_PATH = '/api/v1/auth/login';

/** Path that invalidates the current browser session server-side. */
export const SIGN_OUT_PATH = '/api/v1/auth/logout';

/** Path that reports who the presented credential belongs to. */
export const SESSION_PATH = '/api/v1/auth/session';

/** Path of the identity administration collection. */
export const IDENTITIES_PATH = '/api/v1/users';

/** Path of the API token administration collection. */
export const TOKENS_PATH = '/api/v1/tokens';

/** Name of the readable cookie carrying the session's CSRF token. */
export const CSRF_COOKIE_NAME = 'nilo_csrf';

/** Header a cookie-authenticated state-changing request repeats it in. */
export const CSRF_HEADER_NAME = 'X-NILO-CSRF';

/** The three fixed NILO roles, in increasing order of authority. */
export const BENCH_CORE_ROLES = ['viewer', 'operator', 'administrator'] as const;

/** One of the three fixed roles. */
export type BenchCoreRole = (typeof BENCH_CORE_ROLES)[number];

/**
 * Every permission the access model defines, in the order the token form
 * offers them. This list is compiled in rather than fetched because a scope
 * checkbox has to exist before any token does.
 */
export const BENCH_CORE_PERMISSIONS = [
  'status.read',
  'inventory.read',
  'tokens.read.self',
  'tokens.manage.self',
  'users.read',
  'users.manage',
  'tokens.read.all',
  'tokens.manage.all',
] as const;

/** One of the permissions above. */
export type BenchCorePermission = (typeof BENCH_CORE_PERMISSIONS)[number];

/** Permission required to read the identity listing. */
export const PERMISSION_IDENTITIES_READ: BenchCorePermission = 'users.read';

/** Permission required to change identities. */
export const PERMISSION_IDENTITIES_MANAGE: BenchCorePermission = 'users.manage';

/** Permission required to read every identity's API tokens. */
export const PERMISSION_TOKENS_READ: BenchCorePermission = 'tokens.read.all';

/** Permission required to create and revoke any identity's API tokens. */
export const PERMISSION_TOKENS_MANAGE: BenchCorePermission = 'tokens.manage.all';

/** Who the presented credential belongs to, and what it may ask for. */
export interface BenchCoreSession {
  apiVersion: string;
  username: string;
  role: BenchCoreRole;
  permissions: string[];
  /** Whether the credential was a browser session or a bearer token. */
  credential: string;
  /**
   * The cross-site request forgery token, present only in the answer to a
   * sign-in. A restored session reads it from the readable cookie instead.
   */
  csrfToken: string | null;
}

/** One identity as the administration interface reports it. */
export interface BenchCoreIdentity {
  id: string;
  username: string;
  role: BenchCoreRole;
  enabled: boolean;
  createdAt: string;
}

/** Every identity Bench Core stores. */
export interface IdentityInventory {
  apiVersion: string;
  items: BenchCoreIdentity[];
}

/** One API token's safe metadata. It has no field a secret could occupy. */
export interface BenchCoreToken {
  id: string;
  name: string;
  owner: string;
  scopes: string[];
  createdAt: string;
  expiresAt: string | null;
  revoked: boolean;
  revokedAt: string | null;
}

/** Every API token Bench Core stores. */
export interface TokenInventory {
  apiVersion: string;
  items: BenchCoreToken[];
}

/** A newly created token, with the one disclosure of its secret. */
export interface IssuedToken {
  token: BenchCoreToken;
  secret: string;
}

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
export function resourceUrl(baseUrl: string, path: string): string {
  return `${normalizeBaseUrl(baseUrl)}${path}`;
}

/**
 * Build the path of one identity's administration sub-resource.
 *
 * Usernames are lower case and limited to letters, digits, `-`, `_` and `.`,
 * so they need no escaping; encoding anyway costs nothing and means a future
 * widening of the accepted form cannot silently produce a wrong URL.
 */
export function identityPath(username: string, action: string): string {
  return `${IDENTITIES_PATH}/${encodeURIComponent(username)}/${action}`;
}

/** Build the path that revokes one API token. */
export function tokenRevocationPath(id: string): string {
  return `${TOKENS_PATH}/${encodeURIComponent(id)}/revoke`;
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

/** Validate a decoded sign-in or current-session body. */
export function parseSessionBody(body: unknown): ParseResult<BenchCoreSession> {
  const envelope = readObject(body, 'session response');
  if (!envelope.ok) {
    return envelope;
  }
  const candidate = envelope.value;

  const apiVersion = readApiVersion(candidate, 'session response');
  if (!apiVersion.ok) {
    return apiVersion;
  }
  const username = readString(candidate, 'username', 'session response');
  if (!username.ok) {
    return username;
  }
  const role = readRole(candidate, 'session response');
  if (!role.ok) {
    return role;
  }
  const permissions = readStringArray(candidate, 'permissions', 'session response');
  if (!permissions.ok) {
    return permissions;
  }
  const credential = readString(candidate, 'credential', 'session response');
  if (!credential.ok) {
    return credential;
  }

  return {
    ok: true,
    value: {
      apiVersion: apiVersion.value,
      username: username.value,
      role: role.value,
      permissions: permissions.value,
      credential: credential.value,
      // Absent on a restored session: the token was issued once, at sign-in,
      // and afterwards only the readable cookie carries it.
      csrfToken: typeof candidate.csrf_token === 'string' ? candidate.csrf_token : null,
    },
  };
}

/** Validate a decoded identity listing. */
export function parseIdentitiesBody(body: unknown): ParseResult<IdentityInventory> {
  return parseCollection(body, 'identity listing', readIdentityEntry);
}

/** Validate a decoded identity, as the creation answer returns one. */
export function parseIdentityBody(body: unknown): ParseResult<BenchCoreIdentity> {
  const envelope = readObject(body, 'identity response');
  if (!envelope.ok) {
    return envelope;
  }
  const apiVersion = readApiVersion(envelope.value, 'identity response');
  if (!apiVersion.ok) {
    return apiVersion;
  }
  return readIdentityEntry(envelope.value, 'identity response');
}

/** Validate a decoded token listing. */
export function parseTokensBody(body: unknown): ParseResult<TokenInventory> {
  return parseCollection(body, 'token listing', readTokenEntry);
}

/** Validate a decoded token creation answer, secret included. */
export function parseIssuedTokenBody(body: unknown): ParseResult<IssuedToken> {
  const envelope = readObject(body, 'created token response');
  if (!envelope.ok) {
    return envelope;
  }
  const apiVersion = readApiVersion(envelope.value, 'created token response');
  if (!apiVersion.ok) {
    return apiVersion;
  }
  const nested = readObject(envelope.value.token, 'created token response token');
  if (!nested.ok) {
    return { ok: false, violation: 'the created token response is missing token' };
  }
  const token = readTokenEntry(nested.value, 'created token response token');
  if (!token.ok) {
    return token;
  }
  const secret = readString(envelope.value, 'secret', 'created token response');
  if (!secret.ok) {
    return secret;
  }
  return { ok: true, value: { token: token.value, secret: secret.value } };
}

function readIdentityEntry(
  entry: Record<string, unknown>,
  where: string,
): ParseResult<BenchCoreIdentity> {
  const id = readString(entry, 'id', where);
  if (!id.ok) {
    return id;
  }
  const username = readString(entry, 'username', where);
  if (!username.ok) {
    return username;
  }
  const role = readRole(entry, where);
  if (!role.ok) {
    return role;
  }
  const enabled = readBoolean(entry, 'enabled', where);
  if (!enabled.ok) {
    return enabled;
  }
  const createdAt = readString(entry, 'created_at', where);
  if (!createdAt.ok) {
    return createdAt;
  }
  return {
    ok: true,
    value: {
      id: id.value,
      username: username.value,
      role: role.value,
      enabled: enabled.value,
      createdAt: createdAt.value,
    },
  };
}

function readTokenEntry(
  entry: Record<string, unknown>,
  where: string,
): ParseResult<BenchCoreToken> {
  const id = readString(entry, 'id', where);
  if (!id.ok) {
    return id;
  }
  const name = readString(entry, 'name', where);
  if (!name.ok) {
    return name;
  }
  const owner = readString(entry, 'owner', where);
  if (!owner.ok) {
    return owner;
  }
  const scopes = readStringArray(entry, 'scopes', where);
  if (!scopes.ok) {
    return scopes;
  }
  const createdAt = readString(entry, 'created_at', where);
  if (!createdAt.ok) {
    return createdAt;
  }
  const expiresAt = readNullableString(entry, 'expires_at', where);
  if (!expiresAt.ok) {
    return expiresAt;
  }
  const revoked = readBoolean(entry, 'revoked', where);
  if (!revoked.ok) {
    return revoked;
  }
  const revokedAt = readNullableString(entry, 'revoked_at', where);
  if (!revokedAt.ok) {
    return revokedAt;
  }
  return {
    ok: true,
    value: {
      id: id.value,
      name: name.value,
      owner: owner.value,
      scopes: scopes.value,
      createdAt: createdAt.value,
      expiresAt: expiresAt.value,
      revoked: revoked.value,
      revokedAt: revokedAt.value,
    },
  };
}

/**
 * The administration listings share an envelope of a version and an ordered
 * item array. They carry no scenario: identities and tokens are real state,
 * not a deterministic simulation catalog.
 */
function parseCollection<T>(
  body: unknown,
  resource: string,
  readEntry: (entry: Record<string, unknown>, where: string) => ParseResult<T>,
): ParseResult<{ apiVersion: string; items: T[] }> {
  const envelope = readObject(body, `${resource} response`);
  if (!envelope.ok) {
    return envelope;
  }
  const apiVersion = readApiVersion(envelope.value, `${resource} response`);
  if (!apiVersion.ok) {
    return apiVersion;
  }
  if (!Array.isArray(envelope.value.items)) {
    return {
      ok: false,
      violation: `the ${resource} response is missing items, or items is not a list`,
    };
  }
  const items: T[] = [];
  for (const [index, raw] of envelope.value.items.entries()) {
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
  return { ok: true, value: { apiVersion: apiVersion.value, items } };
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

/**
 * Read a field the backend sends as a string or as an explicit null. An
 * absent field is accepted as null too, so a token that never expires reads
 * the same whether the encoder omitted the field or wrote a null.
 */
function readNullableString(
  candidate: Record<string, unknown>,
  field: string,
  where: string,
): ParseResult<string | null> {
  const value = candidate[field];
  if (value === null || value === undefined) {
    return { ok: true, value: null };
  }
  if (typeof value !== 'string') {
    return { ok: false, violation: `the ${where} has a ${field} that is not a time` };
  }
  return { ok: true, value };
}

/** Read the `role` field and refuse anything outside the three fixed roles. */
function readRole(
  candidate: Record<string, unknown>,
  where: string,
): ParseResult<BenchCoreRole> {
  const role = readString(candidate, 'role', where);
  if (!role.ok) {
    return role;
  }
  const known = BENCH_CORE_ROLES.find((candidateRole) => candidateRole === role.value);
  if (known === undefined) {
    return {
      ok: false,
      violation: `the ${where} declares role ${role.value}, which is not one of ${BENCH_CORE_ROLES.join(', ')}`,
    };
  }
  return { ok: true, value: known };
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
