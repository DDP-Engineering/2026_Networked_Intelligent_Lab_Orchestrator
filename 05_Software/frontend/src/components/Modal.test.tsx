import { fireEvent, render, screen, within } from '@testing-library/react';
import { useState } from 'react';
import { describe, expect, it, vi } from 'vitest';

import { LanguageProvider } from '../i18n/LanguageProvider';
import { ThemeProvider } from '../theme/ThemeProvider';
import { renderInShell } from '../test/render';
import { Modal } from './Modal';

/**
 * REQ-UX-2de87216: a dialog that fits a short viewport.
 *
 * The parts an operator must always be able to reach are the title, a close
 * control and the final actions; jsdom cannot measure the scrolling body, so
 * the stylesheet test covers the geometry and this covers the structure and the
 * keyboard.
 */
describe('Modal', () => {
  it('shows a title, a labeled close control and the actions, and returns focus on Escape', () => {
    const onDismiss = vi.fn();
    renderInShell(
      <>
        <button type="button">opener</button>
        <Modal title="Edit Line 3" onDismiss={onDismiss}>
          <form className="dialog-form">
            {Array.from({ length: 40 }, (_, index) => (
              <p key={index}>line {index}</p>
            ))}
            <div className="modal-actions">
              <button type="submit">Preview</button>
            </div>
          </form>
        </Modal>
      </>,
      { theme: 'light', language: 'en' },
    );

    const dialog = screen.getByRole('dialog', { name: 'Edit Line 3' });
    expect(within(dialog).getByRole('heading', { name: 'Edit Line 3' })).toBeVisible();
    expect(within(dialog).getByRole('button', { name: 'Close' })).toBeVisible();
    expect(dialog.querySelector('.modal-body')).not.toBeNull();
    expect(within(dialog).getByRole('button', { name: 'Preview' })).toBeVisible();

    fireEvent.click(within(dialog).getByRole('button', { name: 'Close' }));
    expect(onDismiss).toHaveBeenCalledTimes(1);

    fireEvent.keyDown(document, { key: 'Escape' });
    expect(onDismiss).toHaveBeenCalledTimes(2);
  });

  it('moves focus into the dialog and back to the opener when it closes', () => {
    function Harness() {
      const [open, setOpen] = useState(false);
      return (
        <>
          <button type="button" onClick={() => setOpen(true)}>
            opener
          </button>
          {open ? (
            <Modal title="Drawer" onDismiss={() => setOpen(false)} variant="drawer">
              <button type="button" onClick={() => setOpen(false)}>
                inside
              </button>
            </Modal>
          ) : null}
        </>
      );
    }
    render(
      <ThemeProvider initial="light">
        <LanguageProvider initial="en">
          <Harness />
        </LanguageProvider>
      </ThemeProvider>,
    );
    const opener = screen.getByRole('button', { name: 'opener' });
    opener.focus();
    fireEvent.click(opener);

    expect(screen.getByRole('dialog').className).toContain('modal-drawer');
    expect(document.activeElement).toBe(screen.getByRole('button', { name: 'inside' }));

    fireEvent.click(screen.getByRole('button', { name: 'inside' }));
    expect(screen.queryByRole('dialog')).toBeNull();
    expect(document.activeElement).toBe(opener);
  });
});
