import { fireEvent, screen, within } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import { renderInShell } from '../test/render';
import { RowActions } from './RowActions';

describe('RowActions', () => {
  it('opens a compact dialog outside the scrolling table and keeps every action clear', () => {
    const activate = vi.fn();
    const remove = vi.fn();
    renderInShell(
      <div className="table-scroll">
        <table>
          <tbody>
            <tr>
              <td>
                <RowActions
                  name="DDP_Remote"
                  typeLabel="WireGuard profile"
                  status="Not active"
                  actions={[
                    { label: 'Activate DDP_Remote', onSelect: activate },
                    {
                      label: 'Remove DDP_Remote',
                      onSelect: remove,
                      disabled: true,
                      reason: 'Disable the profile before removing it.',
                      danger: true,
                    },
                  ]}
                />
              </td>
            </tr>
          </tbody>
        </table>
      </div>,
      { language: 'en', theme: 'light' },
    );

    const trigger = screen.getByRole('button', {
      name: 'Actions for DDP_Remote',
    });
    trigger.focus();
    fireEvent.click(trigger);

    const dialog = screen.getByRole('dialog', { name: 'Actions for DDP_Remote' });
    expect(dialog.closest('.table-scroll')).toBeNull();
    expect(within(dialog).getByText('WireGuard profile')).toBeVisible();
    expect(within(dialog).getByText('Not active')).toBeVisible();
    expect(
      within(dialog).getByRole('button', { name: 'Activate DDP_Remote' }),
    ).toBeVisible();
    const unavailable = within(dialog).getByRole('button', {
      name: /Remove DDP_Remote/,
    });
    expect(unavailable).toBeDisabled();
    expect(
      within(dialog).getByText('Disable the profile before removing it.'),
    ).toBeVisible();
    expect(within(dialog).getByRole('button', { name: 'Cancel' })).toBeVisible();
    expect(screen.queryByRole('menu')).toBeNull();

    fireEvent.click(
      within(dialog).getByRole('button', { name: 'Activate DDP_Remote' }),
    );
    expect(activate).toHaveBeenCalledTimes(1);
    expect(remove).not.toHaveBeenCalled();
    expect(screen.queryByRole('dialog')).toBeNull();
    expect(document.activeElement).toBe(trigger);
  });
});
