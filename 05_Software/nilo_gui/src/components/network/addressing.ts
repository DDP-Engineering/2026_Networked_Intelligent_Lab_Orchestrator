import type { NetworkInterface } from '../../api/contract';

/**
 * Small addressing facts the Network page derives for presentation only. The
 * appliance validates every value it is sent; nothing here decides policy.
 */

const IPV4 = /^(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})$/;

/** Whether the text is a dotted IPv4 address. */
export function isAddress(text: string): boolean {
  const match = IPV4.exec(text);
  return match !== null && match.slice(1).every((octet) => Number(octet) <= 255);
}

/** Whether the text is an IPv4 address with a prefix length, such as 192.168.0.1/24. */
export function isPrefix(text: string): boolean {
  const [address, bits, ...rest] = text.split('/');
  return (
    rest.length === 0 &&
    address !== undefined &&
    bits !== undefined &&
    isAddress(address) &&
    /^\d{1,2}$/.test(bits) &&
    Number(bits) <= 32
  );
}

/** The subnet a prefix belongs to, for the review; the text itself if unreadable. */
export function subnetOf(prefix: string): string {
  if (!isPrefix(prefix)) return prefix;
  const [address, bitsText] = prefix.split('/');
  const bits = Number(bitsText);
  const value = address
    .split('.')
    .map(Number)
    .reduce((total, octet) => total * 256 + octet, 0);
  const mask = bits === 0 ? 0 : (0xffffffff << (32 - bits)) >>> 0;
  const network = (value & mask) >>> 0;
  const octets = [24, 16, 8, 0].map((shift) => (network >>> shift) & 255);
  return `${octets.join('.')}/${bits}`;
}

/**
 * The connected uplink: a wired or wireless adapter that carries a gateway,
 * or failing that one that carries an address.
 */
export function uplinkOf(
  interfaces: readonly NetworkInterface[],
): NetworkInterface | undefined {
  const connection = (entry: NetworkInterface) =>
    (entry.kind === 'ethernet' || entry.kind === 'wifi') && entry.state === 'connected';
  return (
    interfaces.find(
      (entry) => connection(entry) && (entry.gateway4 || entry.gateway6),
    ) ?? interfaces.find((entry) => connection(entry) && entry.ipv4.length > 0)
  );
}
