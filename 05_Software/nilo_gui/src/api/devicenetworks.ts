import { BENCH_CORE_API_VERSION, type ParseResult } from './contract';

/**
 * Device networks: the saved decisions about which device-side adapter faces
 * a device, what is exposed, from where, and whether the operator wants it
 * enabled. The wire shape separates intent (`enabled`) from observation
 * (`state`, `applied`), and this module keeps that separation: nothing here
 * infers one from the other.
 */

export const DEVICE_NETWORKS_PATH = '/api/v1/network/device-networks';

export const DEVICE_NETWORK_STATES = [
  'disabled',
  'active',
  'waiting',
  'conflict',
  'recovery_failed',
  'unsupported',
] as const;
export type DeviceNetworkState = (typeof DEVICE_NETWORK_STATES)[number];

export interface DeviceNetworkServices {
  protocols: string[];
  tcpPorts: number[];
  udpPorts: number[];
  preserveSource: boolean;
}

/** One saved device network with its observed state. */
export interface DeviceNetwork {
  id: string;
  name: string;
  location: string;
  deviceInterface: string;
  applianceAddress: string;
  deviceSubnet: string;
  scope: 'host' | 'subnet';
  target: string;
  addressing: 'original' | 'translated';
  exposed: string;
  access: 'local' | 'wireguard' | 'both';
  localInterface: string;
  localSources: string[];
  vpnProfileId: string;
  vpnSources: string[];
  services: DeviceNetworkServices;
  /** The operator's intent. */
  enabled: boolean;
  revision: string;
  /** What the appliance observes now. */
  state: DeviceNetworkState;
  detail: string;
  applied: boolean;
  observedAt: Date;
}

/** What a caller submits for a create or an update. */
export interface DeviceNetworkDraft {
  name: string;
  location: string;
  device_interface: string;
  appliance_address: string;
  scope: 'host' | 'subnet';
  target: string;
  addressing: 'original' | 'translated';
  exposed: string;
  access: 'local' | 'wireguard' | 'both';
  local_interface: string;
  local_sources: string[];
  vpn_profile_id: string;
  vpn_sources: string[];
  services: {
    protocols: string[];
    tcp_ports: number[];
    udp_ports: number[];
    preserve_source: boolean;
  };
}

export type DeviceNetworkChange =
  | { action: 'create'; device_network: DeviceNetworkDraft }
  | { action: 'update'; target: string; device_network: DeviceNetworkDraft }
  | { action: 'enable' | 'disable' | 'delete'; target: string };

export interface DeviceNetworkFact {
  name: string;
  result: string;
  detail: string;
}

export interface DeviceNetworkCheck {
  deviceNetworkId: string;
  observedAt: Date;
  facts: DeviceNetworkFact[];
}

export interface InstructionStep {
  machine: string;
  text: string;
}

export interface AccessInstructions {
  deviceNetworkId: string;
  targetAddress: string;
  steps: InstructionStep[];
  unmetPrerequisites: string[];
}

export function deviceNetworkPath(id: string, action?: string): string {
  const base = `${DEVICE_NETWORKS_PATH}/${encodeURIComponent(id)}`;
  return action === undefined ? base : `${base}/${action}`;
}

function object(value: unknown, where: string): Record<string, unknown> {
  if (value === null || typeof value !== 'object' || Array.isArray(value)) {
    throw new Error(`the ${where} is not a JSON object`);
  }
  return value as Record<string, unknown>;
}
function text(body: Record<string, unknown>, key: string, where: string): string {
  const value = body[key];
  if (value === undefined || value === null) return '';
  if (typeof value !== 'string') throw new Error(`the ${where} has a bad ${key}`);
  return value;
}
function flag(body: Record<string, unknown>, key: string, where: string): boolean {
  if (typeof body[key] !== 'boolean') throw new Error(`the ${where} is missing ${key}`);
  return body[key];
}
function strings(value: unknown, where: string): string[] {
  if (value === undefined || value === null) return [];
  if (!Array.isArray(value) || value.some((item) => typeof item !== 'string')) {
    throw new Error(`the ${where} has a bad list`);
  }
  return value as string[];
}
function numbers(value: unknown, where: string): number[] {
  if (value === undefined || value === null) return [];
  if (!Array.isArray(value) || value.some((item) => typeof item !== 'number')) {
    throw new Error(`the ${where} has a bad number list`);
  }
  return value as number[];
}
function instant(body: Record<string, unknown>, key: string, where: string): Date {
  const parsed = new Date(text(body, key, where));
  if (Number.isNaN(parsed.getTime())) throw new Error(`the ${where} has a bad ${key}`);
  return parsed;
}
function version(body: Record<string, unknown>, where: string): void {
  if (body.api_version !== BENCH_CORE_API_VERSION) {
    throw new Error(`the ${where} declares an unsupported API version`);
  }
}

function readDeviceNetwork(value: unknown, where: string): DeviceNetwork {
  const body = object(value, where);
  const state = text(body, 'state', where);
  if (!DEVICE_NETWORK_STATES.includes(state as DeviceNetworkState)) {
    throw new Error(`the ${where} reports the unknown state ${state}`);
  }
  const services = object(body.services ?? {}, `${where} services`);
  return {
    id: text(body, 'id', where),
    name: text(body, 'name', where),
    location: text(body, 'location', where),
    deviceInterface: text(body, 'device_interface', where),
    applianceAddress: text(body, 'appliance_address', where),
    deviceSubnet: text(body, 'device_subnet', where),
    scope: text(body, 'scope', where) === 'subnet' ? 'subnet' : 'host',
    target: text(body, 'target', where),
    addressing:
      text(body, 'addressing', where) === 'translated' ? 'translated' : 'original',
    exposed: text(body, 'exposed', where),
    access: (['local', 'wireguard', 'both'].includes(text(body, 'access', where))
      ? text(body, 'access', where)
      : 'local') as DeviceNetwork['access'],
    localInterface: text(body, 'local_interface', where),
    localSources: strings(body.local_sources, where),
    vpnProfileId: text(body, 'vpn_profile_id', where),
    vpnSources: strings(body.vpn_sources, where),
    services: {
      protocols: strings(services.protocols, where),
      tcpPorts: numbers(services.tcp_ports, where),
      udpPorts: numbers(services.udp_ports, where),
      preserveSource: services.preserve_source === true,
    },
    enabled: flag(body, 'enabled', where),
    revision: text(body, 'revision', where),
    state: state as DeviceNetworkState,
    detail: text(body, 'detail', where),
    applied: flag(body, 'applied', where),
    observedAt: instant(body, 'observed_at', where),
  };
}

function attempt<T>(read: () => T): ParseResult<T> {
  try {
    return { ok: true, value: read() };
  } catch (error) {
    return {
      ok: false,
      violation: error instanceof Error ? error.message : String(error),
    };
  }
}

export function parseDeviceNetworksBody(value: unknown): ParseResult<DeviceNetwork[]> {
  return attempt(() => {
    const body = object(value, 'device network listing');
    version(body, 'device network listing');
    if (!Array.isArray(body.items))
      throw new Error('the device network listing has no items');
    return body.items.map((item, index) =>
      readDeviceNetwork(item, `device network entry ${index + 1}`),
    );
  });
}

export function parseDeviceNetworkBody(value: unknown): ParseResult<DeviceNetwork> {
  return attempt(() => {
    const body = object(value, 'device network');
    version(body, 'device network');
    return readDeviceNetwork(body, 'device network');
  });
}

export function parseDeviceNetworkCheckBody(
  value: unknown,
): ParseResult<DeviceNetworkCheck> {
  return attempt(() => {
    const body = object(value, 'device network check');
    version(body, 'device network check');
    if (!Array.isArray(body.facts))
      throw new Error('the device network check has no facts');
    return {
      deviceNetworkId: text(body, 'device_network_id', 'device network check'),
      observedAt: instant(body, 'observed_at', 'device network check'),
      facts: body.facts.map((item, index) => {
        const fact = object(item, `check fact ${index + 1}`);
        return {
          name: text(fact, 'name', 'check fact'),
          result: text(fact, 'result', 'check fact'),
          detail: text(fact, 'detail', 'check fact'),
        };
      }),
    };
  });
}

export function parseAccessInstructionsBody(
  value: unknown,
): ParseResult<AccessInstructions> {
  return attempt(() => {
    const body = object(value, 'access instructions');
    version(body, 'access instructions');
    const steps = Array.isArray(body.steps) ? body.steps : [];
    return {
      deviceNetworkId: text(body, 'device_network_id', 'access instructions'),
      targetAddress: text(body, 'target_address', 'access instructions'),
      steps: steps.map((item, index) => {
        const step = object(item, `instruction step ${index + 1}`);
        return {
          machine: text(step, 'machine', 'step'),
          text: text(step, 'text', 'step'),
        };
      }),
      unmetPrerequisites: strings(body.unmet_prerequisites, 'access instructions'),
    };
  });
}
