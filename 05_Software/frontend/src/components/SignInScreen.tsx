import { useState, type FormEvent } from 'react';

import { useTranslate } from '../i18n/languages';
import type { MessageKey } from '../i18n/en';
import { NILO_VERSION } from '../version';

/** How a sign-in attempt that did not succeed is described to the human. */
export interface SignInRefusal {
  /**
   * A catalogue key when this interface knows the reason, or a sentence Bench
   * Core supplied. A backend message is shown as sent rather than translated,
   * because inventing a translation for it would be a guess at its meaning.
   */
  message: MessageKey | string;
}

export interface SignInScreenProps {
  /** Explains why the previous session ended, when one did. */
  notice: MessageKey | null;
  /** Whether a sign-in attempt is outstanding. */
  working: boolean;
  refusal: SignInRefusal | null;
  /** Called with the credentials a human typed. */
  onSignIn: (username: string, password: string) => void;
}

/**
 * The unauthenticated screen.
 *
 * A human signs in with a username and password and gets a server-managed
 * session cookie; nothing about that credential is held here. The password is
 * local, ephemeral form state and leaves in one request body.
 */
export function SignInScreen({
  notice,
  working,
  refusal,
  onSignIn,
}: SignInScreenProps) {
  const t = useTranslate();
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [passwordVisible, setPasswordVisible] = useState(false);
  const [incomplete, setIncomplete] = useState('');

  /** A refusal is a key when this interface produced it, and prose when Bench Core did. */
  function refusalText(value: MessageKey | string): string {
    const translated = t(value as MessageKey);
    return translated === undefined ? String(value) : translated;
  }

  function submitCredentials(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (username.trim() === '' || password === '') {
      setIncomplete('password');
      return;
    }
    setIncomplete('');
    onSignIn(username.trim(), password);
  }

  return (
    <div className="centred">
      <section className="card sign-in-card" aria-labelledby="sign-in-title">
        <div className="sign-in-brand">
          <p className="sign-in-wordmark">{t('app.name')}</p>
          <p className="sign-in-version">
            {t('signIn.version', { version: NILO_VERSION })}
          </p>
        </div>

        <h1 id="sign-in-title">{t('signIn.title')}</h1>

        {notice === null ? null : (
          <p className="field-notice" role="status">
            {t(notice)}
          </p>
        )}

        <form className="form" onSubmit={submitCredentials} noValidate>
          <div className="field">
            <label htmlFor="sign-in-username">{t('signIn.username')}</label>
            <input
              id="sign-in-username"
              name="username"
              type="text"
              autoComplete="username"
              spellCheck={false}
              value={username}
              onChange={(event) => setUsername(event.target.value)}
            />
          </div>
          <div className="field">
            <label htmlFor="sign-in-password">{t('signIn.password')}</label>
            <div className="password-control">
              <input
                id="sign-in-password"
                name="password"
                type={passwordVisible ? 'text' : 'password'}
                autoComplete="current-password"
                value={password}
                onChange={(event) => setPassword(event.target.value)}
              />
              <button
                type="button"
                className="button password-toggle"
                aria-controls="sign-in-password"
                aria-pressed={passwordVisible}
                onClick={() => setPasswordVisible((visible) => !visible)}
              >
                {t(passwordVisible ? 'signIn.password.hide' : 'signIn.password.show')}
              </button>
            </div>
          </div>
          <div className="form-actions">
            <button type="submit" className="button button-primary" disabled={working}>
              {working ? t('signIn.working') : t('signIn.submit')}
            </button>
          </div>
        </form>

        {incomplete === 'password' ? (
          <p className="field-error" role="alert">
            {t('signIn.incomplete')}
          </p>
        ) : null}
        {refusal !== null ? (
          <p className="field-error" role="alert">
            {refusalText(refusal.message)}
          </p>
        ) : null}
      </section>
    </div>
  );
}
