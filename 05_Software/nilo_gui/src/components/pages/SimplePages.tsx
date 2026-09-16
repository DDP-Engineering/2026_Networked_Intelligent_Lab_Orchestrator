import { useState, type FormEvent } from 'react';

import { useTranslate } from '../../i18n/languages';
import type { ApplianceMetadata, IdentifyResult } from '../../api/appliance';
import type {
  BenchCoreIntegration,
  BenchCoreStatus,
  DeviceInventory,
} from '../../api/contract';
import type { Resource } from '../../api/resource';

export interface AboutPageProps {
  /** The reported backend status, or null when it could not be read. */
  status: BenchCoreStatus | null;
  appliance?: ApplianceMetadata | null;
  labelUrl?: string;
  canIdentify?: boolean;
  onIdentify?: () => Promise<IdentifyResult>;
  onFriendlyName?: (name: string) => Promise<ApplianceMetadata>;
}

/**
 * About: the build identity the backend actually reported.
 *
 * Every value here comes from the status resource, so this page is real rather
 * than a placeholder. The names are translated and the values are not: a version
 * string and a protocol name are identifiers, not prose.
 */
export function AboutPage({
  status,
  appliance,
  labelUrl,
  canIdentify = false,
  onIdentify,
  onFriendlyName,
}: AboutPageProps) {
  const t = useTranslate();
  const [friendlyName, setFriendlyName] = useState(appliance?.friendlyName ?? '');
  const [actionStatus, setActionStatus] = useState('');
  const [working, setWorking] = useState(false);

  const identify = async () => {
    if (onIdentify === undefined) return;
    setWorking(true);
    try {
      const result = await onIdentify();
      setActionStatus(
        result.supported
          ? t('about.identify.active', {
              time: new Date(result.activeUntil).toLocaleTimeString(),
            })
          : t('about.identify.unsupported'),
      );
    } catch {
      setActionStatus(t('about.identify.failed'));
    } finally {
      setWorking(false);
    }
  };

  const saveFriendlyName = async (event: FormEvent) => {
    event.preventDefault();
    if (onFriendlyName === undefined) return;
    setWorking(true);
    try {
      const updated = await onFriendlyName(friendlyName);
      setFriendlyName(updated.friendlyName);
      setActionStatus(t('about.friendly.updated'));
    } catch {
      setActionStatus(t('about.friendly.failed'));
    } finally {
      setWorking(false);
    }
  };

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
        {appliance !== undefined && appliance !== null && (
          <section className="notice" aria-labelledby="appliance-identity-title">
            <h3 id="appliance-identity-title">{t('about.appliance')}</h3>
            <dl className="fact-grid">
              <div className="fact">
                <dt>{t('about.permanentName')}</dt>
                <dd className="cell-mono">{appliance.serialName}</dd>
              </div>
              {appliance.friendlyName !== '' && (
                <div className="fact">
                  <dt>{t('about.friendlyName')}</dt>
                  <dd className="cell-mono">{appliance.friendlyName}</dd>
                </div>
              )}
              <div className="fact">
                <dt>{t('about.lifecycle')}</dt>
                <dd>{appliance.state}</dd>
              </div>
              <div className="fact">
                <dt>{t('about.managementUrl')}</dt>
                <dd>
                  <a className="fact-link" href={appliance.managementUrl}>
                    {appliance.managementUrl}
                  </a>
                </dd>
              </div>
              <div className="fact">
                <dt>{t('about.managementAddresses')}</dt>
                <dd className="cell-mono">
                  {appliance.managementAddresses.join(', ')}
                </dd>
              </div>
              <div className="fact">
                <dt>{t('about.hardwareAddresses')}</dt>
                <dd className="cell-mono">{appliance.managementMacs.join(', ')}</dd>
              </div>
            </dl>
            <div className="form-actions">
              {labelUrl !== undefined && (
                <a className="button" href={labelUrl} target="_blank" rel="noreferrer">
                  {t('about.openLabel')}
                </a>
              )}
              {onIdentify !== undefined && (
                <button
                  type="button"
                  className="button button-primary"
                  disabled={!canIdentify || working}
                  onClick={() => void identify()}
                >
                  {t('about.identify')}
                </button>
              )}
            </div>
            {onFriendlyName !== undefined && (
              <form className="form" onSubmit={(event) => void saveFriendlyName(event)}>
                <div className="field">
                  <label htmlFor="friendly-name">{t('about.friendlyName')}</label>
                  <input
                    id="friendly-name"
                    value={friendlyName}
                    onChange={(event) => setFriendlyName(event.target.value)}
                    pattern="[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?"
                  />
                </div>
                <div className="form-actions">
                  <button type="submit" className="button" disabled={working}>
                    {t('about.friendly.save')}
                  </button>
                </div>
              </form>
            )}
            {actionStatus !== '' && <p role="status">{actionStatus}</p>}
          </section>
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
