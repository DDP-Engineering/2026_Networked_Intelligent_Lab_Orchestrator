import type { MessageKey } from '../../i18n/en';

/**
 * The appliance's stable codes and the label each one is read as.
 *
 * Every map here is a lookup, never a rule: a code the appliance did not write
 * has no entry, and the reader shows the code itself rather than a label this
 * interface invented. The audit vocabulary is the appliance's, and a screen
 * that renamed one of its states would be describing a different history.
 */

export const CATEGORY_KEYS: Record<string, MessageKey> = {
  audit: 'activity.category.audit',
  event: 'activity.category.event',
};

export const COMPONENT_FILTER_OPTIONS: ReadonlyArray<{
  value: string;
  label: MessageKey;
}> = [
  { value: '', label: 'activity.component.all' },
  { value: 'network.*', label: 'activity.component.networkAll' },
  { value: 'network.interfaces', label: 'activity.component.networkInterfaces' },
  {
    value: 'network.device_networks',
    label: 'activity.component.networkDeviceNetworks',
  },
  { value: 'network.wireguard', label: 'activity.component.networkWireGuard' },
  { value: 'identity', label: 'activity.component.identity' },
  { value: 'system', label: 'activity.component.system' },
];

export const ACTOR_KEYS: Record<string, MessageKey> = {
  system: 'activity.actor.system',
  anonymous: 'activity.actor.anonymous',
  token: 'activity.actor.token',
  user: 'activity.actor.user',
};

export const SEVERITY_KEYS: Record<string, MessageKey> = {
  info: 'activity.severity.info',
  warning: 'activity.severity.warning',
  error: 'activity.severity.error',
};

export const TIME_SOURCE_KEYS: Record<string, MessageKey> = {
  synchronized: 'activity.timeSource.synchronized',
  unsynchronized: 'activity.timeSource.unsynchronized',
};

export const OUTCOME_KEYS: Record<string, MessageKey> = {
  requested: 'activity.outcome.requested',
  pending: 'activity.outcome.pending',
  applied: 'activity.outcome.applied',
  success: 'activity.outcome.success',
  confirmed: 'activity.outcome.confirmed',
  recovered: 'activity.outcome.recovered',
  denied: 'activity.outcome.denied',
  refused: 'activity.outcome.refused',
  failed: 'activity.outcome.failed',
  error: 'activity.outcome.error',
};

/**
 * How much attention an outcome asks for.
 *
 * `pending` is deliberately its own tone rather than a weaker `ok`: a change
 * that was requested and never confirmed is the one an operator has to find.
 */
export type OutcomeTone = 'ok' | 'pending' | 'warn' | 'danger' | 'unknown';

export const OUTCOME_TONES: Record<string, OutcomeTone> = {
  applied: 'ok',
  success: 'ok',
  confirmed: 'ok',
  recovered: 'ok',
  requested: 'pending',
  pending: 'pending',
  denied: 'warn',
  refused: 'warn',
  failed: 'danger',
  error: 'danger',
};

/**
 * The mark drawn beside an outcome, so the tone is never carried by colour
 * alone. It is decorative: the label beside it always says the same thing in
 * words.
 */
export const OUTCOME_MARKS: Record<OutcomeTone, string> = {
  ok: '✓',
  pending: '…',
  warn: '⊘',
  danger: '✕',
  unknown: '?',
};

/** The tone of an outcome the appliance wrote, unknown when it is not ours. */
export function outcomeTone(outcome: string): OutcomeTone {
  return OUTCOME_TONES[outcome] ?? 'unknown';
}
