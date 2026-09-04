import { useState } from 'react';

import { useTranslate } from '../../i18n/languages';
import { Icon } from './Icon';
import type { PageDescriptor } from './pages';

export interface SideNavProps {
  pages: readonly PageDescriptor[];
  active: string | null;
  onOpen: (id: string) => void;
  nameOf: (page: PageDescriptor) => string;
}

/**
 * The left navigation: working entries above, the core cluster anchored below.
 *
 * The gap between the two groups is the layout doing the work — the upper area
 * grows with however many integrations the appliance reports, and the core
 * cluster stays where the eye learned to find it. An entry whose page is not
 * built yet is marked rather than hidden, so the agreed navigation shape is
 * present without anything being advertised as working before it is.
 *
 * Every entry keeps its visible label on every viewport: the compact rail only
 * narrows, and the constrained drawer shows the same list.
 */
export function SideNav({ pages, active, onOpen, nameOf }: SideNavProps) {
  const t = useTranslate();
  const [networkExpanded, setNetworkExpanded] = useState(true);
  const working = pages.filter((page) => page.cluster === 'working');
  const instruments = pages.filter((page) => page.cluster === 'instruments');
  const core = pages.filter((page) => page.cluster === 'core');
  const coreRoots = core.filter((page) => page.parentId === undefined);

  function entry(
    page: PageDescriptor,
    options: { child?: boolean; name?: string } = {},
  ) {
    const selected = page.id === active;
    const name = options.name ?? nameOf(page);
    return (
      <li key={page.id}>
        <button
          type="button"
          className={`nav-entry ${options.child ? 'nav-entry-child' : ''} ${selected ? 'nav-entry-active' : ''}`}
          aria-current={selected ? 'page' : undefined}
          aria-label={name}
          onClick={() => onOpen(page.id)}
        >
          <Icon name={page.icon} />
          <span className="nav-label">{name}</span>
          {page.available ? null : <span className="nav-pending" aria-hidden="true" />}
        </button>
      </li>
    );
  }

  function groupedEntry(page: PageDescriptor) {
    const children = core.filter((candidate) => candidate.parentId === page.id);
    if (children.length === 0) return entry(page);
    const groupActive =
      active === page.id || children.some((child) => child.id === active);
    const groupName =
      page.navigationTitleKey === undefined ? nameOf(page) : t(page.navigationTitleKey);
    const overviewName =
      page.overviewTitleKey === undefined ? nameOf(page) : t(page.overviewTitleKey);
    const listId = `nav-group-${page.id}`;
    return (
      <li className="nav-group" key={page.id}>
        <div className="nav-group-row">
          <button
            type="button"
            className={`nav-entry ${groupActive ? 'nav-entry-context' : ''} ${!networkExpanded && groupActive ? 'nav-entry-active' : ''}`}
            aria-current={!networkExpanded && groupActive ? 'page' : undefined}
            aria-label={groupName}
            onClick={() => onOpen(page.id)}
          >
            <Icon name={page.icon} />
            <span className="nav-label">{groupName}</span>
          </button>
          <button
            type="button"
            className="nav-group-toggle"
            aria-label={t(
              networkExpanded ? 'nav.network.collapse' : 'nav.network.expand',
            )}
            aria-expanded={networkExpanded}
            aria-controls={listId}
            onClick={() => setNetworkExpanded((current) => !current)}
          >
            <Icon name={networkExpanded ? 'chevronUp' : 'chevron'} size={16} />
          </button>
        </div>
        {networkExpanded ? (
          <ul className="nav-list nav-sublist" id={listId}>
            {entry(page, { child: true, name: overviewName })}
            {children.map((child) => entry(child, { child: true }))}
          </ul>
        ) : null}
      </li>
    );
  }

  return (
    <nav className="sidenav" aria-label={t('nav.label')}>
      <div className="nav-upper">
        <ul className="nav-list">{working.map((page) => entry(page))}</ul>
        {instruments.length === 0 ? null : (
          <>
            <p className="nav-section">{t('nav.section.instruments')}</p>
            <ul className="nav-list">{instruments.map((page) => entry(page))}</ul>
          </>
        )}
      </div>
      <div className="nav-lower">
        <p className="nav-section">{t('nav.section.core')}</p>
        <ul className="nav-list">{coreRoots.map(groupedEntry)}</ul>
      </div>
    </nav>
  );
}
