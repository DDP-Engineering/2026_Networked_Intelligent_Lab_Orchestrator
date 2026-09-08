import { useCallback, useMemo, useEffect, useRef, useState } from 'react';

import type { BenchCoreClient, Credential } from '../../api/client';
import type {
  BenchCoreSession,
  BenchCoreStatus,
  DeviceInventory,
  IntegrationInventory,
  StatisticsPeriod,
  NetworkStatus,
} from '../../api/contract';
import {
  PERMISSION_ACTIVITY_EXPORT,
  PERMISSION_ACTIVITY_RETENTION_MANAGE,
  PERMISSION_NETWORK_MANAGE,
  PERMISSION_SYSTEM_POWER_MANAGE,
  PERMISSION_VPN_MANAGE,
  PERMISSION_VPN_INSTALL,
} from '../../api/contract';
import type { PowerAction } from '../../api/contract';
import type { ActivityFilter } from '../../api/activity';
import type { DeviceNetworkChange } from '../../api/devicenetworks';
import type { InterfaceSettingsDraft, WiFiJoinDraft } from '../../api/settings';
import type { WireGuardStatus, WireGuardChange } from '../../api/wireguard';
import type { Resource } from '../../api/resource';
import {
  mayManageTokens,
  mayManageUsers,
  mayReadActivity,
} from '../../api/permissions';
import { useTranslate } from '../../i18n/languages';
import { AuthenticationPage } from '../AuthenticationPage';
import { PowerConfirmation } from '../PowerConfirmation';
import { PluginsPage } from '../pages/PluginsPage';
import { BenchPage } from '../pages/BenchPage';
import { ActivityPage } from '../pages/ActivityPage';
import { NetworkPage } from '../pages/NetworkPage';
import { AboutPage, IntegrationPage } from '../pages/SimplePages';
import { Icon } from './Icon';
import { NavDrawer } from './NavDrawer';
import { SideNav } from './SideNav';
import { TopRow } from './TopRow';
import { useViewportClass } from './useViewportClass';
import { buildPages, findPage, integrationPageId, type PageDescriptor } from './pages';
import { usePruneClosedPages, type Workspace } from './useWorkspace';

export interface AppShellProps {
  client: BenchCoreClient;
  credential: Credential;
  session: BenchCoreSession;
  status: BenchCoreStatus;
  integrations: Resource<IntegrationInventory>;
  devices: Resource<DeviceInventory>;
  /**
   * Owned above this component because a refresh replaces the active phase and
   * unmounts the shell; a workspace created in here would take the open tabs
   * down with it every time the bench was re-read.
   */
  workspace: Workspace;
  onRefresh: () => void;
  onSignOut: () => void;
  onSessionEnded: () => void;
}

/**
 * The application shell.
 *
 * One grid, four cells: the wordmark in the navigation column of the header
 * row, the controls in the workspace column of the same row, the navigation
 * below the wordmark and the workspace below the controls, with a slim boundary
 * strip under the workspace. There is no second header row and no page title
 * above the workspace, because the tab already names the page.
 */
export function AppShell({
  client,
  credential,
  session,
  status,
  integrations,
  devices,
  workspace,
  onRefresh,
  onSignOut,
  onSessionEnded,
}: AppShellProps) {
  const t = useTranslate();
  const viewport = useViewportClass();
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [powerAction, setPowerAction] = useState<PowerAction | null>(null);
  const menuButton = useRef<HTMLButtonElement>(null);
  // Focus returns to the Menu control whenever the drawer closes, whichever
  // way it closed and whether or not the pointer that opened it left focus
  // there; a finger on a panel does not.
  const drawerWasOpen = useRef(false);
  useEffect(() => {
    if (drawerWasOpen.current && !drawerOpen) {
      menuButton.current?.focus();
    }
    drawerWasOpen.current = drawerOpen;
  }, [drawerOpen]);
  const [networkStatus, setNetworkStatus] = useState<NetworkStatus | null>(null);
  const [wireGuard, setWireGuard] = useState<WireGuardStatus | null>(null);
  const [benchCoreReachable, setBenchCoreReachable] = useState(true);
  useEffect(() => {
    let abandoned = false;
    const refresh = async () => {
      const [network, vpn, core] = await Promise.allSettled([
        client.getNetworkStatus(credential),
        client.getWireGuardStatus(credential),
        client.getStatus(credential),
      ]);
      if (abandoned) return;
      setNetworkStatus(network.status === 'fulfilled' ? network.value : null);
      setWireGuard(vpn.status === 'fulfilled' ? vpn.value : null);
      setBenchCoreReachable(core.status === 'fulfilled');
    };
    void refresh();
    const timer = setInterval(() => void refresh(), 5000);
    return () => {
      abandoned = true;
      clearInterval(timer);
    };
  }, [client, credential]);

  // Memoised because a fresh empty array on every render would rebuild the
  // navigation, and with it the tab list, on every render too. An unreadable
  // integration inventory leaves the navigation without instrument entries,
  // which is the honest shape: nothing is known to exist.
  const reportedIntegrations = useMemo(
    () => (integrations.ok ? integrations.value.items : []),
    [integrations],
  );
  const pages = useMemo(
    () => buildPages(session, reportedIntegrations),
    [session, reportedIntegrations],
  );
  const pageIds = useMemo(() => pages.map((page) => page.id), [pages]);

  usePruneClosedPages(workspace, pageIds);

  // Bound here rather than passing the client and credential down, so the
  // Network page depends on two reads instead of on the whole client surface —
  // and cannot reach a mutating method it has no business calling.
  const readNetworkStatus = useCallback(
    () => client.getNetworkStatus(credential),
    [client, credential],
  );
  const readNetworkStatistics = useCallback(
    (interfaceId: string, period: StatisticsPeriod) =>
      client.getNetworkStatistics(credential, interfaceId, period),
    [client, credential],
  );
  const previewInterfaceChange = useCallback(
    (interfaceId: string, action: 'enable' | 'disable') =>
      client.previewInterfaceChange(credential, interfaceId, action),
    [client, credential],
  );
  const acknowledgeOperation = useCallback(
    (id: string, revision: string) =>
      client.acknowledgeNetworkOperation(credential, id, revision),
    [client, credential],
  );
  const confirmOperation = useCallback(
    (id: string) => client.confirmNetworkOperation(credential, id),
    [client, credential],
  );
  const rollbackOperation = useCallback(
    (id: string) => client.rollbackNetworkOperation(credential, id),
    [client, credential],
  );
  const canManageInterfaces = session.permissions.includes(PERMISSION_NETWORK_MANAGE);
  const previewWireGuardChange = useCallback(
    (change: WireGuardChange) => client.previewWireGuardChange(credential, change),
    [client, credential],
  );
  const readOperation = useCallback(
    (id: string) => client.getNetworkOperation(credential, id),
    [client, credential],
  );
  const listOperations = useCallback(
    () => client.listNetworkOperations(credential),
    [client, credential],
  );
  const readDeviceNetworks = useCallback(
    () => client.listDeviceNetworks(credential),
    [client, credential],
  );
  const previewDeviceNetworkChange = useCallback(
    (change: DeviceNetworkChange) =>
      client.previewDeviceNetworkChange(credential, change),
    [client, credential],
  );
  const checkDeviceNetwork = useCallback(
    (id: string, tcpPort: number) => client.checkDeviceNetwork(credential, id, tcpPort),
    [client, credential],
  );
  const readAccessInstructions = useCallback(
    (id: string) => client.getAccessInstructions(credential, id),
    [client, credential],
  );
  const previewInterfaceSettings = useCallback(
    (interfaceId: string, settings: InterfaceSettingsDraft) =>
      client.previewInterfaceSettings(credential, interfaceId, settings),
    [client, credential],
  );
  const scanWiFi = useCallback(
    (interfaceId: string) => client.scanWiFi(credential, interfaceId),
    [client, credential],
  );
  const previewWiFiJoin = useCallback(
    (interfaceId: string, join: WiFiJoinDraft) =>
      client.previewWiFiJoin(credential, interfaceId, join),
    [client, credential],
  );
  // The Activity page opens narrowed to what the operator was looking at; the
  // filter is kept here so the tab it opens into can read it.
  const [activityFilter, setActivityFilter] = useState<ActivityFilter>({});
  const openActivity = useCallback(
    (filter: ActivityFilter) => {
      setActivityFilter(filter);
      workspace.open_('activity');
    },
    [workspace],
  );
  const readActivity = useCallback(
    (filter: ActivityFilter) => client.listActivity(credential, filter),
    [client, credential],
  );
  const exportActivity = useCallback(
    (filter: ActivityFilter, format: 'jsonl' | 'csv') =>
      client.exportActivity(credential, filter, format),
    [client, credential],
  );
  const readRetention = useCallback(
    () => client.getActivityRetention(credential),
    [client, credential],
  );
  const saveRetention = useCallback(
    (settings: Parameters<typeof client.setActivityRetention>[1]) =>
      client.setActivityRetention(credential, settings),
    [client, credential],
  );

  /** A fixed page's name is translated; an integration's is what it reported. */
  const nameOf = (page: PageDescriptor): string =>
    page.titleKey === undefined ? (page.title ?? page.id) : t(page.titleKey);

  const openTabs = workspace.open
    .map((id) => findPage(pages, id))
    .filter((page): page is PageDescriptor => page !== undefined);
  const active = workspace.active === null ? null : findPage(pages, workspace.active);
  // On a constrained viewport the rail is gone; the same navigation opens as a
  // drawer, and choosing an entry closes it so the page it opened is on screen.
  const openFromDrawer = (id: string) => {
    setDrawerOpen(false);
    workspace.open_(id);
  };

  return (
    <div className="shell">
      <div className="shell-brand">
        {viewport === 'constrained' && (
          <button
            type="button"
            ref={menuButton}
            className="shell-menu"
            aria-haspopup="dialog"
            aria-expanded={drawerOpen}
            // Named on the button itself, because the visible word gives way
            // on a very narrow viewport while the icon stays.
            aria-label={t('nav.menu')}
            onClick={() => setDrawerOpen(true)}
          >
            <Icon name="menu" size={20} />
            <span className="shell-menu-label">{t('nav.menu')}</span>
          </button>
        )}
        <span className="brand-mark" aria-hidden="true">
          N
        </span>
        <span className="brand-word">{t('app.name')}</span>
      </div>

      <TopRow
        viewport={viewport}
        simulation={status.simulation}
        networkStatus={networkStatus}
        wireGuardStatus={wireGuard}
        tabs={openTabs}
        active={workspace.active}
        session={session}
        benchCoreReachable={benchCoreReachable}
        onActivate={workspace.activate}
        onClose={workspace.close}
        onOpen={workspace.open_}
        onSignOut={onSignOut}
        onPowerAction={setPowerAction}
        nameOf={nameOf}
      />

      {viewport !== 'constrained' && (
        <SideNav
          pages={pages}
          active={workspace.active}
          onOpen={workspace.open_}
          nameOf={nameOf}
        />
      )}
      {drawerOpen && (
        <NavDrawer onDismiss={() => setDrawerOpen(false)}>
          <SideNav
            pages={pages}
            active={workspace.active}
            onOpen={openFromDrawer}
            nameOf={nameOf}
          />
        </NavDrawer>
      )}

      <main
        className="shell-workspace"
        id="workspace-panel"
        role="tabpanel"
        aria-labelledby={
          active === undefined || active === null ? undefined : `tab-${active.id}`
        }
      >
        {active === undefined || active === null ? (
          <div className="page">
            <div className="page-body">
              <section className="notice">
                <h3>{t('tabs.none.title')}</h3>
                <p>{t('tabs.none.body')}</p>
              </section>
            </div>
          </div>
        ) : (
          renderPage(active)
        )}
      </main>
      {powerAction !== null &&
        !status.simulation &&
        session.credential === 'session' &&
        session.permissions.includes(PERMISSION_SYSTEM_POWER_MANAGE) && (
          <PowerConfirmation
            action={powerAction}
            onConfirm={() =>
              client.requestPower(credential, powerAction).then(() => undefined)
            }
            onDismiss={() => setPowerAction(null)}
          />
        )}
    </div>
  );

  function renderPage(page: PageDescriptor) {
    if (page.id === 'bench') {
      return (
        <BenchPage
          status={status}
          integrations={integrations}
          devices={devices}
          onRefresh={onRefresh}
        />
      );
    }
    if (page.networkView !== undefined) {
      return (
        <NetworkPage
          view={page.networkView}
          readNetworkStatus={readNetworkStatus}
          readNetworkStatistics={readNetworkStatistics}
          previewInterfaceChange={previewInterfaceChange}
          acknowledgeOperation={acknowledgeOperation}
          confirmOperation={confirmOperation}
          rollbackOperation={rollbackOperation}
          canManageInterfaces={canManageInterfaces}
          wireGuardStatus={wireGuard}
          canManageVPN={session.permissions.includes(PERMISSION_VPN_MANAGE)}
          canInstallVPN={session.permissions.includes(PERMISSION_VPN_INSTALL)}
          previewWireGuardChange={previewWireGuardChange}
          readOperation={readOperation}
          benchCoreReachable={benchCoreReachable}
          listOperations={listOperations}
          readDeviceNetworks={readDeviceNetworks}
          previewDeviceNetworkChange={previewDeviceNetworkChange}
          checkDeviceNetwork={checkDeviceNetwork}
          readAccessInstructions={readAccessInstructions}
          previewInterfaceSettings={previewInterfaceSettings}
          scanWiFi={scanWiFi}
          previewWiFiJoin={previewWiFiJoin}
          onOpenActivity={openActivity}
        />
      );
    }
    if (page.id === 'activity') {
      return (
        <ActivityPage
          key={JSON.stringify(activityFilter)}
          initialFilter={activityFilter}
          readActivity={readActivity}
          exportActivity={exportActivity}
          canExport={session.permissions.includes(PERMISSION_ACTIVITY_EXPORT)}
          canManageRetention={session.permissions.includes(
            PERMISSION_ACTIVITY_RETENTION_MANAGE,
          )}
          readRetention={readRetention}
          saveRetention={saveRetention}
        />
      );
    }
    if (page.id === 'about') {
      return <AboutPage status={status} />;
    }
    if (page.id === 'plugins' || page.id === integrationPageId('sigrok')) {
      return (
        <PluginsPage
          client={client}
          credential={credential}
          session={session}
          onSessionEnded={onSessionEnded}
        />
      );
    }
    if (page.authenticationView !== undefined) {
      return (
        <AuthenticationPage
          client={client}
          credential={credential}
          session={session}
          view={page.authenticationView}
          canManageUsers={mayManageUsers(session)}
          canManageTokens={mayManageTokens(session)}
          onSessionEnded={onSessionEnded}
          onOpenActivity={
            mayReadActivity(session)
              ? () => openActivity({ component: 'identity' })
              : undefined
          }
        />
      );
    }
    const integration = reportedIntegrations.find(
      (candidate) => integrationPageId(candidate.id) === page.id,
    );
    return integration === undefined ? null : (
      <IntegrationPage integration={integration} devices={devices} />
    );
  }
}
