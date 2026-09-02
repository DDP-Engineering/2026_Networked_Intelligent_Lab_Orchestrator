import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { describe, expect, it } from 'vitest';

/**
 * The integrated browser test is only used if it is one obvious command, and
 * it is only debuggable if there is a way to watch the browser. It must also
 * leave nothing behind: a trace or a video would record the value typed into
 * the token field, which is a credential.
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
      resolve(repositoryRoot, '05_Software', 'scripts', 'aegis_test.py'),
      'utf8',
    );

    expect(readme).toContain('run test:e2e');
    expect(flow).toContain('run test:e2e');
    expect(router).toContain('test:e2e');
    expect(router).not.toContain('test:e2e:headed');
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

  it('never lets the browser spec hold the token at all', () => {
    // A failing browser action repeats its argument in the error it throws once
    // the locator has resolved, and the token field always resolves. The spec
    // therefore never touches the value: it hands the field to the harness,
    // which types it in behind a redaction wrapper.
    const spec = readFileSync(resolve(guiRoot, 'e2e', 'bench-core.spec.ts'), 'utf8');

    expect(spec).toContain('enterTokenInto');
    expect(spec).not.toMatch(/\.token\b/);
    expect(spec).not.toMatch(/\.fill\(/);
  });

  it('keeps its generated output directory out of Git', () => {
    const ignored = readFileSync(resolve(repositoryRoot, '.gitignore'), 'utf8');

    expect(ignored).toContain('test-results/');
    expect(ignored).toContain('playwright-report/');
  });
});
