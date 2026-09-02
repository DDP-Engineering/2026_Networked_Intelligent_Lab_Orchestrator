import { CSRF_COOKIE_NAME } from './contract';

/**
 * Read the session's cross-site request forgery token.
 *
 * Bench Core issues two cookies at sign-in: the session secret, which is
 * `HttpOnly` so that no page script and no injected script can read it, and
 * this one, which is deliberately readable because echoing it in a header is
 * exactly what a cross-site request cannot do. After a page reload the token
 * returned in the sign-in body is gone, and this cookie is the only place the
 * interface can get it from.
 *
 * The reader takes the cookie string so it can be exercised directly; callers
 * in the browser pass nothing and get `document.cookie`.
 */
export function readCsrfToken(cookies: string = document.cookie): string {
  for (const entry of cookies.split(';')) {
    const [name, ...value] = entry.split('=');
    if (name.trim() === CSRF_COOKIE_NAME) {
      return decodeURIComponent(value.join('=').trim());
    }
  }
  return '';
}
