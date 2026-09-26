import { useRef, useState } from 'react';

import { useTranslate } from '../../i18n/languages';

interface NavigationSplitterProps {
  width: number;
  min: number;
  max: number;
  onResize: (width: number) => void;
  onReset: () => void;
}

/** Pointer capture keeps resizing active when the pointer leaves the divider. */
export function NavigationSplitter({
  width,
  min,
  max,
  onResize,
  onReset,
}: NavigationSplitterProps) {
  const t = useTranslate();
  const drag = useRef<{ pointerId: number; x: number; width: number } | null>(null);
  const [dragging, setDragging] = useState(false);

  return (
    <div
      className={`navigation-splitter${dragging ? ' navigation-splitter-dragging' : ''}`}
      role="separator"
      tabIndex={0}
      aria-label={t('nav.resize')}
      aria-orientation="vertical"
      aria-controls="main-navigation"
      aria-valuemin={min}
      aria-valuemax={max}
      aria-valuenow={width}
      aria-valuetext={t('nav.resize.value', { width: String(width) })}
      title={t('nav.resize.help')}
      onPointerDown={(event) => {
        if (!event.isPrimary || event.button !== 0) return;
        event.preventDefault();
        event.currentTarget.focus();
        event.currentTarget.setPointerCapture(event.pointerId);
        drag.current = { pointerId: event.pointerId, x: event.clientX, width };
        setDragging(true);
      }}
      onPointerMove={(event) => {
        const start = drag.current;
        if (start?.pointerId === event.pointerId) {
          onResize(start.width + event.clientX - start.x);
        }
      }}
      onPointerUp={(event) => {
        if (drag.current?.pointerId === event.pointerId) {
          drag.current = null;
          setDragging(false);
          event.currentTarget.releasePointerCapture(event.pointerId);
        }
      }}
      onPointerCancel={() => {
        drag.current = null;
        setDragging(false);
      }}
      onLostPointerCapture={() => {
        drag.current = null;
        setDragging(false);
      }}
      onDoubleClick={onReset}
      onKeyDown={(event) => {
        const step = event.shiftKey ? 32 : 8;
        switch (event.key) {
          case 'ArrowLeft':
            onResize(width - step);
            break;
          case 'ArrowRight':
            onResize(width + step);
            break;
          case 'Home':
            onResize(min);
            break;
          case 'End':
            onResize(max);
            break;
          case 'Enter':
            onReset();
            break;
          default:
            return;
        }
        event.preventDefault();
      }}
    >
      <span className="navigation-splitter-grip" aria-hidden="true" />
    </div>
  );
}
