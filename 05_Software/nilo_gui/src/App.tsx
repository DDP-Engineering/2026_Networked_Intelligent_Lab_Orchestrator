import { useCallback, useState } from 'react';

import './App.css';
import { BenchCoreError, type BenchCoreClient } from './api/client';
import { LoginScreen } from './components/LoginScreen';
import type { Resource } from './components/InventoryPanel';
import { StatusPanel, type Connection } from './components/StatusPanel';
import type { ApiMode } from './config/environment';

export interface AppProps {
  /** The Bench Core client this session talks to. */
  client: BenchCoreClient;
  /** Whether that client reaches a real backend or the contract mock. */
  apiMode: ApiMode;
  /**
   * The Bench Core endpoint to show the developer. Requests themselves are
   * origin-relative and forwarded there, so this is a label, not a target.
   */
  endpoint: string;
}

type Session = { phase: 'unauthenticated' } | ({ token: string } & Connection);

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
 * The NILO development shell.
 *
 * It owns the whole conversation with Bench Core: the bearer token, which is
 * held in this component's state and nowhere else, and the connection state the
 * panels render. Every screen keeps the SIMULATION identity and the
 * physical-equipment boundary visible.
 */
function App({ client, apiMode, endpoint }: AppProps) {
  const [session, setSession] = useState<Session>({ phase: 'unauthenticated' });

  const requestBench = useCallback(
    async (token: string) => {
      setSession({ token, phase: 'checking' });

      // One round trip for the whole screen: the three resources are
      // independent, so nothing is gained by asking for them in turn.
      const [status, integrations, devices] = await Promise.all([
        settle(client.getStatus(token)),
        settle(client.getIntegrations(token)),
        settle(client.getDevices(token)),
      ]);

      // A status that could not be read means the token, the backend, or the
      // contract is wrong for everything, so it settles the whole screen.
      if (!status.ok) {
        setSession({
          token,
          phase: 'failed',
          failure: status.failure,
          detail: status.detail,
        });
        return;
      }

      setSession({
        token,
        phase: 'connected',
        status: status.value,
        integrations,
        devices,
      });
    },
    [client],
  );

  const retry = useCallback(() => {
    if (session.phase !== 'unauthenticated') {
      void requestBench(session.token);
    }
  }, [requestBench, session]);

  const reset = useCallback(() => setSession({ phase: 'unauthenticated' }), []);

  return (
    <div className="app-frame">
      <header className="topbar">
        <div className="brand">
          <span className="brand-mark" aria-hidden="true">
            N
          </span>
          <div>
            <h1>NILO</h1>
            <p>Networked Intelligent Lab Orchestrator</p>
          </div>
        </div>
        <span className="simulation-status" role="status" aria-label="Runtime profile">
          <span className="status-dot" aria-hidden="true" />
          SIMULATION
        </span>
      </header>

      <main className="workspace">
        {session.phase === 'unauthenticated' ? (
          <LoginScreen
            endpoint={endpoint}
            onSubmit={(token) => void requestBench(token)}
          />
        ) : (
          <StatusPanel connection={session} onRetry={retry} onReset={reset} />
        )}
      </main>

      <footer className="boundary">
        <p>
          Simulation profile only — simulated results never represent evidence from
          physical equipment.
        </p>
        <p className="mode-note">
          {apiMode === 'mock'
            ? 'Answers come from this interface’s own contract mock; no backend process is contacted.'
            : 'Requests go to the configured Bench Core development endpoint.'}
        </p>
      </footer>
    </div>
  );
}

export default App;
