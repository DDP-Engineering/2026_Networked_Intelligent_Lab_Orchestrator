import { createContext, useContext } from 'react';

import { de } from './de';
import { en, type Catalogue, type MessageKey } from './en';
import { ro } from './ro';

/**
 * The languages this interface has a complete catalogue for.
 *
 * The selector renders this list and nothing else, so a language can never be
 * offered before it is translated: adding one means adding a catalogue, which
 * the type system will not let you leave half finished.
 */
export const LANGUAGES = [
  { code: 'en', label: 'English', short: 'EN', catalogue: en as unknown as Catalogue },
  { code: 'de', label: 'Deutsch', short: 'DE', catalogue: de },
  { code: 'ro', label: 'Română', short: 'RO', catalogue: ro },
] as const;

/** One of the supported language codes. */
export type LanguageCode = (typeof LANGUAGES)[number]['code'];

/** Every supported code, for resolving a stored preference. */
export const LANGUAGE_CODES = LANGUAGES.map((language) => language.code);

/** The language used before anybody has chosen. */
export const DEFAULT_LANGUAGE: LanguageCode = 'en';

/** Values substituted into a message's `{placeholder}` slots. */
export type MessageValues = Record<string, string | number>;

export interface LanguageState {
  language: LanguageCode;
  setLanguage: (language: LanguageCode) => void;
  /** Look one message up, substituting any placeholders it declares. */
  t: (key: MessageKey, values?: MessageValues) => string;
}

export const LanguageContext = createContext<LanguageState | null>(null);

/** Read the selected language, the setter, and the message lookup. */
export function useLanguage(): LanguageState {
  const value = useContext(LanguageContext);
  if (value === null) {
    throw new Error('useLanguage was called outside a LanguageProvider');
  }
  return value;
}

/** The message lookup on its own, which is what most components need. */
export function useTranslate(): LanguageState['t'] {
  return useLanguage().t;
}
