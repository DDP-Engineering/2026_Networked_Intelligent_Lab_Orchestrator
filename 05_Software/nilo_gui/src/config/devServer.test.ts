import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { describe, expect, it } from 'vitest';

/**
 * The browser is served from the Vite development port while Bench Core
 * listens on its own loopback port. The interface only ever requests
 * origin-relative paths, so the development server has to forward them; this
 * asserts that forwarding is declared, because a missing proxy turns every
 * authenticated request into a preflight Bench Core cannot answer.
 */

const config = readFileSync(resolve(process.cwd(), 'vite.config.ts'), 'utf8');

describe('development server proxy', () => {
  it('takes the forwarding target from the configured Bench Core endpoint', () => {
    expect(config).toMatch(/loadEnv/);
    expect(config).toMatch(/VITE_NILO_API_BASE_URL/);
  });

  it('forwards the API prefix when serving and when previewing a build', () => {
    expect(config).toMatch(/server:\s*{\s*proxy/);
    expect(config).toMatch(/preview:\s*{\s*proxy/);
    expect(config).toMatch(/'\/api'/);
  });
});
