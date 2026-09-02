import { useMemo } from 'react';

import type { BenchCoreClient, Credential } from '../../api/client';
import type {
  BenchCoreSession,
  BenchCoreStatus,
  DeviceInventory,
  IntegrationInventory,
} from '../../api/contract';
import type { Resource } from '../../api/resource';
import { useTranslate } from '../../i18n/languages';
import { AuthenticationPage } from '../AuthenticationPage';
import { BenchPage } from '../pages/BenchPage';
import { NetworkPage } from '../pages/NetworkPage';
import { AboutPage, IntegrationPage, NotAvailablePage } from '../pages/SimplePages';
import { Icon } from './Icon';
import { SideNav } from './SideNav';
import { TopRow } from './TopRow';
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

  /** A fixed page's name is translated; an integration's is what it reported. */
  const nameOf = (page: PageDescriptor): string =>
    page.titleKey === undefined ? (page.title ?? page.id) : t(page.titleKey);

  const openTabs = workspace.open
    .map((id) => findPage(pages, id))
    .filter((page): page is PageDescriptor => page !== undefined);
  const active = workspace.active === null ? null : findPage(pages, workspace.active);

  return (
    <div className="shell">
      <div className="shell-brand">
        <span className="brand-mark" aria-hidden="true">
          N
        </span>
        <span className="brand-word">{t('app.name')}</span>
      </div>

      <TopRow
        tabs={openTabs}
        active={workspace.active}
        session={session}
        // Reaching this shell at all means Bench Core answered the status
        // request; it is stated as that one HTTP fact and nothing more.
        benchCoreReachable
        onActivate={workspace.activate}
        onClose={workspace.close}
        onOpen={workspace.open_}
        onSignOut={onSignOut}
        nameOf={nameOf}
      />

      <SideNav
        pages={pages}
        active={workspace.active}
        onOpen={workspace.open_}
        nameOf={nameOf}
      />

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

      <p className="shell-boundary">
        <Icon name="about" size={13} />
        {t('boundary.statement')}
      </p>
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
    if (page.id === 'network') {
      return <NetworkPage benchCoreReachable />;
    }
    if (page.id === 'about') {
      return <AboutPage status={status} />;
    }
    if (page.id === 'plugins') {
      return <NotAvailablePage nameKey="nav.plugins" bodyKey="page.plugins.body" />;
    }
    if (page.id === 'settings') {
      return <NotAvailablePage nameKey="nav.settings" bodyKey="page.settings.body" />;
    }
    if (page.id === 'authentication') {
      return (
        <AuthenticationPage
          client={client}
          credential={credential}
          session={session}
          onSessionEnded={onSessionEnded}
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
