import type { ReactNode } from 'react';

import type { BenchCoreFailure } from '../api/client';
import type { BenchCoreDevice, BenchCoreIntegration } from '../api/contract';

/**
 * One inventory the interface asked Bench Core for: either the entries it
 * reported, or why this particular resource could not be read. Keeping the
 * failure per resource is what lets an unusable integration inventory be told
 * from an unusable device inventory without blanking the rest of the screen.
 */
export type Resource<T> =
  { ok: true; value: T } | { ok: false; failure: BenchCoreFailure; detail: string };

/** Why one inventory is missing, phrased for the resource that is missing. */
function failureHeadline(resource: string, failure: BenchCoreFailure): string {
  switch (failure) {
    case 'unauthorized':
      return `The ${resource} was refused for this token.`;
    case 'unavailable':
      return `The ${resource} is not reachable.`;
    case 'malformed':
      return `The ${resource} could not be read.`;
  }
}

interface InventorySectionProps {
  /** Heading text, which is also this region's accessible name. */
  title: string;
  /** The resource name used in a failure sentence, in lower case. */
  resource: string;
  id: string;
  state: Resource<{ scenario: string; count: number }>;
  /** The message shown when the backend legitimately reported no entries. */
  emptyMessage: string;
  children: ReactNode;
}

/**
 * The shell every inventory shares: heading, scenario and count, and the
 * failed or empty message in place of the list.
 *
 * An empty inventory is a valid answer, so it is stated plainly and carries no
 * alert semantics; only a failure does.
 */
function InventorySection({
  title,
  resource,
  id,
  state,
  emptyMessage,
  children,
}: InventorySectionProps) {
  return (
    <section className="inventory" aria-labelledby={`${id}-title`}>
      <header className="inventory-head">
        <h3 id={`${id}-title`}>{title}</h3>
        {state.ok ? (
          <p className="inventory-meta">
            <span>
              {state.value.count} {state.value.count === 1 ? 'entry' : 'entries'}
            </span>
            <span className="scenario">scenario: {state.value.scenario}</span>
          </p>
        ) : null}
      </header>

      {state.ok ? (
        state.value.count === 0 ? (
          <p className="inventory-empty">{emptyMessage}</p>
        ) : (
          <div className="inventory-list">{children}</div>
        )
      ) : (
        <div className="inventory-failed">
          <p role="alert">{failureHeadline(resource, state.failure)}</p>
          <p className="failure-detail">{state.detail}</p>
        </div>
      )}
    </section>
  );
}

/** Reduce a resource to the summary the shared shell needs. */
function summarize<T extends { scenario: string; items: unknown[] }>(
  resource: Resource<T>,
): Resource<{ scenario: string; count: number }> {
  return resource.ok
    ? {
        ok: true,
        value: {
          scenario: resource.value.scenario,
          count: resource.value.items.length,
        },
      }
    : resource;
}

export interface InventoryPanelProps {
  integrations: Resource<{ scenario: string; items: BenchCoreIntegration[] }>;
  devices: Resource<{ scenario: string; items: BenchCoreDevice[] }>;
}

/** The integration and device inventories, side by side under the status. */
export function InventoryPanel({ integrations, devices }: InventoryPanelProps) {
  return (
    <div className="inventory-grid">
      <InventorySection
        id="integrations"
        title="Integrations"
        resource="integration inventory"
        state={summarize(integrations)}
        emptyMessage="No integrations are offered in this simulation scenario."
      >
        {integrations.ok
          ? integrations.value.items.map((integration) => (
              <article className="entry" key={integration.id}>
                <p className="entry-name">{integration.displayName}</p>
                <p className="entry-id">{integration.id}</p>
                <dl className="entry-facts">
                  <div>
                    <dt>State</dt>
                    <dd>{integration.status}</dd>
                  </div>
                  <div>
                    <dt>Operations</dt>
                    <dd>{integration.operations.join(', ')}</dd>
                  </div>
                </dl>
              </article>
            ))
          : null}
      </InventorySection>

      <InventorySection
        id="devices"
        title="Devices"
        resource="device inventory"
        state={summarize(devices)}
        emptyMessage="No devices are offered in this simulation scenario."
      >
        {devices.ok
          ? devices.value.items.map((device) => (
              <article className="entry" key={device.id}>
                <p className="entry-name">{device.displayName}</p>
                <p className="entry-id">{device.id}</p>
                <dl className="entry-facts">
                  <div>
                    <dt>Availability</dt>
                    <dd>{device.status}</dd>
                  </div>
                  <div>
                    <dt>Integration</dt>
                    <dd>{device.integrationId}</dd>
                  </div>
                  <div>
                    <dt>Kind</dt>
                    <dd>{device.kind}</dd>
                  </div>
                  <div>
                    <dt>Capabilities</dt>
                    <dd>{device.capabilities.join(', ')}</dd>
                  </div>
                </dl>
              </article>
            ))
          : null}
      </InventorySection>
    </div>
  );
}
