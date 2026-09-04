import { useSyncExternalStore } from 'react';

/**
 * The three shell layouts, decided by the viewport.
 *
 * `wide` is a desktop or field laptop: the full rail. `compact` is the
 * seven-inch panel in its usual 1024x600: a narrower rail that keeps its
 * labels. `constrained` is 800x480 or anything as short as 520 pixels: the
 * rail gives way to a labeled drawer. The same two queries drive the CSS, so
 * script and stylesheet cannot disagree about which shell is on screen.
 */
export type ViewportClass = 'wide' | 'compact' | 'constrained';

export const COMPACT_QUERY = '(width <= 72rem)';
export const CONSTRAINED_QUERY = '(width <= 56rem), (height <= 520px)';

function classify(): ViewportClass {
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') {
    return 'wide';
  }
  if (window.matchMedia(CONSTRAINED_QUERY).matches) return 'constrained';
  if (window.matchMedia(COMPACT_QUERY).matches) return 'compact';
  return 'wide';
}

function subscribe(onChange: () => void): () => void {
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') {
    return () => undefined;
  }
  const queries = [COMPACT_QUERY, CONSTRAINED_QUERY].map((query) =>
    window.matchMedia(query),
  );
  for (const query of queries) query.addEventListener('change', onChange);
  return () => {
    for (const query of queries) query.removeEventListener('change', onChange);
  };
}

/** Which shell layout the viewport currently calls for. */
export function useViewportClass(): ViewportClass {
  return useSyncExternalStore(subscribe, classify, () => 'wide');
}
