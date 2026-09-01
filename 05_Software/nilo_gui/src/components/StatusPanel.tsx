import type { BenchCoreFailure } from '../api/client';
import type { BenchCoreStatus } from '../api/contract';

/** What the shell knows about the current Bench Core conversation. */
export type Connection =
  | { phase: 'checking' }
  | { phase: 'connected'; status: BenchCoreStatus }
  | { phase: 'failed'; failure: BenchCoreFailure; detail: string };

export interface StatusPanelProps {
  connection: Connection;
  /** Repeat the status request with the token already held. */
  onRetry: () => void;
  /** Discard the held token and return to the login screen. */
  onReset: () => void;
}

const FAILURE_HEADLINE: Record<BenchCoreFailure, string> = {
  unauthorized: 'Bench Core rejected this token.',
  unavailable: 'Bench Core is not reachable.',
  malformed: 'Bench Core replied with an unsupported response.',
};

const FAILURE_ADVICE: Record<BenchCoreFailure, string> = {
  unauthorized:
    'The token was refused. Issue a new development token in WSL2 and enter it again.',
  unavailable:
    'Nothing answered on the configured endpoint. Confirm the simulation server is running in WSL2 and listening on that loopback port.',
  malformed:
    'The endpoint answered, but not with a status document this interface understands. Check that the backend and this interface are from the same revision.',
};

/** Render the outcome of the authenticated status request. */
export function StatusPanel({ connection, onRetry, onReset }: StatusPanelProps) {
  if (connection.phase === 'checking') {
    return (
      <section className="panel" aria-labelledby="connection-title">
        <p className="eyebrow">Authenticated request</p>
        <h2 id="connection-title">Contacting Bench Core…</h2>
        <p className="panel-copy" role="status">
          Waiting for the authenticated status response.
        </p>
      </section>
    );
  }

  if (connection.phase === 'failed') {
    return (
      <section className="panel panel-failed" aria-labelledby="connection-title">
        <p className="eyebrow">Authenticated request</p>
        <h2 id="connection-title">{FAILURE_HEADLINE[connection.failure]}</h2>
        <p className="panel-copy" role="alert">
          {FAILURE_ADVICE[connection.failure]}
        </p>
        <p className="failure-detail">{connection.detail}</p>
        <div className="actions">
          {connection.failure === 'unauthorized' ? null : (
            <button type="button" onClick={onRetry}>
              Try again
            </button>
          )}
          <button type="button" className="secondary" onClick={onReset}>
            Use a different token
          </button>
        </div>
      </section>
    );
  }

  const { status } = connection;
  return (
    <section className="panel" aria-labelledby="connection-title">
      <p className="eyebrow">Authenticated request</p>
      <h2 id="connection-title">Bench Core connected</h2>

      <dl className="status-grid">
        <div className="status-card">
          <dt>Profile</dt>
          <dd>{status.profile}</dd>
        </div>
        <div className="status-card">
          <dt>Service status</dt>
          <dd>{status.status}</dd>
        </div>
        <div className="status-card">
          <dt>Simulation</dt>
          <dd>{status.simulation ? 'Yes' : 'No'}</dd>
        </div>
        <div className="status-card">
          <dt>Build version</dt>
          <dd>{status.build.version}</dd>
        </div>
        <div className="status-card">
          <dt>API version</dt>
          <dd>{status.apiVersion}</dd>
        </div>
        <div className="status-card">
          <dt>Integration protocol</dt>
          <dd>{status.build.integrationProtocol}</dd>
        </div>
      </dl>

      <div className="actions">
        <button type="button" onClick={onRetry}>
          Refresh status
        </button>
        <button type="button" className="secondary" onClick={onReset}>
          Use a different token
        </button>
      </div>
    </section>
  );
}
