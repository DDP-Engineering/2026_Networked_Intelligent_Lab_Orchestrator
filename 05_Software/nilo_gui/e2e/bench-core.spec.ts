import { resolve } from 'node:path';
import { expect, test, type Page } from '@playwright/test';

import { DevelopmentTopology } from './topology';

/**
 * The browser-to-WSL2 simulation path, end to end.
 *
 * A real browser drives the interface development server on Windows; that
 * server forwards `/api` to a real Bench Core compiled and running inside a
 * WSL2 distribution on the simulation profile, with its database on the WSL
 * Linux filesystem. Nothing here is mocked, and nothing here proves anything
 * about physical equipment.
 *
 * The tests run in order and share one browser page: the second one starts
 * from the connected screen the first one reached, which is the only way to
 * observe recovery without re-entering the token.
 */

const guiRoot = process.cwd();
const repositoryRoot = resolve(guiRoot, '..', '..');

const INTEGRATION_ID = 'simulation.fake-python';
const DEVICE_ID = 'simulation.fake-python.device.stable-001';

let topology: DevelopmentTopology | null = null;
let page: Page;

test.describe.configure({ mode: 'serial' });

test.beforeAll(async ({ browser }) => {
  // Compiling Bench Core inside the distribution dominates this hook on a cold
  // Go build cache, so it gets its own budget rather than the test timeout.
  test.setTimeout(900_000);
  try {
    topology = await DevelopmentTopology.start({ repositoryRoot, guiRoot });
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
  // reason no WSL process or temporary database outlives the run.
  try {
    await page?.close();
  } finally {
    await topology?.dispose();
    topology = null;
  }
});

test('signs in with a development token and shows the simulated bench', async () => {
  const running = requireTopology();
  await page.goto(running.frontendUrl);

  await expect(page.getByRole('heading', { name: 'Sign in to NILO' })).toBeVisible();
  await expectSimulationIdentity();

  // The harness types it in. There is no accessor for the value, and a failing
  // fill would otherwise repeat its argument in the error it throws.
  await running.enterTokenInto(page.getByLabel('Development bearer token'));
  await page.getByRole('button', { name: 'Connect' }).click();

  await expect(page.getByRole('heading', { name: 'Bench', exact: true })).toBeVisible();
  await expectSimulationIdentity();

  await expect(statusValue('Profile')).toHaveText('simulation');
  await expect(statusValue('Service status')).toHaveText('healthy');
  await expect(statusValue('Simulation')).toHaveText('Yes');

  await page.getByRole('button', { name: 'Account' }).click();
  await expect(page.getByRole('menuitem', { name: 'Restart NILO' })).toHaveCount(0);
  await expect(page.getByRole('menuitem', { name: 'Power off NILO' })).toHaveCount(0);
  await page.keyboard.press('Escape');

  const integrations = page.getByRole('region', { name: 'Integrations' });
  await expect(integrations).toContainText(INTEGRATION_ID);
  await expect(integrations).toContainText('SIMULATION - Fake Python Integration');
  await expect(integrations).toContainText('healthy');

  const devices = page.getByRole('region', { name: 'Devices' });
  await expect(devices).toContainText(DEVICE_ID);
  await expect(devices).toContainText('SIMULATION - Electronics Bench 001');
  await expect(devices).toContainText('ready');
  // The device names the integration that owns it, not just itself.
  await expect(devices).toContainText(INTEGRATION_ID);
});

test('presents Bench Core as unreachable and recovers through the interface', async () => {
  const running = requireTopology();

  await running.stopBackend();
  await page.getByRole('button', { name: 'Refresh' }).click();

  await expect(
    page.getByRole('heading', { name: 'Bench Core is not reachable.' }),
  ).toBeVisible();
  await expectSimulationIdentity();

  await running.startBackend();
  await page.getByRole('button', { name: 'Try again' }).click();

  // No reload and no second token: the interface reuses the one it still holds.
  await expect(page.getByRole('heading', { name: 'Bench', exact: true })).toBeVisible();
  await expect(page.getByRole('region', { name: 'Devices' })).toContainText(DEVICE_ID);
});

/** One persistent runtime indicator; sign-in also explains the development boundary. */
async function expectSimulationIdentity(): Promise<void> {
  await expect(page.getByRole('status', { name: /runtime profile/i })).toContainText(
    'SIM',
  );
  if (await page.locator('.shell').count()) {
    await expect(page.locator('.shell-boundary')).toHaveCount(0);
  } else {
    await expect(
      page.getByText(
        /simulated results never represent evidence from physical equipment/i,
      ),
    ).toBeVisible();
  }
}

/** The value shown for one labelled fact. */
function statusValue(label: string) {
  return page
    .locator('.fact')
    .filter({ hasText: new RegExp(`^${label}`) })
    .locator('dd');
}

function requireTopology(): DevelopmentTopology {
  if (topology === null) {
    throw new Error('the development topology is not running');
  }
  return topology;
}
