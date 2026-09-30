import { fireEvent, screen, waitFor, within } from '@testing-library/react';
import { afterEach, expect, it, vi } from 'vitest';
import {
  BenchCoreError,
  createBenchCoreClient,
  sessionCredential,
} from '../../api/client';
import type { InstrumentPlugin } from '../../api/bench';
import type { BenchCoreSession } from '../../api/contract';
import type { ModbusEndpoint, ModbusProfile } from '../../api/modbus';
import {
  renderInShell,
  installColorScheme,
  restoreMatchMedia,
} from '../../test/render';
import { ModbusPluginPage } from './ModbusPluginPage';

const admin: BenchCoreSession = {
  apiVersion: 'nilo.http/1',
  username: 'admin',
  role: 'administrator',
  permissions: ['bench.read', 'bench.execute', 'plugins.manage'],
  credential: 'session',
  csrfToken: 'csrf',
};
const plugin: InstrumentPlugin = {
  id: 'modbus',
  display_name: 'Modbus',
  version: '0.12.0',
  installed: true,
  enabled: true,
  mode: 'ask',
  status: 'ready',
  simulation: true,
  operations: ['connect'],
  devices: [],
  error: '',
  digest: 'abc',
};
const endpoint: ModbusEndpoint = {
  schema_version: 1,
  id: 'boiler',
  revision: 2,
  name: 'Boiler',
  transport: 'tcp',
  resource_id: 'boiler-bus',
  enabled: true,
  presence: 'configured',
  readiness: 'offline',
  network: { host: '192.0.2.9', port: 502, unit_min: 1, unit_max: 4 },
};
const profile: ModbusProfile = {
  schema_version: 1,
  id: 'standard',
  revision: 3,
  owner_id: 'admin',
  name: 'Standard',
  endpoint_id: 'boiler',
  settings: {
    response_timeout_ms: 1000,
    connect_timeout_ms: 1000,
    inter_request_ms: 0,
    read_retries: 0,
  },
};
const settings = {
  revision: 4,
  settings: {
    max_sessions: 4,
    session_lease_seconds: 3600,
    idle_seconds: 300,
    retention_count: 100,
    retention_bytes: 2147483648,
    part_bytes: 104857600,
  },
};

afterEach(() => {
  restoreMatchMedia();
  vi.restoreAllMocks();
});

it('edits a saved offline TCP endpoint with its current revision and no implicit connection', async () => {
  installColorScheme('light');
  const save = vi.fn(async (_credential, _revision, changed: ModbusEndpoint) => ({
    ...changed,
    revision: 3,
  }));
  const test = vi.fn();
  const client = {
    ...createBenchCoreClient({ baseUrl: '' }),
    listPlugins: vi.fn(async () => [plugin]),
    listModbusEndpoints: vi.fn(async () => [endpoint]),
    listModbusProfiles: vi.fn(async () => []),
    getModbusSettings: vi.fn(async () => settings),
    saveModbusEndpoint: save,
    testModbusConnection: test,
  };
  renderInShell(
    <ModbusPluginPage
      client={client}
      credential={sessionCredential('csrf')}
      session={admin}
      onSessionEnded={vi.fn()}
    />,
  );
  const edit = await screen.findByRole('button', { name: 'Edit Boiler' });
  expect(test).not.toHaveBeenCalled();
  fireEvent.click(edit);
  fireEvent.change(screen.getByLabelText('Host'), { target: { value: '192.0.2.10' } });
  fireEvent.click(screen.getAllByRole('button', { name: 'Save' })[0]);
  await waitFor(() =>
    expect(save).toHaveBeenCalledWith(
      expect.anything(),
      2,
      expect.objectContaining({
        network: expect.objectContaining({ host: '192.0.2.10' }),
      }),
    ),
  );
  expect(test).not.toHaveBeenCalled();
});

it('submits a bounded one-use connection test only after explicit selection', async () => {
  installColorScheme('light');
  const test = vi.fn(async () => ({
    transaction_id: 'job-1',
    session_id: '',
    request_id: 'request-1',
    plugin_id: 'modbus',
    owner_id: 'admin',
    state: 'completed',
    submission: {} as never,
    created_at: '',
    deadline: '',
    result: {},
    replies: [],
    error: '',
    artifact_id: '',
  }));
  const client = {
    ...createBenchCoreClient({ baseUrl: '' }),
    listPlugins: vi.fn(async () => [plugin]),
    listModbusEndpoints: vi.fn(async () => [endpoint]),
    listModbusProfiles: vi.fn(async () => [profile]),
    getModbusSettings: vi.fn(async () => settings),
    testModbusConnection: test,
  };
  renderInShell(
    <ModbusPluginPage
      client={client}
      credential={sessionCredential('csrf')}
      session={admin}
      onSessionEnded={vi.fn()}
    />,
  );
  const testSection = await screen.findByRole('region', { name: 'Test connection' });
  expect(test).not.toHaveBeenCalled();
  const testButton = within(testSection).getByRole('button', {
    name: 'Test connection',
  });
  expect(testButton).toBeDisabled();
  await within(testSection).findByRole('option', { name: 'Boiler' });
  fireEvent.change(within(testSection).getByLabelText('Endpoint'), {
    target: { value: 'boiler' },
  });
  await within(testSection).findByRole('option', { name: 'Standard' });
  fireEvent.change(within(testSection).getByLabelText('Profile'), {
    target: { value: 'standard' },
  });
  fireEvent.click(testButton);
  await waitFor(() =>
    expect(test).toHaveBeenCalledWith(
      expect.anything(),
      'boiler',
      2,
      'standard',
      3,
      expect.any(String),
    ),
  );
  expect(await within(testSection).findByRole('status')).toHaveTextContent('completed');
});

it('saves settings against the fetched revision and reports a stale write', async () => {
  installColorScheme('light');
  const save = vi.fn(async () => {
    throw new BenchCoreError('rejected', 'Stale revision', 'stale_revision');
  });
  const client = {
    ...createBenchCoreClient({ baseUrl: '' }),
    listPlugins: vi.fn(async () => [plugin]),
    listModbusEndpoints: vi.fn(async () => []),
    listModbusProfiles: vi.fn(async () => []),
    getModbusSettings: vi.fn(async () => settings),
    saveModbusSettings: save,
  };
  renderInShell(
    <ModbusPluginPage
      client={client}
      credential={sessionCredential('csrf')}
      session={admin}
      onSessionEnded={vi.fn()}
    />,
  );
  const settingsSection = await screen.findByRole('region', {
    name: 'Operating settings',
  });
  fireEvent.change(within(settingsSection).getByLabelText('Maximum sessions'), {
    target: { value: '6' },
  });
  fireEvent.click(within(settingsSection).getByRole('button', { name: 'Save' }));
  await waitFor(() =>
    expect(save).toHaveBeenCalledWith(
      expect.anything(),
      4,
      expect.objectContaining({ max_sessions: 6 }),
    ),
  );
  expect(await screen.findByText('stale_revision')).toBeVisible();
  expect(within(settingsSection).getByLabelText('Maximum sessions')).toHaveValue(6);
});

it('submits a profile edit with its saved revision and shows a busy policy rejection', async () => {
  installColorScheme('light');
  const save = vi.fn(async (_credential, _revision, changed: ModbusProfile) => ({
    ...changed,
    revision: 4,
  }));
  const lifecycle = vi.fn(async () => {
    throw new BenchCoreError('rejected', 'Plugin is busy', 'plugin_busy');
  });
  const client = {
    ...createBenchCoreClient({ baseUrl: '' }),
    listPlugins: vi.fn(async () => [plugin]),
    listModbusEndpoints: vi.fn(async () => [endpoint]),
    listModbusProfiles: vi.fn(async () => [profile]),
    getModbusSettings: vi.fn(async () => settings),
    saveModbusProfile: save,
    pluginAction: lifecycle,
  };
  renderInShell(
    <ModbusPluginPage
      client={client}
      credential={sessionCredential('csrf')}
      session={admin}
      onSessionEnded={vi.fn()}
    />,
  );
  expect(await screen.findByRole('button', { name: 'Edit Standard' })).toBeVisible();
  fireEvent.click(screen.getByRole('button', { name: 'Edit Standard' }));
  const profileSection = screen.getByRole('region', { name: 'Connection profiles' });
  fireEvent.change(within(profileSection).getByLabelText('Name'), {
    target: { value: 'Careful' },
  });
  fireEvent.click(within(profileSection).getByRole('button', { name: 'Save' }));
  await waitFor(() =>
    expect(save).toHaveBeenCalledWith(
      expect.anything(),
      3,
      expect.objectContaining({ name: 'Careful', endpoint_id: 'boiler' }),
    ),
  );
  fireEvent.change(screen.getByLabelText('Policy'), { target: { value: 'auto' } });
  await waitFor(() =>
    expect(lifecycle).toHaveBeenCalledWith(
      expect.anything(),
      'modbus',
      'enable',
      'auto',
    ),
  );
  expect(await screen.findByText('plugin_busy')).toBeVisible();
});

it('shows an existing RTU record without offering a serial editor', async () => {
  installColorScheme('light');
  const client = {
    ...createBenchCoreClient({ baseUrl: '' }),
    listPlugins: vi.fn(async () => [plugin]),
    listModbusEndpoints: vi.fn(async () => [
      { ...endpoint, id: 'legacy', name: 'Legacy RTU', transport: 'rtu' as const },
    ]),
    listModbusProfiles: vi.fn(async () => []),
    getModbusSettings: vi.fn(async () => settings),
  };
  renderInShell(
    <ModbusPluginPage
      client={client}
      credential={sessionCredential('csrf')}
      session={admin}
      onSessionEnded={vi.fn()}
    />,
  );
  expect(await screen.findByText('Legacy RTU')).toBeVisible();
  expect(screen.queryByRole('button', { name: 'Edit Legacy RTU' })).toBeNull();
});

it('keeps administration controls hidden from a reader', async () => {
  installColorScheme('light');
  const client = { ...createBenchCoreClient({ baseUrl: '' }), listPlugins: vi.fn() };
  renderInShell(
    <ModbusPluginPage
      client={client}
      credential={sessionCredential('csrf')}
      session={{ ...admin, role: 'viewer', permissions: ['bench.read'] }}
      onSessionEnded={vi.fn()}
    />,
  );
  expect(
    screen.getByText('Plugin administration requires an administrator.'),
  ).toBeVisible();
  expect(client.listPlugins).not.toHaveBeenCalled();
});
