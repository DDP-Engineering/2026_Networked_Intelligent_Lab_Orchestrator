import { describe, expect, it } from 'vitest';

import {
  BENCH_CORE_API_VERSION,
  BENCH_CORE_PATHS,
  DEVICES_PATH,
  IDENTITIES_PATH,
  INTEGRATIONS_PATH,
  SESSION_PATH,
  SIGN_IN_PATH,
  SIGN_OUT_PATH,
  STATUS_PATH,
  TOKENS_PATH,
  identityPath,
  parseIdentitiesBody,
  parseIssuedTokenBody,
  parseSessionBody,
  parseTokensBody,
  resourceUrl,
  tokenRevocationPath,
} from './contract';

/**
 * A bearer credential is not a CORS-safelisted header, so any request the
 * browser addresses to another origin is preflighted and Bench Core answers no
 * preflight. Every path the contract declares therefore has to stay
 * origin-relative; this is the guard against one absolute URL slipping back in.
 */

describe('Bench Core request paths', () => {
  it('declares exactly the three resources this interface consumes', () => {
    expect(BENCH_CORE_PATHS).toEqual([STATUS_PATH, INTEGRATIONS_PATH, DEVICES_PATH]);
    expect(STATUS_PATH).toBe('/api/v1/status');
    expect(INTEGRATIONS_PATH).toBe('/api/v1/integrations');
    expect(DEVICES_PATH).toBe('/api/v1/devices');
  });

  it.each(BENCH_CORE_PATHS)('addresses %s relative to the serving origin', (path) => {
    expect(path.startsWith('/api/v1/')).toBe(true);
    expect(path).not.toMatch(/^[a-z][a-z0-9+.-]*:/i);
    expect(path.startsWith('//')).toBe(false);
    expect(path).not.toContain('127.0.0.1');
    expect(path).not.toContain('localhost');
  });

  it.each(BENCH_CORE_PATHS)(
    'joins %s onto the empty same-origin base without a host',
    (path) => {
      const url = resourceUrl('', path);

      expect(url).toBe(path);
      expect(url).not.toMatch(/^https?:/);
      expect(url.startsWith('//')).toBe(false);
    },
  );

  it.each(BENCH_CORE_PATHS)(
    'joins %s onto an explicit base without doubling the separator',
    (path) => {
      expect(resourceUrl('http://127.0.0.1:8090/', path)).toBe(
        `http://127.0.0.1:8090${path}`,
      );
    },
  );

  it('supports exactly one wire version', () => {
    expect(BENCH_CORE_API_VERSION).toBe('nilo.http/1');
  });
});

describe('identity and token administration paths', () => {
  it('declares every administration path relative to the serving origin', () => {
    for (const path of [
      SIGN_IN_PATH,
      SIGN_OUT_PATH,
      SESSION_PATH,
      IDENTITIES_PATH,
      TOKENS_PATH,
      identityPath('jane', 'role'),
      tokenRevocationPath('0199-token'),
    ]) {
      expect(path.startsWith('/api/v1/')).toBe(true);
      expect(path).not.toMatch(/^[a-z][a-z0-9+.-]*:/i);
    }
  });

  it('escapes the identity and token names it puts in a path', () => {
    expect(identityPath('jane', 'password')).toBe('/api/v1/users/jane/password');
    expect(identityPath('a/b', 'role')).toBe('/api/v1/users/a%2Fb/role');
    expect(tokenRevocationPath('a b')).toBe('/api/v1/tokens/a%20b/revoke');
  });
});

describe('session parsing', () => {
  const body = {
    api_version: BENCH_CORE_API_VERSION,
    username: 'admin',
    role: 'administrator',
    permissions: ['users.read'],
    credential: 'session',
    csrf_token: 'token-value',
  };

  it('returns the identity, role, permissions and one-time CSRF token', () => {
    const parsed = parseSessionBody(body);

    expect(parsed).toEqual({
      ok: true,
      value: {
        apiVersion: BENCH_CORE_API_VERSION,
        username: 'admin',
        role: 'administrator',
        permissions: ['users.read'],
        credential: 'session',
        csrfToken: 'token-value',
      },
    });
  });

  it('accepts a restored session that carries no CSRF token', () => {
    const withoutToken: Record<string, unknown> = { ...body };
    delete withoutToken.csrf_token;
    const parsed = parseSessionBody(withoutToken);

    expect(parsed.ok).toBe(true);
    expect(parsed.ok && parsed.value.csrfToken).toBeNull();
  });

  it('refuses a role outside the three the access model defines', () => {
    const parsed = parseSessionBody({ ...body, role: 'superuser' });

    expect(parsed.ok).toBe(false);
    expect(!parsed.ok && parsed.violation).toContain('superuser');
  });

  it('refuses a session response outside the supported version', () => {
    const parsed = parseSessionBody({ ...body, api_version: 'nilo.http/2' });

    expect(parsed.ok).toBe(false);
    expect(!parsed.ok && parsed.violation).toContain('nilo.http/2');
  });
});

describe('identity and token listing parsing', () => {
  const identity = {
    id: '0199-identity',
    username: 'jane',
    role: 'operator',
    enabled: true,
    created_at: '2026-09-02T09:00:00Z',
  };

  const token = {
    id: '0199-token',
    name: 'bench-ci',
    owner: 'jane',
    scopes: ['status.read'],
    created_at: '2026-09-02T09:00:00Z',
    expires_at: null,
    revoked: false,
    revoked_at: null,
  };

  it('returns every identity as a typed value', () => {
    const parsed = parseIdentitiesBody({
      api_version: BENCH_CORE_API_VERSION,
      items: [identity],
    });

    expect(parsed.ok && parsed.value.items).toEqual([
      {
        id: '0199-identity',
        username: 'jane',
        role: 'operator',
        enabled: true,
        createdAt: '2026-09-02T09:00:00Z',
      },
    ]);
  });

  it('accepts a listing that legitimately has no entries', () => {
    const parsed = parseIdentitiesBody({
      api_version: BENCH_CORE_API_VERSION,
      items: [],
    });

    expect(parsed.ok && parsed.value.items).toEqual([]);
  });

  it('names a missing identity field rather than accepting the entry', () => {
    const withoutEnabled: Record<string, unknown> = { ...identity };
    delete withoutEnabled.enabled;
    const parsed = parseIdentitiesBody({
      api_version: BENCH_CORE_API_VERSION,
      items: [withoutEnabled],
    });

    expect(parsed.ok).toBe(false);
    expect(!parsed.ok && parsed.violation).toContain('enabled');
  });

  it('returns every token with its expiry and revocation state', () => {
    const parsed = parseTokensBody({
      api_version: BENCH_CORE_API_VERSION,
      items: [
        token,
        {
          ...token,
          id: 'b',
          expires_at: '2026-10-02T09:00:00Z',
          revoked: true,
          revoked_at: '2026-09-03T09:00:00Z',
        },
      ],
    });

    expect(parsed.ok && parsed.value.items[0].expiresAt).toBeNull();
    expect(parsed.ok && parsed.value.items[0].revoked).toBe(false);
    expect(parsed.ok && parsed.value.items[1].expiresAt).toBe('2026-10-02T09:00:00Z');
    expect(parsed.ok && parsed.value.items[1].revoked).toBe(true);
  });

  it('returns a created token together with the single disclosure of its secret', () => {
    const parsed = parseIssuedTokenBody({
      api_version: BENCH_CORE_API_VERSION,
      token,
      secret: 'nilo_created_secret',
    });

    expect(parsed.ok && parsed.value.secret).toBe('nilo_created_secret');
    expect(parsed.ok && parsed.value.token.name).toBe('bench-ci');
  });

  it('refuses a created token answer with no secret in it', () => {
    const parsed = parseIssuedTokenBody({
      api_version: BENCH_CORE_API_VERSION,
      token,
    });

    expect(parsed.ok).toBe(false);
    expect(!parsed.ok && parsed.violation).toContain('secret');
  });
});
