import { describe, expect, it } from 'vitest';
import {
  parseModbusEndpoints,
  parseModbusProfiles,
  parseModbusSettings,
  parseModbusScan,
} from './modbus';

describe('Modbus Core response contracts', () => {
  it('keeps failed scan probes distinct from absent results', () => {
    const scan = {
      scan_id: 'scan-1',
      session_id: 'live',
      plan: {
        session_id: 'live',
        function: 3,
        unit_from: 1,
        unit_to: 1,
        address_from: 10,
        address_to: 11,
        rate_per_second: 2,
        deadline_seconds: 5,
      },
      state: 'completed',
      total: 2,
      completed: 2,
      results: [
        { function: 3, unit: 1, address: 10, outcome: 'acknowledged', values: [42] },
        { function: 3, unit: 1, address: 11, outcome: 'timeout', values: [] },
      ],
    };
    expect(parseModbusScan(scan)).toEqual({ ok: true, value: scan });
    expect(
      parseModbusScan({
        ...scan,
        results: [{ function: 3, unit: 1, address: 11, values: [] }],
      }).ok,
    ).toBe(false);
  });
  it('keeps configured offline TCP endpoints visible and rejects malformed inventory', () => {
    const endpoint = {
      schema_version: 1,
      id: 'boiler',
      revision: 2,
      name: 'Boiler',
      transport: 'tcp',
      resource_id: 'boiler-bus',
      enabled: true,
      presence: 'configured',
      readiness: 'offline',
      network: { host: '192.0.2.9', port: 502, unit_min: 1, unit_max: 4 },
    };
    expect(parseModbusEndpoints({ endpoints: [endpoint] })).toEqual({
      ok: true,
      value: [endpoint],
    });
    expect(parseModbusEndpoints([endpoint]).ok).toBe(false);
    expect(
      parseModbusEndpoints({ endpoints: [{ ...endpoint, revision: '2' }] }).ok,
    ).toBe(false);
  });

  it('checks revisioned profiles and settings', () => {
    expect(
      parseModbusProfiles({
        profiles: [
          {
            schema_version: 1,
            id: 'default',
            revision: 1,
            owner_id: 'admin',
            name: 'Default',
            endpoint_id: 'boiler',
            settings: {
              response_timeout_ms: 1000,
              connect_timeout_ms: 1000,
              inter_request_ms: 0,
              read_retries: 0,
            },
          },
        ],
      }).ok,
    ).toBe(true);
    expect(
      parseModbusSettings({
        revision: 1,
        settings: {
          max_sessions: 4,
          session_lease_seconds: 3600,
          idle_seconds: 300,
          retention_count: 100,
          retention_bytes: 2147483648,
          part_bytes: 104857600,
        },
      }).ok,
    ).toBe(true);
    expect(
      parseModbusSettings({ revision: 1, settings: { max_sessions: '4' } }).ok,
    ).toBe(false);
  });
});
