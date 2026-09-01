import { describe, expect, it } from 'vitest';

import { BenchCoreError, createBenchCoreClient } from './client';
import {
  BENCH_CORE_API_VERSION,
  DEVICES_PATH,
  INTEGRATIONS_PATH,
  STATUS_PATH,
} from './contract';
import {
  MOCK_HEALTHY_DEVICES_BODY,
  MOCK_HEALTHY_INTEGRATIONS_BODY,
  MOCK_HEALTHY_STATUS_BODY,
  MOCK_SCENARIOS,
  createMockTransport,
  parseMockScenario,
} from './mock';
import type { MockScenario } from './mock';

type Resource = 'status' | 'integrations' | 'devices';

function mockClient(scenario: MockScenario) {
  return createBenchCoreClient({
    baseUrl: '',
    transport: createMockTransport(scenario),
  });
}

function callResource(scenario: MockScenario, resource: Resource): Promise<unknown> {
  const client = mockClient(scenario);
  const token = 'any-development-token';
  switch (resource) {
    case 'status':
      return client.getStatus(token);
    case 'integrations':
      return client.getIntegrations(token);
    case 'devices':
      return client.getDevices(token);
  }
}

async function failureOf(
  scenario: MockScenario,
  resource: Resource = 'status',
): Promise<BenchCoreError> {
  try {
    await callResource(scenario, resource);
  } catch (error) {
    if (error instanceof BenchCoreError) {
      return error;
    }
    throw error;
  }
  throw new Error('the mock scenario resolved instead of failing');
}

describe('Bench Core mock transport', () => {
  it('answers the healthy scenario with a simulation status the real client accepts', async () => {
    const status = await mockClient('healthy').getStatus('any-development-token');

    expect(status.apiVersion).toBe(BENCH_CORE_API_VERSION);
    expect(status.profile).toBe('simulation');
    expect(status.simulation).toBe(true);
    expect(status.status).toBe('healthy');
    expect(status.build.name).toBe('nilo');
    expect(status.build.integrationProtocol).toBe('nilo.integration/1');
  });

  it('declares no field the versioned contract does not define', () => {
    expect(Object.keys(MOCK_HEALTHY_STATUS_BODY).sort()).toEqual([
      'api_version',
      'build',
      'profile',
      'simulation',
      'status',
    ]);
    expect(Object.keys(MOCK_HEALTHY_STATUS_BODY.build).sort()).toEqual([
      'integration_protocol',
      'name',
      'version',
    ]);
    expect(Object.keys(MOCK_HEALTHY_INTEGRATIONS_BODY).sort()).toEqual([
      'api_version',
      'items',
      'scenario',
    ]);
    expect(Object.keys(MOCK_HEALTHY_INTEGRATIONS_BODY.items[0]).sort()).toEqual([
      'display_name',
      'id',
      'operations',
      'simulation',
      'status',
    ]);
    expect(Object.keys(MOCK_HEALTHY_DEVICES_BODY.items[0]).sort()).toEqual([
      'capabilities',
      'display_name',
      'id',
      'integration_id',
      'kind',
      'simulation',
      'status',
    ]);
  });

  it('answers the unauthorized scenario with the client unauthorized cause', async () => {
    expect((await failureOf('unauthorized')).failure).toBe('unauthorized');
  });

  it('answers the unavailable scenario with the client unavailable cause', async () => {
    expect((await failureOf('unavailable')).failure).toBe('unavailable');
  });

  it('answers the malformed-status scenario with the client malformed cause', async () => {
    expect((await failureOf('malformed-status')).failure).toBe('malformed');
  });

  it('recognises every documented scenario name and rejects anything else', () => {
    for (const scenario of MOCK_SCENARIOS) {
      expect(parseMockScenario(scenario)).toBe(scenario);
    }
    expect(parseMockScenario('sideways')).toBeNull();
  });

  it('keeps the earlier malformed name working as the status-malformed scenario', () => {
    expect(parseMockScenario('malformed')).toBe('malformed-status');
  });
});

describe('mock request routing', () => {
  it.each([
    ['status', STATUS_PATH],
    ['integrations', INTEGRATIONS_PATH],
    ['devices', DEVICES_PATH],
  ] as const)('routes the %s resource by its own path', async (resource, path) => {
    const requested: string[] = [];
    const transport = createMockTransport('healthy');
    const client = createBenchCoreClient({
      baseUrl: '',
      transport: (url, request) => {
        requested.push(url);
        return transport(url, request);
      },
    });

    await (resource === 'status'
      ? client.getStatus('t')
      : resource === 'integrations'
        ? client.getIntegrations('t')
        : client.getDevices('t'));

    expect(requested).toEqual([path]);
  });

  it('refuses a path it does not serve, rather than answering it by accident', async () => {
    const response = await createMockTransport('healthy')('/api/v1/operations', {
      headers: {},
    });

    expect(response.ok).toBe(false);
    expect(response.status).toBe(404);
  });

  it.each(['unauthorized', 'unavailable'] as const)(
    'applies the %s scenario to every resource alike',
    async (scenario) => {
      for (const resource of ['status', 'integrations', 'devices'] as const) {
        const failure = await failureOf(scenario, resource);
        expect(failure.failure).toBe(
          scenario === 'unauthorized' ? 'unauthorized' : 'unavailable',
        );
      }
    },
  );
});

describe('mock inventory scenarios', () => {
  it('reproduces the simulation backend ready inventory in the healthy scenario', async () => {
    const integrations = await mockClient('healthy').getIntegrations('t');
    const devices = await mockClient('healthy').getDevices('t');

    expect(integrations.scenario).toBe('healthy');
    expect(integrations.items).toEqual([
      {
        id: 'simulation.fake-python',
        displayName: 'SIMULATION - Fake Python Integration',
        status: 'healthy',
        simulation: true,
        operations: ['artifact', 'crash', 'echo', 'fail', 'sleep'],
      },
    ]);
    expect(devices.scenario).toBe('healthy');
    expect(devices.items).toEqual([
      {
        id: 'simulation.fake-python.device.stable-001',
        integrationId: 'simulation.fake-python',
        displayName: 'SIMULATION - Electronics Bench 001',
        kind: 'simulated-electronics-bench',
        status: 'ready',
        simulation: true,
        capabilities: ['artifact', 'crash', 'echo', 'fail', 'sleep'],
      },
    ]);
  });

  it('reproduces the simulation backend unavailable inventory, with the backend still answering', async () => {
    const status = await mockClient('unavailable-inventory').getStatus('t');
    const integrations = await mockClient('unavailable-inventory').getIntegrations('t');
    const devices = await mockClient('unavailable-inventory').getDevices('t');

    expect(status.status).toBe('healthy');
    expect(integrations.scenario).toBe('unavailable');
    expect(integrations.items[0].status).toBe('unavailable');
    expect(devices.scenario).toBe('unavailable');
    expect(devices.items[0].status).toBe('unavailable');
    expect(devices.items[0].integrationId).toBe(integrations.items[0].id);
  });

  it('answers the empty scenario with valid inventories holding no entries', async () => {
    const integrations = await mockClient('empty').getIntegrations('t');
    const devices = await mockClient('empty').getDevices('t');

    expect(integrations.items).toEqual([]);
    expect(devices.items).toEqual([]);
    expect(integrations.apiVersion).toBe(BENCH_CORE_API_VERSION);
    expect(await mockClient('empty').getStatus('t')).toHaveProperty(
      'status',
      'healthy',
    );
  });

  it('spoils only the integration inventory in the malformed-integrations scenario', async () => {
    expect((await failureOf('malformed-integrations', 'integrations')).failure).toBe(
      'malformed',
    );
    await expect(
      callResource('malformed-integrations', 'status'),
    ).resolves.toBeDefined();
    await expect(
      callResource('malformed-integrations', 'devices'),
    ).resolves.toBeDefined();
  });

  it('spoils only the device inventory in the malformed-devices scenario', async () => {
    expect((await failureOf('malformed-devices', 'devices')).failure).toBe('malformed');
    await expect(callResource('malformed-devices', 'status')).resolves.toBeDefined();
    await expect(
      callResource('malformed-devices', 'integrations'),
    ).resolves.toBeDefined();
  });

  it('spoils only the status in the malformed-status scenario', async () => {
    expect((await failureOf('malformed-status', 'status')).failure).toBe('malformed');
    await expect(
      callResource('malformed-status', 'integrations'),
    ).resolves.toBeDefined();
    await expect(callResource('malformed-status', 'devices')).resolves.toBeDefined();
  });
});
