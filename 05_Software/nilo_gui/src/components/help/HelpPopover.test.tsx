import { fireEvent, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { renderInShell } from '../../test/render';
import { HelpPopover } from './HelpPopover';

/**
 * REQ-UX-2b0b6d59: settings help that works without hover.
 *
 * The popover is the one place a definition lives, so it has to open for a
 * mouse, a keyboard and a finger, be tied to its trigger for a screen reader,
 * and give focus back where it came from.
 */
function renderTwo() {
  return renderInShell(
    <>
      <HelpPopover name="Gateway">
        The next hop for traffic leaving an interface.
      </HelpPopover>
      <HelpPopover name="Prefix">The size of the subnet, for example /24.</HelpPopover>
    </>,
    { theme: 'light', language: 'en' },
  );
}

describe('HelpPopover', () => {
  it('opens on click with an accessible name, and Escape returns focus', () => {
    renderTwo();
    const trigger = screen.getByRole('button', { name: 'Help for Gateway' });
    expect(trigger).toHaveAttribute('aria-expanded', 'false');
    expect(screen.queryByText(/next hop/)).toBeNull();

    fireEvent.click(trigger);
    expect(trigger).toHaveAttribute('aria-expanded', 'true');
    const panel = screen.getByRole('dialog', { name: 'Help for Gateway' });
    expect(panel).toHaveTextContent(/next hop/);
    expect(trigger.getAttribute('aria-controls')).toBe(panel.id);

    fireEvent.keyDown(document, { key: 'Escape' });
    expect(screen.queryByRole('dialog')).toBeNull();
    expect(trigger).toHaveAttribute('aria-expanded', 'false');
    expect(document.activeElement).toBe(trigger);
  });

  it('opens from the keyboard and from a coarse pointer', () => {
    renderTwo();
    const trigger = screen.getByRole('button', { name: 'Help for Prefix' });

    trigger.focus();
    fireEvent.keyDown(trigger, { key: 'Enter' });
    fireEvent.click(trigger);
    expect(screen.getByRole('dialog', { name: 'Help for Prefix' })).toBeVisible();
    fireEvent.click(screen.getByRole('button', { name: 'Close help' }));
    expect(screen.queryByRole('dialog')).toBeNull();
    expect(document.activeElement).toBe(trigger);

    fireEvent.pointerDown(trigger, { pointerType: 'touch' });
    fireEvent.click(trigger);
    expect(screen.getByRole('dialog', { name: 'Help for Prefix' })).toBeVisible();
  });

  it('keeps only one settings help open at a time and closes on outside activation', () => {
    renderTwo();
    fireEvent.click(screen.getByRole('button', { name: 'Help for Gateway' }));
    fireEvent.click(screen.getByRole('button', { name: 'Help for Prefix' }));

    expect(screen.getAllByRole('dialog')).toHaveLength(1);
    expect(screen.getByRole('dialog')).toHaveAccessibleName('Help for Prefix');

    fireEvent.mouseDown(document.body);
    expect(screen.queryByRole('dialog')).toBeNull();
  });

  it('carries its content as text, not as a title attribute', () => {
    const { container } = renderTwo();
    expect(container.querySelector('[title]')).toBeNull();
    fireEvent.click(screen.getByRole('button', { name: 'Help for Gateway' }));
    expect(screen.getByText(/next hop/)).toBeVisible();
    expect(container.querySelector('[title]')).toBeNull();
  });
});
