import { mayReadActivity, mayReadTokens, mayReadUsers } from '../../api/permissions';
import type { BenchCoreIntegration, BenchCoreSession } from '../../api/contract';
import type { MessageKey } from '../../i18n/en';
import type { IconName } from './Icon';
import type { NetworkView } from '../network/views';
import type { AuthenticationView } from '../AuthenticationPage';

/**
 * Which navigation group an entry belongs to.
 *
 * `core` is the bottom-anchored cluster. Network lives there because it is a
 * built-in appliance capability; filing it with the integrations would teach an
 * operator to look for it in the wrong place.
 */
export type NavCluster = 'working' | 'instruments' | 'core';

/** One page the shell can open as a tab. */
export interface PageDescriptor {
  id: string;
  cluster: NavCluster;
  icon: IconName;
  /** The catalogue key for a fixed page's name. */
  titleKey?: MessageKey;
  /** The backend-reported name of an integration page, never translated. */
  title?: string;
  /** Parent page whose nested navigation list contains this destination. */
  parentId?: string;
  /** A shorter label for a group row than the page's workspace-tab title. */
  navigationTitleKey?: MessageKey;
  /** The label used when a page-owning group repeats its landing page below. */
  overviewTitleKey?: MessageKey;
  /** Accessible labels for expanding and collapsing a grouped destination. */
  expandTitleKey?: MessageKey;
  collapseTitleKey?: MessageKey;
  /** The focused Network content this page renders. */
  networkView?: NetworkView;
  /** The focused Authentication content this page renders. */
  authenticationView?: AuthenticationView;
  /**
   * Whether the page does something. An entry that is not available is still
   * shown, because the navigation shape is agreed, but it is marked so that
   * nothing is advertised as working before it is.
   */
  available: boolean;
}

/** Fixed entries of the bottom cluster, top to bottom, with nested children. */
const CORE_PAGES: PageDescriptor[] = [
  {
    id: 'plugins',
    cluster: 'core',
    icon: 'plugins',
    titleKey: 'nav.plugins',
    available: true,
  },
  {
    id: 'network',
    cluster: 'core',
    icon: 'network',
    titleKey: 'nav.network.overviewTab',
    navigationTitleKey: 'nav.network',
    overviewTitleKey: 'nav.network.overview',
    expandTitleKey: 'nav.network.expand',
    collapseTitleKey: 'nav.network.collapse',
    networkView: 'overview',
    available: true,
  },
  {
    id: 'network:adapters',
    parentId: 'network',
    cluster: 'core',
    icon: 'settings',
    titleKey: 'nav.network.adapters',
    networkView: 'adapters',
    available: true,
  },
  {
    id: 'network:device-access',
    parentId: 'network',
    cluster: 'core',
    icon: 'instrument',
    titleKey: 'nav.network.deviceAccess',
    networkView: 'device-access',
    available: true,
  },
  {
    id: 'network:vpn',
    parentId: 'network',
    cluster: 'core',
    icon: 'authentication',
    titleKey: 'nav.network.vpn',
    networkView: 'vpn',
    available: true,
  },
  {
    id: 'network:diagnostics',
    parentId: 'network',
    cluster: 'core',
    icon: 'about',
    titleKey: 'nav.network.diagnostics',
    networkView: 'diagnostics',
    available: true,
  },
  {
    id: 'authentication',
    cluster: 'core',
    icon: 'authentication',
    titleKey: 'auth.usersTitle',
    navigationTitleKey: 'nav.authentication',
    overviewTitleKey: 'nav.authentication.users',
    expandTitleKey: 'nav.authentication.expand',
    collapseTitleKey: 'nav.authentication.collapse',
    authenticationView: 'users',
    available: true,
  },
  {
    id: 'authentication:tokens',
    parentId: 'authentication',
    cluster: 'core',
    icon: 'authentication',
    titleKey: 'auth.tokensTitle',
    navigationTitleKey: 'nav.authentication.tokens',
    authenticationView: 'tokens',
    available: true,
  },
  {
    id: 'activity',
    cluster: 'core',
    icon: 'about',
    titleKey: 'nav.activity',
    available: true,
  },
  {
    id: 'about',
    cluster: 'core',
    icon: 'about',
    titleKey: 'nav.about',
    available: true,
  },
];

const BENCH_PAGE: PageDescriptor = {
  id: 'bench',
  cluster: 'working',
  icon: 'bench',
  titleKey: 'nav.bench',
  available: true,
};

/** The page a signed-in session opens on. */
export const DEFAULT_PAGE = BENCH_PAGE.id;

/** Build the integration page identifier for one reported integration. */
export function integrationPageId(integrationId: string): string {
  return `integration:${integrationId}`;
}

/**
 * Build the navigation for one session and one reported inventory.
 *
 * Instrument entries are the integrations the backend actually reported, so the
 * navigation never advertises an instrument this appliance does not have. The
 * Authentication entry is filtered by permission, which decides visibility and
 * nothing else — Bench Core authorizes every request on its own, and an
 * identity that reaches the page another way is refused there.
 */
export function buildPages(
  session: BenchCoreSession | null,
  integrations: readonly BenchCoreIntegration[],
): PageDescriptor[] {
  const instruments: PageDescriptor[] = integrations.map((integration) => ({
    id: integrationPageId(integration.id),
    cluster: 'instruments',
    icon: 'instrument',
    title: integration.displayName,
    available: true,
  }));

  const canReadUsers = mayReadUsers(session);
  const canReadTokens = mayReadTokens(session);
  const core = CORE_PAGES.filter((page) => {
    if (page.id === 'authentication') return canReadUsers || canReadTokens;
    if (page.id === 'authentication:tokens') return canReadUsers && canReadTokens;
    if (page.id === 'activity') return mayReadActivity(session);
    return true;
  }).map((page) => {
    if (page.id !== 'authentication' || canReadUsers) return page;
    return {
      ...page,
      titleKey: 'auth.tokensTitle' as const,
      overviewTitleKey: 'nav.authentication.tokens' as const,
      authenticationView: 'tokens' as const,
    };
  });

  return [BENCH_PAGE, ...instruments, ...core];
}

/** Find one page in a built navigation. */
export function findPage(
  pages: readonly PageDescriptor[],
  id: string,
): PageDescriptor | undefined {
  return pages.find((page) => page.id === id);
}
