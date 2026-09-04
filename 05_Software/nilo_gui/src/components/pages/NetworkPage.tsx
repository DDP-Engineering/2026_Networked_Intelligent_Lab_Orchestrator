import { useCallback, useEffect, useState } from 'react';

import type {
  NetworkOperation,
  NetworkStatistics,
  NetworkStatus,
  StatisticsPeriod,
} from '../../api/contract';
import { STATISTICS_PERIODS } from '../../api/contract';
import { useTranslate } from '../../i18n/languages';
import { TrafficChart } from '../network/TrafficChart';
import { WireGuardPanel } from '../network/WireGuardPanel';
import { NetworkConfirmation } from '../network/NetworkConfirmation';
import { InstallationProgress } from '../network/InstallationProgress';
import type { WireGuardChange, WireGuardStatus } from '../../api/wireguard';
import type { ActivityFilter } from '../../api/activity';
import type {
  AccessInstructions,
  DeviceNetwork,
  DeviceNetworkChange,
  DeviceNetworkCheck,
} from '../../api/devicenetworks';
import type {
  InterfaceSettingsDraft,
  WiFiJoinDraft,
  WiFiScan,
} from '../../api/settings';
import type { NetworkInterface } from '../../api/contract';
import { BenchCoreError } from '../../api/client';
import { DeviceNetworksPanel } from '../network/DeviceNetworksPanel';
import { InterfaceSettingsDialog } from '../network/InterfaceSettingsDialog';

export interface NetworkPageProps {
  /** Reads the appliance's current network state. */
  readNetworkStatus: () => Promise<NetworkStatus>;
  /** Reads one interface's traffic over one window. */
  readNetworkStatistics: (
    interfaceId: string,
    period: StatisticsPeriod,
  ) => Promise<NetworkStatistics>;
  /** Proposes a change, returning what would happen. It changes nothing. */
  previewInterfaceChange: (
    interfaceId: string,
    action: 'enable' | 'disable',
  ) => Promise<NetworkOperation>;
  /** Agrees to exactly the previewed revision, which runs the change. */
  acknowledgeOperation: (id: string, revision: string) => Promise<NetworkOperation>;
  /** Keeps a change that is inside its confirmation window. */
  confirmOperation: (id: string) => Promise<NetworkOperation>;
  /** Puts a change back before the window runs out. */
  rollbackOperation: (id: string) => Promise<NetworkOperation>;
  /**
   * Whether this identity may change interfaces.
   *
   * Hiding a control is presentation, never a permission: Bench Core authorizes
   * every request on its own, so an identity that reaches the route another way
   * is refused there. This only avoids offering somebody a button they cannot use.
   */
  canManageInterfaces: boolean;
  /** Whether Bench Core answered this interface over HTTP. */
  benchCoreReachable: boolean;
  wireGuardStatus?: WireGuardStatus | null;
  canManageVPN?: boolean;
  canInstallVPN?: boolean;
  previewWireGuardChange?: (change: WireGuardChange) => Promise<NetworkOperation>;
  readOperation?: (id: string) => Promise<NetworkOperation>;
  /** Recent operations, so a change in flight is picked up again after a reload. */
  listOperations?: () => Promise<NetworkOperation[]>;
  readDeviceNetworks?: () => Promise<DeviceNetwork[]>;
  previewDeviceNetworkChange?: (
    change: DeviceNetworkChange,
  ) => Promise<NetworkOperation>;
  checkDeviceNetwork?: (id: string, tcpPort: number) => Promise<DeviceNetworkCheck>;
  readAccessInstructions?: (id: string) => Promise<AccessInstructions>;
  previewInterfaceSettings?: (
    interfaceId: string,
    settings: InterfaceSettingsDraft,
  ) => Promise<NetworkOperation>;
  scanWiFi?: (interfaceId: string) => Promise<WiFiScan>;
  previewWiFiJoin?: (
    interfaceId: string,
    join: WiFiJoinDraft,
  ) => Promise<NetworkOperation>;
  /** Opens the shared Activity page with filters already narrowed. */
  onOpenActivity?: (filter: ActivityFilter) => void;
}

/** What a read produced: still running, a value, or a failure. */
type Reading<T> =
  { state: 'reading' } | { state: 'read'; value: T } | { state: 'failed' };

const PERIOD_LABEL_KEYS = {
  '15m': 'network.period.15m',
  '1h': 'network.period.1h',
  '24h': 'network.period.24h',
} as const;

/** Bytes per second, rendered at a scale an operator reads at a glance. */
function formatRate(bytesPerSecond: number): string {
  if (bytesPerSecond <= 0) {
    return '0 B/s';
  }
  const units = ['B/s', 'kB/s', 'MB/s', 'GB/s'];
  let value = bytesPerSecond;
  let unit = 0;
  while (value >= 1000 && unit < units.length - 1) {
    value /= 1000;
    unit += 1;
  }
  return `${value >= 100 || unit === 0 ? Math.round(value) : value.toFixed(1)} ${units[unit]}`;
}

/**
 * The Network page.
 *
 * Three areas, in the order an operator reads them: what the appliance's
 * interfaces are doing, whether a tunnel is available, and how much traffic has
 * crossed a chosen interface.
 *
 * The shell identifies the runtime once. Missing observations stay missing;
 * a window with no traffic observations is never drawn as zero.
 */
export function NetworkPage({
  readNetworkStatus,
  readNetworkStatistics,
  previewInterfaceChange,
  acknowledgeOperation,
  confirmOperation,
  rollbackOperation,
  canManageInterfaces,
  benchCoreReachable,
  wireGuardStatus = null,
  canManageVPN = false,
  canInstallVPN = false,
  previewWireGuardChange,
  readOperation,
  listOperations,
  readDeviceNetworks,
  previewDeviceNetworkChange,
  checkDeviceNetwork,
  readAccessInstructions,
  previewInterfaceSettings,
  scanWiFi,
  previewWiFiJoin,
  onOpenActivity,
}: NetworkPageProps) {
  const t = useTranslate();
  const [deviceNetworks, setDeviceNetworks] = useState<DeviceNetwork[] | null>(null);
  const [deviceNetworksUnreadable, setDeviceNetworksUnreadable] = useState(false);
  const [configuring, setConfiguring] = useState<NetworkInterface | null>(null);
  const [refusal, setRefusal] = useState('');
  const [proposal, setProposal] = useState<NetworkOperation | null>(null);
  const [running, setRunning] = useState<NetworkOperation | null>(null);
  const [remaining, setRemaining] = useState(0);
  const [status, setStatus] = useState<Reading<NetworkStatus>>({ state: 'reading' });
  const [statistics, setStatistics] = useState<Reading<NetworkStatistics>>({
    state: 'reading',
  });
  const [period, setPeriod] = useState<StatisticsPeriod>('15m');
  const [selected, setSelected] = useState<string | null>(null);
  const [operationUnavailable, setOperationUnavailable] = useState(false);
  const [proposalDetail, setProposalDetail] = useState('');
  const busy =
    running !== null &&
    ['applying', 'awaiting_confirmation', 'rolling_back', 'rollback_failed'].includes(
      running.state,
    );

  const reloadStatus = useCallback(
    async (abandoned: () => boolean) => {
      try {
        const value = await readNetworkStatus();
        if (abandoned()) return;
        setStatus({ state: 'read', value });
        // Open the chart on a device that is actually a connection, rather than
        // on loopback, which carries nothing worth looking at.
        const first = value.interfaces.find((entry) => entry.controllable);
        setSelected((current) => current ?? first?.id ?? null);
      } catch {
        if (!abandoned()) setStatus({ state: 'failed' });
      }
    },
    [readNetworkStatus],
  );

  const reloadStatistics = useCallback(
    async (interfaceId: string, window: StatisticsPeriod, abandoned: () => boolean) => {
      try {
        const value = await readNetworkStatistics(interfaceId, window);
        if (!abandoned()) setStatistics({ state: 'read', value });
      } catch {
        if (!abandoned()) setStatistics({ state: 'failed' });
      }
    },
    [readNetworkStatistics],
  );

  const reloadDeviceNetworks = useCallback(
    async (abandoned: () => boolean) => {
      if (readDeviceNetworks === undefined) return;
      try {
        const value = await readDeviceNetworks();
        if (abandoned()) return;
        setDeviceNetworks(value);
        setDeviceNetworksUnreadable(false);
      } catch {
        if (!abandoned()) setDeviceNetworksUnreadable(true);
      }
    },
    [readDeviceNetworks],
  );

  // Both reads are started from inside an async wrapper, matching the
  // administration page: it puts a microtask between the effect and the first
  // state change, so no render cascades out of the effect itself.
  useEffect(() => {
    let abandoned = false;
    void (async () => {
      await reloadStatus(() => abandoned);
      await reloadDeviceNetworks(() => abandoned);
    })();
    const refresh = setInterval(() => {
      void reloadStatus(() => abandoned);
      void reloadDeviceNetworks(() => abandoned);
    }, 5000);
    return () => {
      abandoned = true;
      clearInterval(refresh);
    };
  }, [reloadStatus, reloadDeviceNetworks]);

  // A change that was in flight when the page was reloaded is picked up again
  // from the appliance's own record, so its window is shown where it is
  // still running rather than forgotten with the previous page.
  useEffect(() => {
    if (listOperations === undefined) return;
    let abandoned = false;
    void (async () => {
      try {
        const recent = await listOperations();
        if (abandoned) return;
        const inFlight = recent.find((operation) =>
          ['applying', 'awaiting_confirmation', 'rolling_back'].includes(
            operation.state,
          ),
        );
        if (inFlight !== undefined) {
          setRunning((current) => current ?? inFlight);
        }
      } catch {
        // The listing is a convenience; the page still works without it.
      }
    })();
    return () => {
      abandoned = true;
    };
  }, [listOperations]);

  useEffect(() => {
    if (selected === null) {
      return;
    }
    let abandoned = false;
    void (async () => {
      await reloadStatistics(selected, period, () => abandoned);
    })();
    const refresh = setInterval(
      () => void reloadStatistics(selected, period, () => abandoned),
      60000,
    );
    return () => {
      abandoned = true;
      clearInterval(refresh);
    };
  }, [reloadStatistics, selected, period]);

  // The countdown is driven from the deadline the appliance sent, not from a
  // number this page counts down on its own: the window belongs to the
  // appliance, and a browser clock drifting would misreport how long is left.
  useEffect(() => {
    if (running === null || running.confirmDeadline === null) {
      return;
    }
    const deadline = running.confirmDeadline.getTime();
    const tick = () =>
      setRemaining(Math.max(0, Math.ceil((deadline - Date.now()) / 1000)));
    tick();
    const timer = setInterval(tick, 1000);
    return () => clearInterval(timer);
  }, [running]);

  useEffect(() => {
    if (
      running === null ||
      readOperation === undefined ||
      !['applying', 'awaiting_confirmation', 'rolling_back'].includes(running.state)
    )
      return;
    let abandoned = false;
    const refresh = async () => {
      try {
        const value = await readOperation(running.id);
        if (!abandoned) {
          setRunning(value);
          setOperationUnavailable(false);
        }
      } catch {
        if (!abandoned) setOperationUnavailable(true);
      }
    };
    const timer = setInterval(() => void refresh(), 1000);
    return () => {
      abandoned = true;
      clearInterval(timer);
    };
  }, [running, readOperation]);

  const propose = useCallback(
    async (interfaceId: string, action: 'enable' | 'disable') => {
      try {
        setProposalDetail('');
        setProposal(await previewInterfaceChange(interfaceId, action));
      } catch {
        setProposal(null);
        setOperationUnavailable(true);
      }
    },
    [previewInterfaceChange],
  );

  const apply = useCallback(async () => {
    if (proposal === null) return;
    const agreed = proposal;
    setProposal(null);
    try {
      const started = await acknowledgeOperation(agreed.id, agreed.revision);
      setRunning(started);
      void reloadStatus(() => false);
    } catch {
      setRunning({ ...agreed, state: 'failed', failure: t('network.change.refused') });
    }
  }, [acknowledgeOperation, proposal, t, reloadStatus]);

  const settle = useCallback(
    async (resolve: (id: string) => Promise<NetworkOperation>, id: string) => {
      try {
        setRunning(await resolve(id));
        void reloadStatus(() => false);
        void reloadDeviceNetworks(() => false);
      } catch {
        setOperationUnavailable(true);
      }
    },
    [reloadStatus, reloadDeviceNetworks],
  );

  // A preview the appliance refused is shown with its reason: a refusal is an
  // answer, not an outage, and the operator can change the proposal.
  const proposeOrRefuse = useCallback(
    async (preview: Promise<NetworkOperation>): Promise<NetworkOperation | null> => {
      try {
        const operation = await preview;
        setRefusal('');
        setProposalDetail('');
        setProposal(operation);
        return operation;
      } catch (error) {
        if (error instanceof BenchCoreError && error.failure === 'rejected') {
          setRefusal(error.detail);
        } else {
          setOperationUnavailable(true);
        }
        return null;
      }
    },
    [],
  );

  return (
    <div className="page network-page">
      <header className="page-head">
        <h2>{t('network.title')}</h2>
        {onOpenActivity !== undefined && (
          <button
            type="button"
            onClick={() => onOpenActivity({ component: 'network' })}
          >
            {t('network.activity.open')}
          </button>
        )}
      </header>

      <div className="page-body">
        {refusal !== '' && (
          <p role="alert" className="notice notice-unknown">
            {t('network.change.refused')} {refusal}
          </p>
        )}
        {operationUnavailable && (
          <p role="alert" className="notice notice-unknown">
            {t('network.wg.pollFailed')}
          </p>
        )}
        {/*
          The change in flight, above everything else. An administrator who has
          just cut their own access needs the way back where they are looking,
          not below a table they have to scroll past.
        */}
        {running !== null && running.state === 'awaiting_confirmation' && (
          <section className="notice" aria-labelledby="network-countdown">
            <h3 id="network-countdown">{t('network.change.awaiting.title')}</h3>
            <p>
              {t('network.change.awaiting.body')
                .replace('{action}', running.action)
                .replace('{target}', running.target)}
            </p>
            <p role="timer" aria-live="polite">
              {t('network.change.remaining').replace('{seconds}', String(remaining))}
            </p>
            <button
              type="button"
              onClick={() => void settle(confirmOperation, running.id)}
            >
              {t('network.change.keep')}
            </button>
            <button
              type="button"
              onClick={() => void settle(rollbackOperation, running.id)}
            >
              {t('network.change.restore')}
            </button>
          </section>
        )}

        {running?.state === 'applying' && running.action === 'wireguard.install' && (
          <InstallationProgress progress={running.progress} />
        )}
        {running !== null && running.state === 'confirmed' && (
          <p className="notice network-notice" role="status">
            <span>
              {t(
                running.action === 'wireguard.install'
                  ? 'network.install.complete'
                  : 'network.change.kept',
              )}
            </span>
            <button type="button" onClick={() => setRunning(null)}>
              {t('network.dismiss')}
            </button>
          </p>
        )}
        {running !== null && running.state === 'rolled_back' && (
          <p className="notice network-notice" role="status">
            <span>{t('network.change.restored')}</span>
            <button type="button" onClick={() => setRunning(null)}>
              {t('network.dismiss')}
            </button>
          </p>
        )}
        {running !== null &&
          (running.state === 'rollback_failed' || running.state === 'failed') && (
            <p className="notice notice-unknown" role="alert">
              {t('network.change.failed')} {running.failure}
            </p>
          )}

        {proposal !== null && (
          <NetworkConfirmation onCancel={() => setProposal(null)}>
            {proposalDetail !== '' && <p>{proposalDetail}</p>}
            <h3 id="network-proposal">{t('network.change.confirm.title')}</h3>
            {/* The exact action and target, spelled out, because agreement to a
                summary is not agreement to what will happen. */}
            <p>
              {t('network.change.confirm.body')
                .replace('{action}', proposal.action)
                .replace('{target}', proposal.target)}
            </p>
            {[
              'interface.enable',
              'interface.disable',
              'interface.settings',
              'wifi.join',
              'vpn.activate',
              'vpn.deactivate',
              'device_network.update',
              'device_network.enable',
              'device_network.disable',
              'device_network.delete',
            ].includes(proposal.action) && <p>{t('network.change.confirm.window')}</p>}
            <button type="button" onClick={() => void apply()}>
              {t('network.change.apply')}
            </button>
            <button type="button" onClick={() => setProposal(null)}>
              {t('network.change.cancel')}
            </button>
          </NetworkConfirmation>
        )}

        <section aria-labelledby="network-interfaces">
          <h3 id="network-interfaces">{t('network.interfaces.title')}</h3>

          {status.state === 'reading' && <p>{t('network.reading')}</p>}
          {status.state === 'failed' && (
            <p className="notice notice-unknown">{t('network.unreadable')}</p>
          )}
          {status.state === 'read' && (
            <>
              {status.value.interfaces.length === 0 ? (
                <p>{t('network.noInterfaces')}</p>
              ) : (
                <div className="table-scroll">
                  <table className="data-table">
                    <thead>
                      <tr>
                        <th scope="col">{t('network.table.interface')}</th>
                        <th scope="col">{t('network.table.state')}</th>
                        <th scope="col">{t('network.table.address')}</th>
                        <th scope="col">{t('network.table.gateway')}</th>
                        <th scope="col">{t('network.table.link')}</th>
                        <th scope="col">{t('network.table.traffic')}</th>
                        {canManageInterfaces && (
                          <th scope="col">{t('network.table.actions')}</th>
                        )}
                      </tr>
                    </thead>
                    <tbody>
                      {status.value.interfaces.map((entry) => (
                        <tr key={entry.id}>
                          <th
                            scope="row"
                            title={entry.notControllableReason || undefined}
                          >
                            {entry.name}
                            <small className="network-secondary">{entry.kind}</small>
                          </th>
                          <td>
                            <span className="network-state">
                              <span
                                className={`state-dot ${entry.state === 'connected' ? 'state-ok' : 'state-unknown'}`}
                                aria-hidden="true"
                              />
                              {entry.state === 'connected'
                                ? t('network.state.connected')
                                : entry.state === 'disconnected'
                                  ? t('network.state.disconnected')
                                  : t('network.state.unavailable')}
                            </span>
                          </td>
                          <td>
                            {[...entry.ipv4, ...entry.ipv6].length === 0 ? (
                              <span className="cell-unknown">
                                <span title={t('network.notReported')}>—</span>
                              </span>
                            ) : (
                              [...entry.ipv4, ...entry.ipv6].map((address) => (
                                <div key={address}>{address}</div>
                              ))
                            )}
                          </td>
                          <td className="network-address">
                            {[entry.gateway4, entry.gateway6].filter(Boolean).length ===
                            0 ? (
                              <span title={t('network.notReported')}>—</span>
                            ) : (
                              [entry.gateway4, entry.gateway6]
                                .filter(Boolean)
                                .map((gateway) => <div key={gateway}>{gateway}</div>)
                            )}
                          </td>
                          <td>
                            {entry.linkCapacityMbps > 0 ? (
                              `${entry.linkCapacityMbps} Mbit/s`
                            ) : (
                              <span className="cell-unknown">
                                <span title={t('network.notReported')}>—</span>
                              </span>
                            )}
                          </td>
                          <td className="network-rates">
                            <div title={t('network.table.rx')}>
                              ↓ {formatRate(entry.rxBytesPerSecond)}
                            </div>
                            <div title={t('network.table.tx')}>
                              ↑ {formatRate(entry.txBytesPerSecond)}
                            </div>
                          </td>
                          {canManageInterfaces && (
                            <td>
                              {entry.controllable && (
                                <button
                                  type="button"
                                  aria-label={t(
                                    entry.state === 'connected'
                                      ? 'network.action.disable'
                                      : 'network.action.enable',
                                  ).replace('{name}', entry.name)}
                                  disabled={
                                    running !== null &&
                                    [
                                      'applying',
                                      'awaiting_confirmation',
                                      'rolling_back',
                                      'rollback_failed',
                                    ].includes(running.state)
                                  }
                                  onClick={() => {
                                    void propose(
                                      entry.id,
                                      entry.state === 'connected'
                                        ? 'disable'
                                        : 'enable',
                                    );
                                  }}
                                >
                                  {t(
                                    entry.state === 'connected'
                                      ? 'network.wg.disable'
                                      : 'network.wg.enable',
                                  )}
                                </button>
                              )}
                              {entry.controllable &&
                                entry.adapterId !== '' &&
                                previewInterfaceSettings !== undefined && (
                                  <button
                                    type="button"
                                    aria-label={`${t('network.settings.configure')} ${entry.name}`}
                                    disabled={busy}
                                    onClick={() => setConfiguring(entry)}
                                  >
                                    {t('network.settings.configure')}
                                  </button>
                                )}
                              {entry.settings !== null && entry.settings.saved && (
                                <small className="network-secondary">
                                  {t('network.settings.saved').replace(
                                    '{method}',
                                    entry.settings.method,
                                  )}
                                  {!entry.settings.applied &&
                                    ` (${t('network.settings.notApplied')})`}
                                </small>
                              )}
                            </td>
                          )}
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
            </>
          )}
        </section>

        {configuring !== null && previewInterfaceSettings !== undefined && (
          <InterfaceSettingsDialog
            target={configuring}
            busy={busy}
            previewSettings={(interfaceId, settings) =>
              proposeOrRefuse(previewInterfaceSettings(interfaceId, settings))
            }
            scanWiFi={scanWiFi}
            previewJoin={
              previewWiFiJoin === undefined
                ? undefined
                : (interfaceId, join) =>
                    proposeOrRefuse(previewWiFiJoin(interfaceId, join))
            }
            onDismiss={() => setConfiguring(null)}
          />
        )}

        {readDeviceNetworks !== undefined && (
          <DeviceNetworksPanel
            networks={deviceNetworks}
            interfaces={status.state === 'read' ? status.value.interfaces : []}
            profiles={wireGuardStatus?.profiles ?? []}
            canManage={canManageInterfaces && previewDeviceNetworkChange !== undefined}
            busy={busy}
            unreadable={deviceNetworksUnreadable}
            propose={(change) =>
              previewDeviceNetworkChange === undefined
                ? Promise.resolve(null)
                : proposeOrRefuse(previewDeviceNetworkChange(change))
            }
            check={(id, tcpPort) =>
              checkDeviceNetwork === undefined
                ? Promise.reject(new Error('no check'))
                : checkDeviceNetwork(id, tcpPort)
            }
            instructions={(id) =>
              readAccessInstructions === undefined
                ? Promise.reject(new Error('no instructions'))
                : readAccessInstructions(id)
            }
            openActivity={
              onOpenActivity === undefined
                ? undefined
                : (networkId) =>
                    onOpenActivity({
                      component: 'network.device_networks',
                      target: networkId,
                    })
            }
          />
        )}

        <WireGuardPanel
          busy={busy}
          status={wireGuardStatus}
          canManage={canManageVPN && !busy}
          canInstall={canInstallVPN}
          propose={(change) => {
            if (previewWireGuardChange === undefined) return;
            setProposalDetail(
              change.action === 'autostart'
                ? `${t('network.wg.startup')}: ${t(change.enabled ? 'network.wg.enabled' : 'network.wg.disabled')}`
                : '',
            );
            void previewWireGuardChange(change)
              .then(setProposal)
              .catch(() => setOperationUnavailable(true));
          }}
        />

        <section aria-labelledby="network-statistics">
          <h3 id="network-statistics">{t('network.statistics.title')}</h3>

          <div className="network-toolbar">
            {/* Which interface the chart is about is the operator's choice, not
              whichever device happened to be listed first. */}
            {status.state === 'read' && status.value.interfaces.length > 0 && (
              <p>
                <label htmlFor="network-chart-interface">
                  {t('network.chart.interface')}
                </label>{' '}
                <select
                  id="network-chart-interface"
                  value={selected ?? ''}
                  onChange={(event) => setSelected(event.target.value)}
                >
                  {status.value.interfaces.map((entry) => (
                    <option key={entry.id} value={entry.id}>
                      {entry.name}
                    </option>
                  ))}
                </select>
              </p>
            )}

            <div
              className="period-group"
              role="group"
              aria-label={t('network.period.label')}
            >
              {STATISTICS_PERIODS.map((candidate) => (
                <button
                  key={candidate}
                  type="button"
                  aria-pressed={candidate === period}
                  onClick={() => setPeriod(candidate)}
                >
                  {t(PERIOD_LABEL_KEYS[candidate])}
                </button>
              ))}
            </div>
          </div>

          {statistics.state === 'reading' && <p>{t('network.reading')}</p>}
          {statistics.state === 'failed' && (
            <p className="notice notice-unknown">
              {t('network.statistics.unreadable')}
            </p>
          )}
          {statistics.state === 'read' && (
            <>
              {statistics.value.stale && (
                <p className="notice notice-unknown">{t('network.statistics.stale')}</p>
              )}
              {statistics.value.points.length === 0 ? (
                <p>{t('network.statistics.empty')}</p>
              ) : (
                <>
                  <TrafficChart statistics={statistics.value} />
                </>
              )}
            </>
          )}
        </section>

        {!benchCoreReachable && (
          <p className="notice notice-unknown" role="alert">
            {t('network.reachability.unreachable')}
          </p>
        )}
      </div>
    </div>
  );
}
