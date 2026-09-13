import { defineConfig, devices } from '@playwright/test';

const browserExecutable = process.env.NILO_BROWSER_EXECUTABLE?.trim();

/**
 * The integrated browser test: a real browser against a real Bench Core on the
 * appliance profile, serving the built interface from its own origin beside the
 * real privileged helper and a real NetworkManager. It runs inside the
 * disposable container created by `02_Tools/tests/appliance/check.sh`. The
 * appliance is started by the suite itself, in `e2e/topology.ts`, because the
 * suite has to stop Bench Core mid-run to observe the unreachable screen and
 * has to prove it released only what it started.
 *
 * Nothing is retained. Trace, video and screenshot are off on every outcome:
 * the suite types a password into the login form, and any one of those
 * artifacts would record it. Stage output and the failing Playwright assertion
 * are the retained diagnostics.
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

  // The first run of the day compiles Bench Core inside the stage container.
  timeout: 120_000,
  expect: { timeout: 15_000 },

  use: {
    ...devices['Desktop Chrome'],
    headless: true,
    // The listener's certificate is generated for this run and trusted by
    // nobody, so the browser is told to accept it explicitly rather than the
    // appliance being served in plaintext.
    ignoreHTTPSErrors: true,
    // The runner is root inside its own disposable container, where Chromium's
    // sandbox cannot start. The appliance it drives still runs unprivileged.
    launchOptions: {
      chromiumSandbox: false,
      ...(browserExecutable === undefined || browserExecutable === ''
        ? {}
        : { executablePath: browserExecutable }),
    },
    trace: 'off',
    video: 'off',
    screenshot: 'off',
  },
});
