import { describe, expect, it } from 'vitest';

import {
  BENCH_CORE_API_VERSION,
  BENCH_CORE_PATHS,
  DEVICES_PATH,
  INTEGRATIONS_PATH,
  STATUS_PATH,
  resourceUrl,
} from './contract';

/**
 * A bearer credential is not a CORS-safelisted header, so any request the
 * browser addresses to another origin is preflighted and Bench Core answers no
 * preflight. Every path the contract declares therefore has to stay
 * origin-relative; this is the guard against one absolute URL slipping back in.
 */

describe('Bench Core request paths', () => {
  it('declares exactly the three resources this interface consumes', () => {
    expect(BENCH_CORE_PATHS).toEqual([STATUS_PATH, INTEGRATIONS_PATH, DEVICES_PATH]);
    expect(STATUS_PATH).toBe('/api/v1/status');
    expect(INTEGRATIONS_PATH).toBe('/api/v1/integrations');
    expect(DEVICES_PATH).toBe('/api/v1/devices');
  });

  it.each(BENCH_CORE_PATHS)('addresses %s relative to the serving origin', (path) => {
    expect(path.startsWith('/api/v1/')).toBe(true);
    expect(path).not.toMatch(/^[a-z][a-z0-9+.-]*:/i);
    expect(path.startsWith('//')).toBe(false);
    expect(path).not.toContain('127.0.0.1');
    expect(path).not.toContain('localhost');
  });

  it.each(BENCH_CORE_PATHS)(
    'joins %s onto the empty same-origin base without a host',
    (path) => {
      const url = resourceUrl('', path);

      expect(url).toBe(path);
      expect(url).not.toMatch(/^https?:/);
      expect(url.startsWith('//')).toBe(false);
    },
  );

  it.each(BENCH_CORE_PATHS)(
    'joins %s onto an explicit base without doubling the separator',
    (path) => {
      expect(resourceUrl('http://127.0.0.1:8090/', path)).toBe(
        `http://127.0.0.1:8090${path}`,
      );
    },
  );

  it('supports exactly one wire version', () => {
    expect(BENCH_CORE_API_VERSION).toBe('nilo.http/1');
  });
});
