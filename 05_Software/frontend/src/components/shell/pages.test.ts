import { describe, expect, it } from 'vitest';

import { buildPages, integrationPageId } from './pages';

describe('physical Camera navigation', () => {
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

  it('groups configured physical cameras under Bench when Camera is enabled', () => {
    const pages = buildPages(null, [camera], devices, [packageRecord]);
    expect(pages.find((page) => page.id === integrationPageId('camera'))).toMatchObject(
      { cluster: 'working' },
    );
    expect(pages.find((page) => page.id === 'plugins:camera')).toMatchObject({
      cluster: 'core',
      parentId: 'plugins',
      title: 'Camera',
      pluginId: 'camera',
    });
    expect(pages.find((page) => page.id === 'camera:cam-1')).toMatchObject({
      cluster: 'working',
      parentId: integrationPageId('camera'),
      title: 'Board view',
      cameraDeviceID: 'cam-1',
    });
    expect(pages.find((page) => page.id === 'camera:cam-1:settings')).toMatchObject({
      parentId: 'camera:cam-1',
      cameraDeviceID: 'cam-1',
      cameraView: 'settings',
    });
    expect(pages.some((page) => page.id === 'camera:cam-2')).toBe(false);
  });

  it('omits the working group when Camera is disabled', () => {
    const pages = buildPages(null, [{ ...camera, status: 'ready' }], devices, [
      { ...packageRecord, enabled: false, status: 'disabled' },
    ]);
    expect(pages.some((page) => page.id === integrationPageId('camera'))).toBe(false);
    expect(pages.some((page) => page.id === 'plugins:camera')).toBe(true);
  });

  it('uses installed package state when the older integration inventory disagrees', () => {
    const pages = buildPages(null, [camera], devices, []);
    expect(pages.some((page) => page.id === integrationPageId('camera'))).toBe(false);
  });

  it('shows every installed plugin under System > Plugins, including disabled ones', () => {
    const pages = buildPages(
      null,
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
