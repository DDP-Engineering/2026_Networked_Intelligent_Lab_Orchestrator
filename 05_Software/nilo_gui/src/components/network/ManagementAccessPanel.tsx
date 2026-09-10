import { useCallback, useEffect, useMemo, useState } from 'react';

import type { ManagementAccessStatus } from '../../api/management-access';
import type { MessageKey } from '../../i18n/en';
import { useTranslate } from '../../i18n/languages';

export interface ManagementAccessPanelProps {
  read: () => Promise<ManagementAccessStatus>;
  update: (
    interfaces: string[],
    confirmCurrentInterface: boolean,
  ) => Promise<ManagementAccessStatus>;
  confirm: () => Promise<ManagementAccessStatus>;
  recover: () => Promise<ManagementAccessStatus>;
}

const ACCESS_STATE_KEYS: Record<ManagementAccessStatus['state'], MessageKey> = {
  pending: 'network.management.state.pending',
  applied: 'network.management.state.applied',
  failed: 'network.management.state.failed',
  recovered: 'network.management.state.recovered',
};

const INTERFACE_STATE_KEYS: Record<
  ManagementAccessStatus['interfaces'][number]['state'],
  MessageKey
> = {
  connected: 'network.state.connected',
  disconnected: 'network.state.disconnected',
  unavailable: 'network.state.unavailable',
};

/** Administrator-only management listener configuration and recovery. */
export function ManagementAccessPanel({
  read,
  update,
  confirm,
  recover,
}: ManagementAccessPanelProps) {
  const t = useTranslate();
  const [status, setStatus] = useState<ManagementAccessStatus | null>(null);
  const [selected, setSelected] = useState<string[]>([]);
  const [working, setWorking] = useState(false);
  const [failure, setFailure] = useState('');
  const [warnCurrent, setWarnCurrent] = useState(false);

  const load = useCallback(async () => {
    try {
      const value = await read();
      setStatus(value);
      setSelected(value.configuredInterfaces);
      setFailure('');
    } catch (error) {
      setFailure(error instanceof Error ? error.message : String(error));
    }
  }, [read]);

  useEffect(() => {
    let abandoned = false;
    const refresh = async () => {
      try {
        const value = await read();
        if (!abandoned) {
          setStatus(value);
          setSelected((current) =>
            current.length === 0 ? value.configuredInterfaces : current,
          );
          setFailure('');
        }
      } catch (error) {
        if (!abandoned)
          setFailure(error instanceof Error ? error.message : String(error));
      }
    };
    void refresh();
    const timer = setInterval(() => void refresh(), 5000);
    return () => {
      abandoned = true;
      clearInterval(timer);
    };
  }, [read]);

  const usableSelected = useMemo(
    () =>
      status?.interfaces.some(
        (entry) => entry.usable && selected.includes(entry.name),
      ) ?? false,
    [selected, status],
  );

  const apply = useCallback(
    async (confirmCurrentInterface: boolean) => {
      setWorking(true);
      setFailure('');
      try {
        const value = await update(selected, confirmCurrentInterface);
        setStatus(value);
        setSelected(value.configuredInterfaces);
        setWarnCurrent(false);
      } catch (error) {
        setFailure(error instanceof Error ? error.message : String(error));
      } finally {
        setWorking(false);
      }
    },
    [selected, update],
  );

  const save = () => {
    if (status?.currentInterface && !selected.includes(status.currentInterface)) {
      setWarnCurrent(true);
      return;
    }
    void apply(false);
  };

  const settle = async (operation: () => Promise<ManagementAccessStatus>) => {
    setWorking(true);
    setFailure('');
    try {
      const value = await operation();
      setStatus(value);
      setSelected(value.configuredInterfaces);
    } catch (error) {
      setFailure(error instanceof Error ? error.message : String(error));
    } finally {
      setWorking(false);
    }
  };

  return (
    <section aria-labelledby="management-access-title" className="management-access">
      <div className="network-section-head">
        <div>
          <h3 id="management-access-title">{t('network.management.title')}</h3>
          <p>{t('network.management.body')}</p>
        </div>
        <button type="button" onClick={() => void load()} disabled={working}>
          {t('network.management.refresh')}
        </button>
      </div>

      {failure !== '' && (
        <p className="notice notice-unknown" role="alert">
          {failure}
        </p>
      )}
      {status === null ? (
        <p>{t('network.reading')}</p>
      ) : (
        <>
          <p
            className={`notice ${status.state === 'failed' ? 'notice-unknown' : ''}`}
            role="status"
          >
            {t(ACCESS_STATE_KEYS[status.state])}
            {status.detail === '' ? '' : ` ${status.detail}`}
          </p>
          {status.tlsMissing.length > 0 && (
            <p className="notice notice-unknown" role="alert">
              {t('network.management.tlsMissing', {
                names: status.tlsMissing.join(', '),
              })}
            </p>
          )}
          <div className="table-scroll">
            <table className="data-table record-table">
              <thead>
                <tr>
                  <th scope="col">{t('network.management.enabled')}</th>
                  <th scope="col">{t('network.table.interface')}</th>
                  <th scope="col">{t('network.table.state')}</th>
                  <th scope="col">{t('network.table.address')}</th>
                </tr>
              </thead>
              <tbody>
                {status.interfaces.map((entry) => (
                  <tr key={entry.name}>
                    <td>
                      <input
                        type="checkbox"
                        aria-label={t('network.management.toggle', {
                          name: entry.name,
                        })}
                        checked={selected.includes(entry.name)}
                        disabled={working || entry.kind === 'loopback'}
                        onChange={(event) =>
                          setSelected((current) =>
                            event.target.checked
                              ? [...current, entry.name]
                              : current.filter((name) => name !== entry.name),
                          )
                        }
                      />
                    </td>
                    <th scope="row">
                      {entry.name}
                      <small className="network-secondary">
                        {entry.kind}
                        {entry.current ? ` · ${t('network.management.current')}` : ''}
                      </small>
                    </th>
                    <td>{t(INTERFACE_STATE_KEYS[entry.state])}</td>
                    <td className="network-address">
                      {[...entry.ipv4, ...entry.ipv6].join(', ') ||
                        t('network.notReported')}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          {!usableSelected && (
            <p className="notice notice-unknown" role="alert">
              {t('network.management.noUsable')}
            </p>
          )}
          <div className="form-actions">
            <button type="button" onClick={save} disabled={working || !usableSelected}>
              {t('network.management.apply')}
            </button>
          </div>
          {warnCurrent && (
            <div
              className="notice notice-unknown"
              role="alertdialog"
              aria-labelledby="management-current-warning"
            >
              <h4 id="management-current-warning">
                {t('network.management.currentWarning')}
              </h4>
              <p>
                {t('network.management.currentWarningBody', {
                  name: status.currentInterface,
                })}
              </p>
              <button type="button" onClick={() => void apply(true)} disabled={working}>
                {t('network.management.currentApply')}
              </button>
              <button
                type="button"
                onClick={() => setWarnCurrent(false)}
                disabled={working}
              >
                {t('network.change.cancel')}
              </button>
            </div>
          )}
          {status.state === 'pending' && (
            <div className="notice" role="status">
              <p>{t('network.management.pendingHelp')}</p>
              <button
                type="button"
                onClick={() => void settle(confirm)}
                disabled={working}
              >
                {t('network.change.keep')}
              </button>
              <button
                type="button"
                onClick={() => void settle(recover)}
                disabled={working}
              >
                {t('network.change.restore')}
              </button>
            </div>
          )}
          <h4>{t('network.management.urls')}</h4>
          <ul>
            {status.urls.map((entry) => (
              <li key={entry.url}>
                <a href={entry.url}>{entry.url}</a> · {entry.kind}
              </li>
            ))}
          </ul>
          <p>{t('network.management.localOnly')}</p>
          <p>{t('network.management.dhcp')}</p>
        </>
      )}
    </section>
  );
}
