import { BENCH_CORE_API_VERSION, type ParseResult } from './contract';

export const WIREGUARD_PATH = '/api/v1/network/wireguard';

export interface WireGuardProfile {
  id: string;
  name: string;
  interface: string;
  niloOwned: boolean;
  active: boolean;
  autoconnect: boolean;
  lastHandshake: Date | null;
  addresses?: string[];
  dns?: string[];
  peers?: { endpoint: string; allowedIPs: string[] }[];
}

export interface WireGuardStatus {
  readinessOnly?: boolean;
  reported: boolean;
  measuredBy: string;
  networkManagerAvailable: boolean;
  networkManagerVersion: string;
  kernelSupported: boolean;
  helperAvailable: boolean;
  canInstall: boolean;
  detail: string;
  tools: { name: string; installed: boolean; version: string }[];
  profiles: WireGuardProfile[];
}

export type WireGuardChange =
  | { action: 'import'; target: string; profile_text: string }
  | { action: 'activate' | 'deactivate' | 'remove'; target: string }
  | { action: 'rename'; target: string; name: string }
  | { action: 'autostart'; target: string; enabled: boolean }
  | { action: 'install' };

function object(value: unknown): Record<string, unknown> {
  if (value === null || typeof value !== 'object' || Array.isArray(value))
    throw new Error('expected object');
  return value as Record<string, unknown>;
}
function text(body: Record<string, unknown>, key: string): string {
  if (typeof body[key] !== 'string') throw new Error(`invalid ${key}`);
  return body[key];
}
function bool(body: Record<string, unknown>, key: string): boolean {
  if (typeof body[key] !== 'boolean') throw new Error(`invalid ${key}`);
  return body[key];
}

function strings(value: unknown): string[] {
  if (value == null) return [];
  if (
    !Array.isArray(value) ||
    value.length > 256 ||
    !value.every((item) => typeof item === 'string' && item.length <= 1024)
  )
    throw new Error('invalid profile list');
  return value;
}

/** Only retain public connection metadata, never key material. */
function profileDetails(value: unknown) {
  if (value == null) return { addresses: [], dns: [], peers: [] };
  const profile = object(value);
  const peers = profile.peers ?? [];
  if (!Array.isArray(peers) || peers.length > 64) throw new Error('invalid peers');
  return {
    addresses: strings(profile.addresses),
    dns: strings(profile.dns),
    peers: peers.map((item) => {
      const peer = object(item);
      return {
        endpoint: text(peer, 'endpoint'),
        allowedIPs: strings(peer.allowed_ips),
      };
    }),
  };
}

/** Validate every field the interface displays and keep inventories bounded. */
export function parseWireGuardStatus(input: unknown): ParseResult<WireGuardStatus> {
  try {
    const body = object(input);
    if (body.api_version !== BENCH_CORE_API_VERSION)
      throw new Error('unsupported API version');
    if (
      !Array.isArray(body.tools) ||
      body.tools.length > 8 ||
      !Array.isArray(body.profiles) ||
      body.profiles.length > 64
    )
      throw new Error('invalid inventory');
    const tools = body.tools.map((item) => {
      const row = object(item);
      return {
        name: text(row, 'name'),
        installed: bool(row, 'installed'),
        version: text(row, 'version'),
      };
    });
    const profiles = body.profiles.map((item) => {
      const row = object(item);
      const handshake =
        row.last_handshake == null ? null : new Date(text(row, 'last_handshake'));
      if (handshake !== null && !Number.isFinite(handshake.getTime()))
        throw new Error('invalid handshake');
      return {
        id: text(row, 'id'),
        name: text(row, 'name'),
        interface: text(row, 'interface'),
        niloOwned: bool(row, 'nilo_owned'),
        active: bool(row, 'active'),
        autoconnect: bool(row, 'autoconnect'),
        lastHandshake: handshake,
        ...profileDetails(row.profile),
      };
    });
    return {
      ok: true,
      value: {
        readinessOnly:
          body.readiness_only === undefined ? false : bool(body, 'readiness_only'),
        reported: bool(body, 'reported'),
        measuredBy: text(body, 'measured_by'),
        networkManagerAvailable: bool(body, 'networkmanager_available'),
        networkManagerVersion: text(body, 'networkmanager_version'),
        kernelSupported: bool(body, 'kernel_supported'),
        helperAvailable: bool(body, 'helper_available'),
        canInstall: bool(body, 'can_install'),
        detail: text(body, 'detail'),
        tools,
        profiles,
      },
    };
  } catch {
    return { ok: false, violation: 'invalid WireGuard status response' };
  }
}
