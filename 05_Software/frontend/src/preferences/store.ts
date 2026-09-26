/**
 * The only durable browser storage this interface uses.
 *
 * Only display preferences. Routing every write through here is what
 * makes the credential rule checkable rather than a habit: a password, a session
 * secret, a cross-site request forgery token and a created API token secret have
 * no path to `localStorage` because no other module touches it.
 *
 * Storage can throw — a browser in private mode, or one with site data disabled,
 * refuses it. A preference that cannot be saved is not worth failing a page
 * over, so a refused write is swallowed and the selection simply lasts for this
 * page's lifetime.
 */

/** The complete set of keys this interface will ever persist. */
export const PREFERENCE_KEYS = {
  theme: 'nilo.theme',
  language: 'nilo.language',
  navigationWidth: 'nilo.navigationWidth',
} as const;

/** One of the persisted display preferences. */
export type PreferenceName = keyof typeof PREFERENCE_KEYS;

/** Read a persisted preference, or null when it is absent or unreadable. */
export function readPreference(name: PreferenceName): string | null {
  try {
    return window.localStorage.getItem(PREFERENCE_KEYS[name]);
  } catch {
    return null;
  }
}

/** Persist one preference. A storage the browser refuses is not an error here. */
export function writePreference(name: PreferenceName, value: string): void {
  try {
    window.localStorage.setItem(PREFERENCE_KEYS[name], value);
  } catch {
    // The selection still applies to this page; only its persistence is lost.
  }
}

/**
 * Resolve a stored preference against the values that are actually offered.
 *
 * A stored value that is no longer supported — a language whose catalogue was
 * removed, a theme that was renamed — falls back rather than rendering an
 * interface in a state no code handles.
 */
export function resolvePreference<T extends string>(
  stored: string | null,
  allowed: readonly T[],
  fallback: T,
): T {
  const match = allowed.find((candidate) => candidate === stored);
  return match ?? fallback;
}
