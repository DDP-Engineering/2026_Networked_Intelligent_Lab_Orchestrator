import { useCallback, useEffect, useMemo, useState, type ReactNode } from 'react';

import {
  readPreference,
  resolvePreference,
  writePreference,
} from '../preferences/store';
import { en, type Catalogue } from './en';
import {
  DEFAULT_LANGUAGE,
  LANGUAGES,
  LANGUAGE_CODES,
  LanguageContext,
  type LanguageCode,
  type LanguageState,
  type MessageValues,
} from './languages';

/**
 * Substitute `{name}` placeholders.
 *
 * A placeholder with no value is left as written rather than replaced with
 * "undefined": a visible `{username}` is a bug report, and the word undefined in
 * a sentence is a mystery.
 */
function format(template: string, values?: MessageValues): string {
  if (values === undefined) {
    return template;
  }
  return template.replace(/\{(\w+)\}/g, (whole, name: string) =>
    name in values ? String(values[name]) : whole,
  );
}

export interface LanguageProviderProps {
  children: ReactNode;
  /** Overrides the persisted value; for tests that need a known language. */
  initial?: LanguageCode;
}

/** Owns the selected language and hands the catalogue to the tree below it. */
export function LanguageProvider({ children, initial }: LanguageProviderProps) {
  const [language, setLanguageState] = useState<LanguageCode>(
    () =>
      initial ??
      resolvePreference(readPreference('language'), LANGUAGE_CODES, DEFAULT_LANGUAGE),
  );

  // The document language is what a screen reader and the browser's own
  // spelling and hyphenation use, so it has to follow the selection.
  useEffect(() => {
    document.documentElement.setAttribute('lang', language);
  }, [language]);

  const setLanguage = useCallback((next: LanguageCode) => {
    setLanguageState(next);
    writePreference('language', next);
  }, []);

  const value = useMemo<LanguageState>(() => {
    const catalogue =
      LANGUAGES.find((entry) => entry.code === language)?.catalogue ??
      (en as unknown as Catalogue);
    return {
      language,
      setLanguage,
      t: (key, values) => format(catalogue[key], values),
    };
  }, [language, setLanguage]);

  return <LanguageContext.Provider value={value}>{children}</LanguageContext.Provider>;
}
