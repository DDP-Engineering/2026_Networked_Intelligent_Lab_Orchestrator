import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { expect, test, type Locator, type Page } from '@playwright/test';

import { DevelopmentTopology, withRedactedSecret } from './topology';

/**
 * The Authentication page, end to end, against a real appliance-stage core.
 *
 * A real browser signs a real administrator in with a password, administers
 * identities and API tokens, and reloads the page to prove the session survives
 * it. Nothing here is mocked: every refusal and every success is Bench Core's
 * own authorization decision over its own SQLite database, reached through
 * the built interface served by Bench Core's HTTPS origin.
 *
 * The tests run in order and share one browser page, because what each one
 * proves is the state the previous one left behind.
 */

const guiRoot = process.cwd();
const repositoryRoot = resolve(guiRoot, '..', '..');
const guiVersion = JSON.parse(
  readFileSync(resolve(guiRoot, 'package.json'), 'utf8'),
) as { version: string };

const ADMINISTRATOR = 'bench-admin';
// A fixture credential for a temporary database this run creates and deletes.
// It is not a secret of anything: no NILO deployment ever sees it.
const ADMINISTRATOR_PASSWORD = 'bench-admin-password-1';
const OPERATOR = 'bench-operator';
const OPERATOR_PASSWORD = 'bench-operator-password-1';
const TOKEN_NAME = 'browser-suite-token';
// An identity created only to be permanently deleted, so no other test's
// subject has to be destroyed to prove that deletion works.
const DISPOSABLE = 'bench-temporary';
const DISPOSABLE_PASSWORD = 'bench-temporary-password-1';

let topology: DevelopmentTopology | null = null;
let page: Page;

test.describe.configure({ mode: 'serial' });

test.beforeAll(async ({ browser }) => {
  // Compiling Bench Core inside the distribution dominates this hook on a cold
  // Go build cache, so it gets its own budget rather than the test timeout.
  test.setTimeout(900_000);
  try {
    topology = await DevelopmentTopology.start({ repositoryRoot, guiRoot });
    await topology.bootstrapAdministrator(ADMINISTRATOR, ADMINISTRATOR_PASSWORD);
  } catch (failure) {
    // The harness already released whatever it had built; re-raise so the run
    // fails loudly with the prerequisite or start-up reason.
    await topology?.dispose().catch(() => undefined);
    topology = null;
    throw failure;
  }
  page = await browser.newPage();
});

test.afterAll(async () => {
  try {
    await page?.close();
  } finally {
    await topology?.dispose();
    topology = null;
  }
});

test('signs a real administrator in with a password', async () => {
  const running = requireTopology();
  for (const viewport of [
    { width: 800, height: 480 },
    { width: 1024, height: 600 },
    { width: 1366, height: 768 },
    { width: 1920, height: 1080 },
  ]) {
    await page.setViewportSize(viewport);
    await page.goto(running.frontendUrl);
    await expectSignedOutLayout(viewport);
  }

  await signIn(ADMINISTRATOR, ADMINISTRATOR_PASSWORD);

  await expect(page.getByRole('heading', { name: 'About', exact: true })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Account' })).toContainText(
    ADMINISTRATOR,
  );
  await expectSimulationIdentity();
});

test('administers identities from the Authentication page', async () => {
  await openAuthentication();

  const users = page.getByRole('region', { name: 'Users' });
  await expect(users).toContainText(ADMINISTRATOR);

  await page.getByRole('button', { name: 'New user' }).click();
  const dialog = page.getByRole('dialog');
  await dialog.getByRole('textbox', { name: 'Username', exact: true }).fill(OPERATOR);
  await dialog
    .getByRole('combobox', { name: 'Role', exact: true })
    .selectOption('operator');
  await fillSecret(dialog.getByLabel('Password', { exact: true }), OPERATOR_PASSWORD);
  await fillSecret(
    dialog.getByLabel('Confirm password', { exact: true }),
    OPERATOR_PASSWORD,
  );
  await dialog.getByRole('button', { name: 'Create identity' }).click();

  await expect(page.getByText(`Created ${OPERATOR} as operator.`)).toBeVisible();
  await expect(users.getByRole('row', { name: new RegExp(OPERATOR) })).toContainText(
    'operator',
  );

  // The change is Bench Core's, not this page's: the role it stores is what the
  // next read reports.
  await users.getByRole('button', { name: `Actions for ${OPERATOR}` }).click();
  await page.getByRole('dialog').getByRole('button', { name: 'Change role' }).click();
  await page
    .getByRole('dialog')
    .getByRole('combobox', { name: 'Role', exact: true })
    .selectOption('viewer');
  await page.getByRole('dialog').getByRole('button', { name: 'Change role' }).click();

  await expect(page.getByText(`${OPERATOR} is now a viewer.`)).toBeVisible();
  await expect(users.getByRole('row', { name: new RegExp(OPERATOR) })).toContainText(
    'viewer',
  );
});

test('keeps keyboard focus inside an open dialog', async () => {
  await page.getByRole('button', { name: 'New user' }).click();
  const dialog = page.getByRole('dialog');
  await expect(dialog).toBeVisible();

  // Tab far enough to have left any real dialog, then again backwards. Only a
  // browser settles this: jsdom cannot reproduce the wrap from the end of the
  // document back to the top, which is how focus reached the page behind.
  for (let press = 0; press < 12; press += 1) {
    await page.keyboard.press('Tab');
    await expect(dialog.locator(':focus')).toHaveCount(1);
  }
  for (let press = 0; press < 12; press += 1) {
    await page.keyboard.press('Shift+Tab');
    await expect(dialog.locator(':focus')).toHaveCount(1);
  }

  await page.keyboard.press('Escape');
  await expect(dialog).toHaveCount(0);
});

test('creates an API token and shows its secret exactly once', async () => {
  await openAuthenticationView('API tokens');
  const tokens = page.getByRole('region', { name: 'API Tokens' });
  // Sign-in uses a server-managed human session, so the fresh appliance starts
  // with no API token. This journey creates the first one explicitly.
  await expect(tokens).toBeVisible();

  await page.getByRole('button', { name: 'New token' }).click();
  const dialog = page.getByRole('dialog');
  await dialog
    .getByRole('combobox', { name: 'Owner', exact: true })
    .selectOption(OPERATOR);
  await dialog.getByRole('textbox', { name: 'Name', exact: true }).fill(TOKEN_NAME);
  await dialog.getByRole('checkbox', { name: 'status.read' }).check();
  await expect(dialog).toContainText('can never exceed that role');
  await dialog.getByRole('button', { name: 'Create token' }).click();

  const disclosure = page.getByTestId('token-secret');
  await expect(disclosure).toContainText('nilo_');
  await expect(page.getByRole('dialog')).toContainText('cannot be retrieved later');
  // The copy affordance is exercised here; that it puts exactly the secret on
  // the clipboard is settled in the component test, which can observe it.
  await page.getByRole('button', { name: 'Copy secret' }).click();

  const secret = (await disclosure.innerText()).trim();
  await page.getByRole('dialog').getByRole('button', { name: 'Done' }).click();
  await expect(page.getByRole('dialog')).toHaveCount(0);
  await expect(page.locator('body')).not.toContainText(secret);

  await expect(tokens.getByRole('row', { name: new RegExp(TOKEN_NAME) })).toContainText(
    'active',
  );
  await expect(tokens.getByRole('row', { name: new RegExp(TOKEN_NAME) })).toContainText(
    OPERATOR,
  );
});

test('restores the session across a reload and never shows the secret again', async () => {
  await page.reload();

  // No sign-in screen and no password: the session cookie the server set is
  // still there, and the interface asked Bench Core who it belongs to.
  await expect(page.getByRole('button', { name: 'Account' })).toContainText(
    ADMINISTRATOR,
  );
  await expect(page.getByRole('heading', { name: 'Sign in to NILO' })).toHaveCount(0);

  await openAuthenticationView('API tokens');
  const tokens = page.getByRole('region', { name: 'API Tokens' });
  await expect(tokens).toContainText(TOKEN_NAME);
  await expect(page.getByTestId('token-secret')).toHaveCount(0);
  await expect(page.locator('body')).not.toContainText('nilo_');
});

test('revokes a token only after an explicit confirmation', async () => {
  const tokens = page.getByRole('region', { name: 'API Tokens' });
  const row = tokens.getByRole('row', { name: new RegExp(TOKEN_NAME) });

  await row.getByRole('button', { name: `Actions for ${TOKEN_NAME}` }).click();
  await page.getByRole('dialog').getByRole('button', { name: 'Revoke' }).click();
  await expect(page.getByRole('dialog')).toContainText(TOKEN_NAME);
  await page.getByRole('dialog').getByRole('button', { name: 'Cancel' }).click();
  await expect(page.getByRole('dialog')).toHaveCount(0);
  await expect(row).toContainText('active');

  await row.getByRole('button', { name: `Actions for ${TOKEN_NAME}` }).click();
  await page.getByRole('dialog').getByRole('button', { name: 'Revoke' }).click();
  await page.getByRole('dialog').getByRole('button', { name: 'Revoke token' }).click();

  await expect(page.getByText(`Revoked the token ${TOKEN_NAME}.`)).toBeVisible();
  await expect(row).toContainText('revoked');
});

test('refuses to delete an identity that still owns an API token', async () => {
  await openAuthenticationView('Users');
  const users = page.getByRole('region', { name: 'Users' });

  await users.getByRole('button', { name: `Actions for ${OPERATOR}` }).click();
  await page
    .getByRole('dialog')
    .getByRole('button', { name: 'Delete permanently' })
    .click();
  const dialog = page.getByRole('dialog');
  await expect(dialog).toContainText('cannot be undone');
  await expect(dialog).toContainText(OPERATOR);

  // Nothing is sent until the exact username is typed back.
  const confirm = dialog.getByRole('button', { name: 'Delete permanently' });
  await expect(confirm).toBeDisabled();
  await dialog.getByLabel('Confirm the username', { exact: true }).fill('bench-oper');
  await expect(confirm).toBeDisabled();
  await dialog.getByLabel('Confirm the username', { exact: true }).fill(OPERATOR);
  await expect(confirm).toBeEnabled();
  await expectFieldSized(confirm);
  await expectFieldSized(dialog.getByLabel('Confirm the username', { exact: true }));

  await confirm.click();

  // Bench Core refuses it: the token created above still names this owner.
  await expect(page.getByRole('dialog')).toContainText(
    'An identity that owns API tokens cannot be deleted',
  );
  await page.getByRole('dialog').getByRole('button', { name: 'Cancel' }).click();
  await expect(page.getByRole('dialog')).toHaveCount(0);
  await expect(users.getByRole('row', { name: new RegExp(OPERATOR) })).toBeVisible();
});

test('deletes a revoked API token permanently', async () => {
  await openAuthenticationView('API tokens');
  const tokens = page.getByRole('region', { name: 'API Tokens' });
  const row = tokens.getByRole('row', { name: new RegExp(TOKEN_NAME) });
  await expect(row).toContainText('revoked');

  // Revoke and Delete are separate entries: the first is spent, the second is
  // still offered, because what it removes is the record.
  await row.getByRole('button', { name: `Actions for ${TOKEN_NAME}` }).click();
  await expect(
    page.getByRole('dialog').getByRole('button', { name: 'Revoke', exact: true }),
  ).toBeDisabled();
  await page
    .getByRole('dialog')
    .getByRole('button', { name: 'Delete permanently' })
    .click();

  const dialog = page.getByRole('dialog');
  await expect(dialog).toContainText('cannot be undone');
  const confirm = dialog.getByRole('button', { name: 'Delete permanently' });
  await expect(confirm).toBeDisabled();
  await dialog.getByLabel('Confirm the token name', { exact: true }).fill(TOKEN_NAME);
  await confirm.click();

  await expect(
    page.getByText(`Deleted the token ${TOKEN_NAME} permanently.`),
  ).toBeVisible();
  await expect(tokens).not.toContainText(TOKEN_NAME);
  await expect(page.getByRole('region', { name: 'Users' })).toHaveCount(0);
});

test('deletes an identity permanently once nothing depends on it', async () => {
  await openAuthenticationView('Users');
  const users = page.getByRole('region', { name: 'Users' });

  await page.getByRole('button', { name: 'New user' }).click();
  const editor = page.getByRole('dialog');
  await editor.getByRole('textbox', { name: 'Username', exact: true }).fill(DISPOSABLE);
  await fillSecret(editor.getByLabel('Password', { exact: true }), DISPOSABLE_PASSWORD);
  await fillSecret(
    editor.getByLabel('Confirm password', { exact: true }),
    DISPOSABLE_PASSWORD,
  );
  await editor.getByRole('button', { name: 'Create identity' }).click();
  await expect(users.getByRole('row', { name: new RegExp(DISPOSABLE) })).toBeVisible();

  // Dismissing the confirmation sends nothing.
  await users.getByRole('button', { name: `Actions for ${DISPOSABLE}` }).click();
  await page
    .getByRole('dialog')
    .getByRole('button', { name: 'Delete permanently' })
    .click();
  await page.getByRole('dialog').getByRole('button', { name: 'Cancel' }).click();
  await expect(page.getByRole('dialog')).toHaveCount(0);
  await expect(users.getByRole('row', { name: new RegExp(DISPOSABLE) })).toBeVisible();

  await users.getByRole('button', { name: `Actions for ${DISPOSABLE}` }).click();
  await page
    .getByRole('dialog')
    .getByRole('button', { name: 'Delete permanently' })
    .click();
  const dialog = page.getByRole('dialog');
  await dialog.getByLabel('Confirm the username', { exact: true }).fill(DISPOSABLE);
  await dialog.getByRole('button', { name: 'Delete permanently' }).click();

  await expect(
    page.getByText(`Deleted the identity ${DISPOSABLE} permanently.`),
  ).toBeVisible();
  await expect(users).not.toContainText(DISPOSABLE);

  // Gone from Bench Core, not merely from this page: the credential no longer
  // authenticates and a reload does not bring the row back.
  await page.reload();
  await openAuthenticationView('Users');
  await expect(page.getByRole('region', { name: 'Users' })).not.toContainText(
    DISPOSABLE,
  );
});

test('opens the common Activity page filtered to identity mutations', async () => {
  await page.getByRole('button', { name: 'View activity', exact: true }).click();

  await expect(page.getByRole('heading', { name: 'Activity', level: 2 })).toBeVisible();
  await expect(
    page.getByRole('combobox', { name: 'Component', exact: true }),
  ).toHaveValue('identity');
  await expect(page.getByText('identity.token.revoked', { exact: true })).toBeVisible();
  // The two subjects these name no longer exist; the history that names them
  // does, and is still reachable through the same component filter.
  await expect(page.getByText('identity.token.deleted', { exact: true })).toBeVisible();
  await expect(page.getByText('identity.user.deleted', { exact: true })).toBeVisible();
});

test('refuses to remove the last enabled administrator', async () => {
  await openAuthenticationView('Users');
  const users = page.getByRole('region', { name: 'Users' });
  await users.getByRole('button', { name: `Actions for ${ADMINISTRATOR}` }).click();
  await expect(
    page.getByRole('dialog').getByRole('button', { name: 'Disable' }),
  ).toBeDisabled();
  await expect(page.getByRole('dialog')).toContainText(
    'The last enabled administrator must remain enabled.',
  );
  // The same invariant, stated for the removal that cannot be undone.
  await expect(
    page.getByRole('dialog').getByRole('button', { name: 'Delete permanently' }),
  ).toBeDisabled();
  await expect(page.getByRole('dialog')).toContainText(
    'The last enabled administrator cannot be deleted.',
  );
  await page.getByRole('dialog').getByRole('button', { name: 'Cancel' }).click();
  await expect(
    users.getByRole('row', { name: new RegExp(ADMINISTRATOR) }),
  ).toContainText('enabled');
});

test('hides the Authentication entry from an identity that may not administer', async () => {
  await signOut();
  await expect(page.getByRole('heading', { name: 'Sign in to NILO' })).toBeVisible();

  // The operator was demoted to Viewer above; its password is unchanged.
  await signIn(OPERATOR, OPERATOR_PASSWORD);

  await expect(page.getByRole('button', { name: 'Account' })).toContainText(OPERATOR);
  const navigation = page.getByRole('navigation');
  await expect(navigation.getByRole('button', { name: 'Instruments' })).toHaveAttribute(
    'aria-expanded',
    'false',
  );
  await expect(navigation.getByRole('button', { name: 'System' })).toHaveAttribute(
    'aria-expanded',
    'false',
  );
  await navigation.getByRole('button', { name: 'System' }).click();
  await expect(navigation.getByRole('button', { name: 'Authentication' })).toHaveCount(
    0,
  );
  await expect(navigation.getByRole('button', { name: 'Inventory' })).toBeVisible();
});

test('ends the session server-side when the human signs out', async () => {
  await signOut();
  await expect(page.getByRole('heading', { name: 'Sign in to NILO' })).toBeVisible();
  await expect(page.getByText('You are signed out.')).toBeVisible();

  // A reload proves it was the server that forgot, not the page: a session that
  // only disappeared from this tab would come straight back.
  await page.reload();
  await expect(page.getByRole('heading', { name: 'Sign in to NILO' })).toBeVisible();
  await expectSimulationIdentity();
});

/** Sign in without letting a failing action repeat the password. */
async function signIn(username: string, password: string): Promise<void> {
  await page.getByLabel('Username').fill(username);
  await fillSecret(page.getByLabel('Password', { exact: true }), password);
  await page.getByRole('button', { name: 'Sign in', exact: true }).click();
}

/**
 * Type a secret into a field, and let no failure repeat it.
 *
 * A browser action failing after its locator has resolved puts its argument in
 * the error it throws, and that message reaches the console reporter whether or
 * not trace, video and screenshot are off.
 */
async function fillSecret(field: Locator, secret: string): Promise<void> {
  await withRedactedSecret(secret, () => field.fill(secret));
}

async function openAuthentication(): Promise<void> {
  await expandSystem();
  await page
    .getByRole('navigation')
    .getByRole('button', { name: 'Authentication', exact: true })
    .click();
  await expect(page.getByRole('region', { name: 'Users' })).toBeVisible();
}

async function openAuthenticationView(name: 'Users' | 'API tokens'): Promise<void> {
  await expandSystem();
  const group = page
    .getByRole('navigation')
    .getByRole('button', { name: 'Expand Authentication' });
  if (await group.isVisible()) await group.click();
  await page.getByRole('navigation').getByRole('button', { name, exact: true }).click();
  const regionName = name === 'Users' ? 'Users' : 'API Tokens';
  await expect(page.getByRole('region', { name: regionName })).toBeVisible();
}

async function expandSystem(): Promise<void> {
  const section = page
    .getByRole('navigation')
    .getByRole('button', { name: 'System', exact: true });
  if ((await section.getAttribute('aria-expanded')) === 'false') await section.click();
}

/**
 * One field control is at least 44 CSS pixels tall.
 *
 * The dialogs a deletion opens are new surfaces, and a control a gloved finger
 * cannot hit is only visible in a browser that has laid the dialog out.
 */
async function expectFieldSized(locator: Locator): Promise<void> {
  const box = await locator.boundingBox();
  expect(box).not.toBeNull();
  expect(box?.height ?? 0).toBeGreaterThanOrEqual(44);
}

/** Sign out, which now lives inside the account menu. */
async function signOut(): Promise<void> {
  await page.getByRole('button', { name: 'Account' }).click();
  await page.getByRole('menuitem', { name: 'Sign out' }).click();
}

/** One persistent runtime indicator; sign-in also explains the development boundary. */
async function expectSimulationIdentity(): Promise<void> {
  if (await page.locator('.shell').count()) {
    await expect(page.getByRole('status', { name: /appliance/i })).toContainText(
      'LIVE',
    );
    await expect(page.locator('.shell-boundary')).toHaveCount(0);
  } else {
    await expect(page.getByRole('status', { name: /appliance/i })).toHaveCount(0);
  }
}

async function expectSignedOutLayout(viewport: {
  width: number;
  height: number;
}): Promise<void> {
  await expect(page.getByRole('heading', { name: 'Sign in to NILO' })).toBeVisible();
  await expect(page.getByText('NILO', { exact: true })).toBeVisible();
  await expect(
    page.getByText(`Version ${guiVersion.version}`, { exact: true }),
  ).toBeVisible();
  await expect(page.getByLabel('Username')).toBeVisible();
  await expect(page.getByLabel('Password', { exact: true })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Show password' })).toBeVisible();
  await expect(
    page.getByRole('button', { name: 'Sign in', exact: true }),
  ).toBeVisible();
  await expect(page.locator('form')).toHaveCount(1);
  await expect(page.getByRole('status', { name: /appliance/i })).toHaveCount(0);

  const geometry = await page.locator('.sign-in-card').evaluate((card) => {
    const rectangle = card.getBoundingClientRect();
    const controls = Array.from(card.querySelectorAll('button, input')).map(
      (element) => {
        const bounds = element.getBoundingClientRect();
        return { width: bounds.width, height: bounds.height };
      },
    );
    const textSizes = Array.from(card.querySelectorAll('*'))
      .filter((element) => (element.textContent ?? '').trim() !== '')
      .map((element) => Number.parseFloat(getComputedStyle(element).fontSize));
    return {
      width: rectangle.width,
      right: rectangle.right,
      bottom: rectangle.bottom,
      controls,
      smallestText: Math.min(...textSizes),
      documentWidth: document.documentElement.scrollWidth,
    };
  });

  expect(geometry.documentWidth).toBeLessThanOrEqual(viewport.width);
  expect(geometry.right).toBeLessThanOrEqual(viewport.width);
  expect(geometry.bottom).toBeLessThanOrEqual(viewport.height);
  expect(geometry.smallestText).toBeGreaterThanOrEqual(14);
  for (const control of geometry.controls) {
    expect(control.width).toBeGreaterThanOrEqual(44);
    expect(control.height).toBeGreaterThanOrEqual(44);
  }
  if (viewport.width >= 1366) {
    expect(geometry.width).toBeLessThan(viewport.width / 2);
  }
}

function requireTopology(): DevelopmentTopology {
  if (topology === null) {
    throw new Error('the development topology is not running');
  }
  return topology;
}
