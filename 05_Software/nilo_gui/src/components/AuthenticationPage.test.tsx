import { fireEvent, screen, waitFor, within } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { AuthenticationPage } from './AuthenticationPage';
import { renderInShell } from '../test/render';
import { BenchCoreError, sessionCredential } from '../api/client';
import type { BenchCoreClient, Credential } from '../api/client';
import type {
  BenchCoreIdentity,
  BenchCoreSession,
  BenchCoreToken,
} from '../api/contract';

const administrator: BenchCoreIdentity = {
  id: 'identity-1',
  username: 'admin',
  role: 'administrator',
  enabled: true,
  createdAt: '2026-09-02T09:00:00Z',
};

const operator: BenchCoreIdentity = {
  id: 'identity-2',
  username: 'bench-operator',
  role: 'operator',
  enabled: true,
  createdAt: '2026-09-02T09:05:00Z',
};

const activeToken: BenchCoreToken = {
  id: 'token-1',
  name: 'bench-ci',
  owner: 'bench-operator',
  scopes: ['status.read'],
  createdAt: '2026-09-02T09:10:00Z',
  expiresAt: null,
  revoked: false,
  revokedAt: null,
};

const session: BenchCoreSession = {
  apiVersion: 'nilo.http/1',
  username: 'admin',
  role: 'administrator',
  permissions: ['users.read', 'users.manage', 'tokens.read.all', 'tokens.manage.all'],
  credential: 'session',
  csrfToken: 'csrf-token-fixture',
};

const CREDENTIAL: Credential = sessionCredential('csrf-token-fixture');

const NEW_PASSWORD = 'a-long-enough-password';

function stubClient(overrides: Partial<BenchCoreClient> = {}): BenchCoreClient {
  const unexpected = () => Promise.reject(new Error('not part of this fixture'));
  return {
    baseUrl: '',
    getStatus: unexpected,
    getIntegrations: unexpected,
    getDevices: unexpected,
    signIn: unexpected,
    currentSession: () => Promise.resolve(session),
    signOut: () => Promise.resolve(),
    listIdentities: () =>
      Promise.resolve({ apiVersion: 'nilo.http/1', items: [administrator, operator] }),
    createIdentity: () => Promise.resolve(operator),
    setIdentityRole: () => Promise.resolve(),
    setIdentityEnabled: () => Promise.resolve(),
    resetIdentityPassword: () => Promise.resolve(),
    listTokens: () =>
      Promise.resolve({ apiVersion: 'nilo.http/1', items: [activeToken] }),
    createToken: () =>
      Promise.resolve({ token: activeToken, secret: 'nilo_fixture_secret_value' }),
    revokeToken: () => Promise.resolve(),
    ...overrides,
  };
}

function renderPage(client: BenchCoreClient, onSessionEnded = vi.fn()) {
  const rendered = renderInShell(
    <AuthenticationPage
      client={client}
      credential={CREDENTIAL}
      session={session}
      onSessionEnded={onSessionEnded}
    />,
    { theme: 'light', language: 'en' },
  );
  return { ...rendered, onSessionEnded };
}

function identitiesRegion(): HTMLElement {
  return screen.getByRole('region', { name: 'Users' });
}

function tokensRegion(): HTMLElement {
  return screen.getByRole('region', { name: 'API Tokens' });
}

/** The row of the identities table naming one username. */
function identityRow(username: string): HTMLElement {
  return within(identitiesRegion()).getByRole('row', {
    name: new RegExp(`^${username}\\b`),
  });
}

function dialog(): HTMLElement {
  return screen.getByRole('dialog');
}

afterEach(() => {
  vi.restoreAllMocks();
  localStorage.clear();
  sessionStorage.clear();
});

describe('administration listings', () => {
  it('shows every identity with its role and enabled state', async () => {
    renderPage(stubClient());

    const identities = within(await screen.findByRole('region', { name: 'Users' }));
    expect(identities.getByText('admin')).toBeVisible();
    expect(identities.getByText('bench-operator')).toBeVisible();
    expect(identities.getAllByText('enabled')).toHaveLength(2);
    expect(identities.getByText('operator')).toBeVisible();
  });

  it('shows every token with its owner, scopes, expiry and revocation state', async () => {
    renderPage(stubClient());

    const tokens = within(await screen.findByRole('region', { name: 'API Tokens' }));
    expect(tokens.getByText('bench-ci')).toBeVisible();
    expect(tokens.getByText('bench-operator')).toBeVisible();
    expect(tokens.getByText('status.read')).toBeVisible();
    expect(tokens.getByText('never')).toBeVisible();
    expect(tokens.getByText('active')).toBeVisible();
  });

  it('reports each listing as loading before it answers', () => {
    renderPage(
      stubClient({
        listIdentities: () => new Promise(() => {}),
        listTokens: () => new Promise(() => {}),
      }),
    );

    expect(screen.getByText(/reading the identity listing/i)).toBeVisible();
    expect(screen.getByText(/reading the token listing/i)).toBeVisible();
  });

  it('states plainly that a listing has no entries', async () => {
    renderPage(
      stubClient({
        listTokens: () => Promise.resolve({ apiVersion: 'nilo.http/1', items: [] }),
      }),
    );

    expect(await screen.findByText(/no api tokens exist yet/i)).toBeVisible();
    expect(within(tokensRegion()).queryByRole('table')).toBeNull();
  });

  it('gives each listing its own bounded scroll compartment', async () => {
    const { container } = renderPage(stubClient());
    await screen.findByRole('region', { name: 'Users' });

    expect(container.querySelectorAll('.admin-list')).toHaveLength(2);
  });

  it('names a listing Bench Core would not serve, without emptying the other', async () => {
    renderPage(
      stubClient({
        listTokens: () =>
          Promise.reject(new BenchCoreError('forbidden', 'tokens.read.all is missing')),
      }),
    );

    await screen.findByRole('region', { name: 'Users' });
    expect(
      within(tokensRegion()).getByText(/token listing could not be read/i),
    ).toBeVisible();
    expect(
      within(tokensRegion()).getByText(/tokens\.read\.all is missing/i),
    ).toBeVisible();
    expect(within(identitiesRegion()).getByText('bench-operator')).toBeVisible();
  });
});

describe('identity administration', () => {
  it('creates an identity and reports what changed', async () => {
    const client = stubClient();
    const createIdentity = vi.spyOn(client, 'createIdentity');
    renderPage(client);
    await screen.findByRole('region', { name: 'Users' });

    fireEvent.click(screen.getByRole('button', { name: /new user/i }));
    fireEvent.change(screen.getByLabelText('Username'), {
      target: { value: 'bench-operator' },
    });
    fireEvent.change(screen.getByLabelText('Role'), { target: { value: 'operator' } });
    fireEvent.change(screen.getByLabelText('Password'), {
      target: { value: NEW_PASSWORD },
    });
    fireEvent.change(screen.getByLabelText('Confirm password'), {
      target: { value: NEW_PASSWORD },
    });
    fireEvent.click(screen.getByRole('button', { name: /create identity/i }));

    expect(
      await screen.findByText(/created bench-operator as operator/i),
    ).toBeVisible();
    expect(createIdentity).toHaveBeenCalledWith(CREDENTIAL, {
      username: 'bench-operator',
      role: 'operator',
      password: NEW_PASSWORD,
    });
  });

  it('refuses to send a password that was mistyped twice', async () => {
    const client = stubClient();
    const createIdentity = vi.spyOn(client, 'createIdentity');
    renderPage(client);
    await screen.findByRole('region', { name: 'Users' });

    fireEvent.click(screen.getByRole('button', { name: /new user/i }));
    fireEvent.change(screen.getByLabelText('Username'), { target: { value: 'jane' } });
    fireEvent.change(screen.getByLabelText('Password'), {
      target: { value: NEW_PASSWORD },
    });
    fireEvent.change(screen.getByLabelText('Confirm password'), {
      target: { value: 'something-else-entirely' },
    });
    fireEvent.click(screen.getByRole('button', { name: /create identity/i }));

    expect(await screen.findByText(/the two passwords do not match/i)).toBeVisible();
    expect(createIdentity).not.toHaveBeenCalled();
  });

  it('changes a role through a focused dialog', async () => {
    const client = stubClient();
    const setIdentityRole = vi.spyOn(client, 'setIdentityRole');
    renderPage(client);
    await screen.findByRole('region', { name: 'Users' });

    fireEvent.click(
      within(identityRow('bench-operator')).getByRole('button', {
        name: /change role/i,
      }),
    );
    fireEvent.change(within(dialog()).getByLabelText('Role'), {
      target: { value: 'viewer' },
    });
    fireEvent.click(within(dialog()).getByRole('button', { name: /change role/i }));

    expect(await screen.findByText(/bench-operator is now a viewer/i)).toBeVisible();
    expect(setIdentityRole).toHaveBeenCalledWith(
      CREDENTIAL,
      'bench-operator',
      'viewer',
    );
  });

  it('carries the reason Bench Core gave for refusing a change', async () => {
    renderPage(
      stubClient({
        setIdentityRole: () =>
          Promise.reject(
            new BenchCoreError(
              'rejected',
              'the last enabled administrator must remain an enabled administrator',
              'last_administrator',
            ),
          ),
      }),
    );
    await screen.findByRole('region', { name: 'Users' });

    fireEvent.click(
      within(identityRow('admin')).getByRole('button', { name: /change role/i }),
    );
    fireEvent.change(within(dialog()).getByLabelText('Role'), {
      target: { value: 'viewer' },
    });
    fireEvent.click(within(dialog()).getByRole('button', { name: /change role/i }));

    expect(
      await screen.findByText(/last enabled administrator must remain/i),
    ).toBeVisible();
    expect(screen.getByRole('dialog')).toBeVisible();
  });

  it('marks a change in progress and refuses a second submission', async () => {
    let release = () => {};
    renderPage(
      stubClient({
        setIdentityEnabled: () =>
          new Promise<void>((settle) => {
            release = settle;
          }),
      }),
    );
    await screen.findByRole('region', { name: 'Users' });

    fireEvent.click(
      within(identityRow('bench-operator')).getByRole('button', { name: /disable/i }),
    );
    fireEvent.click(within(dialog()).getByRole('button', { name: /^disable$/i }));

    const working = await within(dialog()).findByRole('button', { name: /working/i });
    expect(working).toBeDisabled();

    release();
    expect(await screen.findByText(/bench-operator is now disabled/i)).toBeVisible();
  });
});

describe('the dialog keyboard boundary', () => {
  /** Tab as the browser would: to the next focusable stop in document order. */
  function pressTab(shift = false): void {
    const stops = Array.from(
      document.querySelectorAll<HTMLElement>(
        'a[href], button, input, select, textarea',
      ),
    ).filter((element) => !element.hasAttribute('disabled'));
    const at = stops.indexOf(document.activeElement as HTMLElement);
    const next = shift ? stops[at - 1] : stops[at + 1];
    const contained = !fireEvent.keyDown(document.activeElement ?? document.body, {
      key: 'Tab',
      shiftKey: shift,
    });
    if (!contained) {
      next?.focus();
    }
  }

  it('keeps Tab inside the dialog in both directions', async () => {
    renderPage(stubClient());
    await screen.findByRole('region', { name: 'Users' });
    fireEvent.click(screen.getByRole('button', { name: /new user/i }));

    const panel = dialog();
    const stops = Array.from(
      panel.querySelectorAll<HTMLElement>('button, input, select, textarea'),
    ).filter((element) => !element.hasAttribute('disabled'));
    const first = stops[0] as HTMLElement;
    const last = stops[stops.length - 1] as HTMLElement;

    // Forwards off the end wraps to the first stop rather than reaching the
    // page behind, where Close Bench used to receive focus.
    last.focus();
    pressTab();
    expect(panel.contains(document.activeElement)).toBe(true);
    expect(document.activeElement).toBe(first);

    // And backwards off the front.
    first.focus();
    pressTab(true);
    expect(panel.contains(document.activeElement)).toBe(true);
    expect(document.activeElement).toBe(last);
  });
});

describe('confirmation before an irreversible change', () => {
  it.each([
    ['disable', /disable/i, /^disable$/i, 'setIdentityEnabled'],
    ['reset password', /reset password/i, /^reset password$/i, 'resetIdentityPassword'],
  ] as const)(
    'asks before it applies %s, and applies it exactly once when confirmed',
    async (_name, control, confirm, method) => {
      const client = stubClient();
      const spy = vi.spyOn(client, method);
      renderPage(client);
      await screen.findByRole('region', { name: 'Users' });

      const open = () =>
        fireEvent.click(
          within(identityRow('bench-operator')).getByRole('button', { name: control }),
        );

      open();
      expect(dialog()).toHaveTextContent('bench-operator');
      fireEvent.click(within(dialog()).getByRole('button', { name: /cancel/i }));

      await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
      expect(spy).not.toHaveBeenCalled();

      open();
      if (method === 'resetIdentityPassword') {
        // The reset dialog reads the replacement twice before it will submit.
        fireEvent.change(within(dialog()).getByLabelText('Password'), {
          target: { value: NEW_PASSWORD },
        });
        fireEvent.change(within(dialog()).getByLabelText('Confirm password'), {
          target: { value: NEW_PASSWORD },
        });
      }
      fireEvent.click(within(dialog()).getByRole('button', { name: confirm }));

      await waitFor(() => expect(spy).toHaveBeenCalledTimes(1));
      expect(spy.mock.calls[0][0]).toEqual(CREDENTIAL);
      expect(spy.mock.calls[0][1]).toBe('bench-operator');
    },
  );

  it('asks before it revokes a token, and revokes exactly once when confirmed', async () => {
    const client = stubClient();
    const revokeToken = vi.spyOn(client, 'revokeToken');
    renderPage(client);
    await screen.findByRole('region', { name: 'API Tokens' });

    fireEvent.click(within(tokensRegion()).getByRole('button', { name: /revoke/i }));
    expect(dialog()).toHaveTextContent('bench-ci');
    fireEvent.click(within(dialog()).getByRole('button', { name: /cancel/i }));
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(revokeToken).not.toHaveBeenCalled();

    fireEvent.click(within(tokensRegion()).getByRole('button', { name: /revoke/i }));
    fireEvent.click(within(dialog()).getByRole('button', { name: /revoke token/i }));

    expect(await screen.findByText(/revoked the token bench-ci/i)).toBeVisible();
    expect(revokeToken).toHaveBeenCalledExactlyOnceWith(CREDENTIAL, 'token-1');
  });
});

describe('API token creation', () => {
  it('states that a token cannot exceed its owner’s current role', async () => {
    renderPage(stubClient());
    await screen.findByRole('region', { name: 'API Tokens' });

    fireEvent.click(screen.getByRole('button', { name: /new token/i }));

    expect(within(dialog()).getByText(/can never exceed that role/i)).toBeVisible();
  });

  it('sends the selected owner, name, scopes and explicit expiry', async () => {
    const client = stubClient();
    const createToken = vi.spyOn(client, 'createToken');
    renderPage(client);
    await screen.findByRole('region', { name: 'API Tokens' });

    fireEvent.click(screen.getByRole('button', { name: /new token/i }));
    fireEvent.change(within(dialog()).getByLabelText('Owner'), {
      target: { value: 'bench-operator' },
    });
    fireEvent.change(within(dialog()).getByLabelText('Name'), {
      target: { value: 'nightly' },
    });
    fireEvent.click(within(dialog()).getByRole('checkbox', { name: 'status.read' }));
    fireEvent.change(within(dialog()).getByLabelText(/expires/i), {
      target: { value: '2030-01-02T03:04' },
    });
    fireEvent.click(within(dialog()).getByRole('button', { name: /create token/i }));

    await waitFor(() => expect(createToken).toHaveBeenCalledTimes(1));
    const [credential, request] = createToken.mock.calls[0];
    expect(credential).toEqual(CREDENTIAL);
    expect(request.owner).toBe('bench-operator');
    expect(request.name).toBe('nightly');
    expect(request.scopes).toEqual(['status.read']);
    expect(request.expiresAt).toBe(new Date('2030-01-02T03:04').toISOString());
  });

  it('discloses the secret once, copies it, and never shows it again', async () => {
    const writeText = vi.fn(() => Promise.resolve());
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText },
      configurable: true,
    });
    const stored = vi.spyOn(Storage.prototype, 'setItem');
    const { unmount } = renderPage(stubClient());
    await screen.findByRole('region', { name: 'API Tokens' });

    fireEvent.click(screen.getByRole('button', { name: /new token/i }));
    fireEvent.change(within(dialog()).getByLabelText('Name'), {
      target: { value: 'nightly' },
    });
    fireEvent.click(within(dialog()).getByRole('button', { name: /create token/i }));

    const disclosure = await screen.findByTestId('token-secret');
    expect(disclosure).toHaveTextContent('nilo_fixture_secret_value');
    expect(
      within(dialog()).getByText(/only time the secret .* is shown/i),
    ).toBeVisible();
    expect(within(dialog()).getByText(/cannot be retrieved later/i)).toBeVisible();

    fireEvent.click(within(dialog()).getByRole('button', { name: /copy secret/i }));
    await waitFor(() =>
      expect(writeText).toHaveBeenCalledExactlyOnceWith('nilo_fixture_secret_value'),
    );

    fireEvent.click(within(dialog()).getByRole('button', { name: /done/i }));
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(document.body.innerHTML).not.toContain('nilo_fixture_secret_value');

    unmount();
    renderPage(stubClient());
    await screen.findByRole('region', { name: 'API Tokens' });
    expect(document.body.innerHTML).not.toContain('nilo_fixture_secret_value');

    expect(stored).not.toHaveBeenCalled();
    expect(JSON.stringify(localStorage)).not.toContain('nilo_fixture_secret_value');
    expect(JSON.stringify(sessionStorage)).not.toContain('nilo_fixture_secret_value');
  });
});

describe('refusals', () => {
  it('ends the session when Bench Core says the credential is gone', async () => {
    const { onSessionEnded } = renderPage(
      stubClient({
        setIdentityRole: () =>
          Promise.reject(new BenchCoreError('unauthorized', 'session gone')),
      }),
    );
    await screen.findByRole('region', { name: 'Users' });

    fireEvent.click(
      within(identityRow('bench-operator')).getByRole('button', {
        name: /change role/i,
      }),
    );
    fireEvent.click(within(dialog()).getByRole('button', { name: /change role/i }));

    await waitFor(() => expect(onSessionEnded).toHaveBeenCalled());
  });

  it('keeps the identity signed in when a change is merely not permitted', async () => {
    const { onSessionEnded } = renderPage(
      stubClient({
        setIdentityRole: () =>
          Promise.reject(new BenchCoreError('forbidden', 'users.manage is missing')),
      }),
    );
    await screen.findByRole('region', { name: 'Users' });

    fireEvent.click(
      within(identityRow('bench-operator')).getByRole('button', {
        name: /change role/i,
      }),
    );
    fireEvent.click(within(dialog()).getByRole('button', { name: /change role/i }));

    expect(await screen.findByText(/users\.manage is missing/i)).toBeVisible();
    expect(onSessionEnded).not.toHaveBeenCalled();
  });
});
