import { createContext, useContext } from 'react';

import type { ColorScheme } from './useSystemColorScheme';

/** The three selectable themes. `system` defers to the device preference. */
export const THEMES = ['light', 'dark', 'system'] as const;

/** One of the three themes above. */
export type Theme = (typeof THEMES)[number];

/** The default before anybody has chosen: follow the device. */
export const DEFAULT_THEME: Theme = 'system';

export interface ThemeState {
  /** What the user selected, which may be `system`. */
  theme: Theme;
  /** Which palette that selection currently resolves to. */
  resolved: ColorScheme;
  setTheme: (theme: Theme) => void;
}

export const ThemeContext = createContext<ThemeState | null>(null);

/** Read the selected theme, the palette it resolves to, and the setter. */
export function useTheme(): ThemeState {
  const value = useContext(ThemeContext);
  if (value === null) {
    throw new Error('useTheme was called outside a ThemeProvider');
  }
  return value;
}
