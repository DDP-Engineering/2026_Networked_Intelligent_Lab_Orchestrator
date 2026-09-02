import { useState, type FormEvent, type ReactNode } from 'react';

import {
  BENCH_CORE_PERMISSIONS,
  BENCH_CORE_ROLES,
  type BenchCoreIdentity,
  type BenchCoreRole,
} from '../api/contract';
import type { NewIdentity, NewToken } from '../api/client';
import { useTranslate } from '../i18n/languages';
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

interface FieldProps {
  id: string;
  label: string;
  children: ReactNode;
}

function Field({ id, label, children }: FieldProps) {
  return (
    <div className="field">
      <label htmlFor={id}>{label}</label>
      {children}
    </div>
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
        <Field id={`${prefix}-password`} label={t('auth.field.password')}>
          <input
            id={`${prefix}-password`}
            type="password"
            autoComplete="new-password"
            value={password}
            onChange={(event) => setPassword(event.target.value)}
          />
        </Field>
        <Field id={`${prefix}-confirmation`} label={t('auth.field.confirmPassword')}>
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
      <Field id="new-identity-username" label={t('auth.field.username')}>
        <input
          id="new-identity-username"
          type="text"
          autoComplete="off"
          spellCheck={false}
          value={username}
          onChange={(event) => setUsername(event.target.value)}
        />
      </Field>
      <Field id="new-identity-role" label={t('auth.field.role')}>
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
      <p className="card-copy">
        {t('auth.roleExplanation', {
          username: identity.username,
          role: identity.role,
        })}
      </p>
      <Field id="change-role-value" label={t('auth.field.role')}>
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
}

/** Ask before a change that takes access away and cannot be repeated back. */
export function ConfirmationForm({
  question,
  confirmLabel,
  working,
  failure,
  onDismiss,
  onSubmit,
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
      <Field id="new-token-owner" label={t('auth.field.owner')}>
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
      <Field id="new-token-name" label={t('auth.field.name')}>
        <input
          id="new-token-name"
          type="text"
          autoComplete="off"
          value={name}
          onChange={(event) => setName(event.target.value)}
        />
      </Field>

      <fieldset className="scopes">
        <legend>{t('auth.field.scopes')}</legend>
        <p className="card-copy">
          {t('auth.scopeExplanation')}
          {ownerRole === undefined ? '' : ` (${ownerRole})`}
        </p>
        <div className="scope-grid">
          {BENCH_CORE_PERMISSIONS.map((permission) => (
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

      <Field id="new-token-expiry" label={t('auth.field.expires')}>
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
