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
    requestPower: () => Promise.reject(new Error('not part of this fixture')),
    listPlugins: async () => [],
    listInstrumentJobs: async () => [],
    getInstrumentJob: async () => {
      throw new Error('unavailable');
    },
    pluginAction: async () => {
      throw new Error('unavailable');
    },
    discoverInstruments: async () => [],
    submitInstrumentJob: async () => {
      throw new Error('unavailable');
    },
    instrumentJobAction: async () => {
      throw new Error('unavailable');
    },
    downloadInstrumentArtifact: async () => {
      throw new Error('unavailable');
    },
    listNetworkOperations: () => Promise.resolve([]),
    previewInterfaceSettings: () =>
      Promise.reject(new Error('not part of this fixture')),
    scanWiFi: () => Promise.reject(new Error('not part of this fixture')),
    previewWiFiJoin: () => Promise.reject(new Error('not part of this fixture')),
    listDeviceNetworks: () => Promise.resolve([]),
    previewDeviceNetworkChange: () =>
      Promise.reject(new Error('not part of this fixture')),
    checkDeviceNetwork: () => Promise.reject(new Error('not part of this fixture')),
    getAccessInstructions: () => Promise.reject(new Error('not part of this fixture')),
    listActivity: () => Promise.reject(new Error('not part of this fixture')),
    exportActivity: () => Promise.reject(new Error('not part of this fixture')),
    getActivityRetention: () => Promise.reject(new Error('not part of this fixture')),
    setActivityRetention: () => Promise.reject(new Error('not part of this fixture')),
    getStatus: unexpected,
    getIntegrations: unexpected,
    getDevices: unexpected,
    getWireGuardStatus: () => Promise.reject(new Error('no network')),
    previewWireGuardChange: () => Promise.reject(new Error('no network')),
    getNetworkOperation: () => Promise.reject(new Error('no network')),
    getNetworkStatus: unexpected,
    getManagementAccess: unexpected,
    setManagementAccess: unexpected,
    confirmManagementAccess: unexpected,
    recoverManagementAccess: unexpected,
    getNetworkStatistics: unexpected,
    previewInterfaceChange: unexpected,
    acknowledgeNetworkOperation: unexpected,
    confirmNetworkOperation: unexpected,
    rollbackNetworkOperation: unexpected,
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
    deleteIdentity: () => Promise.resolve(),
    deleteToken: () => Promise.resolve(),
    ...overrides,
  };
}

function renderPage(
  client: BenchCoreClient,
  onSessionEnded = vi.fn(),
  view: 'users' | 'tokens' = 'users',
) {
  const rendered = renderInShell(
    <AuthenticationPage
      client={client}
      credential={CREDENTIAL}
      session={session}
      view={view}
      onSessionEnded={onSessionEnded}
    />,
    { theme: 'light', language: 'en' },
  );
  return { ...rendered, onSessionEnded };
}

function identitiesRegion(): HTMLElement {
  return screen.getByRole('region', { name: 'Users' });
}

describe('page-scoped Activity placement', () => {
  it.each(['users', 'tokens'] as const)(
    'groups %s activity beside its title',
    async (view) => {
      const onOpenActivity = vi.fn();
      renderInShell(
        <AuthenticationPage
          client={stubClient()}
          credential={CREDENTIAL}
          session={session}
          view={view}
          onSessionEnded={vi.fn()}
          onOpenActivity={onOpenActivity}
        />,
        { theme: 'light', language: 'en' },
      );
      const title = screen.getByRole('heading', { level: 2 });
      const group = title.closest('.page-title-actions');
      expect(group).not.toBeNull();
      const action = within(group as HTMLElement).getByRole('button', {
        name: 'View activity',
      });
      expect(screen.getAllByRole('button', { name: 'View activity' })).toHaveLength(1);
      expect(title.nextElementSibling).toBe(action);
      expect(group?.nextElementSibling).toHaveClass('page-sub');
      fireEvent.click(action);
      expect(onOpenActivity).toHaveBeenCalledOnce();
      await screen.findByRole('region', {
        name: view === 'users' ? 'Users' : 'API Tokens',
      });
    },
  );

  it.each(['users', 'tokens'] as const)(
    'omits unavailable %s activity',
    async (view) => {
      renderPage(stubClient(), vi.fn(), view);
      await screen.findByRole('region', {
        name: view === 'users' ? 'Users' : 'API Tokens',
      });
      expect(
        screen.queryByRole('button', { name: 'View activity' }),
      ).not.toBeInTheDocument();
    },
  );
});

function tokensRegion(): HTMLElement {
  return screen.getByRole('region', { name: 'API Tokens' });
}

/** The row of the identities table naming one username. */
function identityRow(username: string): HTMLElement {
  return within(identitiesRegion()).getByRole('row', {
    name: new RegExp(`^${username}\\b`),
  });
}

function openIdentityAction(username: string, action: RegExp): void {
  fireEvent.click(
    within(identityRow(username)).getByRole('button', {
      name: `Actions for ${username}`,
    }),
  );
  fireEvent.click(within(dialog()).getByRole('button', { name: action }));
}

function openTokenAction(name: string, action: RegExp): void {
  fireEvent.click(screen.getByRole('button', { name: `Actions for ${name}` }));
  fireEvent.click(within(dialog()).getByRole('button', { name: action }));
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
  it('lets the Users destination own identity loading without fetching tokens', async () => {
    const client = stubClient();
    const listTokens = vi.spyOn(client, 'listTokens');
    renderPage(client, vi.fn(), 'users');

    expect(
      await screen.findByRole('heading', { name: 'Authentication / Users' }),
    ).toBeVisible();
    expect(screen.getByRole('region', { name: 'Users' })).toBeVisible();
    expect(screen.queryByRole('region', { name: 'API Tokens' })).toBeNull();
    expect(listTokens).not.toHaveBeenCalled();
  });

  it('shows every identity with its role and enabled state', async () => {
    renderPage(stubClient());

    const identities = within(await screen.findByRole('region', { name: 'Users' }));
    expect(identities.getByText('admin')).toBeVisible();
    expect(identities.getByText('bench-operator')).toBeVisible();
    expect(identities.getAllByText('enabled')).toHaveLength(2);
    expect(identities.getByText('operator')).toBeVisible();
  });

  it('shows every token with its owner, scopes, expiry and revocation state', async () => {
    renderPage(stubClient(), vi.fn(), 'tokens');

    const tokens = within(await screen.findByRole('region', { name: 'API Tokens' }));
    expect(tokens.getByText('bench-ci')).toBeVisible();
    expect(tokens.getByText('bench-operator')).toBeVisible();
    fireEvent.click(tokens.getByRole('button', { name: 'Details bench-ci' }));
    expect(tokens.getByText('status.read')).toBeVisible();
    expect(tokens.getByText('never')).toBeVisible();
    expect(tokens.getByText('active')).toBeVisible();
  });

  it('reports the focused listing as loading before it answers', () => {
    renderPage(
      stubClient({
        listIdentities: () => new Promise(() => {}),
      }),
    );

    expect(screen.getByText(/reading the identity listing/i)).toBeVisible();
    expect(screen.queryByText(/reading the token listing/i)).toBeNull();
  });

  it('states plainly that a listing has no entries', async () => {
    renderPage(
      stubClient({
        listTokens: () => Promise.resolve({ apiVersion: 'nilo.http/1', items: [] }),
      }),
      vi.fn(),
      'tokens',
    );

    expect(await screen.findByText(/no api tokens exist yet/i)).toBeVisible();
    expect(within(tokensRegion()).queryByRole('table')).toBeNull();
  });

  it('renders one focused listing in normal page flow', async () => {
    const { container } = renderPage(stubClient());
    await screen.findByRole('region', { name: 'Users' });

    expect(container.querySelectorAll('.admin-list')).toHaveLength(1);
  });

  it('names a listing Bench Core would not serve, without emptying the other', async () => {
    renderPage(
      stubClient({
        listTokens: () =>
          Promise.reject(new BenchCoreError('forbidden', 'tokens.read.all is missing')),
      }),
      vi.fn(),
      'tokens',
    );

    expect(await screen.findByText(/token listing could not be read/i)).toBeVisible();
    expect(
      within(tokensRegion()).getByText(/tokens\.read\.all is missing/i),
    ).toBeVisible();
    expect(screen.queryByRole('region', { name: 'Users' })).toBeNull();
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

    openIdentityAction('bench-operator', /change role/i);
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

    openIdentityAction('admin', /change role/i);
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

    openIdentityAction('bench-operator', /disable/i);
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

      const open = () => openIdentityAction('bench-operator', control);

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
    renderPage(client, vi.fn(), 'tokens');
    await screen.findByRole('region', { name: 'API Tokens' });

    openTokenAction('bench-ci', /revoke/i);
    expect(dialog()).toHaveTextContent('bench-ci');
    fireEvent.click(within(dialog()).getByRole('button', { name: /cancel/i }));
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(revokeToken).not.toHaveBeenCalled();

    openTokenAction('bench-ci', /revoke/i);
    fireEvent.click(within(dialog()).getByRole('button', { name: /revoke token/i }));

    expect(await screen.findByText(/revoked the token bench-ci/i)).toBeVisible();
    expect(revokeToken).toHaveBeenCalledExactlyOnceWith(CREDENTIAL, 'token-1');
  });
});

describe('permanent deletion', () => {
  const secondAdministrator: BenchCoreIdentity = {
    id: 'identity-3',
    username: 'second-admin',
    role: 'administrator',
    enabled: true,
    createdAt: '2026-09-02T09:15:00Z',
  };
  const revokedToken: BenchCoreToken = {
    id: 'token-2',
    name: 'retired-ci',
    owner: 'bench-operator',
    scopes: [],
    createdAt: '2026-09-02T09:20:00Z',
    expiresAt: null,
    revoked: true,
    revokedAt: '2026-09-02T09:30:00Z',
  };

  /** Open one row's Actions dialog without choosing anything in it. */
  function openIdentityActions(username: string): void {
    fireEvent.click(
      within(identityRow(username)).getByRole('button', {
        name: `Actions for ${username}`,
      }),
    );
  }

  it('offers deletion in the destructive section, apart from the ordinary actions', async () => {
    renderPage(stubClient());
    await screen.findByRole('region', { name: 'Users' });

    openIdentityActions('bench-operator');
    const destructive = dialog().querySelector('.action-dialog-danger-list');
    expect(destructive).not.toBeNull();
    expect(
      within(destructive as HTMLElement).getByRole('button', {
        name: /delete permanently/i,
      }),
    ).toBeVisible();
    // The reversible action stays in the ordinary group above it.
    const ordinary = dialog().querySelectorAll('.action-dialog-list')[0];
    expect(
      within(ordinary as HTMLElement).getByRole('button', { name: /change role/i }),
    ).toBeVisible();
  });

  it('warns that the removal is permanent and sends nothing when dismissed', async () => {
    const client = stubClient();
    const deleteIdentity = vi.spyOn(client, 'deleteIdentity');
    renderPage(client);
    await screen.findByRole('region', { name: 'Users' });

    openIdentityAction('bench-operator', /delete permanently/i);
    expect(dialog()).toHaveTextContent('bench-operator');
    expect(within(dialog()).getByText(/cannot be undone/i)).toBeVisible();
    expect(
      within(dialog()).getAllByText(/removed from storage/i).length,
    ).toBeGreaterThan(0);

    fireEvent.click(within(dialog()).getByRole('button', { name: /cancel/i }));
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(deleteIdentity).not.toHaveBeenCalled();
  });

  it('holds the deletion back until the exact username is typed', async () => {
    const client = stubClient();
    const deleteIdentity = vi.spyOn(client, 'deleteIdentity');
    renderPage(client);
    await screen.findByRole('region', { name: 'Users' });

    openIdentityAction('bench-operator', /delete permanently/i);
    const confirm = () =>
      within(dialog()).getByRole('button', { name: /^delete permanently$/i });
    expect(confirm()).toBeDisabled();

    fireEvent.change(within(dialog()).getByLabelText('Confirm the username'), {
      target: { value: 'bench-operato' },
    });
    expect(confirm()).toBeDisabled();
    fireEvent.change(within(dialog()).getByLabelText('Confirm the username'), {
      target: { value: 'admin' },
    });
    expect(confirm()).toBeDisabled();
    // Exact means exact: a value that merely reads the same is not the name.
    fireEvent.change(within(dialog()).getByLabelText('Confirm the username'), {
      target: { value: ' bench-operator ' },
    });
    expect(confirm()).toBeDisabled();
    fireEvent.change(within(dialog()).getByLabelText('Confirm the username'), {
      target: { value: 'Bench-Operator' },
    });
    expect(confirm()).toBeDisabled();

    fireEvent.change(within(dialog()).getByLabelText('Confirm the username'), {
      target: { value: 'bench-operator' },
    });
    expect(confirm()).toBeEnabled();
    fireEvent.click(confirm());

    expect(
      await screen.findByText(/deleted the identity bench-operator permanently/i),
    ).toBeVisible();
    expect(deleteIdentity).toHaveBeenCalledExactlyOnceWith(
      CREDENTIAL,
      'bench-operator',
    );
  });

  it('re-reads only the identity listing after a deletion', async () => {
    const client = stubClient();
    const listIdentities = vi.spyOn(client, 'listIdentities');
    const listTokens = vi.spyOn(client, 'listTokens');
    renderPage(client);
    await screen.findByRole('region', { name: 'Users' });
    expect(listIdentities).toHaveBeenCalledTimes(1);

    openIdentityAction('bench-operator', /delete permanently/i);
    fireEvent.change(within(dialog()).getByLabelText('Confirm the username'), {
      target: { value: 'bench-operator' },
    });
    fireEvent.click(
      within(dialog()).getByRole('button', { name: /^delete permanently$/i }),
    );

    await waitFor(() => expect(listIdentities).toHaveBeenCalledTimes(2));
    expect(listTokens).not.toHaveBeenCalled();
  });

  it('says why the last enabled administrator cannot be deleted', async () => {
    const client = stubClient();
    const deleteIdentity = vi.spyOn(client, 'deleteIdentity');
    renderPage(client);
    await screen.findByRole('region', { name: 'Users' });

    openIdentityActions('admin');
    const remove = within(dialog()).getByRole('button', {
      name: /delete permanently/i,
    });
    expect(remove).toBeDisabled();
    expect(
      within(dialog()).getByText(/last enabled administrator cannot be deleted/i),
    ).toBeVisible();

    fireEvent.click(remove);
    expect(deleteIdentity).not.toHaveBeenCalled();
  });

  it('offers deletion of an administrator once another enabled one exists', async () => {
    renderPage(
      stubClient({
        listIdentities: () =>
          Promise.resolve({
            apiVersion: 'nilo.http/1',
            items: [administrator, secondAdministrator],
          }),
      }),
    );
    await screen.findByRole('region', { name: 'Users' });

    openIdentityActions('admin');
    expect(
      within(dialog()).getByRole('button', { name: /delete permanently/i }),
    ).toBeEnabled();
  });

  it('explains a refusal caused by API tokens the identity still owns', async () => {
    renderPage(
      stubClient({
        deleteIdentity: () =>
          Promise.reject(
            new BenchCoreError(
              'rejected',
              'delete every API token this identity owns before deleting the identity',
              'user_has_tokens',
            ),
          ),
      }),
    );
    await screen.findByRole('region', { name: 'Users' });

    openIdentityAction('bench-operator', /delete permanently/i);
    fireEvent.change(within(dialog()).getByLabelText('Confirm the username'), {
      target: { value: 'bench-operator' },
    });
    fireEvent.click(
      within(dialog()).getByRole('button', { name: /^delete permanently$/i }),
    );

    expect(
      await screen.findByText(/an identity that owns api tokens cannot be deleted/i),
    ).toBeVisible();
    expect(screen.getByRole('dialog')).toBeVisible();
  });

  it('returns to sign-in when an administrator deletes their own account', async () => {
    const client = stubClient({
      listIdentities: () =>
        Promise.resolve({
          apiVersion: 'nilo.http/1',
          items: [administrator, secondAdministrator],
        }),
    });
    const listIdentities = vi.spyOn(client, 'listIdentities');
    const { onSessionEnded } = renderPage(client);
    await screen.findByRole('region', { name: 'Users' });

    openIdentityAction('admin', /delete permanently/i);
    fireEvent.change(within(dialog()).getByLabelText('Confirm the username'), {
      target: { value: 'admin' },
    });
    fireEvent.click(
      within(dialog()).getByRole('button', { name: /^delete permanently$/i }),
    );

    await waitFor(() => expect(onSessionEnded).toHaveBeenCalled());
    // The credential that read the listing is gone, so nothing re-reads it.
    expect(listIdentities).toHaveBeenCalledTimes(1);
  });

  it('keeps Revoke and Delete distinct on a token, and offers both', async () => {
    renderPage(stubClient(), vi.fn(), 'tokens');
    await screen.findByRole('region', { name: 'API Tokens' });

    fireEvent.click(screen.getByRole('button', { name: 'Actions for bench-ci' }));
    const destructive = dialog().querySelector('.action-dialog-danger-list');
    expect(
      within(destructive as HTMLElement).getByRole('button', { name: /^revoke$/i }),
    ).toBeEnabled();
    expect(
      within(destructive as HTMLElement).getByRole('button', {
        name: /delete permanently/i,
      }),
    ).toBeEnabled();
  });

  it('deletes a revoked token, which can no longer be revoked', async () => {
    const client = stubClient({
      listTokens: () =>
        Promise.resolve({ apiVersion: 'nilo.http/1', items: [revokedToken] }),
    });
    const deleteToken = vi.spyOn(client, 'deleteToken');
    renderPage(client, vi.fn(), 'tokens');
    await screen.findByRole('region', { name: 'API Tokens' });

    fireEvent.click(screen.getByRole('button', { name: 'Actions for retired-ci' }));
    expect(within(dialog()).getByRole('button', { name: /^revoke$/i })).toBeDisabled();
    fireEvent.click(
      within(dialog()).getByRole('button', { name: /delete permanently/i }),
    );

    expect(within(dialog()).getByText(/cannot be undone/i)).toBeVisible();
    const confirm = () =>
      within(dialog()).getByRole('button', { name: /^delete permanently$/i });
    expect(confirm()).toBeDisabled();
    fireEvent.change(within(dialog()).getByLabelText('Confirm the token name'), {
      target: { value: 'retired-ci' },
    });
    fireEvent.click(confirm());

    expect(
      await screen.findByText(/deleted the token retired-ci permanently/i),
    ).toBeVisible();
    expect(deleteToken).toHaveBeenCalledExactlyOnceWith(CREDENTIAL, 'token-2');
  });

  it('sends no token deletion when the dialog is dismissed', async () => {
    const client = stubClient();
    const deleteToken = vi.spyOn(client, 'deleteToken');
    renderPage(client, vi.fn(), 'tokens');
    await screen.findByRole('region', { name: 'API Tokens' });

    openTokenAction('bench-ci', /delete permanently/i);
    expect(dialog()).toHaveTextContent('bench-ci');
    fireEvent.click(within(dialog()).getByRole('button', { name: /cancel/i }));

    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(deleteToken).not.toHaveBeenCalled();
  });

  it('offers neither deletion to a session that may not manage', async () => {
    renderInShell(
      <AuthenticationPage
        client={stubClient()}
        credential={CREDENTIAL}
        session={session}
        view="users"
        canManageUsers={false}
        onSessionEnded={vi.fn()}
      />,
      { theme: 'light', language: 'en' },
    );
    await screen.findByRole('region', { name: 'Users' });

    openIdentityActions('bench-operator');
    expect(
      within(dialog()).getByRole('button', { name: /delete permanently/i }),
    ).toBeDisabled();
    // Every action the session may not take says the same thing, beside it.
    expect(
      within(dialog()).getAllByText(/does not include users\.manage/i).length,
    ).toBe(4);
  });
});

describe('API token creation', () => {
  it('does not offer session-only appliance power as an API token scope', async () => {
    renderPage(stubClient(), vi.fn(), 'tokens');
    await screen.findByRole('region', { name: 'API Tokens' });

    fireEvent.click(screen.getByRole('button', { name: /new token/i }));

    expect(
      within(dialog()).queryByRole('checkbox', { name: 'system.power.manage' }),
    ).toBeNull();
  });

  it('states that a token cannot exceed its owner’s current role', async () => {
    renderPage(stubClient(), vi.fn(), 'tokens');
    await screen.findByRole('region', { name: 'API Tokens' });

    fireEvent.click(screen.getByRole('button', { name: /new token/i }));

    expect(within(dialog()).getByText(/can never exceed that role/i)).toBeVisible();
  });

  it('sends the selected owner, name, scopes and explicit expiry', async () => {
    const client = stubClient();
    const createToken = vi.spyOn(client, 'createToken');
    renderPage(client, vi.fn(), 'tokens');
    await screen.findByRole('region', { name: 'API Tokens' });

    fireEvent.click(screen.getByRole('button', { name: /new token/i }));
    fireEvent.change(within(dialog()).getByLabelText('Owner'), {
      target: { value: 'bench-operator' },
    });
    fireEvent.change(within(dialog()).getByLabelText('Name'), {
      target: { value: 'nightly' },
    });
    fireEvent.click(within(dialog()).getByRole('checkbox', { name: 'status.read' }));
    fireEvent.change(within(dialog()).getByLabelText('Expires (optional)'), {
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
    const { unmount } = renderPage(stubClient(), vi.fn(), 'tokens');
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
    renderPage(stubClient(), vi.fn(), 'tokens');
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

    openIdentityAction('bench-operator', /change role/i);
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

    openIdentityAction('bench-operator', /change role/i);
    fireEvent.click(within(dialog()).getByRole('button', { name: /change role/i }));

    expect(await screen.findByText(/users\.manage is missing/i)).toBeVisible();
    expect(onSessionEnded).not.toHaveBeenCalled();
  });
});
