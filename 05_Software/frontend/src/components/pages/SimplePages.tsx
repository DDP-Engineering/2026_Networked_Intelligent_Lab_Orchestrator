import { MessageBanner } from '../MessageBanner';
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

const BYTE_UNITS = ['B', 'KiB', 'MiB', 'GiB', 'TiB'] as const;

function formatBytes(bytes: number): string {
  if (bytes <= 0) return `0 ${BYTE_UNITS[0]}`;
  const unit = Math.min(
    Math.floor(Math.log(bytes) / Math.log(1024)),
    BYTE_UNITS.length - 1,
  );
  const value = bytes / 1024 ** unit;
  const fraction = unit > 0 && value < 10 ? 1 : 0;
  return `${value.toLocaleString(undefined, {
    minimumFractionDigits: fraction,
    maximumFractionDigits: fraction,
  })} ${BYTE_UNITS[unit]}`;
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
  const [actionTone, setActionTone] = useState<'info' | 'warning' | 'error'>('info');
  const [working, setWorking] = useState(false);

  const identify = async () => {
    if (onIdentify === undefined) return;
    setWorking(true);
    setActionStatus('');
    setActionTone('info');
    try {
      const result = await onIdentify();
      setActionTone(result.supported ? 'info' : 'warning');
      setActionStatus(
        result.supported
          ? t('about.identify.active', {
              time: new Date(result.activeUntil).toLocaleTimeString(),
            })
          : t('about.identify.unsupported'),
      );
    } catch {
      setActionTone('error');
      setActionStatus(t('about.identify.failed'));
    } finally {
      setWorking(false);
    }
  };

  const saveFriendlyName = async (event: FormEvent) => {
    event.preventDefault();
    if (onFriendlyName === undefined) return;
    setWorking(true);
    setActionStatus('');
    setActionTone('info');
    try {
      const updated = await onFriendlyName(friendlyName);
      setFriendlyName(updated.friendlyName);
      setActionStatus(t('about.friendly.updated'));
    } catch {
      setActionTone('error');
      setActionStatus(t('about.friendly.failed'));
    } finally {
      setWorking(false);
    }
  };

  return (
    <div className="page about-page">
      <header className="page-head about-head">
        <h2>{t('about.title')}</h2>
        {appliance !== undefined && appliance !== null && (
          <div className="about-permanent-identity">
            <span>{t('about.permanentIdentity')}</span>
            <strong className="cell-mono">{appliance.serialName}</strong>
          </div>
        )}
      </header>
      <div className="page-body">
        {appliance !== undefined &&
          appliance !== null &&
          (onFriendlyName !== undefined || onIdentify !== undefined) && (
            <section className="about-controls" aria-label={t('about.controls')}>
              {onFriendlyName !== undefined && (
                <form
                  className="about-friendly-form"
                  onSubmit={(event) => void saveFriendlyName(event)}
                >
                  <div className="field">
                    <label htmlFor="friendly-name">{t('about.friendlyName')}</label>
                    <input
                      id="friendly-name"
                      value={friendlyName}
                      onChange={(event) => setFriendlyName(event.target.value)}
                      pattern="[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?"
                    />
                  </div>
                  <button type="submit" className="button" disabled={working}>
                    {t('about.friendly.save')}
                  </button>
                </form>
              )}
              {onIdentify !== undefined && (
                <button
                  type="button"
                  className="button button-primary about-identify"
                  disabled={!canIdentify || working}
                  onClick={() => void identify()}
                >
                  {t('about.identify')}
                </button>
              )}
              {actionStatus !== '' && (
                <MessageBanner tone={actionTone}>{actionStatus}</MessageBanner>
              )}
            </section>
          )}
        {status === null && (appliance === undefined || appliance === null) ? (
          <section className="notice notice-unknown">
            <p>{t('about.unavailable')}</p>
          </section>
        ) : (
          <div className="about-dashboard">
            <section className="about-card about-information">
              <h3>{t('about.details')}</h3>
              <div className="about-groups">
                <section
                  className="about-group"
                  aria-labelledby="about-software-heading"
                >
                  <h4 id="about-software-heading">{t('about.group.software')}</h4>
                  <dl className="about-facts about-facts-software">
                    <div className="about-fact">
                      <dt>{t('about.softwareVersion')}</dt>
                      <dd className="cell-mono">
                        {status?.build.version ?? appliance?.version}
                      </dd>
                    </div>
                    {status !== null && (
                      <>
                        <div className="about-fact">
                          <dt>{t('about.apiVersion')}</dt>
                          <dd className="cell-mono">{status.apiVersion}</dd>
                        </div>
                        <div className="about-fact">
                          <dt>{t('about.integrationProtocol')}</dt>
                          <dd className="cell-mono">
                            {status.build.integrationProtocol}
                          </dd>
                        </div>
                      </>
                    )}
                  </dl>
                </section>
                {status?.system !== undefined && (
                  <section
                    className="about-group"
                    aria-labelledby="about-system-heading"
                  >
                    <h4 id="about-system-heading">{t('about.group.system')}</h4>
                    <dl className="about-facts">
                      <div className="about-fact">
                        <dt>{t('about.linuxVersion')}</dt>
                        <dd>{status.system.operatingSystem}</dd>
                      </div>
                      <div className="about-fact">
                        <dt>{t('about.kernelVersion')}</dt>
                        <dd className="cell-mono">{status.system.kernelVersion}</dd>
                      </div>
                      <div className="about-fact">
                        <dt>{t('about.architecture')}</dt>
                        <dd className="cell-mono">{status.system.architecture}</dd>
                      </div>
                      <div className="about-fact">
                        <dt>{t('about.storage')}</dt>
                        <dd>
                          {status.system.storageTotalBytes > 0
                            ? t('about.storage.value', {
                                free: formatBytes(status.system.storageFreeBytes),
                                total: formatBytes(status.system.storageTotalBytes),
                              })
                            : t('about.systemUnavailable')}
                        </dd>
                      </div>
                    </dl>
                  </section>
                )}
                {appliance !== undefined && appliance !== null && (
                  <section
                    className="about-group"
                    aria-labelledby="about-management-heading"
                  >
                    <h4 id="about-management-heading">{t('about.group.management')}</h4>
                    <dl className="about-facts">
                      <div className="about-fact">
                        <dt>{t('about.lifecycle')}</dt>
                        <dd>{appliance.state}</dd>
                      </div>
                      {status !== null && (
                        <div className="about-fact">
                          <dt>{t('about.profile')}</dt>
                          <dd className="cell-mono">{status.profile}</dd>
                        </div>
                      )}
                      {appliance.friendlyName !== '' && (
                        <div className="about-fact">
                          <dt>{t('about.friendlyName')}</dt>
                          <dd className="cell-mono">{appliance.friendlyName}</dd>
                        </div>
                      )}
                      <div className="about-fact about-fact-wide">
                        <dt>{t('about.managementUrl')}</dt>
                        <dd>
                          <a className="fact-link" href={appliance.managementUrl}>
                            {appliance.managementUrl}
                          </a>
                        </dd>
                      </div>
                      <div className="about-fact about-fact-wide">
                        <dt>{t('about.managementAddresses')}</dt>
                        <dd>
                          <ul className="about-addresses cell-mono">
                            {appliance.managementAddresses.map((address) => (
                              <li key={address}>{address}</li>
                            ))}
                          </ul>
                        </dd>
                      </div>
                    </dl>
                  </section>
                )}
              </div>
            </section>
            {appliance !== undefined &&
              appliance !== null &&
              labelUrl !== undefined && (
                <section className="about-card about-label-card">
                  <h3>{t('about.printableLabel')}</h3>
                  <a
                    className="about-label-link"
                    href={labelUrl}
                    target="_blank"
                    rel="noreferrer"
                    aria-label={t('about.openLabel')}
                  >
                    <img
                      src={labelUrl}
                      alt={t('about.label.alt', { name: appliance.serialName })}
                    />
                  </a>
                  <p>{t('about.label.hint')}</p>
                </section>
              )}
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
