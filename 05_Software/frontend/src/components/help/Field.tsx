import {
  cloneElement,
  isValidElement,
  useId,
  type ReactElement,
  type ReactNode,
} from 'react';

import { HelpPopover } from './HelpPopover';

export interface FieldProps {
  /** The control's id; the label points at it. */
  id: string;
  label: string;
  /** Short definition and examples, opened from a help control by the label. */
  help?: ReactNode;
  /**
   * One or two sentences the operator needs to choose a valid value. Always
   * visible: it is the part of the help that must survive a touch panel.
   */
  guidance?: ReactNode;
  /** The validation message for the current value, shown after the control. */
  error?: string;
  children: ReactElement<{ 'aria-describedby'?: string; 'aria-invalid'?: boolean }>;
}

/**
 * One labeled setting: label, optional help, the control, persistent guidance
 * and the validation message, in reading order. The guidance and the message
 * are tied to the control through `aria-describedby`, so a screen reader hears
 * them with the field rather than somewhere below it.
 */
export function Field({ id, label, help, guidance, error, children }: FieldProps) {
  const guidanceId = useId();
  const errorId = useId();
  const describedBy = [
    guidance === undefined ? null : guidanceId,
    error === undefined || error === '' ? null : errorId,
  ]
    .filter((entry): entry is string => entry !== null)
    .join(' ');
  const control = isValidElement(children)
    ? cloneElement(children, {
        'aria-describedby': describedBy === '' ? undefined : describedBy,
        'aria-invalid': error === undefined || error === '' ? undefined : true,
      })
    : children;

  return (
    <div className="field">
      <div className="field-label-row">
        <label htmlFor={id}>{label}</label>
        {help === undefined ? null : <HelpPopover name={label}>{help}</HelpPopover>}
      </div>
      {control}
      {guidance === undefined ? null : (
        <p className="field-guidance" id={guidanceId}>
          {guidance}
        </p>
      )}
      {error === undefined || error === '' ? null : (
        <p className="field-error" id={errorId} role="alert">
          {error}
        </p>
      )}
    </div>
  );
}
