import type { ReactNode } from 'react';
import { useTranslate } from '../i18n/languages';

/** Shared, explicitly labelled feedback for application actions. */
export function MessageBanner({
  tone,
  children,
}: {
  tone: 'info' | 'warning' | 'error';
  children: ReactNode;
}) {
  const t = useTranslate();
  return (
    <div
      className={`message-banner message-banner-${tone}`}
      role={tone === 'error' ? 'alert' : 'status'}
    >
      <strong>{t(`message.${tone}`)}:</strong> {children}
    </div>
  );
}
