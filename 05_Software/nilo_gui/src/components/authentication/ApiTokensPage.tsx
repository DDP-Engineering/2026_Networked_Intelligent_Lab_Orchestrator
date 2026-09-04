import { Fragment, useCallback, useEffect, useState, type ReactNode } from 'react';
import {
  BenchCoreError,
  type BenchCoreClient,
  type Credential,
  type NewToken,
} from '../../api/client';
import type {
  BenchCoreIdentity,
  BenchCoreSession,
  BenchCoreToken,
} from '../../api/contract';
import { useTranslate } from '../../i18n/languages';
import {
  ConfirmationForm,
  CreateTokenForm,
  PermanentDeletionForm,
  TokenSecretDisclosure,
} from '../AdministrationForms';
import { Modal } from '../Modal';
import { DetailsToggle, RowActions, type RowAction } from '../RowActions';
import { WorkflowHelp } from '../help/WorkflowHelp';
import { AdministrationPanel, type Listing } from './AdministrationPanel';

type Dialog =
  | { kind: 'create' }
  | { kind: 'revoke'; token: BenchCoreToken }
  | { kind: 'delete'; token: BenchCoreToken }
  | { kind: 'secret'; name: string; secret: string };
export interface ApiTokensPageProps {
  client: BenchCoreClient;
  credential: Credential;
  session: BenchCoreSession;
  canManage: boolean;
  onSessionEnded: () => void;
  onOpenActivity?: () => void;
}

export function ApiTokensPage({
  client,
  credential,
  session,
  canManage,
  onSessionEnded,
  onOpenActivity,
}: ApiTokensPageProps) {
  const t = useTranslate();
  const [listing, setListing] = useState<Listing<BenchCoreToken>>({ phase: 'loading' });
  const [identities, setIdentities] = useState<BenchCoreIdentity[]>([]);
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
      return error instanceof BenchCoreError
        ? error.detail
        : error instanceof Error
          ? error.message
          : String(error);
    },
    [onSessionEnded, t],
  );
  const reloadTokens = useCallback(
    async (abandoned: () => boolean = () => false) => {
      try {
        const read = await client.listTokens(credential);
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
      .listTokens(credential)
      .then((read) => {
        if (!abandoned) setListing({ phase: 'ready', items: read.items });
      })
      .catch((error: unknown) => {
        if (!abandoned) setListing({ phase: 'failed', detail: classify(error) });
      });
    void client
      .listIdentities(credential)
      .then((read) => {
        if (!abandoned) setIdentities(read.items);
      })
      .catch(() => {
        if (!abandoned) setIdentities([]);
      });
    return () => {
      abandoned = true;
    };
  }, [classify, client, credential]);
  const apply = useCallback(
    async (change: () => Promise<Dialog | null>, succeeded: string) => {
      setWorking(true);
      setFailure('');
      try {
        const next = await change();
        setNotice(succeeded);
        setDialog(next);
        await reloadTokens();
      } catch (error) {
        setFailure(classify(error));
      } finally {
        setWorking(false);
      }
    },
    [classify, reloadTokens],
  );
  const close = () => {
    setDialog(null);
    setFailure('');
  };
  const tokens = listing.phase === 'ready' ? listing.items : [];
  const unavailable = t('auth.permission.tokensManage');
  return (
    <div className="page authentication-page">
      <header className="page-head">
        <h2>{t('auth.tokensTitle')}</h2>
        <p className="page-sub">
          {t('auth.signedInAs', { username: session.username, role: session.role })}{' '}
          {t('auth.tokensNote')}
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
          id="tokens"
          title={t('auth.tokens')}
          listing={listing}
          empty={t('auth.noTokens')}
          resource={t('auth.resource.tokens')}
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
                disabled={!canManage || identities.length === 0}
                title={!canManage ? unavailable : undefined}
                onClick={() => setDialog({ kind: 'create' })}
              >
                {t('auth.newToken')}
              </button>
            </>
          }
        >
          <table className="data-table record-table authentication-records">
            <thead>
              <tr>
                <th scope="col">{t('auth.column.name')}</th>
                <th scope="col">{t('auth.column.owner')}</th>
                <th scope="col">{t('auth.column.state')}</th>
                <th scope="col">{t('auth.column.expires')}</th>
                <th scope="col">{t('auth.column.actions')}</th>
              </tr>
            </thead>
            <tbody>
              {tokens.map((token) => {
                const expanded = details.has(token.id);
                const state = tokenState(token);
                const actions: RowAction[] = [
                  {
                    label: t('auth.revoke'),
                    disabled: !canManage || state !== 'active',
                    reason: !canManage
                      ? unavailable
                      : state !== 'active'
                        ? t('auth.revocationUnavailable')
                        : undefined,
                    danger: true,
                    onSelect: () => setDialog({ kind: 'revoke', token }),
                  },
                  {
                    // Deletion is offered for every state, because what it
                    // removes is the record rather than the token's usability.
                    label: t('auth.deleteToken'),
                    disabled: !canManage,
                    reason: canManage ? undefined : unavailable,
                    danger: true,
                    onSelect: () => setDialog({ kind: 'delete', token }),
                  },
                ];
                return (
                  <Fragment key={token.id}>
                    <tr>
                      <th scope="row">{token.name}</th>
                      <td data-label={t('auth.column.owner')}>{token.owner}</td>
                      <td data-label={t('auth.column.state')}>{t(`auth.${state}`)}</td>
                      <td className="cell-mono" data-label={t('auth.column.expires')}>
                        {token.expiresAt ?? t('auth.never')}
                      </td>
                      <td className="record-cell-actions">
                        <RowActions
                          name={token.name}
                          typeLabel={t('auth.tokenType')}
                          status={t(`auth.${state}`)}
                          actions={actions}
                        >
                          <DetailsToggle
                            name={token.name}
                            expanded={expanded}
                            controls={`token-details-${token.id}`}
                            onToggle={() =>
                              setDetails((current) => {
                                const next = new Set(current);
                                if (next.has(token.id)) {
                                  next.delete(token.id);
                                } else {
                                  next.add(token.id);
                                }
                                return next;
                              })
                            }
                          />
                        </RowActions>
                      </td>
                    </tr>
                    {expanded ? (
                      <tr className="record-details" id={`token-details-${token.id}`}>
                        <td colSpan={5}>
                          <dl className="record-facts">
                            <div>
                              <dt>{t('auth.column.scopes')}</dt>
                              <dd>
                                {token.scopes.length === 0
                                  ? t('auth.wholeRole')
                                  : token.scopes.join(', ')}
                              </dd>
                            </div>
                            <div>
                              <dt>{t('auth.column.created')}</dt>
                              <dd className="cell-mono">{token.createdAt}</dd>
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
          <CreateTokenForm
            identities={identities}
            working={working}
            failure={failure}
            onDismiss={close}
            onSubmit={(request: NewToken) =>
              void apply(
                async () => {
                  const issued = await client.createToken(credential, request);
                  return {
                    kind: 'secret',
                    name: issued.token.name,
                    secret: issued.secret,
                  };
                },
                t('auth.tokenCreated', { name: request.name, owner: request.owner }),
              )
            }
          />
        );
      case 'revoke':
        return (
          <ConfirmationForm
            workflowHelp={
              <WorkflowHelp prefix="help.auth" title={t('help.auth.title')} />
            }
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
      case 'delete':
        return (
          <PermanentDeletionForm
            workflowHelp={
              <WorkflowHelp prefix="help.auth" title={t('help.auth.title')} />
            }
            name={dialog.token.name}
            prefix="delete-token"
            explanation={t('auth.deleteTokenExplanation', {
              name: dialog.token.name,
              owner: dialog.token.owner,
            })}
            confirmLabel={t('auth.deleteToken')}
            fieldLabel={t('auth.field.confirmTokenName')}
            fieldHelp={t('auth.help.confirmTokenName')}
            working={working}
            failure={failure}
            onDismiss={close}
            onSubmit={() =>
              void apply(
                async () => {
                  await client.deleteToken(credential, dialog.token.id);
                  return null;
                },
                t('auth.tokenDeleted', { name: dialog.token.name }),
              )
            }
          />
        );
      case 'secret':
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

function tokenState(token: BenchCoreToken): 'active' | 'expired' | 'revoked' {
  if (token.revoked) return 'revoked';
  if (token.expiresAt !== null && new Date(token.expiresAt).getTime() <= Date.now())
    return 'expired';
  return 'active';
}
type Translate = ReturnType<typeof useTranslate>;
function dialogTitle(dialog: Dialog, t: Translate) {
  switch (dialog.kind) {
    case 'create':
      return t('auth.dialog.newToken');
    case 'revoke':
      return t('auth.dialog.revokeToken', { name: dialog.token.name });
    case 'delete':
      return t('auth.dialog.deleteToken', { name: dialog.token.name });
    case 'secret':
      return t('auth.dialog.secret', { name: dialog.name });
  }
}
