import { describe, expect, it, vi } from 'vitest';

import { bearerCredential } from '../api/client';
import type { BenchCoreClient, BenchCoreTransport } from '../api/client';
import { createMockTransport, MOCK_SCENARIOS } from '../api/mock';
import {
  DEFAULT_API_BASE_URL,
  GuiConfigurationError,
  SAME_ORIGIN_BASE_URL,
  createConfiguredClient,
  readGuiEnvironment,
} from './environment';

/**
 * Answers each resource from the contract mock while recording the URL the
 * client asked for, so one transport serves both the configuration assertions
 * and the guard against an absolute browser URL.
 */
function recordingTransport() {
  const answer = createMockTransport('healthy');
  return vi.fn<BenchCoreTransport>((url, request) => answer(url, request));
}

const RESOURCE_CALLS: readonly {
  name: string;
  call: (client: BenchCoreClient, token: string) => Promise<unknown>;
}[] = [
  {
    name: 'status',
    call: (client, token) => client.getStatus(bearerCredential(token)),
  },
  {
    name: 'integrations',
    call: (client, token) => client.getIntegrations(bearerCredential(token)),
  },
  {
    name: 'devices',
    call: (client, token) => client.getDevices(bearerCredential(token)),
  },
];

describe('GUI environment configuration', () => {
  it('defaults to the loopback development backend when nothing is declared', () => {
    expect(readGuiEnvironment({})).toEqual({
      apiMode: 'live',
      apiBaseUrl: DEFAULT_API_BASE_URL,
      mockScenario: 'healthy',
    });
    expect(DEFAULT_API_BASE_URL).toBe('http://127.0.0.1:8090');
  });

  it('takes an explicit live base URL and drops its trailing separator', () => {
    expect(
      readGuiEnvironment({
        VITE_NILO_API_MODE: 'live',
        VITE_NILO_API_BASE_URL: 'http://127.0.0.1:9100/',
      }),
    ).toEqual({
      apiMode: 'live',
      apiBaseUrl: 'http://127.0.0.1:9100',
      mockScenario: 'healthy',
    });
  });

  it('takes mock mode together with a named scenario', () => {
    expect(
      readGuiEnvironment({
        VITE_NILO_API_MODE: 'mock',
        VITE_NILO_MOCK_SCENARIO: 'unauthorized',
      }),
    ).toEqual({
      apiMode: 'mock',
      apiBaseUrl: DEFAULT_API_BASE_URL,
      mockScenario: 'unauthorized',
    });
  });

  it('rejects an unrecognised mode by naming the variable', () => {
    expect(() => readGuiEnvironment({ VITE_NILO_API_MODE: 'sideways' })).toThrowError(
      GuiConfigurationError,
    );
    expect(() => readGuiEnvironment({ VITE_NILO_API_MODE: 'sideways' })).toThrowError(
      /VITE_NILO_API_MODE/,
    );
  });

  it('rejects a base URL that is not an absolute HTTP address', () => {
    expect(() =>
      readGuiEnvironment({ VITE_NILO_API_BASE_URL: '127.0.0.1:8090' }),
    ).toThrowError(/VITE_NILO_API_BASE_URL/);
  });

  it('rejects an unrecognised mock scenario by naming the variable', () => {
    expect(() =>
      readGuiEnvironment({
        VITE_NILO_API_MODE: 'mock',
        VITE_NILO_MOCK_SCENARIO: 'sideways',
      }),
    ).toThrowError(/VITE_NILO_MOCK_SCENARIO/);
  });

  it.each(RESOURCE_CALLS)(
    'requests the $name resource origin-relative so the browser never preflights the backend',
    async ({ call }) => {
      const transport = recordingTransport();
      const client = createConfiguredClient(
        { VITE_NILO_API_MODE: 'live', VITE_NILO_API_BASE_URL: 'http://127.0.0.1:8090' },
        transport,
      );

      await call(client, 'development-token');

      const [url] = transport.mock.calls[0];
      expect(url).toMatch(/^\/api\/v1\//);
      expect(url).not.toMatch(/^[a-z][a-z0-9+.-]*:/i);
      expect(url.startsWith('//')).toBe(false);
      expect(url).not.toContain('127.0.0.1');
      expect(client.baseUrl).toBe('');
    },
  );

  it('never puts the token in a requested URL, whichever resource is asked for', async () => {
    const transport = recordingTransport();
    const client = createConfiguredClient({ VITE_NILO_API_MODE: 'live' }, transport);

    for (const { call } of RESOURCE_CALLS) {
      await call(client, 'issued-development-token');
    }

    for (const [url] of transport.mock.calls) {
      expect(url).not.toContain('issued-development-token');
    }
    expect(transport.mock.calls).toHaveLength(RESOURCE_CALLS.length);
  });

  it.each(MOCK_SCENARIOS)('accepts %s as a declared mock scenario', (scenario) => {
    expect(
      readGuiEnvironment({
        VITE_NILO_API_MODE: 'mock',
        VITE_NILO_MOCK_SCENARIO: scenario,
      }).mockScenario,
    ).toBe(scenario);
  });

  it('keeps the configured endpoint as the label the interface shows', () => {
    expect(
      readGuiEnvironment({ VITE_NILO_API_BASE_URL: 'http://127.0.0.1:9100' })
        .apiBaseUrl,
    ).toBe('http://127.0.0.1:9100');
  });

  it('builds a mock-backed client that needs no backend process', async () => {
    const client = createConfiguredClient({ VITE_NILO_API_MODE: 'mock' });

    const status = await client.getStatus(bearerCredential('any-development-token'));

    expect(client.baseUrl).toBe(SAME_ORIGIN_BASE_URL);
    expect(status.profile).toBe('simulation');
    expect(status.simulation).toBe(true);
  });

  it('serves the whole inventory from the mock without a backend process', async () => {
    const client = createConfiguredClient({
      VITE_NILO_API_MODE: 'mock',
      VITE_NILO_MOCK_SCENARIO: 'healthy',
    });

    const integrations = await client.getIntegrations(
      bearerCredential('any-development-token'),
    );
    const devices = await client.getDevices(bearerCredential('any-development-token'));

    expect(integrations.items[0].simulation).toBe(true);
    expect(devices.items[0].integrationId).toBe(integrations.items[0].id);
  });
});
