import { resolve } from 'node:path';
import { expect, test, type Page } from '@playwright/test';
import { DevelopmentTopology, withRedactedSecret } from './topology';

test.describe.configure({ mode: 'serial' });
let topology: DevelopmentTopology | null = null;
let page: Page;
const password = 'network-test-administrator';

test.beforeAll(async ({ browser }) => {
  test.setTimeout(900_000);
  try {
    topology = await DevelopmentTopology.start({
      repositoryRoot: resolve(process.cwd(), '..', '..'),
      guiRoot: process.cwd(),
    });
    await topology.bootstrapAdministrator('network-admin', password);
    page = await browser.newPage();
  } catch (error) {
    await topology?.dispose();
    topology = null;
    throw error;
  }
});
test.afterAll(async () => {
  try {
    await page?.close();
  } finally {
    await topology?.dispose();
  }
});

test('shows readiness-only telemetry without inferring tunnel state', async () => {
  // This fixture verifies presentation of the helper's new contract; live
  // helper-to-HTTPS observations are covered in the separate Linux system test.
  await page.route('**/api/v1/network/wireguard', async (route) => {
    await route.fulfill({
      json: {
        api_version: 'nilo.http/1',
        reported: true,
        readiness_only: true,
        measured_by: 'readiness-fixture',
        networkmanager_available: true,
        networkmanager_version: '1.52.1',
        kernel_supported: true,
        helper_available: true,
        can_install: false,
        tools: [{ name: 'wg', installed: true, version: '1.0.20210914' }],
        profiles: [],
        detail: '',
      },
    });
  });
  await page.goto(topology!.frontendUrl);
  await page.getByLabel('Username').fill('network-admin');
  await withRedactedSecret(password, () =>
    page.getByLabel('Password', { exact: true }).fill(password),
  );
  await page.getByRole('button', { name: 'Sign in', exact: true }).click();
  await page
    .getByRole('navigation')
    .getByRole('button', { name: 'Diagnostics', exact: true })
    .click();
  const readiness = page.locator('section[aria-labelledby="network-readiness"]');
  await expect(readiness.getByText('1.0.20210914', { exact: true })).toBeVisible();
  await page
    .getByRole('navigation')
    .getByRole('button', { name: 'VPN profiles', exact: true })
    .click();
  const panel = page.locator('section[aria-labelledby="network-wireguard"]');
  await expect(panel).toContainText('WireGuard profiles have not been reported.');
  await expect(panel.getByRole('button')).toHaveCount(0);
  await page.locator('.network-trigger').click();
  await expect(
    page.locator('.network-summary').getByText('Not reported', { exact: true }),
  ).toHaveCount(2);
  await page.keyboard.press('Escape');
  if (process.env.NILO_NETWORK_REVIEW === '1')
    await page.screenshot({
      path: resolve('test-results', 'network-readiness-review.png'),
      fullPage: true,
    });
  await page.unroute('**/api/v1/network/wireguard');
  // The subsequent existing cases own a fresh sign-in.
  await page.context().clearCookies();
});

/** Open one row's compact Actions dialog and choose an action by its full name. */
async function rowAction(row: string, action: string) {
  await page.getByRole('button', { name: `Actions for ${row}`, exact: true }).click();
  const dialog = page.getByRole('dialog', { name: `Actions for ${row}` });
  await dialog.getByRole('button', { name: new RegExp(`^${action}`) }).click();
}

/** Assert whether an action is enabled or withheld, then close the dialog. */
async function expectRowAction(row: string, action: string, enabled: boolean) {
  await page.getByRole('button', { name: `Actions for ${row}`, exact: true }).click();
  const dialog = page.getByRole('dialog', { name: `Actions for ${row}` });
  const item = dialog.getByRole('button', { name: new RegExp(`^${action}`) });
  if (enabled) {
    await expect(item).toBeEnabled();
  } else {
    await expect(item).toBeDisabled();
  }
  await page.keyboard.press('Escape');
  await expect(dialog).toHaveCount(0);
}

async function applyPreview() {
  const dialog = page.getByRole('dialog');
  await expect(dialog).toBeVisible();
  await dialog.getByRole('button', { name: 'Apply', exact: true }).click();
}

test('keeps opposite-role adapters visible and explains sequential repurposing', async () => {
  await page.context().clearCookies();
  await page.route('**/api/v1/network/status', async (route) => {
    const response = await route.fetch();
    const body = await response.json();
    body.interfaces[0].ipv6 = [
      'fe80::2c78:a86:2f1c:24ae/64',
      'fd11:5ee:bad:c0de::acd:9411/128',
    ];
    await route.fulfill({ json: body });
  });
  // The development topology is intentionally simulation-only and therefore
  // omits the appliance management panel. This presentation fixture supplies
  // an appliance status together with the role-aware management response.
  await page.route('**/api/v1/status', async (route) => {
    await route.fulfill({
      json: {
        api_version: 'nilo.http/1',
        status: 'healthy',
        profile: 'appliance',
        simulation: false,
        build: {
          name: 'nilo',
          version: '0.2.0',
          integration_protocol: 'nilo.integration/1',
        },
      },
    });
  });
  await page.route('**/api/v1/network/management-access*', async (route) => {
    await route.fulfill({
      json: {
        api_version: 'nilo.http/1',
        state: 'recovered',
        port: 8443,
        interfaces: [
          {
            name: 'enp1s0',
            adapter_id: '02:00:00:00:00:01',
            kind: 'ethernet',
            state: 'connected',
            ipv4: ['192.168.10.24/24'],
            ipv6: [],
            enabled: true,
            current: true,
            usable: true,
            role: 'management',
            role_available: true,
            role_detail: '',
            conflicting_resource_id: '',
            conflicting_resource_name: '',
          },
          {
            name: 'wlan0',
            adapter_id: '02:00:00:00:00:02',
            kind: 'wifi',
            state: 'disconnected',
            ipv4: [],
            ipv6: [],
            enabled: false,
            current: false,
            usable: false,
            role: 'device',
            role_available: false,
            role_detail: 'assigned to the device role by PLC line',
            conflicting_resource_id: 'dn-plc',
            conflicting_resource_name: 'PLC line',
          },
        ],
        configured_interfaces: ['enp1s0'],
        urls: [{ url: 'https://nilo.local:8443', kind: 'local' }],
        tls_names: ['nilo.local'],
        tls_missing: [],
        confirm_by: null,
        detail: 'Removed a conflicting management binding for wlan0.',
        outcome: {
          code: 'role_conflict_recovered',
          interface: 'wlan0',
          resource_name: 'PLC line',
        },
        current_interface: 'enp1s0',
      },
    });
  });

  await page.goto(topology!.frontendUrl);
  await page.getByLabel('Username').fill('network-admin');
  await withRedactedSecret(password, () =>
    page.getByLabel('Password', { exact: true }).fill(password),
  );
  await page.getByRole('button', { name: 'Sign in', exact: true }).click();
  const managementObserved = page.waitForResponse((response) =>
    response.url().includes('/api/v1/network/management-access'),
  );
  await page
    .getByRole('navigation')
    .getByRole('button', { name: 'Device access', exact: true })
    .click();
  await managementObserved;
  await page.getByRole('button', { name: 'Add device network', exact: true }).click();
  const editor = page.getByRole('dialog', { name: 'New device network' });
  await expect(
    editor.getByLabel('Device-side adapter').getByRole('option', { name: /enp1s0/ }),
  ).toBeDisabled();
  await expect(editor.getByText(/reserved for management access/i)).toBeVisible();
  await expect(
    editor.getByLabel('Local-access adapter').getByRole('option', { name: /enp1s0/ }),
  ).toBeEnabled();
  await editor.getByRole('button', { name: 'How to repurpose an adapter' }).click();
  const guide = page.getByRole('dialog', { name: 'Safe adapter repurposing' });
  await expect(guide.getByText(/^1\. Establish/)).toBeVisible();
  await expect(guide.getByText(/^2\. Remove/)).toBeVisible();
  await expect(guide.getByText(/^3\. Assign/)).toBeVisible();
  await page.keyboard.press('Escape');
  await page.keyboard.press('Escape');

  await page
    .getByRole('navigation')
    .getByRole('button', { name: 'Overview', exact: true })
    .click();
  const management = page.getByRole('region', {
    name: 'Management access interfaces',
  });
  await expect(
    management.getByRole('checkbox', { name: 'Allow management access on wlan0' }),
  ).toBeDisabled();
  await expect(management.getByText(/Device side: PLC line/i)).toBeVisible();
  await expect(management.getByRole('status')).toHaveCount(1);
  await expect(page.getByRole('table')).toHaveCount(1);
  await expect(
    management.getByText(
      /Management access was removed from wlan0 because PLC line owns its device role/i,
    ),
  ).toBeVisible();
  await expect(
    management.getByText(/Removed a conflicting management binding for wlan0/i),
  ).toHaveCount(0);

  const originalViewport = page.viewportSize()!;
  for (const viewport of [
    { width: 1366, height: 768 },
    { width: 1024, height: 600 },
    { width: 800, height: 480 },
  ]) {
    await page.setViewportSize(viewport);
    await expect(management.getByText('fd11:5ee:bad:c0de::acd:9411/128')).toBeVisible();
    const layout = await management.evaluate((region) => {
      const table = region.querySelector('table')!;
      const states = [...table.querySelectorAll('.network-state')];
      return {
        fits: table.scrollWidth <= region.clientWidth + 1,
        statesFit: states.every((state) => {
          const cell = state.parentElement!;
          return (
            getComputedStyle(state).whiteSpace === 'nowrap' &&
            state.getBoundingClientRect().right <= cell.getBoundingClientRect().right
          );
        }),
      };
    });
    expect(layout).toEqual({ fits: true, statesFit: true });
    if (process.env.NILO_NETWORK_REVIEW === '1') {
      await page.screenshot({
        path: resolve('test-results', `network-unified-${viewport.width}.png`),
      });
    }
  }
  await page.setViewportSize(originalViewport);

  await page.unroute('**/api/v1/network/status');
  await page.unroute('**/api/v1/network/management-access*');
  await page.unroute('**/api/v1/status');
  await page.context().clearCookies();
});

test('shows acknowledged installation progress and its verified result', async () => {
  // Presentation fixture: actual package installation is separately exercised
  // through the privileged helper in the disposable Linux system test.
  let acknowledged = false;
  let complete = false;
  const operation = {
    api_version: 'nilo.http/1',
    id: 'install-fixture',
    action: 'wireguard.install',
    target: 'wireguard',
    revision: 'installation-revision',
    actor_id: 'fixture',
    created_at: new Date().toISOString(),
  };
  await page.route('**/api/v1/network/wireguard', async (route) =>
    route.fulfill({
      json: {
        api_version: 'nilo.http/1',
        reported: true,
        readiness_only: true,
        measured_by: 'installation-fixture',
        networkmanager_available: true,
        networkmanager_version: '1.52.1',
        kernel_supported: true,
        helper_available: true,
        can_install: !complete,
        tools: [
          { name: 'wg', installed: complete, version: complete ? '1.0.20210914' : '' },
        ],
        profiles: [],
        detail: '',
      },
    }),
  );
  await page.route('**/api/v1/network/wireguard/install/preview', async (route) =>
    route.fulfill({ json: { ...operation, state: 'previewed' } }),
  );
  await page.route(
    '**/api/v1/network/operations/install-fixture/acknowledge',
    async (route) => {
      expect(route.request().postDataJSON()).toEqual({ revision: operation.revision });
      acknowledged = true;
      await route.fulfill({
        status: 202,
        json: { ...operation, state: 'applying', progress: 'downloading' },
      });
    },
  );
  await page.route('**/api/v1/network/operations/install-fixture', async (route) =>
    route.fulfill({
      json: {
        ...operation,
        state: complete ? 'confirmed' : 'applying',
        progress: complete ? 'complete' : 'downloading',
      },
    }),
  );
  await page.goto(topology!.frontendUrl);
  await page.getByLabel('Username').fill('network-admin');
  await withRedactedSecret(password, () =>
    page.getByLabel('Password', { exact: true }).fill(password),
  );
  await page.getByRole('button', { name: 'Sign in', exact: true }).click();
  await page
    .getByRole('navigation')
    .getByRole('button', { name: 'VPN profiles', exact: true })
    .click();
  await page
    .getByRole('button', { name: 'Install WireGuard tools', exact: true })
    .click();
  await expect(page.getByRole('dialog')).toContainText('wireguard.install');
  expect(acknowledged).toBe(false);
  await applyPreview();
  await expect(
    page.getByRole('status').filter({ hasText: 'Installing WireGuard tools.' }),
  ).toContainText('Downloading wireguard-tools.');
  await expect(
    page.getByRole('button', { name: 'Install WireGuard tools', exact: true }),
  ).toBeDisabled();
  await expect(
    page.getByRole('button', { name: 'Actions for enp1s0', exact: true }),
  ).toHaveCount(0);
  await expect(
    page.getByRole('button', { name: 'Keep change', exact: true }),
  ).toHaveCount(0);
  if (process.env.NILO_NETWORK_REVIEW === '1')
    await page.screenshot({
      path: resolve('test-results', 'network-installation-review.png'),
      fullPage: true,
    });
  complete = true;
  await expect(
    page.getByText('WireGuard tools are installed and verified.', { exact: true }),
  ).toBeVisible();
  await page
    .getByRole('navigation')
    .getByRole('button', { name: 'Diagnostics', exact: true })
    .click();
  await expect(page.getByText('1.0.20210914', { exact: true })).toBeVisible();
  await page.unrouteAll();
  await page.context().clearCookies();
});

test('reports simulated changes, restores them and permits another change', async () => {
  await page.goto(topology!.frontendUrl);
  await page.getByLabel('Username').fill('network-admin');
  await withRedactedSecret(password, () =>
    page.getByLabel('Password', { exact: true }).fill(password),
  );
  await page.getByRole('button', { name: 'Sign in', exact: true }).click();
  await page
    .getByRole('navigation')
    .getByRole('button', { name: 'Overview', exact: true })
    .click();
  await rowAction('enp1s0', 'Disable enp1s0');
  await applyPreview();
  await expect(
    page
      .getByRole('row')
      .filter({ has: page.getByRole('rowheader', { name: /enp1s0/ }) }),
  ).toContainText(/disconnected/i);
  await page.getByRole('button', { name: 'Keep change', exact: true }).focus();
  await page.keyboard.press('Enter');
  await expect(page.getByText('The change was kept.', { exact: true })).toBeVisible();
  await expectRowAction('enp1s0', 'Enable enp1s0', true);
  await rowAction('enp1s0', 'Enable enp1s0');
  await applyPreview();
  await page
    .getByRole('button', { name: 'Restore previous state', exact: true })
    .click();
  await expect(
    page.getByText('The previous state was restored.', { exact: true }),
  ).toBeVisible();
  await rowAction('enp1s0', 'Enable enp1s0');
  await applyPreview();
  await page.getByRole('button', { name: 'Keep change', exact: true }).click();
});

test('imports, selects startup and toggles a simulated WireGuard tunnel', async () => {
  const conf =
    '[Interface]\nPrivateKey = AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=\nAddress = 10.20.0.2/32, fd00:20::2/128\nDNS = 10.20.0.1\n[Peer]\nPublicKey = ERERERERERERERERERERERERERERERERERERERERERE=\nAllowedIPs = 10.20.0.0/24, fd00:20::/64\nEndpoint = vpn.example.test:51820\nPersistentKeepalive = 0\n';
  await page
    .getByRole('navigation')
    .getByRole('button', { name: 'VPN profiles', exact: true })
    .click();
  await page.getByRole('button', { name: 'Import profile', exact: true }).click();
  await page.getByLabel('Profile name', { exact: true }).fill('test-office');
  await page
    .getByLabel('WireGuard configuration (.conf)', { exact: true })
    .setInputFiles({
      name: 'test.conf',
      mimeType: 'text/plain',
      buffer: Buffer.from(conf),
    });
  await page.getByRole('button', { name: 'Preview import', exact: true }).click();
  await applyPreview();
  await expectRowAction('test-office', 'Enable test-office', true);
  await expect(page.getByText('10.20.0.2/32', { exact: true })).toBeVisible();
  // The endpoint, DNS and routes are secondary: one Details away, never dropped.
  await expect(page.getByText('vpn.example.test:51820', { exact: true })).toHaveCount(
    0,
  );
  await page.getByRole('button', { name: 'Details test-office', exact: true }).click();
  await expect(page.getByText('vpn.example.test:51820', { exact: true })).toBeVisible();
  await expect(page.getByText('10.20.0.1', { exact: true })).toBeVisible();
  await expect(
    page.getByText('10.20.0.0/24, fd00:20::/64', { exact: true }),
  ).toBeVisible();
  await page.getByRole('button', { name: 'Details test-office', exact: true }).click();
  await page.getByRole('button', { name: 'Import profile', exact: true }).click();
  await expect(page.getByLabel('Profile name', { exact: true })).toHaveCount(0);
  await page
    .getByRole('checkbox', { name: 'Connect at startup: test-office', exact: true })
    .click();
  await expect(page.getByRole('dialog')).toContainText('Connect at startup: Enabled');
  await applyPreview();
  await expect(
    page.getByRole('checkbox', {
      name: 'Connect at startup: test-office',
      exact: true,
    }),
  ).toBeChecked();
  await rowAction('test-office', 'Enable test-office');
  await applyPreview();
  await page.getByRole('button', { name: 'Keep change', exact: true }).click();
  await expectRowAction('test-office', 'Disable test-office', true);
  await rowAction('test-office', 'Disable test-office');
  await applyPreview();
  await page.getByRole('button', { name: 'Keep change', exact: true }).click();
  await expect(
    page.getByRole('checkbox', {
      name: 'Connect at startup: test-office',
      exact: true,
    }),
  ).toBeChecked();
  await expectRowAction('test-office', 'Remove test-office', true);
});

test('unconfirmed changes restore automatically and real sampling produces history', async () => {
  test.setTimeout(180_000);
  await page
    .getByRole('navigation')
    .getByRole('button', { name: 'Overview', exact: true })
    .click();
  await rowAction('enp1s0', 'Disable enp1s0');
  await applyPreview();
  await expect(page.getByRole('timer')).toBeVisible();
  // No accelerated server clock and no request to the rollback route: this
  // verifies the actual 120-second deadline and the production runtime loop.
  await expect(
    page.getByText('The previous state was restored.', { exact: true }),
  ).toBeVisible({ timeout: 140_000 });
  await expectRowAction('enp1s0', 'Disable enp1s0', true);
  const history = await page.request.get(
    new URL(
      '/api/v1/network/statistics?interface=sim-enp1s0&period=15m',
      topology!.frontendUrl,
    ).href,
  );
  expect(history.status()).toBe(200);
  const body = (await history.json()) as { points: unknown[]; measured_by: string };
  expect(body.measured_by).toBe('simulation');
  expect(body.points.length).toBeGreaterThan(0);
  expect(
    await page
      .locator('html')
      .evaluate((element) => element.scrollWidth <= element.clientWidth),
  ).toBe(true);
  if (process.env.NILO_NETWORK_REVIEW === '1')
    await page.screenshot({
      path: resolve('test-results', 'network-review.png'),
      fullPage: true,
    });
  if (process.env.NILO_NETWORK_REVIEW === '1') {
    await page
      .getByRole('navigation')
      .getByRole('button', { name: 'Diagnostics', exact: true })
      .click();
    await page
      .getByRole('heading', { name: 'Statistics', exact: true })
      .scrollIntoViewIfNeeded();
    await page.screenshot({
      path: resolve('test-results', 'network-history-review.png'),
      fullPage: true,
    });
  }
});
