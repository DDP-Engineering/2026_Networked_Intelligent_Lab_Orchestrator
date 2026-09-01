import type { BenchCoreFailure } from '../api/client';
import type {
  BenchCoreStatus,
  DeviceInventory,
  IntegrationInventory,
} from '../api/contract';
import { InventoryPanel, type Resource } from './InventoryPanel';

/**
 * What the shell knows about the current Bench Core conversation.
 *
 * A status failure settles the whole screen: without a status there is neither
 * a usable token nor a reachable backend to hang an inventory on. Once the
 * status is in, each inventory carries its own outcome.
 */
export type Connection =
  | { phase: 'checking' }
  | {
      phase: 'connected';
      status: BenchCoreStatus;
      integrations: Resource<IntegrationInventory>;
      devices: Resource<DeviceInventory>;
    }
  | { phase: 'failed'; failure: BenchCoreFailure; detail: string };

export interface StatusPanelProps {
  connection: Connection;
  /** Repeat every request with the token already held. */
  onRetry: () => void;
  /** Discard the held token and return to the login screen. */
  onReset: () => void;
}

const FAILURE_HEADLINE: Record<BenchCoreFailure, string> = {
  unauthorized: 'Bench Core rejected this token.',
  unavailable: 'Bench Core is not reachable.',
  malformed: 'Bench Core replied with an unsupported status response.',
};

const FAILURE_ADVICE: Record<BenchCoreFailure, string> = {
  unauthorized:
    'The token was refused. Issue a new development token in WSL2 and enter it again.',
  unavailable:
    'Nothing answered on the configured endpoint. Confirm the simulation server is running in WSL2 and listening on that loopback port.',
  malformed:
    'The endpoint answered, but not with a status document this interface understands. Check that the backend and this interface are from the same revision.',
};

/** Render the outcome of the authenticated requests. */
export function StatusPanel({ connection, onRetry, onReset }: StatusPanelProps) {
  if (connection.phase === 'checking') {
    return (
      <section className="panel" aria-labelledby="connection-title">
        <p className="eyebrow">Authenticated request</p>
        <h2 id="connection-title">Contacting Bench Core…</h2>
        <p className="panel-copy" role="status">
          Waiting for the authenticated status, integration, and device responses.
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
    <div className="connected">
      <section className="panel" aria-labelledby="connection-title">
        <div className="panel-head">
          <div>
            <p className="eyebrow">Authenticated request</p>
            <h2 id="connection-title">Bench Core connected</h2>
          </div>
          <div className="actions">
            <button type="button" onClick={onRetry}>
              Refresh
            </button>
            <button type="button" className="secondary" onClick={onReset}>
              Use a different token
            </button>
          </div>
        </div>

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
      </section>

      <InventoryPanel
        integrations={connection.integrations}
        devices={connection.devices}
      />
    </div>
  );
}
