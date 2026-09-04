import { useState } from 'react';

import type { MessageKey } from '../../i18n/en';
import { useTranslate } from '../../i18n/languages';
import { Modal } from '../Modal';

/** The six things a how-to-configure help has to say, in this order. */
const SECTIONS = ['goal', 'needs', 'example', 'changes', 'external', 'verify'] as const;

export interface WorkflowHelpProps {
  /** Catalogue prefix, for example `help.dn`; the six sections hang off it. */
  prefix: 'help.dn' | 'help.if' | 'help.auth';
  /** The dialog title. */
  title: string;
}

/**
 * "How to configure": documentation for a complex workflow, opened on request.
 *
 * It states the goal, the values to have ready, a small addressing example,
 * what NILO will change, what must already be configured elsewhere, and how to
 * verify and recover. It chooses nothing and applies nothing; the editor it
 * sits in does that, with the operator.
 */
export function WorkflowHelp({ prefix, title }: WorkflowHelpProps) {
  const t = useTranslate();
  const [open, setOpen] = useState(false);

  return (
    <>
      <button type="button" className="button" onClick={() => setOpen(true)}>
        {t('help.howTo')}
      </button>
      {open ? (
        <Modal title={title} onDismiss={() => setOpen(false)}>
          <div className="workflow-help">
            {SECTIONS.map((section) => (
              <section key={section}>
                <h4>{t(`help.section.${section}` as MessageKey)}</h4>
                {t(`${prefix}.${section}` as MessageKey)
                  .split('\n')
                  .map((line, index) => (
                    <p key={index}>{line}</p>
                  ))}
              </section>
            ))}
            <div className="modal-actions">
              <button type="button" className="button" onClick={() => setOpen(false)}>
                {t('dialog.close')}
              </button>
            </div>
          </div>
        </Modal>
      ) : null}
    </>
  );
}
