import type { BenchCoreTransport, TransportRequest, TransportResponse } from './client';
import { MANAGEMENT_ACCESS_PATH } from './management-access';
import {
  BENCH_CORE_API_VERSION,
  CSRF_HEADER_NAME,
  DEVICES_PATH,
  IDENTITIES_PATH,
  INTEGRATIONS_PATH,
  NETWORK_STATISTICS_PATH,
  NETWORK_STATUS_PATH,
  POWER_PATH,
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

/**
 * The password every seeded mock identity signs in with.
 *
 * Deliberately the same pair the real appliance seeds under
 * `serve --development-administrator`, so a developer learns one username and
 * one password and it works whichever of the two they started.
 */
export const MOCK_DEVELOPMENT_PASSWORD = 'admin_password';

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
  viewer: ['inventory.read', 'network.read', 'status.read'],
  operator: [
    'inventory.read',
    'network.read',
    'status.read',
    'system.power.manage',
    'tokens.manage.self',
    'tokens.read.self',
  ],
  administrator: [
    'inventory.read',
    'network.manage',
    'network.read',
    'status.read',
    'tokens.manage.all',
    'tokens.manage.self',
    'tokens.read.all',
    'tokens.read.self',
    'users.manage',
    'users.read',
    'vpn.install',
    'vpn.manage',
    'activity.read',
    'activity.export',
    'activity.retention.manage',
    'system.power.manage',
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

/** One managed change, in the wire shape the operation routes return. */
interface MockOperation {
  api_version: string;
  id: string;
  action: string;
  target: string;
  state: string;
  revision: string;
  created_at: string;
  confirm_deadline?: string;
  failure: string;
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
  /** Managed changes, by identifier, for the life of the page. */
  operations = new Map<string, MockOperation>();
  /** Saved device networks, in the wire shape the routes return. */
  deviceNetworks = new Map<string, Record<string, unknown>>();
  /** Interface-name boundary retained by the management-access API. */
  managementInterfaces = ['enp1s0'];
  managementState: 'pending' | 'applied' | 'recovered' = 'applied';
  /** The shared history, appended by every managed change. */
  activity: Record<string, unknown>[] = [];
  policy: {
    retention_days: Record<string, number>;
    quota_bytes: Record<string, number>;
  } = {
    retention_days: { audit: 90, event: 14 },
    quota_bytes: { audit: 268435456, event: 134217728 },
  };
  /** The device-network draft a preview proposed, applied on acknowledgement. */
  pendingDrafts = new Map<
    string,
    { action: string; target: string; draft?: Record<string, unknown> }
  >();
  #sequence = 0;
  #activitySequence = 0;

  /** Append one history entry with the envelope the appliance would add. */
  record(
    category: 'audit' | 'event',
    component: string,
    action: string,
    outcome: string,
    subjectType: string,
    subjectId: string,
    actor: MockActor | null,
    operationId = '',
    details: Record<string, unknown> = {},
  ): void {
    this.#activitySequence += 1;
    this.activity.push({
      id: `mock-event-${this.#activitySequence}`,
      sequence: this.#activitySequence,
      created_at: new Date().toISOString(),
      category,
      component,
      action,
      outcome,
      severity: outcome === 'failed' || outcome === 'denied' ? 'warning' : 'info',
      actor_kind: actor === null ? 'system' : actor.kind === 'token' ? 'token' : 'user',
      actor_id:
        actor === null ? '00000000-0000-4000-8000-00000000005e' : actor.identity.id,
      subject_type: subjectType,
      subject_id: subjectId,
      target_label: subjectId,
      operation_id: operationId,
      appliance_id: 'mock-appliance',
      boot_id: 'mock-boot',
      time_source: 'unsynchronized',
      details,
    });
  }

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

  /**
   * Propose a change. Nothing is applied: the operation is held in `previewed`
   * until an acknowledgement carrying its revision arrives.
   */
  previewOperation(
    action: string,
    target: string,
    actor: MockActor | null = null,
  ): MockOperation {
    const id = this.nextId('operation');
    this.record(
      'audit',
      componentOf(action),
      action,
      'requested',
      'target',
      target,
      actor,
      id,
      {
        state: 'previewed',
      },
    );
    const operation: MockOperation = {
      api_version: BENCH_CORE_API_VERSION,
      id,
      action,
      target,
      state: 'previewed',
      // Derived from the proposal so the same change previews to the same
      // revision, as the appliance's digest does.
      revision: `revision-${action}-${target}`,
      created_at: MOCK_EPOCH,
      failure: '',
    };
    this.operations.set(id, operation);
    return operation;
  }

  /**
   * Apply an acknowledged change. Only the actions that can cut the caller's
   * own path enter a confirmation window; the rest finish immediately.
   */
  acknowledgeOperation(id: string): MockOperation {
    const operation = this.operations.get(id) as MockOperation;
    const connectivity =
      operation.action.startsWith('interface.') ||
      operation.action === 'wifi.join' ||
      operation.action === 'vpn.activate' ||
      operation.action === 'vpn.deactivate' ||
      [
        'device_network.update',
        'device_network.enable',
        'device_network.disable',
        'device_network.delete',
      ].includes(operation.action);

    const applied: MockOperation = connectivity
      ? {
          ...operation,
          state: 'awaiting_confirmation',
          // Two minutes from now, so the countdown in the interface runs
          // against a real deadline rather than a fixed one.
          confirm_deadline: new Date(Date.now() + 120_000).toISOString(),
        }
      : { ...operation, state: 'confirmed' };
    this.operations.set(id, applied);
    return applied;
  }

  /** Keep or restore a change that is inside its window. */
  settleOperation(id: string, state: 'confirmed' | 'rolled_back'): MockOperation {
    const operation = this.operations.get(id) as MockOperation;
    const settled: MockOperation = { ...operation, state, confirm_deadline: undefined };
    this.operations.set(id, settled);
    return settled;
  }

  /** Apply an acknowledged device-network change to the saved inventory. */
  applyDeviceNetworkChange(id: string): void {
    const pending = this.pendingDrafts.get(id);
    if (pending === undefined) return;
    this.pendingDrafts.delete(id);
    const now = new Date().toISOString();
    if (pending.action === 'create' && pending.draft !== undefined) {
      const networkId = this.nextId('device-network');
      this.deviceNetworks.set(
        networkId,
        mockDeviceNetworkBody(networkId, pending.draft, false, now),
      );
      return;
    }
    const existing = this.deviceNetworks.get(pending.target);
    if (existing === undefined) return;
    switch (pending.action) {
      case 'update':
        if (pending.draft !== undefined) {
          this.deviceNetworks.set(
            pending.target,
            mockDeviceNetworkBody(
              pending.target,
              pending.draft,
              existing.enabled === true,
              now,
            ),
          );
        }
        break;
      case 'enable':
        this.deviceNetworks.set(pending.target, {
          ...existing,
          enabled: true,
          state: 'active',
          applied: true,
          detail: '',
          observed_at: now,
        });
        break;
      case 'disable':
        this.deviceNetworks.set(pending.target, {
          ...existing,
          enabled: false,
          state: 'disabled',
          applied: false,
          detail: '',
          observed_at: now,
        });
        break;
      case 'delete':
        this.deviceNetworks.delete(pending.target);
        break;
    }
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
    return Promise.resolve(answer(store, scenario, pathOf(url), queryOf(url), request));
  };
}

function answer(
  store: MockAdministration,
  scenario: MockScenario,
  path: string,
  // Carried separately from the path because one route reads the window it was
  // asked for out of it, and routing itself is always on the bare path.
  query: string,
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
  if (path === POWER_PATH) {
    if (actor.kind !== 'session') return refusal(403, 'forbidden');
    return gated(actor, 'system.power.manage', () =>
      refusal(503, 'power_control_unavailable'),
    );
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
  if (path === NETWORK_STATUS_PATH) {
    return gated(actor, 'network.read', () =>
      jsonResponse(200, MOCK_NETWORK_STATUS_BODY),
    );
  }
  if (path === NETWORK_STATISTICS_PATH) {
    return gated(actor, 'network.read', () =>
      jsonResponse(200, mockStatisticsFor(query)),
    );
  }
  if (path.startsWith('/api/v1/network/')) {
    return manageNetwork(store, actor, path, query, request);
  }
  if (path.startsWith('/api/v1/activity')) {
    return observe(store, actor, path, query, request);
  }
  return administer(store, actor, path, request);
}

/**
 * The managed-change routes, mirroring the Go operation lifecycle: a preview
 * that changes nothing, an acknowledgement bound to the revision it was shown,
 * and a confirmation that is a separate act from acknowledging.
 */
function manageNetwork(
  store: MockAdministration,
  actor: MockActor,
  path: string,
  query: string,
  request: TransportRequest,
): TransportResponse {
  const body = decodeBody(request);
  void query;

  if (path === MANAGEMENT_ACCESS_PATH && request.method === 'GET') {
    return gated(actor, 'network.read', () =>
      jsonResponse(200, mockManagementAccessBody(store)),
    );
  }
  if (path === MANAGEMENT_ACCESS_PATH && request.method === 'PUT') {
    return gated(actor, 'network.manage', () => {
      if (
        !Array.isArray(body.interfaces) ||
        !body.interfaces.every((entry) => typeof entry === 'string')
      ) {
        return refusal(400, 'management_access_rejected');
      }
      store.managementInterfaces = [...body.interfaces];
      store.managementState = 'pending';
      return jsonResponse(200, mockManagementAccessBody(store));
    });
  }
  if (path === `${MANAGEMENT_ACCESS_PATH}/confirm` && request.method === 'POST') {
    return gated(actor, 'network.manage', () => {
      store.managementState = 'applied';
      return jsonResponse(200, mockManagementAccessBody(store));
    });
  }
  if (path === `${MANAGEMENT_ACCESS_PATH}/recover` && request.method === 'POST') {
    return gated(actor, 'network.manage', () => {
      store.managementInterfaces = ['enp1s0'];
      store.managementState = 'recovered';
      return jsonResponse(200, mockManagementAccessBody(store));
    });
  }

  if (path === '/api/v1/network/operations' && request.method === 'GET') {
    return gated(actor, 'network.read', () =>
      jsonResponse(200, {
        api_version: BENCH_CORE_API_VERSION,
        items: [...store.operations.values()].reverse(),
      }),
    );
  }
  const settingsPreview = path.match(
    /^\/api\/v1\/network\/interfaces\/([^/]+)\/settings\/preview$/,
  );
  if (settingsPreview && request.method === 'POST') {
    const method = String(body.method ?? '');
    if (method !== 'static' && method !== 'automatic') {
      return refusal(
        400,
        'interface_settings_rejected',
        'method must be static or automatic',
      );
    }
    if (
      method === 'static' &&
      !/^\d+\.\d+\.\d+\.\d+\/\d+$/.test(String(body.address ?? ''))
    ) {
      return refusal(
        400,
        'interface_settings_rejected',
        'a static address needs a prefix length',
      );
    }
    return gated(actor, 'network.manage', () =>
      jsonResponse(
        200,
        store.previewOperation(
          'interface.settings',
          decodeURIComponent(settingsPreview[1]),
        ),
      ),
    );
  }
  const wifi = path.match(
    /^\/api\/v1\/network\/interfaces\/([^/]+)\/wifi(\/preview)?$/,
  );
  if (wifi && wifi[2] === undefined && request.method === 'GET') {
    return gated(actor, 'network.read', () =>
      jsonResponse(200, {
        api_version: BENCH_CORE_API_VERSION,
        interface_id: decodeURIComponent(wifi[1]),
        supported: decodeURIComponent(wifi[1]) === 'sim-wlan0',
        observed_at: new Date().toISOString(),
        networks: MOCK_WIFI_NETWORKS,
      }),
    );
  }
  if (wifi && wifi[2] !== undefined && request.method === 'POST') {
    if (String(body.ssid ?? '') === '') {
      return refusal(
        400,
        'interface_settings_rejected',
        'a join needs the network name',
      );
    }
    return gated(actor, 'network.manage', () =>
      jsonResponse(
        200,
        store.previewOperation('wifi.join', decodeURIComponent(wifi[1])),
      ),
    );
  }

  if (path === '/api/v1/network/device-networks' && request.method === 'GET') {
    return gated(actor, 'network.read', () =>
      jsonResponse(200, {
        api_version: BENCH_CORE_API_VERSION,
        items: [...store.deviceNetworks.values()],
      }),
    );
  }
  if (path === '/api/v1/network/device-networks/preview' && request.method === 'POST') {
    const action = String(body.action ?? '');
    const target = String(body.target ?? '');
    const draft =
      typeof body.device_network === 'object' && body.device_network !== null
        ? (body.device_network as Record<string, unknown>)
        : undefined;
    if (!['create', 'update', 'enable', 'disable', 'delete'].includes(action)) {
      return refusal(400, 'network_action_unsupported');
    }
    if ((action === 'create' || action === 'update') && draft === undefined) {
      return refusal(
        400,
        'device_network_rejected',
        'a create or update needs the device network',
      );
    }
    if (
      draft !== undefined &&
      String(draft.scope ?? 'host') === 'host' &&
      String(draft.target ?? '') === ''
    ) {
      return refusal(
        400,
        'device_network_rejected',
        'a host scope needs its target address',
      );
    }
    if (draft !== undefined && String(draft.name ?? '') === '') {
      return refusal(400, 'device_network_rejected', 'a device network needs a name');
    }
    if (action !== 'create' && !store.deviceNetworks.has(target)) {
      return refusal(404, 'device_network_not_found');
    }
    return gated(actor, 'network.manage', () => {
      const operation = store.previewOperation(
        `device_network.${action}`,
        action === 'create' ? 'new' : target,
      );
      store.pendingDrafts.set(operation.id, { action, target, draft });
      return jsonResponse(200, operation);
    });
  }
  const deviceNetwork = path.match(
    /^\/api\/v1\/network\/device-networks\/([^/]+)(?:\/(checks|access-instructions))?$/,
  );
  if (deviceNetwork) {
    const id = decodeURIComponent(deviceNetwork[1]);
    const held = store.deviceNetworks.get(id);
    if (held === undefined) {
      return refusal(404, 'device_network_not_found');
    }
    if (deviceNetwork[2] === undefined) {
      return gated(actor, 'network.read', () =>
        jsonResponse(200, { api_version: BENCH_CORE_API_VERSION, ...held }),
      );
    }
    if (deviceNetwork[2] === 'checks') {
      return gated(actor, 'network.manage', () =>
        jsonResponse(200, {
          api_version: BENCH_CORE_API_VERSION,
          device_network_id: id,
          observed_at: new Date().toISOString(),
          facts: [
            { name: 'saved', result: 'passed', detail: 'the device network is saved' },
            {
              name: 'forwarding',
              result: held.applied === true ? 'passed' : 'failed',
              detail:
                held.applied === true
                  ? 'the forwarding for this network is installed'
                  : 'the forwarding for this network is not installed',
            },
            {
              name: 'target',
              result: held.applied === true ? 'passed' : 'unknown',
              detail:
                held.applied === true
                  ? 'the simulated device accepted a connection'
                  : 'the probe did not complete; this says nothing about whether the device is online',
            },
          ],
        }),
      );
    }
    return gated(actor, 'network.read', () =>
      jsonResponse(200, {
        api_version: BENCH_CORE_API_VERSION,
        device_network_id: id,
        target_address: String(held.exposed ?? held.target ?? ''),
        steps: [
          {
            machine: 'client',
            text: `Add a route to ${String(held.device_subnet ?? '')} via ${String(held.appliance_address ?? '').split('/')[0]}.`,
          },
          {
            machine: 'client',
            text: `Connect to ${String(held.exposed ?? held.target ?? '')}.`,
          },
        ],
        unmet_prerequisites:
          held.enabled === true ? [] : ['the device network is disabled'],
      }),
    );
  }

  const interfacePreview = path.match(
    /^\/api\/v1\/network\/interfaces\/([^/]+)\/preview$/,
  );
  if (interfacePreview && request.method === 'POST') {
    const action = String(body.action ?? '');
    if (action !== 'enable' && action !== 'disable') {
      return refusal(400, 'network_action_unsupported');
    }
    return gated(actor, 'network.manage', () =>
      jsonResponse(
        200,
        store.previewOperation(
          `interface.${action}`,
          decodeURIComponent(interfacePreview[1]),
        ),
      ),
    );
  }
  if (path === '/api/v1/network/wireguard/preview' && request.method === 'POST') {
    const action = String(body.action ?? '');
    const target = String(body.target ?? '');
    if (action === '' || target === '') {
      return refusal(400, 'network_action_unsupported');
    }
    return gated(actor, 'vpn.manage', () =>
      jsonResponse(200, store.previewOperation(`vpn.${action}`, target)),
    );
  }
  if (
    path === '/api/v1/network/wireguard/install/preview' &&
    request.method === 'POST'
  ) {
    return gated(actor, 'vpn.install', () =>
      jsonResponse(200, store.previewOperation('wireguard.install', 'wireguard')),
    );
  }

  const operation = path.match(/^\/api\/v1\/network\/operations\/([^/]+)(?:\/(\w+))?$/);
  if (operation) {
    const id = decodeURIComponent(operation[1]);
    const act = operation[2];
    const held = store.operations.get(id);
    if (held === undefined) {
      return refusal(404, 'network_operation_not_found');
    }
    if (act === undefined && request.method === 'GET') {
      return gated(actor, 'network.read', () => jsonResponse(200, held));
    }
    // Acting on an operation needs the permission its action requires, which is
    // only known once the operation is resolved — the same rule the Go route
    // table applies rather than a looser one.
    const permission: BenchCorePermission =
      held.action.startsWith('interface.') ||
      held.action.startsWith('device_network.') ||
      held.action === 'wifi.join'
        ? 'network.manage'
        : held.action === 'wireguard.install'
          ? 'vpn.install'
          : 'vpn.manage';
    return gated(actor, permission, () => {
      switch (act) {
        case 'acknowledge': {
          if (String(body.revision ?? '') !== held.revision) {
            return refusal(409, 'network_preview_stale');
          }
          if (held.state !== 'previewed') {
            return refusal(409, 'network_already_acknowledged');
          }
          const applied = store.acknowledgeOperation(id);
          store.applyDeviceNetworkChange(id);
          store.record(
            'audit',
            componentOf(held.action),
            held.action,
            'applied',
            'target',
            held.target,
            actor,
            id,
            {
              state: applied.state,
            },
          );
          return jsonResponse(202, applied);
        }
        case 'confirm': {
          const settled = store.settleOperation(id, 'confirmed');
          store.record(
            'audit',
            componentOf(held.action),
            held.action,
            'confirmed',
            'target',
            held.target,
            actor,
            id,
            {
              state: settled.state,
            },
          );
          return jsonResponse(200, settled);
        }
        case 'rollback': {
          const settled = store.settleOperation(id, 'rolled_back');
          store.record(
            'audit',
            componentOf(held.action),
            held.action,
            'recovered',
            'target',
            held.target,
            actor,
            id,
            {
              state: settled.state,
            },
          );
          return jsonResponse(200, settled);
        }
        default:
          return refusal(404, 'network_operation_not_found');
      }
    });
  }
  return refusal(404, 'not_found');
}

/** Which product function an action belongs to, as the appliance labels it. */
function componentOf(action: string): string {
  if (action.startsWith('device_network.')) return 'network.device_networks';
  if (action.startsWith('vpn.') || action.startsWith('wireguard.'))
    return 'network.wireguard';
  if (action === 'wifi.join') return 'network.wifi';
  return 'network.interfaces';
}

const MOCK_WIFI_NETWORKS = [
  { ssid: 'lab-hotspot', security: 'wpa2-personal', strength: 78, supported: true },
  { ssid: 'open-cafe', security: 'open', strength: 41, supported: true },
  { ssid: 'site-gateway', security: 'wpa3-personal', strength: 63, supported: true },
  { ssid: 'corp-enterprise', security: 'unsupported', strength: 55, supported: false },
];

/** The wire shape of one saved device network built from a submitted draft. */
function mockDeviceNetworkBody(
  id: string,
  draft: Record<string, unknown>,
  enabled: boolean,
  now: string,
): Record<string, unknown> {
  const applianceAddress = String(draft.appliance_address ?? '192.168.0.1/24');
  const [address, prefix] = applianceAddress.split('/');
  const octets = address.split('.');
  const deviceSubnet = `${octets.slice(0, 3).join('.')}.0/${prefix ?? '24'}`;
  const scope = String(draft.scope ?? 'host');
  const addressing = String(draft.addressing ?? 'original');
  const target = String(draft.target ?? '');
  const services =
    typeof draft.services === 'object' && draft.services !== null
      ? (draft.services as Record<string, unknown>)
      : {};
  const exposed =
    addressing === 'translated'
      ? String(draft.exposed ?? '')
      : scope === 'host'
        ? target
        : deviceSubnet;
  return {
    id,
    name: String(draft.name ?? ''),
    location: String(draft.location ?? ''),
    device_interface: String(draft.device_interface ?? ''),
    appliance_address: applianceAddress,
    device_subnet: deviceSubnet,
    scope,
    target,
    addressing,
    exposed,
    access: String(draft.access ?? 'local'),
    local_interface: String(draft.local_interface ?? ''),
    local_sources: Array.isArray(draft.local_sources) ? draft.local_sources : [],
    vpn_profile_id: String(draft.vpn_profile_id ?? ''),
    vpn_sources: Array.isArray(draft.vpn_sources) ? draft.vpn_sources : [],
    services: {
      protocols:
        Array.isArray(services.protocols) && services.protocols.length > 0
          ? services.protocols
          : ['tcp', 'udp', 'icmp'],
      tcp_ports: Array.isArray(services.tcp_ports) ? services.tcp_ports : [],
      udp_ports: Array.isArray(services.udp_ports) ? services.udp_ports : [],
      preserve_source: services.preserve_source === true,
    },
    enabled,
    revision: `revision-${id}-${String(draft.name ?? '')}`,
    state: enabled ? 'active' : 'disabled',
    detail: '',
    applied: enabled,
    observed_at: now,
  };
}

/**
 * The shared history and retention policy, mirroring the Go routes: a filtered
 * page with a sequence cursor, an export in both formats, and the policy.
 */
function observe(
  store: MockAdministration,
  actor: MockActor,
  path: string,
  query: string,
  request: TransportRequest,
): TransportResponse {
  const body = decodeBody(request);
  const parameters = new URLSearchParams(query);
  const filtered = () => {
    const selected = store.activity.filter((event) => {
      const matches = (key: string, field: string) => {
        const wanted = parameters.get(key);
        if (key === 'component' && wanted?.endsWith('.*')) {
          return String(event[field]).startsWith(wanted.slice(0, -1));
        }
        return wanted === null || wanted === '' || String(event[field]) === wanted;
      };
      return (
        matches('category', 'category') &&
        matches('component', 'component') &&
        matches('target', 'subject_id') &&
        matches('actor', 'actor_id') &&
        matches('severity', 'severity') &&
        matches('outcome', 'outcome') &&
        matches('operation', 'operation_id') &&
        matches('action', 'action')
      );
    });
    const sort = parameters.get('sort');
    const order = parameters.get('order') === 'desc' ? -1 : 1;
    const value = (event: Record<string, unknown>) => {
      if (sort === 'time') return String(event.created_at ?? '');
      if (sort === 'activity') return String(event.action ?? '');
      if (sort === 'outcome') return String(event.outcome ?? '');
      if (sort === 'actor')
        return `${String(event.actor_kind ?? '')}\u001f${String(event.actor_id ?? '')}`;
      if (sort === 'target')
        return String(event.target_label || event.subject_id || '');
      return Number(event.sequence ?? 0);
    };
    selected.sort((left, right) => {
      const a = value(left);
      const b = value(right);
      if (a < b) return -1 * order;
      if (a > b) return order;
      return (Number(left.sequence) - Number(right.sequence)) * order;
    });
    return selected;
  };
  if (path === '/api/v1/activity' && request.method === 'GET') {
    return gated(actor, 'activity.read', () => {
      const events = filtered();
      const rawLimit = parameters.get('limit') ?? '100';
      const limit =
        rawLimit === 'all'
          ? events.length
          : Math.min(Math.max(Number(rawLimit), 1), 500);
      const page = Math.max(Number(parameters.get('page') ?? '1'), 1);
      const offset = rawLimit === 'all' ? 0 : (page - 1) * limit;
      const items = events.slice(offset, offset + limit);
      const last = items[items.length - 1];
      return jsonResponse(200, {
        api_version: BENCH_CORE_API_VERSION,
        schema_version: 'nilo.activity/1',
        items,
        next_cursor:
          offset + limit < events.length && last !== undefined
            ? String(last.sequence)
            : '',
        total: events.length,
        retained_window: {
          audit: {
            count: store.activity.filter((event) => event.category === 'audit').length,
            truncated: false,
          },
          event: {
            count: store.activity.filter((event) => event.category === 'event').length,
            truncated: false,
          },
        },
        logging_health: { audit_writable: true, failures: 0 },
      });
    });
  }
  if (path === '/api/v1/activity/export' && request.method === 'GET') {
    return gated(actor, 'activity.export', () => {
      const format = parameters.get('format') ?? 'jsonl';
      const events = filtered();
      const header = {
        schema_version: 'nilo.activity/1',
        appliance_id: 'mock-appliance',
        exported_at: new Date().toISOString(),
        redaction: 'envelope and safe details only',
      };
      const text =
        format === 'csv'
          ? `# schema_version=nilo.activity/1\n# appliance_id=mock-appliance\nsequence,action,outcome\n${events
              .map(
                (event) =>
                  `${String(event.sequence)},${String(event.action)},${String(event.outcome)}`,
              )
              .join('\n')}\n# rows=${events.length} truncated=false\n`
          : [
              header,
              ...events,
              { trailer: true, rows: events.length, truncated: false },
            ]
              .map((line) => JSON.stringify(line))
              .join('\n') + '\n';
      store.record(
        'audit',
        'system',
        'activity.exported',
        'applied',
        'activity',
        'export',
        actor,
        '',
        {
          format,
          rows: events.length,
        },
      );
      return { status: 200, ok: true, text: () => Promise.resolve(text) };
    });
  }
  if (path === '/api/v1/activity/retention' && request.method === 'GET') {
    return gated(actor, 'activity.retention.manage', () =>
      jsonResponse(200, { api_version: BENCH_CORE_API_VERSION, ...store.policy }),
    );
  }
  if (path === '/api/v1/activity/retention' && request.method === 'PUT') {
    return gated(actor, 'activity.retention.manage', () => {
      const days =
        typeof body.retention_days === 'object' && body.retention_days !== null
          ? (body.retention_days as Record<string, number>)
          : {};
      const bytes =
        typeof body.quota_bytes === 'object' && body.quota_bytes !== null
          ? (body.quota_bytes as Record<string, number>)
          : {};
      for (const category of ['audit', 'event']) {
        const retention = days[category];
        const quota = bytes[category];
        if (typeof retention !== 'number' || retention < 1 || retention > 3650) {
          return refusal(
            400,
            'activity_policy_rejected',
            `${category} retention must be between 1 and 3650 days`,
          );
        }
        if (typeof quota !== 'number' || quota < 1048576 || quota > 17179869184) {
          return refusal(
            400,
            'activity_policy_rejected',
            `${category} quota must be between 1 MiB and 16 GiB`,
          );
        }
      }
      const before = store.policy;
      store.policy = { retention_days: { ...days }, quota_bytes: { ...bytes } };
      store.record(
        'audit',
        'system',
        'activity.retention.changed',
        'applied',
        'activity',
        'policy',
        actor,
        '',
        {
          before,
          after: store.policy,
        },
      );
      return jsonResponse(200, {
        api_version: BENCH_CORE_API_VERSION,
        ...store.policy,
      });
    });
  }
  return refusal(404, 'not_found');
}

/**
 * The network reading the mock serves. It reproduces the Go simulated catalog
 * exactly, the way the inventory scenarios already do, so the interface behaves
 * identically against the mock and against a real simulation backend.
 */
const MOCK_NETWORK_STATUS_BODY = {
  api_version: BENCH_CORE_API_VERSION,
  measured_by: 'simulation',
  interfaces: [
    {
      id: 'sim-enp1s0',
      name: 'enp1s0',
      adapter_id: '02:00:00:00:00:01',
      kind: 'ethernet',
      state: 'connected',
      ipv4: ['192.168.10.24/24'],
      ipv6: ['fd00:10::24/64'],
      link_capacity_mbps: 1000,
      rx_bytes_per_second: 12480,
      tx_bytes_per_second: 3120,
      controllable: true,
      not_controllable_reason: '',
    },
    {
      id: 'sim-wlan0',
      name: 'wlan0',
      adapter_id: '02:00:00:00:00:02',
      kind: 'wifi',
      state: 'disconnected',
      ipv4: [],
      ipv6: [],
      link_capacity_mbps: 0,
      rx_bytes_per_second: 0,
      tx_bytes_per_second: 0,
      controllable: true,
      not_controllable_reason: '',
    },
    {
      id: 'sim-lo',
      name: 'lo',
      kind: 'loopback',
      state: 'connected',
      ipv4: ['127.0.0.1/8'],
      ipv6: ['::1/128'],
      link_capacity_mbps: 0,
      rx_bytes_per_second: 0,
      tx_bytes_per_second: 0,
      controllable: false,
      not_controllable_reason: 'the loopback device is not an appliance connection',
    },
  ],
};

/** Management selection with the same stable identities as the mock inventory. */
function mockManagementAccessBody(store: MockAdministration): unknown {
  const deviceClaims = new Map<string, Record<string, unknown>>();
  for (const network of store.deviceNetworks.values()) {
    const adapterID = String(network.device_interface ?? '').toLowerCase();
    if (adapterID !== '') deviceClaims.set(adapterID, network);
  }
  return {
    api_version: BENCH_CORE_API_VERSION,
    state: store.managementState,
    port: 8443,
    interfaces: MOCK_NETWORK_STATUS_BODY.interfaces.map((entry) => {
      const adapterID = 'adapter_id' in entry ? String(entry.adapter_id) : '';
      const device = deviceClaims.get(adapterID.toLowerCase());
      const enabled = store.managementInterfaces.includes(entry.name);
      return {
        name: entry.name,
        adapter_id: adapterID,
        kind: entry.kind,
        state: entry.state,
        ipv4: entry.ipv4,
        ipv6: entry.ipv6,
        enabled,
        current: entry.name === 'enp1s0',
        usable:
          entry.state === 'connected' && entry.ipv4.length + entry.ipv6.length > 0,
        role:
          device === undefined
            ? enabled && adapterID !== ''
              ? 'management'
              : ''
            : 'device',
        role_available: device === undefined,
        role_detail:
          device === undefined
            ? ''
            : `assigned to device network ${String(device.name ?? '')}`,
        conflicting_resource_id: device === undefined ? '' : String(device.id ?? ''),
        conflicting_resource_name:
          device === undefined ? '' : String(device.name ?? ''),
      };
    }),
    configured_interfaces: [...store.managementInterfaces],
    urls: [{ url: 'https://nilo.local:8443', kind: 'local' }],
    tls_names: ['nilo.local'],
    tls_missing: [],
    confirm_by:
      store.managementState === 'pending'
        ? new Date(Date.now() + 120_000).toISOString()
        : null,
    detail: '',
    outcome: { code: '', interface: '', resource_name: '' },
    current_interface: 'enp1s0',
  };
}

/**
 * Build a deterministic series for the requested window.
 *
 * Every fourth minute is deliberately missing. A gap is what an unmeasured
 * minute looks like, and a mock that returned an unbroken series would let the
 * interface be built without ever facing one.
 */
function mockStatisticsFor(rawQuery: string): unknown {
  const query = new URLSearchParams(rawQuery);
  const interfaceId = query.get('interface') ?? '';
  const period = query.get('period') ?? '15m';
  const minutes = period === '24h' ? 24 * 60 : period === '1h' ? 60 : 15;

  // A fixed end instant keeps every mock run identical.
  const to = new Date('2026-09-03T10:00:00.000Z');
  const from = new Date(to.getTime() - minutes * 60_000);

  const points = [];
  for (let index = 0; index < minutes; index += 1) {
    if (index % 4 === 3) {
      continue;
    }
    const minuteStart = new Date(from.getTime() + index * 60_000);
    points.push({
      minute_start: minuteStart.toISOString(),
      rx_bytes: 600_000 + index * 1_500,
      tx_bytes: 150_000 + index * 400,
      rx_errors: 0,
      tx_errors: 0,
      rx_dropped: index % 30 === 0 ? 1 : 0,
      tx_dropped: 0,
      samples: 11,
      stale: false,
    });
  }

  return {
    api_version: BENCH_CORE_API_VERSION,
    interface_id: interfaceId,
    period,
    from: from.toISOString(),
    to: to.toISOString(),
    measured_by: 'simulation',
    stale: false,
    points,
  };
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
  const identityResource = /^\/api\/v1\/users\/([^/]+)$/.exec(path);
  if (identityResource !== null && request.method === 'DELETE') {
    return gated(actor, 'users.manage', () =>
      deleteIdentity(store, decodeURIComponent(identityResource[1])),
    );
  }
  const tokenResource = /^\/api\/v1\/tokens\/([^/]+)$/.exec(path);
  if (tokenResource !== null && request.method === 'DELETE') {
    return gated(actor, 'tokens.manage.all', () =>
      deleteToken(store, decodeURIComponent(tokenResource[1])),
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

/**
 * Remove one identity, with the two refusals the appliance owns.
 *
 * The token check counts revoked tokens too: revoking is not removing, so an
 * identity that owns a revoked token still owns a record that names it.
 */
function deleteIdentity(
  store: MockAdministration,
  username: string,
): TransportResponse {
  const identity = store.find(username);
  if (identity === undefined) {
    return refusal(404, 'user_not_found', 'no identity carries that username');
  }
  if (removesLastAdministrator(store, identity, true)) {
    return lastAdministrator();
  }
  if (store.tokens.some((token) => token.owner === identity.username)) {
    return refusal(
      409,
      'user_has_tokens',
      'delete every API token this identity owns before deleting the identity',
    );
  }
  store.identities = store.identities.filter(
    (candidate) => candidate.id !== identity.id,
  );
  if (store.session?.id === identity.id) {
    // The identity that was signed in is gone, so the session it held is too.
    store.session = null;
  }
  return emptyResponse(204);
}

/** Remove one token record, whatever state it is in. */
function deleteToken(store: MockAdministration, id: string): TransportResponse {
  const token = store.tokens.find((candidate) => candidate.id === id);
  if (token === undefined) {
    return refusal(404, 'token_not_found', 'no API token carries that identifier');
  }
  store.tokens = store.tokens.filter((candidate) => candidate.id !== id);
  return emptyResponse(204);
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

/** The query string of a request URL, without its leading question mark. */
function queryOf(url: string): string {
  const separator = url.indexOf('?');
  return separator === -1 ? '' : url.slice(separator + 1);
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
