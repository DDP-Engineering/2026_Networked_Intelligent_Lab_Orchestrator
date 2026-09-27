import { test, expect } from '@playwright/test';
for (const [width, height] of [
  [320, 640],
  [800, 480],
  [1024, 600],
  [1920, 1080],
]) {
  test(`serial workspace reflows at ${width}x${height}`, async ({ page }) => {
    await page.setViewportSize({ width, height });
    await page.route('**/api/v1/serial-devices/**', async (route) => {
      const path = new URL(route.request().url()).pathname;
      const body = path.endsWith('/workspace')
        ? {
            device_id: 'serial-1',
            connected: false,
            logging: false,
            rx_bytes: 0,
            tx_bytes: 0,
            can_control: false,
          }
        : path.endsWith('/logs')
          ? { logs: [], total: 0, offset: 0 }
          : {
              revision: 1,
              limits: { files: 100, total_bytes: 2147483648, file_bytes: 104857600 },
              usage_bytes: 0,
            };
      await route.fulfill({ json: body });
    });
    await page.goto('/e2e/serial.fixture.html');
    await expect(
      page.getByRole('heading', { name: 'USB console · RealTime View' }),
    ).toBeVisible();
    if (width >= 1024) {
      const actions = await page.locator('.page-head .workspace-actions').boundingBox();
      const head = await page.locator('.page-head').boundingBox();
      expect(actions!.x + actions!.width).toBeGreaterThan(head!.x + head!.width - 30);
    }
    await page.getByText('Port settings', { exact: true }).focus();
    await page.keyboard.press('Enter');
    await expect(page.getByLabel('Baud rate')).toBeVisible();
    await expect(page.getByLabel('Baud rate')).toHaveValue('115200');
    expect(
      await page
        .locator('.page-body')
        .evaluate((node) => node.scrollWidth <= node.clientWidth + 1),
    ).toBe(true);
    expect(
      await page
        .locator('.page-head')
        .evaluate((node) => node.scrollWidth <= node.clientWidth + 1),
    ).toBe(true);
    const send = page.getByRole('button', { name: 'Send', exact: true });
    const box = await send.boundingBox();
    expect(box!.height).toBeGreaterThanOrEqual(44);
    const findBox = await page.getByRole('searchbox').boundingBox();
    expect(findBox!.height).toBeGreaterThanOrEqual(44);
    await page.getByRole('button', { name: 'Fill workspace' }).click();
    await expect(page.getByLabel('Baud rate')).not.toBeVisible();
    if (width >= 800) {
      const filledSend = await send.boundingBox();
      expect(filledSend!.y + filledSend!.height).toBeLessThanOrEqual(height);
      expect(
        await page.locator('.serial-terminal').evaluate((node) => node.clientHeight),
      ).toBeGreaterThanOrEqual(96);
      const terminalBox = await page.locator('.serial-terminal').boundingBox();
      expect(terminalBox!.height).toBeGreaterThan(height - terminalBox!.y - 160);
    }
    await page.getByRole('button', { name: 'Full screen', exact: true }).click();
    await page.getByRole('button', { name: 'Exit full screen' }).click();
    await expect(
      page.getByRole('button', { name: 'Full screen', exact: true }),
    ).toBeVisible();
    const originalTerminal = await page.locator('.serial-terminal').boundingBox();
    await page.goto('/e2e/serial.fixture.html?notice=1');
    await page.getByRole('button', { name: 'Fill workspace' }).click();
    const withNotice = await page.locator('.serial-terminal').boundingBox();
    expect(withNotice!.height).toBeCloseTo(originalTerminal!.height, 0);
    const notice = page.getByRole('region', { name: 'Notifications' });
    const noticeBox = await notice.boundingBox();
    const headerBox = await page.locator('.page-head').boundingBox();
    expect(noticeBox!.y).toBeGreaterThanOrEqual(headerBox!.y + headerBox!.height);
    expect(noticeBox!.x).toBeGreaterThanOrEqual(0);
    expect(noticeBox!.x + noticeBox!.width).toBeLessThanOrEqual(width);
    const closeBox = await notice
      .getByRole('button', { name: 'Dismiss notification' })
      .boundingBox();
    expect(closeBox!.width).toBeGreaterThanOrEqual(44);
    expect(closeBox!.height).toBeGreaterThanOrEqual(44);
    expect(closeBox!.x).toBeLessThan(noticeBox!.x + 16);
    await page.screenshot({ path: test.info().outputPath('notification.png') });
    await page.getByRole('button', { name: 'Full screen', exact: true }).click();
    await expect(page.locator(':fullscreen .notification-stack')).toBeVisible();
    await notice.getByRole('button', { name: 'Dismiss notification' }).click();
    await page.getByRole('button', { name: 'Exit full screen' }).click();
    await expect(page.getByText('Test notification')).toHaveCount(0);
    await page.goto('/e2e/serial.fixture.html?view=sessions');
    await expect(page.getByText('No saved sessions.')).toBeVisible();
    await expect(
      page.getByRole('heading', { name: 'Storage', exact: true }),
    ).toBeVisible();
    await expect(page.getByRole('button', { name: 'Next', exact: true })).toHaveCount(
      0,
    );
    expect(
      await page
        .locator('.page-body')
        .evaluate((node) => node.scrollWidth <= node.clientWidth + 1),
    ).toBe(true);
  });
}
