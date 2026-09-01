import type { BenchCoreTransport, TransportResponse } from './client';
import {
  BENCH_CORE_API_VERSION,
  DEVICES_PATH,
  INTEGRATIONS_PATH,
  STATUS_PATH,
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
 * serves. This exists so the interface can be developed and tested on Windows
 * with no WSL2 backend running; it is never reachable in live mode.
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

/** Build a transport that answers every resource according to one scenario. */
export function createMockTransport(scenario: MockScenario): BenchCoreTransport {
  return (url) => {
    if (scenario === 'unauthorized') {
      return Promise.resolve(jsonResponse(401, { error: 'unauthorized' }));
    }
    if (scenario === 'unavailable') {
      return Promise.reject(
        new TypeError('the mock Bench Core backend is configured as unreachable'),
      );
    }

    if (url.endsWith(STATUS_PATH)) {
      return Promise.resolve(
        jsonResponse(
          200,
          scenario === 'malformed-status'
            ? {
                api_version: BENCH_CORE_API_VERSION,
                status: 'healthy',
                simulation: true,
              }
            : MOCK_HEALTHY_STATUS_BODY,
        ),
      );
    }
    if (url.endsWith(INTEGRATIONS_PATH)) {
      return Promise.resolve(jsonResponse(200, integrationsFor(scenario)));
    }
    if (url.endsWith(DEVICES_PATH)) {
      return Promise.resolve(jsonResponse(200, devicesFor(scenario)));
    }

    // An unrouted path is a mistake in the caller, not a resource with no
    // entries; answering it with a body would hide that.
    return Promise.resolve(jsonResponse(404, { error: 'no such mock resource' }));
  };
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

function jsonResponse(status: number, body: unknown): TransportResponse {
  const encoded = JSON.stringify(body);
  return {
    status,
    ok: status >= 200 && status < 300,
    text: () => Promise.resolve(encoded),
  };
}
