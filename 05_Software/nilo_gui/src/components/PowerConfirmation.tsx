import { useState, type FormEvent } from 'react';

import type { PowerAction } from '../api/contract';
import { BenchCoreError } from '../api/client';
import { useTranslate } from '../i18n/languages';
import { Modal, ModalActions } from './Modal';

export interface PowerConfirmationProps {
  action: PowerAction;
  onConfirm: () => Promise<void>;
  onDismiss: () => void;
}

type Outcome = 'idle' | 'working' | 'accepted' | 'refused' | 'failed';

/** Confirmation and truthful outcome reporting for one disruptive power action. */
export function PowerConfirmation({
  action,
  onConfirm,
  onDismiss,
}: PowerConfirmationProps) {
  const t = useTranslate();
  const [outcome, setOutcome] = useState<Outcome>('idle');
  const suffix = action === 'restart' ? 'restart' : 'powerOff';

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (outcome === 'working' || outcome === 'accepted') return;
    setOutcome('working');
    try {
      await onConfirm();
      setOutcome('accepted');
    } catch (error) {
      setOutcome(
        error instanceof BenchCoreError && error.code !== '' ? 'refused' : 'failed',
      );
    }
  };

  return (
    <Modal
      title={t(`power.${suffix}.title`)}
      onDismiss={outcome === 'working' ? () => {} : onDismiss}
      variant="actions"
    >
      <form onSubmit={(event) => void submit(event)}>
        <p className="card-copy">{t(`power.${suffix}.body`)}</p>
        {outcome === 'accepted' ? (
          <>
            <p className="callout callout-info" role="status">
              {t(`power.${suffix}.accepted`)}
            </p>
            <div className="modal-actions">
              <button
                type="button"
                className="button button-primary"
                onClick={onDismiss}
              >
                {t('power.close')}
              </button>
            </div>
          </>
        ) : (
          <>
            {(outcome === 'failed' || outcome === 'refused') && (
              <p className="callout callout-error" role="alert">
                {t(`power.${suffix}.${outcome}`)}
              </p>
            )}
            <ModalActions
              confirmLabel={
                outcome === 'failed' || outcome === 'refused'
                  ? t('power.tryAgain')
                  : t(`power.${suffix}.confirm`)
              }
              working={outcome === 'working'}
              onDismiss={onDismiss}
              destructive={action === 'power_off'}
            />
          </>
        )}
      </form>
    </Modal>
  );
}
