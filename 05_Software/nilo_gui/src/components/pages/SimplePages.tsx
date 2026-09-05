import { useTranslate } from '../../i18n/languages';
import type {
  BenchCoreIntegration,
  BenchCoreStatus,
  DeviceInventory,
} from '../../api/contract';
import type { Resource } from '../../api/resource';

export interface AboutPageProps {
  /** The reported backend status, or null when it could not be read. */
  status: BenchCoreStatus | null;
}

/**
 * About: the build identity the backend actually reported.
 *
 * Every value here comes from the status resource, so this page is real rather
 * than a placeholder. The names are translated and the values are not: a version
 * string and a protocol name are identifiers, not prose.
 */
export function AboutPage({ status }: AboutPageProps) {
  const t = useTranslate();

  return (
    <div className="page">
      <header className="page-head">
        <h2>{t('about.title')}</h2>
      </header>
      <div className="page-body">
        {status === null ? (
          <section className="notice notice-unknown">
            <p>{t('about.unavailable')}</p>
          </section>
        ) : (
          <div className="table-scroll">
            <table className="data-table record-table">
              <thead>
                <tr>
                  <th scope="col">{t('about.component')}</th>
                  <th scope="col">{t('about.value')}</th>
                </tr>
              </thead>
              <tbody>
                <tr>
                  <th scope="row">{t('about.name')}</th>
                  <td className="cell-mono">{status.build.name}</td>
                </tr>
                <tr>
                  <th scope="row">{t('about.version')}</th>
                  <td className="cell-mono">{status.build.version}</td>
                </tr>
                <tr>
                  <th scope="row">{t('about.apiVersion')}</th>
                  <td className="cell-mono">{status.apiVersion}</td>
                </tr>
                <tr>
                  <th scope="row">{t('about.integrationProtocol')}</th>
                  <td className="cell-mono">{status.build.integrationProtocol}</td>
                </tr>
                <tr>
                  <th scope="row">{t('about.profile')}</th>
                  <td className="cell-mono">{status.profile}</td>
                </tr>
              </tbody>
            </table>
          </div>
        )}
        <section className="notice">
          <h3>{t('about.licence')}</h3>
          <p>{t('about.licence.body')}</p>
        </section>
      </div>
    </div>
  );
}

export interface IntegrationPageProps {
  integration: BenchCoreIntegration;
  /**
   * The device inventory as it was read, not as a list. An unreadable inventory
   * is not an integration that owns no devices, and saying so would invent a
   * fact about the bench that nobody established.
   */
  devices: Resource<DeviceInventory>;
}

/**
 * One integration, as the backend reports it.
 *
 * This is not an instrument panel: the appliance exposes an integration's
 * identity, state and declared operations, and that is what this shows. Building
 * a capture view for a device NILO cannot yet drive would be an illustration,
 * not a feature.
 */
export function IntegrationPage({ integration, devices }: IntegrationPageProps) {
  const t = useTranslate();
  const owned = devices.ok
    ? devices.value.items.filter((device) => device.integrationId === integration.id)
    : null;

  // No page-head: the tab already carries this integration's name, and the
  // agreed layout forbids a duplicate page-title row above an instrument. The
  // identifier the head used to carry is a fact among the others instead.
  return (
    <div className="page">
      <div className="page-body">
        <dl className="fact-grid">
          <div className="fact">
            <dt>{t('integration.identifier')}</dt>
            <dd className="cell-mono">{integration.id}</dd>
          </div>
          <div className="fact">
            <dt>{t('integration.state')}</dt>
            <dd className="cell-mono">{integration.status}</dd>
          </div>
          <div className="fact">
            <dt>{t('integration.simulated')}</dt>
            <dd className="cell-mono">
              {integration.simulation ? t('bench.yes') : t('bench.no')}
            </dd>
          </div>
          <div className="fact">
            <dt>{t('integration.operations')}</dt>
            <dd className="cell-mono">{integration.operations.join(', ')}</dd>
          </div>
        </dl>

        <h3 className="section-title">{t('integration.devices')}</h3>
        {owned === null ? (
          <p className="empty">{t('integration.devicesUnavailable')}</p>
        ) : owned.length === 0 ? (
          <p className="empty">{t('integration.noDevices')}</p>
        ) : (
          <div className="table-scroll">
            <table className="data-table record-table">
              <thead>
                <tr>
                  <th scope="col">{t('integration.displayName')}</th>
                  <th scope="col">{t('integration.identifier')}</th>
                  <th scope="col">{t('bench.availability')}</th>
                  <th scope="col">{t('bench.kind')}</th>
                  <th scope="col">{t('bench.capabilities')}</th>
                </tr>
              </thead>
              <tbody>
                {owned.map((device) => (
                  <tr key={device.id}>
                    <th scope="row">{device.displayName}</th>
                    <td className="cell-mono" data-label={t('integration.identifier')}>
                      {device.id}
                    </td>
                    <td className="cell-mono" data-label={t('bench.availability')}>
                      {device.status}
                    </td>
                    <td className="cell-mono" data-label={t('bench.kind')}>
                      {device.kind}
                    </td>
                    <td className="cell-mono" data-label={t('bench.capabilities')}>
                      {device.capabilities.join(', ')}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  );
}
