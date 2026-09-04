/**
 * The restrained icon set.
 *
 * Every glyph is a 20x20 stroked outline drawn from `currentColor`, so an icon
 * inherits the token colour of whatever it sits in and needs no per-theme
 * variant. They are decorative: each one accompanies a written label, and none
 * is the only way to tell two entries apart.
 */
export type IconName =
  | 'bench'
  | 'instrument'
  | 'network'
  | 'plugins'
  | 'authentication'
  | 'settings'
  | 'about'
  | 'close'
  | 'chevron'
  | 'chevronUp'
  | 'check'
  | 'light'
  | 'dark'
  | 'system'
  | 'menu'
  | 'account';

const PATHS: Record<IconName, string> = {
  // A bench surface with a trace across it.
  bench: 'M3 15h18M5 15V9m14 6V9M3 6l4 4 3-4 3 5 4-6',
  // A probe over a device.
  instrument: 'M4 17h16M7 17V7l5-3 5 3v10M10 10h4',
  // Three nodes joined to one.
  network: 'M12 4v5m0 0L6 13m6-4l6 4M4 13h4v5H4zm12 0h4v5h-4zM10 4h4v5h-4z',
  // Two interlocking pieces.
  plugins:
    'M9 4h6v3a2 2 0 104 0h1v6h-3a2 2 0 100 4v3H9v-3a2 2 0 10-4 0H4V7h3a2 2 0 104 0z',
  // A shield with a tick.
  authentication: 'M12 3l7 3v6c0 4-3 7-7 9-4-2-7-5-7-9V6zM9 12l2.2 2.2L15 10.5',
  // A gear.
  settings:
    'M12 9a3 3 0 100 6 3 3 0 000-6zM4 12h2m12 0h2M12 4v2m0 12v2M6.5 6.5l1.5 1.5m8 8l1.5 1.5m0-11l-1.5 1.5m-8 8L6.5 17.5',
  // An information circle.
  about: 'M12 3a9 9 0 100 18 9 9 0 000-18zm0 5v1m0 3v5',
  close: 'M6 6l12 12M18 6L6 18',
  chevron: 'M7 10l5 5 5-5',
  chevronUp: 'M7 14l5-5 5 5',
  check: 'M5 12.5l4.5 4.5L19 7',
  light:
    'M12 8a4 4 0 100 8 4 4 0 000-8zM12 3v2m0 14v2M3 12h2m14 0h2M6 6l1.5 1.5M16.5 16.5L18 18M18 6l-1.5 1.5M7.5 16.5L6 18',
  dark: 'M20 14.5A8.5 8.5 0 019.5 4a8.5 8.5 0 1010.5 10.5z',
  system: 'M4 5h16v10H4zM9 19h6M12 15v4',
  // Three lines: the navigation drawer.
  menu: 'M4 7h16M4 12h16M4 17h16',
  // A person: the account menu.
  account: 'M12 12a4 4 0 100-8 4 4 0 000 8zm-7 8a7 7 0 0114 0',
};

export interface IconProps {
  name: IconName;
  /** Pixel size of the square box. Defaults to the 18px the shell uses. */
  size?: number;
}

/** One decorative glyph. It carries no accessible name of its own. */
export function Icon({ name, size = 18 }: IconProps) {
  return (
    <svg
      className="icon"
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.6"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      focusable="false"
    >
      <path d={PATHS[name]} />
    </svg>
  );
}
