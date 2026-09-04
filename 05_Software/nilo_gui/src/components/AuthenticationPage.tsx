import { useCallback, useEffect, useState, type ReactNode } from 'react';

import {
  BenchCoreError,
  type BenchCoreClient,
  type Credential,
  type NewIdentity,
  type NewToken,
} from '../api/client';
import type {
  BenchCoreIdentity,
  BenchCoreRole,
  BenchCoreSession,
  BenchCoreToken,
} from '../api/contract';
import {
  ChangeRoleForm,
  ConfirmationForm,
  CreateIdentityForm,
  CreateTokenForm,
  ResetPasswordForm,
  TokenSecretDisclosure,
} from './AdministrationForms';
import { useTranslate } from '../i18n/languages';

/** The message lookup, passed to the helpers that live outside the component. */
type Translate = ReturnType<typeof useTranslate>;
import { Modal } from './Modal';

/** One listing the page asked Bench Core for. */
type Listing<T> =
  | { phase: 'loading' }
  | { phase: 'ready'; items: T[] }
  | { phase: 'failed'; detail: string };

/** Which focused dialog is open, and what it is about. */
type Dialog =
  | { kind: 'create-identity' }
  | { kind: 'change-role'; identity: BenchCoreIdentity }
  | { kind: 'set-enabled'; identity: BenchCoreIdentity; enabled: boolean }
  | { kind: 'reset-password'; identity: BenchCoreIdentity }
  | { kind: 'create-token' }
  | { kind: 'revoke-token'; token: BenchCoreToken }
  | { kind: 'token-secret'; name: string; secret: string };

export interface AuthenticationPageProps {
  client: BenchCoreClient;
  credential: Credential;
  session: BenchCoreSession;
  /** Called when Bench Core reports this session no longer exists. */
  onSessionEnded: () => void;
}

/**
 * The administrator's Authentication page: identities on the left, API tokens
 * on the right, and every change in a focused dialog.
 *
 * Nothing here is an authorization decision. The navigation entry that opens
 * this page and the controls it offers are presentation; Bench Core authorizes
 * every request on its own, and a refusal is reported rather than pre-empted.
 */
export function AuthenticationPage({
  client,
  credential,
  session,
  onSessionEnded,
}: AuthenticationPageProps) {
  const t = useTranslate();
  const [identities, setIdentities] = useState<Listing<BenchCoreIdentity>>({
    phase: 'loading',
  });
  const [tokens, setTokens] = useState<Listing<BenchCoreToken>>({ phase: 'loading' });
  const [dialog, setDialog] = useState<Dialog | null>(null);
  const [working, setWorking] = useState(false);
  const [failure, setFailure] = useState('');
  const [notice, setNotice] = useState('');

  // A refusal that means the session is gone always ends the session, wherever
  // it surfaced: continuing to show administrative controls for a credential
  // the server no longer knows would be a lie.
  const classify = useCallback(
    (error: unknown): string => {
      if (error instanceof BenchCoreError && error.failure === 'unauthorized') {
        onSessionEnded();
        return t('auth.sessionEnded');
      }
      if (error instanceof BenchCoreError) {
        return error.detail;
      }
      return error instanceof Error ? error.message : String(error);
    },
    [onSessionEnded, t],
  );

  const reload = useCallback(
    async (abandoned: () => boolean = () => false) => {
      const [readIdentities, readTokens] = await Promise.allSettled([
        client.listIdentities(credential),
        client.listTokens(credential),
      ]);
      if (abandoned()) {
        return;
      }
      setIdentities(
        readIdentities.status === 'fulfilled'
          ? { phase: 'ready', items: readIdentities.value.items }
          : { phase: 'failed', detail: classify(readIdentities.reason) },
      );
      setTokens(
        readTokens.status === 'fulfilled'
          ? { phase: 'ready', items: readTokens.value.items }
          : { phase: 'failed', detail: classify(readTokens.reason) },
      );
    },
    [classify, client, credential],
  );

  // Both listings are read once when the page opens, and again after every
  // change, so what is on screen is what Bench Core holds rather than what this
  // page hoped it would.
  useEffect(() => {
    let abandoned = false;
    void (async () => {
      await reload(() => abandoned);
    })();
    return () => {
      abandoned = true;
    };
  }, [reload]);

  /**
   * Run one administrative change: mark it in progress, report what it did or
   * why it was refused, and re-read both listings so the screen shows what
   * Bench Core now holds rather than what this page hoped it would.
   */
  const apply = useCallback(
    async (change: () => Promise<Dialog | null>, succeeded: string) => {
      setWorking(true);
      setFailure('');
      try {
        const next = await change();
        setNotice(succeeded);
        setDialog(next);
        await reload();
      } catch (error) {
        setFailure(classify(error));
      } finally {
        setWorking(false);
      }
    },
    [classify, reload],
  );

  function close() {
    setDialog(null);
    setFailure('');
  }

  const knownIdentities = identities.phase === 'ready' ? identities.items : [];

  return (
    <div className="page">
      <header className="page-head">
        <h2>{t('auth.title')}</h2>
        <p className="page-sub">
          {t('auth.signedInAs', { username: session.username, role: session.role })}{' '}
          {t('auth.identitiesNote')}
        </p>
      </header>

      <div className="page-body">
        {credential.kind === 'session' && credential.csrfToken === '' ? (
          <p className="field-error" role="alert">
            {t('auth.csrfMissing')}
          </p>
        ) : null}
        {notice === '' ? null : (
          <p className="field-notice" role="status">
            {notice}
          </p>
        )}

        <div className="admin-grid">
          <AdminPanel
            id="identities"
            title={t('auth.users')}
            action={
              <button
                type="button"
                className="button button-primary"
                onClick={() => setDialog({ kind: 'create-identity' })}
              >
                {t('auth.newUser')}
              </button>
            }
            listing={identities}
            empty={t('auth.noIdentities')}
            resource={t('auth.resource.identities')}
          >
            <table className="data-table record-table">
              <thead>
                <tr>
                  <th scope="col">{t('auth.column.username')}</th>
                  <th scope="col">{t('auth.column.role')}</th>
                  <th scope="col">{t('auth.column.state')}</th>
                  <th scope="col">{t('auth.column.actions')}</th>
                </tr>
              </thead>
              <tbody>
                {knownIdentities.map((identity) => (
                  <tr key={identity.id}>
                    <th scope="row">{identity.username}</th>
                    <td className="cell-mono" data-label={t('auth.column.role')}>
                      {identity.role}
                    </td>
                    <td data-label={t('auth.column.state')}>
                      {identity.enabled ? t('auth.enabled') : t('auth.disabled')}
                    </td>
                    <td className="row-actions record-cell-actions">
                      <button
                        type="button"
                        className="button"
                        onClick={() => setDialog({ kind: 'change-role', identity })}
                      >
                        {t('auth.changeRole')}
                      </button>
                      <button
                        type="button"
                        className="button"
                        onClick={() =>
                          setDialog({
                            kind: 'set-enabled',
                            identity,
                            enabled: !identity.enabled,
                          })
                        }
                      >
                        {identity.enabled ? t('auth.disable') : t('auth.enable')}
                      </button>
                      <button
                        type="button"
                        className="button"
                        onClick={() => setDialog({ kind: 'reset-password', identity })}
                      >
                        {t('auth.resetPassword')}
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </AdminPanel>

          <AdminPanel
            id="tokens"
            title={t('auth.tokens')}
            action={
              <button
                type="button"
                className="button button-primary"
                disabled={knownIdentities.length === 0}
                onClick={() => setDialog({ kind: 'create-token' })}
              >
                {t('auth.newToken')}
              </button>
            }
            listing={tokens}
            empty={t('auth.noTokens')}
            resource={t('auth.resource.tokens')}
          >
            <table className="data-table record-table">
              <thead>
                <tr>
                  <th scope="col">{t('auth.column.name')}</th>
                  <th scope="col">{t('auth.column.owner')}</th>
                  <th scope="col">{t('auth.column.scopes')}</th>
                  <th scope="col">{t('auth.column.expires')}</th>
                  <th scope="col">{t('auth.column.state')}</th>
                  <th scope="col">{t('auth.column.actions')}</th>
                </tr>
              </thead>
              <tbody>
                {(tokens.phase === 'ready' ? tokens.items : []).map((token) => (
                  <tr key={token.id}>
                    <th scope="row">{token.name}</th>
                    <td className="cell-mono" data-label={t('auth.column.owner')}>
                      {token.owner}
                    </td>
                    <td data-label={t('auth.column.scopes')}>
                      {token.scopes.length === 0
                        ? t('auth.wholeRole')
                        : token.scopes.join(', ')}
                    </td>
                    <td className="cell-mono" data-label={t('auth.column.expires')}>
                      {token.expiresAt ?? t('auth.never')}
                    </td>
                    <td data-label={t('auth.column.state')}>
                      {token.revoked ? t('auth.revoked') : t('auth.active')}
                    </td>
                    <td className="row-actions record-cell-actions">
                      {token.revoked ? null : (
                        <button
                          type="button"
                          className="button"
                          onClick={() => setDialog({ kind: 'revoke-token', token })}
                        >
                          {t('auth.revoke')}
                        </button>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </AdminPanel>
        </div>
      </div>

      {dialog === null ? null : (
        <Modal title={dialogTitle(dialog, t)} onDismiss={close}>
          {renderDialog()}
        </Modal>
      )}
    </div>
  );

  function renderDialog(): ReactNode {
    if (dialog === null) {
      return null;
    }
    switch (dialog.kind) {
      case 'create-identity':
        return (
          <CreateIdentityForm
            working={working}
            failure={failure}
            onDismiss={close}
            onSubmit={(identity: NewIdentity) =>
              void apply(
                async () => {
                  await client.createIdentity(credential, identity);
                  return null;
                },
                t('auth.created', { username: identity.username, role: identity.role }),
              )
            }
          />
        );
      case 'change-role':
        return (
          <ChangeRoleForm
            identity={dialog.identity}
            working={working}
            failure={failure}
            onDismiss={close}
            onSubmit={(role: BenchCoreRole) =>
              void apply(
                async () => {
                  await client.setIdentityRole(
                    credential,
                    dialog.identity.username,
                    role,
                  );
                  return null;
                },
                t('auth.roleChanged', { username: dialog.identity.username, role }),
              )
            }
          />
        );
      case 'set-enabled':
        return (
          <ConfirmationForm
            question={t(
              dialog.enabled ? 'auth.enableExplanation' : 'auth.disableExplanation',
              { username: dialog.identity.username },
            )}
            confirmLabel={dialog.enabled ? t('auth.enable') : t('auth.disable')}
            working={working}
            failure={failure}
            onDismiss={close}
            onSubmit={() =>
              void apply(
                async () => {
                  await client.setIdentityEnabled(
                    credential,
                    dialog.identity.username,
                    dialog.enabled,
                  );
                  return null;
                },
                t('auth.stateChanged', {
                  username: dialog.identity.username,
                  state: dialog.enabled ? t('auth.enabled') : t('auth.disabled'),
                }),
              )
            }
          />
        );
      case 'reset-password':
        return (
          <ResetPasswordForm
            identity={dialog.identity}
            working={working}
            failure={failure}
            onDismiss={close}
            onSubmit={(password: string) =>
              void apply(
                async () => {
                  await client.resetIdentityPassword(
                    credential,
                    dialog.identity.username,
                    password,
                  );
                  return null;
                },
                t('auth.passwordReset', { username: dialog.identity.username }),
              )
            }
          />
        );
      case 'create-token':
        return (
          <CreateTokenForm
            identities={knownIdentities}
            working={working}
            failure={failure}
            onDismiss={close}
            onSubmit={(request: NewToken) =>
              void apply(
                async () => {
                  const issued = await client.createToken(credential, request);
                  // The secret goes straight into the disclosure dialog and
                  // nowhere else: it is never stored, logged, or put in a URL.
                  return {
                    kind: 'token-secret',
                    name: issued.token.name,
                    secret: issued.secret,
                  };
                },
                t('auth.tokenCreated', { name: request.name, owner: request.owner }),
              )
            }
          />
        );
      case 'revoke-token':
        return (
          <ConfirmationForm
            question={t('auth.revokeExplanation', {
              name: dialog.token.name,
              owner: dialog.token.owner,
            })}
            confirmLabel={t('auth.revokeToken')}
            working={working}
            failure={failure}
            onDismiss={close}
            onSubmit={() =>
              void apply(
                async () => {
                  await client.revokeToken(credential, dialog.token.id);
                  return null;
                },
                t('auth.tokenRevoked', { name: dialog.token.name }),
              )
            }
          />
        );
      case 'token-secret':
        return (
          <TokenSecretDisclosure
            name={dialog.name}
            secret={dialog.secret}
            onDismiss={close}
          />
        );
    }
  }
}

interface AdminPanelProps<T> {
  id: string;
  title: string;
  action: ReactNode;
  listing: Listing<T>;
  empty: string;
  /** How the listing is named in a failure sentence. */
  resource: string;
  children: ReactNode;
}

/**
 * One bounded administration panel: a heading, its own scroll compartment, and
 * the loading, empty and failed states in place of the table.
 */
function AdminPanel<T>({
  id,
  title,
  action,
  listing,
  empty,
  resource,
  children,
}: AdminPanelProps<T>) {
  const t = useTranslate();

  return (
    <section className="admin-panel" aria-labelledby={`${id}-title`}>
      <header className="admin-head">
        <h3 id={`${id}-title`}>{title}</h3>
        {action}
      </header>
      {listing.phase === 'loading' ? (
        <p className="admin-message" role="status">
          {t('auth.reading', { resource })}
        </p>
      ) : listing.phase === 'failed' ? (
        <div className="admin-message">
          <p role="alert">{t('auth.readFailed', { resource })}</p>
          <p className="failure-detail">{listing.detail}</p>
        </div>
      ) : listing.items.length === 0 ? (
        <p className="admin-message">{empty}</p>
      ) : (
        <div className="admin-list">{children}</div>
      )}
    </section>
  );
}

function dialogTitle(dialog: Dialog, t: Translate): string {
  switch (dialog.kind) {
    case 'create-identity':
      return t('auth.dialog.newUser');
    case 'change-role':
      return t('auth.dialog.changeRole', { username: dialog.identity.username });
    case 'set-enabled':
      return t(dialog.enabled ? 'auth.dialog.enableUser' : 'auth.dialog.disableUser', {
        username: dialog.identity.username,
      });
    case 'reset-password':
      return t('auth.dialog.resetPassword', { username: dialog.identity.username });
    case 'create-token':
      return t('auth.dialog.newToken');
    case 'revoke-token':
      return t('auth.dialog.revokeToken', { name: dialog.token.name });
    case 'token-secret':
      return t('auth.dialog.secret', { name: dialog.name });
  }
}
