import { useCallback, useEffect, useState } from 'react';

import {
  BenchCoreError,
  sessionCredential,
  type BenchCoreClient,
  type BenchCoreFailure,
  type Credential,
} from './api/client';
import type {
  BenchCoreSession,
  BenchCoreStatus,
  DeviceInventory,
  IntegrationInventory,
} from './api/contract';
import { readCsrfToken } from './api/csrf';
import type { Resource } from './api/resource';
import type { InstrumentPlugin } from './api/bench';
import { useTranslate } from './i18n/languages';
import type { MessageKey } from './i18n/en';
import { AppShell } from './components/shell/AppShell';
import { useWorkspace } from './components/shell/useWorkspace';
import { DEFAULT_PAGE } from './components/shell/pages';
import { SignInScreen, type SignInRefusal } from './components/SignInScreen';
import type { ApiMode, RuntimeProfile } from './config/environment';
import { NILO_VERSION } from './version';

export interface AppProps {
  /** The Bench Core client this session talks to. */
  client: BenchCoreClient;
  /** Whether that client reaches a real backend or the contract mock. */
  apiMode: ApiMode;
  runtimeProfile?: RuntimeProfile;
  /**
   * The Bench Core endpoint to show the developer. Requests themselves are
   * origin-relative and forwarded there, so this is a label, not a target.
   */
  endpoint: string;
}

/**
 * What the shell knows about the current conversation with Bench Core.
 *
 * Every page load starts in `restoring`: the browser session secret lives in a
 * cookie this page deliberately cannot read, so the only way to learn whether
 * somebody is signed in is to ask Bench Core who the browser's cookies belong
 * to. Nothing about the credential is kept anywhere durable by this interface.
 */
type Shell =
  | { phase: 'restoring' }
  | {
      phase: 'signed-out';
      notice: MessageKey | null;
      refusal: SignInRefusal | null;
      working: boolean;
    }
  | { phase: 'connecting'; credential: Credential; session: BenchCoreSession | null }
  | {
      phase: 'refused';
      credential: Credential;
      session: BenchCoreSession | null;
      failure: BenchCoreFailure;
      detail: string;
    }
  | {
      phase: 'active';
      credential: Credential;
      session: BenchCoreSession;
      status: BenchCoreStatus;
      integrations: Resource<IntegrationInventory>;
      devices: Resource<DeviceInventory>;
    };

const FAILURE_HEADLINE: Record<BenchCoreFailure, MessageKey> = {
  unauthorized: 'failure.unauthorized',
  forbidden: 'failure.forbidden',
  rejected: 'failure.rejected',
  unavailable: 'failure.unavailable',
  malformed: 'failure.malformed',
};

const FAILURE_ADVICE: Record<BenchCoreFailure, MessageKey> = {
  unauthorized: 'failure.unauthorized.advice',
  forbidden: 'failure.forbidden.advice',
  rejected: 'failure.rejected.advice',
  unavailable: 'failure.unavailable.advice',
  malformed: 'failure.malformed.advice',
};

/** The signed-out state, with whatever there is to say about how we got here. */
function signedOut(
  notice: MessageKey | null = null,
  refusal: SignInRefusal | null = null,
): Shell {
  return { phase: 'signed-out', notice, refusal, working: false };
}

/** Turn a settled request into a value or the reason it could not be used. */
async function settle<T>(request: Promise<T>): Promise<Resource<T>> {
  try {
    return { ok: true, value: await request };
  } catch (error) {
    return error instanceof BenchCoreError
      ? { ok: false, failure: error.failure, detail: error.detail }
      : {
          ok: false,
          failure: 'unavailable',
          detail: error instanceof Error ? error.message : String(error),
        };
  }
}

/**
 * The NILO interface.
 *
 * It owns the whole conversation with Bench Core: which credential is in use and
 * who that credential belongs to. A password never outlives the request that
 * carries it, and the browser session lives in the server's own `HttpOnly`
 * cookie.
 */
function App({ client }: AppProps) {
  const t = useTranslate();
  const [shell, setShell] = useState<Shell>({ phase: 'restoring' });
  const [lastIntegrations, setLastIntegrations] = useState<IntegrationInventory | null>(
    null,
  );
  const [lastDevices, setLastDevices] = useState<DeviceInventory | null>(null);
  const [lastPlugins, setLastPlugins] = useState<InstrumentPlugin[] | null>(null);
  const clearInventory = useCallback(() => {
    setLastIntegrations(null);
    setLastDevices(null);
    setLastPlugins(null);
  }, []);
  // Owned here rather than in the shell so a refresh, which passes through the
  // connecting phase and unmounts the shell, does not close the open tabs.
  const workspace = useWorkspace([DEFAULT_PAGE]);
  const [expandedNavigation, setExpandedNavigation] = useState<Record<string, boolean>>(
    {},
  );
  const toggleNavigation = useCallback((id: string) => {
    setExpandedNavigation((current) => ({ ...current, [id]: !current[id] }));
  }, []);
  const resetNavigation = useCallback(() => setExpandedNavigation({}), []);

  /** Read the bench with a credential whose owner is already known. */
  const loadBench = useCallback(
    async (credential: Credential, session: BenchCoreSession) => {
      setShell({ phase: 'connecting', credential, session });

      // One round trip for the whole screen: the three resources are
      // independent, so nothing is gained by asking for them in turn.
      const [status, integrations, devices] = await Promise.all([
        settle(client.getStatus(credential)),
        settle(client.getIntegrations(credential)),
        settle(client.getDevices(credential)),
      ]);

      // A status that could not be read means the credential, the backend, or
      // the contract is wrong for everything, so it settles the whole screen.
      if (!status.ok) {
        if (status.failure === 'unauthorized' && credential.kind === 'session') {
          setShell(signedOut('signIn.ended'));
          return;
        }
        setShell({
          phase: 'refused',
          credential,
          session,
          failure: status.failure,
          detail: status.detail,
        });
        return;
      }
      if (integrations.ok) setLastIntegrations(integrations.value);
      if (devices.ok) setLastDevices(devices.value);
      setShell({
        phase: 'active',
        credential,
        session,
        status: status.value,
        integrations,
        devices,
      });
    },
    [client],
  );

  /** Ask who a credential belongs to, then read the bench as that identity. */
  const connect = useCallback(
    async (credential: Credential) => {
      setShell({ phase: 'connecting', credential, session: null });
      const session = await settle(client.currentSession(credential));
      if (!session.ok) {
        if (session.failure === 'unauthorized' && credential.kind === 'session') {
          setShell(signedOut('signIn.ended'));
          return;
        }
        setShell({
          phase: 'refused',
          credential,
          session: null,
          failure: session.failure,
          detail: session.detail,
        });
        return;
      }
      await loadBench(credential, session.value);
    },
    [client, loadBench],
  );

  // Restoration, once, at start-up. A browser that holds no usable session
  // simply gets the sign-in screen; that is not a failure worth reporting.
  useEffect(() => {
    let abandoned = false;
    void (async () => {
      const credential = sessionCredential(readCsrfToken());
      const session = await settle(client.currentSession(credential));
      if (abandoned) {
        return;
      }
      if (!session.ok) {
        setShell(signedOut());
        return;
      }
      await loadBench(credential, session.value);
    })();
    return () => {
      abandoned = true;
    };
  }, [client, loadBench]);

  const signIn = useCallback(
    async (username: string, password: string) => {
      setShell({ phase: 'signed-out', notice: null, refusal: null, working: true });
      try {
        // The password exists only as an argument to this one call: it is in
        // one request body and in no store, URL or log.
        const session = await client.signIn(username, password);
        workspace.reset();
        resetNavigation();
        clearInventory();
        await loadBench(
          sessionCredential(session.csrfToken ?? readCsrfToken()),
          session,
        );
      } catch (error) {
        setShell(signedOut(null, { message: signInRefusal(error) }));
      }
    },
    [client, loadBench, workspace, resetNavigation, clearInventory],
  );

  const signOut = useCallback(async () => {
    const credential = credentialOf(shell);
    // A new identity inherits neither tabs nor disclosure choices.
    workspace.reset();
    resetNavigation();
    clearInventory();
    setShell(signedOut('signIn.signedOut'));
    if (credential?.kind === 'session') {
      // The session has to end at the server; forgetting it here would leave a
      // cookie that still works for whoever holds it.
      await settle(client.signOut(credential));
    }
  }, [client, shell, workspace, resetNavigation, clearInventory]);

  const retry = useCallback(() => {
    if (shell.phase === 'refused' && shell.session !== null) {
      void loadBench(shell.credential, shell.session);
      return;
    }
    const credential = credentialOf(shell);
    if (credential !== null) {
      void connect(credential);
    }
  }, [connect, loadBench, shell]);

  if (shell.phase === 'active') {
    return (
      <AppShell
        client={client}
        credential={shell.credential}
        session={shell.session}
        status={shell.status}
        integrations={shell.integrations}
        devices={shell.devices}
        lastIntegrations={lastIntegrations}
        lastDevices={lastDevices}
        lastPlugins={lastPlugins}
        onPluginsLoaded={setLastPlugins}
        workspace={workspace}
        expandedNavigation={expandedNavigation}
        onToggleNavigation={toggleNavigation}
        onRefresh={() => void loadBench(shell.credential, shell.session)}
        onSignOut={() => void signOut()}
        onSessionEnded={() => {
          workspace.reset();
          resetNavigation();
          clearInventory();
          setShell(signedOut('signIn.ended'));
        }}
      />
    );
  }

  if (shell.phase === 'signed-out') {
    return (
      <SignInScreen
        notice={shell.notice}
        working={shell.working}
        refusal={shell.refusal}
        onSignIn={(username, password) => void signIn(username, password)}
      />
    );
  }

  // Restoring, connecting and refused all share the centred product card:
  // there is no workspace to frame yet.
  return (
    <div className="centred">
      <section className="card" aria-labelledby="connection-title">
        <div className="sign-in-brand">
          <p className="sign-in-wordmark">{t('app.name')}</p>
          <p className="sign-in-version">
            {t('signIn.version', { version: NILO_VERSION })}
          </p>
        </div>

        {shell.phase === 'refused' ? (
          <>
            <h2 id="connection-title">{t(FAILURE_HEADLINE[shell.failure])}</h2>
            <p className="card-copy" role="alert">
              {t(FAILURE_ADVICE[shell.failure])}
            </p>
            <p className="failure-detail">{shell.detail}</p>
            <div className="form-actions">
              {shell.failure === 'unauthorized' ? null : (
                <button type="button" className="button button-primary" onClick={retry}>
                  {t('failure.tryAgain')}
                </button>
              )}
              <button
                type="button"
                className="button"
                onClick={() => setShell(signedOut())}
              >
                {t('bench.signInAgain')}
              </button>
            </div>
          </>
        ) : (
          <>
            <h2 id="connection-title">
              {shell.phase === 'restoring'
                ? t('restoring.title')
                : t('bench.connecting')}
            </h2>
            <p className="card-copy" role="status">
              {shell.phase === 'restoring'
                ? t('restoring.body')
                : t('bench.connecting.body')}
            </p>
          </>
        )}
      </section>
    </div>
  );
}

/** The credential the shell currently holds, if it holds one at all. */
function credentialOf(shell: Shell): Credential | null {
  switch (shell.phase) {
    case 'connecting':
    case 'refused':
    case 'active':
      return shell.credential;
    default:
      return null;
  }
}

/**
 * Say why a sign-in did not succeed.
 *
 * A refused password and a throttled username are different problems: one is
 * fixed by typing again and the other only by waiting, so they do not share a
 * message.
 */
function signInRefusal(error: unknown): MessageKey | string {
  if (!(error instanceof BenchCoreError)) {
    return error instanceof Error ? error.message : String(error);
  }
  if (error.code === 'too_many_attempts') {
    return 'signIn.throttled';
  }
  if (error.failure === 'unauthorized') {
    return 'signIn.refused';
  }
  return error.detail;
}

export default App;
