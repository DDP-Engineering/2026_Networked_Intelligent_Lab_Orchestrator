import { expect, it, vi } from 'vitest';
import { createBenchCoreClient, sessionCredential } from './client';
import { parseInstrumentJob, parsePlugins } from './bench';

it('uses authenticated CSRF-protected lifecycle requests', async () => {
  const body = {
    id: 'sigrok',
    display_name: 'Sigrok',
    version: '0.1.0',
    installed: true,
    enabled: true,
    mode: 'ask',
    status: 'ready',
    simulation: true,
    operations: [],
    devices: [],
    error: '',
    digest: 'digest',
  };
  const transport = vi.fn(async () => ({
    ok: true,
    status: 200,
    text: async () => JSON.stringify(body),
  }));
  const client = createBenchCoreClient({ baseUrl: '', transport });
  await client.pluginAction(sessionCredential('csrf-value'), 'sigrok', 'enable', 'ask');
  expect(transport).toHaveBeenCalledWith(
    '/api/v1/plugins/sigrok/enable',
    expect.objectContaining({
      method: 'POST',
      credentials: 'same-origin',
      headers: expect.objectContaining({ 'X-NILO-CSRF': 'csrf-value' }),
      body: '{"mode":"ask"}',
    }),
  );
});

it('rejects malformed replies and exposed PWM inventory', () => {
  expect(
    parseInstrumentJob({
      transaction_id: 'one',
      replies: [{ transaction_id: 'another' }],
    }).ok,
  ).toBe(false);
  expect(parsePlugins({ plugins: [{ devices: [{ channels: ['PWM0'] }] }] }).ok).toBe(
    false,
  );
});

it('accepts a managed physical camera without analyzer channels', () => {
  const parsed = parsePlugins({
    plugins: [
      {
        id: 'camera',
        display_name: 'Camera',
        version: '0.1.0',
        installed: true,
        enabled: true,
        mode: 'auto',
        status: 'ready',
        simulation: false,
        operations: ['discover_devices', 'snapshot'],
        error: '',
        digest: 'digest',
        devices: [
          {
            id: 'cam-1',
            display_name: 'Bench Camera',
            kind: 'camera',
            presence: 'present',
            readiness: 'ready',
            configured: true,
            enabled: true,
            revision: 2,
            capabilities: { formats: ['MJPG'] },
          },
        ],
      },
    ],
  });
  expect(parsed.ok).toBe(true);
});
