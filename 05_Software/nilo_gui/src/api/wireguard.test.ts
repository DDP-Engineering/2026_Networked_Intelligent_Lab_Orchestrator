import { describe, expect, it } from 'vitest';
import { parseWireGuardStatus } from './wireguard';

describe('WireGuard contract', () => {
  const body = {
    api_version: 'nilo.http/1',
    reported: true,
    measured_by: 'simulation',
    networkmanager_available: true,
    networkmanager_version: 'simulated',
    kernel_supported: true,
    helper_available: true,
    can_install: false,
    detail: '',
    tools: [{ name: 'wg', installed: true, version: 'simulated' }],
    profiles: [
      {
        id: 'office',
        name: 'Office',
        interface: 'wg-test',
        nilo_owned: true,
        active: true,
        autoconnect: false,
        last_handshake: null,
      },
    ],
  };
  it('keeps activation separate from an unobserved handshake', () => {
    const parsed = parseWireGuardStatus(body);
    expect(parsed.ok).toBe(true);
    if (parsed.ok) {
      expect(parsed.value.profiles[0].active).toBe(true);
      expect(parsed.value.profiles[0].lastHandshake).toBeNull();
    }
  });
  it('reads safe profile addresses, DNS and peer routes without retaining secrets', () => {
    const profile = {
      addresses: ['10.7.0.2/24'],
      dns: ['10.7.0.1'],
      peers: [{ endpoint: 'vpn.example.test:51820', allowed_ips: ['0.0.0.0/0'] }],
      private_key: 'must-not-be-retained',
    };
    const parsed = parseWireGuardStatus({
      ...body,
      profiles: [{ ...body.profiles[0], profile }],
    });
    expect(parsed).toMatchObject({
      ok: true,
      value: {
        profiles: [
          {
            addresses: profile.addresses,
            dns: profile.dns,
            peers: [{ endpoint: 'vpn.example.test:51820', allowedIPs: ['0.0.0.0/0'] }],
          },
        ],
      },
    });
    expect(JSON.stringify(parsed)).not.toContain('must-not-be-retained');
    expect(
      parseWireGuardStatus({
        ...body,
        profiles: [{ ...body.profiles[0], profile: { ...profile, addresses: [7] } }],
      }).ok,
    ).toBe(false);
  });
  it('distinguishes readiness from an observed profile inventory', () => {
    const parsed = parseWireGuardStatus({
      ...body,
      readiness_only: true,
      profiles: [],
    });
    expect(parsed.ok && parsed.value.readinessOnly).toBe(true);
    expect(parseWireGuardStatus({ ...body, readiness_only: 'yes' }).ok).toBe(false);
  });
  it('rejects malformed readiness and unbounded profile inventories', () => {
    expect(parseWireGuardStatus({ ...body, kernel_supported: 'yes' }).ok).toBe(false);
    expect(
      parseWireGuardStatus({ ...body, profiles: Array(65).fill(body.profiles[0]) }).ok,
    ).toBe(false);
    expect(
      parseWireGuardStatus({
        ...body,
        profiles: [{ ...body.profiles[0], last_handshake: 'bad' }],
      }).ok,
    ).toBe(false);
  });
});
