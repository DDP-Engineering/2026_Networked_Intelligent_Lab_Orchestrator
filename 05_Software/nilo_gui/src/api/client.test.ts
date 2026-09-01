import { describe, expect, it, vi } from 'vitest';

import { BenchCoreError, createBenchCoreClient } from './client';
import type { BenchCoreTransport, TransportResponse } from './client';

const healthyBody = {
  api_version: 'nilo.http/1',
  status: 'healthy',
  profile: 'simulation',
  simulation: true,
  build: {
    name: 'nilo',
    version: '0.1.0',
    integration_protocol: 'nilo.integration/1',
  },
};

function respondWith(status: number, body: string): TransportResponse {
  return {
    status,
    ok: status >= 200 && status < 300,
    text: () => Promise.resolve(body),
  };
}

function respondWithJson(status: number, body: unknown): TransportResponse {
  return respondWith(status, JSON.stringify(body));
}

async function failureOf(transport: BenchCoreTransport): Promise<BenchCoreError> {
  const client = createBenchCoreClient({ baseUrl: 'http://127.0.0.1:8090', transport });
  try {
    await client.getStatus('development-token');
  } catch (error) {
    if (error instanceof BenchCoreError) {
      return error;
    }
    throw error;
  }
  throw new Error('the client resolved instead of failing');
}

describe('Bench Core status client', () => {
  it('requests the versioned status path with the supplied bearer token', async () => {
    const transport = vi.fn<BenchCoreTransport>(() =>
      Promise.resolve(respondWithJson(200, healthyBody)),
    );
    const client = createBenchCoreClient({
      baseUrl: 'http://127.0.0.1:8090/',
      transport,
    });

    await client.getStatus('dev-token');

    expect(transport).toHaveBeenCalledTimes(1);
    const [url, request] = transport.mock.calls[0];
    expect(url).toBe('http://127.0.0.1:8090/api/v1/status');
    expect(request.headers.Authorization).toBe('Bearer dev-token');
    expect(request.headers.Accept).toBe('application/json');
  });

  it('returns the profile, status, simulation flag, and build identity', async () => {
    const client = createBenchCoreClient({
      baseUrl: 'http://127.0.0.1:8090',
      transport: () => Promise.resolve(respondWithJson(200, healthyBody)),
    });

    const status = await client.getStatus('development-token');

    expect(status).toEqual({
      apiVersion: 'nilo.http/1',
      status: 'healthy',
      profile: 'simulation',
      simulation: true,
      build: {
        name: 'nilo',
        version: '0.1.0',
        integrationProtocol: 'nilo.integration/1',
      },
    });
  });

  it('exposes the configured base URL without a trailing separator', () => {
    const client = createBenchCoreClient({
      baseUrl: 'http://127.0.0.1:8090/',
      transport: () => Promise.resolve(respondWithJson(200, healthyBody)),
    });

    expect(client.baseUrl).toBe('http://127.0.0.1:8090');
  });

  it('reports a rejected token as unauthorized', async () => {
    const failure = await failureOf(() =>
      Promise.resolve(respondWithJson(401, { error: 'unauthorized' })),
    );

    expect(failure.failure).toBe('unauthorized');
  });

  it('reports an unreachable backend as unavailable', async () => {
    const failure = await failureOf(() =>
      Promise.reject(new TypeError('Failed to fetch')),
    );

    expect(failure.failure).toBe('unavailable');
    expect(failure.detail).toContain('Failed to fetch');
  });

  it('reports a non-success response other than 401 as unavailable', async () => {
    const failure = await failureOf(() => Promise.resolve(respondWith(503, '')));

    expect(failure.failure).toBe('unavailable');
    expect(failure.detail).toContain('503');
  });

  it('reports a body that is not JSON as malformed', async () => {
    const failure = await failureOf(() =>
      Promise.resolve(respondWith(200, '<html>not json</html>')),
    );

    expect(failure.failure).toBe('malformed');
  });

  it('reports a missing contract field as malformed', async () => {
    const withoutProfile: Record<string, unknown> = { ...healthyBody };
    delete withoutProfile.profile;
    const failure = await failureOf(() =>
      Promise.resolve(respondWithJson(200, withoutProfile)),
    );

    expect(failure.failure).toBe('malformed');
    expect(failure.detail).toContain('profile');
  });

  it('reports an unsupported API version as malformed', async () => {
    const failure = await failureOf(() =>
      Promise.resolve(
        respondWithJson(200, { ...healthyBody, api_version: 'nilo.http/2' }),
      ),
    );

    expect(failure.failure).toBe('malformed');
    expect(failure.detail).toContain('nilo.http/2');
  });
});
