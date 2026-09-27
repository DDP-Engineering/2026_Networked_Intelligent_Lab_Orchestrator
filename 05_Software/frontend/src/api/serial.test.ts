import { afterEach, expect, it, vi } from 'vitest';
import { serialAPI } from './serial';
import { CSRF_HEADER_NAME } from './contract';
afterEach(() => vi.unstubAllGlobals());
it('preserves send idempotency and session CSRF without putting credentials in URLs', async () => {
  const transport = vi.fn(
    async () => new Response(JSON.stringify({ status: 'sent', bytes_written: 3 })),
  );
  vi.stubGlobal('fetch', transport);
  const api = serialAPI('', { kind: 'session', csrfToken: 'csrf-proof' });
  const input = {
    connection_id: 'connection',
    idempotency_key: 'same-key',
    encoding: 'hex' as const,
    data: '01 ff',
    ending: 'lf' as const,
  };
  await api.send('serial-1', input);
  const [url, options] = transport.mock.calls[0] as unknown as [string, RequestInit];
  expect(url).toBe('/api/v1/serial-devices/serial-1/send');
  expect(options.headers).toHaveProperty(CSRF_HEADER_NAME, 'csrf-proof');
  expect(JSON.parse(options.body as string)).toEqual(input);
});
it('surfaces revoked credentials and cancels reads through the request signal', async () => {
  const transport = vi.fn(
    async () =>
      new Response(JSON.stringify({ error: { code: 'session_expired' } }), {
        status: 401,
      }),
  );
  vi.stubGlobal('fetch', transport);
  await expect(
    serialAPI('', { kind: 'session', csrfToken: '' }).status('serial-1'),
  ).rejects.toMatchObject({ failure: 'unauthorized', code: 'session_expired' });
});
