import { useState, type FormEvent } from 'react';

import { useTranslate } from '../i18n/languages';
import type { MessageKey } from '../i18n/en';
import type { ApiMode, RuntimeProfile } from '../config/environment';

/** How a sign-in attempt that did not succeed is described to the human. */
export interface SignInRefusal {
  /** Which form the message belongs beside. */
  form: 'password' | 'token';
  /**
   * A catalogue key when this interface knows the reason, or a sentence Bench
   * Core supplied. A backend message is shown as sent rather than translated,
   * because inventing a translation for it would be a guess at its meaning.
   */
  message: MessageKey | string;
}

export interface SignInScreenProps {
  /** The Bench Core base URL this screen will authenticate against. */
  endpoint: string;
  apiMode: ApiMode;
  runtimeProfile?: RuntimeProfile;
  /** Explains why the previous session ended, when one did. */
  notice: MessageKey | null;
  /** Whether a sign-in attempt is outstanding. */
  working: boolean;
  refusal: SignInRefusal | null;
  /** Called with the credentials a human typed. */
  onSignIn: (username: string, password: string) => void;
  /** Called with a non-empty development bearer token. */
  onConnectWithToken: (token: string) => void;
}

/**
 * The unauthenticated screen.
 *
 * A human signs in with a username and password and gets a server-managed
 * session cookie; nothing about that credential is held here. The development
 * bearer token is kept beside it, deliberately secondary: it is how a developer
 * reaches a backend with no identity bootstrapped yet, and it is never required
 * for a person to sign in. Neither value is written anywhere durable — the
 * password leaves in one request body, and the token lives in application
 * memory for the page's lifetime.
 */
export function SignInScreen({
  endpoint,
  apiMode,
  runtimeProfile = 'simulation',
  notice,
  working,
  refusal,
  onSignIn,
  onConnectWithToken,
}: SignInScreenProps) {
  const t = useTranslate();
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [token, setToken] = useState('');
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

  function submitToken(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const candidate = token.trim();
    if (candidate === '') {
      setIncomplete('token');
      return;
    }
    setIncomplete('');
    onConnectWithToken(candidate);
  }

  return (
    <div className="centred">
      <section className="card" aria-labelledby="sign-in-title">
        <div className="card-head">
          <p className="eyebrow">{t('signIn.eyebrow')}</p>
          {runtimeProfile === 'simulation' && (
            <span
              className="sim-badge"
              role="status"
              aria-label={t('top.simulation.label')}
            >
              <span className="state-dot state-warn" aria-hidden="true" />
              {t('top.simulation')}
            </span>
          )}
        </div>

        <h1 id="sign-in-title">{t('signIn.title')}</h1>
        <p className="card-copy">{t('signIn.intro')}</p>

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
            <input
              id="sign-in-password"
              name="password"
              type="password"
              autoComplete="current-password"
              value={password}
              onChange={(event) => setPassword(event.target.value)}
            />
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
        {refusal?.form === 'password' ? (
          <p className="field-error" role="alert">
            {refusalText(refusal.message)}
          </p>
        ) : null}

        {/*
          Deliberately visible rather than folded away: the browser end-to-end
          suite types into this field, and a control a test has to reveal first
          is a control a developer has to hunt for.
        */}
        <div className="divider">
          <h3>{t('signIn.token.title')}</h3>
          <p className="card-copy">{t('signIn.token.intro')}</p>
          <form className="form" onSubmit={submitToken} noValidate>
            <div className="field">
              <label htmlFor="bench-core-token">{t('signIn.token.label')}</label>
              <input
                id="bench-core-token"
                name="bench-core-token"
                type="password"
                autoComplete="off"
                spellCheck={false}
                value={token}
                onChange={(event) => setToken(event.target.value)}
              />
            </div>
            <div className="form-actions">
              <button type="submit" className="button" disabled={working}>
                {t('signIn.token.submit')}
              </button>
            </div>
          </form>
          {incomplete === 'token' ? (
            <p className="field-error" role="alert">
              {t('signIn.token.incomplete')}
            </p>
          ) : null}
          {refusal?.form === 'token' ? (
            <p className="field-error" role="alert">
              {refusalText(refusal.message)}
            </p>
          ) : null}
        </div>

        <dl className="divider">
          <dt className="eyebrow">{t('signIn.endpoint')}</dt>
          <dd className="cell-mono">{endpoint}</dd>
        </dl>

        <p className="card-boundary">
          {t(
            runtimeProfile === 'simulation'
              ? 'boundary.statement'
              : runtimeProfile === 'appliance'
                ? 'boundary.appliance'
                : 'boundary.unknown',
          )}
        </p>
        <p className="mode-note">
          {apiMode === 'mock' ? t('mode.mock') : t('mode.live')}
        </p>
      </section>
    </div>
  );
}
