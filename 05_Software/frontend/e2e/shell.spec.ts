import { resolve } from 'node:path';
import { expect, test, type Locator, type Page } from '@playwright/test';

import { DevelopmentTopology, withRedactedSecret } from './topology';

/**
 * The redesigned shell, in a real browser against an appliance-stage core.
 *
 * What only a browser can settle is here: that the header really is one row,
 * that a theme and a language survive an actual page reload, and that the
 * session survives it with them. Everything else about the shell is settled far
 * more cheaply in the component tests.
 */

const guiRoot = process.cwd();
const repositoryRoot = resolve(guiRoot, '..', '..');

const ADMINISTRATOR = 'shell-admin';
// A fixture credential for a temporary database this run creates and deletes.
const ADMINISTRATOR_PASSWORD = 'shell-admin-password-1';

let topology: DevelopmentTopology | null = null;
let page: Page;

test.describe.configure({ mode: 'serial' });

test.beforeAll(async ({ browser }) => {
  test.setTimeout(900_000);
  try {
    topology = await DevelopmentTopology.start({ repositoryRoot, guiRoot });
    await topology.bootstrapAdministrator(ADMINISTRATOR, ADMINISTRATOR_PASSWORD);
  } catch (failure) {
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

test('puts the wordmark, tabs and every control on one row', async () => {
  const running = requireTopology();
  await page.goto(running.frontendUrl);

  await expect(page.getByRole('heading', { name: 'Sign in to NILO' })).toBeVisible();
  await signIn(ADMINISTRATOR, ADMINISTRATOR_PASSWORD);
  await expect(page.getByRole('heading', { name: 'About', exact: true })).toBeVisible();
  await expect(
    page.getByRole('tablist', { name: 'Open pages' }).getByRole('tab'),
  ).toHaveCount(1);
  for (const name of ['Instruments', 'System']) {
    await expect(
      page.getByRole('navigation').getByRole('button', { name, exact: true }),
    ).toHaveAttribute('aria-expanded', 'false');
  }
  await expect(
    page.locator('.sidenav').getByRole('button', { name: 'Bench', exact: true }),
  ).toHaveCount(0);

  const row = page.locator('.toprow');
  await expect(row.getByRole('tablist', { name: 'Open pages' })).toBeVisible();
  await expect(row.getByRole('button', { name: 'Network', exact: true })).toBeVisible();
  await expect(row.getByRole('button', { name: 'Appearance' })).toBeVisible();
  await expect(row.getByRole('button', { name: 'Language' })).toBeVisible();
  await expect(row.getByRole('button', { name: 'Account' })).toBeVisible();

  // One row, and within the agreed height.
  const height = await row.evaluate(
    (element) => element.getBoundingClientRect().height,
  );
  expect(height).toBeGreaterThanOrEqual(44);
  expect(height).toBeLessThanOrEqual(48);

  // One tab strip, and the document does not scroll. The measurement is taken
  // from the root element itself, so this file needs no DOM globals.
  await expect(page.getByRole('tablist')).toHaveCount(1);
  expect(
    await page
      .locator('html')
      .evaluate((root) => root.scrollHeight <= root.clientHeight + 1),
  ).toBe(true);
});

test('anchors Network in the lower cluster, not among the instruments', async () => {
  await page
    .getByRole('navigation')
    .getByRole('button', { name: 'System', exact: true })
    .click();
  const lower = page.locator('.nav-lower');

  await expect(
    lower.locator(
      '> .nav-list > li > .nav-entry, > .nav-list > li > .nav-group-row > .nav-entry',
    ),
  ).toHaveText([
    'Plugins',
    'Network',
    'Authentication',
    'Activity',
    'About',
    'Inventory',
  ]);
  await expect(
    page.locator('.nav-upper').getByRole('button', { name: 'Network', exact: true }),
  ).toHaveCount(0);
  await expect(
    page.locator('.nav-lower').getByRole('button', { name: 'Settings', exact: true }),
  ).toHaveCount(0);
});

test('opens and closes pages as tabs', async () => {
  const tabs = page.getByRole('tablist', { name: 'Open pages' });

  await page
    .locator('.nav-lower')
    .getByRole('button', { name: 'Network', exact: true })
    .click();
  await expect(
    page.getByRole('heading', { name: 'Network / Overview', exact: true }),
  ).toBeVisible();
  await expect(tabs.getByRole('tab')).toHaveCount(2);

  await page.locator('.nav-lower').getByRole('button', { name: 'About' }).click();
  await expect(tabs.getByRole('tab')).toHaveCount(2);

  // Closing the active tab falls back to its left neighbour.
  await page.getByRole('button', { name: 'Close About' }).click();
  await expect(
    page.getByRole('heading', { name: 'Network / Overview', exact: true }),
  ).toBeVisible();
  await expect(tabs.getByRole('tab')).toHaveCount(1);
});

test('reports the appliance network state the backend actually served', async () => {
  await expect(
    page.getByRole('heading', { name: 'Network / Overview', exact: true }),
  ).toBeVisible();

  // Overview states where NILO is reachable before the focused pages add detail.
  // The address is the one the stage's own adapter carries, read back through
  // Bench Core rather than asserted from a catalog.
  await expect(page.getByRole('region', { name: 'Overview' })).toContainText(
    'eth0 · 192.0.2.2/24',
  );
  // Opening the Network workspace does not disclose its child navigation.
  const navigation = page.getByRole('navigation');
  await expect(
    navigation.getByRole('button', { name: 'Expand Network' }),
  ).toHaveAttribute('aria-expanded', 'false');
  await navigation.getByRole('button', { name: 'Expand Network' }).click();
  await navigation.getByRole('button', { name: 'Overview', exact: true }).click();

  // Bench Core serves the adapters NetworkManager detected, so the page reports
  // what it was given rather than saying nothing is available.
  await expect(page.getByRole('table', { name: 'Interfaces' })).toBeVisible();
  // The row, specifically: the chart's interface chooser lists the same names,
  // so a plain text match would resolve to two elements.
  await expect(page.getByRole('rowheader', { name: /eth0/ })).toBeVisible();
  // In the interfaces row specifically: the overview above names the same
  // address, so a plain text match would resolve to two elements.
  await expect(
    page
      .getByRole('row')
      .filter({ has: page.getByRole('rowheader', { name: /eth0/ }) })
      .getByText('192.0.2.2/24', { exact: true }),
  ).toBeVisible();

  await expect(page.getByRole('status', { name: /appliance/i })).toHaveCount(1);
  await expect(
    page.getByText('Measured in the simulated environment, not on a bench.'),
  ).toHaveCount(0);
  await expect(page.getByRole('columnheader', { name: 'Gateway' })).toBeVisible();
  await expect(page.getByText('192.0.2.1', { exact: true })).toBeVisible();
  // None of the illustrative values from the design references is present.
  const body = (await page.locator('body').innerText()).replace(/\s+/g, ' ');
  for (const value of [
    '192.168.10.42',
    '10.8.0.12',
    '1 Gbps',
    '2.4 Mbps',
    '0.3 Mbps',
  ]) {
    expect(body).not.toContain(value);
  }
});

test('keeps the theme and the language across a reload, with the session', async () => {
  await page.getByRole('button', { name: 'Appearance' }).click();
  await page.getByRole('menuitemradio', { name: 'Dark' }).click();
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');

  await page.getByRole('button', { name: 'Language' }).click();
  await page.getByRole('menuitemradio', { name: 'Deutsch' }).click();
  await expect(
    page.locator('.nav-lower').getByRole('button', { name: 'Netzwerk', exact: true }),
  ).toBeVisible();

  await page.reload();

  // The reload proves all three: the cookie session, and both preferences.
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');
  await expect(page.locator('html')).toHaveAttribute('lang', 'de');
  await expect(page.getByRole('heading', { name: 'Sign in to NILO' })).toHaveCount(0);
  await expect(
    page.getByRole('navigation').getByRole('button', { name: 'System' }),
  ).toHaveAttribute('aria-expanded', 'false');
  await page.getByRole('navigation').getByRole('button', { name: 'System' }).click();
  await expect(
    page.locator('.nav-lower').getByRole('button', { name: 'Netzwerk', exact: true }),
  ).toBeVisible();

  // Back to English so the remaining assertions read in one language.
  await page.getByRole('button', { name: 'Sprache' }).click();
  await page.getByRole('menuitemradio', { name: 'English' }).click();
  await expect(
    page.locator('.nav-lower').getByRole('button', { name: 'Network', exact: true }),
  ).toBeVisible();
});

test('narrows the navigation to a labeled rail on a small viewport without clipping it', async () => {
  await page.setViewportSize({ width: 900, height: 720 });

  // The rail narrows, but every entry is still there with its visible label,
  // still named for a screen reader, and still clickable.
  await expect(
    page.locator(
      '.nav-lower > .nav-list > li > .nav-entry, .nav-lower > .nav-list > li > .nav-group-row > .nav-entry',
    ),
  ).toHaveCount(6);
  await expect(
    page.locator('.nav-lower').getByRole('button', { name: 'Network', exact: true }),
  ).toBeVisible();
  await expect(
    page.locator('.nav-lower').getByRole('button', { name: 'Network', exact: true }),
  ).toHaveText('Network');
  const boxes = await page
    .locator('.nav-lower button')
    .evaluateAll((elements) =>
      elements.map((element) => element.getBoundingClientRect().width),
    );
  expect(Math.min(...boxes)).toBeGreaterThan(0);
  expect(
    await page
      .locator('html')
      .evaluate((root) => root.scrollWidth <= root.clientWidth + 1),
  ).toBe(true);

  await page.setViewportSize({ width: 1280, height: 720 });
});

test('keeps one runtime indicator on every screen', async () => {
  for (const entry of ['Inventory', 'Authentication', 'About']) {
    await page
      .locator('.sidenav')
      .getByRole('button', { name: entry, exact: true })
      .click();
    await expect(page.getByRole('status', { name: /appliance/i })).toBeVisible();
    await expect(page.locator('.shell-boundary')).toHaveCount(0);
    await expect(page.locator('.runtime-badge')).toHaveCount(1);
  }
});

/** Sign in without letting a failing action repeat the password. */
async function signIn(username: string, password: string): Promise<void> {
  await page.getByLabel('Username').fill(username);
  await fillSecret(page.getByLabel('Password', { exact: true }), password);
  await page.getByRole('button', { name: 'Sign in', exact: true }).click();
}

async function fillSecret(field: Locator, secret: string): Promise<void> {
  await withRedactedSecret(secret, () => field.fill(secret));
}

function requireTopology(): DevelopmentTopology {
  if (topology === null) {
    throw new Error('the development topology is not running');
  }
  return topology;
}
