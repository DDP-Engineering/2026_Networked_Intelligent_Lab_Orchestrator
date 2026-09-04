import {
  PERMISSION_ACTIVITY_READ,
  PERMISSION_IDENTITIES_READ,
  PERMISSION_TOKENS_READ,
  type BenchCoreSession,
} from './contract';

/** Report whether this identity may open the shared Activity page. */
export function mayReadActivity(session: BenchCoreSession | null): boolean {
  return session !== null && session.permissions.includes(PERMISSION_ACTIVITY_READ);
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
  if (session === null) {
    return false;
  }
  return (
    session.permissions.includes(PERMISSION_IDENTITIES_READ) ||
    session.permissions.includes(PERMISSION_TOKENS_READ)
  );
}
