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
 */
export function SideNav({ pages, active, onOpen, nameOf }: SideNavProps) {
  const t = useTranslate();
  const working = pages.filter((page) => page.cluster === 'working');
  const instruments = pages.filter((page) => page.cluster === 'instruments');
  const core = pages.filter((page) => page.cluster === 'core');

  function entry(page: PageDescriptor) {
    const selected = page.id === active;
    const name = nameOf(page);
    return (
      <li key={page.id}>
        <button
          type="button"
          className={`nav-entry ${selected ? 'nav-entry-active' : ''}`}
          aria-current={selected ? 'page' : undefined}
          // Named explicitly because the visible label is hidden on a narrow
          // viewport, where the icon alone would leave the button nameless.
          aria-label={name}
          title={name}
          onClick={() => onOpen(page.id)}
        >
          <Icon name={page.icon} />
          <span className="nav-label">{name}</span>
          {page.available ? null : <span className="nav-pending" aria-hidden="true" />}
        </button>
      </li>
    );
  }

  return (
    <nav className="sidenav" aria-label={t('nav.label')}>
      <div className="nav-upper">
        <ul className="nav-list">{working.map(entry)}</ul>
        {instruments.length === 0 ? null : (
          <>
            <p className="nav-section">{t('nav.section.instruments')}</p>
            <ul className="nav-list">{instruments.map(entry)}</ul>
          </>
        )}
      </div>
      <div className="nav-lower">
        <p className="nav-section">{t('nav.section.core')}</p>
        <ul className="nav-list">{core.map(entry)}</ul>
      </div>
    </nav>
  );
}
