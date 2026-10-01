import { createRoot } from 'react-dom/client';
import { createBenchCoreClient, sessionCredential } from '../api/client';
import type { InstrumentPlugin } from '../api/bench';
import type {
  ModbusEndpoint,
  ModbusProfile,
  ModbusWorkspace,
  ModbusMap,
} from '../api/modbus';
import { ModbusPluginPage } from '../components/pages/ModbusPluginPage';
import { ModbusWorkspacePage } from '../components/pages/ModbusWorkspacePage';
import { LanguageProvider } from '../i18n/LanguageProvider';
import type { LanguageCode } from '../i18n/languages';
import { ThemeProvider } from '../theme/ThemeProvider';
import '../index.css';

const query = new URLSearchParams(location.search);
const language = (query.get('lang') as LanguageCode) || 'en';
const endpoint: ModbusEndpoint = {
  schema_version: 1,
  id: 'boiler',
  revision: 2,
  name: 'Offline boiler',
  transport: 'tcp',
  resource_id: 'boiler-bus',
  enabled: true,
  presence: 'configured',
  readiness: 'offline',
  network: { host: '192.0.2.9', port: 502, unit_min: 1, unit_max: 4 },
};
const profile: ModbusProfile = {
  schema_version: 1,
  id: 'default',
  revision: 1,
  owner_id: 'admin',
  name: 'Default',
  endpoint_id: 'boiler',
  settings: {
    response_timeout_ms: 1000,
    connect_timeout_ms: 1000,
    inter_request_ms: 0,
    read_retries: 0,
  },
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
const workspace: ModbusWorkspace = {
  schema_version: 1,
  id: 'plant',
  revision: 1,
  owner_id: 'admin',
  name: 'Plant',
  endpoint_id: 'boiler',
  profile_ref: { id: 'default', revision: 1 },
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
    address_base: 'reference',
    series: ['temperature'],
    display_paused: false,
  },
};
const map: ModbusMap = {
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
      access: 'read_write',
      unit: '°C',
      write_min: 0,
      write_max: 100,
    },
  ],
};
const client = {
  ...createBenchCoreClient({ baseUrl: '' }),
  listPlugins: async () => [plugin],
  listModbusEndpoints: async () => [endpoint],
  listModbusProfiles: async () => [profile],
  listModbusMaps: async () => [],
  listModbusWorkspaces: async () => [
    { id: 'plant', name: 'Plant', revision: 1, endpoint_id: 'boiler' },
  ],
  getModbusWorkspace: async () => workspace,
  getModbusMap: async () => map,
  listModbusRecordings: async () => ({ recordings: [{ id: '00000000-0000-4000-8000-000000000001', session_id: 'old-session', endpoint_id: 'boiler', owner_id: 'admin', name: 'October run', tags: ['commissioning'], mode: 'both' as const, sampling: 'every' as const, interval_ms: 0, status: 'completed' as const, started_at: '2026-10-01T10:00:00Z', last_sequence: 2, size_bytes: 200, part_count: 1 }], next_cursor: '', count: 1, no_match: false }),
  findModbusRecording: async () => ({ events: [{ sequence: 1, timestamp: '2026-10-01T10:00:01Z', elapsed_ns: 1000000000, kind: 'sample' as const, group_id: 'temperatures', request: { function: 3, unit: 1, address: 10, count: 1, values: [] }, result: { request_id: 'read', outcome: 'acknowledged', values: [42], attempts: 1 }, point_id: 'temperature', quality: 'good', raw: [42], typed: { value: 42 } }, { sequence: 2, timestamp: '2026-10-01T10:00:02Z', elapsed_ns: 2000000000, kind: 'sample' as const, group_id: 'temperatures', request: { function: 3, unit: 1, address: 10, count: 1, values: [] }, result: { request_id: 'read', outcome: 'acknowledged', values: [43], attempts: 1 }, point_id: 'temperature', quality: 'good', raw: [43], typed: { value: 43 } }], next_cursor: 2, count: 2, scanned: 2, no_match: false, truncated: false }),
  modbusEvents: async (
    _credential: unknown,
    _endpointId: string,
    _sessionId: string,
    after: number,
  ) => ({ events: [], next_cursor: after, gap: false }),
  getModbusSession: async () => ({
    state: query.get('connected') === '1' ? 'connected' : 'disconnected',
    endpoint_id: 'boiler',
    connected: query.get('connected') === '1',
    session_id: query.get('connected') === '1' ? 'session-fixture' : '',
    can_control: true,
    workspace_id: 'plant',
    workspace_revision: 1,
  }),
  getModbusSettings: async () => ({
    revision: 1,
    settings: {
      max_sessions: 4,
      session_lease_seconds: 3600,
      idle_seconds: 300,
      retention_count: 100,
      retention_bytes: 2147483648,
      part_bytes: 104857600,
    },
  }),
};
const credential = sessionCredential('test-csrf');
createRoot(document.getElementById('root')!).render(
  <ThemeProvider initial={query.get('theme') === 'dark' ? 'dark' : 'light'}>
    <LanguageProvider initial={language}>
      <main style={{ minWidth: 0, minHeight: '100dvh' }}>
        {query.get('view') === 'workspace' || query.get('view') === 'live' || query.get('view') === 'sessions' ? (
          <ModbusWorkspacePage
            client={client}
            credential={credential}
            endpoint={endpoint}
            view={query.get('view') === 'live' ? 'live' : query.get('view') === 'sessions' ? 'sessions' : 'maps'}
            onSessionEnded={() => {}}
          />
        ) : (
          <ModbusPluginPage
            client={client}
            credential={credential}
            session={{
              apiVersion: 'nilo.http/1',
              username: 'admin',
              role: 'administrator',
              permissions: ['bench.read', 'bench.execute', 'plugins.manage'],
              credential: 'session',
              csrfToken: 'test-csrf',
            }}
            onSessionEnded={() => {}}
          />
        )}
      </main>
    </LanguageProvider>
  </ThemeProvider>,
);
