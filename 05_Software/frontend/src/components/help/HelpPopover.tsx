import {
  useCallback,
  useEffect,
  useId,
  useLayoutEffect,
  useRef,
  useState,
  type CSSProperties,
  type ReactNode,
} from 'react';

import { useTranslate } from '../../i18n/languages';
import { useLayer } from '../layers';
import { Icon } from '../shell/Icon';

/** Fired on the document when one help opens, so every other help closes. */
const OPEN_EVENT = 'nilo-help-open';

export interface HelpPopoverProps {
  /** The setting the help is about; names the trigger and the popover. */
  name: string;
  /** Short text and simple examples. Never a control somebody has to find. */
  children: ReactNode;
}

/**
 * Settings help that works without hover.
 *
 * The trigger is a real button beside the label: a click, a tap or Enter opens
 * the popover and keeps it open; a mouse passing over it or keyboard focus
 * landing on it opens a transient one that closes again on the way out. Escape,
 * the close control and any activation outside close it and hand focus back to
 * the trigger. One help is open at a time, because two open popovers cover the
 * fields they explain.
 *
 * The panel is positioned from the trigger's measured rectangle, below it when
 * there is room and above it when there is not, and clamped to the viewport.
 * It is plain text with a heading and a close control; nothing in it is the
 * only way to reach an action.
 */
export function HelpPopover({ name, children }: HelpPopoverProps) {
  const t = useTranslate();
  const [open, setOpen] = useState(false);
  // Whether the operator asked for it (click, tap, Enter) rather than passed
  // over it. A pinned popover ignores the pointer leaving.
  const pinned = useRef(false);
  const trigger = useRef<HTMLButtonElement>(null);
  const panel = useRef<HTMLDivElement>(null);
  const container = useRef<HTMLSpanElement>(null);
  const panelId = useId();
  const [placement, setPlacement] = useState<CSSProperties>({});
  const label = t('help.for', { name });
  const onTop = useLayer(open);

  const close = useCallback((returnFocus: boolean) => {
    pinned.current = false;
    setOpen(false);
    if (returnFocus) trigger.current?.focus();
  }, []);

  const show = useCallback(() => {
    document.dispatchEvent(new CustomEvent(OPEN_EVENT, { detail: panelId }));
    setOpen(true);
  }, [panelId]);

  // Another help opening closes this one.
  useEffect(() => {
    function onOtherOpen(event: Event) {
      if ((event as CustomEvent<string>).detail !== panelId) {
        pinned.current = false;
        setOpen(false);
      }
    }
    document.addEventListener(OPEN_EVENT, onOtherOpen);
    return () => document.removeEventListener(OPEN_EVENT, onOtherOpen);
  }, [panelId]);

  useEffect(() => {
    if (!open) return;
    function onKeyDown(event: KeyboardEvent) {
      if (event.key === 'Escape' && onTop()) {
        close(true);
      }
    }
    function onPointerDown(event: MouseEvent) {
      if (!container.current?.contains(event.target as Node)) {
        close(false);
      }
    }
    function onScroll(event: Event) {
      // A fixed panel would detach from a trigger that scrolls away.
      if (panel.current?.contains(event.target as Node)) return;
      close(false);
    }
    document.addEventListener('keydown', onKeyDown);
    document.addEventListener('mousedown', onPointerDown);
    document.addEventListener('scroll', onScroll, true);
    window.addEventListener('resize', onScroll);
    return () => {
      document.removeEventListener('keydown', onKeyDown);
      document.removeEventListener('mousedown', onPointerDown);
      document.removeEventListener('scroll', onScroll, true);
      window.removeEventListener('resize', onScroll);
    };
  }, [open, close, onTop]);

  // Place the panel below the trigger, or above when the bottom of the viewport
  // is too close, and keep it inside the viewport horizontally.
  useLayoutEffect(() => {
    if (!open || trigger.current === null || panel.current === null) return;
    const margin = 8;
    const anchor = trigger.current.getBoundingClientRect();
    const size = panel.current.getBoundingClientRect();
    const viewportWidth = window.innerWidth;
    const viewportHeight = window.innerHeight;
    let left = anchor.left;
    if (left + size.width + margin > viewportWidth) {
      left = Math.max(margin, viewportWidth - size.width - margin);
    }
    let top = anchor.bottom + 4;
    if (top + size.height + margin > viewportHeight) {
      top = Math.max(margin, anchor.top - size.height - 4);
    }
    setPlacement({ top, left });
  }, [open]);

  return (
    <span className="help" ref={container}>
      <button
        type="button"
        ref={trigger}
        className="help-trigger"
        aria-label={label}
        aria-expanded={open}
        aria-controls={open ? panelId : undefined}
        onClick={() => {
          if (open && pinned.current) {
            close(false);
          } else {
            pinned.current = true;
            show();
          }
        }}
        onMouseEnter={() => {
          if (!open) show();
        }}
        onMouseLeave={() => {
          if (!pinned.current) close(false);
        }}
        onFocus={() => {
          if (!open) show();
        }}
        onBlur={(event) => {
          if (pinned.current) return;
          if (panel.current?.contains(event.relatedTarget as Node)) return;
          close(false);
        }}
      >
        <span className="help-glyph" aria-hidden="true">
          ?
        </span>
      </button>
      {open ? (
        <div
          className="help-panel"
          id={panelId}
          role="dialog"
          aria-label={label}
          ref={panel}
          style={placement}
          onMouseEnter={() => {
            pinned.current = true;
          }}
        >
          <div className="help-panel-head">
            <p className="help-panel-title">{name}</p>
            <button
              type="button"
              className="help-close"
              aria-label={t('help.close')}
              onClick={() => close(true)}
            >
              <Icon name="close" size={18} />
            </button>
          </div>
          {children}
        </div>
      ) : null}
    </span>
  );
}
