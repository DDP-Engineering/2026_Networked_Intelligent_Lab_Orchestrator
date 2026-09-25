import { useEffect, useState } from 'react';

/** The two palettes a device preference can resolve to. */
export type ColorScheme = 'light' | 'dark';

const DARK_QUERY = '(prefers-color-scheme: dark)';

/** Read the device preference once, tolerating an environment without matchMedia. */
export function currentColorScheme(): ColorScheme {
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') {
    return 'light';
  }
  return window.matchMedia(DARK_QUERY).matches ? 'dark' : 'light';
}

/**
 * Track the device's colour-scheme preference.
 *
 * The System theme is applied by CSS, not by this hook: the token stylesheet
 * carries a `prefers-color-scheme` branch, so the palette follows the device
 * whether or not any script is listening. This exists for the code that has to
 * *know* the resolved value — the appearance menu saying which way System
 * currently resolves — and it is the seam the reactivity test drives.
 */
export function useSystemColorScheme(): ColorScheme {
  const [scheme, setScheme] = useState<ColorScheme>(currentColorScheme);

  useEffect(() => {
    if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') {
      return;
    }
    const query = window.matchMedia(DARK_QUERY);
    const update = (event: MediaQueryListEvent) =>
      setScheme(event.matches ? 'dark' : 'light');

    // The initial value was read during render; this only subscribes to later
    // changes, which is the one thing an effect is the right place for.
    query.addEventListener('change', update);
    return () => query.removeEventListener('change', update);
  }, []);

  return scheme;
}
