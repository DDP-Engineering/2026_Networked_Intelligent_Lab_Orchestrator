import { defineConfig } from '@playwright/test';
export default defineConfig({
  testDir: './e2e',
  testMatch: 'serial-layout.check.ts',
  workers: 1,
  retries: 0,
  reporter: 'list',
  use: {
    headless: true,
    baseURL: 'http://127.0.0.1:5178',
    launchOptions: {
      ...(process.env.NILO_BROWSER_EXECUTABLE
        ? { executablePath: process.env.NILO_BROWSER_EXECUTABLE }
        : {}),
    },
  },
  webServer: {
    command: 'npm run dev -- --port 5178 --strictPort',
    url: 'http://127.0.0.1:5178/e2e/serial.fixture.html',
    reuseExistingServer: false,
  },
});
