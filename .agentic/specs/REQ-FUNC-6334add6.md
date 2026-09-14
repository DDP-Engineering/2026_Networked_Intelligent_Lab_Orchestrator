# REQ-FUNC-6334add6 — Advertise commissioned appliances through DNS-SD

## Behavior

A commissioned appliance publishes standard DNS-SD service type
`_nilo._tcp` in domain `local.` on each interface in its persistent effective
management allowlist. Provisioning does not publish this service; it publishes
only the temporary `nilo.local` alias. Temporary commissioned recovery does not
widen the permanent service to the recovery interface.

The service is replaced when its interface indexes, management port, lifecycle
state, stable identity, or configured system hostname changes. An identical
desired publication is a no-op only while the helper proves that its Avahi
entry group still exists. A lifecycle switch withdraws the old process-owned
publication before attempting the replacement, so failure cannot leave the
shared provisioning alias reachable on a commissioned appliance. Failure leaves
the last proven publication state unknown and is retried at the next network
reconciliation without stopping HTTPS.

## Data / interface

The validated publication object contains typed fields only:

- `instance`: exactly `NILO <canonical-appliance-UUID>`;
- `service`: exactly `_nilo._tcp`;
- `domain`: exactly `local`;
- `interfaces`: unique, non-loopback interface names resolved by the helper to
  current positive interface indexes;
- `hostname`: the configured single-label system hostname;
- `port`: the configured HTTPS management port, 1 through 65535;
- `state`: exactly `commissioned`.

The Avahi service uses the configured system FQDN as the SRV target.
Its TXT set is an exact allowlist, UTF-8 and individually within DNS-SD limits:

```text
txtvers=1
id=<canonical-appliance-UUID>
hostname=<configured-system-hostname>
url=https://<configured-system-hostname>.local:<port>/
path=/
state=commissioned
```

No arbitrary TXT keys or raw responder configuration cross the helper socket.
The instance remains stable across hostname/address changes. The configured
system hostname must already be unique on the local link and its `.local` name
must be covered by the installed TLS certificate. If Avahi reports a different
effective hostname after collision handling, NILO refuses that publication and
retries; it never advertises an HTTPS URL for an unconfigured, uncertified
suffixed name. The unique UUID still prevents service-instance collisions, and
no commissioned appliance claims `nilo.local`.

## Examples

For appliance `0199a2d0-1234-7abc-8def-0123456789ab`, configured hostname
`bench-west`, and port `8443`, the normal service instance is
`NILO 0199a2d0-1234-7abc-8def-0123456789ab._nilo._tcp.local.` with SRV target
`bench-west.local.` and URL `https://bench-west.local:8443/`.

If the responder reports the effective host as `bench-west-2.local.`, NILO
leaves the UUID, configured hostname, certificate, and operating-system
hostname unchanged, refuses to publish the mismatched target, and retries after
the hostname collision is resolved administratively.

## Acceptance

- Browsing `_nilo._tcp.local` reveals one stable, unique instance per appliance.
- SRV plus TXT yields a usable HTTPS URL and identifies both stable appliance
  identity and configured system hostname.
- Publication occurs on commissioned management interfaces only and tracks
  their changes.
- Invalid labels, UUIDs, ports, states, interfaces, extra TXT data, and raw
  responder content are rejected before mutation.
- Publication failure is observable and retried; it does not weaken HTTPS.
- A failed provisioning-to-commissioned replacement cannot leave `nilo.local`
  published.
- A collision cannot make discovery advertise a name outside the certificate.

## Resolved design boundary

Avahi is the supported appliance responder. Cross-VLAN unicast DNS-SD and a
client discovery application remain outside this requirement.
