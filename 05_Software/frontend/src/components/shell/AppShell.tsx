import { NotificationProvider } from '../MessageBanner';
import {
  useCallback,
  useMemo,
  useEffect,
  useRef,
  useState,
  type CSSProperties,
} from 'react';

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
  resourceUrl,
} from '../../api/contract';
import type { PowerAction } from '../../api/contract';
import type { ActivityFilter } from '../../api/activity';
import type { DeviceNetworkChange } from '../../api/devicenetworks';
import type { InterfaceSettingsDraft, WiFiJoinDraft } from '../../api/settings';
import type { WireGuardStatus, WireGuardChange } from '../../api/wireguard';
import { APPLIANCE_PATH, type ApplianceMetadata } from '../../api/appliance';
import type { Resource } from '../../api/resource';
import type { InstrumentPlugin } from '../../api/bench';
import {
  mayManageTokens,
  mayManageUsers,
  mayReadActivity,
} from '../../api/permissions';
import { useTranslate } from '../../i18n/languages';
import { AuthenticationPage } from '../AuthenticationPage';
import { PowerConfirmation } from '../PowerConfirmation';
import { CameraPluginPage } from '../pages/CameraPluginPage';
import { SerialWorkspacePage } from '../pages/SerialWorkspacePage';
import { CameraWorkspacePage } from '../pages/CameraWorkspacePage';
import { PluginsPage } from '../pages/PluginsPage';
import { BenchPage } from '../pages/BenchPage';
import { ActivityPage } from '../pages/ActivityPage';
import { NetworkPage } from '../pages/NetworkPage';
import { AboutPage, IntegrationPage } from '../pages/SimplePages';
import { Icon } from './Icon';
import { NavDrawer } from './NavDrawer';
import { SideNav } from './SideNav';
import { NavigationSplitter } from './NavigationSplitter';
import { useNavigationWidth } from './useNavigationWidth';
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
  lastIntegrations: IntegrationInventory | null;
  lastDevices: DeviceInventory | null;
  lastPlugins: InstrumentPlugin[] | null;
  onPluginsLoaded: (plugins: InstrumentPlugin[]) => void;
  /**
   * Owned above this component because a refresh replaces the active phase and
   * unmounts the shell; a workspace created in here would take the open tabs
   * down with it every time the bench was re-read.
   */
  workspace: Workspace;
  expandedNavigation: Readonly<Record<string, boolean>>;
  onToggleNavigation: (id: string) => void;
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
  lastIntegrations,
  lastDevices,
  lastPlugins,
  onPluginsLoaded,
  workspace,
  expandedNavigation,
  onToggleNavigation,
  onRefresh,
  onSignOut,
  onSessionEnded,
}: AppShellProps) {
  const t = useTranslate();
  const viewport = useViewportClass();
  const navigation = useNavigationWidth(viewport);
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
  const [appliance, setAppliance] = useState<ApplianceMetadata | null>(null);
  const [pluginInventory, setPluginInventory] = useState<InstrumentPlugin[] | null>(
    lastPlugins,
  );
  const [pluginInventoryFresh, setPluginInventoryFresh] = useState(false);
  const refreshPluginInventory = useCallback(async () => {
    try {
      const plugins = await client.listPlugins(credential);
      setPluginInventory(plugins);
      onPluginsLoaded(plugins);
      setPluginInventoryFresh(true);
    } catch {
      // Keep the last known inventory: a transient read failure must not close tabs.
      setPluginInventoryFresh(false);
    }
  }, [client, credential, onPluginsLoaded]);
  useEffect(() => {
    const initial = window.setTimeout(() => void refreshPluginInventory(), 0);
    const timer = window.setInterval(() => void refreshPluginInventory(), 5000);
    return () => {
      window.clearTimeout(initial);
      window.clearInterval(timer);
    };
  }, [refreshPluginInventory]);
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
  useEffect(() => {
    if (status.profile !== 'appliance' || client.getAppliance === undefined) {
      return;
    }
    let abandoned = false;
    void client.getAppliance(credential).then(
      (value) => {
        if (!abandoned) setAppliance(value);
      },
      () => {
        if (!abandoned) setAppliance(null);
      },
    );
    return () => {
      abandoned = true;
    };
  }, [client, credential, status.profile]);
  const currentAppliance = status.profile === 'appliance' ? appliance : null;

  // Memoised because a fresh empty array on every render would rebuild the
  // navigation, and with it the tab list, on every render too. An unreadable
  // integration inventory leaves the navigation without instrument entries,
  // which is the honest shape: nothing is known to exist.
  const reportedIntegrations = useMemo(
    () =>
      integrations.ok ? integrations.value.items : (lastIntegrations?.items ?? []),
    [integrations, lastIntegrations],
  );
  const reportedDevices = useMemo(
    () => (devices.ok ? devices.value.items : (lastDevices?.items ?? [])),
    [devices, lastDevices],
  );
  const pages = useMemo(
    () =>
      buildPages(session, reportedIntegrations, reportedDevices, pluginInventory ?? []),
    [session, reportedIntegrations, reportedDevices, pluginInventory],
  );
  const pageIds = useMemo(
    () =>
      !integrations.ok || !devices.ok || !pluginInventoryFresh || !benchCoreReachable
        ? [
            ...pages.filter((page) => !page.navigationOnly).map((page) => page.id),
            ...workspace.open.filter(
              (id) =>
                id.startsWith('integration:') ||
                id.startsWith('camera:') ||
                id.startsWith('serial:'),
            ),
          ]
        : pages.filter((page) => !page.navigationOnly).map((page) => page.id),
    [
      pages,
      integrations,
      devices,
      pluginInventoryFresh,
      benchCoreReachable,
      workspace.open,
    ],
  );

  usePruneClosedPages(workspace, pageIds);
  const navigationPages = useMemo(
    () =>
      pluginInventoryFresh && benchCoreReachable && integrations.ok && devices.ok
        ? pages
        : pages.filter((page) => page.cluster !== 'instruments'),
    [pages, pluginInventoryFresh, benchCoreReachable, integrations, devices],
  );

  // Bound here rather than passing the client and credential down, so the
  // Network page depends on two reads instead of on the whole client surface —
  // and cannot reach a mutating method it has no business calling.
  const readNetworkStatus = useCallback(
    () => client.getNetworkStatus(credential),
    [client, credential],
  );
  const readManagementAccess = useCallback(
    () => client.getManagementAccess(credential),
    [client, credential],
  );
  const updateManagementAccess = useCallback(
    (interfaces: string[], confirmCurrentInterface: boolean) =>
      client.setManagementAccess(credential, interfaces, confirmCurrentInterface),
    [client, credential],
  );
  const confirmManagementAccess = useCallback(
    () => client.confirmManagementAccess(credential),
    [client, credential],
  );
  const recoverManagementAccess = useCallback(
    () => client.recoverManagementAccess(credential),
    [client, credential],
  );
  const completeCommissioning = useCallback(
    (interfaces: string[], confirmCurrentInterface: boolean) =>
      client.completeCommissioning(credential, interfaces, confirmCurrentInterface),
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
    <div
      className="shell"
      style={
        {
          '--nav-width': `${navigation.width}px`,
          '--nav-width-compact': `${navigation.width}px`,
        } as CSSProperties
      }
    >
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
        <div className="shell-navigation" id="main-navigation">
          <SideNav
            pages={navigationPages}
            active={workspace.active}
            onOpen={workspace.open_}
            nameOf={nameOf}
            expanded={expandedNavigation}
            onToggle={onToggleNavigation}
            instrumentsUnavailable={
              !integrations.ok ||
              !devices.ok ||
              !pluginInventoryFresh ||
              !benchCoreReachable
            }
          />
          <NavigationSplitter
            width={navigation.width}
            min={navigation.min}
            max={navigation.max}
            onResize={navigation.setWidth}
            onReset={navigation.reset}
          />
        </div>
      )}
      {drawerOpen && (
        <NavDrawer onDismiss={() => setDrawerOpen(false)}>
          <SideNav
            pages={navigationPages}
            active={workspace.active}
            onOpen={openFromDrawer}
            nameOf={nameOf}
            expanded={expandedNavigation}
            onToggle={onToggleNavigation}
            instrumentsUnavailable={
              !integrations.ok ||
              !devices.ok ||
              !pluginInventoryFresh ||
              !benchCoreReachable
            }
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
          <NotificationProvider scope={active.id}>
            {renderPage(active)}
          </NotificationProvider>
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
    if (page.serialDeviceID && page.serialView) {
      const device = pluginInventory
        ?.find((plugin) => plugin.id === 'serial')
        ?.devices.find((item) => item.id === page.serialDeviceID);
      return device ? (
        <SerialWorkspacePage
          key={`${device.id}:${page.serialView}`}
          client={client}
          credential={credential}
          session={session}
          device={device}
          view={page.serialView}
          onSessionEnded={onSessionEnded}
        />
      ) : null;
    }
    if (
      page.cameraDeviceID &&
      (page.cameraView === 'live' ||
        page.cameraView === 'settings' ||
        page.cameraView === 'recordings')
    ) {
      const device = pluginInventory
        ?.find((plugin) => plugin.id === 'camera')
        ?.devices.find((item) => item.id === page.cameraDeviceID);
      return device ? (
        <CameraWorkspacePage
          key={device.id}
          client={client}
          credential={credential}
          session={session}
          device={device}
          view={page.cameraView === 'settings' ? 'live' : page.cameraView}
        />
      ) : null;
    }
    if (page.id === 'inventory') {
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
          readManagementAccess={status.simulation ? undefined : readManagementAccess}
          updateManagementAccess={
            status.simulation ? undefined : updateManagementAccess
          }
          confirmManagementAccess={
            status.simulation ? undefined : confirmManagementAccess
          }
          recoverManagementAccess={
            status.simulation ? undefined : recoverManagementAccess
          }
          completeCommissioning={status.simulation ? undefined : completeCommissioning}
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
      return (
        <AboutPage
          key={`appliance:${currentAppliance?.serialName ?? 'about'}`}
          status={status}
          appliance={currentAppliance}
          labelUrl={resourceUrl(client.baseUrl, `${APPLIANCE_PATH}/label.svg`)}
          canIdentify={canManageInterfaces}
          onIdentify={
            client.identifyAppliance === undefined
              ? undefined
              : () => client.identifyAppliance!(credential)
          }
          onFriendlyName={
            client.setApplianceFriendlyName === undefined || !canManageInterfaces
              ? undefined
              : async (name) => {
                  const updated = await client.setApplianceFriendlyName!(
                    credential,
                    name,
                  );
                  setAppliance(updated);
                  return updated;
                }
          }
        />
      );
    }
    if (page.pluginId === 'camera' || page.pluginId === 'serial') {
      return (
        <CameraPluginPage
          pluginId={page.pluginId}
          client={client}
          credential={credential}
          session={session}
          onSessionEnded={onSessionEnded}
          onPluginsChanged={refreshPluginInventory}
          onOpenActivity={
            mayReadActivity(session)
              ? () => openActivity({ component: 'bench', target: page.pluginId })
              : undefined
          }
        />
      );
    }
    if (
      page.id === 'plugins' ||
      page.id === integrationPageId('sigrok') ||
      page.pluginId !== undefined ||
      page.cameraView !== undefined
    ) {
      return (
        <PluginsPage
          client={client}
          credential={credential}
          session={session}
          onSessionEnded={onSessionEnded}
          view={
            page.id === 'plugins'
              ? 'catalog'
              : page.id === integrationPageId('sigrok')
                ? 'legacy-sigrok'
                : page.pluginId !== undefined
                  ? 'administration'
                  : page.cameraView === 'live' || page.cameraView === 'recordings'
                    ? 'overview'
                    : page.cameraView
          }
          focusPluginId={page.pluginId}
          focusDevice={page.cameraDeviceID}
          onPluginsChanged={refreshPluginInventory}
          onOpenPluginAdmin={(pluginId) => workspace.open_(`plugins:${pluginId}`)}
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
