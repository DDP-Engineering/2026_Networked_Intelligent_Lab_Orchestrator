import { describe, expect, it } from 'vitest';

import { parseIdentifyResult } from './appliance';

describe('appliance Identify contract', () => {
  it('accepts the supported response with its absolute deadline', () => {
    expect(
      parseIdentifyResult({
        api_version: 'nilo.http/1',
        supported: true,
        active_until: '2026-09-15T12:00:10Z',
      }),
    ).toEqual({
      ok: true,
      value: { supported: true, activeUntil: '2026-09-15T12:00:10Z' },
    });
  });

  it('accepts the typed unsupported response', () => {
    expect(
      parseIdentifyResult({
        api_version: 'nilo.http/1',
        supported: false,
        reason: 'indicator_unavailable',
      }),
    ).toEqual({
      ok: true,
      value: { supported: false, reason: 'indicator_unavailable' },
    });
  });

  it('rejects the superseded duration response', () => {
    expect(
      parseIdentifyResult({
        api_version: 'nilo.http/1',
        status: 'accepted',
        duration_seconds: 10,
      }).ok,
    ).toBe(false);
  });
});
