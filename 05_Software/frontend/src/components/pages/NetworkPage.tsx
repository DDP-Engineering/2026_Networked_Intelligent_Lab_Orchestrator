import { MessageBanner } from '../MessageBanner';
import { useCallback, useEffect, useState } from 'react';

import type {
  NetworkOperation,
  NetworkStatistics,
  NetworkStatus,
  StatisticsPeriod,
} from '../../api/contract';
import { STATISTICS_PERIODS } from '../../api/contract';
import type { MessageKey } from '../../i18n/en';
import { useTranslate } from '../../i18n/languages';
import { TrafficChart } from '../network/TrafficChart';
import { WireGuardDiagnostics, WireGuardPanel } from '../network/WireGuardPanel';
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
import { NetworkOverview } from '../network/NetworkOverview';
import { CommissioningPanel } from '../network/CommissioningPanel';
import { ManagementAccessPanel } from '../network/ManagementAccessPanel';
import { WorkflowHelp } from '../help/WorkflowHelp';
import type {
  AdapterRoleConflict,
  ManagementAccessStatus,
} from '../../api/management-access';
import type { NetworkView } from '../network/views';
import { NetworkInterfacesTable } from '../network/NetworkInterfacesTable';

export interface NetworkPageProps {
  /** The one focused Network destination this workspace tab owns. */
  view: NetworkView;
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
  readManagementAccess?: () => Promise<ManagementAccessStatus>;
  updateManagementAccess?: (
    interfaces: string[],
    confirmCurrentInterface: boolean,
  ) => Promise<ManagementAccessStatus>;
  confirmManagementAccess?: () => Promise<ManagementAccessStatus>;
  recoverManagementAccess?: () => Promise<ManagementAccessStatus>;
  /** Ends first boot. Absent for an identity that may not manage interfaces. */
  completeCommissioning?: (
    interfaces: string[],
    confirmCurrentInterface: boolean,
  ) => Promise<ManagementAccessStatus>;
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

interface RefusalNotice {
  detail: string;
  conflict: AdapterRoleConflict | null;
}

function sameAdapter(first: string, second: string): boolean {
  return first.trim().toLowerCase() === second.trim().toLowerCase();
}

function operationNeedsObservation(state: NetworkOperation['state']): boolean {
  return (
    state === 'applying' ||
    state === 'awaiting_confirmation' ||
    state === 'rolling_back' ||
    state === 'rollback_failed'
  );
}

const PERIOD_LABEL_KEYS = {
  '15m': 'network.period.15m',
  '1h': 'network.period.1h',
  '24h': 'network.period.24h',
} as const;

const VIEW_TITLE_KEYS: Record<NetworkView, MessageKey> = {
  overview: 'nav.network.overview',
  'device-access': 'nav.network.deviceAccess',
  vpn: 'nav.network.vpn',
  diagnostics: 'nav.network.diagnostics',
};

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
  view,
  readNetworkStatus,
  readNetworkStatistics,
  previewInterfaceChange,
  acknowledgeOperation,
  confirmOperation,
  rollbackOperation,
  canManageInterfaces,
  readManagementAccess,
  updateManagementAccess,
  confirmManagementAccess,
  recoverManagementAccess,
  completeCommissioning,
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
  const [managementAccess, setManagementAccess] = useState<
    Reading<ManagementAccessStatus>
  >({ state: 'reading' });
  const [configuring, setConfiguring] = useState<NetworkInterface | null>(null);
  const [refusal, setRefusal] = useState<RefusalNotice | null>(null);
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
  const busy = running !== null && operationNeedsObservation(running.state);

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
        setRefusal((current) => {
          const conflict = current?.conflict;
          if (
            conflict?.existingRole === 'device' &&
            !value.some((entry) =>
              sameAdapter(entry.deviceInterface, conflict.adapterId),
            )
          ) {
            return null;
          }
          return current;
        });
      } catch {
        if (!abandoned()) setDeviceNetworksUnreadable(true);
      }
    },
    [readDeviceNetworks],
  );

  const reloadManagementAccess = useCallback(
    async (abandoned: () => boolean) => {
      if (readManagementAccess === undefined) return;
      try {
        const value = await readManagementAccess();
        if (!abandoned()) {
          setManagementAccess({ state: 'read', value });
          setRefusal((current) => {
            const conflict = current?.conflict;
            if (
              conflict?.existingRole === 'management' &&
              !value.interfaces.some(
                (entry) =>
                  entry.role === 'management' &&
                  sameAdapter(entry.adapterId, conflict.adapterId),
              )
            ) {
              return null;
            }
            return current;
          });
        }
      } catch {
        if (!abandoned()) setManagementAccess({ state: 'failed' });
      }
    },
    [readManagementAccess],
  );

  const refreshOverview = useCallback(() => {
    const active = () => false;
    void Promise.all([
      reloadStatus(active),
      reloadDeviceNetworks(active),
      reloadManagementAccess(active),
    ]);
  }, [reloadDeviceNetworks, reloadManagementAccess, reloadStatus]);

  const reattachRecentOperation = useCallback(
    async (
      abandoned: () => boolean,
      operationId?: string,
    ): Promise<NetworkOperation | null> => {
      if (listOperations === undefined) return null;
      try {
        const recent = await listOperations();
        if (abandoned()) return null;
        const authoritative = recent.find(
          (operation) =>
            operationNeedsObservation(operation.state) &&
            (operationId === undefined || operation.id === operationId),
        );
        if (authoritative === undefined) return null;
        setRunning((current) =>
          operationId !== undefined ||
          current === null ||
          !operationNeedsObservation(current.state)
            ? authoritative
            : current,
        );
        setOperationUnavailable(false);
        return authoritative;
      } catch {
        // The listing is a convenience; the page still works without it.
        return null;
      }
    },
    [listOperations],
  );

  // Both reads are started from inside an async wrapper, matching the
  // administration page: it puts a microtask between the effect and the first
  // state change, so no render cascades out of the effect itself.
  useEffect(() => {
    let abandoned = false;
    void (async () => {
      await reloadStatus(() => abandoned);
      await reloadDeviceNetworks(() => abandoned);
      await reloadManagementAccess(() => abandoned);
    })();
    const refresh = setInterval(() => {
      void reloadStatus(() => abandoned);
      void reloadDeviceNetworks(() => abandoned);
      void reloadManagementAccess(() => abandoned);
    }, 5000);
    return () => {
      abandoned = true;
      clearInterval(refresh);
    };
  }, [reloadStatus, reloadDeviceNetworks, reloadManagementAccess]);

  // A change that was in flight when the page was reloaded is picked up again
  // from the appliance's own record, so its window is shown where it is
  // still running rather than forgotten with the previous page.
  useEffect(() => {
    let abandoned = false;
    void (async () => {
      await reattachRecentOperation(() => abandoned);
    })();
    return () => {
      abandoned = true;
    };
  }, [reattachRecentOperation]);

  useEffect(() => {
    if (view !== 'diagnostics' || selected === null) {
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
  }, [reloadStatistics, selected, period, view]);

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
      !operationNeedsObservation(running.state)
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

  const describeRefusal = useCallback(
    (error: BenchCoreError): RefusalNotice => {
      const conflict = error.adapterRoleConflict;
      if (conflict?.existingRole === 'management') {
        return {
          detail: t('network.role.managementUnavailable', {
            name: conflict.interface || conflict.adapterId,
          }),
          conflict,
        };
      }
      if (conflict?.existingRole === 'device') {
        return {
          detail: t('network.role.deviceUnavailable', {
            name:
              conflict.conflictingResourceName ||
              t('network.role.unnamedDeviceNetwork'),
          }),
          conflict,
        };
      }
      if (error.code === 'network_interface_role_conflict') {
        return {
          detail: t('network.role.malformedConflict'),
          conflict: null,
        };
      }
      return { detail: error.detail, conflict: null };
    },
    [t],
  );

  const apply = useCallback(async () => {
    if (proposal === null) return;
    const agreed = proposal;
    setProposal(null);
    try {
      const started = await acknowledgeOperation(agreed.id, agreed.revision);
      setRunning(started);
      void reloadStatus(() => false);
    } catch (error) {
      if (error instanceof BenchCoreError && error.failure === 'rejected') {
        setRefusal(describeRefusal(error));
        setRunning(null);
        setOperationUnavailable(false);
      } else {
        const authoritative = await reattachRecentOperation(() => false, agreed.id);
        if (authoritative === null) {
          setRunning({
            ...agreed,
            state: 'failed',
            failure: t('network.change.refused'),
          });
        }
      }
    }
  }, [
    acknowledgeOperation,
    proposal,
    t,
    reloadStatus,
    describeRefusal,
    reattachRecentOperation,
  ]);

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
        setRefusal(null);
        setProposalDetail('');
        setProposal(operation);
        return operation;
      } catch (error) {
        if (error instanceof BenchCoreError && error.failure === 'rejected') {
          setRefusal(describeRefusal(error));
        } else {
          setOperationUnavailable(true);
        }
        return null;
      }
    },
    [describeRefusal],
  );

  return (
    <div className={`page network-page network-page-${view}`}>
      <header className="page-head">
        <div className="page-title-actions">
          <h2>
            {t('network.title')} / {t(VIEW_TITLE_KEYS[view])}
          </h2>
          {onOpenActivity !== undefined && (
            <button
              type="button"
              onClick={() => onOpenActivity({ component: 'network' })}
            >
              {t('network.activity.open')}
            </button>
          )}
        </div>
        <div className="form-actions">
          {view === 'overview' && canManageInterfaces && (
            <WorkflowHelp
              prefix="help.roles"
              title={t('help.roles.title')}
              buttonLabel={t('help.roles.open')}
            />
          )}
          {view === 'overview' && (
            <button type="button" onClick={refreshOverview}>
              {t('network.management.refresh')}
            </button>
          )}
        </div>
      </header>

      <div className="page-body">
        {refusal !== null && (
          <MessageBanner tone="error">
            {t('network.change.refused')} {refusal.detail}
          </MessageBanner>
        )}
        {operationUnavailable && (
          <MessageBanner tone="error">{t('network.wg.pollFailed')}</MessageBanner>
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
          <MessageBanner tone="info" onDismiss={() => setRunning(null)}>
            <span>
              {t(
                running.action === 'wireguard.install'
                  ? 'network.install.complete'
                  : 'network.change.kept',
              )}
            </span>
          </MessageBanner>
        )}
        {running !== null && running.state === 'rolled_back' && (
          <MessageBanner tone="info" onDismiss={() => setRunning(null)}>
            <span>{t('network.change.restored')}</span>
          </MessageBanner>
        )}
        {running !== null &&
          (running.state === 'rollback_failed' || running.state === 'failed') && (
            <MessageBanner tone="error">
              {t('network.change.failed')} {running.failure}
            </MessageBanner>
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

        {view === 'overview' && status.state === 'reading' && (
          <p>{t('network.reading')}</p>
        )}
        {view === 'overview' && status.state === 'failed' && (
          <MessageBanner tone="warning">{t('network.unreadable')}</MessageBanner>
        )}
        {view === 'overview' && status.state === 'read' && (
          <NetworkOverview
            interfaces={status.value.interfaces}
            deviceNetworks={deviceNetworks}
            wireGuard={wireGuardStatus}
            running={running}
          />
        )}

        {view === 'overview' &&
          status.state === 'read' &&
          (managementAccess.state === 'read' &&
          managementAccess.value.provisioning &&
          completeCommissioning !== undefined ? (
            /*
              While the appliance is provisioning the allowlist follows the
              hardware, so the ordinary editor has nothing to edit. The
              commissioning workflow stands in its place until first boot ends.
            */
            <CommissioningPanel
              status={managementAccess.value}
              canManage={canManageInterfaces}
              busy={busy}
              complete={async (interfaces, confirmCurrentInterface) => {
                const value = await completeCommissioning(
                  interfaces,
                  confirmCurrentInterface,
                );
                setManagementAccess({ state: 'read', value });
                return value;
              }}
            />
          ) : canManageInterfaces &&
            readManagementAccess !== undefined &&
            updateManagementAccess !== undefined &&
            confirmManagementAccess !== undefined &&
            recoverManagementAccess !== undefined ? (
            <ManagementAccessPanel
              read={readManagementAccess}
              observedStatus={
                managementAccess.state === 'read' ? managementAccess.value : null
              }
              observedReadState={managementAccess.state}
              onStatusChange={(value) => setManagementAccess({ state: 'read', value })}
              update={updateManagementAccess}
              confirm={confirmManagementAccess}
              recover={recoverManagementAccess}
              interfaces={status.value.interfaces}
              deviceNetworks={deviceNetworks}
              busy={busy}
              onToggle={
                canManageInterfaces
                  ? (entry) =>
                      void propose(
                        entry.id,
                        entry.state === 'connected' ? 'disable' : 'enable',
                      )
                  : undefined
              }
              onConfigure={
                canManageInterfaces && previewInterfaceSettings !== undefined
                  ? setConfiguring
                  : undefined
              }
            />
          ) : (
            <NetworkInterfacesTable
              interfaces={status.value.interfaces}
              deviceNetworks={deviceNetworks}
              busy={busy}
              onToggle={
                canManageInterfaces
                  ? (entry) =>
                      void propose(
                        entry.id,
                        entry.state === 'connected' ? 'disable' : 'enable',
                      )
                  : undefined
              }
              onConfigure={
                canManageInterfaces && previewInterfaceSettings !== undefined
                  ? setConfiguring
                  : undefined
              }
            />
          ))}

        {view === 'overview' &&
          configuring !== null &&
          previewInterfaceSettings !== undefined && (
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

        {view === 'device-access' && readDeviceNetworks !== undefined && (
          <DeviceNetworksPanel
            networks={deviceNetworks}
            interfaces={status.state === 'read' ? status.value.interfaces : []}
            profiles={wireGuardStatus?.profiles ?? []}
            managementInterfaces={
              managementAccess.state === 'read' ? managementAccess.value.interfaces : []
            }
            managementRoleState={
              readManagementAccess === undefined ? 'read' : managementAccess.state
            }
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

        {view === 'vpn' && (
          <WireGuardPanel
            busy={busy}
            status={wireGuardStatus}
            canManage={canManageVPN && !busy}
            canInstall={canInstallVPN}
            showDiagnostics={false}
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
        )}

        {view === 'diagnostics' && (
          <>
            <WireGuardDiagnostics status={wireGuardStatus} collapsible={false} />
            <section
              className="network-statistics"
              aria-labelledby="network-statistics"
            >
              <div className="network-statistics-head">
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
              </div>

              {statistics.state === 'reading' && <p>{t('network.reading')}</p>}
              {statistics.state === 'failed' && (
                <MessageBanner tone="warning">
                  {t('network.statistics.unreadable')}
                </MessageBanner>
              )}
              {statistics.state === 'read' && (
                <>
                  {statistics.value.stale && (
                    <MessageBanner tone="warning">
                      {t('network.statistics.stale')}
                    </MessageBanner>
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
          </>
        )}

        {!benchCoreReachable && (
          <MessageBanner tone="error">
            {t('network.reachability.unreachable')}
          </MessageBanner>
        )}
      </div>
    </div>
  );
}
