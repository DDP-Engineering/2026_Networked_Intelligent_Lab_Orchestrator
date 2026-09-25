import { describe, expect, it, vi } from 'vitest';

import {
  createBenchCoreClient,
  sessionCredential,
  type BenchCoreTransport,
} from './client';
import { createMockTransport, MOCK_DEVELOPMENT_PASSWORD } from './mock';
import {
  BENCH_CORE_API_VERSION,
  BENCH_CORE_PERMISSIONS,
  NETWORK_STATISTICS_PATH,
  NETWORK_STATUS_PATH,
  PERMISSION_NETWORK_MANAGE,
  PERMISSION_NETWORK_READ,
  PERMISSION_VPN_INSTALL,
  PERMISSION_VPN_MANAGE,
  networkStatisticsPath,
  parseNetworkStatisticsBody,
  parseNetworkStatusBody,
} from './contract';

const statusBody = {
  api_version: BENCH_CORE_API_VERSION,
  measured_by: 'simulation',
  interfaces: [
    {
      id: 'sim-enp1s0',
      name: 'enp1s0',
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

const statisticsBody = {
  api_version: BENCH_CORE_API_VERSION,
  interface_id: 'sim-enp1s0',
  period: '15m',
  from: '2026-09-03T09:45:00Z',
  to: '2026-09-03T10:00:00Z',
  measured_by: 'simulation',
  stale: false,
  points: [
    {
      minute_start: '2026-09-03T09:45:00Z',
      rx_bytes: 4096,
      tx_bytes: 1024,
      rx_errors: 0,
      tx_errors: 0,
      rx_dropped: 0,
      tx_dropped: 0,
      samples: 11,
      stale: false,
    },
  ],
};

it('reads both gateway families and rejects malformed optional gateway data', () => {
  const body = {
    ...statusBody,
    interfaces: [
      { ...statusBody.interfaces[0], gateway4: '192.168.10.1', gateway6: 'fd00:10::1' },
    ],
  };
  expect(parseNetworkStatusBody(body)).toMatchObject({
    ok: true,
    value: {
      interfaces: [{ gateway4: '192.168.10.1', gateway6: 'fd00:10::1' }],
    },
  });
  expect(
    parseNetworkStatusBody({
      ...body,
      interfaces: [{ ...body.interfaces[0], gateway4: 7 }],
    }).ok,
  ).toBe(false);
});

function respondWithJson(status: number, body: unknown) {
  return {
    status,
    ok: status >= 200 && status < 300,
    text: () => Promise.resolve(JSON.stringify(body)),
  };
}

describe('network client methods', () => {
  it('reads network status over an origin-relative path with the bearer header', async () => {
    const transport = vi.fn<BenchCoreTransport>(() =>
      Promise.resolve(respondWithJson(200, statusBody)),
    );
    const client = createBenchCoreClient({
      baseUrl: 'http://127.0.0.1:8090',
      transport,
    });

    const status = await client.getNetworkStatus({
      kind: 'bearer',
      token: 'secret-token',
    });
    expect(status.interfaces).toHaveLength(2);

    expect(transport).toHaveBeenCalledTimes(1);
    const [url, request] = transport.mock.calls[0];
    // The configured client uses an empty same-origin base; this test supplies
    // one explicitly, so the assertion is on the path it appended.
    expect(url.endsWith(NETWORK_STATUS_PATH)).toBe(true);
    expect(request.headers.Authorization).toBe('Bearer secret-token');
    expect(request.credentials).toBe('same-origin');
    // A token in a URL ends up in logs and history; it is only ever a header.
    expect(url).not.toContain('secret-token');
  });

  it('asks for statistics by interface and period', async () => {
    const transport = vi.fn<BenchCoreTransport>(() =>
      Promise.resolve(respondWithJson(200, statisticsBody)),
    );
    const client = createBenchCoreClient({
      baseUrl: 'http://127.0.0.1:8090',
      transport,
    });

    const statistics = await client.getNetworkStatistics(
      { kind: 'bearer', token: 'secret-token' },
      'sim-enp1s0',
      '15m',
    );
    expect(statistics.points).toHaveLength(1);

    const [url] = transport.mock.calls[0];
    expect(url).toContain('/api/v1/network/statistics?');
    expect(url).toContain('interface=sim-enp1s0');
    expect(url).toContain('period=15m');
  });

  it('classifies a refused network read the same way every other resource is', async () => {
    const client = createBenchCoreClient({
      baseUrl: 'http://127.0.0.1:8090',
      transport: () => Promise.resolve(respondWithJson(403, { error: 'forbidden' })),
    });
    await expect(
      client.getNetworkStatus({ kind: 'bearer', token: 'narrowed' }),
    ).rejects.toMatchObject({ failure: 'forbidden' });
  });

  it('reports a body outside the contract as malformed rather than as data', async () => {
    const client = createBenchCoreClient({
      baseUrl: 'http://127.0.0.1:8090',
      transport: () =>
        Promise.resolve(
          respondWithJson(200, { ...statusBody, interfaces: 'not a list' }),
        ),
    });
    await expect(
      client.getNetworkStatus({ kind: 'bearer', token: 'token' }),
    ).rejects.toMatchObject({ failure: 'malformed' });
  });
});

/**
 * Sign in against the mock as the seeded administrator.
 *
 * A bearer token resolves to a Viewer here, exactly as a real development token
 * does, so the management routes need a password session — which is also how a
 * developer reaches the controls in mock mode.
 */
async function signedInAdministrator() {
  const client = createBenchCoreClient({
    baseUrl: '',
    transport: createMockTransport('healthy'),
  });
  const session = await client.signIn('admin', MOCK_DEVELOPMENT_PASSWORD);
  return { client, credential: sessionCredential(session.csrfToken ?? '') };
}

describe('the development mock', () => {
  it('answers the network routes with a body the production parser accepts', async () => {
    const client = createBenchCoreClient({
      baseUrl: '',
      transport: createMockTransport('healthy'),
    });
    const credential = { kind: 'bearer', token: 'mock-token' } as const;

    const status = await client.getNetworkStatus(credential);
    expect(status.measuredBy).toBe('simulation');
    expect(status.interfaces.map((entry) => entry.id)).toEqual([
      'sim-enp1s0',
      'sim-wlan0',
      'sim-lo',
    ]);

    const statistics = await client.getNetworkStatistics(
      credential,
      'sim-enp1s0',
      '15m',
    );
    expect(statistics.interfaceId).toBe('sim-enp1s0');
    expect(statistics.period).toBe('15m');
    // Every fourth minute is missing on purpose: an interface built against an
    // unbroken series would never have to face a gap.
    expect(statistics.points.length).toBeLessThan(15);
    expect(statistics.points.length).toBeGreaterThan(0);
  });

  it('serves role-aware management access through the development mock', async () => {
    const { client, credential } = await signedInAdministrator();

    const management = await client.getManagementAccess(credential);

    expect(management.configuredInterfaces).toEqual(['enp1s0']);
    expect(management.interfaces).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          name: 'enp1s0',
          adapterId: '02:00:00:00:00:01',
          role: 'management',
          roleAvailable: true,
        }),
      ]),
    );
  });

  // The half-built mock let the Disable button appear and silently do nothing,
  // because the preview route was never mocked. This drives the whole journey
  // through the mock so that gap cannot reopen unnoticed.
  it('carries a change through preview, acknowledgement and confirmation', async () => {
    const { client, credential } = await signedInAdministrator();

    const preview = await client.previewInterfaceChange(
      credential,
      'sim-enp1s0',
      'disable',
    );
    expect(preview.state).toBe('previewed');
    expect(preview.action).toBe('interface.disable');
    expect(preview.target).toBe('sim-enp1s0');
    expect(preview.revision).not.toBe('');

    const running = await client.acknowledgeNetworkOperation(
      credential,
      preview.id,
      preview.revision,
    );
    expect(running.state).toBe('awaiting_confirmation');
    expect(running.confirmDeadline).toBeInstanceOf(Date);

    const confirmed = await client.confirmNetworkOperation(credential, preview.id);
    expect(confirmed.state).toBe('confirmed');
  });

  it('refuses an acknowledgement that does not carry the previewed revision', async () => {
    const { client, credential } = await signedInAdministrator();

    const preview = await client.previewInterfaceChange(
      credential,
      'sim-enp1s0',
      'disable',
    );
    await expect(
      client.acknowledgeNetworkOperation(credential, preview.id, 'not-the-revision'),
    ).rejects.toMatchObject({ code: 'network_preview_stale' });
  });

  it('restores rather than keeps when the administrator asks it to', async () => {
    const { client, credential } = await signedInAdministrator();

    const preview = await client.previewInterfaceChange(
      credential,
      'sim-enp1s0',
      'disable',
    );
    await client.acknowledgeNetworkOperation(credential, preview.id, preview.revision);
    const restored = await client.rollbackNetworkOperation(credential, preview.id);
    expect(restored.state).toBe('rolled_back');
  });

  it('scales the returned window to the requested period', async () => {
    const client = createBenchCoreClient({
      baseUrl: '',
      transport: createMockTransport('healthy'),
    });
    const credential = { kind: 'bearer', token: 'mock-token' } as const;

    const hour = await client.getNetworkStatistics(credential, 'sim-enp1s0', '1h');
    const quarter = await client.getNetworkStatistics(credential, 'sim-enp1s0', '15m');
    expect(hour.points.length).toBeGreaterThan(quarter.points.length);
    expect(hour.to.getTime() - hour.from.getTime()).toBe(60 * 60_000);
  });
});

describe('network permissions', () => {
  it('declares the four fixed network permissions the backend enforces', () => {
    for (const permission of [
      PERMISSION_NETWORK_READ,
      PERMISSION_NETWORK_MANAGE,
      PERMISSION_VPN_MANAGE,
      PERMISSION_VPN_INSTALL,
    ]) {
      expect(BENCH_CORE_PERMISSIONS).toContain(permission);
    }
    expect(PERMISSION_NETWORK_READ).toBe('network.read');
    expect(PERMISSION_NETWORK_MANAGE).toBe('network.manage');
    expect(PERMISSION_VPN_MANAGE).toBe('vpn.manage');
    expect(PERMISSION_VPN_INSTALL).toBe('vpn.install');
  });
});

describe('network paths', () => {
  it('addresses the backend through origin-relative paths only', () => {
    expect(NETWORK_STATUS_PATH).toBe('/api/v1/network/status');
    expect(NETWORK_STATISTICS_PATH).toBe('/api/v1/network/statistics');
    for (const path of [NETWORK_STATUS_PATH, NETWORK_STATISTICS_PATH]) {
      expect(path.startsWith('/')).toBe(true);
      expect(path).not.toContain('://');
    }
  });

  it('encodes the interface and period into the statistics query', () => {
    const path = networkStatisticsPath('sim-enp1s0', '24h');
    expect(path).toContain('interface=sim-enp1s0');
    expect(path).toContain('period=24h');
    expect(networkStatisticsPath('a b/c', '1h')).toContain('interface=a%20b%2Fc');
  });
});

describe('parseNetworkStatusBody', () => {
  it('returns every reported field of every interface', () => {
    const parsed = parseNetworkStatusBody(statusBody);
    expect(parsed.ok).toBe(true);
    if (!parsed.ok) return;

    expect(parsed.value.measuredBy).toBe('simulation');
    expect(parsed.value.interfaces).toHaveLength(2);

    const [wired, loopback] = parsed.value.interfaces;
    expect(wired).toMatchObject({
      id: 'sim-enp1s0',
      name: 'enp1s0',
      kind: 'ethernet',
      state: 'connected',
      linkCapacityMbps: 1000,
      rxBytesPerSecond: 12480,
      txBytesPerSecond: 3120,
      controllable: true,
    });
    expect(wired.ipv4).toEqual(['192.168.10.24/24']);
    expect(wired.ipv6).toEqual(['fd00:10::24/64']);

    // The reason travels with the refusal, so the page can say why a control
    // is absent rather than silently omitting it.
    expect(loopback.controllable).toBe(false);
    expect(loopback.notControllableReason).not.toBe('');
  });

  it('refuses a response that is not the contract this interface understands', () => {
    for (const body of [
      null,
      [],
      'not an object',
      { ...statusBody, api_version: 'nilo.http/99' },
      { ...statusBody, measured_by: undefined },
      { ...statusBody, interfaces: 'not a list' },
      { ...statusBody, interfaces: [{ ...statusBody.interfaces[0], id: 42 }] },
      {
        ...statusBody,
        interfaces: [{ ...statusBody.interfaces[0], rx_bytes_per_second: 'lots' }],
      },
      {
        ...statusBody,
        interfaces: [{ ...statusBody.interfaces[0], controllable: 'yes' }],
      },
    ]) {
      const parsed = parseNetworkStatusBody(body);
      expect(parsed.ok, `accepted ${JSON.stringify(body)}`).toBe(false);
    }
  });

  it('accepts an appliance that reports no interfaces at all', () => {
    const parsed = parseNetworkStatusBody({ ...statusBody, interfaces: [] });
    expect(parsed.ok).toBe(true);
    if (!parsed.ok) return;
    expect(parsed.value.interfaces).toEqual([]);
  });
});

describe('parseNetworkStatisticsBody', () => {
  it('returns the bounded series with its freshness', () => {
    const parsed = parseNetworkStatisticsBody(statisticsBody);
    expect(parsed.ok).toBe(true);
    if (!parsed.ok) return;

    expect(parsed.value).toMatchObject({
      interfaceId: 'sim-enp1s0',
      period: '15m',
      measuredBy: 'simulation',
      stale: false,
    });
    expect(parsed.value.points).toHaveLength(1);
    expect(parsed.value.points[0]).toMatchObject({
      rxBytes: 4096,
      txBytes: 1024,
      samples: 11,
      stale: false,
    });
    expect(parsed.value.points[0].minuteStart).toBeInstanceOf(Date);
  });

  it('carries a stale series through as stale rather than dropping it', () => {
    const parsed = parseNetworkStatisticsBody({
      ...statisticsBody,
      stale: true,
      points: [{ ...statisticsBody.points[0], stale: true }],
    });
    expect(parsed.ok).toBe(true);
    if (!parsed.ok) return;
    expect(parsed.value.stale).toBe(true);
    expect(parsed.value.points[0].stale).toBe(true);
  });

  it('accepts an empty series, which is what a gap in measurement looks like', () => {
    const parsed = parseNetworkStatisticsBody({
      ...statisticsBody,
      points: [],
      stale: true,
    });
    expect(parsed.ok).toBe(true);
    if (!parsed.ok) return;
    expect(parsed.value.points).toEqual([]);
  });

  it('refuses a period it cannot draw and a point it cannot place', () => {
    for (const body of [
      { ...statisticsBody, period: '7d' },
      { ...statisticsBody, period: '' },
      { ...statisticsBody, stale: 'no' },
      { ...statisticsBody, from: 'not a time' },
      {
        ...statisticsBody,
        points: [{ ...statisticsBody.points[0], minute_start: 'never' }],
      },
      { ...statisticsBody, points: [{ ...statisticsBody.points[0], rx_bytes: null }] },
      { ...statisticsBody, points: 'not a list' },
    ]) {
      const parsed = parseNetworkStatisticsBody(body);
      expect(parsed.ok, `accepted ${JSON.stringify(body)}`).toBe(false);
    }
  });
});
