import { defineConfig } from '@playwright/test';
export default defineConfig({
  testDir: './e2e',
  testMatch: 'modbus-layout.check.ts',
  workers: 1,
  retries: 0,
  reporter: 'list',
  use: {
    headless: true,
    baseURL: 'http://127.0.0.1:5179',
    launchOptions: {
      ...(process.env.NILO_BROWSER_EXECUTABLE
        ? { executablePath: process.env.NILO_BROWSER_EXECUTABLE }
        : {}),
    },
  },
  webServer: {
    command: 'npm run dev -- --port 5179 --strictPort',
    url: 'http://127.0.0.1:5179/e2e/modbus.fixture.html',
    reuseExistingServer: false,
  },
});
