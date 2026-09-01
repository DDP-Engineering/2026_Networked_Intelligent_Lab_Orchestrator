import { describe, expect, it } from 'vitest';

import { BenchCoreError, createBenchCoreClient } from './client';
import { BENCH_CORE_API_VERSION } from './contract';
import {
  MOCK_HEALTHY_STATUS_BODY,
  createMockTransport,
  parseMockScenario,
} from './mock';
import type { MockScenario } from './mock';

function mockClient(scenario: MockScenario) {
  return createBenchCoreClient({
    baseUrl: 'http://mock.invalid',
    transport: createMockTransport(scenario),
  });
}

async function failureOf(scenario: MockScenario): Promise<BenchCoreError> {
  try {
    await mockClient(scenario).getStatus('any-development-token');
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

  it('declares no status field the versioned contract does not define', () => {
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
  });

  it('answers the unauthorized scenario with the client unauthorized cause', async () => {
    expect((await failureOf('unauthorized')).failure).toBe('unauthorized');
  });

  it('answers the unavailable scenario with the client unavailable cause', async () => {
    expect((await failureOf('unavailable')).failure).toBe('unavailable');
  });

  it('answers the malformed scenario with the client malformed cause', async () => {
    expect((await failureOf('malformed')).failure).toBe('malformed');
  });

  it('recognises every documented scenario name and rejects anything else', () => {
    expect(parseMockScenario('healthy')).toBe('healthy');
    expect(parseMockScenario('unauthorized')).toBe('unauthorized');
    expect(parseMockScenario('unavailable')).toBe('unavailable');
    expect(parseMockScenario('malformed')).toBe('malformed');
    expect(parseMockScenario('sideways')).toBeNull();
  });
});
