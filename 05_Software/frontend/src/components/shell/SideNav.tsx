import { useTranslate } from '../../i18n/languages';
import { Icon } from './Icon';
import type { PageDescriptor } from './pages';

export interface SideNavProps {
  pages: readonly PageDescriptor[];
  active: string | null;
  onOpen: (id: string) => void;
  nameOf: (page: PageDescriptor) => string;
  expanded: Readonly<Record<string, boolean>>;
  onToggle: (id: string) => void;
  instrumentsUnavailable: boolean;
}

/**
 * Two session-controlled sections share the same disclosure state in the rail
 * and drawer. A section's presence never depends on its eligible children.
 * The compact rail retains visible labels; the drawer shows the same list.
 */
export function SideNav({
  pages,
  active,
  onOpen,
  nameOf,
  expanded,
  onToggle,
  instrumentsUnavailable,
}: SideNavProps) {
  const t = useTranslate();
  const instruments = pages.filter((page) => page.cluster === 'instruments');
  const core = pages.filter((page) => page.cluster === 'core');
  const roots = (entries: PageDescriptor[]) =>
    entries.filter((page) => page.parentId === undefined);
  const childrenOf = (id: string) => pages.filter((page) => page.parentId === id);
  const containsActive = (page: PageDescriptor): boolean =>
    page.id === active || childrenOf(page.id).some(containsActive);

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
    const isExpanded = expanded[page.id] ?? false;
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
            className={`nav-entry ${page.parentId !== undefined ? 'nav-entry-child' : ''} ${groupActive ? 'nav-entry-context' : ''} ${!isExpanded && groupActive ? 'nav-entry-active' : ''}`}
            aria-current={
              !page.navigationOnly && !isExpanded && groupActive ? 'page' : undefined
            }
            aria-label={groupName}
            aria-expanded={page.navigationOnly ? isExpanded : undefined}
            aria-controls={page.navigationOnly ? listId : undefined}
            onClick={() => (page.navigationOnly ? onToggle(page.id) : onOpen(page.id))}
          >
            <Icon name={page.icon} />
            <span className="nav-label">{groupName}</span>
          </button>
          <button
            type="button"
            className="nav-group-toggle"
            aria-label={
              isExpanded
                ? page.collapseTitleKey
                  ? t(page.collapseTitleKey)
                  : t('nav.group.collapse', { name: groupName })
                : page.expandTitleKey
                  ? t(page.expandTitleKey)
                  : t('nav.group.expand', { name: groupName })
            }
            aria-expanded={isExpanded}
            aria-controls={listId}
            onClick={() => onToggle(page.id)}
          >
            <Icon name={isExpanded ? 'chevronUp' : 'chevron'} size={16} />
          </button>
        </div>
        {isExpanded ? (
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
        <button
          type="button"
          className="nav-section nav-section-toggle"
          aria-expanded={expanded.instruments ?? false}
          aria-controls="nav-instruments"
          onClick={() => onToggle('instruments')}
        >
          {t('nav.section.instruments')}
          <Icon name={expanded.instruments ? 'chevronUp' : 'chevron'} size={16} />
        </button>
        {expanded.instruments && (
          <ul className="nav-list" id="nav-instruments">
            {roots(instruments).map(groupedEntry)}
            {instruments.length === 0 && (
              <li className="nav-empty">
                {t(
                  instrumentsUnavailable
                    ? 'nav.instruments.unavailable'
                    : 'nav.instruments.empty',
                )}
              </li>
            )}
          </ul>
        )}
      </div>
      <div className="nav-lower">
        <button
          type="button"
          className="nav-section nav-section-toggle"
          aria-expanded={expanded.system ?? false}
          aria-controls="nav-system"
          onClick={() => onToggle('system')}
        >
          {t('nav.section.core')}
          <Icon name={expanded.system ? 'chevronUp' : 'chevron'} size={16} />
        </button>
        {expanded.system && (
          <ul className="nav-list" id="nav-system">
            {roots(core).map(groupedEntry)}
          </ul>
        )}
      </div>
    </nav>
  );
}
