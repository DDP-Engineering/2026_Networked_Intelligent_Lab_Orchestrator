import { useEffect, useId, useRef, type ReactNode } from 'react';

import { useTranslate } from '../i18n/languages';

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
  children: ReactNode;
}

/**
 * One focused dialog.
 *
 * Every administrative change happens in here rather than inline in a table
 * row, so an in-progress state, a failure and a confirmation each have one
 * place to appear instead of six.
 *
 * It is a plain element rather than `<dialog>`: the modal semantics a screen
 * reader needs are `role="dialog"` and `aria-modal`, and building on the
 * element would add a browser API whose behavior differs between the browser
 * and the component test environment for no gain here.
 */
export function Modal({ title, onDismiss, children }: ModalProps) {
  const panel = useRef<HTMLDivElement>(null);
  const titleId = useId();

  // Focus moves into the dialog when it opens and returns to whatever opened
  // it when it closes, so a keyboard user is never left on a control that has
  // gone away.
  useEffect(() => {
    const opener = document.activeElement;
    if (panel.current !== null) {
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
      if (event.key === 'Escape') {
        onDismiss();
      }
    }
    document.addEventListener('keydown', dismissOnEscape);
    return () => document.removeEventListener('keydown', dismissOnEscape);
  }, [onDismiss]);

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

  return (
    <div
      className="modal-backdrop"
      // Only a press that both starts and ends on the backdrop dismisses, so a
      // drag that began inside the dialog does not discard a half-typed form.
      onMouseDown={(event) => {
        if (event.target === event.currentTarget) {
          onDismiss();
        }
      }}
    >
      <div
        className="modal"
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        ref={panel}
      >
        <h3 className="modal-title" id={titleId}>
          {title}
        </h3>
        {children}
      </div>
    </div>
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
}

/** The submit and cancel pair every dialog ends with. */
export function ModalActions({
  confirmLabel,
  working,
  onDismiss,
  destructive = false,
}: ModalActionsProps) {
  const t = useTranslate();

  return (
    <div className="modal-actions">
      <button
        type="submit"
        className={`button ${destructive ? 'button-danger' : 'button-primary'}`}
        disabled={working}
      >
        {working ? t('auth.working') : confirmLabel}
      </button>
      <button type="button" className="button" onClick={onDismiss} disabled={working}>
        {t('auth.cancel')}
      </button>
    </div>
  );
}
