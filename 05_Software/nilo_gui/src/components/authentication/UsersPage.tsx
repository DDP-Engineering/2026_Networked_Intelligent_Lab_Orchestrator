import { Fragment, useCallback, useEffect, useState, type ReactNode } from 'react';
import {
  BenchCoreError,
  type BenchCoreClient,
  type Credential,
  type NewIdentity,
} from '../../api/client';
import type {
  BenchCoreIdentity,
  BenchCoreRole,
  BenchCoreSession,
} from '../../api/contract';
import { useTranslate } from '../../i18n/languages';
import {
  ChangeRoleForm,
  ConfirmationForm,
  CreateIdentityForm,
  PermanentDeletionForm,
  ResetPasswordForm,
} from '../AdministrationForms';
import { Modal } from '../Modal';
import { DetailsToggle, RowActions, type RowAction } from '../RowActions';
import { WorkflowHelp } from '../help/WorkflowHelp';
import { AdministrationPanel, type Listing } from './AdministrationPanel';

type Dialog =
  | { kind: 'create' }
  | { kind: 'role'; identity: BenchCoreIdentity }
  | { kind: 'enabled'; identity: BenchCoreIdentity; enabled: boolean }
  | { kind: 'password'; identity: BenchCoreIdentity }
  | { kind: 'delete'; identity: BenchCoreIdentity };

export interface UsersPageProps {
  client: BenchCoreClient;
  credential: Credential;
  session: BenchCoreSession;
  canManage: boolean;
  onSessionEnded: () => void;
  onOpenActivity?: () => void;
}

export function UsersPage({
  client,
  credential,
  session,
  canManage,
  onSessionEnded,
  onOpenActivity,
}: UsersPageProps) {
  const t = useTranslate();
  const [listing, setListing] = useState<Listing<BenchCoreIdentity>>({
    phase: 'loading',
  });
  const [dialog, setDialog] = useState<Dialog | null>(null);
  const [details, setDetails] = useState<Set<string>>(() => new Set());
  const [working, setWorking] = useState(false);
  const [failure, setFailure] = useState('');
  const [notice, setNotice] = useState('');
  const classify = useCallback(
    (error: unknown) => {
      if (error instanceof BenchCoreError && error.failure === 'unauthorized') {
        onSessionEnded();
        return t('auth.sessionEnded');
      }
      // One refusal is answered in the reader's language rather than with the
      // appliance's English sentence, because it names the next thing to do.
      if (error instanceof BenchCoreError && error.code === 'user_has_tokens') {
        return t('auth.ownedTokensReason');
      }
      return error instanceof BenchCoreError
        ? error.detail
        : error instanceof Error
          ? error.message
          : String(error);
    },
    [onSessionEnded, t],
  );
  const reload = useCallback(
    async (abandoned: () => boolean = () => false) => {
      try {
        const read = await client.listIdentities(credential);
        if (!abandoned()) setListing({ phase: 'ready', items: read.items });
      } catch (error) {
        if (!abandoned()) setListing({ phase: 'failed', detail: classify(error) });
      }
    },
    [classify, client, credential],
  );
  useEffect(() => {
    let abandoned = false;
    void client
      .listIdentities(credential)
      .then((read) => {
        if (!abandoned) setListing({ phase: 'ready', items: read.items });
      })
      .catch((error: unknown) => {
        if (!abandoned) setListing({ phase: 'failed', detail: classify(error) });
      });
    return () => {
      abandoned = true;
    };
  }, [classify, client, credential]);
  const apply = useCallback(
    // `refresh` is cleared by the one change that ends the session applying it:
    // re-reading the listing would then be a request the appliance is right to
    // refuse, reported as a failure of a change that in fact succeeded.
    async (change: () => Promise<void>, succeeded: string, refresh = true) => {
      setWorking(true);
      setFailure('');
      try {
        await change();
        setNotice(succeeded);
        setDialog(null);
        if (refresh) {
          await reload();
        }
      } catch (error) {
        setFailure(classify(error));
      } finally {
        setWorking(false);
      }
    },
    [classify, reload],
  );
  const close = () => {
    setDialog(null);
    setFailure('');
  };
  const identities = listing.phase === 'ready' ? listing.items : [];
  const enabledAdministrators = identities.filter(
    (identity) => identity.enabled && identity.role === 'administrator',
  ).length;
  const unavailable = t('auth.permission.usersManage');

  return (
    <div className="page authentication-page">
      <header className="page-head">
        <h2>{t('auth.usersTitle')}</h2>
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
        <AdministrationPanel
          id="identities"
          title={t('auth.users')}
          listing={listing}
          empty={t('auth.noIdentities')}
          resource={t('auth.resource.identities')}
          action={
            <>
              {onOpenActivity === undefined ? null : (
                <button type="button" className="button" onClick={onOpenActivity}>
                  {t('auth.viewActivity')}
                </button>
              )}
              <button
                type="button"
                className="button button-primary"
                disabled={!canManage}
                title={canManage ? undefined : unavailable}
                onClick={() => setDialog({ kind: 'create' })}
              >
                {t('auth.newUser')}
              </button>
            </>
          }
        >
          <table className="data-table record-table authentication-records">
            <thead>
              <tr>
                <th scope="col">{t('auth.column.username')}</th>
                <th scope="col">{t('auth.column.role')}</th>
                <th scope="col">{t('auth.column.state')}</th>
                <th scope="col">{t('auth.column.actions')}</th>
              </tr>
            </thead>
            <tbody>
              {identities.map((identity) => {
                const expanded = details.has(identity.id);
                const lastAdministrator =
                  identity.enabled &&
                  identity.role === 'administrator' &&
                  enabledAdministrators === 1;
                const actions: RowAction[] = [
                  {
                    label: t('auth.changeRole'),
                    disabled: !canManage,
                    reason: canManage ? undefined : unavailable,
                    onSelect: () => setDialog({ kind: 'role', identity }),
                  },
                  {
                    label: identity.enabled ? t('auth.disable') : t('auth.enable'),
                    disabled: !canManage || lastAdministrator,
                    reason: !canManage
                      ? unavailable
                      : lastAdministrator
                        ? t('auth.lastAdministratorReason')
                        : undefined,
                    danger: identity.enabled,
                    onSelect: () =>
                      setDialog({
                        kind: 'enabled',
                        identity,
                        enabled: !identity.enabled,
                      }),
                  },
                  {
                    label: t('auth.resetPassword'),
                    disabled: !canManage,
                    reason: canManage ? undefined : unavailable,
                    danger: true,
                    onSelect: () => setDialog({ kind: 'password', identity }),
                  },
                  {
                    // Bench Core owns both refusals; naming them here only
                    // spares the operator a request that cannot succeed.
                    label: t('auth.deleteUser'),
                    disabled: !canManage || lastAdministrator,
                    reason: !canManage
                      ? unavailable
                      : lastAdministrator
                        ? t('auth.lastAdministratorDeleteReason')
                        : undefined,
                    danger: true,
                    onSelect: () => setDialog({ kind: 'delete', identity }),
                  },
                ];
                return (
                  <Fragment key={identity.id}>
                    <tr>
                      <th scope="row">{identity.username}</th>
                      <td data-label={t('auth.column.role')}>{identity.role}</td>
                      <td data-label={t('auth.column.state')}>
                        {identity.enabled ? t('auth.enabled') : t('auth.disabled')}
                      </td>
                      <td className="record-cell-actions">
                        <RowActions
                          name={identity.username}
                          typeLabel={t('auth.userType')}
                          status={
                            identity.enabled ? t('auth.enabled') : t('auth.disabled')
                          }
                          actions={actions}
                        >
                          <DetailsToggle
                            name={identity.username}
                            expanded={expanded}
                            controls={`user-details-${identity.id}`}
                            onToggle={() =>
                              setDetails((current) => {
                                const next = new Set(current);
                                if (next.has(identity.id)) {
                                  next.delete(identity.id);
                                } else {
                                  next.add(identity.id);
                                }
                                return next;
                              })
                            }
                          />
                        </RowActions>
                      </td>
                    </tr>
                    {expanded ? (
                      <tr className="record-details" id={`user-details-${identity.id}`}>
                        <td colSpan={4}>
                          <dl className="record-facts">
                            <div>
                              <dt>{t('auth.column.created')}</dt>
                              <dd className="cell-mono">{identity.createdAt}</dd>
                            </div>
                          </dl>
                        </td>
                      </tr>
                    ) : null}
                  </Fragment>
                );
              })}
            </tbody>
          </table>
        </AdministrationPanel>
      </div>
      {dialog === null ? null : (
        <Modal title={dialogTitle(dialog, t)} onDismiss={close}>
          {renderDialog()}
        </Modal>
      )}
    </div>
  );

  function renderDialog(): ReactNode {
    if (dialog === null) return null;
    switch (dialog.kind) {
      case 'create':
        return (
          <CreateIdentityForm
            working={working}
            failure={failure}
            onDismiss={close}
            onSubmit={(identity: NewIdentity) =>
              void apply(
                async () => {
                  await client.createIdentity(credential, identity);
                },
                t('auth.created', { username: identity.username, role: identity.role }),
              )
            }
          />
        );
      case 'role':
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
                },
                t('auth.roleChanged', { username: dialog.identity.username, role }),
              )
            }
          />
        );
      case 'enabled':
        return (
          <ConfirmationForm
            workflowHelp={
              <WorkflowHelp prefix="help.auth" title={t('help.auth.title')} />
            }
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
                },
                t('auth.stateChanged', {
                  username: dialog.identity.username,
                  state: dialog.enabled ? t('auth.enabled') : t('auth.disabled'),
                }),
              )
            }
          />
        );
      case 'delete':
        return (
          <PermanentDeletionForm
            workflowHelp={
              <WorkflowHelp prefix="help.auth" title={t('help.auth.title')} />
            }
            name={dialog.identity.username}
            prefix="delete-user"
            explanation={t('auth.deleteUserExplanation', {
              username: dialog.identity.username,
              role: dialog.identity.role,
            })}
            confirmLabel={t('auth.deleteUser')}
            fieldLabel={t('auth.field.confirmUsername')}
            fieldHelp={t('auth.help.confirmUsername')}
            working={working}
            failure={failure}
            onDismiss={close}
            onSubmit={() => {
              const own = dialog.identity.username === session.username;
              void apply(
                async () => {
                  await client.deleteIdentity(credential, dialog.identity.username);
                  // Removing one's own identity ends the session that did it,
                  // so the page returns to sign-in rather than reading on with
                  // a credential the appliance no longer knows.
                  if (own) {
                    onSessionEnded();
                  }
                },
                t('auth.userDeleted', { username: dialog.identity.username }),
                !own,
              );
            }}
          />
        );
      case 'password':
        return (
          <ResetPasswordForm
            identity={dialog.identity}
            working={working}
            failure={failure}
            onDismiss={close}
            onSubmit={(password) =>
              void apply(
                async () => {
                  await client.resetIdentityPassword(
                    credential,
                    dialog.identity.username,
                    password,
                  );
                },
                t('auth.passwordReset', { username: dialog.identity.username }),
              )
            }
          />
        );
    }
  }
}

type Translate = ReturnType<typeof useTranslate>;
function dialogTitle(dialog: Dialog, t: Translate) {
  switch (dialog.kind) {
    case 'create':
      return t('auth.dialog.newUser');
    case 'role':
      return t('auth.dialog.changeRole', { username: dialog.identity.username });
    case 'enabled':
      return t(dialog.enabled ? 'auth.dialog.enableUser' : 'auth.dialog.disableUser', {
        username: dialog.identity.username,
      });
    case 'password':
      return t('auth.dialog.resetPassword', { username: dialog.identity.username });
    case 'delete':
      return t('auth.dialog.deleteUser', { username: dialog.identity.username });
  }
}
