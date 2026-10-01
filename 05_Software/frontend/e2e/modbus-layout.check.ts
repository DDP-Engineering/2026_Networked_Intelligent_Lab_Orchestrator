import { test, expect } from '@playwright/test';

const labels = [
  ['en', 'Modbus administration', 'New TCP endpoint', 'Host', 'Register Maps'],
  ['de', 'Modbus-Verwaltung', 'Neuer TCP-Endpunkt', 'Host', 'Registerabbildungen'],
  ['ro', 'Administrare Modbus', 'Punct TCP nou', 'Gazdă', 'Hărți de registre'],
] as const;

for (const [language, heading, add, host, mapTitle] of labels) {
  test(`Modbus admin keeps ${language} controls usable`, async ({ page }) => {
    await page.setViewportSize({ width: 320, height: 640 });
    await page.goto(`/e2e/modbus.fixture.html?lang=${language}`);
    await expect(page.getByRole('heading', { name: heading })).toBeVisible();
    const addButton = page.getByRole('button', { name: add });
    await addButton.focus();
    await page.keyboard.press('Enter');
    await expect(page.getByLabel(host)).toBeVisible();
    const box = await addButton.boundingBox();
    expect(box!.height).toBeGreaterThanOrEqual(44);
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1),
    ).toBe(true);
    await page.goto(`/e2e/modbus.fixture.html?view=workspace&lang=${language}`);
    await expect(page.getByRole('heading', { name: mapTitle })).toBeVisible();
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1),
    ).toBe(true);
  });
}

for (const [width, height] of [
  [800, 480],
  [1024, 600],
  [1920, 1080],
]) {
  test(`Modbus admin and offline workspace reflow at ${width}x${height}`, async ({
    page,
  }) => {
    await page.setViewportSize({ width, height });
    await page.goto('/e2e/modbus.fixture.html');
    await expect(
      page.getByRole('button', { name: 'Edit Offline boiler' }),
    ).toBeVisible();
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1),
    ).toBe(true);
    await page.goto('/e2e/modbus.fixture.html?view=workspace');
    await expect(page.getByRole('heading', { name: 'Register Maps' })).toBeVisible();
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1),
    ).toBe(true);
  });
}

for (const [language, workspaceLabel] of [
  ['en', 'Workspace'],
  ['de', 'Arbeitsbereich'],
  ['ro', 'Spațiu de lucru'],
] as const) {
  test(`Modbus live grid and write dialog are keyboard usable in ${language}`, async ({
    page,
  }) => {
    await page.setViewportSize({ width: 320, height: 640 });
    await page.goto(`/e2e/modbus.fixture.html?view=live&connected=1&lang=${language}`);
    const selector = page.getByRole('combobox', { name: workspaceLabel });
    await expect(selector.getByRole('option', { name: /Plant/ })).toBeAttached();
    await selector.selectOption('plant');
    const grid = page.locator('.modbus-grid-viewport');
    await expect(grid.getByText('Temperature')).toBeVisible();
    const write = grid.locator('button').last();
    await write.focus();
    await page.keyboard.press('Enter');
    const dialog = page.getByRole('dialog');
    await expect(dialog).toBeVisible();
    await expect(dialog.locator(':focus')).toHaveCount(1);
    await page.keyboard.press('Escape');
    await expect(dialog).toHaveCount(0);
    await expect(write).toBeFocused();
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1),
    ).toBe(true);
  });
}

for (const [width, height, theme] of [
  [800, 480, 'light'],
  [1024, 600, 'dark'],
  [1920, 1080, 'light'],
] as const) {
  test(`Modbus live workspace reflows at ${width}x${height} in ${theme}`, async ({
    page,
  }) => {
    await page.setViewportSize({ width, height });
    await page.goto(`/e2e/modbus.fixture.html?view=live&connected=1&theme=${theme}`);
    await page.getByRole('combobox', { name: 'Workspace' }).selectOption('plant');
    await expect(
      page.locator('.modbus-grid-viewport').getByText('Temperature'),
    ).toBeVisible();
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1),
    ).toBe(true);
  });
}

for (const [language, width, height] of [
  ['en', 320, 640],
  ['de', 800, 480],
  ['ro', 1024, 600],
  ['en', 1920, 1080],
] as const) {
  test(`Modbus saved sessions stay usable offline in ${language} at ${width}x${height}`, async ({ page }) => {
    await page.setViewportSize({ width, height });
    await page.goto(`/e2e/modbus.fixture.html?view=sessions&lang=${language}`);
    await expect(page.getByText('October run')).toBeVisible();
    const open = page.locator('.modbus-sessions .modbus-list button');
    await open.focus();
    await page.keyboard.press('Enter');
    await expect(page.locator('.modbus-grid-viewport').getByText('temperature').first()).toBeVisible();
    await expect(page.locator('.modbus-trend')).toBeVisible();
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
  });
}
