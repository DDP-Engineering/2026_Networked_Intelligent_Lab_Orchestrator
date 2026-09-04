import { BENCH_CORE_API_VERSION, type ParseResult } from './contract';

/**
 * Adapter configuration and Wi-Fi.
 *
 * A configuration is bound to the adapter's hardware identity and is a
 * decision: it is reported beside what the adapter actually carries, never
 * in place of it. A wireless password travels once, in the join request, and
 * is never read back.
 */

export interface InterfaceSettingsDraft {
  method: 'static' | 'automatic';
  address: string;
  gateway: string;
  dns: string[];
  dns_automatic: boolean;
  management_protected: boolean;
  never_default: boolean;
}

export interface WiFiJoinDraft {
  ssid: string;
  security: string;
  password: string;
  connect_at_startup: boolean;
}

export interface WiFiNetwork {
  ssid: string;
  security: string;
  strength: number;
  supported: boolean;
}

export interface WiFiScan {
  interfaceId: string;
  supported: boolean;
  observedAt: Date;
  networks: WiFiNetwork[];
}

export function interfaceSettingsPreviewPath(interfaceId: string): string {
  return `/api/v1/network/interfaces/${encodeURIComponent(interfaceId)}/settings/preview`;
}

export function wifiScanPath(interfaceId: string): string {
  return `/api/v1/network/interfaces/${encodeURIComponent(interfaceId)}/wifi`;
}

export function wifiJoinPreviewPath(interfaceId: string): string {
  return `${wifiScanPath(interfaceId)}/preview`;
}

export function parseWiFiScanBody(value: unknown): ParseResult<WiFiScan> {
  if (value === null || typeof value !== 'object' || Array.isArray(value)) {
    return { ok: false, violation: 'the Wi-Fi scan is not a JSON object' };
  }
  const body = value as Record<string, unknown>;
  if (body.api_version !== BENCH_CORE_API_VERSION) {
    return {
      ok: false,
      violation: 'the Wi-Fi scan declares an unsupported API version',
    };
  }
  const observedAt = new Date(String(body.observed_at ?? ''));
  if (Number.isNaN(observedAt.getTime())) {
    return { ok: false, violation: 'the Wi-Fi scan has no observation time' };
  }
  const raw = Array.isArray(body.networks) ? body.networks : [];
  const networks: WiFiNetwork[] = [];
  for (const item of raw) {
    if (item === null || typeof item !== 'object') {
      return { ok: false, violation: 'the Wi-Fi scan lists a bad network' };
    }
    const entry = item as Record<string, unknown>;
    if (typeof entry.ssid !== 'string' || typeof entry.security !== 'string') {
      return { ok: false, violation: 'the Wi-Fi scan lists a bad network' };
    }
    networks.push({
      ssid: entry.ssid,
      security: entry.security,
      strength: typeof entry.strength === 'number' ? entry.strength : 0,
      supported: entry.supported === true,
    });
  }
  return {
    ok: true,
    value: {
      interfaceId: typeof body.interface_id === 'string' ? body.interface_id : '',
      supported: body.supported === true,
      observedAt,
      networks,
    },
  };
}
