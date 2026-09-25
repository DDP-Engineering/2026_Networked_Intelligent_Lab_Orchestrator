import type { BenchCoreFailure } from '../../api/client';
import type {
  BenchCoreStatus,
  DeviceInventory,
  IntegrationInventory,
} from '../../api/contract';
import type { Resource } from '../../api/resource';
import { useTranslate } from '../../i18n/languages';
import type { MessageKey } from '../../i18n/en';

/** The failure sentence for one named inventory. */
const INVENTORY_FAILURE: Record<BenchCoreFailure, MessageKey> = {
  unauthorized: 'failure.inventory.unauthorized',
  forbidden: 'failure.inventory.forbidden',
  rejected: 'failure.inventory.rejected',
  unavailable: 'failure.inventory.unavailable',
  malformed: 'failure.inventory.malformed',
};

export interface BenchPageProps {
  status: BenchCoreStatus;
  integrations: Resource<IntegrationInventory>;
  devices: Resource<DeviceInventory>;
  onRefresh: () => void;
}

/**
 * The Bench page: the reported status, then the two inventories.
 *
 * The status is a plain fact row rather than a panel, and each inventory is one
 * bounded table. The card nesting the previous layout used bought nothing that
 * a divider and a heading do not, and it cost the workspace the space this
 * design exists to give back.
 */
export function BenchPage({
  status,
  integrations,
  devices,
  onRefresh,
}: BenchPageProps) {
  const t = useTranslate();

  const facts: [MessageKey, string][] = [
    ['bench.profile', status.profile],
    ['bench.status', status.status],
    ['bench.simulation', status.simulation ? t('bench.yes') : t('bench.no')],
    ['bench.buildVersion', status.build.version],
    ['bench.apiVersion', status.apiVersion],
    ['bench.integrationProtocol', status.build.integrationProtocol],
  ];

  return (
    <div className="page">
      <header className="page-head">
        <h2>{t('bench.title')}</h2>
        <button type="button" className="button" onClick={onRefresh}>
          {t('bench.refresh')}
        </button>
      </header>

      <div className="page-body">
        <dl className="fact-grid">
          {facts.map(([key, value]) => (
            <div className="fact" key={key}>
              <dt>{t(key)}</dt>
              {/* Reported values are identifiers, so they stay as sent. */}
              <dd className="cell-mono">{value}</dd>
            </div>
          ))}
        </dl>

        <div className="inventory-grid">
          <Inventory
            id="integrations"
            title={t('bench.integrations')}
            resourceName={t('failure.resource.integrations')}
            empty={t('bench.noIntegrations')}
            state={integrations}
            count={integrations.ok ? integrations.value.items.length : 0}
            scenario={integrations.ok ? integrations.value.scenario : ''}
          >
            {integrations.ok ? (
              <table className="data-table record-table">
                <thead>
                  <tr>
                    <th scope="col">{t('integration.displayName')}</th>
                    <th scope="col">{t('bench.state')}</th>
                    <th scope="col">{t('bench.operations')}</th>
                  </tr>
                </thead>
                <tbody>
                  {integrations.value.items.map((integration) => (
                    <tr key={integration.id}>
                      <th scope="row">
                        {integration.displayName}
                        <span className="row-id cell-mono">{integration.id}</span>
                      </th>
                      <td className="cell-mono" data-label={t('bench.state')}>
                        {integration.status}
                      </td>
                      <td className="cell-mono" data-label={t('bench.operations')}>
                        {integration.operations.join(', ')}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            ) : null}
          </Inventory>

          <Inventory
            id="devices"
            title={t('bench.devices')}
            resourceName={t('failure.resource.devices')}
            empty={t('bench.noDevices')}
            state={devices}
            count={devices.ok ? devices.value.items.length : 0}
            scenario={devices.ok ? devices.value.scenario : ''}
          >
            {devices.ok ? (
              <table className="data-table record-table">
                <thead>
                  <tr>
                    <th scope="col">{t('integration.displayName')}</th>
                    <th scope="col">{t('bench.availability')}</th>
                    <th scope="col">{t('bench.integration')}</th>
                    <th scope="col">{t('bench.kind')}</th>
                  </tr>
                </thead>
                <tbody>
                  {devices.value.items.map((device) => (
                    <tr key={device.id}>
                      <th scope="row">
                        {device.displayName}
                        <span className="row-id cell-mono">{device.id}</span>
                      </th>
                      <td className="cell-mono" data-label={t('bench.availability')}>
                        {device.status}
                      </td>
                      <td className="cell-mono" data-label={t('bench.integration')}>
                        {device.integrationId}
                      </td>
                      <td className="cell-mono" data-label={t('bench.kind')}>
                        {device.kind}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            ) : null}
          </Inventory>
        </div>
      </div>
    </div>
  );
}

interface InventoryProps<T> {
  id: string;
  title: string;
  /** How this inventory is named inside a failure sentence. */
  resourceName: string;
  empty: string;
  state: Resource<T>;
  count: number;
  scenario: string;
  children: React.ReactNode;
}

/**
 * One inventory: its heading and count, and then entries, an explicit empty
 * statement, or the reason it could not be read. An empty inventory is a valid
 * answer, so it carries no alert semantics; only a failure does.
 */
function Inventory<T>({
  id,
  title,
  resourceName,
  empty,
  state,
  count,
  scenario,
  children,
}: InventoryProps<T>) {
  const t = useTranslate();

  return (
    <section className="inventory" aria-labelledby={`${id}-title`}>
      <header className="inventory-head">
        <h3 id={`${id}-title`}>{title}</h3>
        {state.ok ? (
          <p className="inventory-meta">
            <span>{t(count === 1 ? 'bench.entry' : 'bench.entries', { count })}</span>
            <span className="cell-mono">{t('bench.scenario', { name: scenario })}</span>
          </p>
        ) : null}
      </header>
      {state.ok ? (
        count === 0 ? (
          <p className="empty">{empty}</p>
        ) : (
          <div className="inventory-list">{children}</div>
        )
      ) : (
        <div className="inventory-failed">
          <p role="alert">
            {t(INVENTORY_FAILURE[state.failure], { resource: resourceName })}
          </p>
          <p className="failure-detail">{state.detail}</p>
        </div>
      )}
    </section>
  );
}
