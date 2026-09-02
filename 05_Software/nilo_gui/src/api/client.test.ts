import { describe, expect, it, vi } from 'vitest';

import {
  BenchCoreError,
  bearerCredential,
  createBenchCoreClient,
  sessionCredential,
} from './client';
import type { BenchCoreClient, BenchCoreTransport, TransportResponse } from './client';

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

const integrationsBody = {
  api_version: 'nilo.http/1',
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

const devicesBody = {
  api_version: 'nilo.http/1',
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

/** The three resource methods, so one table can prove a rule for all of them. */
const RESOURCES = [
  { name: 'status', path: '/api/v1/status', body: healthyBody },
  { name: 'integrations', path: '/api/v1/integrations', body: integrationsBody },
  { name: 'devices', path: '/api/v1/devices', body: devicesBody },
] as const;

function callResource(
  client: BenchCoreClient,
  name: (typeof RESOURCES)[number]['name'],
  token: string,
): Promise<unknown> {
  switch (name) {
    case 'status':
      return client.getStatus(bearerCredential(token));
    case 'integrations':
      return client.getIntegrations(bearerCredential(token));
    case 'devices':
      return client.getDevices(bearerCredential(token));
  }
}

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

async function failureOf(
  transport: BenchCoreTransport,
  resource: (typeof RESOURCES)[number]['name'] = 'status',
): Promise<BenchCoreError> {
  const client = createBenchCoreClient({ baseUrl: 'http://127.0.0.1:8090', transport });
  try {
    await callResource(client, resource, 'development-token');
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

    await client.getStatus(bearerCredential('dev-token'));

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

    const status = await client.getStatus(bearerCredential('development-token'));

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

describe('Bench Core integration inventory client', () => {
  it('requests the versioned integration path with the supplied bearer token', async () => {
    const transport = vi.fn<BenchCoreTransport>(() =>
      Promise.resolve(respondWithJson(200, integrationsBody)),
    );
    const client = createBenchCoreClient({
      baseUrl: 'http://127.0.0.1:8090/',
      transport,
    });

    await client.getIntegrations(bearerCredential('dev-token'));

    expect(transport).toHaveBeenCalledTimes(1);
    const [url, request] = transport.mock.calls[0];
    expect(url).toBe('http://127.0.0.1:8090/api/v1/integrations');
    expect(request.headers.Authorization).toBe('Bearer dev-token');
    expect(request.headers.Accept).toBe('application/json');
  });

  it('returns the scenario and every reported integration as typed values', async () => {
    const client = createBenchCoreClient({
      baseUrl: 'http://127.0.0.1:8090',
      transport: () => Promise.resolve(respondWithJson(200, integrationsBody)),
    });

    expect(await client.getIntegrations(bearerCredential('development-token'))).toEqual(
      {
        apiVersion: 'nilo.http/1',
        scenario: 'healthy',
        items: [
          {
            id: 'simulation.fake-python',
            displayName: 'SIMULATION - Fake Python Integration',
            status: 'healthy',
            simulation: true,
            operations: ['artifact', 'crash', 'echo', 'fail', 'sleep'],
          },
        ],
      },
    );
  });

  it('returns an inventory with no entries when the backend reports none', async () => {
    const client = createBenchCoreClient({
      baseUrl: 'http://127.0.0.1:8090',
      transport: () =>
        Promise.resolve(respondWithJson(200, { ...integrationsBody, items: [] })),
    });

    const inventory = await client.getIntegrations(
      bearerCredential('development-token'),
    );

    expect(inventory.items).toEqual([]);
    expect(inventory.scenario).toBe('healthy');
  });

  it('reports a rejected token, an unreachable backend, and a bad body distinctly', async () => {
    expect(
      (
        await failureOf(
          () => Promise.resolve(respondWithJson(401, { error: 'unauthorized' })),
          'integrations',
        )
      ).failure,
    ).toBe('unauthorized');
    expect(
      (
        await failureOf(
          () => Promise.reject(new TypeError('Failed to fetch')),
          'integrations',
        )
      ).failure,
    ).toBe('unavailable');
    expect(
      (
        await failureOf(
          () => Promise.resolve(respondWith(200, '<html>not json</html>')),
          'integrations',
        )
      ).failure,
    ).toBe('malformed');
  });

  it('reports a missing entry field as malformed and names it', async () => {
    const withoutStatus: Record<string, unknown> = { ...integrationsBody.items[0] };
    delete withoutStatus.status;
    const failure = await failureOf(
      () =>
        Promise.resolve(
          respondWithJson(200, { ...integrationsBody, items: [withoutStatus] }),
        ),
      'integrations',
    );

    expect(failure.failure).toBe('malformed');
    expect(failure.detail).toContain('status');
  });

  it('reports a non-array items field as malformed', async () => {
    const failure = await failureOf(
      () =>
        Promise.resolve(
          respondWithJson(200, { ...integrationsBody, items: 'everything' }),
        ),
      'integrations',
    );

    expect(failure.failure).toBe('malformed');
    expect(failure.detail).toContain('items');
  });

  it('reports an operation list holding a non-string as malformed', async () => {
    const failure = await failureOf(
      () =>
        Promise.resolve(
          respondWithJson(200, {
            ...integrationsBody,
            items: [{ ...integrationsBody.items[0], operations: ['echo', 7] }],
          }),
        ),
      'integrations',
    );

    expect(failure.failure).toBe('malformed');
    expect(failure.detail).toContain('operations');
  });

  it('reports an unsupported API version as malformed', async () => {
    const failure = await failureOf(
      () =>
        Promise.resolve(
          respondWithJson(200, { ...integrationsBody, api_version: 'nilo.http/2' }),
        ),
      'integrations',
    );

    expect(failure.failure).toBe('malformed');
    expect(failure.detail).toContain('nilo.http/2');
  });
});

describe('Bench Core device inventory client', () => {
  it('requests the versioned device path with the supplied bearer token', async () => {
    const transport = vi.fn<BenchCoreTransport>(() =>
      Promise.resolve(respondWithJson(200, devicesBody)),
    );
    const client = createBenchCoreClient({
      baseUrl: 'http://127.0.0.1:8090/',
      transport,
    });

    await client.getDevices(bearerCredential('dev-token'));

    expect(transport).toHaveBeenCalledTimes(1);
    const [url, request] = transport.mock.calls[0];
    expect(url).toBe('http://127.0.0.1:8090/api/v1/devices');
    expect(request.headers.Authorization).toBe('Bearer dev-token');
    expect(request.headers.Accept).toBe('application/json');
  });

  it('returns the scenario and every reported device as typed values', async () => {
    const client = createBenchCoreClient({
      baseUrl: 'http://127.0.0.1:8090',
      transport: () => Promise.resolve(respondWithJson(200, devicesBody)),
    });

    expect(await client.getDevices(bearerCredential('development-token'))).toEqual({
      apiVersion: 'nilo.http/1',
      scenario: 'healthy',
      items: [
        {
          id: 'simulation.fake-python.device.stable-001',
          integrationId: 'simulation.fake-python',
          displayName: 'SIMULATION - Electronics Bench 001',
          kind: 'simulated-electronics-bench',
          status: 'ready',
          simulation: true,
          capabilities: ['artifact', 'crash', 'echo', 'fail', 'sleep'],
        },
      ],
    });
  });

  it('returns an inventory with no entries when the backend reports none', async () => {
    const client = createBenchCoreClient({
      baseUrl: 'http://127.0.0.1:8090',
      transport: () =>
        Promise.resolve(respondWithJson(200, { ...devicesBody, items: [] })),
    });

    expect(
      (await client.getDevices(bearerCredential('development-token'))).items,
    ).toEqual([]);
  });

  it('reports a rejected token, an unreachable backend, and a bad body distinctly', async () => {
    expect(
      (
        await failureOf(
          () => Promise.resolve(respondWithJson(401, { error: 'unauthorized' })),
          'devices',
        )
      ).failure,
    ).toBe('unauthorized');
    expect(
      (await failureOf(() => Promise.resolve(respondWith(503, '')), 'devices')).failure,
    ).toBe('unavailable');
    expect(
      (
        await failureOf(
          () => Promise.resolve(respondWith(200, 'not json at all')),
          'devices',
        )
      ).failure,
    ).toBe('malformed');
  });

  it('reports a missing owning integration as malformed and names it', async () => {
    const orphaned: Record<string, unknown> = { ...devicesBody.items[0] };
    delete orphaned.integration_id;
    const failure = await failureOf(
      () =>
        Promise.resolve(respondWithJson(200, { ...devicesBody, items: [orphaned] })),
      'devices',
    );

    expect(failure.failure).toBe('malformed');
    expect(failure.detail).toContain('integration_id');
  });

  it('reports a non-object entry as malformed', async () => {
    const failure = await failureOf(
      () =>
        Promise.resolve(respondWithJson(200, { ...devicesBody, items: ['bench-001'] })),
      'devices',
    );

    expect(failure.failure).toBe('malformed');
  });

  it('reports a missing scenario as malformed', async () => {
    const withoutScenario: Record<string, unknown> = { ...devicesBody };
    delete withoutScenario.scenario;
    const failure = await failureOf(
      () => Promise.resolve(respondWithJson(200, withoutScenario)),
      'devices',
    );

    expect(failure.failure).toBe('malformed');
    expect(failure.detail).toContain('scenario');
  });
});

describe('every Bench Core resource', () => {
  it.each(RESOURCES)(
    'sends the $name resource to $path and nowhere else',
    async ({ name, path, body }) => {
      const transport = vi.fn<BenchCoreTransport>(() =>
        Promise.resolve(respondWithJson(200, body)),
      );
      const client = createBenchCoreClient({ baseUrl: '', transport });

      await callResource(client, name, 'development-token');

      const [url] = transport.mock.calls[0];
      expect(url).toBe(path);
    },
  );

  it.each(RESOURCES)(
    'never puts the $name resource token anywhere but the Authorization header',
    async ({ name, body }) => {
      const transport = vi.fn<BenchCoreTransport>(() =>
        Promise.resolve(respondWithJson(200, body)),
      );
      const client = createBenchCoreClient({ baseUrl: '', transport });

      // Deliberately short: the repository secret scanner matches a long run
      // of token characters after "bearer", and a fixture is not a credential.
      await callResource(client, name, 'fixture-tok');

      const [url, request] = transport.mock.calls[0];
      expect(url).not.toContain('fixture-tok');
      expect(request.headers.Authorization).toBe('Bearer fixture-tok');
    },
  );
});

describe('credentials on the wire', () => {
  /** Record every request one call made, so a URL can be inspected. */
  function recording(status = 200, body: unknown = {}) {
    return vi.fn<BenchCoreTransport>(() =>
      Promise.resolve(respondWithJson(status, body)),
    );
  }

  const sessionBody = {
    api_version: 'nilo.http/1',
    username: 'admin',
    role: 'administrator',
    permissions: ['users.manage'],
    credential: 'session',
    csrf_token: 'csrf-value',
  };

  it('sends a password in one request body and never in the URL', async () => {
    const transport = recording(200, sessionBody);
    const client = createBenchCoreClient({ baseUrl: '', transport });

    await client.signIn('admin', 'fixture-pass-word');

    expect(transport).toHaveBeenCalledTimes(1);
    const [url, request] = transport.mock.calls[0];
    expect(url).toBe('/api/v1/auth/login');
    expect(url).not.toContain('fixture-pass-word');
    expect(request.method).toBe('POST');
    expect(request.body).toContain('fixture-pass-word');
    expect(JSON.stringify(request.headers)).not.toContain('fixture-pass-word');
  });

  it('sends a replacement password in the body of the reset request only', async () => {
    const transport = recording(204);
    const client = createBenchCoreClient({ baseUrl: '', transport });

    await client.resetIdentityPassword(
      sessionCredential('csrf-value'),
      'jane',
      'fixture-pass-word',
    );

    const [url, request] = transport.mock.calls[0];
    expect(url).toBe('/api/v1/users/jane/password');
    expect(url).not.toContain('fixture-pass-word');
    expect(request.body).toContain('fixture-pass-word');
  });

  it('repeats the CSRF token in a header on a state change and not on a read', async () => {
    const transport = recording(204);
    const client = createBenchCoreClient({ baseUrl: '', transport });
    const credential = sessionCredential('csrf-value');

    await client.setIdentityRole(credential, 'jane', 'viewer');
    const [changeUrl, change] = transport.mock.calls[0];
    expect(change.headers['X-NILO-CSRF']).toBe('csrf-value');
    expect(change.headers.Authorization).toBeUndefined();
    expect(changeUrl).not.toContain('csrf-value');

    const listing = recording(200, { api_version: 'nilo.http/1', items: [] });
    await createBenchCoreClient({ baseUrl: '', transport: listing }).listIdentities(
      credential,
    );
    const [readUrl, read] = listing.mock.calls[0];
    expect(read.headers['X-NILO-CSRF']).toBeUndefined();
    expect(readUrl).not.toContain('csrf-value');
  });

  it('classifies a refusal by its status and carries the reason it was given', async () => {
    for (const [status, failure] of [
      [401, 'unauthorized'],
      [403, 'forbidden'],
      [400, 'rejected'],
      [404, 'rejected'],
      [409, 'rejected'],
      [500, 'unavailable'],
    ] as const) {
      const client = createBenchCoreClient({
        baseUrl: '',
        transport: () =>
          Promise.resolve(
            respondWithJson(status, { error: 'a_code', message: 'a fixed message' }),
          ),
      });
      const error = await client.listIdentities(sessionCredential('csrf-value')).then(
        () => null,
        (thrown: unknown) => thrown as BenchCoreError,
      );

      expect(error?.failure).toBe(failure);
      if (status !== 401 && status !== 500) {
        expect(error?.detail).toBe('a fixed message');
        expect(error?.code).toBe('a_code');
      }
    }
  });
});
