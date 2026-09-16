import { useCallback, useEffect, useState } from 'react';
import {
  BenchCoreError,
  type BenchCoreClient,
  type Credential,
} from '../../api/client';
import type { PasswordPolicy } from '../../api/contract';
import { useTranslate } from '../../i18n/languages';
import { Field } from '../help/Field';

/**
 * The appliance's password policy, shown and edited beside the identities it
 * governs. The rule itself lives on the appliance: this panel submits both
 * values together, because a partial change would relax the half it did not
 * send, and it reports the appliance's refusal rather than restating the bounds.
 *
 * The policy applies when a password is set and never when one is checked, so
 * changing it here never affects who can sign in today.
 */
export function PasswordPolicyPanel({
  client,
  credential,
  canManage,
  onSessionEnded,
}: {
  client: BenchCoreClient;
  credential: Credential;
  canManage: boolean;
  onSessionEnded: () => void;
}) {
  const t = useTranslate();
  const [stored, setStored] = useState<PasswordPolicy | null>(null);
  const [length, setLength] = useState('');
  const [classes, setClasses] = useState('');
  const [working, setWorking] = useState(false);
  const [failure, setFailure] = useState('');
  const [notice, setNotice] = useState('');

  const classify = useCallback(
    (error: unknown) => {
      if (error instanceof BenchCoreError && error.failure === 'unauthorized') {
        onSessionEnded();
        return t('auth.sessionEnded');
      }
      return error instanceof BenchCoreError
        ? error.detail
        : error instanceof Error
          ? error.message
          : String(error);
    },
    [onSessionEnded, t],
  );

  const show = useCallback((policy: PasswordPolicy) => {
    setStored(policy);
    setLength(String(policy.minimumLength));
    setClasses(String(policy.minimumCharacterClasses));
  }, []);

  useEffect(() => {
    let abandoned = false;
    void client
      .getPasswordPolicy(credential)
      .then((policy) => {
        if (!abandoned) show(policy);
      })
      .catch((error: unknown) => {
        if (!abandoned) setFailure(classify(error));
      });
    return () => {
      abandoned = true;
    };
  }, [classify, client, credential, show]);

  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    setWorking(true);
    setFailure('');
    setNotice('');
    try {
      const replaced = await client.setPasswordPolicy(credential, {
        minimumLength: Number(length),
        minimumCharacterClasses: Number(classes),
      });
      show(replaced);
      setNotice(t('auth.policySaved'));
    } catch (error) {
      // The stored policy is unchanged, so the shown values return to it rather
      // than leaving a refused number on screen as though it were in force.
      if (stored !== null) show(stored);
      setFailure(classify(error));
    } finally {
      setWorking(false);
    }
  };

  return (
    <section className="admin-panel" aria-labelledby="password-policy-title">
      <header className="admin-head">
        <h3 id="password-policy-title">{t('auth.policyTitle')}</h3>
      </header>
      <p className="admin-message">{t('auth.policyNote')}</p>
      {stored === null ? (
        failure === '' ? (
          <p className="admin-message" role="status">
            {t('auth.reading', { resource: t('auth.resource.passwordPolicy') })}
          </p>
        ) : (
          <div className="admin-message">
            <p role="alert">
              {t('auth.readFailed', { resource: t('auth.resource.passwordPolicy') })}
            </p>
            <p className="failure-detail">{failure}</p>
          </div>
        )
      ) : (
        <form className="admin-policy-form" onSubmit={(event) => void submit(event)}>
          {/*
            No min or max on either control. The bounds belong to the appliance,
            and a browser restating them would both drift from it and swallow
            the submission that earns the appliance's own message.
          */}
          <Field
            id="password-policy-length"
            label={t('auth.policyMinimumLength')}
            help={t('auth.help.policyMinimumLength')}
          >
            <input
              id="password-policy-length"
              type="number"
              inputMode="numeric"
              value={length}
              disabled={!canManage || working}
              onChange={(event) => setLength(event.target.value)}
            />
          </Field>
          <Field
            id="password-policy-classes"
            label={t('auth.policyMinimumClasses')}
            help={t('auth.help.policyMinimumClasses')}
          >
            <input
              id="password-policy-classes"
              type="number"
              inputMode="numeric"
              value={classes}
              disabled={!canManage || working}
              onChange={(event) => setClasses(event.target.value)}
            />
          </Field>
          {notice === '' ? null : (
            <p className="field-notice" role="status">
              {notice}
            </p>
          )}
          {failure === '' ? null : (
            <p className="field-error" role="alert">
              {failure}
            </p>
          )}
          <div className="admin-policy-actions">
            <button
              type="submit"
              className="button button-primary"
              disabled={!canManage || working}
              title={canManage ? undefined : t('auth.permission.passwordPolicyManage')}
            >
              {t('auth.policySave')}
            </button>
          </div>
        </form>
      )}
    </section>
  );
}
