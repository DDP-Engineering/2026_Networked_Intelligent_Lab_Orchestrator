import { act, fireEvent, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { PREFERENCE_KEYS } from '../../preferences/store';
import { renderInShell } from '../../test/render';
import { NavigationSplitter } from './NavigationSplitter';
import { useNavigationWidth } from './useNavigationWidth';

function Layout() {
  const navigation = useNavigationWidth('wide');
  return (
    <NavigationSplitter
      {...navigation}
      onResize={navigation.setWidth}
      onReset={navigation.reset}
    />
  );
}

afterEach(() => {
  localStorage.clear();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe('navigation splitter', () => {
  it('resizes with the keyboard, persists across remounts and resets', () => {
    vi.stubGlobal('innerWidth', 1920);
    const first = renderInShell(<Layout />);
    const divider = screen.getByRole('separator', { name: 'Navigation width' });
    fireEvent.keyDown(divider, { key: 'ArrowRight', shiftKey: true });
    expect(divider).toHaveAttribute('aria-valuenow', '248');
    expect(localStorage.getItem(PREFERENCE_KEYS.navigationWidth)).toBe('248');
    first.unmount();
    renderInShell(<Layout />);
    const restored = screen.getByRole('separator');
    expect(restored).toHaveAttribute('aria-valuenow', '248');
    fireEvent.keyDown(restored, { key: 'Enter' });
    expect(restored).toHaveAttribute('aria-valuenow', '216');
    fireEvent.keyDown(restored, { key: 'ArrowLeft' });
    expect(restored).toHaveAttribute('aria-valuenow', '208');
    fireEvent.doubleClick(restored);
    expect(restored).toHaveAttribute('aria-valuenow', '216');
  });

  it('bounds the width and restores the preference after a window resize', () => {
    vi.stubGlobal('innerWidth', 1920);
    renderInShell(<Layout />);
    const divider = screen.getByRole('separator');
    fireEvent.keyDown(divider, { key: 'End' });
    fireEvent.keyDown(divider, { key: 'ArrowRight' });
    expect(divider).toHaveAttribute('aria-valuenow', '480');
    vi.stubGlobal('innerWidth', 1000);
    act(() => window.dispatchEvent(new Event('resize')));
    expect(divider).toHaveAttribute('aria-valuenow', '400');
    expect(localStorage.getItem(PREFERENCE_KEYS.navigationWidth)).toBe('480');
    vi.stubGlobal('innerWidth', 1920);
    act(() => window.dispatchEvent(new Event('resize')));
    expect(divider).toHaveAttribute('aria-valuenow', '480');
    fireEvent.keyDown(divider, { key: 'Home' });
    fireEvent.keyDown(divider, { key: 'ArrowLeft' });
    expect(divider).toHaveAttribute('aria-valuenow', '168');
  });

  it('ignores corrupt preferences and works when persistence is unavailable', () => {
    localStorage.setItem(PREFERENCE_KEYS.navigationWidth, 'NaN');
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('disabled');
    });
    renderInShell(<Layout />);
    const divider = screen.getByRole('separator');
    expect(divider).toHaveAttribute('aria-valuenow', '216');
    fireEvent.keyDown(divider, { key: 'ArrowRight' });
    expect(divider).toHaveAttribute('aria-valuenow', '224');
  });

  it('captures a drag and stops resizing after release or cancellation', () => {
    // jsdom has no PointerEvent or pointer capture; provide the browser event fields.
    vi.stubGlobal('PointerEvent', MouseEvent);
    renderInShell(<Layout />);
    const divider = screen.getByRole('separator');
    divider.setPointerCapture = vi.fn();
    divider.releasePointerCapture = vi.fn();
    function pointer(type: string, x: number) {
      const event = new MouseEvent(type, { bubbles: true, clientX: x, button: 0 });
      Object.defineProperties(event, {
        pointerId: { value: 1 },
        isPrimary: { value: true },
      });
      fireEvent(divider, event);
    }
    pointer('pointerdown', 216);
    pointer('pointermove', 320);
    expect(divider).toHaveAttribute('aria-valuenow', '320');
    expect(divider.setPointerCapture).toHaveBeenCalledWith(1);
    pointer('pointerup', 320);
    pointer('pointermove', 350);
    expect(divider).toHaveAttribute('aria-valuenow', '320');
    pointer('pointerdown', 320);
    pointer('pointercancel', 320);
    pointer('pointermove', 350);
    expect(divider).toHaveAttribute('aria-valuenow', '320');
  });
});
