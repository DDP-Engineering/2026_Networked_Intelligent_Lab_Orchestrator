import { mayAdminister } from '../../api/permissions';
import type { BenchCoreIntegration, BenchCoreSession } from '../../api/contract';
import type { MessageKey } from '../../i18n/en';
import type { IconName } from './Icon';

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
  /**
   * Whether the page does something. An entry that is not available is still
   * shown, because the navigation shape is agreed, but it is marked so that
   * nothing is advertised as working before it is.
   */
  available: boolean;
}

/** The five fixed entries of the bottom cluster, in their agreed order. */
const CORE_PAGES: PageDescriptor[] = [
  {
    id: 'network',
    cluster: 'core',
    icon: 'network',
    titleKey: 'nav.network',
    available: true,
  },
  {
    id: 'plugins',
    cluster: 'core',
    icon: 'plugins',
    titleKey: 'nav.plugins',
    available: false,
  },
  {
    id: 'authentication',
    cluster: 'core',
    icon: 'authentication',
    titleKey: 'nav.authentication',
    available: true,
  },
  {
    id: 'settings',
    cluster: 'core',
    icon: 'settings',
    titleKey: 'nav.settings',
    available: false,
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

  const core = CORE_PAGES.filter(
    (page) => page.id !== 'authentication' || mayAdminister(session),
  );

  return [BENCH_PAGE, ...instruments, ...core];
}

/** Find one page in a built navigation. */
export function findPage(
  pages: readonly PageDescriptor[],
  id: string,
): PageDescriptor | undefined {
  return pages.find((page) => page.id === id);
}
