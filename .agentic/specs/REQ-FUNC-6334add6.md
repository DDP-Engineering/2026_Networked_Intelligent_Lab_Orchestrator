# REQ-FUNC-6334add6 - Advertise the permanent appliance service through DNS-SD

## Behavior

An appliance publishes standard DNS-SD service type `_nilo._tcp` in domain
`local.` on each interface in its current effective management set while
provisioning or commissioned, including a temporary commissioned recovery link.
The permanent serial host record and service share one process-owned group per
interface. The provisioning-only `nilo.local` alias and optional friendly alias
use independent groups, so their collision or withdrawal cannot remove serial
discovery.

The service is replaced when its interface indexes, management port, lifecycle
state, stable identity, serial/friendly name or project version changes. An identical
desired publication is a no-op only while the helper proves that its Avahi
entry group still exists. A lifecycle switch withdraws the old process-owned
publication before attempting the replacement, so failure cannot leave the
shared provisioning alias reachable on a commissioned appliance. Failure leaves
the last proven publication state unknown and is retried at the next network
reconciliation without stopping HTTPS or unaffected interface publications.

## Data / interface

The validated publication object contains typed fields only:

- `instance`: exactly `NILO <serial-name>`;
- `service`: exactly `_nilo._tcp`;
- `domain`: exactly `local`;
- `interfaces`: unique, non-loopback interface names resolved by the helper to
  current positive interface indexes;
- `serial`: the immutable validated `nilo-<10-character-id>` label;
- `friendly_name`: absent or one validated friendly label;
- `port`: the configured HTTPS management port, 1 through 65535;
- `state`: exactly `provisioning` or `commissioned`;
- `version`: the canonical project semantic version.

The Avahi service always uses `<serial>.local.` as the SRV target. Its TXT set
is an exact allowlist, UTF-8 and individually within DNS-SD limits:

```text
txtvers=2
id=<canonical-appliance-UUID>
serial=<serial-name>
name=<friendly-name>                 # omitted when absent
url=https://<serial-name>.local:<port>/
path=/
state=provisioning|commissioned
version=<project-version>
```

No arbitrary TXT keys or raw responder configuration cross the helper socket.
The instance remains stable across friendly-name/address/lifecycle changes. The
serial `.local` name is covered before publication. If Avahi reports a different
effective hostname after collision handling, NILO refuses only that link's
publication and retries; it never advertises a suffixed, unrecorded or
uncertified name. UUID remains the client merge key. Only provisioning or
explicit temporary recovery claims `nilo.local`, through its separate group.

## Examples

For appliance `0199a2d0-1234-7abc-8def-0123456789ab`, serial
`nilo-4j7m2q9wxs`, friendly name `bench-west`, and port `8443`, the service is
`NILO nilo-4j7m2q9wxs._nilo._tcp.local.` with SRV target
`nilo-4j7m2q9wxs.local.` and URL `https://nilo-4j7m2q9wxs.local:8443/`.

If the responder reports `nilo-4j7m2q9wxs-2.local.`, NILO leaves the UUID,
serial, certificate and operating-system hostname unchanged, refuses that
link's mismatched target and retries after collision resolution.

## Acceptance

- Browsing `_nilo._tcp.local` reveals one stable, unique instance per appliance.
- SRV plus TXT yields a serial-name HTTPS URL and identifies UUID, public
  serial, optional friendly name, state and version.
- Publication occurs on every effective management interface in both lifecycle
  states and tracks interface/recovery changes.
- Invalid labels, UUIDs, ports, states, interfaces, extra TXT data, and raw
  responder content are rejected before mutation.
- Publication failure is observable and retried; it does not weaken HTTPS.
- Serial/service, recovery alias and friendly alias failures are isolated.
- A collision cannot make discovery advertise a name outside the certificate.

## Resolved design boundary

Avahi is the appliance responder. Cross-VLAN unicast DNS-SD remains outside;
the CLI browses the standard service directly.
