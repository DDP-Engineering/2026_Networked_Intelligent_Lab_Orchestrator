import {
  PERMISSION_ACTIVITY_READ,
  PERMISSION_IDENTITIES_MANAGE,
  PERMISSION_IDENTITIES_READ,
  PERMISSION_TOKENS_MANAGE,
  PERMISSION_TOKENS_READ,
  type BenchCoreSession,
} from './contract';

/** Report whether this identity may open the shared Activity page. */
export function mayReadActivity(session: BenchCoreSession | null): boolean {
  return session !== null && session.permissions.includes(PERMISSION_ACTIVITY_READ);
}

/** Report whether the current session may read the Users destination. */
export function mayReadUsers(session: BenchCoreSession | null): boolean {
  return session !== null && session.permissions.includes(PERMISSION_IDENTITIES_READ);
}

/** Report whether the current session may change user identities. */
export function mayManageUsers(session: BenchCoreSession | null): boolean {
  return session !== null && session.permissions.includes(PERMISSION_IDENTITIES_MANAGE);
}

/** Report whether the current session may read the API tokens destination. */
export function mayReadTokens(session: BenchCoreSession | null): boolean {
  return session !== null && session.permissions.includes(PERMISSION_TOKENS_READ);
}

/** Report whether the current session may create and revoke API tokens. */
export function mayManageTokens(session: BenchCoreSession | null): boolean {
  return session !== null && session.permissions.includes(PERMISSION_TOKENS_MANAGE);
}

/**
 * Report whether this identity has any reason to open the Authentication page.
 *
 * This decides what the navigation offers and nothing else. Bench Core
 * authorizes every administration request on its own, so an identity that
 * reaches the page another way is refused there rather than here, and a
 * navigation entry that is merely hidden has never been a permission.
 */
export function mayAdminister(session: BenchCoreSession | null): boolean {
  return mayReadUsers(session) || mayReadTokens(session);
}
