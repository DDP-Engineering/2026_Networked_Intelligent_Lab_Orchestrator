import { useCallback, useEffect, useId, useRef, useState, type ReactNode } from 'react';

import { Icon } from './Icon';

export interface MenuProps {
  /** What the trigger shows. */
  label: ReactNode;
  /** The trigger's accessible name, when the label alone is not descriptive. */
  ariaLabel?: string;
  /** Extra classes for the trigger, for the few controls that differ. */
  triggerClass?: string;
  /** Whether to draw the chevron that marks this as a menu. */
  chevron?: boolean;
  /** The panel's contents. */
  children: (close: () => void) => ReactNode;
}

/**
 * One top-row popover: appearance, language, account and the network summary.
 *
 * All four behave identically on purpose — Escape closes, a click outside
 * closes, focus returns to the trigger — so a person learns the behaviour once.
 * The panel is positioned by the trigger's own stacking context rather than by
 * measurement, which keeps it correct when the row reflows on a narrow screen.
 */
export function Menu({
  label,
  ariaLabel,
  triggerClass,
  chevron = true,
  children,
}: MenuProps) {
  const [open, setOpen] = useState(false);
  const container = useRef<HTMLDivElement>(null);
  const trigger = useRef<HTMLButtonElement>(null);
  const panelId = useId();

  const close = useCallback(() => setOpen(false), []);

  // Focus returns to the trigger after the panel closes, rather than inside the
  // callback that closes it: a keyboard user must not be left on a control that
  // has just been removed, and the ref is read here rather than during render.
  const wasOpen = useRef(false);
  useEffect(() => {
    if (wasOpen.current && !open) {
      trigger.current?.focus();
    }
    wasOpen.current = open;
  }, [open]);

  useEffect(() => {
    if (!open) {
      return;
    }
    function onKeyDown(event: KeyboardEvent) {
      if (event.key === 'Escape') {
        close();
      }
    }
    function onPointerDown(event: MouseEvent) {
      if (!container.current?.contains(event.target as Node)) {
        setOpen(false);
      }
    }
    document.addEventListener('keydown', onKeyDown);
    document.addEventListener('mousedown', onPointerDown);
    return () => {
      document.removeEventListener('keydown', onKeyDown);
      document.removeEventListener('mousedown', onPointerDown);
    };
  }, [open, close]);

  return (
    <div className="menu" ref={container}>
      <button
        type="button"
        ref={trigger}
        className={`menu-trigger ${triggerClass ?? ''}`}
        aria-expanded={open}
        aria-haspopup="menu"
        aria-controls={open ? panelId : undefined}
        aria-label={ariaLabel}
        onClick={() => setOpen((current) => !current)}
      >
        {label}
        {chevron ? <Icon name="chevron" size={14} /> : null}
      </button>
      {open ? (
        <div className="menu-panel" id={panelId} role="menu">
          {children(close)}
        </div>
      ) : null}
    </div>
  );
}

export interface MenuItemProps {
  onSelect: () => void;
  /** Marks the entry that is currently in effect. */
  selected?: boolean;
  icon?: ReactNode;
  children: ReactNode;
}

/** One selectable row in a menu panel. */
export function MenuItem({
  onSelect,
  selected = false,
  icon,
  children,
}: MenuItemProps) {
  return (
    <button
      type="button"
      role="menuitemradio"
      aria-checked={selected}
      className="menu-item"
      onClick={onSelect}
    >
      <span className="menu-item-icon">{icon}</span>
      <span className="menu-item-label">{children}</span>
      {selected ? <Icon name="check" size={15} /> : null}
    </button>
  );
}
