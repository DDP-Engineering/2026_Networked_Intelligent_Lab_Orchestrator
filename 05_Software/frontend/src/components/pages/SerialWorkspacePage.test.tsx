import { fireEvent, screen, waitFor, within } from '@testing-library/react';
import type { SerialLog } from '../../api/serial';
import { afterEach, expect, it, vi } from 'vitest';
import { createBenchCoreClient, sessionCredential } from '../../api/client';
import type { BenchCoreSession } from '../../api/contract';
import {
  renderInShell,
  installColorScheme,
  restoreMatchMedia,
} from '../../test/render';
import { SerialWorkspacePage } from './SerialWorkspacePage';
import { CameraPluginPage } from './CameraPluginPage';
const session: BenchCoreSession = {
  apiVersion: 'nilo.http/1',
  username: 'admin',
  role: 'administrator',
  permissions: ['bench.read', 'bench.execute', 'plugins.manage', 'bench.approve'],
  credential: 'session',
  csrfToken: 'csrf',
};
const device = {
  id: 'serial-1',
  display_name: 'Console',
  revision: 1,
  configured: true,
  enabled: true,
  presence: 'present',
  readiness: 'ready',
};
afterEach(() => {
  restoreMatchMedia();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});
function setup(
  view: 'live' | 'sessions' = 'live',
  connected = true,
  logs: SerialLog[] = [],
) {
  installColorScheme('light');
  const calls: { path: string; body: unknown }[] = [];
  vi.stubGlobal(
    'fetch',
    vi.fn(async (path: string, options: RequestInit) => {
      calls.push({
        path,
        body: options.body ? JSON.parse(String(options.body)) : undefined,
      });
      const body = path.endsWith('/workspace')
        ? {
            device_id: device.id,
            connected,
            can_control: connected,
            connection_id: 'connection-1',
            logging: false,
            rx_bytes: 2,
            tx_bytes: 0,
            signals: { rts: null, dtr: null },
          }
        : path.includes('/read?')
          ? {
              events: [
                {
                  sequence: 1,
                  timestamp: '2026-09-27T01:00:00Z',
                  direction: 'rx',
                  data_base64: 'SGkK',
                  byte_count: 3,
                },
              ],
              next_cursor: 1,
              gap: false,
            }
          : path.endsWith('/send')
            ? { status: 'sent', bytes_written: 2 }
            : path.includes('/logs?')
              ? { logs, total: logs.length, offset: 0 }
              : path.endsWith('/retention')
                ? {
                    revision: 1,
                    limits: {
                      files: 100,
                      total_bytes: 2147483648,
                      file_bytes: 104857600,
                    },
                    usage_bytes: 0,
                  }
                : {};
      return new Response(JSON.stringify(body));
    }),
  );
  renderInShell(
    <SerialWorkspacePage
      client={createBenchCoreClient({ baseUrl: '' })}
      credential={sessionCredential('csrf')}
      session={session}
      device={device}
      view={view}
      onSessionEnded={vi.fn()}
    />,
  );
  return calls;
}
it('shows live bytes and submits selected hex and line ending once', async () => {
  const calls = setup();
  await screen.findByText(/RX Hi/);
  fireEvent.change(screen.getByLabelText('Encoding'), { target: { value: 'hex' } });
  fireEvent.change(screen.getByLabelText('Line ending'), { target: { value: 'crlf' } });
  fireEvent.change(screen.getByLabelText('Message to send'), {
    target: { value: 'ff 00' },
  });
  fireEvent.click(screen.getByRole('button', { name: 'Send' }));
  await waitFor(() =>
    expect(calls.filter((call) => call.path.endsWith('/send'))).toHaveLength(1),
  );
  expect(calls.find((call) => call.path.endsWith('/send'))?.body).toMatchObject({
    connection_id: 'connection-1',
    encoding: 'hex',
    data: 'ff 00',
    ending: 'crlf',
  });
  expect(screen.getByRole('button', { name: 'Connect' })).toBeDisabled();
});
it('disables transmission when disconnected and provides collapsed port settings', async () => {
  setup('live', false);
  await screen.findByText('Disconnected');
  expect(screen.getByRole('button', { name: 'Send' })).toBeDisabled();
  expect(screen.getByText('Port settings').closest('details')).not.toHaveAttribute(
    'open',
  );
});
it('groups live settings in separate collapsed cards and reflects fullscreen exit', async () => {
  setup('live', false);
  await screen.findByText('Disconnected');
  const cards = ['Port settings', 'Display settings', 'Transmit and signals'].map(
    (label) => screen.getByText(label, { selector: 'summary' }).closest('details')!,
  );
  for (const card of cards) {
    expect(card).toHaveClass('instrument-card');
    expect(card).not.toHaveAttribute('open');
  }
  expect(new Set(cards.map((card) => card.parentElement)).size).toBe(1);
  const workspace = screen.getByRole('heading', { name: /Console/ }).closest('.page');
  Object.defineProperty(document, 'fullscreenElement', {
    configurable: true,
    value: workspace,
  });
  try {
    fireEvent(document, new Event('fullscreenchange'));
    expect(screen.getByRole('button', { name: 'Exit full screen' })).toBeVisible();
    Object.defineProperty(document, 'fullscreenElement', {
      configurable: true,
      value: null,
    });
    fireEvent(document, new Event('fullscreenchange'));
    expect(screen.getByRole('button', { name: 'Full screen' })).toBeVisible();
  } finally {
    Reflect.deleteProperty(document, 'fullscreenElement');
  }
});
it('shows empty sessions and admin storage defaults without fabricated logs', async () => {
  setup('sessions');
  await screen.findByText('No saved sessions.');
  expect(screen.getByLabelText('Keep last files')).toHaveValue(100);
  expect(screen.getByLabelText('Maximum file size (MiB)')).toHaveValue(100);
});
it('reconciles a read that finishes after disconnect without reporting a false error', async () => {
  setup();
  const original = globalThis.fetch;
  let closed = false;
  vi.stubGlobal(
    'fetch',
    vi.fn(async (path: string, options: RequestInit) => {
      if (path.includes('/read?')) {
        closed = true;
        return new Response('{}', { status: 503 });
      }
      if (closed && path.endsWith('/workspace')) {
        return new Response(
          JSON.stringify({
            connected: false,
            can_control: false,
            warning: 'disconnected',
          }),
        );
      }
      return original(path, options);
    }),
  );
  await waitFor(() => expect(closed).toBe(true));
  await waitFor(() =>
    expect(screen.getByRole('button', { name: 'Connect' })).toBeEnabled(),
  );
  expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  expect(screen.queryByText('disconnected', { exact: true })).not.toBeInTheDocument();
});
it.each([401, 403, 503])(
  'keeps an active read failure visible for HTTP %s',
  async (status) => {
    setup();
    const original = globalThis.fetch;
    vi.stubGlobal(
      'fetch',
      vi.fn(async (path: string, options: RequestInit) => {
        if (path.includes('/read?')) return new Response('{}', { status });
        return original(path, options);
      }),
    );
    expect(await screen.findByRole('alert')).toHaveTextContent(/Error/);
  },
);
it('keeps forbidden reads visible even if the port has since disconnected', async () => {
  setup();
  const original = globalThis.fetch;
  let denied = false;
  vi.stubGlobal(
    'fetch',
    vi.fn(async (path: string, options: RequestInit) => {
      if (path.includes('/read?')) {
        denied = true;
        return new Response('{}', { status: 403 });
      }
      if (denied && path.endsWith('/workspace')) {
        return new Response(JSON.stringify({ connected: false, can_control: false }));
      }
      return original(path, options);
    }),
  );
  expect(await screen.findByRole('alert')).toHaveTextContent(/Error/);
});
it('retains the send key after a lost response so retry cannot replay TX', async () => {
  setup();
  await screen.findByText(/RX Hi/);
  const original = globalThis.fetch;
  const sent: { idempotency_key: string }[] = [];
  vi.stubGlobal(
    'fetch',
    vi.fn(async (path: string, options: RequestInit) => {
      if (path.endsWith('/send')) {
        sent.push(JSON.parse(String(options.body)));
        if (sent.length === 1) throw new Error('connection lost');
      }
      return original(path, options);
    }),
  );
  fireEvent.change(screen.getByLabelText('Message to send'), {
    target: { value: 'hello' },
  });
  fireEvent.click(screen.getByRole('button', { name: 'Send' }));
  await screen.findByText(/connection lost/);
  await waitFor(() =>
    expect(screen.getByRole('button', { name: 'Send' })).toBeEnabled(),
  );
  fireEvent.click(screen.getByRole('button', { name: 'Send' }));
  await waitFor(() => expect(sent).toHaveLength(2));
  expect(sent[1].idempotency_key).toBe(sent[0].idempotency_key);
});
it('exports an authorized log URL and confirms selected deletion', async () => {
  const log: SerialLog = {
    id: 'log-1',
    device_id: 'serial-1',
    connection_id: 'connection-1',
    owner_id: 'owner',
    started_at: '2026-09-27T01:00:00Z',
    ended_at: '2026-09-27T01:01:00Z',
    status: 'completed',
    rx_bytes: 20,
    tx_bytes: 0,
    size_bytes: 300,
  };
  const calls = setup('sessions', false, [log]);
  await screen.findByRole('button', { name: 'View session' });
  const click = vi
    .spyOn(HTMLAnchorElement.prototype, 'click')
    .mockImplementation(function (this: HTMLAnchorElement) {
      expect(this.href).toContain('/api/v1/serial-logs/log-1/content?format=rx');
    });
  fireEvent.change(screen.getByLabelText('Export'), { target: { value: 'rx' } });
  fireEvent.click(screen.getByRole('button', { name: 'Export' }));
  expect(click).toHaveBeenCalledOnce();
  fireEvent.click(screen.getByRole('checkbox', { name: /Sessions/ }));
  fireEvent.click(screen.getByRole('button', { name: 'Delete selected (1)' }));
  expect(calls.some((call) => call.path === '/api/v1/serial-logs/log-1')).toBe(false);
  fireEvent.click(
    within(screen.getByRole('dialog')).getByRole('button', {
      name: 'Delete',
    }),
  );
  await waitFor(() =>
    expect(calls.some((call) => call.path === '/api/v1/serial-logs/log-1')).toBe(true),
  );
});
it('uses serial discovery and empty state in the shared admin layout', async () => {
  installColorScheme('light');
  const client = {
    ...createBenchCoreClient({ baseUrl: '' }),
    listPlugins: vi.fn(async () => [
      {
        id: 'serial',
        display_name: 'USB to Serial',
        version: '0.10.0',
        installed: true,
        enabled: true,
        mode: 'ask' as const,
        status: 'ready',
        simulation: false,
        operations: [],
        devices: [],
        error: '',
        digest: '',
      },
    ]),
    discoverInstruments: vi.fn(async () => []),
  };
  renderInShell(
    <CameraPluginPage
      pluginId="serial"
      client={client}
      credential={sessionCredential('csrf')}
      session={session}
      onSessionEnded={vi.fn()}
    />,
  );
  await screen.findByText('No USB serial ports discovered.');
  fireEvent.click(screen.getByRole('button', { name: 'Discover ports' }));
  await waitFor(() =>
    expect(client.discoverInstruments).toHaveBeenCalledWith(
      sessionCredential('csrf'),
      'serial',
    ),
  );
});
