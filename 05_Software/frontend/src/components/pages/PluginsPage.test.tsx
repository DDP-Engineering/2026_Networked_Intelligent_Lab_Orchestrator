import { fireEvent, screen, waitFor, within } from '@testing-library/react';
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
      view="legacy-sigrok"
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
      view="legacy-sigrok"
    />,
  );
  fireEvent.click(await screen.findByRole('button', { name: 'transaction-one' }));
  expect(await screen.findByText('Result and replies')).toBeVisible();
  expect(screen.queryByRole('button', { name: 'Approve' })).toBeNull();
  expect(screen.queryByRole('button', { name: 'Uninstall' })).toBeNull();
  expect(screen.getByRole('button', { name: 'Queue capture' })).toBeDisabled();
});

it('keeps UART decode actions off a completed camera image', async () => {
  installColorScheme('light');
  const camera: InstrumentPlugin = {
    ...plugin,
    id: 'camera',
    display_name: 'Camera',
    simulation: false,
    operations: ['snapshot'],
    devices: [
      {
        id: 'cam-1',
        display_name: 'Camera',
        kind: 'camera',
        presence: 'present',
        readiness: 'ready',
        configured: true,
        enabled: true,
        revision: 2,
        capabilities: { formats: ['MJPG'], controls: [] },
        settings: {},
      },
    ],
  };
  const snapshot: InstrumentJob = {
    ...job,
    transaction_id: 'camera-transaction',
    plugin_id: 'camera',
    state: 'completed',
    artifact_id: 'image-one',
    submission: {
      ...job.submission,
      plugin_id: 'camera',
      operation: 'snapshot',
      device_id: 'cam-1',
      parameters: { width: 640, height: 480 },
    },
  };
  const client = {
    ...createBenchCoreClient({ baseUrl: '' }),
    listPlugins: async () => [plugin, camera],
    listInstrumentJobs: async () => [snapshot],
    getInstrumentJob: async () => snapshot,
    previewInstrumentArtifact: async () => new Blob([], { type: 'image/jpeg' }),
  };
  renderInShell(
    <PluginsPage
      client={client}
      credential={sessionCredential('csrf')}
      session={session}
      onSessionEnded={() => {}}
      view="snapshot"
      focusDevice="cam-1"
    />,
  );
  fireEvent.click(await screen.findByRole('button', { name: 'camera-transaction' }));
  expect(await screen.findByRole('button', { name: 'Download capture' })).toBeVisible();
  expect(screen.queryByRole('button', { name: 'Decode UART' })).toBeNull();
});

it('requires the exact camera ID before rebinding a serial-less record', async () => {
  installColorScheme('light');
  const camera: InstrumentPlugin = {
    ...plugin,
    id: 'camera',
    display_name: 'Camera',
    simulation: false,
    operations: ['snapshot'],
    devices: [
      {
        id: 'cam-1',
        display_name: 'Board view',
        kind: 'camera',
        presence: 'absent',
        readiness: 'failed',
        configured: true,
        enabled: true,
        revision: 4,
        capabilities: {},
        settings: {},
      },
      {
        id: 'cam-2',
        display_name: 'USB Camera (cam-2)',
        kind: 'camera',
        presence: 'present',
        readiness: 'unconfigured',
        configured: false,
        enabled: true,
        revision: 1,
        capabilities: {},
        settings: {},
      },
    ],
  };
  const rebind = vi.fn(async () => camera.devices[0]);
  const client = {
    ...createBenchCoreClient({ baseUrl: '' }),
    listPlugins: async () => [camera],
    listInstrumentJobs: async () => [],
    rebindInstrumentDevice: rebind,
  };
  renderInShell(
    <PluginsPage
      client={client}
      credential={sessionCredential('csrf')}
      session={session}
      onSessionEnded={() => {}}
      view="administration"
    />,
  );
  fireEvent.click(await screen.findByRole('button', { name: 'Rebind' }));
  const dialog = within(screen.getByRole('dialog'));
  expect(dialog.getByRole('button', { name: 'Rebind camera' })).toBeDisabled();
  fireEvent.change(dialog.getByRole('textbox', { name: 'Camera ID' }), {
    target: { value: 'cam-1' },
  });
  fireEvent.click(dialog.getByRole('button', { name: 'Rebind camera' }));
  await waitFor(() =>
    expect(rebind).toHaveBeenCalledWith(
      sessionCredential('csrf'),
      'camera',
      'cam-1',
      4,
      'cam-2',
      1,
      'cam-1',
    ),
  );
});

it('saves camera mounting and capture defaults with the current device revision', async () => {
  installColorScheme('light');
  const camera: InstrumentPlugin = {
    ...plugin,
    id: 'camera',
    display_name: 'Camera',
    simulation: false,
    operations: ['snapshot'],
    devices: [
      {
        id: 'cam-1',
        display_name: 'Board view',
        kind: 'camera',
        presence: 'present',
        readiness: 'ready',
        configured: true,
        enabled: true,
        revision: 3,
        capabilities: { formats: ['MJPG'], controls: [] },
        settings: {},
      },
    ],
  };
  const configure = vi.fn(async () => camera.devices[0]);
  const client = {
    ...createBenchCoreClient({ baseUrl: '' }),
    listPlugins: async () => [camera],
    listInstrumentJobs: async () => [],
    configureInstrumentDevice: configure,
  };
  renderInShell(
    <PluginsPage
      client={client}
      credential={sessionCredential('csrf')}
      session={session}
      onSessionEnded={() => {}}
      view="administration"
    />,
  );
  fireEvent.change(await screen.findByRole('combobox', { name: 'Mounting rotation' }), {
    target: { value: '90' },
  });
  fireEvent.change(screen.getByRole('spinbutton', { name: 'Default width' }), {
    target: { value: '800' },
  });
  fireEvent.click(screen.getByRole('button', { name: 'Save camera configuration' }));
  await waitFor(() =>
    expect(configure).toHaveBeenCalledWith(
      sessionCredential('csrf'),
      'camera',
      'cam-1',
      3,
      'Board view',
      '',
      expect.objectContaining({ mounting_rotation: 90, default_width: 800 }),
    ),
  );
});

it('submits physical camera composition and encoding options', async () => {
  installColorScheme('light');
  const camera: InstrumentPlugin = {
    ...plugin,
    id: 'camera',
    display_name: 'Camera',
    simulation: false,
    operations: ['snapshot'],
    devices: [
      {
        id: 'cam-1',
        display_name: 'Board view',
        kind: 'camera',
        presence: 'present',
        readiness: 'ready',
        configured: true,
        enabled: true,
        revision: 3,
        capabilities: { formats: ['MJPG'], controls: [] },
        settings: {},
      },
    ],
  };
  const submit = vi.fn(async () => job);
  const client = {
    ...createBenchCoreClient({ baseUrl: '' }),
    listPlugins: async () => [camera],
    listInstrumentJobs: async () => [],
    submitInstrumentJob: submit,
  };
  renderInShell(
    <PluginsPage
      client={client}
      credential={sessionCredential('csrf')}
      session={session}
      onSessionEnded={() => {}}
      view="snapshot"
      focusDevice="cam-1"
    />,
  );
  fireEvent.change(await screen.findByRole('combobox', { name: 'Image encoding' }), {
    target: { value: 'png' },
  });
  fireEvent.change(
    screen.getByRole('spinbutton', { name: 'Maximum image dimension' }),
    { target: { value: '320' } },
  );
  fireEvent.click(screen.getByRole('checkbox', { name: 'Crosshair overlay' }));
  fireEvent.change(screen.getByRole('textbox', { name: 'Text overlay' }), {
    target: { value: 'Board' },
  });
  fireEvent.click(screen.getByRole('button', { name: 'Snapshot' }));
  await waitFor(() =>
    expect(submit).toHaveBeenCalledWith(
      sessionCredential('csrf'),
      expect.objectContaining({
        plugin_id: 'camera',
        operation: 'snapshot',
        device_id: 'cam-1',
        parameters: expect.objectContaining({
          encoding: 'png',
          maximum_dimension: 320,
          processing: expect.objectContaining({
            overlays: [
              { kind: 'crosshair', x: 0.5, y: 0.5 },
              { kind: 'text', x: 0.05, y: 0.05, text: 'Board' },
            ],
          }),
        }),
      }),
    ),
  );
});

it('offers a ready camera quick snapshot and two saved images for comparison', async () => {
  installColorScheme('light');
  const cameras: InstrumentPlugin = {
    ...plugin,
    id: 'camera',
    display_name: 'Camera',
    simulation: false,
    operations: ['snapshot'],
    devices: [
      {
        id: 'cam-1',
        display_name: 'Board view',
        kind: 'camera',
        presence: 'present',
        readiness: 'ready',
        configured: true,
        enabled: true,
        revision: 3,
        capabilities: { formats: ['MJPG'], controls: [] },
        settings: { default_width: 800, default_height: 600 },
      },
      {
        id: 'cam-2',
        display_name: 'Second view',
        kind: 'camera',
        presence: 'present',
        readiness: 'ready',
        configured: true,
        enabled: true,
        revision: 2,
        capabilities: { formats: ['MJPG'], controls: [] },
        settings: {},
      },
    ],
  };
  const snapshots: InstrumentJob[] = cameras.devices.map((device, index) => ({
    ...job,
    transaction_id: `camera-${index + 1}`,
    plugin_id: 'camera',
    state: 'completed',
    artifact_id: `image-${index + 1}`,
    created_at: `2026-09-05T00:00:0${index}Z`,
    submission: {
      ...job.submission,
      plugin_id: 'camera',
      operation: 'snapshot',
      device_id: device.id,
    },
  }));
  const submit = vi.fn(async () => job);
  const client = {
    ...createBenchCoreClient({ baseUrl: '' }),
    listPlugins: async () => [cameras],
    listInstrumentJobs: async () => snapshots,
    submitInstrumentJob: submit,
    previewInstrumentArtifact: async () => new Blob([], { type: 'image/jpeg' }),
  };
  renderInShell(
    <PluginsPage
      client={client}
      credential={sessionCredential('csrf')}
      session={session}
      onSessionEnded={() => {}}
      view="overview"
    />,
  );
  fireEvent.click(
    await screen.findByRole('button', { name: 'Quick snapshot Board view' }),
  );
  await waitFor(() =>
    expect(submit).toHaveBeenCalledWith(
      sessionCredential('csrf'),
      expect.objectContaining({
        plugin_id: 'camera',
        operation: 'snapshot',
        device_id: 'cam-1',
        parameters: expect.objectContaining({
          width: 800,
          height: 600,
          format: 'MJPG',
        }),
      }),
    ),
  );
  expect(
    screen.getByRole('combobox', { name: 'Compare first image' }),
  ).toBeInTheDocument();
  expect(
    screen.getByRole('combobox', { name: 'Compare second image' }),
  ).toBeInTheDocument();
});

it('projects a placed image marker into the next physical snapshot request', async () => {
  installColorScheme('light');
  const prior = Object.getOwnPropertyDescriptor(URL, 'createObjectURL');
  Object.defineProperty(URL, 'createObjectURL', {
    configurable: true,
    value: () => 'blob:camera-preview',
  });
  const camera: InstrumentPlugin = {
    ...plugin,
    id: 'camera',
    display_name: 'Camera',
    simulation: false,
    operations: ['snapshot'],
    devices: [
      {
        id: 'cam-1',
        display_name: 'Board view',
        kind: 'camera',
        presence: 'present',
        readiness: 'ready',
        configured: true,
        enabled: true,
        revision: 3,
        capabilities: { formats: ['MJPG'], controls: [] },
        settings: {},
      },
    ],
  };
  const saved: InstrumentJob = {
    ...job,
    transaction_id: 'saved-one',
    plugin_id: 'camera',
    state: 'completed',
    artifact_id: 'image-one',
    submission: {
      ...job.submission,
      plugin_id: 'camera',
      operation: 'snapshot',
      device_id: 'cam-1',
    },
  };
  const submit = vi.fn(async () => job);
  const client = {
    ...createBenchCoreClient({ baseUrl: '' }),
    listPlugins: async () => [camera],
    listInstrumentJobs: async () => [saved],
    submitInstrumentJob: submit,
    previewInstrumentArtifact: async () => new Blob([], { type: 'image/jpeg' }),
  };
  try {
    renderInShell(
      <PluginsPage
        client={client}
        credential={sessionCredential('csrf')}
        session={session}
        onSessionEnded={() => {}}
        focusDevice="cam-1"
        view="snapshot"
      />,
    );
    const marker = await screen.findByRole('button', { name: 'Place local marker' });
    marker.getBoundingClientRect = () => ({
      left: 0,
      top: 0,
      width: 100,
      height: 100,
      right: 100,
      bottom: 100,
      x: 0,
      y: 0,
      toJSON: () => ({}),
    });
    fireEvent.click(marker, { clientX: 75, clientY: 50 });
    fireEvent.click(screen.getByRole('button', { name: 'Snapshot' }));
    await waitFor(() =>
      expect(submit).toHaveBeenCalledWith(
        sessionCredential('csrf'),
        expect.objectContaining({
          parameters: expect.objectContaining({
            processing: expect.objectContaining({
              overlays: [{ kind: 'crosshair', x: 0.75, y: 0.5, color: '#ff2200' }],
            }),
          }),
        }),
      ),
    );
  } finally {
    if (prior) Object.defineProperty(URL, 'createObjectURL', prior);
    else delete (URL as unknown as Record<string, unknown>).createObjectURL;
  }
});

it('submits line, rectangle and calibrated scale overlays', async () => {
  installColorScheme('light');
  const camera: InstrumentPlugin = {
    ...plugin,
    id: 'camera',
    display_name: 'Camera',
    simulation: false,
    operations: ['snapshot'],
    devices: [
      {
        id: 'cam-1',
        display_name: 'Board view',
        kind: 'camera',
        presence: 'present',
        readiness: 'ready',
        configured: true,
        enabled: true,
        revision: 3,
        capabilities: { formats: ['MJPG'], controls: [] },
        settings: { calibration_mm_per_pixel: 0.1 },
      },
    ],
  };
  const submit = vi.fn(async () => job);
  const client = {
    ...createBenchCoreClient({ baseUrl: '' }),
    listPlugins: async () => [camera],
    listInstrumentJobs: async () => [],
    submitInstrumentJob: submit,
  };
  renderInShell(
    <PluginsPage
      client={client}
      credential={sessionCredential('csrf')}
      session={session}
      onSessionEnded={() => {}}
      focusDevice="cam-1"
      view="snapshot"
    />,
  );
  fireEvent.click(await screen.findByRole('checkbox', { name: 'Line overlay' }));
  fireEvent.change(screen.getByRole('spinbutton', { name: 'Line start X' }), {
    target: { value: '0.2' },
  });
  fireEvent.click(screen.getByRole('checkbox', { name: 'Rectangle overlay' }));
  fireEvent.click(screen.getByRole('checkbox', { name: 'Calibrated scale bar' }));
  fireEvent.change(screen.getByRole('spinbutton', { name: 'Scale bar length (mm)' }), {
    target: { value: '5' },
  });
  fireEvent.click(screen.getByRole('button', { name: 'Snapshot' }));
  await waitFor(() =>
    expect(submit).toHaveBeenCalledWith(
      sessionCredential('csrf'),
      expect.objectContaining({
        parameters: expect.objectContaining({
          processing: expect.objectContaining({
            overlays: [
              expect.objectContaining({
                kind: 'line',
                x: 0.2,
                y: 0.1,
                x2: 0.9,
                y2: 0.9,
              }),
              expect.objectContaining({ kind: 'rectangle' }),
              expect.objectContaining({ kind: 'scale_bar', length_mm: 5 }),
            ],
          }),
        }),
      }),
    ),
  );
});

it('submits a supported physical frame interval', async () => {
  installColorScheme('light');
  const camera: InstrumentPlugin = {
    ...plugin,
    id: 'camera',
    display_name: 'Camera',
    simulation: false,
    operations: ['snapshot'],
    devices: [
      {
        id: 'cam-1',
        display_name: 'Board view',
        kind: 'camera',
        presence: 'present',
        readiness: 'ready',
        configured: true,
        enabled: true,
        revision: 3,
        capabilities: {
          formats: ['MJPG'],
          controls: [],
          frame_intervals: [
            { format: 'MJPG', width: 640, height: 480, numerator: 1, denominator: 30 },
          ],
        },
        settings: {},
      },
    ],
  };
  const submit = vi.fn(async () => job);
  const client = {
    ...createBenchCoreClient({ baseUrl: '' }),
    listPlugins: async () => [camera],
    listInstrumentJobs: async () => [],
    submitInstrumentJob: submit,
  };
  renderInShell(
    <PluginsPage
      client={client}
      credential={sessionCredential('csrf')}
      session={session}
      onSessionEnded={() => {}}
      focusDevice="cam-1"
      view="snapshot"
    />,
  );
  fireEvent.change(await screen.findByRole('combobox', { name: 'Frame interval' }), {
    target: { value: '1/30' },
  });
  fireEvent.click(screen.getByRole('button', { name: 'Snapshot' }));
  await waitFor(() =>
    expect(submit).toHaveBeenCalledWith(
      sessionCredential('csrf'),
      expect.objectContaining({
        parameters: expect.objectContaining({
          frame_interval: { numerator: 1, denominator: 30 },
        }),
      }),
    ),
  );
});

it('keeps Camera appliance controls in System for administrators', async () => {
  installColorScheme('light');
  const camera: InstrumentPlugin = {
    ...plugin,
    id: 'camera',
    display_name: 'Camera',
    simulation: false,
    devices: [
      {
        id: 'cam-1',
        display_name: 'Board view',
        kind: 'camera',
        presence: 'present',
        readiness: 'ready',
        configured: true,
        enabled: true,
        revision: 3,
        capabilities: { formats: ['MJPG'], controls: [] },
        settings: {},
      },
    ],
  };
  const client = {
    ...createBenchCoreClient({ baseUrl: '' }),
    listPlugins: async () => [camera],
    listInstrumentJobs: async () => [],
  };
  const { unmount } = renderInShell(
    <PluginsPage
      client={client}
      credential={sessionCredential('csrf')}
      session={session}
      onSessionEnded={() => {}}
      view="snapshot"
      focusDevice="cam-1"
    />,
  );
  await screen.findByRole('button', { name: 'Snapshot' });
  expect(
    screen.queryByRole('button', { name: 'Save camera configuration' }),
  ).toBeNull();
  expect(screen.queryByRole('button', { name: 'Disable' })).toBeNull();
  unmount();
  renderInShell(
    <PluginsPage
      client={client}
      credential={sessionCredential('csrf')}
      session={{ ...session, role: 'viewer', permissions: ['bench.read'] }}
      onSessionEnded={() => {}}
      view="administration"
    />,
  );
  await screen.findByText('Board view (cam-1)');
  expect(
    screen.queryByRole('button', { name: 'Save camera configuration' }),
  ).toBeNull();
  expect(screen.queryByRole('button', { name: 'Disable' })).toBeNull();
});

it('offers Camera installation from the catalog without showing work controls', async () => {
  installColorScheme('light');
  const camera: InstrumentPlugin = {
    ...plugin,
    id: 'camera',
    display_name: 'Camera',
    installed: false,
    enabled: false,
    simulation: false,
    devices: [],
  };
  const client = {
    ...createBenchCoreClient({ baseUrl: '' }),
    listPlugins: async () => [camera],
    listInstrumentJobs: async () => [],
  };
  renderInShell(
    <PluginsPage
      client={client}
      credential={sessionCredential('csrf')}
      session={session}
      onSessionEnded={() => {}}
      view="catalog"
    />,
  );
  expect(await screen.findByRole('button', { name: 'Install' })).toBeVisible();
  expect(screen.queryByRole('button', { name: 'Snapshot' })).toBeNull();
});
