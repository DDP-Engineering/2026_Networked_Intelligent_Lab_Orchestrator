import { fireEvent, screen, waitFor, within } from '@testing-library/react';
import { afterEach, expect, it, vi } from 'vitest';
import { createBenchCoreClient, sessionCredential } from '../../api/client';
import type { InstrumentPlugin } from '../../api/bench';
import type { BenchCoreSession } from '../../api/contract';
import {
  renderInShell,
  installColorScheme,
  restoreMatchMedia,
} from '../../test/render';
import { CameraPluginPage } from './CameraPluginPage';

const session: BenchCoreSession = {
  apiVersion: 'nilo.http/1',
  username: 'admin',
  role: 'administrator',
  permissions: ['bench.read', 'bench.execute', 'plugins.manage'],
  credential: 'session',
  csrfToken: 'csrf',
};
const camera: InstrumentPlugin = {
  id: 'camera',
  display_name: 'Camera',
  version: '0.1.0',
  installed: true,
  enabled: false,
  mode: 'ask',
  status: 'disabled',
  simulation: false,
  operations: [],
  devices: [],
  error: '',
  digest: 'abc',
};
afterEach(() => {
  restoreMatchMedia();
  vi.restoreAllMocks();
});

function setup(record = camera, actor = session) {
  installColorScheme('light');
  let current = record;
  const client = {
    ...createBenchCoreClient({ baseUrl: '' }),
    listPlugins: vi.fn(async () => [current]),
    pluginAction: vi.fn(async (_credential, _id, action, mode) => {
      current = {
        ...current,
        enabled: action === 'enable',
        mode: mode ?? current.mode,
      };
      return current;
    }),
    discoverInstruments: vi.fn(async () => []),
    configureInstrumentDevice: vi.fn(
      async (_credential, _plugin, id, revision, name, purpose, settings) => {
        const device = {
          ...current.devices.find((item) => item.id === id)!,
          display_name: name,
          purpose,
          settings,
          revision: revision + 1,
          configured: true,
        };
        current = { ...current, devices: [device] };
        return device;
      },
    ),
    instrumentDeviceAction: vi.fn(
      async (_credential, _plugin, id, action, revision) => {
        const device = {
          ...current.devices.find((item) => item.id === id)!,
          enabled: action === 'enable',
          revision: revision + 1,
        };
        current = { ...current, devices: [device] };
        return device;
      },
    ),
  };
  const activity = vi.fn();
  renderInShell(
    <CameraPluginPage
      client={client}
      credential={sessionCredential('csrf')}
      session={actor}
      onSessionEnded={vi.fn()}
      onOpenActivity={activity}
    />,
  );
  return { client, activity };
}

it('enables with the selected approval policy and updates control states', async () => {
  const { client, activity } = setup();
  expect(await screen.findByText('Current version: 0.1.0')).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Update plugin' })).toBeDisabled();
  expect(screen.getByRole('button', { name: 'Disable' })).toBeDisabled();
  fireEvent.change(screen.getByLabelText('Approval policy'), {
    target: { value: 'auto' },
  });
  fireEvent.click(screen.getByRole('button', { name: 'Enable' }));
  await waitFor(() =>
    expect(client.pluginAction).toHaveBeenCalledWith(
      sessionCredential('csrf'),
      'camera',
      'enable',
      'auto',
    ),
  );
  await waitFor(() =>
    expect(screen.getByRole('button', { name: 'Enable' })).toBeDisabled(),
  );
  expect(screen.getByRole('button', { name: 'Disable' })).toBeEnabled();
  expect(screen.getByRole('status')).toHaveTextContent('Info:');
  fireEvent.click(screen.getByRole('button', { name: 'View activity' }));
  expect(activity).toHaveBeenCalled();
});

it('allows update only when Core reports a verified newer bundle', async () => {
  const { client } = setup({
    ...camera,
    available_version: '0.2.0',
    update_available: true,
  });
  await waitFor(() =>
    expect(screen.getByRole('button', { name: 'Update plugin' })).toBeEnabled(),
  );
  fireEvent.click(screen.getByRole('button', { name: 'Update plugin' }));
  await waitFor(() =>
    expect(client.pluginAction).toHaveBeenCalledWith(
      sessionCredential('csrf'),
      'camera',
      'upgrade',
    ),
  );
});

it('refuses operators before reading or changing plugin configuration', () => {
  const { client } = setup(camera, { ...session, role: 'operator' });
  expect(screen.getByRole('status')).toHaveTextContent('Warning: Only administrators');
  expect(client.listPlugins).not.toHaveBeenCalled();
  expect(screen.queryByRole('button', { name: 'Enable' })).not.toBeInTheDocument();
});

it('renames a camera without losing its settings and uses the returned revision to enable it', async () => {
  const device = {
    id: 'cam-1',
    display_name: 'USB camera',
    configured: false,
    enabled: false,
    revision: 3,
    purpose: 'bench',
    settings: { baseline_controls: { brightness: 20 } },
    presence: 'present',
    readiness: 'unconfigured',
  };
  const { client } = setup({ ...camera, enabled: true, devices: [device] });
  fireEvent.change(await screen.findByLabelText('User name'), {
    target: { value: 'Board view' },
  });
  fireEvent.click(screen.getByRole('button', { name: 'Save camera configuration' }));
  await waitFor(() =>
    expect(client.configureInstrumentDevice).toHaveBeenCalledWith(
      sessionCredential('csrf'),
      'camera',
      'cam-1',
      3,
      'Board view',
      'bench',
      device.settings,
    ),
  );
  await waitFor(() =>
    expect(screen.getAllByRole('button', { name: 'Enable' })[1]).toBeEnabled(),
  );
  fireEvent.click(screen.getAllByRole('button', { name: 'Enable' })[1]);
  await waitFor(() =>
    expect(client.instrumentDeviceAction).toHaveBeenCalledWith(
      sessionCredential('csrf'),
      'camera',
      'cam-1',
      'enable',
      4,
    ),
  );
});

it('displays a labelled error when an action fails', async () => {
  const { client } = setup();
  client.pluginAction.mockRejectedValueOnce(new Error('offline'));
  await screen.findByText('Current version: 0.1.0');
  fireEvent.click(screen.getByRole('button', { name: 'Enable' }));
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'Error: The request could not be completed.',
  );
  expect(screen.getByRole('button', { name: 'Enable' })).toBeEnabled();
});

it('keeps connected cameras available for setup and re-enabling inside the controls frame', async () => {
  const device = {
    id: 'cam-1',
    display_name: 'Ready camera',
    presence: 'present',
    readiness: 'ready',
    configured: true,
    enabled: true,
  };
  setup({
    ...camera,
    enabled: true,
    devices: [
      device,
      {
        ...device,
        id: 'cam-2',
        display_name: 'New camera',
        configured: false,
        readiness: 'unconfigured',
      },
      {
        ...device,
        id: 'cam-3',
        display_name: 'Disabled camera',
        enabled: false,
        readiness: 'failed',
      },
      {
        ...device,
        id: 'cam-4',
        display_name: 'Disconnected camera',
        presence: 'absent',
      },
      { ...device, id: 'cam-5', display_name: 'Unknown camera', presence: 'unknown' },
      { ...device, id: 'cam-6', display_name: 'Failed camera', readiness: 'failed' },
    ],
  });
  await screen.findByDisplayValue('Ready camera');
  const panel = screen.getByRole('region', { name: 'Camera plugin' });
  expect(within(panel).getAllByRole('textbox', { name: 'User name' })).toHaveLength(4);
  expect(within(panel).getByDisplayValue('New camera')).toBeEnabled();
  expect(within(panel).getByDisplayValue('Disabled camera')).toBeEnabled();
  expect(within(panel).getByRole('button', { name: 'Discover cameras' })).toBeEnabled();
  expect(screen.queryByDisplayValue('Disconnected camera')).not.toBeInTheDocument();
  expect(screen.queryByDisplayValue('Unknown camera')).not.toBeInTheDocument();
  expect(screen.queryByText('present', { exact: true })).not.toBeInTheDocument();
  expect(screen.queryByText('ready', { exact: true })).not.toBeInTheDocument();
  expect(
    screen.queryByText('The Camera plugin is enabled and appears under Instruments.'),
  ).not.toBeInTheDocument();
  expect(screen.getByRole('status')).toHaveTextContent(
    'Warning: Camera initialization failed: Failed camera.',
  );
});
