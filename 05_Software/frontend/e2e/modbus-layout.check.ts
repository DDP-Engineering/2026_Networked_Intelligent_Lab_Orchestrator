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
