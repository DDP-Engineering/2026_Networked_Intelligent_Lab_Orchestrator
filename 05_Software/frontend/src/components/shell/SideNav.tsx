import { useEffect, useState } from 'react';

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
  const [expandedGroups, setExpandedGroups] = useState<Record<string, boolean>>({});
  const working = pages.filter((page) => page.cluster === 'working');
  const instruments = pages.filter((page) => page.cluster === 'instruments');
  const core = pages.filter((page) => page.cluster === 'core');
  const roots = (entries: PageDescriptor[]) =>
    entries.filter((page) => page.parentId === undefined);
  const childrenOf = (id: string) => pages.filter((page) => page.parentId === id);
  const containsActive = (page: PageDescriptor): boolean =>
    page.id === active || childrenOf(page.id).some(containsActive);

  useEffect(() => {
    const ancestors: string[] = [];
    let parentId = pages.find((page) => page.id === active)?.parentId;
    while (parentId !== undefined) {
      ancestors.push(parentId);
      parentId = pages.find((page) => page.id === parentId)?.parentId;
    }
    if (ancestors.length > 0) {
      const timer = window.setTimeout(
        () =>
          setExpandedGroups((current) =>
            Object.fromEntries([
              ...Object.entries(current),
              ...ancestors.map((id) => [id, true] as const),
            ]),
          ),
        0,
      );
      return () => window.clearTimeout(timer);
    }
  }, [active, pages]);

  function entry(
    page: PageDescriptor,
    options: { child?: boolean; name?: string } = {},
  ) {
    const selected = page.id === active;
    const name =
      options.name ??
      (page.navigationTitleKey === undefined
        ? nameOf(page)
        : t(page.navigationTitleKey));
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
    const children = childrenOf(page.id);
    if (
      children.length === 0 &&
      page.overviewTitleKey === undefined &&
      !page.navigationOnly
    ) {
      return entry(page, { child: page.parentId !== undefined });
    }
    const expanded = expandedGroups[page.id] ?? true;
    const groupActive = containsActive(page);
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
            className={`nav-entry ${page.parentId !== undefined ? 'nav-entry-child' : ''} ${groupActive ? 'nav-entry-context' : ''} ${!expanded && groupActive ? 'nav-entry-active' : ''}`}
            aria-current={
              !page.navigationOnly && !expanded && groupActive ? 'page' : undefined
            }
            aria-label={groupName}
            aria-expanded={page.navigationOnly ? expanded : undefined}
            aria-controls={page.navigationOnly ? listId : undefined}
            onClick={() =>
              page.navigationOnly
                ? setExpandedGroups((current) => ({ ...current, [page.id]: !expanded }))
                : onOpen(page.id)
            }
          >
            <Icon name={page.icon} />
            <span className="nav-label">{groupName}</span>
          </button>
          <button
            type="button"
            className="nav-group-toggle"
            aria-label={
              expanded
                ? page.collapseTitleKey
                  ? t(page.collapseTitleKey)
                  : t('nav.group.collapse', { name: groupName })
                : page.expandTitleKey
                  ? t(page.expandTitleKey)
                  : t('nav.group.expand', { name: groupName })
            }
            aria-expanded={expanded}
            aria-controls={listId}
            onClick={() =>
              setExpandedGroups((current) => ({
                ...current,
                [page.id]: !expanded,
              }))
            }
          >
            <Icon name={expanded ? 'chevronUp' : 'chevron'} size={16} />
          </button>
        </div>
        {expanded ? (
          <ul className="nav-list nav-sublist" id={listId}>
            {page.overviewTitleKey !== undefined &&
              entry(page, { child: true, name: overviewName })}
            {children.map(groupedEntry)}
          </ul>
        ) : null}
      </li>
    );
  }

  return (
    <nav className="sidenav" aria-label={t('nav.label')}>
      <div className="nav-upper">
        <ul className="nav-list">{roots(working).map(groupedEntry)}</ul>
        {instruments.length === 0 ? null : (
          <>
            <p className="nav-section">{t('nav.section.instruments')}</p>
            <ul className="nav-list">{roots(instruments).map(groupedEntry)}</ul>
          </>
        )}
      </div>
      <div className="nav-lower">
        <p className="nav-section">{t('nav.section.core')}</p>
        <ul className="nav-list">{roots(core).map(groupedEntry)}</ul>
      </div>
    </nav>
  );
}
