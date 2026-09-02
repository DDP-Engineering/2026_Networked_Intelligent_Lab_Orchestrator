import { LANGUAGES, useLanguage, useTranslate } from '../../i18n/languages';
import { THEMES, useTheme, type Theme } from '../../theme/themes';
import type { BenchCoreSession } from '../../api/contract';
import { Icon, type IconName } from './Icon';
import { Menu, MenuItem } from './Menu';
import { NetworkSummary } from './NetworkSummary';
import { TabStrip } from './TabStrip';
import type { PageDescriptor } from './pages';

const THEME_ICON: Record<Theme, IconName> = {
  light: 'light',
  dark: 'dark',
  system: 'system',
};

const THEME_LABEL = {
  light: 'top.appearance.light',
  dark: 'top.appearance.dark',
  system: 'top.appearance.system',
} as const;

export interface TopRowProps {
  tabs: readonly PageDescriptor[];
  active: string | null;
  session: BenchCoreSession | null;
  benchCoreReachable: boolean;
  onActivate: (id: string) => void;
  onClose: (id: string) => void;
  onOpen: (id: string) => void;
  onSignOut: () => void;
  nameOf: (page: PageDescriptor) => string;
}

/**
 * The single top row.
 *
 * Everything the shell offers above the workspace is here: the open tabs on the
 * left, and the network, simulation, appearance, language and account controls
 * on the right. There is deliberately nowhere else for chrome to go — no second
 * strip, no breadcrumb, no page title — because every row above the workspace is
 * taken from the instrument that needs the space.
 */
export function TopRow({
  tabs,
  active,
  session,
  benchCoreReachable,
  onActivate,
  onClose,
  onOpen,
  onSignOut,
  nameOf,
}: TopRowProps) {
  const t = useTranslate();
  const { theme, resolved, setTheme } = useTheme();
  const { language, setLanguage } = useLanguage();
  const short = LANGUAGES.find((entry) => entry.code === language)?.short ?? 'EN';

  return (
    <div className="toprow">
      <TabStrip
        tabs={tabs}
        active={active}
        onActivate={onActivate}
        onClose={onClose}
        nameOf={nameOf}
      />

      <div className="toprow-controls">
        <NetworkSummary
          benchCoreReachable={benchCoreReachable}
          onOpenNetwork={() => onOpen('network')}
        />

        <span
          className="sim-badge"
          role="status"
          aria-label={t('top.simulation.label')}
        >
          <span className="state-dot state-warn" aria-hidden="true" />
          {t('top.simulation')}
        </span>

        <Menu
          ariaLabel={t('top.appearance')}
          label={
            <>
              <Icon name={THEME_ICON[theme]} size={16} />
              <span className="control-label">{t(THEME_LABEL[theme])}</span>
            </>
          }
        >
          {(close) => (
            <>
              <p className="menu-title">{t('top.appearance')}</p>
              {THEMES.map((candidate) => (
                <MenuItem
                  key={candidate}
                  selected={theme === candidate}
                  icon={<Icon name={THEME_ICON[candidate]} size={16} />}
                  onSelect={() => {
                    setTheme(candidate);
                    close();
                  }}
                >
                  {t(THEME_LABEL[candidate])}
                </MenuItem>
              ))}
              <p className="menu-foot">
                {t('top.appearance.follows')}
                {theme === 'system' ? ` · ${t(THEME_LABEL[resolved])}` : ''}
              </p>
            </>
          )}
        </Menu>

        <Menu
          ariaLabel={t('top.language')}
          label={<span className="control-label">{short}</span>}
        >
          {(close) => (
            <>
              <p className="menu-title">{t('top.language')}</p>
              {LANGUAGES.map((entry) => (
                <MenuItem
                  key={entry.code}
                  selected={language === entry.code}
                  onSelect={() => {
                    setLanguage(entry.code);
                    close();
                  }}
                >
                  {entry.label}
                </MenuItem>
              ))}
            </>
          )}
        </Menu>

        {session === null ? null : (
          <Menu
            ariaLabel={t('top.account')}
            label={<span className="control-label">{session.username}</span>}
          >
            {(close) => (
              <>
                <p className="menu-title">
                  {t('top.signedInAs', { username: session.username })}
                </p>
                {/* The role is an identifier the backend matches on, so it is
                    rendered as reported rather than translated. */}
                <p className="menu-foot menu-mono">{session.role}</p>
                <hr className="menu-rule" />
                <button
                  type="button"
                  className="menu-item"
                  role="menuitem"
                  onClick={() => {
                    close();
                    onSignOut();
                  }}
                >
                  <span className="menu-item-icon" />
                  <span className="menu-item-label">{t('top.signOut')}</span>
                </button>
              </>
            )}
          </Menu>
        )}
      </div>
    </div>
  );
}
