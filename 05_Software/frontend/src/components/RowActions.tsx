import { useState, type ReactNode } from 'react';

import { useTranslate } from '../i18n/languages';
import { Modal } from './Modal';
import { Icon } from './shell/Icon';

/** One action a row offers. A disabled action states why. */
export interface RowAction {
  /** The visible and accessible label, already carrying the item's name. */
  label: string;
  onSelect: () => void;
  disabled?: boolean;
  /** Why the action is disabled, shown beside it rather than hidden. */
  reason?: string;
  danger?: boolean;
}

export interface RowActionsProps {
  /** The item's name, for the trigger's accessible name. */
  name: string;
  /** Generic object kind shown as context at the top of the dialog. */
  typeLabel: string;
  /** Current state shown before any action is chosen. */
  status: string;
  actions: readonly RowAction[];
  /** Rendered before the dialog trigger; a Details toggle, typically. */
  children?: ReactNode;
}

/**
 * The compact Actions dialog a row or card carries.
 *
 * Several actions on one item used to be a cluster of small buttons, which a
 * finger cannot tell apart on a seven-inch panel. A dropdown inside a table is
 * also clipped by that table's scroll compartment. One labeled trigger now
 * opens a viewport-level dialog: every permitted action is a large button,
 * every disabled one says why, and destructive actions are separated.
 */
export function RowActions({
  name,
  typeLabel,
  status,
  actions,
  children,
}: RowActionsProps) {
  const t = useTranslate();
  const [open, setOpen] = useState(false);
  if (actions.length === 0) {
    return <div className="record-actions">{children}</div>;
  }
  const ordinary = actions.filter((action) => !action.danger);
  const destructive = actions.filter((action) => action.danger);

  const actionList = (entries: readonly RowAction[], danger: boolean) =>
    entries.map((action) => (
      <div className="action-dialog-entry" key={action.label}>
        <button
          type="button"
          className={`button action-dialog-action ${danger ? 'button-danger' : ''}`}
          disabled={action.disabled}
          onClick={() => {
            setOpen(false);
            action.onSelect();
          }}
        >
          {action.label}
        </button>
        {action.disabled && action.reason !== undefined && action.reason !== '' ? (
          <p className="action-dialog-reason">{action.reason}</p>
        ) : null}
      </div>
    ));

  return (
    <div className="record-actions">
      {children}
      <button
        type="button"
        className="button action-dialog-trigger"
        aria-label={t('actions.for', { name })}
        aria-haspopup="dialog"
        aria-expanded={open}
        onClick={() => setOpen(true)}
      >
        <span>{t('network.table.actions')}</span>
        <Icon name="chevron" size={16} />
      </button>
      {open ? (
        <Modal
          title={t('actions.for', { name })}
          onDismiss={() => setOpen(false)}
          variant="actions"
        >
          <div className="action-dialog">
            <p className="action-dialog-intro">{t('actions.choose')}</p>
            <dl className="action-dialog-context">
              <div>
                <dt>{t('actions.type')}</dt>
                <dd>{typeLabel}</dd>
              </div>
              <div>
                <dt>{t('actions.status')}</dt>
                <dd>{status}</dd>
              </div>
            </dl>
            <div className="action-dialog-list">{actionList(ordinary, false)}</div>
            {destructive.length > 0 ? (
              <div className="action-dialog-list action-dialog-danger-list">
                {actionList(destructive, true)}
              </div>
            ) : null}
            <div className="modal-actions">
              <button type="button" className="button" onClick={() => setOpen(false)}>
                {t('auth.cancel')}
              </button>
            </div>
          </div>
        </Modal>
      ) : null}
    </div>
  );
}

/** The Details toggle that opens a record's secondary values. */
export function DetailsToggle({
  name,
  expanded,
  controls,
  onToggle,
}: {
  name: string;
  expanded: boolean;
  controls: string;
  onToggle: () => void;
}) {
  const t = useTranslate();
  return (
    <button
      type="button"
      className="button network-detail-toggle"
      aria-label={`${t('network.details')} ${name}`}
      aria-expanded={expanded}
      aria-controls={controls}
      onClick={onToggle}
    >
      {t('network.details')}
      <Icon name={expanded ? 'chevronUp' : 'chevron'} size={16} />
    </button>
  );
}
