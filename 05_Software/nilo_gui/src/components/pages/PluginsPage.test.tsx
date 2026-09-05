import { fireEvent, screen, waitFor } from '@testing-library/react';
import { afterEach, expect, it, vi } from 'vitest';
import { createBenchCoreClient, sessionCredential } from '../../api/client';
import type { InstrumentJob, InstrumentPlugin } from '../../api/bench';
import type { BenchCoreSession } from '../../api/contract';
import {
  renderInShell,
  installColorScheme,
  restoreMatchMedia,
} from '../../test/render';
import { PluginsPage } from './PluginsPage';

const plugin: InstrumentPlugin = {
  id: 'sigrok',
  display_name: 'Sigrok Instruments',
  version: '0.1.0',
  installed: true,
  enabled: true,
  mode: 'ask',
  status: 'ready',
  simulation: true,
  error: '',
  digest: 'abc',
  operations: ['capture'],
  devices: [
    {
      id: 'atk-dl16-1.3',
      connection: '1.3',
      display_name: 'ATK DL16',
      driver: 'atk-dl16',
      channels: Array.from({ length: 16 }, (_, i) => `D${i}`),
      samplerates: [1000000],
      maximum_samples: 1000000,
    },
  ],
};
const session: BenchCoreSession = {
  apiVersion: 'nilo.http/1',
  username: 'admin',
  role: 'administrator',
  permissions: ['bench.read', 'bench.execute', 'bench.approve', 'plugins.manage'],
  credential: 'session',
  csrfToken: 'csrf',
};
const job: InstrumentJob = {
  transaction_id: 'transaction-one',
  session_id: 'session-one',
  request_id: 'request-one',
  plugin_id: 'sigrok',
  owner_id: 'admin',
  state: 'pending_approval',
  submission: {
    plugin_id: 'sigrok',
    operation: 'capture',
    device_id: 'atk-dl16-1.3',
    parameters: { channels: ['D0'], samplerate: 1000000, samples: 1024 },
    idempotency_key: 'retry-command',
    timeout_seconds: 60,
  },
  created_at: '2026-09-05T00:00:00Z',
  deadline: '2026-09-05T00:01:00Z',
  result: {},
  replies: [],
  error: '',
  artifact_id: '',
};
afterEach(() => {
  restoreMatchMedia();
  vi.restoreAllMocks();
});

it('submits only digital channels and retains the key across an uncertain retry', async () => {
  installColorScheme('light');
  const submit = vi
    .fn()
    .mockRejectedValueOnce(new Error('connection lost'))
    .mockResolvedValue(job);
  const client = {
    ...createBenchCoreClient({ baseUrl: '' }),
    listPlugins: async () => [plugin],
    listInstrumentJobs: async () => [],
    getInstrumentJob: async () => job,
    submitInstrumentJob: submit,
  };
  renderInShell(
    <PluginsPage
      client={client}
      credential={sessionCredential('csrf')}
      session={session}
      onSessionEnded={() => {}}
    />,
  );
  const capture = await screen.findByRole('button', { name: 'Queue capture' });
  expect(screen.queryByText('PWM0')).toBeNull();
  fireEvent.click(capture);
  await screen.findByRole('alert');
  await waitFor(() => expect(capture).not.toBeDisabled());
  fireEvent.click(capture);
  await waitFor(() => expect(submit).toHaveBeenCalledTimes(2));
  expect(submit.mock.calls[0][1].idempotency_key).toBe(
    submit.mock.calls[1][1].idempotency_key,
  );
  expect(submit.mock.calls[1][1].parameters).toEqual({
    channels: ['D0'],
    samplerate: 1000000,
    samples: 1024,
  });
  expect(await screen.findByRole('button', { name: 'Approve' })).toBeVisible();
});

it('reattaches to a pending transaction and hides mutation controls from viewers', async () => {
  installColorScheme('light');
  const client = {
    ...createBenchCoreClient({ baseUrl: '' }),
    listPlugins: async () => [plugin],
    listInstrumentJobs: async () => [job],
    getInstrumentJob: async () => job,
  };
  renderInShell(
    <PluginsPage
      client={client}
      credential={sessionCredential('csrf')}
      session={{ ...session, role: 'viewer', permissions: ['bench.read'] }}
      onSessionEnded={() => {}}
    />,
  );
  fireEvent.click(await screen.findByRole('button', { name: 'transaction-one' }));
  expect(await screen.findByText('Result and replies')).toBeVisible();
  expect(screen.queryByRole('button', { name: 'Approve' })).toBeNull();
  expect(screen.queryByRole('button', { name: 'Uninstall' })).toBeNull();
  expect(screen.getByRole('button', { name: 'Queue capture' })).toBeDisabled();
});
