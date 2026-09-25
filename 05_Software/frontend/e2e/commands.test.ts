import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { describe, expect, it } from 'vitest';

/**
 * The integrated browser test is only used if it has one obvious acceptance
 * entry point. It must also leave nothing behind: a trace or video would record
 * values typed into credential fields.
 */

const guiRoot = process.cwd();
const repositoryRoot = resolve(guiRoot, '..', '..');

const manifest = JSON.parse(readFileSync(resolve(guiRoot, 'package.json'), 'utf8')) as {
  scripts: Record<string, string>;
  devDependencies: Record<string, string>;
};
const playwrightConfig = readFileSync(resolve(guiRoot, 'playwright.config.ts'), 'utf8');

describe('the integrated browser test commands', () => {
  it('offers one headless command and one headed variant of the same suite', () => {
    expect(manifest.scripts['test:e2e']).toBe('playwright test');
    expect(manifest.scripts['test:e2e:headed']).toBe('playwright test --headed');
  });

  it('leaves the existing unit, component, quality and build commands alone', () => {
    expect(manifest.scripts['test:ci']).toBe('vitest run');
    expect(manifest.scripts['test']).toBe('vitest');
    expect(manifest.scripts['build']).toBe('tsc -b && vite build');
    expect(manifest.scripts['dev:mock']).toBe('vite --host 127.0.0.1 --mode mock');
  });

  it('pins the browser test package to one exact version', () => {
    expect(manifest.devDependencies['@playwright/test']).toMatch(/^\d+\.\d+\.\d+$/);
  });
});

describe('the documentation and the gate', () => {
  it('name the headless command, not the headed one', () => {
    const readme = readFileSync(resolve(guiRoot, 'README.md'), 'utf8');
    const flow = readFileSync(
      resolve(repositoryRoot, '06_SoftwareDocu', 'Development_and_Simulation_Flow.md'),
      'utf8',
    );
    const router = readFileSync(
      resolve(repositoryRoot, '02_Tools', 'scripts', 'aegis_test.py'),
      'utf8',
    );

    expect(readme).toContain('02_Tools/tests/appliance/check.sh');
    expect(flow).toContain('02_Tools/tests/appliance/check.sh');
    // The gate no longer runs the browser suite itself: it needs an appliance,
    // so the router reaches the stage that can start one.
    expect(router).toContain('_run_appliance_stage');
    expect(router).toContain('02_Tools/tests/appliance/check.sh');
    expect(router).not.toContain('test:e2e:headed');
  });

  it('keeps every browser assertion on the appliance response path', () => {
    const field = readFileSync(
      resolve(
        repositoryRoot,
        '05_Software',
        'frontend',
        'e2e',
        'field-display.spec.ts',
      ),
      'utf8',
    );

    expect(field).not.toContain("page.route('**/api/v1/network/statistics?*'");
  });
});

describe('what a browser run is allowed to leave behind', () => {
  it('records no trace, no video and no screenshot on any outcome', () => {
    expect(playwrightConfig).toMatch(/trace:\s*'off'/);
    expect(playwrightConfig).toMatch(/video:\s*'off'/);
    expect(playwrightConfig).toMatch(/screenshot:\s*'off'/);
  });

  it('runs headless, serially and without retries by default', () => {
    expect(playwrightConfig).toMatch(/workers:\s*1/);
    expect(playwrightConfig).toMatch(/retries:\s*0/);
    expect(playwrightConfig).toMatch(/headless:\s*true/);
  });

  it('keeps the browser suite out of the component runner and vice versa', () => {
    const viteConfig = readFileSync(resolve(guiRoot, 'vite.config.ts'), 'utf8');

    expect(playwrightConfig).toMatch(/testMatch:\s*'\*\*\/\*\.spec\.ts'/);
    expect(viteConfig).toMatch(/include:\s*\[/);
    expect(viteConfig).toContain('*.test.ts');
  });

  it('never lets the bench-core spec type its password unredacted', () => {
    const spec = readFileSync(resolve(guiRoot, 'e2e', 'bench-core.spec.ts'), 'utf8');

    expect(spec).toContain('withRedactedSecret');
    expect(spec).not.toMatch(/\.fill\(\s*[A-Za-z_]*PASSWORD/);
  });

  it('never lets the administration spec type a password unredacted', () => {
    const spec = readFileSync(
      resolve(guiRoot, 'e2e', 'authentication.spec.ts'),
      'utf8',
    );

    expect(spec).toContain('withRedactedSecret');
    expect(spec).not.toMatch(/\.fill\(\s*[A-Za-z_]*PASSWORD/);
  });

  it('bootstraps its administrator through standard input, never a command line', () => {
    // Write access to the data directory is already full control, but a
    // password in argv is readable by every other process on the host.
    const harness = readFileSync(resolve(guiRoot, 'e2e', 'topology.ts'), 'utf8');
    const method = harness.slice(harness.indexOf('async bootstrapAdministrator'));
    const body = method.slice(0, method.indexOf('\n  }'));

    expect(body).toContain('runWithInput');
    expect(body).toMatch(/\$\{password\}\n\$\{password\}\n/);
    expect(body).not.toMatch(/'--password'/);
    expect(body).toContain('withRedactedSecret');
    expect(body).not.toContain('--password');
  });

  it('keeps its generated output directory out of Git', () => {
    const ignored = readFileSync(resolve(repositoryRoot, '.gitignore'), 'utf8');

    expect(ignored).toContain('test-results/');
    expect(ignored).toContain('playwright-report/');
  });
});
