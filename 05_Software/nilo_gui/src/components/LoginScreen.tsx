import { useState, type FormEvent } from 'react';

export interface LoginScreenProps {
  /** The Bench Core base URL this screen will authenticate against. */
  endpoint: string;
  /** Called with a non-empty token when the developer submits the form. */
  onSubmit: (token: string) => void;
}

/**
 * The development login screen.
 *
 * The token lives in this component's state until it is submitted and then in
 * the application shell's state; nothing here writes it anywhere durable.
 */
export function LoginScreen({ endpoint, onSubmit }: LoginScreenProps) {
  const [token, setToken] = useState('');
  const [rejected, setRejected] = useState(false);

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const candidate = token.trim();
    if (candidate === '') {
      setRejected(true);
      return;
    }
    setRejected(false);
    onSubmit(candidate);
  }

  return (
    <section className="panel login" aria-labelledby="login-title">
      <p className="eyebrow">Development interface</p>
      <h2 id="login-title">Connect to Bench Core</h2>
      <p className="panel-copy">
        Issue a development token in WSL2 with <code>nilo development issue-token</code>{' '}
        and paste it here. The token is held in memory for this page only.
      </p>

      <form className="login-form" onSubmit={handleSubmit} noValidate>
        <label htmlFor="bench-core-token">Development bearer token</label>
        <input
          id="bench-core-token"
          name="bench-core-token"
          type="password"
          autoComplete="off"
          spellCheck={false}
          value={token}
          onChange={(event) => setToken(event.target.value)}
        />
        <button type="submit">Connect</button>
      </form>

      {rejected ? (
        <p className="field-error" role="alert">
          Enter the bearer token issued by the development backend.
        </p>
      ) : null}

      <dl className="endpoint">
        <dt>Endpoint</dt>
        <dd>
          <code>{endpoint}</code>
        </dd>
      </dl>
    </section>
  );
}
