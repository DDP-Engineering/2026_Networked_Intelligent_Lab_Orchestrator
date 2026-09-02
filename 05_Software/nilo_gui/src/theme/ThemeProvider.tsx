import { useCallback, useEffect, useMemo, useState, type ReactNode } from 'react';

import {
  readPreference,
  resolvePreference,
  writePreference,
} from '../preferences/store';
import { useSystemColorScheme } from './useSystemColorScheme';
import {
  DEFAULT_THEME,
  THEMES,
  ThemeContext,
  type Theme,
  type ThemeState,
} from './themes';

export interface ThemeProviderProps {
  children: ReactNode;
  /** Overrides the persisted value; for tests that need a known starting theme. */
  initial?: Theme;
}

/**
 * Owns the selected theme and stamps it on the document root.
 *
 * The stamp is the whole mechanism: `styles/tokens.css` defines the palette for
 * `[data-theme='dark']` and, under a colour-scheme query, for
 * `[data-theme='system']`. Nothing here computes a colour, and no component
 * branches on the theme, so light and dark cannot drift apart.
 */
export function ThemeProvider({ children, initial }: ThemeProviderProps) {
  const [theme, setThemeState] = useState<Theme>(
    () => initial ?? resolvePreference(readPreference('theme'), THEMES, DEFAULT_THEME),
  );
  const system = useSystemColorScheme();

  useEffect(() => {
    document.documentElement.setAttribute('data-theme', theme);
  }, [theme]);

  const setTheme = useCallback((next: Theme) => {
    setThemeState(next);
    writePreference('theme', next);
  }, []);

  const value = useMemo<ThemeState>(
    () => ({ theme, resolved: theme === 'system' ? system : theme, setTheme }),
    [theme, system, setTheme],
  );

  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>;
}
