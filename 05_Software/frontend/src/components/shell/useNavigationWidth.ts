import { useState, useSyncExternalStore } from 'react';

import { readPreference, writePreference } from '../../preferences/store';
import type { ViewportClass } from './useViewportClass';

const MIN_WIDTH = 168;
const MAX_WIDTH = 480;

function subscribe(onChange: () => void) {
  window.addEventListener('resize', onChange);
  return () => window.removeEventListener('resize', onChange);
}

function readWidth(): number | null {
  const stored = readPreference('navigationWidth');
  if (stored === null || stored === '') return null;
  const width = Number(stored);
  return Number.isFinite(width) && width >= MIN_WIDTH && width <= MAX_WIDTH
    ? width
    : null;
}

/** Keep the chosen width while temporarily clamping it for a smaller window. */
export function useNavigationWidth(viewport: ViewportClass) {
  const [preferred, setPreferred] = useState(readWidth);
  const windowWidth = useSyncExternalStore(
    subscribe,
    () => window.innerWidth,
    () => 1024,
  );
  const defaultWidth = viewport === 'compact' ? 168 : 216;
  const max = Math.max(MIN_WIDTH, Math.min(MAX_WIDTH, Math.floor(windowWidth * 0.4)));
  const clamp = (value: number) =>
    Math.max(MIN_WIDTH, Math.min(max, Math.round(value)));
  const width = clamp(preferred ?? defaultWidth);

  function setWidth(value: number) {
    const next = clamp(value);
    setPreferred(next);
    writePreference('navigationWidth', String(next));
  }

  function reset() {
    setPreferred(null);
    writePreference('navigationWidth', '');
  }

  return { width, min: MIN_WIDTH, max, setWidth, reset };
}
