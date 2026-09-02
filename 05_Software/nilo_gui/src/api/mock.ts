import type { BenchCoreTransport, TransportRequest, TransportResponse } from './client';
import {
  BENCH_CORE_API_VERSION,
  CSRF_HEADER_NAME,
  DEVICES_PATH,
  IDENTITIES_PATH,
  INTEGRATIONS_PATH,
  SESSION_PATH,
  SIGN_IN_PATH,
  SIGN_OUT_PATH,
  STATUS_PATH,
  TOKENS_PATH,
  type BenchCorePermission,
  type BenchCoreRole,
  type DevicesResponseBody,
  type IntegrationsResponseBody,
  type StatusResponseBody,
} from './contract';

/**
 * Contract-aligned mock answers for the versioned Bench Core resources.
 *
 * The transport routes on the request path first and the scenario second, so
 * one scenario can leave the status healthy while an inventory is unusable —
 * which is exactly what the interface has to tell apart. Every body below is
 * typed against the same contract the production client parses, so a field the
 * backend renames stops compiling here too.
 *
 * The inventories reproduce the deterministic catalog the Go simulation profile
 * serves. Beside them the mock keeps a small identity and token store, so a
 * developer can sign in with a password, administer identities and tokens, and
 * see every screen without a WSL2 backend running. That store lives in memory
 * for the life of the page: reloading it signs the developer out, which is the
 * one place mock mode deliberately differs from a real cookie session.
 *
 * This is never reachable in live mode.
 */

/** The mock behaviors a developer can select. */
export type MockScenario =
  | 'healthy'
  | 'unavailable-inventory'
  | 'empty'
  | 'unauthorized'
  | 'unavailable'
  | 'malformed-status'
  | 'malformed-integrations'
  | 'malformed-devices';

/** Every scenario name, in the order they are documented. */
export const MOCK_SCENARIOS: readonly MockScenario[] = [
  'healthy',
  'unavailable-inventory',
  'empty',
  'unauthorized',
  'unavailable',
  'malformed-status',
  'malformed-integrations',
  'malformed-devices',
];

/**
 * Earlier spellings that still resolve. `malformed` named the only malformed
 * case that existed when the status was the only resource; keeping it accepted
 * means an existing ignored `.env.local` does not start failing validation.
 */
const SCENARIO_ALIASES: Readonly<Record<string, MockScenario>> = {
  malformed: 'malformed-status',
};

/** The password every seeded mock identity signs in with. */
export const MOCK_DEVELOPMENT_PASSWORD = 'simulation-password';

/** The identities the mock store starts with, one per fixed role. */
export const MOCK_IDENTITIES: readonly { username: string; role: BenchCoreRole }[] = [
  { username: 'admin', role: 'administrator' },
  { username: 'operator', role: 'operator' },
  { username: 'viewer', role: 'viewer' },
];

/** The healthy answer, typed against the versioned response contract. */
export const MOCK_HEALTHY_STATUS_BODY: StatusResponseBody = {
  api_version: BENCH_CORE_API_VERSION,
  status: 'healthy',
  profile: 'simulation',
  simulation: true,
  build: {
    name: 'nilo',
    version: '0.1.0',
    integration_protocol: 'nilo.integration/1',
  },
};

/** The ready integration inventory, as the simulation catalog defines it. */
export const MOCK_HEALTHY_INTEGRATIONS_BODY: IntegrationsResponseBody = {
  api_version: BENCH_CORE_API_VERSION,
  scenario: 'healthy',
  items: [
    {
      id: 'simulation.fake-python',
      display_name: 'SIMULATION - Fake Python Integration',
      status: 'healthy',
      simulation: true,
      operations: ['artifact', 'crash', 'echo', 'fail', 'sleep'],
    },
  ],
};

/** The ready device inventory, as the simulation catalog defines it. */
export const MOCK_HEALTHY_DEVICES_BODY: DevicesResponseBody = {
  api_version: BENCH_CORE_API_VERSION,
  scenario: 'healthy',
  items: [
    {
      id: 'simulation.fake-python.device.stable-001',
      integration_id: 'simulation.fake-python',
      display_name: 'SIMULATION - Electronics Bench 001',
      kind: 'simulated-electronics-bench',
      status: 'ready',
      simulation: true,
      capabilities: ['artifact', 'crash', 'echo', 'fail', 'sleep'],
    },
  ],
};

/** The same catalog in its unavailable scenario: present, but not usable. */
export const MOCK_UNAVAILABLE_INTEGRATIONS_BODY: IntegrationsResponseBody = {
  ...MOCK_HEALTHY_INTEGRATIONS_BODY,
  scenario: 'unavailable',
  items: MOCK_HEALTHY_INTEGRATIONS_BODY.items.map((item) => ({
    ...item,
    status: 'unavailable',
  })),
};

/** The same catalog in its unavailable scenario: present, but not usable. */
export const MOCK_UNAVAILABLE_DEVICES_BODY: DevicesResponseBody = {
  ...MOCK_HEALTHY_DEVICES_BODY,
  scenario: 'unavailable',
  items: MOCK_HEALTHY_DEVICES_BODY.items.map((item) => ({
    ...item,
    status: 'unavailable',
  })),
};

/** A bench with nothing on it: a valid answer that happens to be empty. */
export const MOCK_EMPTY_INTEGRATIONS_BODY: IntegrationsResponseBody = {
  api_version: BENCH_CORE_API_VERSION,
  scenario: 'healthy',
  items: [],
};

/** A bench with nothing on it: a valid answer that happens to be empty. */
export const MOCK_EMPTY_DEVICES_BODY: DevicesResponseBody = {
  api_version: BENCH_CORE_API_VERSION,
  scenario: 'healthy',
  items: [],
};

/** Return the named scenario, or null when the name is not one of ours. */
export function parseMockScenario(value: string): MockScenario | null {
  return (
    MOCK_SCENARIOS.find((scenario) => scenario === value) ??
    SCENARIO_ALIASES[value] ??
    null
  );
}

/** The permissions each fixed role carries, mirroring the compiled-in matrix. */
const ROLE_PERMISSIONS: Readonly<Record<BenchCoreRole, BenchCorePermission[]>> = {
  viewer: ['inventory.read', 'status.read'],
  operator: ['inventory.read', 'status.read', 'tokens.manage.self', 'tokens.read.self'],
  administrator: [
    'inventory.read',
    'status.read',
    'tokens.manage.all',
    'tokens.manage.self',
    'tokens.read.all',
    'tokens.read.self',
    'users.manage',
    'users.read',
  ],
};

interface MockIdentity {
  id: string;
  username: string;
  role: BenchCoreRole;
  enabled: boolean;
  created_at: string;
  password: string;
}

interface MockToken {
  id: string;
  name: string;
  owner: string;
  scopes: string[];
  created_at: string;
  expires_at: string | null;
  revoked: boolean;
  revoked_at: string | null;
}

/** One authenticated mock caller: a signed-in identity or a bearer token. */
interface MockActor {
  identity: MockIdentity;
  kind: 'session' | 'token';
  permissions: BenchCorePermission[];
}

const MOCK_EPOCH = '2026-09-02T09:00:00Z';
const MOCK_CSRF_TOKEN = 'mock-csrf-token';

/**
 * The mutable half of the mock: identities, tokens and the one session a
 * developer can hold. It enforces the same role matrix and the same CSRF rule
 * the backend does, so a screen that works here is not relying on the mock
 * being lenient.
 */
class MockAdministration {
  identities: MockIdentity[];
  tokens: MockToken[];
  session: MockIdentity | null = null;
  #sequence = 0;

  constructor() {
    this.identities = MOCK_IDENTITIES.map((seed, index) => ({
      id: `mock-identity-${index + 1}`,
      username: seed.username,
      role: seed.role,
      enabled: true,
      created_at: MOCK_EPOCH,
      password: MOCK_DEVELOPMENT_PASSWORD,
    }));
    this.tokens = [
      {
        id: 'mock-token-1',
        name: 'bench-ci',
        owner: 'operator',
        scopes: ['status.read'],
        created_at: MOCK_EPOCH,
        expires_at: null,
        revoked: false,
        revoked_at: null,
      },
    ];
  }

  nextId(prefix: string): string {
    this.#sequence += 1;
    return `${prefix}-${this.#sequence + 1}`;
  }

  find(username: string): MockIdentity | undefined {
    return this.identities.find((identity) => identity.username === username);
  }

  enabledAdministrators(): number {
    return this.identities.filter(
      (identity) => identity.role === 'administrator' && identity.enabled,
    ).length;
  }
}

/** Build a transport that answers every route according to one scenario. */
export function createMockTransport(scenario: MockScenario): BenchCoreTransport {
  const store = new MockAdministration();

  return (url, request) => {
    if (scenario === 'unavailable') {
      return Promise.reject(
        new TypeError('the mock Bench Core backend is configured as unreachable'),
      );
    }
    if (scenario === 'unauthorized') {
      return Promise.resolve(refusal(401, 'unauthorized'));
    }
    return Promise.resolve(answer(store, scenario, pathOf(url), request));
  };
}

function answer(
  store: MockAdministration,
  scenario: MockScenario,
  path: string,
  request: TransportRequest,
): TransportResponse {
  if (path === SIGN_IN_PATH && request.method === 'POST') {
    return signIn(store, request);
  }

  const actor = resolveActor(store, request);
  if (actor === null) {
    return refusal(401, 'unauthorized');
  }
  // A cookie is attached by the browser on its own, so a state-changing
  // session request has to repeat a value only page code can know.
  if (
    actor.kind === 'session' &&
    request.method !== 'GET' &&
    request.headers[CSRF_HEADER_NAME] !== MOCK_CSRF_TOKEN
  ) {
    return refusal(403, 'forbidden');
  }

  if (path === SESSION_PATH) {
    return jsonResponse(200, sessionBody(actor, null));
  }
  if (path === SIGN_OUT_PATH) {
    store.session = null;
    return emptyResponse(204);
  }
  if (path === STATUS_PATH) {
    return gated(actor, 'status.read', () =>
      jsonResponse(
        200,
        scenario === 'malformed-status'
          ? { api_version: BENCH_CORE_API_VERSION, status: 'healthy', simulation: true }
          : MOCK_HEALTHY_STATUS_BODY,
      ),
    );
  }
  if (path === INTEGRATIONS_PATH) {
    return gated(actor, 'inventory.read', () =>
      jsonResponse(200, integrationsFor(scenario)),
    );
  }
  if (path === DEVICES_PATH) {
    return gated(actor, 'inventory.read', () =>
      jsonResponse(200, devicesFor(scenario)),
    );
  }
  return administer(store, actor, path, request);
}

function administer(
  store: MockAdministration,
  actor: MockActor,
  path: string,
  request: TransportRequest,
): TransportResponse {
  const body = decodeBody(request);

  if (path === IDENTITIES_PATH && request.method === 'GET') {
    return gated(actor, 'users.read', () =>
      jsonResponse(200, {
        api_version: BENCH_CORE_API_VERSION,
        items: store.identities.map(identityBody),
      }),
    );
  }
  if (path === IDENTITIES_PATH && request.method === 'POST') {
    return gated(actor, 'users.manage', () => createIdentity(store, body));
  }
  if (path === TOKENS_PATH && request.method === 'GET') {
    return gated(actor, 'tokens.read.all', () =>
      jsonResponse(200, {
        api_version: BENCH_CORE_API_VERSION,
        items: store.tokens.map(tokenBody),
      }),
    );
  }
  if (path === TOKENS_PATH && request.method === 'POST') {
    return gated(actor, 'tokens.manage.all', () => createToken(store, body));
  }

  const identityAction = /^\/api\/v1\/users\/([^/]+)\/(role|enabled|password)$/.exec(
    path,
  );
  if (identityAction !== null) {
    const [, username, action] = identityAction;
    return gated(actor, 'users.manage', () =>
      changeIdentity(store, decodeURIComponent(username), action, body),
    );
  }
  const revocation = /^\/api\/v1\/tokens\/([^/]+)\/revoke$/.exec(path);
  if (revocation !== null) {
    return gated(actor, 'tokens.manage.all', () =>
      revokeToken(store, decodeURIComponent(revocation[1])),
    );
  }

  // An unrouted path is a mistake in the caller, not a resource with no
  // entries; answering it with a body would hide that.
  return refusal(404, 'no_such_mock_resource');
}

function signIn(
  store: MockAdministration,
  request: TransportRequest,
): TransportResponse {
  const body = decodeBody(request);
  const identity = store.find(String(body.username ?? ''));
  if (
    identity === undefined ||
    !identity.enabled ||
    identity.password !== String(body.password ?? '')
  ) {
    return refusal(401, 'unauthorized');
  }
  store.session = identity;
  return jsonResponse(200, sessionBody(asActor(identity, 'session'), MOCK_CSRF_TOKEN));
}

function createIdentity(
  store: MockAdministration,
  body: Record<string, unknown>,
): TransportResponse {
  const username = String(body.username ?? '').trim();
  const role = String(body.role ?? '');
  const password = String(body.password ?? '');
  if (!/^[a-z0-9._-]{2,64}$/.test(username)) {
    return refusal(400, 'invalid_username', 'username must be 2 to 64 characters');
  }
  if (!isRole(role)) {
    return refusal(
      400,
      'invalid_role',
      'role must be one of administrator, operator or viewer',
    );
  }
  if (password.length < 12) {
    return refusal(400, 'invalid_password', 'password must be at least 12 bytes');
  }
  if (store.find(username) !== undefined) {
    return refusal(
      409,
      'username_taken',
      'a different identity already uses that username',
    );
  }
  const created: MockIdentity = {
    id: store.nextId('mock-identity'),
    username,
    role,
    enabled: true,
    created_at: MOCK_EPOCH,
    password,
  };
  store.identities.push(created);
  return jsonResponse(201, {
    api_version: BENCH_CORE_API_VERSION,
    ...identityBody(created),
  });
}

function changeIdentity(
  store: MockAdministration,
  username: string,
  action: string,
  body: Record<string, unknown>,
): TransportResponse {
  const identity = store.find(username);
  if (identity === undefined) {
    return refusal(404, 'user_not_found', 'no identity carries that username');
  }
  if (action === 'role') {
    const role = String(body.role ?? '');
    if (!isRole(role)) {
      return refusal(
        400,
        'invalid_role',
        'role must be one of administrator, operator or viewer',
      );
    }
    if (removesLastAdministrator(store, identity, role !== 'administrator')) {
      return lastAdministrator();
    }
    identity.role = role;
    return emptyResponse(204);
  }
  if (action === 'enabled') {
    if (typeof body.enabled !== 'boolean') {
      return refusal(400, 'invalid_enabled', 'enabled must be true or false');
    }
    if (removesLastAdministrator(store, identity, !body.enabled)) {
      return lastAdministrator();
    }
    identity.enabled = body.enabled;
    return emptyResponse(204);
  }
  const password = String(body.password ?? '');
  if (password.length < 12) {
    return refusal(400, 'invalid_password', 'password must be at least 12 bytes');
  }
  identity.password = password;
  return emptyResponse(204);
}

function createToken(
  store: MockAdministration,
  body: Record<string, unknown>,
): TransportResponse {
  const owner = String(body.owner ?? '');
  const name = String(body.name ?? '').trim();
  const scopes = Array.isArray(body.scopes) ? body.scopes.map(String) : [];
  const expiresAt = typeof body.expires_at === 'string' ? body.expires_at : null;

  if (store.find(owner) === undefined) {
    return refusal(404, 'user_not_found', 'no identity carries that username');
  }
  if (name === '') {
    return refusal(400, 'invalid_name', 'name must not be empty');
  }
  if (
    scopes.some(
      (scope) => !ROLE_PERMISSIONS.administrator.includes(scope as BenchCorePermission),
    )
  ) {
    return refusal(
      400,
      'invalid_scope',
      'every scope must name a permission this appliance defines',
    );
  }
  if (expiresAt !== null && !(Date.parse(expiresAt) > Date.now())) {
    return refusal(
      400,
      'invalid_expiry',
      'expiry must be an RFC 3339 instant in the future',
    );
  }
  const created: MockToken = {
    id: store.nextId('mock-token'),
    name,
    owner,
    scopes,
    created_at: MOCK_EPOCH,
    expires_at: expiresAt,
    revoked: false,
    revoked_at: null,
  };
  store.tokens.push(created);
  return jsonResponse(201, {
    api_version: BENCH_CORE_API_VERSION,
    token: tokenBody(created),
    secret: `nilo_mock_${created.id}`,
  });
}

function revokeToken(store: MockAdministration, id: string): TransportResponse {
  const token = store.tokens.find(
    (candidate) => candidate.id === id && !candidate.revoked,
  );
  if (token === undefined) {
    return refusal(
      404,
      'token_not_found',
      'no active API token carries that identifier',
    );
  }
  token.revoked = true;
  token.revoked_at = MOCK_EPOCH;
  return emptyResponse(204);
}

/**
 * Resolve who is asking. A bearer token authenticates as the seeded Viewer,
 * which is what `nilo development issue-token` produces against a real
 * backend, so a development token is refused by the administration routes here
 * for the same reason it would be there.
 */
function resolveActor(
  store: MockAdministration,
  request: TransportRequest,
): MockActor | null {
  const authorization = request.headers.Authorization ?? '';
  if (authorization.startsWith('Bearer ') && authorization.length > 'Bearer '.length) {
    const viewer = store.find('viewer');
    return viewer === undefined ? null : asActor(viewer, 'token');
  }
  return store.session === null ? null : asActor(store.session, 'session');
}

function asActor(identity: MockIdentity, kind: 'session' | 'token'): MockActor {
  return { identity, kind, permissions: ROLE_PERMISSIONS[identity.role] };
}

function gated(
  actor: MockActor,
  permission: BenchCorePermission,
  answerWith: () => TransportResponse,
): TransportResponse {
  return actor.permissions.includes(permission)
    ? answerWith()
    : refusal(403, 'forbidden');
}

function removesLastAdministrator(
  store: MockAdministration,
  identity: MockIdentity,
  removes: boolean,
): boolean {
  return (
    removes &&
    identity.role === 'administrator' &&
    identity.enabled &&
    store.enabledAdministrators() <= 1
  );
}

function lastAdministrator(): TransportResponse {
  return refusal(
    409,
    'last_administrator',
    'the last enabled administrator must remain an enabled administrator',
  );
}

function sessionBody(
  actor: MockActor,
  csrfToken: string | null,
): Record<string, unknown> {
  return {
    api_version: BENCH_CORE_API_VERSION,
    username: actor.identity.username,
    role: actor.identity.role,
    permissions: actor.permissions,
    credential: actor.kind,
    ...(csrfToken === null ? {} : { csrf_token: csrfToken }),
  };
}

function identityBody(identity: MockIdentity): Record<string, unknown> {
  return {
    id: identity.id,
    username: identity.username,
    role: identity.role,
    enabled: identity.enabled,
    created_at: identity.created_at,
  };
}

function tokenBody(token: MockToken): Record<string, unknown> {
  return {
    id: token.id,
    name: token.name,
    owner: token.owner,
    scopes: token.scopes,
    created_at: token.created_at,
    expires_at: token.expires_at,
    revoked: token.revoked,
    revoked_at: token.revoked_at,
  };
}

function isRole(value: string): value is BenchCoreRole {
  return value === 'administrator' || value === 'operator' || value === 'viewer';
}

function decodeBody(request: TransportRequest): Record<string, unknown> {
  if (request.body === undefined) {
    return {};
  }
  try {
    const decoded: unknown = JSON.parse(request.body);
    return typeof decoded === 'object' && decoded !== null
      ? (decoded as Record<string, unknown>)
      : {};
  } catch {
    return {};
  }
}

/** The origin-relative path of a request URL, which is all the mock routes on. */
function pathOf(url: string): string {
  const withoutOrigin = url.replace(/^[a-z]+:\/\/[^/]+/i, '');
  return withoutOrigin.split('?')[0];
}

function integrationsFor(scenario: MockScenario): unknown {
  switch (scenario) {
    case 'unavailable-inventory':
      return MOCK_UNAVAILABLE_INTEGRATIONS_BODY;
    case 'empty':
      return MOCK_EMPTY_INTEGRATIONS_BODY;
    case 'malformed-integrations':
      return { api_version: BENCH_CORE_API_VERSION, scenario: 'healthy' };
    default:
      return MOCK_HEALTHY_INTEGRATIONS_BODY;
  }
}

function devicesFor(scenario: MockScenario): unknown {
  switch (scenario) {
    case 'unavailable-inventory':
      return MOCK_UNAVAILABLE_DEVICES_BODY;
    case 'empty':
      return MOCK_EMPTY_DEVICES_BODY;
    case 'malformed-devices':
      return {
        api_version: BENCH_CORE_API_VERSION,
        scenario: 'healthy',
        items: [{ id: 'simulation.fake-python.device.stable-001' }],
      };
    default:
      return MOCK_HEALTHY_DEVICES_BODY;
  }
}

function refusal(status: number, code: string, message?: string): TransportResponse {
  return jsonResponse(
    status,
    message === undefined ? { error: code } : { error: code, message },
  );
}

function emptyResponse(status: number): TransportResponse {
  return { status, ok: status >= 200 && status < 300, text: () => Promise.resolve('') };
}

function jsonResponse(status: number, body: unknown): TransportResponse {
  const encoded = JSON.stringify(body);
  return {
    status,
    ok: status >= 200 && status < 300,
    text: () => Promise.resolve(encoded),
  };
}
