import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { expect, test, type Locator, type Page } from '@playwright/test';

import { DevelopmentTopology, withRedactedSecret } from './topology';

/**
 * The browser-to-appliance path, end to end.
 *
 * A real browser drives the built interface from Bench Core's HTTPS origin.
 * Bench Core runs inside the appliance verification stage on the appliance
 * profile, with its database on that disposable Linux filesystem. Nothing here
 * is mocked, and nothing here proves anything about physical equipment.
 *
 * The tests run in order and share one browser page: the second one starts
 * from the connected screen the first one reached, which is the only way to
 * observe recovery without signing in again.
 */

const guiRoot = process.cwd();
const repositoryRoot = resolve(guiRoot, '..', '..');
const guiVersion = JSON.parse(
  readFileSync(resolve(guiRoot, 'package.json'), 'utf8'),
) as { version: string };

// A fresh appliance carries no managed integration until one is installed, and
// the empty inventory is itself the assertion: nothing invented appears there.
const ADMINISTRATOR = 'bench-core-admin';
const ADMINISTRATOR_PASSWORD = 'bench-core-admin-password-1';

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
    topology = null;
    throw failure;
  }
  page = await browser.newPage();
});

test.afterAll(async () => {
  // Runs after a passing suite and after a failing one, which is the only
  // reason no owned appliance process or temporary database outlives the run.
  try {
    await page?.close();
  } finally {
    await topology?.dispose();
    topology = null;
  }
});

test('signs in with a human session and opens Inventory from System', async () => {
  const running = requireTopology();
  await page.goto(running.frontendUrl);

  await expect(page.getByRole('heading', { name: 'Sign in to NILO' })).toBeVisible();
  await expect(page.getByText(`Version ${guiVersion.version}`)).toBeVisible();
  await page.getByLabel('Username').fill(ADMINISTRATOR);
  await fillSecret(
    page.getByLabel('Password', { exact: true }),
    ADMINISTRATOR_PASSWORD,
  );
  await page.getByRole('button', { name: 'Sign in', exact: true }).click();

  await expect(page.getByRole('heading', { name: 'About', exact: true })).toBeVisible();
  await expect(
    page.getByRole('tablist', { name: 'Open pages' }).getByRole('tab'),
  ).toHaveCount(1);
  const navigation = page.getByRole('navigation');
  for (const section of ['Instruments', 'System']) {
    await expect(
      navigation.getByRole('button', { name: section, exact: true }),
    ).toHaveAttribute('aria-expanded', 'false');
  }
  await navigation.getByRole('button', { name: 'Instruments', exact: true }).click();
  await expect(navigation.getByText('No instruments are available.')).toBeVisible();
  await navigation.getByRole('button', { name: 'System', exact: true }).click();
  await navigation.getByRole('button', { name: 'Inventory', exact: true }).click();
  await expect(
    page.getByRole('heading', { name: 'Inventory', exact: true }),
  ).toBeVisible();
  await expectApplianceIdentity();

  await expect(statusValue('Profile')).toHaveText('appliance');
  await expect(statusValue('Service status')).toHaveText('healthy');

  // An appliance dispatches power through the privileged helper, so an
  // administrator session offers both controls. They are observed, never
  // clicked: this run has no business restarting the machine it is proving.
  await page.getByRole('button', { name: 'Account' }).click();
  await expect(page.getByRole('menuitem', { name: 'Restart NILO' })).toHaveCount(1);
  await expect(page.getByRole('menuitem', { name: 'Power off NILO' })).toHaveCount(1);
  await page.keyboard.press('Escape');

  // A fresh appliance offers no integration and no device, and says so rather
  // than presenting anything nobody installed.
  await expectEmptyInventory();
});

test('presents Bench Core as unreachable and recovers through the interface', async () => {
  const running = requireTopology();

  await running.stopBackend();
  await page.getByRole('button', { name: 'Refresh' }).click();

  await expect(
    page.getByRole('heading', { name: 'Bench Core is not reachable.' }),
  ).toBeVisible();
  await expect(page.getByText(`Version ${guiVersion.version}`)).toBeVisible();
  await expect(page.getByRole('status', { name: /appliance/i })).toHaveCount(0);

  await running.startBackend();
  await page.getByRole('button', { name: 'Try again' }).click();

  // No reload and no second sign-in: the interface reuses the session it still holds.
  await expect(
    page.getByRole('heading', { name: 'Inventory', exact: true }),
  ).toBeVisible();
  await expectEmptyInventory();
});

/** A fresh appliance remains empty after a backend restart. */
async function expectEmptyInventory(): Promise<void> {
  const integrations = page.getByRole('region', { name: 'Integrations' });
  const devices = page.getByRole('region', { name: 'Devices' });
  await expect(integrations.getByText(/no integrations/i)).toBeVisible();
  await expect(devices.getByText(/no devices/i)).toBeVisible();
  await expect(integrations.getByRole('row')).toHaveCount(0);
  await expect(devices.getByRole('row')).toHaveCount(0);
}

/** One persistent runtime indicator inside the authenticated workspace. */
async function expectApplianceIdentity(): Promise<void> {
  await expect(page.getByRole('status', { name: /appliance/i })).toContainText('LIVE');
  await expect(page.locator('.shell-boundary')).toHaveCount(0);
}

/** The value shown for one labelled fact. */
function statusValue(label: string) {
  return page
    .locator('.fact')
    .filter({ hasText: new RegExp(`^${label}`) })
    .locator('dd');
}

async function fillSecret(field: Locator, secret: string): Promise<void> {
  await withRedactedSecret(secret, () => field.fill(secret));
}

function requireTopology(): DevelopmentTopology {
  if (topology === null) {
    throw new Error('the appliance topology is not running');
  }
  return topology;
}
