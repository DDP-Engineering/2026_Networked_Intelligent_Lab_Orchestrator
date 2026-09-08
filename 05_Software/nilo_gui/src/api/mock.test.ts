import { describe, expect, it } from 'vitest';

import {
  BenchCoreError,
  bearerCredential,
  createBenchCoreClient,
  sessionCredential,
} from './client';
import {
  BENCH_CORE_API_VERSION,
  DEVICES_PATH,
  INTEGRATIONS_PATH,
  STATUS_PATH,
} from './contract';
import {
  MOCK_DEVELOPMENT_PASSWORD,
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
      return client.getStatus(bearerCredential(token));
    case 'integrations':
      return client.getIntegrations(bearerCredential(token));
    case 'devices':
      return client.getDevices(bearerCredential(token));
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
  it('keeps simulation power control inert even for an administrator session', async () => {
    const client = mockClient('healthy');
    const session = await client.signIn('admin', MOCK_DEVELOPMENT_PASSWORD);

    const error = await client
      .requestPower(sessionCredential(session.csrfToken ?? ''), 'restart')
      .then(
        () => null,
        (failure: unknown) => failure,
      );

    expect(error).toBeInstanceOf(BenchCoreError);
    expect((error as BenchCoreError).failure).toBe('unavailable');
    expect((error as BenchCoreError).code).toBe('power_control_unavailable');
  });

  it('answers the healthy scenario with a simulation status the real client accepts', async () => {
    const status = await mockClient('healthy').getStatus(
      bearerCredential('any-development-token'),
    );

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
      ? client.getStatus(bearerCredential('t'))
      : resource === 'integrations'
        ? client.getIntegrations(bearerCredential('t'))
        : client.getDevices(bearerCredential('t')));

    expect(requested).toEqual([path]);
  });

  it('refuses a path it does not serve, rather than answering it by accident', async () => {
    const response = await createMockTransport('healthy')('/api/v1/operations', {
      method: 'GET',
      headers: { Authorization: 'Bearer any-development-token' },
      credentials: 'same-origin',
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
    const integrations = await mockClient('healthy').getIntegrations(
      bearerCredential('t'),
    );
    const devices = await mockClient('healthy').getDevices(bearerCredential('t'));

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
    const status = await mockClient('unavailable-inventory').getStatus(
      bearerCredential('t'),
    );
    const integrations = await mockClient('unavailable-inventory').getIntegrations(
      bearerCredential('t'),
    );
    const devices = await mockClient('unavailable-inventory').getDevices(
      bearerCredential('t'),
    );

    expect(status.status).toBe('healthy');
    expect(integrations.scenario).toBe('unavailable');
    expect(integrations.items[0].status).toBe('unavailable');
    expect(devices.scenario).toBe('unavailable');
    expect(devices.items[0].status).toBe('unavailable');
    expect(devices.items[0].integrationId).toBe(integrations.items[0].id);
  });

  it('answers the empty scenario with valid inventories holding no entries', async () => {
    const integrations = await mockClient('empty').getIntegrations(
      bearerCredential('t'),
    );
    const devices = await mockClient('empty').getDevices(bearerCredential('t'));

    expect(integrations.items).toEqual([]);
    expect(devices.items).toEqual([]);
    expect(integrations.apiVersion).toBe(BENCH_CORE_API_VERSION);
    expect(await mockClient('empty').getStatus(bearerCredential('t'))).toHaveProperty(
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

describe('mock identity and token administration', () => {
  const administrator = sessionCredential('mock-csrf-token');

  /** Sign in against one mock transport and keep using that same transport. */
  async function signedInMock(username = 'admin') {
    const client = mockClient('healthy');
    const session = await client.signIn(username, MOCK_DEVELOPMENT_PASSWORD);
    return { client, session };
  }

  it('refuses every resource until somebody signs in', async () => {
    const client = mockClient('healthy');

    await expect(client.currentSession(administrator)).rejects.toMatchObject({
      failure: 'unauthorized',
    });
  });

  it('signs a seeded identity in and reports its role and permissions', async () => {
    const { session } = await signedInMock('admin');

    expect(session.username).toBe('admin');
    expect(session.role).toBe('administrator');
    expect(session.permissions).toContain('users.manage');
    expect(session.csrfToken).toBe('mock-csrf-token');
  });

  it('refuses a wrong password without disclosing which part was wrong', async () => {
    const client = mockClient('healthy');

    await expect(client.signIn('admin', 'not-the-password')).rejects.toMatchObject({
      failure: 'unauthorized',
    });
    await expect(
      client.signIn('nobody', MOCK_DEVELOPMENT_PASSWORD),
    ).rejects.toMatchObject({ failure: 'unauthorized' });
  });

  it('restores the signed-in identity from the current-session route', async () => {
    const { client } = await signedInMock('admin');

    expect((await client.currentSession(administrator)).username).toBe('admin');
  });

  it('refuses a state-changing session request that repeats no CSRF token', async () => {
    const { client } = await signedInMock('admin');

    await expect(
      client.setIdentityRole(
        sessionCredential('the-wrong-token'),
        'viewer',
        'operator',
      ),
    ).rejects.toMatchObject({ failure: 'forbidden' });
  });

  it('administers identities and tokens for an administrator', async () => {
    const { client } = await signedInMock('admin');

    await client.createIdentity(administrator, {
      username: 'jane',
      role: 'operator',
      password: 'a-long-enough-password',
    });
    expect(
      (await client.listIdentities(administrator)).items.map((entry) => entry.username),
    ).toContain('jane');

    await client.setIdentityRole(administrator, 'jane', 'viewer');
    await client.setIdentityEnabled(administrator, 'jane', false);
    const jane = (await client.listIdentities(administrator)).items.find(
      (entry) => entry.username === 'jane',
    );
    expect(jane?.role).toBe('viewer');
    expect(jane?.enabled).toBe(false);

    const issued = await client.createToken(administrator, {
      owner: 'jane',
      name: 'nightly',
      scopes: ['status.read'],
      expiresAt: null,
    });
    expect(issued.secret).not.toBe('');
    await client.revokeToken(administrator, issued.token.id);
    const revoked = (await client.listTokens(administrator)).items.find(
      (entry) => entry.id === issued.token.id,
    );
    expect(revoked?.revoked).toBe(true);
  });

  it('refuses administration for an identity without the permission', async () => {
    const { client } = await signedInMock('operator');

    await expect(client.listIdentities(administrator)).rejects.toMatchObject({
      failure: 'forbidden',
    });
    await expect(
      client.setIdentityRole(administrator, 'viewer', 'administrator'),
    ).rejects.toMatchObject({ failure: 'forbidden' });
  });

  it('refuses administration for a development bearer token, as the backend does', async () => {
    const client = mockClient('healthy');

    await expect(
      client.listIdentities(bearerCredential('any-development-token')),
    ).rejects.toMatchObject({ failure: 'forbidden' });
    await expect(
      client.getStatus(bearerCredential('any-development-token')),
    ).resolves.toBeDefined();
  });

  it('keeps the last enabled administrator, as the backend does', async () => {
    const { client } = await signedInMock('admin');

    await expect(
      client.setIdentityRole(administrator, 'admin', 'viewer'),
    ).rejects.toMatchObject({ failure: 'rejected', code: 'last_administrator' });
  });

  it('deletes an identity permanently once nothing depends on it', async () => {
    const { client } = await signedInMock('admin');

    await client.createIdentity(administrator, {
      username: 'leaving',
      role: 'operator',
      password: 'a-long-enough-password',
    });
    await client.deleteIdentity(administrator, 'leaving');

    expect(
      (await client.listIdentities(administrator)).items.map((entry) => entry.username),
    ).not.toContain('leaving');
    await expect(client.deleteIdentity(administrator, 'leaving')).rejects.toMatchObject(
      { failure: 'rejected', code: 'user_not_found' },
    );
  });

  it('refuses to delete an identity that still owns a token, as the backend does', async () => {
    const { client } = await signedInMock('admin');

    await client.createIdentity(administrator, {
      username: 'holder',
      role: 'operator',
      password: 'a-long-enough-password',
    });
    const issued = await client.createToken(administrator, {
      owner: 'holder',
      name: 'held',
      scopes: [],
      expiresAt: null,
    });

    await expect(client.deleteIdentity(administrator, 'holder')).rejects.toMatchObject({
      failure: 'rejected',
      code: 'user_has_tokens',
    });
    // Revoking is not removing: the record still names this owner.
    await client.revokeToken(administrator, issued.token.id);
    await expect(client.deleteIdentity(administrator, 'holder')).rejects.toMatchObject({
      code: 'user_has_tokens',
    });

    await client.deleteToken(administrator, issued.token.id);
    await expect(
      client.deleteIdentity(administrator, 'holder'),
    ).resolves.toBeUndefined();
  });

  it('refuses to delete the last enabled administrator, as the backend does', async () => {
    const { client } = await signedInMock('admin');

    await expect(client.deleteIdentity(administrator, 'admin')).rejects.toMatchObject({
      failure: 'rejected',
      code: 'last_administrator',
    });
  });

  it('deletes a token whatever state it is in, and only once', async () => {
    const { client } = await signedInMock('admin');

    const issued = await client.createToken(administrator, {
      owner: 'viewer',
      name: 'retired',
      scopes: [],
      expiresAt: null,
    });
    await client.revokeToken(administrator, issued.token.id);

    await client.deleteToken(administrator, issued.token.id);
    expect(
      (await client.listTokens(administrator)).items.map((entry) => entry.id),
    ).not.toContain(issued.token.id);
    await expect(
      client.deleteToken(administrator, issued.token.id),
    ).rejects.toMatchObject({ failure: 'rejected', code: 'token_not_found' });
  });

  it('refuses both deletions for an identity without the permission', async () => {
    const { client } = await signedInMock('operator');

    await expect(client.deleteIdentity(administrator, 'viewer')).rejects.toMatchObject({
      failure: 'forbidden',
    });
    await expect(
      client.deleteToken(administrator, 'mock-token-1'),
    ).rejects.toMatchObject({ failure: 'forbidden' });
  });

  it('refuses an unknown scope and an expiry that has already passed', async () => {
    const { client } = await signedInMock('admin');

    await expect(
      client.createToken(administrator, {
        owner: 'viewer',
        name: 'bad',
        scopes: ['hardware.flash'],
        expiresAt: null,
      }),
    ).rejects.toMatchObject({ code: 'invalid_scope' });
    await expect(
      client.createToken(administrator, {
        owner: 'viewer',
        name: 'bad',
        scopes: [],
        expiresAt: '2020-01-01T00:00:00Z',
      }),
    ).rejects.toMatchObject({ code: 'invalid_expiry' });
  });
});
