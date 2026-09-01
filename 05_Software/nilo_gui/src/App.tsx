import { useCallback, useState } from 'react';

import './App.css';
import { BenchCoreError, type BenchCoreClient } from './api/client';
import { LoginScreen } from './components/LoginScreen';
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

/**
 * The NILO development shell.
 *
 * It owns the whole conversation with Bench Core: the bearer token, which is
 * held in this component's state and nowhere else, and the connection state
 * the panels render. Every screen keeps the SIMULATION identity and the
 * physical-equipment boundary visible.
 */
function App({ client, apiMode, endpoint }: AppProps) {
  const [session, setSession] = useState<Session>({ phase: 'unauthenticated' });

  const requestStatus = useCallback(
    async (token: string) => {
      setSession({ token, phase: 'checking' });
      try {
        const status = await client.getStatus(token);
        setSession({ token, phase: 'connected', status });
      } catch (error) {
        setSession(
          error instanceof BenchCoreError
            ? { token, phase: 'failed', failure: error.failure, detail: error.detail }
            : {
                token,
                phase: 'failed',
                failure: 'unavailable',
                detail: error instanceof Error ? error.message : String(error),
              },
        );
      }
    },
    [client],
  );

  const retry = useCallback(() => {
    if (session.phase !== 'unauthenticated') {
      void requestStatus(session.token);
    }
  }, [requestStatus, session]);

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
            onSubmit={(token) => void requestStatus(token)}
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
