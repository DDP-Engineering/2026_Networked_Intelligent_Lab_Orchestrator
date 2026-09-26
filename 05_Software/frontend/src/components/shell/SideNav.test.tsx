import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import { LanguageProvider } from '../../i18n/LanguageProvider';
import { SideNav } from './SideNav';
import { buildPages, type PageDescriptor } from './pages';

describe('nested plugin navigation', () => {
  const pages: PageDescriptor[] = [
    { id: 'bench', cluster: 'working', icon: 'bench', title: 'Bench', available: true },
    {
      id: 'integration:camera',
      cluster: 'working',
      icon: 'instrument',
      title: 'Camera',
      overviewTitleKey: 'nav.camera.overview',
      available: true,
    },
    {
      id: 'camera:cam-1',
      parentId: 'integration:camera',
      cluster: 'working',
      icon: 'instrument',
      title: 'Board view',
      overviewTitleKey: 'camera.snapshot',
      available: true,
    },
    {
      id: 'camera:cam-1:settings',
      parentId: 'camera:cam-1',
      cluster: 'working',
      icon: 'instrument',
      title: 'Settings',
      available: true,
    },
  ];

  it('opens a device view nested below its named physical camera', () => {
    const onOpen = vi.fn();
    render(
      <LanguageProvider initial="en">
        <SideNav
          pages={pages}
          active="camera:cam-1:settings"
          onOpen={onOpen}
          nameOf={(page) => page.title ?? page.id}
        />
      </LanguageProvider>,
    );
    expect(screen.getByRole('button', { name: 'Board view' })).toBeVisible();
    fireEvent.click(screen.getByRole('button', { name: 'Settings' }));
    expect(onOpen).toHaveBeenCalledWith('camera:cam-1:settings');
  });
});

it('opens installed plugins from the System submenu', () => {
  const onOpen = vi.fn();
  const pages = buildPages(
    null,
    [],
    [],
    [
      {
        id: 'sigrok',
        display_name: 'Sigrok Instruments',
        version: '0.1.0',
        installed: true,
        enabled: false,
        mode: 'ask',
        status: 'disabled',
        simulation: false,
        operations: [],
        devices: [],
        error: '',
        digest: '',
      },
    ],
  );
  render(
    <LanguageProvider initial="en">
      <SideNav
        pages={pages}
        active="plugins"
        onOpen={onOpen}
        nameOf={(page) => page.title ?? page.id}
      />
    </LanguageProvider>,
  );
  expect(screen.getByRole('button', { name: 'Collapse plugins' })).toBeVisible();
  fireEvent.click(screen.getByRole('button', { name: 'Sigrok Instruments' }));
  expect(onOpen).toHaveBeenCalledWith('plugins:sigrok');
});
