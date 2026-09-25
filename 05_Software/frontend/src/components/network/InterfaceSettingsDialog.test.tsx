import { fireEvent, screen, within } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import type { NetworkInterface } from '../../api/contract';
import { renderInShell } from '../../test/render';
import { InterfaceSettingsDialog } from './InterfaceSettingsDialog';

const wired: NetworkInterface = {
  id: 'sim-enp2s0',
  name: 'enp2s0',
  kind: 'ethernet',
  state: 'connected',
  ipv4: [],
  ipv6: [],
  linkCapacityMbps: 1000,
  rxBytesPerSecond: 0,
  txBytesPerSecond: 0,
  controllable: true,
  notControllableReason: '',
  adapterId: '02:00:00:00:00:02',
  ssid: '',
  settings: null,
};

/**
 * REQ-UX-8859ce12 and REQ-UX-1860069d on the adapter dialog: three groups,
 * field help, the how-to help, and a validation message beside the field.
 */
describe('InterfaceSettingsDialog', () => {
  it('groups addressing, DNS and protection, with help beside the settings', () => {
    renderInShell(
      <InterfaceSettingsDialog
        target={wired}
        busy={false}
        previewSettings={() => Promise.resolve(null)}
        onDismiss={vi.fn()}
      />,
      { theme: 'light', language: 'en' },
    );
    const dialog = screen.getByRole('dialog', { name: 'Configure enp2s0' });
    const groups = within(dialog)
      .getAllByRole('group')
      .map((group) => group.getAttribute('aria-label'));
    expect(groups).toEqual(['Addressing', 'DNS', 'Protection']);

    expect(
      within(dialog).getByRole('button', { name: 'How to configure' }),
    ).toBeVisible();
    expect(
      within(dialog).getByRole('button', { name: 'Help for Address source' }),
    ).toBeVisible();
    expect(
      within(dialog).getByRole('button', {
        name: /Help for Protect as a management path/,
      }),
    ).toBeVisible();

    // The consequence of protecting the path is persistent, not only in help.
    expect(within(dialog).getByText(/last management path/i)).toBeVisible();
  });

  it('flags a static address without a prefix beside the field and previews nothing', () => {
    const previewSettings = vi.fn(() => Promise.resolve(null));
    renderInShell(
      <InterfaceSettingsDialog
        target={wired}
        busy={false}
        previewSettings={previewSettings}
        onDismiss={vi.fn()}
      />,
      { theme: 'light', language: 'en' },
    );
    fireEvent.change(screen.getByLabelText('Address source'), {
      target: { value: 'static' },
    });
    const address = screen.getByLabelText(/IPv4 address/, { selector: 'input' });
    fireEvent.change(address, { target: { value: '192.168.0.1' } });
    fireEvent.click(screen.getByRole('button', { name: 'Preview' }));
    expect(previewSettings).not.toHaveBeenCalled();
    expect(address).toHaveAccessibleDescription(/prefix length/i);
    expect(screen.getByRole('alert')).toHaveTextContent(/192\.168\.0\.1\/24/);
  });

  it('opens the how-to help with the six sections', () => {
    renderInShell(
      <InterfaceSettingsDialog
        target={wired}
        busy={false}
        previewSettings={() => Promise.resolve(null)}
        onDismiss={vi.fn()}
      />,
      { theme: 'light', language: 'en' },
    );
    fireEvent.click(screen.getByRole('button', { name: 'How to configure' }));
    const help = screen.getByRole('dialog', { name: /how to configure an adapter/i });
    expect(
      within(help).getByRole('heading', { name: 'Verify and recover' }),
    ).toBeVisible();
    expect(help).toHaveTextContent(/192\.168\.0\.1\/24/);
    expect(help).toHaveTextContent(/default route/i);
  });
});
