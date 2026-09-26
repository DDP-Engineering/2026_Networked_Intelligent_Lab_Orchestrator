import { fireEvent, screen, waitFor } from '@testing-library/react';
import { afterEach, expect, it, vi } from 'vitest';
import { createBenchCoreClient, sessionCredential } from '../../api/client';
import type {
  InstrumentDevice,
  InstrumentJob,
  InstrumentSubmission,
} from '../../api/bench';
import type { BenchCoreSession } from '../../api/contract';
import {
  defaultCameraSettings,
  type CameraWorkspaceState,
} from '../../api/cameraWorkspace';
import {
  installColorScheme,
  renderInShell,
  restoreMatchMedia,
} from '../../test/render';
import { CameraWorkspacePage } from './CameraWorkspacePage';

const session: BenchCoreSession = {
  apiVersion: 'nilo.http/1',
  username: 'admin',
  role: 'administrator',
  permissions: ['bench.read', 'bench.execute', 'bench.approve', 'plugins.manage'],
  credential: 'session',
  csrfToken: 'csrf',
};
const device: InstrumentDevice = {
  id: 'cam-1',
  display_name: 'Board camera',
  kind: 'camera',
  presence: 'present',
  readiness: 'ready',
  configured: true,
  enabled: true,
  revision: 1,
  capabilities: {
    formats: ['MJPG'],
    sizes: {
      MJPG: [
        [640, 480],
        [1280, 720],
        [1920, 1080],
      ],
    },
    controls: [
      {
        id: 'brightness',
        display_name: 'Brightness',
        type: 'integer',
        current: 0,
        minimum: -64,
        maximum: 64,
        step: 1,
        read_only: false,
        inactive: false,
      },
      {
        id: 'gamma',
        display_name: 'Gamma',
        type: 'integer',
        current: 100,
        minimum: 72,
        maximum: 500,
        step: 1,
        read_only: false,
        inactive: false,
      },
    ],
  },
};
const base: CameraWorkspaceState = {
  state: 'stopped',
  revision: 4,
  settings: defaultCameraSettings,
  retention: {
    snapshots: 100,
    total_bytes: 2 * 2 ** 30,
    video_bytes: 500 * 2 ** 20,
    video_seconds: 600,
  },
  used_bytes: 0,
};
afterEach(() => {
  restoreMatchMedia();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

function setup(view: 'live' | 'recordings', state = base) {
  installColorScheme('light');
  const fetcher = vi.fn(async (input: RequestInfo | URL) => {
    const path = String(input);
    if (path.endsWith('/workspace')) return Response.json(state);
    if (path.includes('/media?')) return Response.json({ items: [] });
    if (path.endsWith('/frame')) return new Response('', { status: 503 });
    if (path.endsWith('/stop')) return Response.json({ stopped: true });
    throw new Error(`Unexpected request ${path}`);
  });
  vi.stubGlobal('fetch', fetcher);
  const submit = vi.fn(
    async (
      _credential: unknown,
      input: InstrumentSubmission,
    ): Promise<InstrumentJob> => ({
      transaction_id: 'job-1',
      session_id: 'session-1',
      request_id: 'request-1',
      plugin_id: 'camera',
      owner_id: 'owner',
      state: 'pending_approval',
      submission: input,
      created_at: '2026-09-26T00:00:00Z',
      deadline: '2026-09-26T00:00:35Z',
      result: {},
      replies: [],
      error: '',
      artifact_id: '',
    }),
  );
  const client = {
    ...createBenchCoreClient({ baseUrl: '' }),
    listInstrumentJobs: vi.fn(async () => []),
    submitInstrumentJob: submit,
  };
  renderInShell(
    <CameraWorkspacePage
      client={client}
      credential={sessionCredential('csrf')}
      session={session}
      device={device}
      view={view}
    />,
  );
  return { submit, fetcher };
}

it('retains hardware controls and a combined resolution selector', async () => {
  setup('live');
  fireEvent.click(await screen.findByText('Camera settings'));
  fireEvent.click(screen.getByText('Hardware controls'));
  expect(await screen.findByLabelText('Brightness')).toHaveValue(0);
  expect(screen.getByLabelText('Gamma')).toHaveValue(100);
  expect(screen.getByLabelText('Resolution')).toHaveValue('640x480');
  expect(screen.getByRole('option', { name: '1280×720' })).toBeInTheDocument();
  expect(screen.queryByRole('option', { name: '1920×1080' })).not.toBeInTheDocument();
  expect(screen.getByLabelText(/Digital zoom/)).toHaveAttribute('type', 'range');
  expect(screen.getByRole('button', { name: 'Take snapshot' })).toBeDisabled();
});

it('presents one retention policy with the complete capture inventory', async () => {
  setup('recordings', {
    ...base,
    retention_inventory: { count: 27, bytes: 3 * 2 ** 20 },
  });
  fireEvent.click(await screen.findByRole('button', { name: 'Save limits' }));
  expect(await screen.findByText(/27 completed captures/)).toHaveTextContent('3.0 MiB');
  expect(screen.queryByLabelText(/Include older captures/)).not.toBeInTheDocument();
  expect(
    screen.queryByRole('heading', { name: 'Existing snapshots' }),
  ).not.toBeInTheDocument();
});

it('binds snapshot approval to the current camera settings revision', async () => {
  const { submit } = setup('live', { ...base, state: 'live' });
  await waitFor(() =>
    expect(screen.getByRole('button', { name: 'Take snapshot' })).toBeEnabled(),
  );
  fireEvent.click(screen.getByRole('button', { name: 'Take snapshot' }));
  await waitFor(() =>
    expect(submit).toHaveBeenCalledWith(
      expect.anything(),
      expect.objectContaining({
        operation: 'workspace',
        parameters: { action: 'snapshot', revision: 4 },
      }),
    ),
  );
  expect(
    await screen.findByText('This operation requires approval.'),
  ).toBeInTheDocument();
});

it('keeps Stop recording available and bypasses a new approval while freezing settings', async () => {
  const { submit, fetcher } = setup('live', {
    ...base,
    state: 'recording',
    recording: { id: 'media-1', elapsed: 3, bytes: 1000, frames: 45, mode: 'video' },
  });
  await waitFor(() => expect(screen.getByLabelText('Resolution')).toBeDisabled());
  expect(screen.getByLabelText('Brightness')).toBeDisabled();
  fireEvent.click(screen.getByRole('button', { name: 'Stop recording' }));
  await waitFor(() =>
    expect(fetcher).toHaveBeenCalledWith(
      '/api/v1/cameras/cam-1/stop',
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({ action: 'record_stop' }),
        headers: expect.objectContaining({ 'X-NILO-CSRF': 'csrf' }),
      }),
    ),
  );
  expect(submit).not.toHaveBeenCalled();
});

it('keeps all three live settings groups collapsed until opened', async () => {
  setup('live');
  for (const label of ['Camera settings', 'Overlays', 'Hardware controls']) {
    const summary = await screen.findByText(label, { selector: 'summary' });
    const section = summary.closest('details');
    expect(section).not.toHaveAttribute('open');
    fireEvent.click(summary);
    expect(section).toHaveAttribute('open');
    fireEvent.click(summary);
    expect(section).not.toHaveAttribute('open');
  }
  fireEvent.click(screen.getByText('Overlays', { selector: 'summary' }));
  expect(screen.getByLabelText('Crosshair')).toBeVisible();
  expect(screen.getByLabelText('Grid')).toBeVisible();
});
