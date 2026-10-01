import { fireEvent, screen, waitFor, within } from '@testing-library/react';
import { afterEach, expect, it, vi } from 'vitest';
import {
  BenchCoreError,
  createBenchCoreClient,
  sessionCredential,
} from '../../api/client';
import type { InstrumentJob } from '../../api/bench';
import type { ModbusEndpoint, ModbusWorkspace, ModbusMap, ModbusRecordingPage, ModbusRecordingEventPage } from '../../api/modbus';
import {
  renderInShell,
  installColorScheme,
  restoreMatchMedia,
} from '../../test/render';
import { ModbusWorkspacePage } from './ModbusWorkspacePage';

const endpoint: ModbusEndpoint = {
  schema_version: 1,
  id: 'boiler',
  revision: 2,
  name: 'Boiler',
  transport: 'tcp',
  resource_id: 'boiler-bus',
  enabled: true,
  presence: 'configured',
  readiness: 'ready',
  network: { host: '192.0.2.9', port: 502, unit_min: 1, unit_max: 4 },
};
const workspace: ModbusWorkspace = {
  schema_version: 1,
  owner_id: 'admin',
  id: 'plant',
  revision: 1,
  name: 'Plant',
  endpoint_id: 'boiler',
  profile_ref: { id: 'standard', revision: 3 },
  map_refs: [{ id: 'registers', revision: 1 }],
  poll_groups: [
    {
      id: 'temperatures',
      name: 'Temperatures',
      enabled: true,
      unit_id: 1,
      function: 3,
      address: 10,
      quantity: 1,
      interval_ms: 1000,
      map_ref: { id: 'registers', revision: 1 },
    },
  ],
  layout: {
    columns: ['name', 'raw', 'value'],
    address_base: 'zero',
    series: ['temperature'],
    display_paused: false,
  },
};

afterEach(() => {
  restoreMatchMedia();
  vi.restoreAllMocks();
});

it('keeps the new live workflow on network endpoints', async () => {
  installColorScheme('light');
  const client = {
    ...createBenchCoreClient({ baseUrl: '' }),
    listModbusEndpoints: vi.fn(async () => []),
    listModbusProfiles: vi.fn(async () => []),
    listModbusMaps: vi.fn(async () => []),
    listModbusWorkspaces: vi.fn(async () => []),
    getModbusSession: vi.fn(async () => ({
      state: 'disconnected',
      endpoint_id: 'serial',
      connected: false,
    })),
  };
  renderInShell(
    <ModbusWorkspacePage
      client={client}
      credential={sessionCredential('csrf')}
      endpoint={{ ...endpoint, id: 'serial', transport: 'rtu', network: undefined }}
      view="live"
      onSessionEnded={vi.fn()}
    />,
  );
  expect(
    await screen.findByText(/supports TCP and RTU-over-TCP endpoints/),
  ).toBeTruthy();
  expect(screen.queryByRole('button', { name: 'Connect' })).toBeNull();
});

it('opens saved recording evidence offline without connecting hardware', async () => {
  installColorScheme('light');
  const connect = vi.fn();
  const listModbusRecordings = vi.fn(async (): Promise<ModbusRecordingPage> => ({
    recordings: [{ id: 'record-1', session_id: 'old-session', endpoint_id: 'boiler', owner_id: 'admin', name: 'October run', tags: [], mode: 'both', sampling: 'every', interval_ms: 0, status: 'completed', started_at: '2026-10-01T10:00:00Z', last_sequence: 2, size_bytes: 200, part_count: 1 }],
    next_cursor: '',
  }));
  const findModbusRecording = vi.fn(async (): Promise<ModbusRecordingEventPage> => ({ events: [{ sequence: 1, timestamp: '2026-10-01T10:00:01Z', elapsed_ns: 1, kind: 'sample', group_id: 'temperatures', request: { function: 3, unit: 1, address: 10, count: 1, values: [] }, result: { request_id: 'read', outcome: 'acknowledged', values: [42], attempts: 1 }, point_id: 'temperature', quality: 'good', raw: [42], typed: { value: 42 } }], next_cursor: 1, count: 1, scanned: 1, no_match: false, truncated: false }));
  const client = {
    ...createBenchCoreClient({ baseUrl: '' }),
    listModbusEndpoints: vi.fn(async () => [endpoint]),
    listModbusProfiles: vi.fn(async () => []),
    listModbusMaps: vi.fn(async () => []),
    listModbusWorkspaces: vi.fn(async () => []),
    getModbusSession: vi.fn(async () => ({ state: 'disconnected', endpoint_id: 'boiler', connected: false })),
    listModbusRecordings,
    findModbusRecording,
    connectModbus: connect,
  };
  renderInShell(<ModbusWorkspacePage client={client} credential={sessionCredential('csrf')} endpoint={endpoint} view="sessions" onSessionEnded={vi.fn()} />);
  expect(await screen.findByText('October run')).toBeTruthy();
  fireEvent.click(screen.getByRole('button', { name: 'Open recording' }));
  expect(await screen.findByText('temperature')).toBeTruthy();
  expect(connect).not.toHaveBeenCalled();
});

it('finds saved evidence, updates tags, and keeps protected recordings after deletion is denied', async () => {
  installColorScheme('light');
  const recording: ModbusRecordingPage['recordings'][number] = {
    id: 'protected-1', session_id: 'old-session', endpoint_id: 'boiler', owner_id: 'admin',
    name: 'October run', tags: [], mode: 'both', sampling: 'every', interval_ms: 0,
    status: 'completed', started_at: '2026-10-01T10:00:00Z', last_sequence: 2,
    size_bytes: 200, part_count: 1,
  };
  const findModbusRecording = vi.fn(async (): Promise<ModbusRecordingEventPage> => ({
    events: [], next_cursor: 0, count: 0, scanned: 0, no_match: true, truncated: false,
  }));
  const updateModbusRecording = vi.fn(async () => ({ ...recording, tags: ['baseline'] }));
  const deleteModbusRecording = vi.fn(async () => {
    throw new BenchCoreError('forbidden', 'protected recording', 'recording_protected');
  });
  const client = {
    ...createBenchCoreClient({ baseUrl: '' }),
    listModbusEndpoints: vi.fn(async () => [endpoint]),
    listModbusProfiles: vi.fn(async () => []),
    listModbusMaps: vi.fn(async () => []),
    listModbusWorkspaces: vi.fn(async () => []),
    getModbusSession: vi.fn(async () => ({ state: 'disconnected', endpoint_id: 'boiler', connected: false })),
    listModbusRecordings: vi.fn(async (): Promise<ModbusRecordingPage> => ({ recordings: [recording], next_cursor: '' })),
    findModbusRecording,
    updateModbusRecording,
    deleteModbusRecording,
  };
  vi.spyOn(window, 'confirm').mockReturnValue(true);
  renderInShell(<ModbusWorkspacePage client={client} credential={sessionCredential('csrf')} endpoint={endpoint} view="sessions" onSessionEnded={vi.fn()} />);
  await screen.findByText('October run');
  fireEvent.click(screen.getByRole('button', { name: 'Open recording' }));
  await waitFor(() => expect(findModbusRecording).toHaveBeenCalledTimes(1));
  fireEvent.change(screen.getByLabelText('Frame hex'), { target: { value: '01 03' } });
  fireEvent.click(screen.getAllByRole('button', { name: 'Find' })[1]);
  await waitFor(() => expect(findModbusRecording).toHaveBeenLastCalledWith(expect.anything(), 'protected-1', 0, expect.objectContaining({ hex: '01 03' })));
  fireEvent.change(screen.getByLabelText('Tags (comma separated)'), { target: { value: 'baseline' } });
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));
  await waitFor(() => expect(updateModbusRecording).toHaveBeenCalledWith(expect.anything(), 'protected-1', 'October run', ['baseline']));
  fireEvent.click(screen.getByRole('button', { name: 'Delete' }));
  await waitFor(() => expect(screen.getByText('recording_protected')).toBeTruthy());
  expect(deleteModbusRecording).toHaveBeenCalledWith(expect.anything(), 'protected-1');
  expect(screen.getAllByText('October run')).toHaveLength(2);
});

it('starts explicit logging from an owned live session', async () => {
  installColorScheme('light');
  const startModbusRecording = vi.fn(async () => ({ id: 'record-1', status: 'active' }));
  const client = {
    ...createBenchCoreClient({ baseUrl: '' }),
    listModbusEndpoints: vi.fn(async () => [endpoint]),
    listModbusProfiles: vi.fn(async () => []),
    listModbusMaps: vi.fn(async () => []),
    listModbusWorkspaces: vi.fn(async () => []),
    getModbusSession: vi.fn(async () => ({ state: 'connected', endpoint_id: 'boiler', session_id: 'live', connected: true, can_control: true, poll_paused: false, display_paused: false, group_stats: {} })),
    startModbusRecording,
  };
  renderInShell(<ModbusWorkspacePage client={client as unknown as ReturnType<typeof createBenchCoreClient>} credential={sessionCredential('csrf')} endpoint={endpoint} view="live" onSessionEnded={vi.fn()} />);
  fireEvent.click(await screen.findByRole('button', { name: 'Start logging' }));
  await waitFor(() => expect(startModbusRecording).toHaveBeenCalledWith(expect.anything(), 'boiler', 'live', expect.objectContaining({ mode: 'both', sampling: 'every' })));
});

it('loads a saved workspace inertly and connects only after the operator acts', async () => {
  installColorScheme('light');
  const connect = vi.fn(
    async () =>
      ({ transaction_id: 'job-1', state: 'pending_approval' }) as InstrumentJob,
  );
  const client = {
    ...createBenchCoreClient({ baseUrl: '' }),
    listModbusEndpoints: vi.fn(async () => [endpoint]),
    listModbusProfiles: vi.fn(async () => []),
    listModbusMaps: vi.fn(async () => []),
    listModbusWorkspaces: vi.fn(async () => [
      { id: 'plant', name: 'Plant', revision: 1, endpoint_id: 'boiler' },
    ]),
    getModbusWorkspace: vi.fn(async () => workspace),
    getModbusMap: vi.fn(async () => ({
      id: 'registers',
      revision: 1,
      name: 'Registers',
      points: [],
    })),
    getModbusSession: vi.fn(async () => ({
      state: 'disconnected',
      endpoint_id: 'boiler',
      connected: false,
    })),
    connectModbus: connect,
  };
  renderInShell(
    <ModbusWorkspacePage
      client={client}
      credential={sessionCredential('csrf')}
      endpoint={endpoint}
      view="live"
      onSessionEnded={vi.fn()}
    />,
  );
  await screen.findByRole('option', { name: /Plant · r1/ });
  const selector = screen.getByRole('combobox', { name: 'Workspace' });
  fireEvent.change(selector, { target: { value: 'plant' } });
  await waitFor(() => expect(client.getModbusWorkspace).toHaveBeenCalled());
  expect(connect).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole('button', { name: 'Connect' }));
  await waitFor(() =>
    expect(connect).toHaveBeenCalledWith(
      expect.anything(),
      'boiler',
      expect.objectContaining({
        endpoint_revision: 2,
        profile_id: 'standard',
        profile_revision: 3,
        groups: [expect.objectContaining({ id: 'temperatures' })],
      }),
    ),
  );
});

it('shows mapped values, paired traffic and stale quality after a failed poll', async () => {
  installColorScheme('light');
  let connected = true;
  const client = {
    ...createBenchCoreClient({ baseUrl: '' }),
    listModbusEndpoints: vi.fn(async () => [endpoint]),
    listModbusProfiles: vi.fn(async () => []),
    listModbusMaps: vi.fn(async () => []),
    listModbusWorkspaces: vi.fn(async () => [
      { id: 'plant', name: 'Plant', revision: 1, endpoint_id: 'boiler' },
    ]),
    getModbusWorkspace: vi.fn(async () => workspace),
    getModbusMap: vi.fn(
      async () =>
        ({
          id: 'registers',
          revision: 1,
          name: 'Registers',
          points: [
            {
              id: 'temperature',
              name: 'Temperature',
              area: 'holding',
              address: 10,
              type: 'uint16',
              width: 1,
              unit: '°C',
              access: 'read',
              write_min: null,
              write_max: null,
            },
          ],
        }) as ModbusMap,
    ),
    getModbusSession: vi.fn(async () => ({
      state: connected ? 'connected' : 'disconnected',
      endpoint_id: 'boiler',
      connected,
      session_id: 'session-1',
      can_control: connected,
      workspace_id: 'plant',
      workspace_revision: 1,
      group_stats: {
        temperatures: {
          paused: false,
          requested_interval_ms: 1000,
          actual_interval_ms: 1030,
          latency_ms: 20,
          missed_polls: 1,
          errors: 1,
          samples: 2,
        },
      },
    })),
    disconnectModbus: vi.fn(async () => {
      connected = false;
    }),
    modbusEvents: vi.fn(
      async (_credential, _endpointId, _sessionId, after: number) => ({
        events:
          after === 0
            ? [
                {
                  sequence: 1,
                  timestamp: '2026-09-30T12:00:00Z',
                  group_id: 'temperatures',
                  request: { function: 3, unit: 1, address: 10, count: 1, values: [] },
                  result: {
                    request_id: 'one',
                    outcome: 'acknowledged',
                    values: [42],
                    attempts: 1,
                  },
                },
                {
                  sequence: 2,
                  timestamp: '2026-09-30T12:00:01Z',
                  group_id: 'temperatures',
                  request: { function: 3, unit: 1, address: 10, count: 1, values: [] },
                  result: {
                    request_id: 'two',
                    outcome: 'timeout',
                    values: [],
                    attempts: 1,
                  },
                },
              ]
            : [],
        next_cursor: after === 0 ? 2 : after,
        gap: after === 2,
      }),
    ),
    decodeModbusPoint: vi.fn(async () => ({
      type: 'uint16',
      value: 4.2,
      raw_words: [42],
      quality: 'good',
    })),
  };
  renderInShell(
    <ModbusWorkspacePage
      client={client}
      credential={sessionCredential('csrf')}
      endpoint={endpoint}
      view="live"
      onSessionEnded={vi.fn()}
    />,
  );
  await screen.findByRole('option', { name: /Plant · r1/ });
  fireEvent.change(screen.getByRole('combobox', { name: 'Workspace' }), {
    target: { value: 'plant' },
  });
  fireEvent.click(await screen.findByRole('button', { name: 'Refresh live data' }));
  const grid = await screen.findByRole('region', { name: 'Registers' });
  expect(within(grid).getByText('Temperature')).toBeTruthy();
  expect(await within(grid).findByText(/4\.2 °C/)).toBeTruthy();
  expect(within(grid).getByRole('cell', { name: 'timeout' })).toBeTruthy();
  expect(within(grid).getByText(/^42/)).toBeTruthy();
  expect(screen.getAllByText(/TX/).length).toBeGreaterThan(0);
  fireEvent.click(screen.getByRole('button', { name: 'Refresh live data' }));
  expect(
    await within(screen.getByRole('region', { name: 'Traffic' })).findByText(
      /Event gap/,
    ),
  ).toBeTruthy();
  expect(within(grid).getByRole('cell', { name: 'stale' })).toBeTruthy();
  expect(
    within(screen.getByRole('region', { name: 'Trends' })).getByText(/Event gap/),
  ).toBeTruthy();
  fireEvent.click(screen.getByRole('button', { name: 'Disconnect' }));
  await waitFor(() =>
    expect(within(grid).getByRole('cell', { name: 'disconnected' })).toBeTruthy(),
  );
  expect(within(grid).getByText(/4\.2 °C/)).toBeTruthy();
});

it('previews exact typed write bytes before one explicit submit', async () => {
  installColorScheme('light');
  const write = vi.fn(async (...args: [unknown, string, string, string, unknown]) => {
    void args;
    return {
      command_id: 'cmd-1',
      outcome: 'acknowledged',
      result: {
        request_id: 'write-1',
        outcome: 'acknowledged',
        values: [42],
        attempts: 1,
      },
    };
  });
  const preview = vi.fn(
    async (_credential, _endpointId, _sessionId, _key, request) => ({
      session_id: 'session-1',
      endpoint_revision: 2,
      profile_revision: 3,
      request,
      pdu_hex: '06000a002a',
      digest: 'digest',
    }),
  );
  const client = {
    ...createBenchCoreClient({ baseUrl: '' }),
    listModbusEndpoints: vi.fn(async () => [endpoint]),
    listModbusProfiles: vi.fn(async () => []),
    listModbusMaps: vi.fn(async () => []),
    listModbusWorkspaces: vi.fn(async () => [
      { id: 'plant', name: 'Plant', revision: 1, endpoint_id: 'boiler' },
    ]),
    getModbusWorkspace: vi.fn(async () => workspace),
    getModbusMap: vi.fn(
      async () =>
        ({
          id: 'registers',
          revision: 1,
          name: 'Registers',
          points: [
            {
              id: 'setpoint',
              name: 'Setpoint',
              area: 'holding',
              address: 10,
              type: 'uint16',
              width: 1,
              unit: '°C',
              access: 'read_write',
              write_min: 0,
              write_max: 10,
            },
          ],
        }) as ModbusMap,
    ),
    getModbusSession: vi.fn(async () => ({
      state: 'connected',
      endpoint_id: 'boiler',
      connected: true,
      session_id: 'session-1',
      can_control: true,
      workspace_id: 'plant',
      workspace_revision: 1,
    })),
    encodeModbusPoint: vi.fn(async () => ({
      point_id: 'setpoint',
      area: 'holding',
      address: 10,
      words: [42],
    })),
    previewModbusWrite: preview,
    writeModbus: write,
  };
  renderInShell(
    <ModbusWorkspacePage
      client={client}
      credential={sessionCredential('csrf')}
      endpoint={endpoint}
      view="live"
      onSessionEnded={vi.fn()}
    />,
  );
  await screen.findByRole('option', { name: /Plant · r1/ });
  fireEvent.change(screen.getByRole('combobox', { name: 'Workspace' }), {
    target: { value: 'plant' },
  });
  fireEvent.click(await screen.findByRole('button', { name: 'Write Setpoint' }));
  fireEvent.change(screen.getByRole('textbox', { name: 'Engineering value' }), {
    target: { value: '4.2' },
  });
  fireEvent.click(screen.getByRole('button', { name: 'Preview write' }));
  expect(await screen.findByText('06000a002a')).toBeTruthy();
  expect(write).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole('button', { name: 'Submit write' }));
  await waitFor(() => expect(write).toHaveBeenCalledTimes(1));
  expect(write.mock.calls[0][3]).toBe(preview.mock.calls[0][3]);
  expect(screen.getByRole('button', { name: 'Submit write' })).toBeDisabled();
  expect(await screen.findByText(/Write outcome: acknowledged/)).toBeTruthy();
  fireEvent.keyDown(screen.getByRole('dialog', { name: 'Write Setpoint' }), {
    key: 'Escape',
  });
  expect(screen.queryByRole('dialog', { name: 'Write Setpoint' })).toBeNull();
  expect(screen.getByRole('button', { name: 'Write Setpoint' })).toHaveFocus();
});

it('bounds traffic and trends while display pause and clear leave acquisition alone', async () => {
  installColorScheme('light');
  const poll = vi.fn();
  const events = Array.from({ length: 220 }, (_, index) => ({
    sequence: index + 1,
    timestamp: '2026-09-30T12:00:00Z',
    group_id: 'temperatures',
    request: { function: 3, unit: 1, address: 10, count: 1, values: [] },
    result: {
      request_id: `read-${index}`,
      outcome: 'acknowledged',
      values: [index],
      attempts: 1,
    },
  }));
  const readEvents = vi.fn(
    async (_credential, _endpointId, _sessionId, after: number) => ({
      events: after === 0 ? events : [],
      next_cursor: after === 0 ? 220 : after,
      gap: false,
    }),
  );
  const client = {
    ...createBenchCoreClient({ baseUrl: '' }),
    listModbusEndpoints: vi.fn(async () => [endpoint]),
    listModbusProfiles: vi.fn(async () => []),
    listModbusMaps: vi.fn(async () => []),
    listModbusWorkspaces: vi.fn(async () => [
      { id: 'plant', name: 'Plant', revision: 1, endpoint_id: 'boiler' },
    ]),
    getModbusWorkspace: vi.fn(async () => workspace),
    getModbusMap: vi.fn(
      async () =>
        ({
          id: 'registers',
          revision: 1,
          name: 'Registers',
          points: [
            {
              id: 'temperature',
              name: 'Temperature',
              area: 'holding',
              address: 10,
              type: 'uint16',
              width: 1,
              unit: '°C',
              access: 'read',
            },
          ],
        }) as ModbusMap,
    ),
    getModbusSession: vi.fn(async () => ({
      state: 'connected',
      endpoint_id: 'boiler',
      connected: true,
      session_id: 'session-1',
      can_control: true,
      workspace_id: 'plant',
      workspace_revision: 1,
    })),
    modbusEvents: readEvents,
    decodeModbusPoint: vi.fn(
      async (_credential, _mapId, _revision, _pointId, raw: number[]) => ({
        type: 'uint16',
        value: raw[0],
        raw_words: raw,
        quality: 'good',
      }),
    ),
    pollModbus: poll,
  };
  renderInShell(
    <ModbusWorkspacePage
      client={client}
      credential={sessionCredential('csrf')}
      endpoint={endpoint}
      view="live"
      onSessionEnded={vi.fn()}
    />,
  );
  await screen.findByRole('option', { name: /Plant · r1/ });
  fireEvent.change(screen.getByRole('combobox', { name: 'Workspace' }), {
    target: { value: 'plant' },
  });
  fireEvent.click(await screen.findByRole('button', { name: 'Refresh live data' }));
  await waitFor(() => expect(screen.getAllByText(/TX: U1/)).toHaveLength(200));
  expect(screen.getByRole('img', { name: 'Trends: 120 samples' })).toBeTruthy();
  fireEvent.click(screen.getByRole('button', { name: 'Pause display' }));
  expect(screen.getByRole('button', { name: 'Refresh live data' })).toBeDisabled();
  expect(screen.getByRole('button', { name: 'Read once' })).toBeDisabled();
  fireEvent.click(screen.getByRole('button', { name: 'Clear display' }));
  expect(screen.queryByText(/TX: U1/)).toBeNull();
  expect(screen.getByRole('img', { name: 'Trends: 0 samples' })).toBeTruthy();
  expect(poll).not.toHaveBeenCalled();
});

it('does not offer a second submit after an unconfirmed write response', async () => {
  installColorScheme('light');
  const write = vi.fn(async () => {
    throw new BenchCoreError('unavailable', 'connection lost');
  });
  const client = {
    ...createBenchCoreClient({ baseUrl: '' }),
    listModbusEndpoints: vi.fn(async () => [endpoint]),
    listModbusProfiles: vi.fn(async () => []),
    listModbusMaps: vi.fn(async () => []),
    listModbusWorkspaces: vi.fn(async () => [
      { id: 'plant', name: 'Plant', revision: 1, endpoint_id: 'boiler' },
    ]),
    getModbusWorkspace: vi.fn(async () => workspace),
    getModbusMap: vi.fn(
      async () =>
        ({
          id: 'registers',
          revision: 1,
          name: 'Registers',
          points: [
            {
              id: 'setpoint',
              name: 'Setpoint',
              area: 'holding',
              address: 10,
              type: 'uint16',
              width: 1,
              unit: '°C',
              access: 'read_write',
              write_min: 0,
              write_max: 10,
            },
          ],
        }) as ModbusMap,
    ),
    getModbusSession: vi.fn(async () => ({
      state: 'connected',
      endpoint_id: 'boiler',
      connected: true,
      session_id: 'session-1',
      can_control: true,
      workspace_id: 'plant',
      workspace_revision: 1,
    })),
    encodeModbusPoint: vi.fn(async () => ({
      point_id: 'setpoint',
      area: 'holding',
      address: 10,
      words: [42],
    })),
    previewModbusWrite: vi.fn(
      async (_credential, _endpointId, _sessionId, _key, request) => ({
        session_id: 'session-1',
        endpoint_revision: 2,
        profile_revision: 3,
        request,
        pdu_hex: '06000a002a',
        digest: 'digest',
      }),
    ),
    writeModbus: write,
  };
  renderInShell(
    <ModbusWorkspacePage
      client={client}
      credential={sessionCredential('csrf')}
      endpoint={endpoint}
      view="live"
      onSessionEnded={vi.fn()}
    />,
  );
  await screen.findByRole('option', { name: /Plant · r1/ });
  fireEvent.change(screen.getByRole('combobox', { name: 'Workspace' }), {
    target: { value: 'plant' },
  });
  fireEvent.click(await screen.findByRole('button', { name: 'Write Setpoint' }));
  fireEvent.change(screen.getByRole('textbox', { name: 'Engineering value' }), {
    target: { value: '4.2' },
  });
  fireEvent.click(screen.getByRole('button', { name: 'Preview write' }));
  fireEvent.click(await screen.findByRole('button', { name: 'Submit write' }));
  expect(await screen.findByText(/write outcome is unconfirmed/i)).toBeTruthy();
  expect(screen.getByRole('button', { name: 'Submit write' })).toBeDisabled();
  expect(write).toHaveBeenCalledTimes(1);
});

it('edits and saves poll groups without starting a session', async () => {
  installColorScheme('light');
  const save = vi.fn(async (_credential, _expected, changed: ModbusWorkspace) => ({
    ...changed,
    revision: 2,
  }));
  const connect = vi.fn();
  const client = {
    ...createBenchCoreClient({ baseUrl: '' }),
    listModbusEndpoints: vi.fn(async () => [endpoint]),
    listModbusProfiles: vi.fn(async () => []),
    listModbusMaps: vi.fn(async () => []),
    listModbusWorkspaces: vi.fn(async () => [
      { id: 'plant', name: 'Plant', revision: 1, endpoint_id: 'boiler' },
    ]),
    getModbusWorkspace: vi.fn(async () => workspace),
    getModbusMap: vi.fn(async () => ({
      id: 'registers',
      revision: 1,
      name: 'Registers',
      points: [],
    })),
    getModbusSession: vi.fn(async () => ({
      state: 'disconnected',
      endpoint_id: 'boiler',
      connected: false,
    })),
    saveModbusWorkspace: save,
    connectModbus: connect,
  };
  renderInShell(
    <ModbusWorkspacePage
      client={client}
      credential={sessionCredential('csrf')}
      endpoint={endpoint}
      view="live"
      onSessionEnded={vi.fn()}
    />,
  );
  await screen.findByRole('option', { name: /Plant · r1/ });
  fireEvent.change(screen.getByRole('combobox', { name: 'Workspace' }), {
    target: { value: 'plant' },
  });
  fireEvent.click(
    await screen.findByRole('button', { name: 'Duplicate Temperatures' }),
  );
  fireEvent.click(screen.getByRole('button', { name: 'Save workspace' }));
  await waitFor(() =>
    expect(save).toHaveBeenCalledWith(
      expect.anything(),
      1,
      expect.objectContaining({
        poll_groups: expect.arrayContaining([
          expect.objectContaining({ id: 'temperatures' }),
          expect.objectContaining({ unit_id: 1, function: 3, quantity: 1 }),
        ]),
      }),
    ),
  );
  expect(connect).not.toHaveBeenCalled();
});
