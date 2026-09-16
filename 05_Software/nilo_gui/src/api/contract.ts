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

/** Path that requests an orderly whole-appliance power transition. */
export const POWER_PATH = '/api/v1/system/power';

/** Path that reports who the presented credential belongs to. */
export const SESSION_PATH = '/api/v1/auth/session';

/** Path of the identity administration collection. */
export const IDENTITIES_PATH = '/api/v1/users';

/** Path of the API token administration collection. */
export const TOKENS_PATH = '/api/v1/tokens';

/** Path of the appliance password policy. */
export const PASSWORD_POLICY_PATH = '/api/v1/password-policy';

/** Path of the authenticated network status resource. */
export const NETWORK_STATUS_PATH = '/api/v1/network/status';

/** Path of the authenticated traffic history resource. */
export const NETWORK_STATISTICS_PATH = '/api/v1/network/statistics';

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
export const BENCH_CORE_TOKEN_SCOPES = [
  'bench.read',
  'bench.execute',
  'bench.approve',
  'plugins.manage',
  'status.read',
  'inventory.read',
  'tokens.read.self',
  'tokens.manage.self',
  'users.read',
  'users.manage',
  'tokens.read.all',
  'tokens.manage.all',
  'network.read',
  'network.manage',
  'vpn.manage',
  'vpn.install',
  'activity.read',
  'activity.export',
  'activity.retention.manage',
  'password.policy.manage',
] as const;

/** Every fixed permission, including session-only appliance power control. */
export const BENCH_CORE_PERMISSIONS = [
  ...BENCH_CORE_TOKEN_SCOPES,
  'system.power.manage',
] as const;

/** One of the permissions above. */
export type BenchCorePermission = (typeof BENCH_CORE_PERMISSIONS)[number];

/** Permission required for whole-appliance restart and power-off. */
export const PERMISSION_SYSTEM_POWER_MANAGE: BenchCorePermission =
  'system.power.manage';

export type PowerAction = 'restart' | 'power_off';

export interface PowerResult {
  apiVersion: string;
  action: PowerAction;
  status: 'accepted';
}

/** Permission required to read network status and traffic history. Every role holds it. */
export const PERMISSION_ACTIVITY_READ: BenchCorePermission = 'activity.read';
export const PERMISSION_ACTIVITY_EXPORT: BenchCorePermission = 'activity.export';
export const PERMISSION_ACTIVITY_RETENTION_MANAGE: BenchCorePermission =
  'activity.retention.manage';
export const PERMISSION_NETWORK_READ: BenchCorePermission = 'network.read';

/** Permission required to read and change the appliance password policy. Administrators only. */
export const PERMISSION_PASSWORD_POLICY_MANAGE: BenchCorePermission =
  'password.policy.manage';

/**
 * The appliance's one rule for a password being set. It is never applied when a
 * password is checked, so tightening it locks nobody out.
 */
export interface PasswordPolicy {
  /** Fewest bytes a password may be. */
  minimumLength: number;
  /** Fewest distinct classes from lowercase, uppercase, digit and other. */
  minimumCharacterClasses: number;
}

/** Validate a decoded password policy. Both values are always present. */
export function parsePasswordPolicyBody(body: unknown): ParseResult<PasswordPolicy> {
  const envelope = readObject(body, 'password policy');
  if (!envelope.ok) {
    return envelope;
  }
  const minimumLength = envelope.value.minimum_length;
  const minimumCharacterClasses = envelope.value.minimum_character_classes;
  if (
    typeof minimumLength !== 'number' ||
    !Number.isInteger(minimumLength) ||
    typeof minimumCharacterClasses !== 'number' ||
    !Number.isInteger(minimumCharacterClasses)
  ) {
    return {
      ok: false,
      violation: 'the password policy does not carry both whole-number values',
    };
  }
  return { ok: true, value: { minimumLength, minimumCharacterClasses } };
}

/** Permission required to enable and disable interfaces. Administrators only. */
export const PERMISSION_NETWORK_MANAGE: BenchCorePermission = 'network.manage';

/** Permission required to manage WireGuard profiles and the tunnel. Administrators only. */
export const PERMISSION_VPN_MANAGE: BenchCorePermission = 'vpn.manage';

/** Permission required to install the WireGuard tooling. Administrators only. */
export const PERMISSION_VPN_INSTALL: BenchCorePermission = 'vpn.install';

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

/** Build the path of one identity itself, which is what a deletion addresses. */
export function identityResourcePath(username: string): string {
  return `${IDENTITIES_PATH}/${encodeURIComponent(username)}`;
}

/** Build the path that revokes one API token. */
export function tokenRevocationPath(id: string): string {
  return `${TOKENS_PATH}/${encodeURIComponent(id)}/revoke`;
}

/**
 * Build the path of one API token itself.
 *
 * Revocation is a sub-resource because it is something done to a token that
 * stays; deletion addresses the token, because afterwards there is none.
 */
export function tokenResourcePath(id: string): string {
  return `${TOKENS_PATH}/${encodeURIComponent(id)}`;
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

/** Validate an accepted power request without interpreting transport loss as success. */
export function parsePowerResultBody(body: unknown): ParseResult<PowerResult> {
  const envelope = readObject(body, 'power response');
  if (!envelope.ok) return envelope;
  const apiVersion = readApiVersion(envelope.value, 'power response');
  if (!apiVersion.ok) return apiVersion;
  const action = readString(envelope.value, 'action', 'power response');
  if (!action.ok) return action;
  if (action.value !== 'restart' && action.value !== 'power_off') {
    return { ok: false, violation: 'the power response has an unsupported action' };
  }
  const status = readString(envelope.value, 'status', 'power response');
  if (!status.ok) return status;
  if (status.value !== 'accepted') {
    return { ok: false, violation: 'the power response was not accepted' };
  }
  return {
    ok: true,
    value: { apiVersion: apiVersion.value, action: action.value, status: 'accepted' },
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

function readNumber(
  candidate: Record<string, unknown>,
  field: string,
  where: string,
): ParseResult<number> {
  const value = candidate[field];
  if (typeof value !== 'number' || !Number.isFinite(value)) {
    return {
      ok: false,
      violation: `the ${where} is missing ${field}, or it is not a number`,
    };
  }
  return { ok: true, value };
}

/**
 * Read an RFC 3339 instant. A timestamp the browser cannot place is a parse
 * failure rather than an Invalid Date quietly reaching a chart axis.
 */
function readInstant(
  candidate: Record<string, unknown>,
  field: string,
  where: string,
): ParseResult<Date> {
  const value = readString(candidate, field, where);
  if (!value.ok) {
    return value;
  }
  const instant = new Date(value.value);
  if (Number.isNaN(instant.getTime())) {
    return {
      ok: false,
      violation: `the ${where} carries ${field} as an unreadable time`,
    };
  }
  return { ok: true, value: instant };
}

/** The three windows the appliance offers, and the only ones it can draw. */
export const STATISTICS_PERIODS = ['15m', '1h', '24h'] as const;
export type StatisticsPeriod = (typeof STATISTICS_PERIODS)[number];

/** One network device as the appliance reports it. */
export interface NetworkInterface {
  id: string;
  name: string;
  kind: string;
  state: string;
  ipv4: string[];
  ipv6: string[];
  /** Empty or absent when the provider reports no gateway for this family. */
  gateway4?: string;
  gateway6?: string;
  linkCapacityMbps: number;
  rxBytesPerSecond: number;
  txBytesPerSecond: number;
  /**
   * Whether NILO will accept a change for this device. When false,
   * notControllableReason says why, so a missing control can be explained
   * rather than silently absent.
   */
  controllable: boolean;
  notControllableReason: string;
  /** The adapter's permanent hardware identity, which saved configuration binds to. */
  adapterId: string;
  /** The wireless network the adapter is on, empty for a wired adapter. */
  ssid: string;
  /** The saved configuration for this adapter, absent when none is saved. */
  settings: InterfaceSettingsState | null;
}

/** A saved adapter configuration beside what the adapter actually carries. */
export interface InterfaceSettingsState {
  saved: boolean;
  applied: boolean;
  revision: string;
  method: string;
  managementProtected: boolean;
  neverDefault: boolean;
  ssid: string;
}

/**
 * One complete network reading.
 *
 * measuredBy names what observed it — the simulated environment or the backend
 * host — so a simulated figure is never read as a bench observation.
 */
export interface NetworkStatus {
  measuredBy: string;
  interfaces: NetworkInterface[];
}

/** One measured minute of traffic. */
export interface NetworkStatisticsPoint {
  minuteStart: Date;
  rxBytes: number;
  txBytes: number;
  rxErrors: number;
  txErrors: number;
  rxDropped: number;
  txDropped: number;
  /** How many five-second intervals composed this minute. */
  samples: number;
  stale: boolean;
}

/**
 * One bounded traffic series.
 *
 * An empty points list is a legitimate answer: it says nothing was measured in
 * the window, which is what a gap looks like. It is never drawn as zero traffic.
 */
export interface NetworkStatistics {
  interfaceId: string;
  period: StatisticsPeriod;
  from: Date;
  to: Date;
  measuredBy: string;
  stale: boolean;
  points: NetworkStatisticsPoint[];
}

/** Every state one managed network change can be in. */
export const NETWORK_OPERATION_STATES = [
  'previewed',
  'applying',
  'awaiting_confirmation',
  'confirmed',
  'failed',
  'rolling_back',
  'rolled_back',
  'rollback_failed',
] as const;
export type NetworkOperationState = (typeof NETWORK_OPERATION_STATES)[number];

/**
 * One managed network change, as the appliance reports it.
 *
 * `revision` is the digest of the exact proposal that was previewed. An
 * acknowledgement carries it back, which is what stops a confirmation applying
 * to something other than what was shown.
 */
export interface NetworkOperation {
  id: string;
  action: string;
  target: string;
  state: NetworkOperationState;
  revision: string;
  createdAt: Date;
  /** Present only while a connectivity change is inside its window. */
  confirmDeadline: Date | null;
  failure: string;
  progress?: string;
}

/** Build the statistics query for one interface over one window. */
export function networkStatisticsPath(
  interfaceId: string,
  period: StatisticsPeriod,
): string {
  const query = new URLSearchParams({ interface: interfaceId, period });
  // URLSearchParams encodes a space as "+", which is correct for a form body
  // but not what a reader of a query string expects; %20 is unambiguous in both.
  return `${NETWORK_STATISTICS_PATH}?${query.toString().replace(/\+/g, '%20')}`;
}

function readNetworkInterface(
  entry: unknown,
  index: number,
): ParseResult<NetworkInterface> {
  const where = `network status interface ${index + 1}`;
  const candidate = readObject(entry, where);
  if (!candidate.ok) {
    return candidate;
  }
  const value = candidate.value;

  const id = readString(value, 'id', where);
  if (!id.ok) return id;
  const name = readString(value, 'name', where);
  if (!name.ok) return name;
  const kind = readString(value, 'kind', where);
  if (!kind.ok) return kind;
  const state = readString(value, 'state', where);
  if (!state.ok) return state;
  const ipv4 = readStringArray(value, 'ipv4', where);
  if (!ipv4.ok) return ipv4;
  const ipv6 = readStringArray(value, 'ipv6', where);
  if (!ipv6.ok) return ipv6;
  const gateway4 = readString({ gateway4: '', ...value }, 'gateway4', where);
  if (!gateway4.ok) return gateway4;
  const gateway6 = readString({ gateway6: '', ...value }, 'gateway6', where);
  if (!gateway6.ok) return gateway6;
  const linkCapacityMbps = readNumber(value, 'link_capacity_mbps', where);
  if (!linkCapacityMbps.ok) return linkCapacityMbps;
  const rxBytesPerSecond = readNumber(value, 'rx_bytes_per_second', where);
  if (!rxBytesPerSecond.ok) return rxBytesPerSecond;
  const txBytesPerSecond = readNumber(value, 'tx_bytes_per_second', where);
  if (!txBytesPerSecond.ok) return txBytesPerSecond;
  const controllable = readBoolean(value, 'controllable', where);
  if (!controllable.ok) return controllable;
  const notControllableReason = readString(value, 'not_controllable_reason', where);
  if (!notControllableReason.ok) return notControllableReason;
  const adapterId = typeof value.adapter_id === 'string' ? value.adapter_id : '';
  const ssid = typeof value.ssid === 'string' ? value.ssid : '';
  let settings: InterfaceSettingsState | null = null;
  if (value.settings !== undefined && value.settings !== null) {
    const state = readObject(value.settings, `${where} settings`);
    if (!state.ok) return state;
    settings = {
      saved: state.value.saved === true,
      applied: state.value.applied === true,
      revision: typeof state.value.revision === 'string' ? state.value.revision : '',
      method: typeof state.value.method === 'string' ? state.value.method : '',
      managementProtected: state.value.management_protected === true,
      neverDefault: state.value.never_default === true,
      ssid: typeof state.value.ssid === 'string' ? state.value.ssid : '',
    };
  }

  return {
    ok: true,
    value: {
      id: id.value,
      name: name.value,
      kind: kind.value,
      state: state.value,
      ipv4: ipv4.value,
      ipv6: ipv6.value,
      gateway4: gateway4.value,
      gateway6: gateway6.value,
      linkCapacityMbps: linkCapacityMbps.value,
      rxBytesPerSecond: rxBytesPerSecond.value,
      txBytesPerSecond: txBytesPerSecond.value,
      controllable: controllable.value,
      notControllableReason: notControllableReason.value,
      adapterId,
      ssid,
      settings,
    },
  };
}

/** Parse one network status response. */
export function parseNetworkStatusBody(body: unknown): ParseResult<NetworkStatus> {
  const envelope = readObject(body, 'network status response');
  if (!envelope.ok) {
    return envelope;
  }
  const candidate = envelope.value;

  const apiVersion = readApiVersion(candidate, 'network status response');
  if (!apiVersion.ok) {
    return apiVersion;
  }
  const measuredBy = readString(candidate, 'measured_by', 'network status response');
  if (!measuredBy.ok) {
    return measuredBy;
  }
  if (!Array.isArray(candidate.interfaces)) {
    return {
      ok: false,
      violation: 'the network status response is missing interfaces',
    };
  }

  const interfaces: NetworkInterface[] = [];
  for (const [index, entry] of candidate.interfaces.entries()) {
    const parsed = readNetworkInterface(entry, index);
    if (!parsed.ok) {
      return parsed;
    }
    interfaces.push(parsed.value);
  }
  return { ok: true, value: { measuredBy: measuredBy.value, interfaces } };
}

function readStatisticsPoint(
  entry: unknown,
  index: number,
): ParseResult<NetworkStatisticsPoint> {
  const where = `network statistics point ${index + 1}`;
  const candidate = readObject(entry, where);
  if (!candidate.ok) {
    return candidate;
  }
  const value = candidate.value;

  const minuteStart = readInstant(value, 'minute_start', where);
  if (!minuteStart.ok) return minuteStart;

  const counts: Record<string, number> = {};
  for (const field of [
    'rx_bytes',
    'tx_bytes',
    'rx_errors',
    'tx_errors',
    'rx_dropped',
    'tx_dropped',
    'samples',
  ]) {
    const parsed = readNumber(value, field, where);
    if (!parsed.ok) return parsed;
    counts[field] = parsed.value;
  }
  const stale = readBoolean(value, 'stale', where);
  if (!stale.ok) return stale;

  return {
    ok: true,
    value: {
      minuteStart: minuteStart.value,
      rxBytes: counts.rx_bytes,
      txBytes: counts.tx_bytes,
      rxErrors: counts.rx_errors,
      txErrors: counts.tx_errors,
      rxDropped: counts.rx_dropped,
      txDropped: counts.tx_dropped,
      samples: counts.samples,
      stale: stale.value,
    },
  };
}

/** The listing of recent managed operations, newest first. */
export const NETWORK_OPERATIONS_PATH = '/api/v1/network/operations';

/** Parse the listing of recent managed operations. */
export function parseNetworkOperationsBody(
  body: unknown,
): ParseResult<NetworkOperation[]> {
  const envelope = readObject(body, 'network operations response');
  if (!envelope.ok) {
    return envelope;
  }
  const apiVersion = readApiVersion(envelope.value, 'network operations response');
  if (!apiVersion.ok) return apiVersion;
  if (!Array.isArray(envelope.value.items)) {
    return { ok: false, violation: 'the network operations response has no items' };
  }
  const items: NetworkOperation[] = [];
  for (const item of envelope.value.items) {
    const parsed = parseNetworkOperationBody({
      api_version: apiVersion.value,
      ...(typeof item === 'object' && item !== null ? item : {}),
    });
    if (!parsed.ok) return parsed;
    items.push(parsed.value);
  }
  return { ok: true, value: items };
}

/** Path that proposes enabling or disabling one interface. */
export function interfacePreviewPath(interfaceId: string): string {
  return `/api/v1/network/interfaces/${encodeURIComponent(interfaceId)}/preview`;
}

/** Path of one managed operation, optionally with the act being taken on it. */
export function networkOperationPath(id: string, action?: string): string {
  const base = `/api/v1/network/operations/${encodeURIComponent(id)}`;
  return action === undefined ? base : `${base}/${action}`;
}

/** Parse one managed network operation. */
export function parseNetworkOperationBody(
  body: unknown,
): ParseResult<NetworkOperation> {
  const envelope = readObject(body, 'network operation response');
  if (!envelope.ok) {
    return envelope;
  }
  const candidate = envelope.value;

  const apiVersion = readApiVersion(candidate, 'network operation response');
  if (!apiVersion.ok) return apiVersion;
  const id = readString(candidate, 'id', 'network operation response');
  if (!id.ok) return id;
  const action = readString(candidate, 'action', 'network operation response');
  if (!action.ok) return action;
  const target = readString(candidate, 'target', 'network operation response');
  if (!target.ok) return target;
  const state = readString(candidate, 'state', 'network operation response');
  if (!state.ok) return state;
  if (!NETWORK_OPERATION_STATES.includes(state.value as NetworkOperationState)) {
    return {
      ok: false,
      violation: `the network operation response reports the unknown state ${state.value}`,
    };
  }
  const revision = readString(candidate, 'revision', 'network operation response');
  if (!revision.ok) return revision;
  const createdAt = readInstant(candidate, 'created_at', 'network operation response');
  if (!createdAt.ok) return createdAt;
  if (candidate.progress !== undefined && typeof candidate.progress !== 'string') {
    return { ok: false, violation: 'network operation progress must be a string' };
  }

  // Absent for an action that needs no confirmation, which is not an error: it
  // is how the appliance says this change carries no countdown.
  let confirmDeadline: Date | null = null;
  if (candidate.confirm_deadline !== undefined && candidate.confirm_deadline !== null) {
    const parsed = readInstant(
      candidate,
      'confirm_deadline',
      'network operation response',
    );
    if (!parsed.ok) return parsed;
    confirmDeadline = parsed.value;
  }

  return {
    ok: true,
    value: {
      id: id.value,
      action: action.value,
      target: target.value,
      state: state.value as NetworkOperationState,
      revision: revision.value,
      createdAt: createdAt.value,
      confirmDeadline,
      failure: typeof candidate.failure === 'string' ? candidate.failure : '',
      progress: typeof candidate.progress === 'string' ? candidate.progress : '',
    },
  };
}

/** Parse one bounded traffic series. */
export function parseNetworkStatisticsBody(
  body: unknown,
): ParseResult<NetworkStatistics> {
  const envelope = readObject(body, 'network statistics response');
  if (!envelope.ok) {
    return envelope;
  }
  const candidate = envelope.value;

  const apiVersion = readApiVersion(candidate, 'network statistics response');
  if (!apiVersion.ok) {
    return apiVersion;
  }
  const interfaceId = readString(
    candidate,
    'interface_id',
    'network statistics response',
  );
  if (!interfaceId.ok) return interfaceId;
  const period = readString(candidate, 'period', 'network statistics response');
  if (!period.ok) return period;
  if (!STATISTICS_PERIODS.includes(period.value as StatisticsPeriod)) {
    return {
      ok: false,
      violation: `the network statistics response declares the period ${period.value}, which this interface cannot draw`,
    };
  }
  const from = readInstant(candidate, 'from', 'network statistics response');
  if (!from.ok) return from;
  const to = readInstant(candidate, 'to', 'network statistics response');
  if (!to.ok) return to;
  const measuredBy = readString(
    candidate,
    'measured_by',
    'network statistics response',
  );
  if (!measuredBy.ok) return measuredBy;
  const stale = readBoolean(candidate, 'stale', 'network statistics response');
  if (!stale.ok) return stale;

  if (!Array.isArray(candidate.points)) {
    return {
      ok: false,
      violation: 'the network statistics response is missing points',
    };
  }
  const points: NetworkStatisticsPoint[] = [];
  for (const [index, entry] of candidate.points.entries()) {
    const parsed = readStatisticsPoint(entry, index);
    if (!parsed.ok) {
      return parsed;
    }
    points.push(parsed.value);
  }

  return {
    ok: true,
    value: {
      interfaceId: interfaceId.value,
      period: period.value as StatisticsPeriod,
      from: from.value,
      to: to.value,
      measuredBy: measuredBy.value,
      stale: stale.value,
      points,
    },
  };
}
