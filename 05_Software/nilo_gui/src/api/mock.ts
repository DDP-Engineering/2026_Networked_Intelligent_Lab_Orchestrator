import type { BenchCoreTransport, TransportResponse } from './client';
import { BENCH_CORE_API_VERSION, type StatusResponseBody } from './contract';

/**
 * Contract-aligned mock answers for the versioned Bench Core status resource.
 *
 * The bodies below are typed against the same contract the production client
 * parses, so a field the backend renames stops compiling here too. This exists
 * so the interface can be developed and tested on Windows with no WSL2 backend
 * running; it is never reachable in live mode.
 */

/** The mock behaviors a developer can select. */
export type MockScenario = 'healthy' | 'unauthorized' | 'unavailable' | 'malformed';

/** Every scenario name, in the order they are documented. */
export const MOCK_SCENARIOS: readonly MockScenario[] = [
  'healthy',
  'unauthorized',
  'unavailable',
  'malformed',
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

/** Return the named scenario, or null when the name is not one of ours. */
export function parseMockScenario(value: string): MockScenario | null {
  return MOCK_SCENARIOS.find((scenario) => scenario === value) ?? null;
}

/** Build a transport that always answers with the given scenario. */
export function createMockTransport(scenario: MockScenario): BenchCoreTransport {
  return () => {
    switch (scenario) {
      case 'healthy':
        return Promise.resolve(jsonResponse(200, MOCK_HEALTHY_STATUS_BODY));
      case 'unauthorized':
        return Promise.resolve(jsonResponse(401, { error: 'unauthorized' }));
      case 'unavailable':
        return Promise.reject(
          new TypeError('the mock Bench Core backend is configured as unreachable'),
        );
      case 'malformed':
        return Promise.resolve(
          jsonResponse(200, {
            api_version: BENCH_CORE_API_VERSION,
            status: 'healthy',
            simulation: true,
          }),
        );
    }
  };
}

function jsonResponse(status: number, body: unknown): TransportResponse {
  const encoded = JSON.stringify(body);
  return {
    status,
    ok: status >= 200 && status < 300,
    text: () => Promise.resolve(encoded),
  };
}
