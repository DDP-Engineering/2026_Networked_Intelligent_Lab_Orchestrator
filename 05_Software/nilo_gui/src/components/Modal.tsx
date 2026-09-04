import { useEffect, useId, useRef, type ReactNode } from 'react';
import { createPortal } from 'react-dom';

import { useTranslate } from '../i18n/languages';
import { useLayer } from './layers';
import { Icon } from './shell/Icon';

/**
 * The dialog's focusable stops, in tab order.
 *
 * Disabled controls are excluded because they cannot take focus, and a dialog
 * whose only button is disabled while a request is outstanding would otherwise
 * look like it had somewhere to send the key.
 */
function focusableWithin(panel: HTMLElement): readonly HTMLElement[] {
  return Array.from(
    panel.querySelectorAll<HTMLElement>(
      'a[href], button, input, select, textarea, [tabindex]',
    ),
  ).filter(
    (element) =>
      !element.hasAttribute('disabled') && element.getAttribute('tabindex') !== '-1',
  );
}

export interface ModalProps {
  /** The dialog's accessible name, shown as its heading. */
  title: string;
  /** Called when the dialog is dismissed without applying anything. */
  onDismiss: () => void;
  /**
   * `dialog` is a centred panel that becomes a full-screen sheet on a short
   * or narrow viewport; `drawer` is anchored to the left edge and holds the
   * navigation on a constrained viewport; `actions` keeps a short choice list
   * compact on a 1024 by 600 field display.
   */
  variant?: 'dialog' | 'drawer' | 'actions';
  children: ReactNode;
}

/**
 * One focused dialog.
 *
 * Every administrative change happens in here rather than inline in a table
 * row, so an in-progress state, a failure and a confirmation each have one
 * place to appear instead of six.
 *
 * Three parts, so a dialog fits a 480-pixel viewport: a head with the title
 * and a close control that never scrolls away, a body that scrolls on its own,
 * and the `.modal-actions` bar a form ends with, which sticks to the bottom of
 * that body while it scrolls.
 *
 * It is a plain element rather than `<dialog>`: the modal semantics a screen
 * reader needs are `role="dialog"` and `aria-modal`, and building on the
 * element would add a browser API whose behavior differs between the browser
 * and the component test environment for no gain here.
 */
export function Modal({ title, onDismiss, variant = 'dialog', children }: ModalProps) {
  const t = useTranslate();
  const panel = useRef<HTMLDivElement>(null);
  const body = useRef<HTMLDivElement>(null);
  const titleId = useId();
  const onTop = useLayer(true);

  // Focus moves into the dialog when it opens — to the first control of its
  // body, or to the close control when the body has none — and returns to
  // whatever opened it when it closes, so a keyboard user is never left on a
  // control that has gone away.
  useEffect(() => {
    const opener = document.activeElement;
    const first = body.current === null ? undefined : focusableWithin(body.current)[0];
    if (first !== undefined) {
      first.focus();
    } else if (panel.current !== null) {
      focusableWithin(panel.current)[0]?.focus();
    }
    return () => {
      if (opener instanceof HTMLElement) {
        opener.focus();
      }
    };
  }, []);

  useEffect(() => {
    function dismissOnEscape(event: KeyboardEvent) {
      if (event.key === 'Escape' && onTop()) {
        onDismiss();
      }
    }
    document.addEventListener('keydown', dismissOnEscape);
    return () => document.removeEventListener('keydown', dismissOnEscape);
  }, [onDismiss, onTop]);

  // Tab is confined to the dialog in both directions. `aria-modal` tells a
  // screen reader the rest of the page is unavailable, but it moves no focus, so
  // without this a keyboard user tabs straight out onto the page behind and can
  // operate controls the dialog is supposed to be covering.
  useEffect(() => {
    function confineTab(event: KeyboardEvent) {
      if (event.key !== 'Tab' || panel.current === null) {
        return;
      }
      const stops = focusableWithin(panel.current);
      const first = stops[0];
      const last = stops[stops.length - 1];
      if (first === undefined || last === undefined) {
        // Nothing to land on: keep focus on the dialog rather than let it leave.
        event.preventDefault();
        return;
      }
      const on = document.activeElement;
      const leavingBackwards =
        event.shiftKey && (on === first || !panel.current.contains(on));
      const leavingForwards = !event.shiftKey && on === last;
      if (leavingBackwards) {
        event.preventDefault();
        last.focus();
      } else if (leavingForwards) {
        event.preventDefault();
        first.focus();
      }
    }
    document.addEventListener('keydown', confineTab);
    return () => document.removeEventListener('keydown', confineTab);
  }, []);

  const drawer = variant === 'drawer';
  const actions = variant === 'actions';
  return createPortal(
    <div
      className={`modal-backdrop ${drawer ? 'modal-backdrop-drawer' : ''} ${actions ? 'modal-backdrop-actions' : ''}`}
    >
      <div
        className={`modal ${drawer ? 'modal-drawer' : ''} ${actions ? 'modal-actions-dialog' : ''}`}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        ref={panel}
      >
        <div className="modal-head">
          <h3 className="modal-title" id={titleId}>
            {title}
          </h3>
          <button
            type="button"
            className="modal-close"
            aria-label={t('dialog.close')}
            onClick={onDismiss}
          >
            <Icon name="close" size={20} />
          </button>
        </div>
        <div className="modal-body" ref={body}>
          {children}
        </div>
      </div>
    </div>,
    document.body,
  );
}

export interface ModalActionsProps {
  /** Label of the control that applies the change. */
  confirmLabel: string;
  /** Whether the request is outstanding, which also blocks a second submission. */
  working: boolean;
  onDismiss: () => void;
  /** Set for a change that takes access away and cannot be undone by repeating it. */
  destructive?: boolean;
  /**
   * Set while the dialog has not yet been given what it needs to submit, such
   * as the typed confirmation a permanent deletion asks for. Cancel stays
   * available: only the change is held back.
   */
  blocked?: boolean;
}

/** The submit and cancel pair every dialog ends with. */
export function ModalActions({
  confirmLabel,
  working,
  onDismiss,
  destructive = false,
  blocked = false,
}: ModalActionsProps) {
  const t = useTranslate();

  return (
    <div className="modal-actions">
      <button
        type="submit"
        className={`button ${destructive ? 'button-danger' : 'button-primary'}`}
        disabled={working || blocked}
      >
        {working ? t('auth.working') : confirmLabel}
      </button>
      <button type="button" className="button" onClick={onDismiss} disabled={working}>
        {t('auth.cancel')}
      </button>
    </div>
  );
}
