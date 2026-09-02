import { resolve } from 'node:path';
import { expect, test, type Locator, type Page } from '@playwright/test';

import { DevelopmentTopology, withRedactedSecret } from './topology';

/**
 * The Authentication page, end to end, against a real Bench Core under WSL2.
 *
 * A real browser signs a real administrator in with a password, administers
 * identities and API tokens, and reloads the page to prove the session survives
 * it. Nothing here is mocked: every refusal and every success is Bench Core's
 * own authorization decision over its own SQLite database, reached through the
 * interface development server's `/api` forwarding.
 *
 * The tests run in order and share one browser page, because what each one
 * proves is the state the previous one left behind.
 */

const guiRoot = process.cwd();
const repositoryRoot = resolve(guiRoot, '..', '..');

const ADMINISTRATOR = 'bench-admin';
// A fixture credential for a temporary database this run creates and deletes.
// It is not a secret of anything: no NILO deployment ever sees it.
const ADMINISTRATOR_PASSWORD = 'bench-admin-password-1';
const OPERATOR = 'bench-operator';
const OPERATOR_PASSWORD = 'bench-operator-password-1';
const TOKEN_NAME = 'browser-suite-token';

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
  await page.goto(running.frontendUrl);

  await expect(page.getByRole('heading', { name: 'Sign in to NILO' })).toBeVisible();
  await expectSimulationIdentity();

  await signIn(ADMINISTRATOR, ADMINISTRATOR_PASSWORD);

  await expect(page.getByRole('heading', { name: 'Bench', exact: true })).toBeVisible();
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
  await dialog.getByLabel('Username').fill(OPERATOR);
  await dialog.getByLabel('Role').selectOption('operator');
  await fillSecret(dialog.getByLabel('Password', { exact: true }), OPERATOR_PASSWORD);
  await fillSecret(dialog.getByLabel('Confirm password'), OPERATOR_PASSWORD);
  await dialog.getByRole('button', { name: 'Create identity' }).click();

  await expect(page.getByText(`Created ${OPERATOR} as operator.`)).toBeVisible();
  await expect(users.getByRole('row', { name: new RegExp(OPERATOR) })).toContainText(
    'operator',
  );

  // The change is Bench Core's, not this page's: the role it stores is what the
  // next read reports.
  await users
    .getByRole('row', { name: new RegExp(OPERATOR) })
    .getByRole('button', { name: 'Change role' })
    .click();
  await page.getByRole('dialog').getByLabel('Role').selectOption('viewer');
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
  const tokens = page.getByRole('region', { name: 'API Tokens' });
  // The harness issued itself a development token to reach the bench with, so
  // this listing already has an entry: an unnarrowed token owned by the
  // development identity that token belongs to.
  await expect(tokens).toContainText("owner's whole role");

  await page.getByRole('button', { name: 'New token' }).click();
  const dialog = page.getByRole('dialog');
  await dialog.getByLabel('Owner').selectOption(OPERATOR);
  await dialog.getByLabel('Name').fill(TOKEN_NAME);
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

  await openAuthentication();
  const tokens = page.getByRole('region', { name: 'API Tokens' });
  await expect(tokens).toContainText(TOKEN_NAME);
  await expect(page.getByTestId('token-secret')).toHaveCount(0);
  await expect(page.locator('body')).not.toContainText('nilo_');
});

test('revokes a token only after an explicit confirmation', async () => {
  const tokens = page.getByRole('region', { name: 'API Tokens' });
  const row = tokens.getByRole('row', { name: new RegExp(TOKEN_NAME) });

  await row.getByRole('button', { name: 'Revoke' }).click();
  await expect(page.getByRole('dialog')).toContainText(TOKEN_NAME);
  await page.getByRole('dialog').getByRole('button', { name: 'Cancel' }).click();
  await expect(page.getByRole('dialog')).toHaveCount(0);
  await expect(row).toContainText('active');

  await row.getByRole('button', { name: 'Revoke' }).click();
  await page.getByRole('dialog').getByRole('button', { name: 'Revoke token' }).click();

  await expect(page.getByText(`Revoked the token ${TOKEN_NAME}.`)).toBeVisible();
  await expect(row).toContainText('revoked');
});

test('refuses to remove the last enabled administrator', async () => {
  const users = page.getByRole('region', { name: 'Users' });
  await users
    .getByRole('row', { name: new RegExp(ADMINISTRATOR) })
    .getByRole('button', { name: 'Disable' })
    .click();
  await page.getByRole('dialog').getByRole('button', { name: 'Disable' }).click();

  await expect(page.getByRole('dialog')).toContainText(
    'last enabled administrator must remain an enabled administrator',
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
  await expect(navigation.getByRole('button', { name: 'Bench' })).toBeVisible();
  await expect(navigation.getByRole('button', { name: 'Authentication' })).toHaveCount(
    0,
  );
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
  await page
    .getByRole('navigation')
    .getByRole('button', { name: 'Authentication' })
    .click();
  await expect(page.getByRole('region', { name: 'Users' })).toBeVisible();
}

/** Sign out, which now lives inside the account menu. */
async function signOut(): Promise<void> {
  await page.getByRole('button', { name: 'Account' }).click();
  await page.getByRole('menuitem', { name: 'Sign out' }).click();
}

/** The identity and the boundary statement every screen must carry. */
async function expectSimulationIdentity(): Promise<void> {
  await expect(page.getByRole('status', { name: /runtime profile/i })).toContainText(
    'SIM',
  );
  await expect(
    page.getByText(
      /simulated results never represent evidence from physical equipment/i,
    ),
  ).toBeVisible();
}

function requireTopology(): DevelopmentTopology {
  if (topology === null) {
    throw new Error('the development topology is not running');
  }
  return topology;
}
