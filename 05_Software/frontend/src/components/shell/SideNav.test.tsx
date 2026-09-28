import { fireEvent, render, screen } from '@testing-library/react';
import { useState } from 'react';
import { describe, expect, it, vi } from 'vitest';

import { LanguageProvider } from '../../i18n/LanguageProvider';
import { SideNav } from './SideNav';
import { buildPages, type PageDescriptor } from './pages';

function TestNav({
  pages,
  active,
  onOpen,
  unavailable = false,
}: {
  pages: PageDescriptor[];
  active: string;
  onOpen: (id: string) => void;
  unavailable?: boolean;
}) {
  const [expanded, setExpanded] = useState<Record<string, boolean>>({});
  return (
    <SideNav
      pages={pages}
      active={active}
      onOpen={onOpen}
      nameOf={(page) => page.title ?? page.id}
      expanded={expanded}
      onToggle={(id) => setExpanded((current) => ({ ...current, [id]: !current[id] }))}
      instrumentsUnavailable={unavailable}
    />
  );
}

const cameraPages: PageDescriptor[] = [
  {
    id: 'integration:camera',
    cluster: 'instruments',
    icon: 'instrument',
    title: 'Camera',
    navigationOnly: true,
    available: true,
  },
  {
    id: 'camera:cam-1',
    parentId: 'integration:camera',
    cluster: 'instruments',
    icon: 'instrument',
    title: 'Board view',
    overviewTitleKey: 'cw.realtime',
    available: true,
  },
  {
    id: 'camera:cam-1:recordings',
    parentId: 'camera:cam-1',
    cluster: 'instruments',
    icon: 'instrument',
    title: 'Recordings',
    available: true,
  },
];

describe('section and nested navigation', () => {
  it('keeps both sections closed with About active, and distinguishes empty from unavailable', () => {
    const { unmount } = render(
      <LanguageProvider initial="en">
        <TestNav pages={buildPages(null, [])} active="about" onOpen={vi.fn()} />
      </LanguageProvider>,
    );
    expect(screen.getByRole('button', { name: 'Instruments' })).toHaveAttribute(
      'aria-expanded',
      'false',
    );
    expect(screen.getByRole('button', { name: 'System' })).toHaveAttribute(
      'aria-expanded',
      'false',
    );
    fireEvent.click(screen.getByRole('button', { name: 'Instruments' }));
    expect(screen.getByText('No instruments are available.')).toBeVisible();
    unmount();
    render(
      <LanguageProvider initial="en">
        <TestNav
          pages={buildPages(null, [])}
          active="about"
          onOpen={vi.fn()}
          unavailable
        />
      </LanguageProvider>,
    );
    fireEvent.click(screen.getByRole('button', { name: 'Instruments' }));
    expect(screen.getByText('Instrument inventory is unavailable.')).toBeVisible();
  });

  it('opens only explicitly expanded camera and device groups and preserves the tab on collapse', () => {
    const onOpen = vi.fn();
    render(
      <LanguageProvider initial="en">
        <TestNav pages={cameraPages} active="camera:cam-1:recordings" onOpen={onOpen} />
      </LanguageProvider>,
    );
    expect(screen.queryByRole('button', { name: 'Camera' })).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Instruments' }));
    expect(
      screen.queryByRole('button', { name: 'Board view' }),
    ).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Camera' }));
    fireEvent.click(screen.getByRole('button', { name: 'Expand Board view' }));
    fireEvent.click(screen.getByRole('button', { name: 'Recordings' }));
    expect(onOpen).toHaveBeenCalledWith('camera:cam-1:recordings');
    fireEvent.click(screen.getByRole('button', { name: 'Instruments' }));
    expect(screen.getByRole('button', { name: 'Instruments' })).toHaveAttribute(
      'aria-expanded',
      'false',
    );
  });

  it('opens installed plugins from System', () => {
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
        <TestNav pages={pages} active="about" onOpen={onOpen} />
      </LanguageProvider>,
    );
    fireEvent.click(screen.getByRole('button', { name: 'System' }));
    fireEvent.click(screen.getByRole('button', { name: 'Expand plugins' }));
    fireEvent.click(screen.getByRole('button', { name: 'Sigrok Instruments' }));
    expect(onOpen).toHaveBeenCalledWith('plugins:sigrok');
  });
});
