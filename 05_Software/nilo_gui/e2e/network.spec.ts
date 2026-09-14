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
  await page.goto(topology!.frontendUrl);
  await page.getByLabel('Username').fill('network-admin');
  await withRedactedSecret(password, () =>
    page.getByLabel('Password', { exact: true }).fill(password),
  );
  await page.getByRole('button', { name: 'Sign in', exact: true }).click();
  await page
    .getByRole('navigation')
    .getByRole('button', { name: 'Device access', exact: true })
    .click();
  await page.getByRole('button', { name: 'Add device network', exact: true }).click();
  const liveEditor = page.getByRole('dialog', { name: 'New device network' });
  const eth0AdapterId =
    (await liveEditor
      .getByLabel('Device-side adapter')
      .getByRole('option', { name: /eth0/ })
      .getAttribute('value')) ?? '';
  const eth1AdapterId =
    (await liveEditor
      .getByLabel('Device-side adapter')
      .getByRole('option', { name: /eth1/ })
      .getAttribute('value')) ?? '';
  expect(eth0AdapterId).not.toBe('');
  expect(eth1AdapterId).not.toBe('');
  await liveEditor.getByRole('button', { name: 'Close', exact: true }).click();
  await page.context().clearCookies();
  let configuredInterfaces = ['eth0'];
  let managementState: 'recovered' | 'pending' | 'applied' = 'recovered';
  let currentInterface = 'eth0';
  let showRoleRecovery = true;
  // The appliance reserves its real browser ingress for management. This
  // presentation fixture adds a second, conflicting role assignment that the
  // disposable two-adapter stage cannot naturally hold at the same time.
  await page.route('**/api/v1/status', async (route) => {
    await route.fulfill({
      json: {
        api_version: 'nilo.http/1',
        status: 'healthy',
        profile: 'appliance',
        simulation: false,
        build: {
          name: 'nilo',
          version: '0.4.0',
          integration_protocol: 'nilo.integration/1',
        },
      },
    });
  });
  await page.route('**/api/v1/network/management-access**', async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    if (request.method() === 'PUT') {
      const body = request.postDataJSON() as {
        interfaces: string[];
        confirm_current_interface: boolean;
      };
      if (configuredInterfaces.length === 1 && configuredInterfaces[0] === 'eth0') {
        expect(body).toEqual({
          interfaces: ['eth0', 'eth1'],
          confirm_current_interface: false,
        });
      } else {
        expect(body).toEqual({
          interfaces: ['eth1'],
          confirm_current_interface: true,
        });
      }
      configuredInterfaces = [...body.interfaces];
      managementState = 'pending';
      showRoleRecovery = false;
    } else if (request.method() === 'POST' && url.pathname.endsWith('/confirm')) {
      expect(managementState).toBe('pending');
      managementState = 'applied';
      if (configuredInterfaces.length === 1) currentInterface = 'eth1';
    } else {
      expect(request.method()).toBe('GET');
    }

    const ownsManagement = (name: string) => configuredInterfaces.includes(name);
    await route.fulfill({
      json: {
        api_version: 'nilo.http/1',
        state: managementState,
        port: 8443,
        interfaces: [
          {
            name: 'eth0',
            adapter_id: eth0AdapterId,
            kind: 'ethernet',
            state: 'connected',
            ipv4: ['192.168.10.24/24'],
            ipv6: [],
            enabled: ownsManagement('eth0'),
            current: currentInterface === 'eth0',
            usable: true,
            role: ownsManagement('eth0') ? 'management' : '',
            role_available: true,
            role_detail: '',
            conflicting_resource_id: '',
            conflicting_resource_name: '',
          },
          {
            name: 'eth1',
            adapter_id: eth1AdapterId,
            kind: 'ethernet',
            state: 'connected',
            ipv4: ['192.168.20.24/24'],
            ipv6: [],
            enabled: ownsManagement('eth1'),
            current: currentInterface === 'eth1',
            usable: true,
            role: ownsManagement('eth1') ? 'management' : '',
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
        configured_interfaces: configuredInterfaces,
        urls: [{ url: 'https://nilo.local:8443', kind: 'local' }],
        tls_names: ['nilo.local'],
        tls_missing: [],
        confirm_by: managementState === 'pending' ? '2026-09-13T15:00:00Z' : null,
        detail: showRoleRecovery
          ? 'Removed a conflicting management binding for wlan0.'
          : '',
        outcome: {
          code: showRoleRecovery ? 'role_conflict_recovered' : '',
          interface: showRoleRecovery ? 'wlan0' : '',
          resource_name: showRoleRecovery ? 'PLC line' : '',
        },
        current_interface: currentInterface,
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
  // The fixture claims the real eth0 identity for management, so the device
  // selector must preserve it but prevent simultaneous assignment.
  await expect(
    editor.getByLabel('Device-side adapter').getByRole('option', { name: /eth0/ }),
  ).toBeDisabled();
  await expect(editor.getByText(/reserved for management access/i)).toBeVisible();
  await expect(
    editor.getByLabel('Local-access adapter').getByRole('option', { name: /eth0/ }),
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
    await expect(management.getByText('fd00:10::2/64')).toBeVisible();
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

  const eth0Management = management.getByRole('checkbox', {
    name: 'Allow management access on eth0',
  });
  const eth1Management = management.getByRole('checkbox', {
    name: 'Allow management access on eth1',
  });

  // The last-route guard prevents removal before a replacement exists.
  await eth0Management.uncheck();
  await expect(
    management.getByText(/Keep at least one connected interface/i),
  ).toBeVisible();
  await expect(
    management.getByRole('button', { name: 'Apply management access' }),
  ).toBeDisabled();
  await eth0Management.check();

  // Establish and confirm eth1 as a replacement while eth0 remains enabled.
  await eth1Management.check();
  await management.getByRole('button', { name: 'Apply management access' }).click();
  await expect(management.getByRole('status')).toHaveText(/Pending confirmation/i);
  await management.getByRole('button', { name: 'Keep change' }).click();
  await expect(management.getByRole('status')).toHaveText(/Applied/i);

  // Removing the browser's current path requires the explicit second warning,
  // and the interface offers no one-click role swap around either guard.
  await eth0Management.uncheck();
  await management.getByRole('button', { name: 'Apply management access' }).click();
  const currentWarning = management.getByRole('alertdialog');
  await expect(currentWarning).toContainText(/This browser reached NILO through eth0/i);
  await expect(currentWarning).toContainText(/previous allowlist will be restored/i);
  await expect(management.getByRole('button', { name: /swap/i })).toHaveCount(0);
  await currentWarning
    .getByRole('button', { name: 'Disable current interface and apply' })
    .click();
  await expect(management.getByRole('status')).toHaveText(/Pending confirmation/i);
  await management.getByRole('button', { name: 'Keep change' }).click();
  await expect(management.getByRole('status')).toHaveText(/Applied/i);

  // Only after both confirmations does eth0 become device-facing availability.
  await page
    .getByRole('navigation')
    .getByRole('button', { name: 'Device access', exact: true })
    .click();
  await page.getByRole('button', { name: 'Add device network', exact: true }).click();
  const replacedEditor = page.getByRole('dialog', { name: 'New device network' });
  await expect(
    replacedEditor
      .getByLabel('Device-side adapter')
      .getByRole('option', { name: /eth0/ }),
  ).toBeEnabled();
  await expect(
    replacedEditor
      .getByLabel('Device-side adapter')
      .getByRole('option', { name: /eth1/ }),
  ).toBeDisabled();
  await expect(
    replacedEditor
      .getByLabel('Local-access adapter')
      .getByRole('option', { name: /eth0/ }),
  ).toBeEnabled();
  await replacedEditor.getByRole('button', { name: 'Close', exact: true }).click();

  await page.unroute('**/api/v1/network/status');
  await page.unroute('**/api/v1/network/management-access**');
  await page.unroute('**/api/v1/status');
  await page.context().clearCookies();
});

test('selects adjacent dense traffic minutes by real pointer position', async () => {
  await page.route('**/api/v1/network/statistics*', async (route) => {
    const from = Date.parse('2026-09-13T10:00:00Z');
    await route.fulfill({
      json: {
        api_version: 'nilo.http/1',
        interface_id: 'dense-fixture',
        period: '1h',
        from: new Date(from).toISOString(),
        to: new Date(from + 60 * 60_000).toISOString(),
        measured_by: 'presentation-fixture',
        stale: false,
        points: Array.from({ length: 60 }, (_, minute) => ({
          minute_start: new Date(from + minute * 60_000).toISOString(),
          rx_bytes: minute === 1 ? 0 : minute + 1,
          tx_bytes: 0,
          rx_errors: 0,
          tx_errors: 0,
          rx_dropped: 0,
          tx_dropped: 0,
          samples: 1,
          stale: false,
        })),
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
  const chart = page.getByRole('listbox', { name: /traffic for dense-fixture/i });
  await expect(chart).toBeVisible();
  const box = await chart.boundingBox();
  expect(box).not.toBeNull();
  const step = ((box?.width ?? 0) * (720 - 64 - 12)) / 720 / 60;
  const plotLeft = ((box?.width ?? 0) * 64) / 720;
  await page.mouse.click((box?.x ?? 0) + plotLeft + step * 1.35, (box?.y ?? 0) + 80);
  await expect(page.getByRole('status', { name: 'Selected minute' })).toContainText(
    /RX 0 B.*TX 0 B.*Total 0 B/i,
  );
  await page.mouse.click((box?.x ?? 0) + plotLeft + step * 2.35, (box?.y ?? 0) + 80);
  await expect(page.getByRole('status', { name: 'Selected minute' })).toContainText(
    /RX 3 B.*TX 0 B.*Total 3 B/i,
  );
  await page.unroute('**/api/v1/network/statistics*');
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
    page.getByRole('button', { name: 'Actions for eth0', exact: true }),
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

test('applies appliance changes, restores them and permits another change', async () => {
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
  await rowAction('eth0', 'Disable eth0');
  await applyPreview();
  await expect(
    page
      .getByRole('row')
      .filter({ has: page.getByRole('rowheader', { name: /eth0/ }) }),
  ).toContainText(/disconnected/i);
  await page.getByRole('button', { name: 'Keep change', exact: true }).focus();
  await page.keyboard.press('Enter');
  await expect(page.getByText('The change was kept.', { exact: true })).toBeVisible();
  await expectRowAction('eth0', 'Enable eth0', true);
  await rowAction('eth0', 'Enable eth0');
  await applyPreview();
  await page
    .getByRole('button', { name: 'Restore previous state', exact: true })
    .click();
  await expect(
    page.getByText('The previous state was restored.', { exact: true }),
  ).toBeVisible();
  await rowAction('eth0', 'Enable eth0');
  await applyPreview();
  await page.getByRole('button', { name: 'Keep change', exact: true }).click();
});

test('imports, selects startup and toggles a WireGuard tunnel', async () => {
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
  test.setTimeout(240_000);
  await page
    .getByRole('navigation')
    .getByRole('button', { name: 'Overview', exact: true })
    .click();
  await rowAction('eth0', 'Disable eth0');
  await applyPreview();
  await expect(page.getByRole('timer')).toBeVisible();
  // No accelerated server clock and no request to the rollback route: this
  // verifies the actual 120-second deadline and the production runtime loop.
  await expect(
    page.getByText('The previous state was restored.', { exact: true }),
  ).toBeVisible({ timeout: 140_000 });
  await expectRowAction('eth0', 'Disable eth0', true);
  const interfaceId = await page.evaluate(async () => {
    const response = await fetch('/api/v1/network/status');
    const status = (await response.json()) as {
      interfaces?: Array<{ id?: string; name?: string }>;
    };
    return status.interfaces?.find((entry) => entry.name === 'eth0')?.id ?? '';
  });
  expect(interfaceId).not.toBe('');
  // Fetch through the browser network stack: Chromium lives on the isolated
  // management ingress, while Playwright's API request client lives beside the
  // appliance and would correctly be refused by the interface firewall.
  let measuredBy = '';
  await expect
    .poll(
      async () => {
        const history = await page.evaluate(async (observedInterfaceId) => {
          const response = await fetch(
            `/api/v1/network/statistics?interface=${encodeURIComponent(observedInterfaceId)}&period=15m`,
          );
          return { status: response.status, body: await response.json() };
        }, interfaceId);
        expect(history.status).toBe(200);
        const body = history.body as { points: unknown[]; measured_by: string };
        measuredBy = body.measured_by;
        return body.points.length;
      },
      { timeout: 75_000 },
    )
    .toBeGreaterThan(0);
  expect(measuredBy).not.toBe('simulation');
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
