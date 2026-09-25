import { useRef, type KeyboardEvent } from 'react';

import { useTranslate } from '../../i18n/languages';
import { Icon } from './Icon';
import type { PageDescriptor } from './pages';

export interface TabStripProps {
  tabs: readonly PageDescriptor[];
  active: string | null;
  onActivate: (id: string) => void;
  onClose: (id: string) => void;
  /** Resolves a page's visible name, translated for a fixed page. */
  nameOf: (page: PageDescriptor) => string;
}

/**
 * The open pages, as one scrolling row of tabs.
 *
 * The row never wraps: excess tabs scroll horizontally, because a header that
 * grows a second line would take the space this whole layout exists to give the
 * workspace. Keyboard behaviour follows the ARIA tabs pattern — one tab stop for
 * the strip, arrows to move the selection, Home and End for the ends — so a
 * person can reach every page without leaving the keyboard.
 */
export function TabStrip({ tabs, active, onActivate, onClose, nameOf }: TabStripProps) {
  const t = useTranslate();
  const strip = useRef<HTMLDivElement>(null);

  function moveTo(index: number) {
    const bounded = Math.max(0, Math.min(tabs.length - 1, index));
    const target = tabs[bounded];
    if (target === undefined) {
      return;
    }
    onActivate(target.id);
    strip.current
      ?.querySelector<HTMLElement>(`[data-tab-id="${CSS.escape(target.id)}"]`)
      ?.focus();
  }

  function onKeyDown(event: KeyboardEvent<HTMLDivElement>, index: number) {
    switch (event.key) {
      case 'ArrowRight':
        event.preventDefault();
        moveTo(index + 1);
        break;
      case 'ArrowLeft':
        event.preventDefault();
        moveTo(index - 1);
        break;
      case 'Home':
        event.preventDefault();
        moveTo(0);
        break;
      case 'End':
        event.preventDefault();
        moveTo(tabs.length - 1);
        break;
      default:
        break;
    }
  }

  return (
    <div className="tabstrip" role="tablist" aria-label={t('tabs.label')} ref={strip}>
      {tabs.map((tab, index) => {
        const name = nameOf(tab);
        const selected = tab.id === active;
        return (
          <div
            key={tab.id}
            className={`tab ${selected ? 'tab-active' : ''}`}
            role="tab"
            id={`tab-${tab.id}`}
            data-tab-id={tab.id}
            aria-selected={selected}
            aria-controls="workspace-panel"
            tabIndex={selected ? 0 : -1}
            onKeyDown={(event) => onKeyDown(event, index)}
            onClick={() => onActivate(tab.id)}
          >
            <Icon name={tab.icon} size={15} />
            <span className="tab-name">{name}</span>
            <button
              type="button"
              className="tab-close"
              aria-label={t('tabs.close', { name })}
              // The close button sits inside the tab, so its click would
              // otherwise also activate the tab it is removing.
              onClick={(event) => {
                event.stopPropagation();
                onClose(tab.id);
              }}
            >
              <Icon name="close" size={13} />
            </button>
          </div>
        );
      })}
    </div>
  );
}
