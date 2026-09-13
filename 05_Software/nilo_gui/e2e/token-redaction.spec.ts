import { expect, test } from '@playwright/test';

import { REDACTED_SECRET, withRedactedSecret } from './topology';

/**
 * A forced-failure regression against a real browser.
 *
 * Playwright puts a call log on the error a failing action throws, and that log
 * repeats the argument once the locator has resolved. Turning off trace, video
 * and screenshot does not prevent it: the message reaches the console reporter
 * and the error-context note. The password field always resolves, so this is the
 * shape that would leak a live credential.
 *
 * These tests deliberately provoke that failure with a dummy marker and assert
 * the harness's wrapper removes it. They contact no backend and use no real
 * credential; they exist so that a future Playwright release changing its error shape
 * is caught here rather than in a run holding a real credential.
 */

const MARKER = 'DUMMY-MARKER-9f3a2b7c';

test('a failing fill on a resolved field never surfaces its argument', async ({
  page,
}) => {
  await page.setContent(
    '<main><label for="t">Appliance fixture password</label><input id="t" disabled></main>',
  );
  const field = page.getByLabel('Appliance fixture password');

  const failure = await withRedactedSecret(MARKER, () =>
    field.fill(MARKER, { timeout: 1_500 }),
  ).then(
    () => null,
    (error: unknown) => error as Error,
  );

  // The action really did fail: this is a forced failure, not a vacuous pass.
  expect(failure).toBeInstanceOf(Error);
  expect(failure?.message ?? '').toContain('locator.fill');
  expect(failure?.message ?? '').not.toContain(MARKER);
  expect(String(failure?.stack ?? '')).not.toContain(MARKER);
  expect(failure?.message ?? '').toContain(REDACTED_SECRET);
});

test('an unwrapped fill would have leaked it, which is why the wrapper exists', async ({
  page,
}) => {
  await page.setContent(
    '<main><label for="t">Appliance fixture password</label><input id="t" disabled></main>',
  );

  const leaked = await page
    .getByLabel('Appliance fixture password')
    .fill(MARKER, { timeout: 1_500 })
    .then(
      () => '',
      (error: unknown) => String((error as Error)?.message ?? ''),
    );

  // Guards the premise. If Playwright ever stops echoing the argument this
  // fails, and the wrapper can be reconsidered on evidence rather than removed
  // on a guess.
  expect(leaked).toContain(MARKER);
});
