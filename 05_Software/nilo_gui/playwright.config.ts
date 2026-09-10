import { defineConfig, devices } from '@playwright/test';

/**
 * The integrated browser test: a real browser, the real interface development
 * server and a real Bench Core on the simulation profile. On Windows the core
 * runs under WSL2; inside the mandatory Linux build container both processes
 * run directly. The topology is started by the suite itself, in `e2e/topology.ts`,
 * because the suite has to stop the backend mid-run to observe the unreachable
 * screen and has to prove it released only what it started.
 *
 * Nothing is retained. Trace, video and screenshot are off on every outcome:
 * the suite types a bearer token into the login form, and any one of those
 * artifacts would record it. A failure is diagnosed by running the same suite
 * with `test:e2e:headed` and watching it.
 *
 * `*.spec.ts` is this runner; `*.test.ts` is the component runner in
 * `vite.config.ts`. The two never collect each other's files.
 */
export default defineConfig({
  testDir: './e2e',
  testMatch: '**/*.spec.ts',
  outputDir: './test-results',
  reporter: 'list',

  // One browser, one run, in order. The suite shares one backend and one
  // browser session across its tests on purpose, and a retry would silently
  // repeat a scenario whose whole point is the state it was left in.
  fullyParallel: false,
  workers: 1,
  retries: 0,

  // The first run of the day compiles Bench Core inside the distribution.
  timeout: 120_000,
  expect: { timeout: 15_000 },

  use: {
    ...devices['Desktop Chrome'],
    headless: true,
    trace: 'off',
    video: 'off',
    screenshot: 'off',
  },
});
