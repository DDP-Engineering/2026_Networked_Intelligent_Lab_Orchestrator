import { mkdirSync } from 'node:fs';
import { resolve } from 'node:path';
import { expect, test, type Page } from '@playwright/test';

import { DevelopmentTopology, withRedactedSecret } from './topology';

/**
 * The compact field interface, in a real browser against a real Bench Core
 * on the appliance stage, at the four viewports the seven-inch panel, the field laptop and
 * the desktop present: 1920x1080, 1366x768, 1024x600 and 800x480.
 *
 * What only a browser can settle is here: that no operator-facing text is
 * rendered below 14 pixels, that no interactive target on a field viewport is
 * below 44 pixels, that nothing scrolls the document sideways, that help opens
 * and closes by pointer and by keyboard and stays inside the viewport, that
 * the editors reach their final actions, and that the constrained shell uses a
 * drawer and full-screen sheets. Review screenshots are written only when
 * NILO_FIELD_REVIEW=1; nothing else is retained.
 *
 * It is CSS-pixel proof. Acceptance on the physical panel is a separate step.
 */

const guiRoot = process.cwd();
const repositoryRoot = resolve(guiRoot, '..', '..');

const ADMINISTRATOR = 'field-admin';
// A fixture credential for a temporary database this run creates and deletes.
const ADMINISTRATOR_PASSWORD = 'field-admin-password-1';

const VIEWPORTS = [
  { width: 1920, height: 1080, field: false },
  { width: 1366, height: 768, field: false },
  { width: 1024, height: 600, field: true },
  { width: 800, height: 480, field: true },
] as const;

const REVIEW = process.env.NILO_FIELD_REVIEW === '1';
const REVIEW_DIR = resolve(guiRoot, 'test-results', 'field');

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
  if (REVIEW) mkdirSync(REVIEW_DIR, { recursive: true });
});

test.afterAll(async () => {
  try {
    await page?.close();
  } finally {
    await topology?.dispose();
    topology = null;
  }
});

test('signs in and states then restores a change awaiting confirmation', async () => {
  test.setTimeout(180_000);
  const running = requireTopology();
  await page.setViewportSize({ width: 1024, height: 600 });
  await page.goto(running.frontendUrl);
  await signIn();
  await waitForRealNetworkSample();
  await openNetwork('Overview');
  await page.getByRole('button', { name: 'Actions for eth0', exact: true }).click();
  await page
    .getByRole('dialog', { name: 'Actions for eth0' })
    .getByRole('button', { name: /^Disable eth0/ })
    .click();
  await applyPreview();
  await expect(page.getByRole('timer')).toBeVisible();
  await openNetwork('Overview');
  await expect(page.getByRole('region', { name: 'Overview' })).toContainText(
    'interface.disable',
  );
  await expectNoDocumentOverflow();
  await review('1024x600-light-pending');
  const rollbackAnswer = page.waitForResponse((response) =>
    new URL(response.url()).pathname.endsWith('/rollback'),
  );
  await page
    .getByRole('button', { name: 'Restore previous state', exact: true })
    .click();
  const rollback = await rollbackAnswer;
  const rollbackBody = (await rollback.json()) as {
    state?: string;
    failure?: string;
  };
  expect(rollback.status(), rollbackBody.failure).toBe(200);
  expect(rollbackBody.state, rollbackBody.failure).toBe('rolled_back');
  await expect(
    page.getByText('The previous state was restored.', { exact: true }),
  ).toBeVisible({ timeout: 60_000 });
});

test('saves one device network and one profile to read back at every size', async () => {
  await openNetwork('Device access');

  // A device network, so every later viewport has a record to read.
  await page.getByRole('button', { name: 'Add device network', exact: true }).click();
  const editor = page.getByRole('dialog', { name: 'New device network' });
  // A device network binds a device-facing adapter distinct from the uplink, so
  // the run chooses both the way an operator does.
  await selectAdapter(editor, 'Device-side adapter', /eth1/);
  await selectAdapter(editor, 'Local-access adapter', /eth0/);
  await editor.getByRole('textbox', { name: 'Name', exact: true }).fill('Line 3');
  await editor
    .getByRole('textbox', { name: 'Device address', exact: true })
    .fill('192.168.0.130');
  await editor.getByRole('textbox', { name: /^TCP ports/ }).fill('504');
  await editor
    .getByRole('textbox', { name: /^Authorized local sources/ })
    .fill('192.168.10.0/24');
  await expect(editor.getByRole('region', { name: 'Review' })).toContainText('504');
  await editor.getByRole('button', { name: 'Preview', exact: true }).click();
  await applyPreview();
  await expect(page.getByRole('button', { name: 'Details Line 3' })).toBeVisible();

  // And a profile.
  await openNetwork('VPN profiles');
  const conf =
    '[Interface]\nPrivateKey = AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=\nAddress = 10.30.0.2/32\nDNS = 10.30.0.1\n[Peer]\nPublicKey = ERERERERERERERERERERERERERERERERERERERERERE=\nAllowedIPs = 10.30.0.0/24\nEndpoint = field.example.test:51820\n';
  await page.getByRole('button', { name: 'Import profile', exact: true }).click();
  await page.getByLabel('Profile name', { exact: true }).fill('field-office');
  await page
    .getByLabel('WireGuard configuration (.conf)', { exact: true })
    .setInputFiles({
      name: 'field.conf',
      mimeType: 'text/plain',
      buffer: Buffer.from(conf),
    });
  await page.getByRole('button', { name: 'Preview import', exact: true }).click();
  await applyPreview();
  await expect(
    page.getByRole('button', { name: 'Details field-office' }),
  ).toBeVisible();
});

for (const viewport of VIEWPORTS) {
  const size = `${viewport.width}x${viewport.height}`;

  test(`${size}: the shell and the Network page are readable and complete`, async () => {
    test.setTimeout(240_000);
    await page.setViewportSize({ width: viewport.width, height: viewport.height });
    await page.reload();
    await expect(page.getByRole('heading', { name: 'About', level: 2 })).toBeVisible();

    if (viewport.width <= 896 || viewport.height <= 520) {
      // The constrained shell: a labeled drawer replaces the rail.
      await expect(page.getByRole('navigation')).toHaveCount(0);
      await page.getByRole('button', { name: 'Menu', exact: true }).click();
      const drawer = page.getByRole('dialog', { name: 'Navigation' });
      await expect(drawer.getByRole('button', { name: 'System' })).toHaveAttribute(
        'aria-expanded',
        'false',
      );
      await drawer.getByRole('button', { name: 'System' }).click();
      await drawer.getByRole('button', { name: 'Network', exact: true }).click();
      await expect(drawer).toHaveCount(0);
      await expect(
        page.getByRole('button', { name: 'Menu', exact: true }),
      ).toBeFocused();
    } else {
      await expect(
        page.getByRole('navigation').getByRole('button', { name: 'System' }),
      ).toHaveAttribute('aria-expanded', 'false');
      await openNetwork('Overview');
    }
    await expect(
      page.getByRole('heading', { name: 'Network / Overview', level: 2 }),
    ).toBeVisible();
    await expect(page.getByRole('region', { name: 'Overview' })).toBeVisible();
    await expect(page.getByRole('status', { name: /appliance/i })).toHaveCount(1);

    await expectNoDocumentOverflow();
    await expectReadableText();
    if (viewport.field) await expectFieldTargets();
    await expectNoTitleOnlyExplanation();

    // Each focused page keeps its own records and details.
    for (const [destination, name] of [
      ['Overview', 'eth0'],
      ['VPN profiles', 'field-office'],
      ['Device access', 'Line 3'],
    ] as const) {
      await openNetwork(destination);
      const toggle = page.getByRole('button', { name: `Details ${name}`, exact: true });
      await toggle.scrollIntoViewIfNeeded();
      await toggle.click();
      const region = page.getByRole('region', { name: `Details ${name}` });
      await expect(region).toBeVisible();
      if (name === 'eth0') await expect(region).toContainText('Mbit/s');
      if (name === 'field-office')
        await expect(region).toContainText('field.example.test');
      if (name === 'Line 3') await expect(region).toContainText('192.168.10.0/24');
      await toggle.click();
      await expect(region).toHaveCount(0);
    }
    await expectNoDocumentOverflow();

    // The traffic graphic explains itself at field sizes and an exact sample
    // remains reachable without relying on hover.
    await openNetwork('Diagnostics');
    const trafficChart = page.getByRole('listbox', {
      name: /traffic for .*eth0/i,
    });
    await expect(trafficChart).toBeVisible();
    if (viewport.width >= 1024 && viewport.height >= 600) {
      await expectNoWorkspaceVerticalOverflow();
    } else {
      await trafficChart.scrollIntoViewIfNeeded();
    }
    await expect(page.getByText('RX · Received', { exact: true })).toBeVisible();
    await expect(page.getByText('TX · Sent', { exact: true })).toBeVisible();
    const trafficSamples = trafficChart.getByRole('option');
    await expect(trafficSamples.first()).toBeVisible();
    await trafficChart.focus();
    await page.keyboard.press('Home');
    await expect(trafficSamples.first()).toHaveAttribute('aria-selected', 'true');
    await expect(page.getByRole('status', { name: 'Selected minute' })).toContainText(
      /RX .* TX .* Total /,
    );
    await expectInsideViewport(trafficChart);
    await expectNoDocumentOverflow();
    if (viewport.field) await expectFieldTargets();

    // The compact dialog is immediately visible above the table; loopback has none.
    await openNetwork('Overview');
    const scrollBefore = await page
      .locator('.shell-workspace')
      .evaluate((workspace) => workspace.scrollTop);
    await page.getByRole('button', { name: 'Actions for eth0', exact: true }).click();
    const actionDialog = page.getByRole('dialog', { name: 'Actions for eth0' });
    await expect(actionDialog).toContainText('Network interface');
    await expect(actionDialog).toContainText('Connected');
    await expectInsideViewport(actionDialog);
    await expect(
      actionDialog.getByRole('button', { name: /^Disable eth0/ }),
    ).toBeEnabled();
    await expect(
      actionDialog.getByRole('button', { name: /^Configure eth0/ }),
    ).toBeEnabled();
    expect(
      await page
        .locator('.shell-workspace')
        .evaluate((workspace) => workspace.scrollTop),
    ).toBe(scrollBefore);
    await page.keyboard.press('Escape');
    await expect(actionDialog).toHaveCount(0);
    await expect(page.getByRole('button', { name: 'Actions for lo' })).toHaveCount(0);

    await review(`${size}-light-network`);

    // The adapter editor reaches its final actions.
    await page.getByRole('button', { name: 'Actions for eth0', exact: true }).click();
    await page
      .getByRole('dialog', { name: 'Actions for eth0' })
      .getByRole('button', { name: /^Configure eth0/ })
      .click();
    const adapter = page.getByRole('dialog', { name: 'Configure eth0' });
    await expect(
      adapter.getByRole('heading', { name: 'Configure eth0' }),
    ).toBeVisible();
    await expect(
      adapter.getByRole('button', { name: 'How to configure' }),
    ).toBeVisible();
    await expect(
      adapter.getByRole('button', { name: 'Preview', exact: true }),
    ).toBeVisible();
    await expectInsideViewport(
      adapter.getByRole('button', { name: 'Preview', exact: true }),
    );
    await adapter.getByRole('button', { name: 'Close', exact: true }).click();
    await expect(adapter).toHaveCount(0);

    // The device-network editor: help by pointer and by keyboard, validation
    // beside the field, the sticky action bar, the sheet on a small display.
    await openNetwork('Device access');
    await page.getByRole('button', { name: 'Add device network', exact: true }).click();
    const editor = page.getByRole('dialog', { name: 'New device network' });
    await expect(editor).toBeVisible();
    if (viewport.width <= 896 || viewport.height <= 640) {
      const box = await editor.boundingBox();
      expect(box?.width).toBe(viewport.width);
      expect(box?.height).toBe(viewport.height);
    }

    const help = editor.getByRole('button', { name: /Help for Device address/ });
    await help.scrollIntoViewIfNeeded();
    await help.click();
    const popover = page.getByRole('dialog', { name: /Help for Device address/ });
    await expect(popover).toContainText('192.168.0.130');
    await expectInsideViewport(popover);
    await page.keyboard.press('Escape');
    await expect(popover).toHaveCount(0);
    await expect(help).toBeFocused();
    await expect(editor).toBeVisible();
    await page.keyboard.press('Enter');
    await expect(popover).toBeVisible();
    await popover.getByRole('button', { name: 'Close help' }).click();
    await expect(popover).toHaveCount(0);
    await expect(help).toBeFocused();

    await editor.getByRole('textbox', { name: 'Name', exact: true }).fill('Bad');
    await editor
      .getByRole('textbox', { name: /^Appliance address/ })
      .fill('192.168.0.1');
    await editor
      .getByRole('textbox', { name: 'Device address', exact: true })
      .fill('192.168.0.130');
    await editor.getByRole('textbox', { name: /^TCP ports/ }).fill('70000');
    const preview = editor.getByRole('button', { name: 'Preview', exact: true });
    await preview.click();
    await expect(editor.getByText(/prefix length/)).toBeVisible();
    await expect(editor.getByText(/between 1 and 65535/)).toBeVisible();
    await expect(page.getByRole('dialog', { name: 'Confirm this change' })).toHaveCount(
      0,
    );

    // Scroll the body to the top: the actions stay reachable at the bottom.
    await editor.locator('.modal-body').evaluate((body) => body.scrollTo(0, 0));
    await expectInsideViewport(preview);
    await expectInsideViewport(
      editor.getByRole('heading', { name: 'New device network' }),
    );
    await review(`${size}-light-editor`);
    await page.keyboard.press('Escape');
    await expect(editor).toHaveCount(0);

    await exerciseAuthentication(viewport);
    await review(`${size}-light-authentication`);

    // Dark theme, same page, same checks on sizes and overflow.
    await setTheme('dark');
    await openNetworkAfterReload(viewport);
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');
    await expectNoDocumentOverflow();
    await expectReadableText();
    await review(`${size}-dark-network`);
    await exerciseAuthentication(viewport);
    await review(`${size}-dark-authentication`);
    await setTheme('light');
    await openNetworkAfterReload(viewport);
  });
}

test('1024x600: the other pages stay readable without sideways scrolling', async () => {
  await page.setViewportSize({ width: 1024, height: 600 });
  await openNetworkAfterReload({ width: 1024, height: 600 });
  for (const [entry, heading] of [
    ['Inventory', 'Inventory'],
    ['Activity', 'Activity'],
    ['Authentication', 'Authentication / Users'],
    ['API tokens', 'Authentication / API tokens'],
    ['About', 'About'],
  ] as const) {
    await openSystemDestination(entry, false);
    await expect(page.getByRole('heading', { name: heading, level: 2 })).toBeVisible();
    await expectNoDocumentOverflow();
    await expectReadableText();
    await expectFieldTargets();
    await review(`1024x600-light-${entry.toLowerCase()}`);
  }
});

// The audit listing is the page an operator reads longest, so it is exercised
// at both field viewports rather than only walked past: the chips re-query, a
// record opens in a dialog that fits, and nothing scrolls the document
// sideways at 800x480 where the six columns become labelled records.
for (const viewport of VIEWPORTS.filter((candidate) => candidate.field)) {
  const size = `${viewport.width}x${viewport.height}`;
  test(`${size}: the activity listing reads as records and opens one in a dialog`, async () => {
    await page.setViewportSize({ width: viewport.width, height: viewport.height });
    await openActivity();

    const chips = page.getByRole('group', { name: 'Category' });
    await expect(
      chips.getByRole('button', { name: 'All', exact: true }),
    ).toHaveAttribute('aria-pressed', 'true');
    await chips.getByRole('button', { name: 'Audit', exact: true }).click();
    await expect(
      chips.getByRole('button', { name: 'Audit', exact: true }),
    ).toHaveAttribute('aria-pressed', 'true');
    await expectNoDocumentOverflow();

    // REQ-UX-ba46f6d3: the scope row is a flex item of the scrolling page body
    // here, so it is measured against what is actually on screen rather than
    // against its own box. A row that shrank under the retention disclosure
    // would still report 44px buttons while showing a sliver of each.
    const scopeRow = await chips.evaluate((row) => {
      type Element_ = typeof row;
      const box = row.getBoundingClientRect();
      const buttons = Array.from(
        row.querySelectorAll('button') as unknown as ArrayLike<Element_>,
      ).map((button) => {
        const target = button.getBoundingClientRect();
        return {
          name: button.textContent?.trim() ?? '',
          height: Math.round(target.height),
          // What survives the row's own clipping: the part of the control the
          // operator can see and put a finger on.
          visible: Math.round(
            Math.min(target.bottom, box.bottom) - Math.max(target.top, box.top),
          ),
        };
      });
      return { height: Math.round(box.height), buttons };
    });
    const scopeGeometry = JSON.stringify(scopeRow);
    for (const button of scopeRow.buttons) {
      expect(
        button.visible,
        `visible height of ${button.name} ${scopeGeometry}`,
      ).toBeGreaterThanOrEqual(44);
    }

    // Records first: the whole filter panel is a closed drawer here, and the
    // first record starts inside the viewport without scrolling.
    await expect(page.getByRole('form', { name: 'Filters' })).toBeHidden();
    // REQ-UX-dd4b2c16 at 1024x600: the whole first record, not merely its top
    // edge, is on screen before anything is scrolled. At 800x480 the row is a
    // labelled card several lines tall, so only its start is required there.
    const recordBox = await page
      .locator('.activity-table tbody tr')
      .first()
      .boundingBox();
    expect(recordBox, 'first record').not.toBeNull();
    const recordEnd =
      viewport.width >= 1024
        ? (recordBox?.y ?? Number.POSITIVE_INFINITY) + (recordBox?.height ?? 0)
        : (recordBox?.y ?? Number.POSITIVE_INFINITY);
    expect(recordEnd, 'first record inside the viewport').toBeLessThanOrEqual(
      viewport.height,
    );

    // Opening the drawer, then the three less frequent filters behind their
    // own disclosure, must not push anything off the side of a 800-pixel panel.
    await page.locator('summary', { hasText: 'Filters' }).first().click();
    await expect(page.getByRole('form', { name: 'Filters' })).toBeVisible();
    await page.getByText('More filters', { exact: true }).click();
    await expect(page.getByLabel('Operation ID')).toBeVisible();
    await expectNoDocumentOverflow();
    await expectFieldTargets();
    await page.locator('summary', { hasText: 'Filters' }).first().click();
    await expect(page.getByRole('form', { name: 'Filters' })).toBeHidden();

    const details = page.getByRole('button', { name: /^Details for entry / }).first();
    await details.click();
    const dialog = page.getByRole('dialog', { name: /^Activity entry / });
    await expect(dialog).toContainText('Event ID');
    await expect(dialog).toContainText('Time source');
    await expectInsideViewport(dialog);
    await expectReadableText();
    await expectFieldTargets();
    await review(`${size}-light-activity-details`);
    await page.keyboard.press('Escape');
    await expect(dialog).toBeHidden();

    // REQ-UX-0291ba55: retention is the only administration on the page, and
    // it is reached above the filters rather than past every row it governs.
    await expect(page.getByText('Temporary debug logging')).toHaveCount(0);
    await expect(page.getByRole('button', { name: 'Debug' })).toHaveCount(0);
    await expect(page.getByLabel('Debug retention (days)')).toHaveCount(0);
    await expect(page.getByLabel('Debug quota (MiB)')).toHaveCount(0);
    await page.getByText('Activity retention', { exact: true }).click();
    await expect(page.getByRole('form', { name: 'Retention policy' })).toBeVisible();
    await expectNoDocumentOverflow();
    await expectFieldTargets();
    await review(`${size}-light-activity-retention`);
  });
}

// The Activity header is one line at every width, Export at its right edge
// (REQ-UX-3e01a5b6). This runs at all four viewports on purpose: a
// narrow-viewport rule that drops the auto margin leaves Export sitting against
// the title, and a check at only the two widest sizes would never see it.
for (const viewport of VIEWPORTS) {
  const size = `${viewport.width}x${viewport.height}`;
  test(`${size}: the activity header keeps Export at its right edge`, async () => {
    await page.setViewportSize({ width: viewport.width, height: viewport.height });
    await openActivity();

    const header = page.locator('header.page-head');
    const title = await header.getByRole('heading', { name: 'Activity' }).boundingBox();
    const exported = await header.getByRole('button', { name: 'Export' }).boundingBox();
    expect(title, 'page title').not.toBeNull();
    expect(exported, 'export control').not.toBeNull();
    expect(sharesRow(title, exported), 'export beside the title').toBe(true);
    const headerBox = await header.boundingBox();
    // Top right: nothing of the header stands to the right of Export.
    expect(
      (exported?.x ?? 0) + (exported?.width ?? 0),
      'export at the right edge of the header',
    ).toBeGreaterThan((headerBox?.x ?? 0) + (headerBox?.width ?? 0) - 64);
    // The header carries no description: the tab already said what this is.
    await expect(header.locator('p')).toHaveCount(0);
    await expectNoDocumentOverflow();
    if (viewport.field) await expectFieldTargets();
  });
}

// Where the filter panel is shown inline, scope, the six fields and the More
// filters disclosure are one wrapping line (REQ-UX-f8c58c4c). Only the two
// widest viewports show that panel inline; 1366 is the width that proves the
// line wraps instead of reverting to a row per control.
for (const width of [1920, 1366] as const) {
  test(`${width}: the activity filters are one wrapping line`, async () => {
    await page.setViewportSize({ width, height: width === 1920 ? 1080 : 768 });
    await openActivity();

    const filters = page.getByRole('form', { name: 'Filters' });
    await expect(filters).toBeVisible();

    // Read the flex items themselves, in document order, and say which of them
    // begin a line. A wrapped line restarts at the container's content edge, so
    // the left edge is what identifies a line start - unlike vertical overlap,
    // which a line of differently aligned controls does not produce even when
    // every one of them is on it.
    const line = await filters.evaluate((form) => {
      type Element_ = typeof form;
      const children = Array.from(form.children as unknown as ArrayLike<Element_>);
      return children
        .filter((element) => !element.className.includes('form-actions'))
        .map((element) => {
          const box = element.getBoundingClientRect();
          return {
            name: element.className.split(' ').pop() ?? element.tagName,
            x: Math.round(box.x),
            width: Math.round(box.width),
            bottom: Math.round(box.bottom),
          };
        })
        .filter((item) => item.width > 0);
    });

    const geometry = JSON.stringify(line);
    // Scope, the six fields and the disclosure, and nothing else.
    expect(line.length, `controls on the filter line ${geometry}`).toBe(8);
    const edge = Math.min(...line.map((item) => item.x));
    const starts = line
      .map((item, index) => (Math.abs(item.x - edge) <= 2 ? index : -1))
      .filter((index) => index >= 0);
    // Every line carries at least two controls: none holds a row of its own,
    // which is the whole of REQ-UX-f8c58c4c that survives a change of width.
    for (const [ordinal, start] of starts.entries()) {
      const end = starts[ordinal + 1] ?? line.length;
      expect(
        end - start,
        `controls on line ${ordinal + 1} of ${geometry}`,
      ).toBeGreaterThanOrEqual(2);
    }
    if (width === 1920) {
      expect(starts.length, `one line at 1920 ${geometry}`).toBe(1);
    }

    await expectNoDocumentOverflow();
    await expectReadableText();
    await review(`${width}x${page.viewportSize()?.height}-light-activity-line`);
  });
}

/* ---------- helpers ---------- */

/** Whether two laid-out boxes overlap vertically, and so share a row. */
function sharesRow(
  left: { y: number; height: number } | null,
  right: { y: number; height: number } | null,
): boolean {
  if (left === null || right === null) return false;
  return left.y < right.y + right.height && right.y < left.y + left.height;
}

/**
 * Open Activity on a page that has just been reloaded.
 *
 * The suite shares one browser page, and this screen keeps the chosen category
 * and the two disclosures in component state, so a run that inherited them from
 * the previous viewport would be asserting the last test's leftovers.
 */
async function openActivity(): Promise<void> {
  await page.reload();
  await expect(page.getByRole('heading', { name: 'About', level: 2 })).toBeVisible();
  const viewport = page.viewportSize();
  const constrained =
    viewport !== null && (viewport.width <= 896 || viewport.height <= 520);
  await openSystemDestination('Activity', constrained);
  await expect(page.getByRole('heading', { name: 'Activity', level: 2 })).toBeVisible();
}

async function signIn(): Promise<void> {
  await page.getByLabel('Username').fill(ADMINISTRATOR);
  await withRedactedSecret(ADMINISTRATOR_PASSWORD, () =>
    page.getByLabel('Password', { exact: true }).fill(ADMINISTRATOR_PASSWORD),
  );
  await page.getByRole('button', { name: 'Sign in', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'About', level: 2 })).toBeVisible();
}

/** Wait until the appliance sampler has persisted a real interval for the chart. */
async function waitForRealNetworkSample(): Promise<void> {
  const interfaceId = await page.evaluate(async () => {
    const response = await fetch('/api/v1/network/status');
    const status = (await response.json()) as {
      interfaces?: Array<{ id?: string; name?: string }>;
    };
    return status.interfaces?.find((entry) => entry.name === 'eth0')?.id ?? '';
  });
  expect(interfaceId).not.toBe('');

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
}

type NetworkDestination = 'Overview' | 'Device access' | 'VPN profiles' | 'Diagnostics';

async function expectPageActivityBesideTitle(): Promise<void> {
  const header = page.locator('#workspace-panel .page-head');
  const group = header.locator('.page-title-actions');
  const title = group.getByRole('heading', { level: 2 });
  const action = group.getByRole('button', { name: 'View activity', exact: true });
  await expect(action).toBeVisible();
  await expect(
    header.getByRole('button', { name: 'View activity', exact: true }),
  ).toHaveCount(1);
  const titleBox = await title.boundingBox();
  const actionBox = await action.boundingBox();
  const groupBox = await group.boundingBox();
  expect(titleBox).not.toBeNull();
  expect(actionBox).not.toBeNull();
  expect(groupBox).not.toBeNull();
  if (!titleBox || !actionBox || !groupBox)
    throw new Error('Missing title/action geometry');
  expect(actionBox.width).toBeGreaterThanOrEqual(44);
  expect(actionBox.height).toBeGreaterThanOrEqual(44);
  // When both controls fit, they must be adjacent, not pushed to the far right.
  if (titleBox.width + actionBox.width + 16 <= groupBox.width) {
    expect(
      Math.abs(titleBox.y + titleBox.height / 2 - actionBox.y - actionBox.height / 2),
    ).toBeLessThan(2);
    expect(actionBox.x - titleBox.x - titleBox.width).toBeGreaterThanOrEqual(0);
    expect(actionBox.x - titleBox.x - titleBox.width).toBeLessThanOrEqual(24);
  } else {
    expect(actionBox.y).toBeGreaterThanOrEqual(titleBox.y + titleBox.height);
  }
  expect(actionBox.x + actionBox.width).toBeLessThanOrEqual(
    groupBox.x + groupBox.width + 1,
  );
}

async function openNetwork(destination: NetworkDestination): Promise<void> {
  const viewport = page.viewportSize();
  const constrained =
    viewport !== null && (viewport.width <= 896 || viewport.height <= 520);
  await openSystemDestination(destination, constrained);
  await expect(
    page.getByRole('heading', { name: `Network / ${destination}`, level: 2 }),
  ).toBeVisible();
  await expectPageActivityBesideTitle();
}

async function openNetworkAfterReload(viewport: { width: number; height: number }) {
  await page.reload();
  await expect(page.getByRole('heading', { name: 'About', level: 2 })).toBeVisible();
  await openSystemDestination(
    'Overview',
    viewport.width <= 896 || viewport.height <= 520,
  );
  await expect(
    page.getByRole('heading', { name: 'Network / Overview', level: 2 }),
  ).toBeVisible();
}

async function openSystemDestination(
  destination: string,
  constrained: boolean,
): Promise<void> {
  if (constrained)
    await page.getByRole('button', { name: 'Menu', exact: true }).click();
  const nav = constrained
    ? page.getByRole('dialog', { name: 'Navigation' })
    : page.getByRole('navigation');
  const system = nav.getByRole('button', { name: 'System', exact: true });
  if ((await system.getAttribute('aria-expanded')) === 'false') await system.click();
  if (
    ['Overview', 'Device access', 'VPN profiles', 'Diagnostics'].includes(destination)
  ) {
    const network = nav.getByRole('button', { name: 'Expand Network' });
    if (await network.isVisible()) await network.click();
  }
  if (destination === 'API tokens') {
    const authentication = nav.getByRole('button', { name: 'Expand Authentication' });
    if (await authentication.isVisible()) await authentication.click();
  }
  await nav.getByRole('button', { name: destination, exact: true }).click();
}

async function openAuthenticationView(
  destination: 'Users' | 'API tokens',
): Promise<void> {
  const viewport = page.viewportSize();
  const constrained =
    viewport !== null && (viewport.width <= 896 || viewport.height <= 520);
  await openSystemDestination(destination, constrained);
  const title =
    destination === 'Users' ? 'Authentication / Users' : 'Authentication / API tokens';
  await expect(page.getByRole('heading', { name: title, level: 2 })).toBeVisible();
  await expectPageActivityBesideTitle();
}

async function exerciseAuthentication(viewport: (typeof VIEWPORTS)[number]) {
  await openAuthenticationView('Users');
  const users = page.getByRole('region', { name: 'Users' });
  await expect(users).toBeVisible();
  const details = users.getByRole('button', {
    name: `Details ${ADMINISTRATOR}`,
    exact: true,
  });
  await details.click();
  await expect(users).toContainText('Created');
  await details.click();

  await users
    .getByRole('button', { name: `Actions for ${ADMINISTRATOR}`, exact: true })
    .click();
  const actions = page.getByRole('dialog', {
    name: `Actions for ${ADMINISTRATOR}`,
  });
  await expect(actions).toContainText('User identity');
  await expect(actions.getByRole('button', { name: 'Disable' })).toBeDisabled();
  await expect(actions).toContainText(
    'The last enabled administrator must remain enabled.',
  );
  // The permanent removal is offered in the destructive group and is refused
  // here for its own stated reason, at every viewport this suite covers.
  await expect(
    actions.getByRole('button', { name: 'Delete permanently' }),
  ).toBeDisabled();
  await expect(actions).toContainText(
    'The last enabled administrator cannot be deleted.',
  );
  await expectInsideViewport(actions);
  if (viewport.field) await expectFieldTargets();
  await actions.getByRole('button', { name: 'Cancel' }).click();

  await page.getByRole('button', { name: 'New user', exact: true }).click();
  const editor = page.getByRole('dialog', { name: 'New user' });
  const fieldHelp = editor.getByRole('button', { name: 'Help for Username' });
  await fieldHelp.click();
  const popover = page.getByRole('dialog', { name: 'Help for Username' });
  await expect(popover).toContainText('stable sign-in name');
  await expectInsideViewport(popover);
  await popover.getByRole('button', { name: 'Close help' }).click();
  await editor.getByRole('button', { name: 'How to configure' }).click();
  const workflow = page.getByRole('dialog', { name: 'Authentication workflow' });
  await expect(workflow).toContainText('What NILO will change');
  await expect(workflow).toContainText('Verify and recover');
  await expectInsideViewport(workflow);
  await workflow.getByRole('button', { name: 'Close' }).last().click();
  await editor.getByRole('button', { name: 'Close' }).click();

  await openAuthenticationView('API tokens');
  await expect(page.getByRole('region', { name: 'API Tokens' })).toBeVisible();
  await expectNoDocumentOverflow();
  await expectReadableText();
  if (viewport.field) await expectFieldTargets();
}

async function setTheme(theme: 'light' | 'dark'): Promise<void> {
  await page
    .locator('body')
    .evaluate(
      (body, value) =>
        body.ownerDocument.defaultView?.localStorage.setItem('nilo.theme', value),
      theme,
    );
}

async function applyPreview(): Promise<void> {
  const dialog = page.getByRole('dialog', { name: 'Confirm this change' });
  await expect(dialog).toBeVisible();
  await dialog.getByRole('button', { name: 'Apply', exact: true }).click();
}

async function review(name: string): Promise<void> {
  if (!REVIEW) return;
  await page.screenshot({ path: resolve(REVIEW_DIR, `${name}.png`) });
}

async function expectNoDocumentOverflow(): Promise<void> {
  const overflow = await page
    .locator('html')
    .evaluate((root) => root.scrollWidth - root.clientWidth);
  expect(overflow, 'document-level horizontal overflow').toBeLessThanOrEqual(1);
}

/** Diagnostics is an at-a-glance page from 1024x600 upward. */
async function expectNoWorkspaceVerticalOverflow(): Promise<void> {
  const overflow = await page
    .locator('.page-body')
    .evaluate((body) => body.scrollHeight - body.clientHeight);
  expect(overflow, 'Diagnostics workspace vertical overflow').toBeLessThanOrEqual(1);
}

/**
 * Every visible text node on the page renders at 14 pixels or more.
 *
 * The browser callbacks below run in the page and are written without naming a
 * DOM global: this file is compiled with the Node library, like the rest of the
 * harness, so `document` and `HTMLElement` are not types it can see. Everything
 * is reached from the body element the locator hands in.
 */
async function expectReadableText(): Promise<void> {
  const small = await page.locator('body').evaluate((body) => {
    const view = body.ownerDocument.defaultView;
    if (view === null) return [];
    const found: string[] = [];
    // 4 is NodeFilter.SHOW_TEXT, named numerically for the same reason.
    const walker = body.ownerDocument.createTreeWalker(body, 4);
    let node = walker.nextNode();
    while (node !== null) {
      const element = node.parentElement;
      const text = node.textContent?.trim() ?? '';
      node = walker.nextNode();
      if (text === '' || element === null) continue;
      const style = view.getComputedStyle(element);
      if (style.display === 'none' || style.visibility === 'hidden') continue;
      const box = element.getBoundingClientRect();
      if (box.width === 0 || box.height === 0) continue;
      const px = parseFloat(style.fontSize);
      if (px < 14) {
        found.push(
          `${element.tagName}.${element.className} ${px}px "${text.slice(0, 30)}"`,
        );
      }
    }
    return found;
  });
  expect(small, 'text rendered below 14px').toEqual([]);
}

/** Every visible interactive element on a field viewport is at least 44x44. */
async function expectFieldTargets(): Promise<void> {
  const short = await page.locator('body').evaluate((body) => {
    const view = body.ownerDocument.defaultView;
    if (view === null) return [];
    const found: string[] = [];
    // The element type is taken from the body the locator handed in, because
    // this file is compiled without the DOM library and cannot name it.
    type Element_ = typeof body;
    const targets = body.querySelectorAll(
      'button, a[href], input:not([type=checkbox]):not([type=radio]), select, [role=button], [role=listbox], [role=tab], label:has(> input[type=checkbox])',
    ) as unknown as ArrayLike<Element_>;
    for (const element of Array.from(targets)) {
      const style = view.getComputedStyle(element);
      if (style.display === 'none' || style.visibility === 'hidden') continue;
      const box = element.getBoundingClientRect();
      if (box.width === 0 || box.height === 0) continue;
      if (box.width < 44 || box.height < 44) {
        const name =
          element.getAttribute('aria-label') ?? element.textContent?.trim() ?? '';
        found.push(
          `${element.tagName}.${element.className} ${Math.round(box.width)}x${Math.round(box.height)} "${name.slice(0, 30)}"`,
        );
      }
    }
    return found;
  });
  expect(short, 'interactive target below 44x44').toEqual([]);
}

/** No explanation on the page lives only in a title attribute. */
async function expectNoTitleOnlyExplanation(): Promise<void> {
  const titles = await page.locator('.shell-workspace [title]').count();
  expect(titles, 'title attributes inside the workspace').toBe(0);
}

async function expectInsideViewport(
  locator: ReturnType<Page['getByRole']>,
): Promise<void> {
  const box = await locator.boundingBox();
  const size = page.viewportSize();
  expect(box).not.toBeNull();
  expect(size).not.toBeNull();
  if (box === null || size === null) return;
  expect(box.x).toBeGreaterThanOrEqual(0);
  expect(box.y).toBeGreaterThanOrEqual(0);
  expect(box.x + box.width).toBeLessThanOrEqual(size.width + 1);
  expect(box.y + box.height).toBeLessThanOrEqual(size.height + 1);
}

function requireTopology(): DevelopmentTopology {
  if (topology === null) {
    throw new Error('the development topology is not running');
  }
  return topology;
}

/** Choose one adapter by the name the interface shows for it. */
async function selectAdapter(
  editor: ReturnType<Page['getByRole']>,
  label: string,
  name: RegExp,
): Promise<void> {
  // The help trigger carries the same accessible name, so the control is
  // matched exactly rather than by prefix.
  const select = editor.getByLabel(label, { exact: true });
  const value = await select.getByRole('option', { name }).getAttribute('value');
  await select.selectOption(value ?? '');
}
