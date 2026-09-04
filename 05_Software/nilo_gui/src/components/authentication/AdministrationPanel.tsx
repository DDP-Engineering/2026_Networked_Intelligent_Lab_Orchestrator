import type { ReactNode } from 'react';
import { useTranslate } from '../../i18n/languages';

export type Listing<T> =
  | { phase: 'loading' }
  | { phase: 'ready'; items: T[] }
  | { phase: 'failed'; detail: string };

export function AdministrationPanel<T>({
  id,
  title,
  action,
  listing,
  empty,
  resource,
  children,
}: {
  id: string;
  title: string;
  action: ReactNode;
  listing: Listing<T>;
  empty: string;
  resource: string;
  children: ReactNode;
}) {
  const t = useTranslate();
  return (
    <section className="admin-panel" aria-labelledby={`${id}-title`}>
      <header className="admin-head">
        <h3 id={`${id}-title`}>{title}</h3>
        <div className="admin-head-actions">{action}</div>
      </header>
      {listing.phase === 'loading' ? (
        <p className="admin-message" role="status">
          {t('auth.reading', { resource })}
        </p>
      ) : listing.phase === 'failed' ? (
        <div className="admin-message">
          <p role="alert">{t('auth.readFailed', { resource })}</p>
          <p className="failure-detail">{listing.detail}</p>
        </div>
      ) : listing.items.length === 0 ? (
        <p className="admin-message">{empty}</p>
      ) : (
        <div className="admin-list">{children}</div>
      )}
    </section>
  );
}
