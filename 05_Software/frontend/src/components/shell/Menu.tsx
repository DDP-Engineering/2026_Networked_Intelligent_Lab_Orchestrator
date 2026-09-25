import { useCallback, useEffect, useId, useRef, useState, type ReactNode } from 'react';

import { useLayer } from '../layers';
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
  /** Which edge of the trigger the panel lines up with. */
  align?: 'start' | 'end';
  /** The panel's contents. */
  children: (close: () => void) => ReactNode;
}

/**
 * One popover menu: the top-row controls and every row's Actions.
 *
 * All of them behave identically on purpose — Escape closes, a click outside
 * closes, focus returns to the trigger — so a person learns the behaviour once.
 * The panel is positioned by the trigger's own stacking context rather than by
 * measurement, which keeps it correct when the row reflows on a narrow screen.
 */
export function Menu({
  label,
  ariaLabel,
  triggerClass,
  chevron = true,
  align = 'end',
  children,
}: MenuProps) {
  const [open, setOpen] = useState(false);
  const container = useRef<HTMLDivElement>(null);
  const trigger = useRef<HTMLButtonElement>(null);
  const panelId = useId();
  const onTop = useLayer(open);

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
      if (event.key === 'Escape' && onTop()) {
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
  }, [open, close, onTop]);

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
        {chevron ? <Icon name="chevron" size={16} /> : null}
      </button>
      {open ? (
        <div
          className={`menu-panel ${align === 'start' ? 'menu-panel-start' : ''}`}
          id={panelId}
          role="menu"
        >
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

/** One selectable row in a menu panel: a choice among alternatives. */
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
      {selected ? <Icon name="check" size={16} /> : null}
    </button>
  );
}

export interface MenuActionProps {
  onSelect: () => void;
  disabled?: boolean;
  /** Why the action is disabled; shown under the label, never only on hover. */
  reason?: string;
  danger?: boolean;
  children: ReactNode;
}

/** One action in a menu panel: something that happens, not a choice. */
export function MenuAction({
  onSelect,
  disabled = false,
  reason,
  danger = false,
  children,
}: MenuActionProps) {
  return (
    <button
      type="button"
      role="menuitem"
      className={`menu-item ${danger ? 'menu-item-danger' : ''}`}
      disabled={disabled}
      onClick={onSelect}
    >
      <span className="menu-item-label">
        {children}
        {disabled && reason !== undefined && reason !== '' ? (
          <span className="menu-item-reason">{reason}</span>
        ) : null}
      </span>
    </button>
  );
}
