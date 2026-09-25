import { render, type RenderResult } from '@testing-library/react';
import type { ReactElement } from 'react';

import { LanguageProvider } from '../i18n/LanguageProvider';
import { type LanguageCode } from '../i18n/languages';
import { ThemeProvider } from '../theme/ThemeProvider';
import { type Theme } from '../theme/themes';

export interface RenderOptions {
  /** Start from a known theme rather than whatever is persisted. */
  theme?: Theme;
  /** Start from a known language rather than whatever is persisted. */
  language?: LanguageCode;
}

/**
 * Render inside the providers the application always runs inside.
 *
 * Both providers are part of the component contract — every screen reads the
 * catalogue, and the theme is stamped on the document root — so a test that
 * rendered without them would be testing a tree that never ships.
 */
export function renderInShell(
  element: ReactElement,
  { theme, language }: RenderOptions = {},
): RenderResult {
  return render(
    <ThemeProvider initial={theme}>
      <LanguageProvider initial={language}>{element}</LanguageProvider>
    </ThemeProvider>,
  );
}

/**
 * Report the device colour-scheme preference, and let a test change it.
 *
 * jsdom has no `matchMedia`, so the System theme has nothing to follow unless
 * one is supplied. This installs a controllable one and returns the setter the
 * reactivity test drives.
 */
export function installColorScheme(
  initial: 'light' | 'dark',
): (next: 'light' | 'dark') => void {
  let current = initial;
  const listeners = new Set<(event: MediaQueryListEvent) => void>();

  window.matchMedia = ((query: string) => ({
    matches: query.includes('dark') && current === 'dark',
    media: query,
    onchange: null,
    addEventListener: (_: string, listener: (event: MediaQueryListEvent) => void) =>
      listeners.add(listener),
    removeEventListener: (_: string, listener: (event: MediaQueryListEvent) => void) =>
      listeners.delete(listener),
    addListener: () => {},
    removeListener: () => {},
    dispatchEvent: () => false,
  })) as unknown as typeof window.matchMedia;

  return (next) => {
    current = next;
    for (const listener of listeners) {
      listener({ matches: next === 'dark' } as MediaQueryListEvent);
    }
  };
}

/**
 * Report a viewport class to the shell, which reads it through `matchMedia`.
 *
 * jsdom has no layout, so the compact and constrained shells can only be
 * exercised by answering the two media queries the shell asks. Anything else
 * a component asks (the colour scheme, for instance) answers false.
 */
const originalMatchMedia = window.matchMedia;

export function installViewportClass(cls: 'wide' | 'compact' | 'constrained'): void {
  window.matchMedia = ((query: string) => ({
    matches:
      query.includes('56rem') || query.includes('520px')
        ? cls === 'constrained'
        : query.includes('72rem')
          ? cls !== 'wide'
          : false,
    media: query,
    onchange: null,
    addEventListener: () => {},
    removeEventListener: () => {},
    addListener: () => {},
    removeListener: () => {},
    dispatchEvent: () => false,
  })) as unknown as typeof window.matchMedia;
}

/** Put back whatever `matchMedia` the environment had before a test stubbed it. */
export function restoreMatchMedia(): void {
  window.matchMedia = originalMatchMedia;
}
