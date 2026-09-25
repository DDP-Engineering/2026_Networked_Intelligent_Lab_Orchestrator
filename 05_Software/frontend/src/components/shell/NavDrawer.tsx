import type { ReactNode } from 'react';

import { useTranslate } from '../../i18n/languages';
import { Modal } from '../Modal';

export interface NavDrawerProps {
  onDismiss: () => void;
  children: ReactNode;
}

/**
 * The navigation on a constrained viewport: the same entries the rail shows,
 * with their labels, in a drawer anchored to the left edge. It is a dialog,
 * so Escape closes it and focus returns to the Menu control that opened it.
 */
export function NavDrawer({ onDismiss, children }: NavDrawerProps) {
  const t = useTranslate();
  return (
    <Modal title={t('nav.drawer')} onDismiss={onDismiss} variant="drawer">
      {children}
    </Modal>
  );
}
