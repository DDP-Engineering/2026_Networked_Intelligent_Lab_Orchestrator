import type { BenchCoreSession } from '../../api/contract';
import { describe, expect, it } from 'vitest';

import { buildPages, findPage, integrationPageId } from './pages';

describe('physical Camera navigation', () => {
  const admin: BenchCoreSession = {
    apiVersion: 'nilo.http/1',
    username: 'admin',
    role: 'administrator',
    permissions: ['plugins.manage'],
    credential: 'session',
    csrfToken: 'csrf',
  };
  const camera = {
    id: 'camera',
    displayName: 'Camera',
    status: 'ready',
    simulation: false,
    operations: ['snapshot'],
  };
  const packageRecord = {
    id: 'camera',
    display_name: 'Camera',
    version: '0.1.0',
    installed: true,
    enabled: true,
    mode: 'ask' as const,
    status: 'ready',
    simulation: false,
    operations: ['snapshot'],
    devices: [
      {
        id: 'cam-1',
        display_name: 'Board view',
        configured: true,
        enabled: true,
        presence: 'present',
        readiness: 'ready',
      },
      {
        id: 'cam-2',
        display_name: 'USB Camera (cam-2)',
        configured: false,
        enabled: true,
        presence: 'present',
        readiness: 'unconfigured',
      },
    ],
    error: '',
    digest: '',
  };
  const devices = [
    {
      id: 'cam-1',
      integrationId: 'camera',
      displayName: 'Board view',
      kind: 'camera',
      status: 'ready',
      simulation: false,
      capabilities: [],
      configured: true,
    },
    {
      id: 'cam-2',
      integrationId: 'camera',
      displayName: 'USB Camera (cam-2)',
      kind: 'camera',
      status: 'ready',
      simulation: false,
      capabilities: [],
      configured: false,
    },
  ];

  it('groups configured physical cameras under Instruments when Camera is enabled', () => {
    const pages = buildPages(admin, [camera], devices, [packageRecord]);
    expect(pages.find((page) => page.id === integrationPageId('camera'))).toMatchObject(
      { cluster: 'instruments', navigationOnly: true },
    );
    expect(findPage(pages, integrationPageId('camera'))).toBeUndefined();
    expect(pages.find((page) => page.id === 'plugins:camera')).toMatchObject({
      cluster: 'core',
      parentId: 'plugins',
      title: 'Camera',
      pluginId: 'camera',
    });
    expect(pages.find((page) => page.id === 'camera:cam-1')).toMatchObject({
      cluster: 'instruments',
      parentId: integrationPageId('camera'),
      title: 'Board view',
      cameraDeviceID: 'cam-1',
    });
    expect(pages.some((page) => page.id === 'camera:cam-1:settings')).toBe(false);
    expect(pages.find((page) => page.id === 'camera:cam-1')).toMatchObject({
      cameraView: 'live',
    });
    expect(pages.some((page) => page.id === 'camera:cam-2')).toBe(false);
  });

  it('hides Camera administration from operators and unauthenticated sessions', () => {
    for (const session of [null, { ...admin, role: 'operator' as const }]) {
      expect(
        buildPages(session, [camera], devices, [packageRecord]).some(
          (page) => page.id === 'plugins:camera',
        ),
      ).toBe(false);
    }
  });

  it('omits the working group when Camera is disabled', () => {
    const pages = buildPages(admin, [{ ...camera, status: 'ready' }], devices, [
      { ...packageRecord, enabled: false, status: 'disabled' },
    ]);
    expect(pages.some((page) => page.id === integrationPageId('camera'))).toBe(false);
    expect(pages.some((page) => page.id === 'plugins:camera')).toBe(true);
  });

  it('uses installed package state when the older integration inventory disagrees', () => {
    const pages = buildPages(admin, [camera], devices, []);
    expect(pages.some((page) => page.id === integrationPageId('camera'))).toBe(false);
  });

  it.each([
    { presence: 'absent' },
    { presence: 'unknown' },
    { readiness: 'failed' },
    { readiness: 'initializing' },
    { readiness: 'unconfigured' },
    { enabled: false },
    { configured: false },
    { presence: undefined },
  ])('hides unusable cameras and empty groups: %j', (change) => {
    const pages = buildPages(admin, [camera], devices, [
      { ...packageRecord, devices: [{ ...packageRecord.devices[0], ...change }] },
    ]);
    expect(pages.filter((page) => page.cluster === 'instruments')).toEqual([]);
    expect(pages.some((page) => page.id === 'plugins:camera')).toBe(true);
  });

  it('requires a ready plugin even when its saved devices say ready', () => {
    const pages = buildPages(admin, [camera], devices, [
      { ...packageRecord, status: 'unavailable' },
    ]);
    expect(pages.filter((page) => page.cluster === 'instruments')).toEqual([]);
  });

  it('filters legacy instruments by plugin and hardware availability', () => {
    const integration = { ...camera, id: 'sigrok' };
    const hardware = { ...devices[0], integrationId: 'sigrok' };
    const plugin = { ...packageRecord, id: 'sigrok' };
    const menu = (state: string, ready: string, enabled: boolean) =>
      buildPages(
        admin,
        [{ ...integration, status: state }],
        [{ ...hardware, status: ready }],
        [{ ...plugin, enabled }],
      ).filter((page) => page.cluster === 'instruments');
    expect(menu('ready', 'ready', true)).toHaveLength(1);
    expect(menu('ready', 'ready', false)).toEqual([]);
    expect(menu('unavailable', 'ready', true)).toEqual([]);
    expect(menu('ready', 'unavailable', true)).toEqual([]);
    expect(
      buildPages(admin, [integration], [], [plugin]).filter(
        (page) => page.cluster === 'instruments',
      ),
    ).toEqual([]);
  });

  it('shows every installed plugin under System > Plugins, including disabled ones', () => {
    const pages = buildPages(
      admin,
      [],
      [],
      [
        packageRecord,
        {
          ...packageRecord,
          id: 'sigrok',
          display_name: 'Sigrok Instruments',
          enabled: false,
          devices: [],
        },
        { ...packageRecord, id: 'available', installed: false, devices: [] },
      ],
    );
    expect(
      pages
        .filter((page) => page.parentId === 'plugins')
        .map((page) => ({
          id: page.id,
          title: page.title,
          pluginId: page.pluginId,
        })),
    ).toEqual([
      { id: 'plugins:camera', title: 'Camera', pluginId: 'camera' },
      {
        id: 'plugins:sigrok',
        title: 'Sigrok Instruments',
        pluginId: 'sigrok',
      },
    ]);
  });
});

describe('Modbus endpoint navigation', () => {
  const admin: BenchCoreSession = {
    apiVersion: 'nilo.http/1',
    username: 'admin',
    role: 'administrator',
    permissions: ['plugins.manage', 'bench.read'],
    credential: 'session',
    csrfToken: 'csrf',
  };
  const plugin = {
    id: 'modbus',
    display_name: 'Modbus',
    version: '0.12.0',
    installed: true,
    enabled: true,
    mode: 'ask' as const,
    status: 'ready',
    simulation: false,
    operations: ['connect'],
    devices: [],
    error: '',
    digest: '',
  };
  const endpoint = {
    schema_version: 1,
    id: 'tcp-a',
    revision: 1,
    name: 'Boiler controller',
    transport: 'tcp',
    resource_id: 'bus-a',
    enabled: true,
    presence: 'configured',
    readiness: 'offline',
    network: { host: '192.0.2.9', port: 502, unit_min: 1, unit_max: 4 },
  };

  it('keeps a configured offline target and its three named pages visible', () => {
    const pages = buildPages(admin, [], [], [plugin], [endpoint]);
    expect(pages.find((page) => page.id === integrationPageId('modbus'))).toMatchObject(
      {
        navigationOnly: true,
        cluster: 'instruments',
      },
    );
    expect(pages.find((page) => page.id === 'modbus:tcp-a')).toMatchObject({
      title: 'Boiler controller',
      modbusEndpointID: 'tcp-a',
      modbusView: 'live',
    });
    expect(pages.find((page) => page.id === 'modbus:tcp-a:sessions')).toMatchObject({
      modbusView: 'sessions',
    });
    expect(pages.find((page) => page.id === 'modbus:tcp-a:maps')).toMatchObject({
      modbusView: 'maps',
    });
  });

  it('retains definition access without an installed plugin and hides administration from readers', () => {
    const operator = {
      ...admin,
      role: 'operator' as const,
      permissions: ['bench.read'],
    };
    const pages = buildPages(
      operator,
      [],
      [],
      [{ ...plugin, installed: false, enabled: false }],
      [endpoint],
    );
    expect(pages.some((page) => page.id === 'modbus:tcp-a')).toBe(false);
    expect(pages.some((page) => page.id === 'modbus:definitions')).toBe(true);
    expect(pages.some((page) => page.id === 'plugins:modbus')).toBe(false);
    expect(
      buildPages(
        admin,
        [],
        [],
        [{ ...plugin, installed: false, enabled: false }],
        [endpoint],
      ).some((page) => page.id === 'plugins:modbus'),
    ).toBe(true);
  });
});
