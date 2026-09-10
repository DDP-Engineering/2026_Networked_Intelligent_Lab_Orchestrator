import { BENCH_CORE_API_VERSION, type ParseResult } from './contract';

export const MANAGEMENT_ACCESS_PATH = '/api/v1/network/management-access';

export type ManagementAccessState = 'pending' | 'applied' | 'failed' | 'recovered';

export interface ManagementAccessInterface {
  name: string;
  kind: 'ethernet' | 'wifi' | 'wireguard' | 'loopback' | 'other';
  state: 'connected' | 'disconnected' | 'unavailable';
  ipv4: string[];
  ipv6: string[];
  enabled: boolean;
  current: boolean;
  usable: boolean;
}

export interface ManagementAccessURL {
  url: string;
  kind: 'local' | 'vpn';
}

export interface ManagementAccessStatus {
  state: ManagementAccessState;
  port: number;
  interfaces: ManagementAccessInterface[];
  configuredInterfaces: string[];
  urls: ManagementAccessURL[];
  tlsNames: string[];
  tlsMissing: string[];
  confirmBy: Date | null;
  detail: string;
  currentInterface: string;
}

function strings(value: unknown): string[] | null {
  if (!Array.isArray(value) || !value.every((entry) => typeof entry === 'string')) {
    return null;
  }
  return value;
}

export function parseManagementAccess(
  value: unknown,
): ParseResult<ManagementAccessStatus> {
  if (value === null || typeof value !== 'object' || Array.isArray(value)) {
    return { ok: false, violation: 'management access is not a JSON object' };
  }
  const body = value as Record<string, unknown>;
  if (body.api_version !== BENCH_CORE_API_VERSION) {
    return {
      ok: false,
      violation: 'management access declares an unsupported API version',
    };
  }
  if (
    !['pending', 'applied', 'failed', 'recovered'].includes(String(body.state)) ||
    typeof body.port !== 'number' ||
    typeof body.detail !== 'string' ||
    typeof body.current_interface !== 'string'
  ) {
    return { ok: false, violation: 'management access has invalid state metadata' };
  }
  const configuredInterfaces = strings(body.configured_interfaces);
  const tlsNames = strings(body.tls_names);
  const tlsMissing = strings(body.tls_missing);
  if (configuredInterfaces === null || tlsNames === null || tlsMissing === null) {
    return {
      ok: false,
      violation: 'management access has an invalid string inventory',
    };
  }
  if (!Array.isArray(body.interfaces) || !Array.isArray(body.urls)) {
    return {
      ok: false,
      violation: 'management access has no interface or URL inventory',
    };
  }
  const interfaces: ManagementAccessInterface[] = [];
  for (const raw of body.interfaces) {
    if (raw === null || typeof raw !== 'object' || Array.isArray(raw)) {
      return { ok: false, violation: 'management access lists an invalid interface' };
    }
    const entry = raw as Record<string, unknown>;
    const ipv4 = strings(entry.ipv4);
    const ipv6 = strings(entry.ipv6);
    if (
      typeof entry.name !== 'string' ||
      !['ethernet', 'wifi', 'wireguard', 'loopback', 'other'].includes(
        String(entry.kind),
      ) ||
      !['connected', 'disconnected', 'unavailable'].includes(String(entry.state)) ||
      ipv4 === null ||
      ipv6 === null ||
      typeof entry.enabled !== 'boolean' ||
      typeof entry.current !== 'boolean' ||
      typeof entry.usable !== 'boolean'
    ) {
      return { ok: false, violation: 'management access lists an invalid interface' };
    }
    interfaces.push({
      name: entry.name,
      kind: entry.kind as ManagementAccessInterface['kind'],
      state: entry.state as ManagementAccessInterface['state'],
      ipv4,
      ipv6,
      enabled: entry.enabled,
      current: entry.current,
      usable: entry.usable,
    });
  }
  const urls: ManagementAccessURL[] = [];
  for (const raw of body.urls) {
    if (raw === null || typeof raw !== 'object' || Array.isArray(raw)) {
      return { ok: false, violation: 'management access lists an invalid URL' };
    }
    const entry = raw as Record<string, unknown>;
    if (
      typeof entry.url !== 'string' ||
      !['local', 'vpn'].includes(String(entry.kind))
    ) {
      return { ok: false, violation: 'management access lists an invalid URL' };
    }
    urls.push({ url: entry.url, kind: entry.kind as ManagementAccessURL['kind'] });
  }
  let confirmBy: Date | null = null;
  if (body.confirm_by !== null && body.confirm_by !== undefined) {
    confirmBy = new Date(String(body.confirm_by));
    if (Number.isNaN(confirmBy.getTime())) {
      return {
        ok: false,
        violation: 'management access has an invalid confirmation deadline',
      };
    }
  }
  return {
    ok: true,
    value: {
      state: body.state as ManagementAccessState,
      port: body.port,
      interfaces,
      configuredInterfaces,
      urls,
      tlsNames,
      tlsMissing,
      confirmBy,
      detail: body.detail,
      currentInterface: body.current_interface,
    },
  };
}
