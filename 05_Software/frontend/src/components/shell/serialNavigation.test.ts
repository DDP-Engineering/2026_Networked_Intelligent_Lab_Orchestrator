import { expect, it } from 'vitest';
import { buildPages } from './pages';
import { parsePlugins } from '../../api/bench';
import type { BenchCoreSession } from '../../api/contract';

const serial = {
  id: 'serial',
  display_name: 'USB to Serial',
  version: '0.10.0',
  installed: true,
  enabled: true,
  mode: 'ask' as const,
  status: 'ready',
  simulation: false,
  operations: ['connect'],
  error: '',
  digest: '',
  devices: [
    {
      id: 'serial-1',
      display_name: 'Console',
      kind: 'usb-serial',
      configured: true,
      enabled: true,
      presence: 'present',
      readiness: 'ready',
      revision: 1,
      capabilities: {},
      settings: {},
    },
  ],
};
const admin: BenchCoreSession = {
  apiVersion: 'nilo.http/1',
  username: 'admin',
  role: 'administrator',
  permissions: ['plugins.manage'],
  credential: 'session',
  csrfToken: 'csrf',
};

it('accepts managed USB serial inventory without analyzer channel fields', () => {
  expect(parsePlugins({ plugins: [serial] }).ok).toBe(true);
});
it('groups enabled serial devices with realtime and sessions destinations', () => {
  const pages = buildPages(admin, [], [], [serial]);
  expect(pages.find((p) => p.id === 'integration:serial')).toMatchObject({
    navigationOnly: true,
  });
  expect(pages.find((p) => p.id === 'serial:serial-1')).toMatchObject({
    serialView: 'live',
    serialDeviceID: 'serial-1',
  });
  expect(pages.find((p) => p.id === 'serial:serial-1:sessions')).toMatchObject({
    serialView: 'sessions',
  });
});
it('restricts serial administration and hides disabled instrument entries', () => {
  const viewer = { ...admin, role: 'viewer' as const, permissions: ['bench.read'] };
  expect(
    buildPages(viewer, [], [], [serial]).some((p) => p.id === 'plugins:serial'),
  ).toBe(false);
  expect(
    buildPages(admin, [], [], [{ ...serial, enabled: false }]).some(
      (p) => p.id === 'serial:serial-1',
    ),
  ).toBe(false);
});

it.each([
  { presence: 'absent' },
  { presence: 'unknown' },
  { readiness: 'initializing' },
  { readiness: 'failed' },
  { configured: false },
  { enabled: false },
])('hides serial ports that are not usable: %j', (change) => {
  const pages = buildPages(
    admin,
    [],
    [],
    [{ ...serial, devices: [{ ...serial.devices[0], ...change }] }],
  );
  expect(pages.filter((page) => page.cluster === 'instruments')).toEqual([]);
  expect(pages.some((page) => page.id === 'plugins:serial')).toBe(true);
});
