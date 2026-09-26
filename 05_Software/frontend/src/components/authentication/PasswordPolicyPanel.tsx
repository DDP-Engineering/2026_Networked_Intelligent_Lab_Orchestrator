import { MessageBanner } from '../MessageBanner';
import { useCallback, useEffect, useState } from 'react';
import {
  BenchCoreError,
  type BenchCoreClient,
  type Credential,
} from '../../api/client';
import type { PasswordPolicy } from '../../api/contract';
import { useTranslate } from '../../i18n/languages';
import { Field } from '../help/Field';
import { HelpPopover } from '../help/HelpPopover';

/**
 * The appliance's password policy, beside the identities it governs.
 *
 * It is a disclosure that starts closed: the rule is set once and then left, so
 * the identity listing is what the page should open on. Inside, the minimum
 * length and the four class requirements share one wrapping line, and each
 * requirement is a checkbox that says which class it demands - a count never
 * said which, so nobody could act on it.
 *
 * The rule itself stays on the appliance. This panel submits the whole policy,
 * because a partial change would relax the part it did not send, and it reports
 * the appliance's refusal rather than restating the bounds.
 */

/** The four requirements, in the order the panel presents them. */
const CLASSES = [
  { key: 'requireLowercase', label: 'auth.policyClass.lowercase' },
  { key: 'requireUppercase', label: 'auth.policyClass.uppercase' },
  { key: 'requireDigit', label: 'auth.policyClass.digit' },
  { key: 'requireSpecial', label: 'auth.policyClass.special' },
] as const;

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
  const [draft, setDraft] = useState<PasswordPolicy | null>(null);
  const [length, setLength] = useState('');
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
    setDraft(policy);
    setLength(String(policy.minimumLength));
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
    if (draft === null) return;
    setWorking(true);
    setFailure('');
    setNotice('');
    try {
      const replaced = await client.setPasswordPolicy(credential, {
        ...draft,
        minimumLength: Number(length),
      });
      show(replaced);
      setNotice(t('auth.policySaved'));
    } catch (error) {
      // The stored policy is unchanged, so the shown values return to it rather
      // than leaving a refused value on screen as though it were in force.
      if (stored !== null) show(stored);
      setFailure(classify(error));
    } finally {
      setWorking(false);
    }
  };

  return (
    <details className="form-group admin-policy">
      <summary>{t('auth.policyTitle')}</summary>
      <section aria-label={t('auth.policyTitle')}>
        {stored === null || draft === null ? (
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
            <div className="admin-policy-line">
              {/*
                No min or max on the length. The bounds belong to the appliance,
                and a browser restating them would both drift from it and
                swallow the submission that earns the appliance's own message.
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
              <fieldset className="admin-policy-classes">
                {/* The group carries the guidance a single Field would, because
                    the requirements are one decision made across four boxes. */}
                <legend>
                  {t('auth.policyClasses')}
                  <HelpPopover name={t('auth.policyClasses')}>
                    {t('auth.help.policyClasses')}
                  </HelpPopover>
                </legend>
                {CLASSES.map((entry) => (
                  <label key={entry.key} htmlFor={`password-policy-${entry.key}`}>
                    <input
                      id={`password-policy-${entry.key}`}
                      type="checkbox"
                      checked={draft[entry.key]}
                      disabled={!canManage || working}
                      onChange={(event) =>
                        setDraft({ ...draft, [entry.key]: event.target.checked })
                      }
                    />
                    {t(entry.label)}
                  </label>
                ))}
              </fieldset>
            </div>
            {notice === '' ? null : <MessageBanner tone="info">{notice}</MessageBanner>}
            {failure === '' ? null : (
              <MessageBanner tone="error">{failure}</MessageBanner>
            )}
            <div className="admin-policy-actions">
              <button
                type="submit"
                className="button button-primary"
                disabled={!canManage || working}
                title={
                  canManage ? undefined : t('auth.permission.passwordPolicyManage')
                }
              >
                {t('auth.policySave')}
              </button>
            </div>
          </form>
        )}
      </section>
    </details>
  );
}
