import { useState, type FormEvent, type ReactNode } from 'react';

import {
  BENCH_CORE_TOKEN_SCOPES,
  BENCH_CORE_ROLES,
  type BenchCoreIdentity,
  type BenchCoreRole,
} from '../api/contract';
import type { NewIdentity, NewToken } from '../api/client';
import { useTranslate } from '../i18n/languages';
import { Field } from './help/Field';
import { HelpPopover } from './help/HelpPopover';
import { WorkflowHelp } from './help/WorkflowHelp';
import { ModalActions } from './Modal';

/** What every dialog body needs to report and to close itself. */
interface FormShellProps {
  working: boolean;
  /** The reason the last attempt was refused, or the empty string. */
  failure: string;
  onDismiss: () => void;
}

/** The failure line every dialog shows in the same place. */
function FormFailure({ failure }: { failure: string }) {
  return failure === '' ? null : (
    <p className="field-error" role="alert">
      {failure}
    </p>
  );
}

/**
 * Read a password twice.
 *
 * Both fields are masked, and a mismatch is caught here rather than sent: a
 * password nobody can read back is exactly the value a typing mistake in must
 * not be discovered later, when the person it belongs to cannot sign in.
 */
function usePasswordPair() {
  const t = useTranslate();
  const [password, setPassword] = useState('');
  const [confirmation, setConfirmation] = useState('');
  const [mismatch, setMismatch] = useState(false);

  function fields(prefix: string) {
    return (
      <>
        <Field
          id={`${prefix}-password`}
          label={t('auth.field.password')}
          help={t('auth.help.password')}
        >
          <input
            id={`${prefix}-password`}
            type="password"
            autoComplete="new-password"
            value={password}
            onChange={(event) => setPassword(event.target.value)}
          />
        </Field>
        <Field
          id={`${prefix}-confirmation`}
          label={t('auth.field.confirmPassword')}
          help={t('auth.help.confirmPassword')}
        >
          <input
            id={`${prefix}-confirmation`}
            type="password"
            autoComplete="new-password"
            value={confirmation}
            onChange={(event) => setConfirmation(event.target.value)}
          />
        </Field>
        {mismatch ? (
          <p className="field-error" role="alert">
            {t('auth.passwordMismatch')}
          </p>
        ) : null}
      </>
    );
  }

  function settled(): string | null {
    if (password === '' || password !== confirmation) {
      setMismatch(true);
      return null;
    }
    setMismatch(false);
    return password;
  }

  return { fields, settled };
}

export interface CreateIdentityFormProps extends FormShellProps {
  onSubmit: (identity: NewIdentity) => void;
}

/** Create an identity with one of the three fixed roles. */
export function CreateIdentityForm({
  working,
  failure,
  onDismiss,
  onSubmit,
}: CreateIdentityFormProps) {
  const t = useTranslate();
  const [username, setUsername] = useState('');
  const [role, setRole] = useState<BenchCoreRole>('viewer');
  const password = usePasswordPair();

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const settled = password.settled();
    if (settled === null) {
      return;
    }
    onSubmit({ username: username.trim(), role, password: settled });
  }

  return (
    <form className="dialog-form" onSubmit={submit} noValidate>
      <WorkflowHelp prefix="help.auth" title={t('help.auth.title')} />
      <Field
        id="new-identity-username"
        label={t('auth.field.username')}
        help={t('auth.help.username')}
      >
        <input
          id="new-identity-username"
          type="text"
          autoComplete="off"
          spellCheck={false}
          value={username}
          onChange={(event) => setUsername(event.target.value)}
        />
      </Field>
      <Field
        id="new-identity-role"
        label={t('auth.field.role')}
        help={t('auth.help.role')}
        guidance={t('auth.roleMatrix')}
      >
        <select
          id="new-identity-role"
          value={role}
          onChange={(event) => setRole(event.target.value as BenchCoreRole)}
        >
          {BENCH_CORE_ROLES.map((candidate) => (
            <option key={candidate} value={candidate}>
              {candidate}
            </option>
          ))}
        </select>
      </Field>
      {password.fields('new-identity')}
      <FormFailure failure={failure} />
      <ModalActions
        confirmLabel={t('auth.createIdentity')}
        working={working}
        onDismiss={onDismiss}
      />
    </form>
  );
}

export interface ChangeRoleFormProps extends FormShellProps {
  identity: BenchCoreIdentity;
  onSubmit: (role: BenchCoreRole) => void;
}

/** Move one identity between the three fixed roles. */
export function ChangeRoleForm({
  identity,
  working,
  failure,
  onDismiss,
  onSubmit,
}: ChangeRoleFormProps) {
  const t = useTranslate();
  const [role, setRole] = useState<BenchCoreRole>(identity.role);

  return (
    <form
      className="dialog-form"
      onSubmit={(event) => {
        event.preventDefault();
        onSubmit(role);
      }}
      noValidate
    >
      <WorkflowHelp prefix="help.auth" title={t('help.auth.title')} />
      <p className="card-copy">
        {t('auth.roleExplanation', {
          username: identity.username,
          role: identity.role,
        })}
      </p>
      <Field
        id="change-role-value"
        label={t('auth.field.role')}
        help={t('auth.help.role')}
        guidance={t('auth.roleMatrix')}
      >
        <select
          id="change-role-value"
          value={role}
          onChange={(event) => setRole(event.target.value as BenchCoreRole)}
        >
          {BENCH_CORE_ROLES.map((candidate) => (
            <option key={candidate} value={candidate}>
              {candidate}
            </option>
          ))}
        </select>
      </Field>
      <FormFailure failure={failure} />
      <ModalActions
        confirmLabel={t('auth.changeRole')}
        working={working}
        onDismiss={onDismiss}
      />
    </form>
  );
}

export interface ResetPasswordFormProps extends FormShellProps {
  identity: BenchCoreIdentity;
  onSubmit: (password: string) => void;
}

/** Replace one identity's password, ending every session it holds. */
export function ResetPasswordForm({
  identity,
  working,
  failure,
  onDismiss,
  onSubmit,
}: ResetPasswordFormProps) {
  const t = useTranslate();
  const password = usePasswordPair();

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const settled = password.settled();
    if (settled === null) {
      return;
    }
    onSubmit(settled);
  }

  return (
    <form className="dialog-form" onSubmit={submit} noValidate>
      <WorkflowHelp prefix="help.auth" title={t('help.auth.title')} />
      <p className="card-copy">
        {t('auth.resetExplanation', { username: identity.username })}
      </p>
      {password.fields('reset')}
      <FormFailure failure={failure} />
      <ModalActions
        confirmLabel={t('auth.resetPassword')}
        working={working}
        onDismiss={onDismiss}
        destructive
      />
    </form>
  );
}

export interface ConfirmationFormProps extends FormShellProps {
  /** The sentence naming exactly what will change, and to what. */
  question: ReactNode;
  confirmLabel: string;
  onSubmit: () => void;
  workflowHelp?: ReactNode;
}

/** Ask before a change that takes access away and cannot be repeated back. */
export function ConfirmationForm({
  question,
  confirmLabel,
  working,
  failure,
  onDismiss,
  onSubmit,
  workflowHelp,
}: ConfirmationFormProps) {
  return (
    <form
      className="dialog-form"
      onSubmit={(event) => {
        event.preventDefault();
        onSubmit();
      }}
      noValidate
    >
      {workflowHelp}
      <p className="card-copy">{question}</p>
      <FormFailure failure={failure} />
      <ModalActions
        confirmLabel={confirmLabel}
        working={working}
        onDismiss={onDismiss}
        destructive
      />
    </form>
  );
}

export interface PermanentDeletionFormProps extends FormShellProps {
  /** The exact name that has to be typed back, and that the warning names. */
  name: string;
  /** The sentence stating what will be removed and what else it takes. */
  explanation: ReactNode;
  /** The label of the control that applies the removal. */
  confirmLabel: string;
  /** The field label and help for the name that has to be repeated. */
  fieldLabel: string;
  fieldHelp: string;
  /** The id prefix, so two of these dialogs never share a control id. */
  prefix: string;
  onSubmit: () => void;
  workflowHelp?: ReactNode;
}

/**
 * Ask before a removal that nothing can undo.
 *
 * A confirmation button alone is what an operator clicks through; this dialog
 * therefore asks for the record's own name, typed exactly. Two things follow.
 * The dialog names the subject in a way a mis-aimed click cannot satisfy, and
 * the warning stays on screen while the name is being typed rather than being
 * dismissed by the act of confirming.
 *
 * Nothing here is a security boundary. Every rule this dialog states is
 * enforced by Bench Core inside the transaction that applies the removal; the
 * dialog exists so the person is never surprised by it.
 */
export function PermanentDeletionForm({
  name,
  explanation,
  confirmLabel,
  fieldLabel,
  fieldHelp,
  prefix,
  working,
  failure,
  onDismiss,
  onSubmit,
  workflowHelp,
}: PermanentDeletionFormProps) {
  const t = useTranslate();
  const [typed, setTyped] = useState('');
  // Compared exactly, with no trimming. The dialog asks for the name and means
  // it: accepting a value that merely reads the same leaves a confirmation
  // weaker than the one the operator was told they were giving.
  const matches = typed === name;

  return (
    <form
      className="dialog-form"
      onSubmit={(event) => {
        event.preventDefault();
        if (matches) {
          onSubmit();
        }
      }}
      noValidate
    >
      {workflowHelp}
      <p className="card-copy">{explanation}</p>
      <p className="field-error" role="alert">
        {t('auth.deleteWarning')}
      </p>
      <Field
        id={`${prefix}-confirmation`}
        label={fieldLabel}
        help={fieldHelp}
        guidance={t('auth.deleteTypeExactly', { name })}
      >
        <input
          id={`${prefix}-confirmation`}
          type="text"
          autoComplete="off"
          spellCheck={false}
          value={typed}
          onChange={(event) => setTyped(event.target.value)}
        />
      </Field>
      <FormFailure failure={failure} />
      <ModalActions
        confirmLabel={confirmLabel}
        working={working}
        onDismiss={onDismiss}
        destructive
        blocked={!matches}
      />
    </form>
  );
}

export interface CreateTokenFormProps extends FormShellProps {
  /** The identities a token may be created for. */
  identities: BenchCoreIdentity[];
  onSubmit: (token: NewToken) => void;
}

/** Create a named API token for an existing identity. */
export function CreateTokenForm({
  identities,
  working,
  failure,
  onDismiss,
  onSubmit,
}: CreateTokenFormProps) {
  const t = useTranslate();
  const [owner, setOwner] = useState(identities[0]?.username ?? '');
  const [name, setName] = useState('');
  const [scopes, setScopes] = useState<string[]>([]);
  const [expiry, setExpiry] = useState('');
  const [unreadableExpiry, setUnreadableExpiry] = useState(false);

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    let expiresAt: string | null = null;
    if (expiry !== '') {
      const parsed = new Date(expiry);
      if (Number.isNaN(parsed.getTime())) {
        setUnreadableExpiry(true);
        return;
      }
      expiresAt = parsed.toISOString();
    }
    setUnreadableExpiry(false);
    onSubmit({ owner, name: name.trim(), scopes, expiresAt });
  }

  const ownerRole = identities.find((identity) => identity.username === owner)?.role;

  return (
    <form className="dialog-form" onSubmit={submit} noValidate>
      <WorkflowHelp prefix="help.auth" title={t('help.auth.title')} />
      <Field
        id="new-token-owner"
        label={t('auth.field.owner')}
        help={t('auth.help.owner')}
      >
        <select
          id="new-token-owner"
          value={owner}
          onChange={(event) => setOwner(event.target.value)}
        >
          {identities.map((identity) => (
            <option key={identity.id} value={identity.username}>
              {identity.username} ({identity.role})
            </option>
          ))}
        </select>
      </Field>
      <Field
        id="new-token-name"
        label={t('auth.field.name')}
        help={t('auth.help.tokenName')}
      >
        <input
          id="new-token-name"
          type="text"
          autoComplete="off"
          value={name}
          onChange={(event) => setName(event.target.value)}
        />
      </Field>

      <fieldset className="scopes">
        <legend>
          {t('auth.field.scopes')}{' '}
          <HelpPopover name={t('auth.field.scopes')}>
            {t('auth.help.scopes')}
          </HelpPopover>
        </legend>
        <p className="card-copy">
          {t('auth.scopeExplanation')}
          {ownerRole === undefined ? '' : ` (${ownerRole})`}
        </p>
        <div className="scope-grid">
          {BENCH_CORE_TOKEN_SCOPES.map((permission) => (
            <label className="scope" key={permission}>
              <input
                type="checkbox"
                value={permission}
                checked={scopes.includes(permission)}
                onChange={(event) =>
                  setScopes((current) =>
                    event.target.checked
                      ? [...current, permission]
                      : current.filter((scope) => scope !== permission),
                  )
                }
              />
              <span>{permission}</span>
            </label>
          ))}
        </div>
      </fieldset>

      <Field
        id="new-token-expiry"
        label={t('auth.field.expires')}
        help={t('auth.help.expiry')}
      >
        <input
          id="new-token-expiry"
          type="datetime-local"
          value={expiry}
          onChange={(event) => setExpiry(event.target.value)}
        />
      </Field>
      {unreadableExpiry ? (
        <p className="field-error" role="alert">
          {t('auth.expiryUnreadable')}
        </p>
      ) : null}

      <FormFailure failure={failure} />
      <ModalActions
        confirmLabel={t('auth.createToken')}
        working={working}
        onDismiss={onDismiss}
      />
    </form>
  );
}

export interface TokenSecretDisclosureProps {
  name: string;
  secret: string;
  onDismiss: () => void;
}

/**
 * The one and only showing of a token secret.
 *
 * Bench Core keeps a verifier, never the secret, so this dialog is the only
 * moment it exists outside the caller. Dismissing it discards the value from
 * application memory, and nothing writes it anywhere it could be recovered.
 */
export function TokenSecretDisclosure({
  name,
  secret,
  onDismiss,
}: TokenSecretDisclosureProps) {
  const t = useTranslate();
  const [copyResult, setCopyResult] = useState('');

  async function copy() {
    try {
      await navigator.clipboard.writeText(secret);
      setCopyResult(t('auth.secretCopied'));
    } catch {
      setCopyResult(t('auth.secretCopyFailed'));
    }
  }

  return (
    <div className="dialog-form">
      <p className="card-copy" role="alert">
        {t('auth.secretExplanation', { name })}
      </p>
      <p className="token-secret" data-testid="token-secret">
        <code>{secret}</code>
      </p>
      <div className="modal-actions">
        <button
          type="button"
          className="button button-primary"
          onClick={() => void copy()}
        >
          {t('auth.copySecret')}
        </button>
        <button type="button" className="button" onClick={onDismiss}>
          {t('auth.done')}
        </button>
      </div>
      {copyResult === '' ? null : (
        <p className="field-notice" role="status">
          {copyResult}
        </p>
      )}
    </div>
  );
}
