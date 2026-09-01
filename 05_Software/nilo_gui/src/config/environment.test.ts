import { describe, expect, it, vi } from 'vitest';

import type { BenchCoreTransport, TransportResponse } from '../api/client';
import { MOCK_HEALTHY_STATUS_BODY } from '../api/mock';
import {
  DEFAULT_API_BASE_URL,
  GuiConfigurationError,
  SAME_ORIGIN_BASE_URL,
  createConfiguredClient,
  readGuiEnvironment,
} from './environment';

function recordingTransport() {
  const response: TransportResponse = {
    status: 200,
    ok: true,
    text: () => Promise.resolve(JSON.stringify(MOCK_HEALTHY_STATUS_BODY)),
  };
  return vi.fn<BenchCoreTransport>(() => Promise.resolve(response));
}

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

  it('requests an origin-relative path so the browser never preflights the backend', async () => {
    const transport = recordingTransport();
    const client = createConfiguredClient(
      { VITE_NILO_API_MODE: 'live', VITE_NILO_API_BASE_URL: 'http://127.0.0.1:8090' },
      transport,
    );

    await client.getStatus('development-token');

    const [url] = transport.mock.calls[0];
    expect(url).toBe('/api/v1/status');
    expect(url).not.toMatch(/^https?:/);
    expect(client.baseUrl).toBe('');
  });

  it('keeps the configured endpoint as the label the interface shows', () => {
    expect(
      readGuiEnvironment({ VITE_NILO_API_BASE_URL: 'http://127.0.0.1:9100' })
        .apiBaseUrl,
    ).toBe('http://127.0.0.1:9100');
  });

  it('builds a mock-backed client that needs no backend process', async () => {
    const client = createConfiguredClient({ VITE_NILO_API_MODE: 'mock' });

    const status = await client.getStatus('any-development-token');

    expect(client.baseUrl).toBe(SAME_ORIGIN_BASE_URL);
    expect(status.profile).toBe('simulation');
    expect(status.simulation).toBe(true);
  });
});
