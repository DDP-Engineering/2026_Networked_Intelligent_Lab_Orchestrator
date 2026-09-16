# Network operations and development

Use this guide for the current network workflow, Linux test entry point and
appliance setup. Historical phase reports belong in Git history, not this guide.
The default development topology is described in
[Development and Simulation Flow](Development_and_Simulation_Flow.md).

## Runtime and interfaces

The simulation profile uses simulated network state. The Linux appliance profile
uses an unprivileged Bench Core and a root NetworkManager helper. The helper
owns managed connections, WireGuard key storage, device-network forwarding and
recovery. It also owns the dedicated `inet nilo_management` nftables table that
enforces the HTTPS management-interface allowlist before a request reaches
Bench Core. Bench Core sends only validated interface names and a port through
the typed helper protocol; it has no network-administration capability and
cannot submit arbitrary firewall rules. NILO-managed resources are distinct from external connections and
tunnels; do not manage an external tunnel as if NILO created it.

A new appliance starts in first-boot provisioning and is reachable on every
eligible adapter until it is commissioned; see First-boot provisioning below.

### Local discovery and Raspberry Pi recovery

On first start the helper derives and persists the enclosure identity
`nilo-xxxxxxxxxx` from a stable Pi or DMI source, falling back once to protected
random state when neither source is usable. Raw source values never become
public metadata. It replaces the system hostname only when that value exactly
matches a configured image default; a customized hostname is left alone.

During first boot, NILO prepares a volatile IPv4 link-local profile on every
unassigned, role-safe physical Ethernet adapter. Exact ownership is recorded in
the root-only helper inventory `commissioning-profiles.json` by stable adapter
identity, interface, and NetworkManager UUID; reconciliation removes only an
inventoried profile whose complete settings still match. A foreign profile is
never owned merely because its name uses a NILO prefix. The profiles use standard
address conflict detection and never supply DHCP, DNS, a default route,
forwarding, NAT, or router advertisements. Both the permanent serial name and
`nilo.local` follow the interfaces that currently carry commissioning
management access. A saved interface
profile, a device-network/PLC role, or existing usable addressing excludes an
adapter; no physical port is designated as the setup port.

In both lifecycle states the appliance advertises `_nilo._tcp.local` on its
management interfaces. The instance is `NILO <serial-name>`; its version 2 TXT
record carries the UUID, serial, optional friendly name, serial HTTPS URL,
lifecycle state and project version, and its SRV target is always the permanent
serial name. Commissioning withdraws only the shared `nilo.local` recovery
alias. An administrator may add one independent lowercase friendly alias. NILO
activates certificate coverage before publishing it and refuses a collision
without removing the previous alias or permanent serial publication. The UUID
makes observations mergeable across protocols. NILO publishes
process-owned Avahi D-Bus entries; it does not replace
`/etc/avahi/avahi-daemon.conf`.

The optional Raspberry Pi delivery wiring gives the physical button three
actions, all performed on release:

- less than 3 seconds: run the bounded Identify LED pattern and refresh the
  current mDNS and SSDP presence;
- 3 seconds to less than 10 seconds: arm or extend a 10-minute temporary
  direct-connect recovery window;
- 10 seconds or longer: clear the commissioned lifecycle, explicit management
  selection, and temporary recovery state, then continue live in commissioning.

At a recovery-duration button release, the privileged helper durably records the UTC release time
and snapshots which identified adapters already have carrier. If a connected
Ethernet adapter has no stable identity, the snapshot is invalid and recovery
is refused safely. Only a later link-up
on an identified, unassigned physical Ethernet adapter may activate. Core and
helper independently refuse saved/static profiles, management/device/recovery
roles, existing IPv4 or non-link-local IPv6 addresses or gateways, active or available production
profiles, and transitioning or identity-mismatched devices. DHCP detection is
only an additional refusal signal. Candidates are ordered by stable adapter
identity and only the first safe one wins.

An active recovery link gets one helper-owned in-memory NetworkManager IPv4LL
profile, the existing HTTPS destination admission, and an interface-scoped
`nilo.local` alias. The permanent service stays on its commissioned interfaces.
Authentication, authorization, TLS, CSRF protection, and Activity audit remain
unchanged. Expiry removes the alias and admission before deleting the exact
owned profile after verifying its durable UUID, adapter/interface/MAC binding,
volatile status, and non-routing settings. Both core and helper persist the deadline, and the helper plus
its independent recovery timer remove an expired or interrupted link even when
Bench Core is unavailable. On Bench Core restart, non-idle recovery is restored
before another button event is handled; it is not silently re-adopted. Pending
button events are coalesced so reset supersedes recovery and a later recovery
release extends from its actual release time. The highest handled generation is
retained in an idle tombstone, including after network commissioning reset, so
an acknowledgement failure cannot replay the action. A failed post-mutation
audit remains durably pending and is retried without repeating its network
effect. An incomplete restore
retains the role interlock for retry. No
password, token, TLS identity, appliance identity, hostname, audit record,
integration, saved interface setting, PLC assignment, or device network is
changed by the long-press reset.

A GPIO read error invalidates the press in progress. When sampling resumes, the
current level establishes a new baseline and a fresh debounced press is required;
an unreadable interval can never lengthen a short press into network reset.

Reference Raspberry Pi wiring is an active-low momentary button between BCM17
and ground using the internal pull-up, and an active-high LED from BCM27 through
an appropriate resistor. Power the Raspberry Pi down and disconnect its supply
before installing or changing this wiring. LED states are off when idle, 500 ms blinking while
armed, steady while active, 125 ms blinking while reset is pending, and a
two-flash failure pattern. Enable it only through the Pi-specific installer
option:

```sh
sudo sh 02_Tools/scripts/install-nethelper.sh \
  /path/to/nilo-nethelper nilo --raspberry-pi-recovery
```

The installer validates Raspberry Pi hardware and `/dev/gpiochip0`. Omit the
option on mini PCs, ordinary PCs, laptops, simulations, and other Ubuntu hosts;
those systems use their normal console/display and reinstall procedures for
recovery. The helper generates one appliance-specific ECDSA key and certificate
after the permanent identity exists. Bench Core receives the public certificate
and a SHA-256-only signing boundary, never the private key. Active and newly
requested local names are covered before publication; the previous certificate
generation remains signable for a five-minute handshake drain.

The helper also announces SSDP presence independently on each management
interface and serves its safe device description through Bench Core. Removal or
shutdown sends byebye; device, WireGuard, loopback and non-allowlisted links are
silent. NILO-owned automatic DHCP client profiles send the permanent hostname
and NILO vendor identity. Manual, link-local, AP, WireGuard and foreign profiles
are unchanged.

While provisioning, an otherwise unused physical Wi-Fi radio may host a
volatile WPA2 access point named for the permanent appliance. A saved or active
client profile or another protected adapter role excludes the radio. The fixed
`192.168.247.0/24` service starts only when it overlaps no foreign host route;
DHCP, wildcard captive DNS and HTTP redirect bind to the AP address only. The
profile and services are removed at commissioning and reconciled again after a
network commissioning reset. `dnsmasq` and `iproute2` are helper prerequisites.

See [Find your NILO](Find_Your_NILO.md) for client-specific discovery and the
`nilo discover`, `nilo open`, and printable-label commands.

In the GUI, Network contains Overview, Device access, VPN profiles and
Diagnostics. Overview starts with appliance status and continues directly with
management access and the interface inventory. Activity provides authorized audit queries,
exports and retention controls. Use the real simulation backend for integrated
workflows; the browser mock does not reproduce every network lifecycle.

WireGuard profiles can be saved, renamed, activated and removed, with one
managed profile active at a time. Readiness checks report tooling/kernel support;
they do not prove handshake state or remote reachability. Device access records
specify the device-facing adapter/subnet, exposed addresses, allowed sources and
services. Configured intent, locally applied forwarding and remote connectivity
are separate facts. Access instructions identify routes needed on other machines.

## Changes and recovery

Network mutations require review and acknowledgement. Connectivity-changing
operations then require confirmation before their deadline, normally 120
seconds. Acknowledging a proposal is not confirmation that connectivity survived.
Retain an independent console when testing management connectivity changes.

Accepted operations have durable IDs. A browser reconnects to the recorded
operation; a lost response is not proof that a change failed. Inspect the existing
operation before retrying. NetworkManager checkpoints protect connection changes;
device forwarding has a separate durable inventory of NILO-owned nftables state.
Unverified recovery blocks further changes rather than reporting success.

Management access is a separate, persistent change with its own last-known-good
record. The API continues to accept interface names, but present physical
adapters are persisted by stable hardware identity with their last observed
Linux name; DHCP addresses remain observations. A saved physical adapter admits
only its matching observed identity: reusing its old Linux name does not grant
a replacement adapter management access. Absent adapters retain their role
claims. If saved or observed adapters share a name, ordinary saves retain every
absent selection and its checkbox stays checked and disabled. Reconnect the
saved adapters under distinct names to make those selections editable again.
An empty observed allowlist blocks new management connections until
a saved interface returns. A single HTTPS server listens on the configured management port,
then both the socket gate and the helper firewall admit only connections whose
destination belongs to an enabled interface. The listener refreshes the
interface/address mapping while it runs, so address renewal, link loss and link
return do not require another process or a database restart.

An update must retain at least one currently usable path. Removing the path used
by the current browser requires a second explicit action. An accepted change is
`pending` for 90 seconds and must be confirmed through one of the newly enabled
interfaces. NILO restores the last-known-good allowlist after the deadline, on
an explicit restore, or on startup when it finds an interrupted pending change.
The GUI reports `pending`, `applied`, `failed` and `recovered` rather than
turning a lost response into success. Keep console or independent VPN access
available while changing the allowlist.

A physical Ethernet, Wi-Fi or USB Ethernet adapter may be management-facing or
device-facing, never both. Saved device networks claim their device-side adapter
even while disabled or temporarily absent. Pending changes retain both proposed
and rollback claims until they settle. Device-network changes persist the
pre-change inventory and complete claim union in their first applying record,
before helper preparation or mutation. Creates persist their final record ID at
the same boundary. On restart, a checkpoint-less create is confirmed only when
that exact record exists; an earlier attempt is failed, and other pre-helper
attempts restore the saved inventory. If a prepared helper checkpoint cannot be
persisted, it is restored before the operation is settled. Helper-backed
operations otherwise rehydrate and reconcile before management access or
forwarding is composed. A failed or unverified helper recovery remains durably
unfinished, rehydrates its connectivity interlock after restart, and is checked
again by runtime maintenance; only a matching restored helper record releases
it. Runtime maintenance stops on an operation or management reconciliation
error before refreshing device forwarding. The uncertain adapter union remains
reserved. Local-access adapters and NILO-owned
WireGuard interfaces are ingress paths and do not acquire the device role.
Conflicts return HTTP 409 with `network_interface_role_conflict` before any
store, firewall, forwarding, NetworkManager or helper mutation.
The browser disables device-side assignment while management-role inventory is
unknown and localizes fixed outcome codes; backend diagnostic prose remains a
console and audit aid rather than browser content. Role-conflict notices from
preview, acknowledgement and management update survive unchanged polling and
clear only after the owning claim disappears; malformed conflict details use a
fixed localized fallback.

Startup loads both stores, restores unfinished device-network recovery state,
and reconciles roles before it installs device forwarding or opens management
application traffic. If legacy state assigns one
adapter to both roles, the saved device network wins only when another usable
management path remains; NILO removes the conflicting management binding and
reports `recovered`. If no safe management path remains, startup fails closed
and requires local correction. Rename and hotplug maintenance use the same
resolve, reconcile, firewall, then destination-gate ordering.

The helper stores recovery records under `/var/lib/nilo-nethelper`. Its service
and the independent `nilo-nethelper-recovery.timer` reconcile interrupted work.
The timer invokes `nilo-nethelper recover`; it does not accept arbitrary remote
commands. Keep runtime records and WireGuard private keys out of the GUI, source
tree, logs and exported diagnostics.

## CLI examples

Replace IDs, URLs, token paths and addresses with the intended appliance values.
Use `--ca-file PATH` when its certificate authority is not in the trust store.

```text
nilo network status --server URL --token-file TOKEN_FILE
nilo network operation list --server URL --token-file TOKEN_FILE
nilo network operation OPERATION_ID --server URL --token-file TOKEN_FILE
nilo network operation confirm OPERATION_ID --server URL --token-file TOKEN_FILE
nilo network interface settings ID --method static --address 192.168.0.1/24 --never-default --server URL --token-file TOKEN_FILE
nilo network wifi scan ID --server URL --token-file TOKEN_FILE
nilo network vpn profiles rename ID --name "Head office" --server URL --token-file TOKEN_FILE
nilo network device-networks create --file line-3.json --server URL --token-file TOKEN_FILE
nilo network device-networks enable ID --server URL --token-file TOKEN_FILE
nilo network device-networks check ID --tcp-port 502 --server URL --token-file TOKEN_FILE
nilo activity list --category audit --operation OPERATION_ID --server URL --token-file TOKEN_FILE
nilo activity export --format csv --out activity.csv --server URL --token-file TOKEN_FILE
```

To initialize interface-managed appliance access, use repeatable interface
options. This creates one server and one state directory;
it must not be replaced by parallel `nilo serve` processes.

```sh
nilo serve --profile appliance \
  --data-dir /var/lib/nilo \
  --management-interface wlan0 \
  --management-interface nilo0 \
  --management-port 8443 \
  --gui-dir /opt/nilo/gui \
  --nethelper-socket /run/nilo/nethelper.sock
```

After initialization, start the service with `--management-port 8443` and no
`--management-interface` options. NILO loads the saved allowlist and TLS names
from `/var/lib/nilo`; this lets an administrator's later GUI changes survive a
service restart and reboot. Repeating interface options that still match the
saved state is harmless. A different console list is refused unless
`--management-reconfigure` is present; that explicit change is staged as
pending with the previous list retained for automatic recovery. `--bind` and
interface-managed options are mutually exclusive. Existing installations
continue to use their literal `--bind` listener until an administrator
deliberately initializes interface-managed access. A commissioned legacy
private-address listener still publishes the permanent `_nilo._tcp.local`
service on the Ethernet interface owning that literal address; a loopback
listener remains undiscoverable. The helper-backed certificate covers the
permanent serial `.local` discovery target; an explicit file-backed legacy
certificate must carry that name itself.

Interface names must be valid non-loopback Linux interface names. Empty selections
and invalid or duplicate TLS identities are refused. Physical saved selections are unique by adapter
identity and may share a last-known name; the active firewall names remain unique.
A saved interface may be absent at
ordinary startup; initialization and explicit reconfiguration cannot select a
list with no currently usable path. Startup reports the configured interfaces
and their observed addresses.
Do not treat a Wi-Fi DHCP address as configuration.

Mutations prompt for acknowledgement or accept their explicit `--acknowledge`
option. Use the command's help for its complete options. GUI, CLI and exposed MCP
tools share authorization; do not assume every mutating CLI command has an MCP
counterpart. Activity access uses its own permissions.

## Isolated Linux test

With the repository's WSL distribution and Docker prepared, run from Windows:

```powershell
wsl -d Ubuntu-24.04 -- sh -lc 'sh /mnt/f/2026_Networked_Intelligent_Lab_Orchestrator/02_Tools/tests/network/check.sh'
```

Set `NILO_NETWORK_RACE=1` inside the Linux command for the race detector.
The harness creates a disposable NetworkManager environment with a private
system bus/network namespace and mounts the checkout read-only. Privileged
container capabilities apply to that fixture, not the host network. Allow time
for image compilation and real recovery timeouts. Ordinary unit tests do not
replace this opt-in system test.

For the Ubuntu installation-only fixture, set
`NILO_NETWORK_DISTRO=ubuntu24 NILO_INSTALL_ONLY=1` before invoking the harness.
The default fixture is Debian 13. Tests must not install the live helper onto
the Windows/WSL development host or reconfigure its network.

## Prepare a disposable appliance

Use a Linux machine with NetworkManager, an existing non-root NILO service
account, a separate management path and a trusted TLS certificate. Resolve the
bench Pi through [Local Hardware Access](Local_Hardware_Access.md); historical
IP addresses are not deployment configuration.

1. Build the GUI with `npm run build` through the repository tool wrapper in
   `05_Software/nilo_gui`; deploy its `dist` contents to `/opt/nilo/gui`.
2. Run `sh 02_Tools/scripts/build-appliance.sh` using the prepared Linux
   toolchain. Binaries are produced under
   `05_Software/nilo_assistant/dist/linux-<arch>/`; choose `arm64` for the Pi.
3. Install the helper explicitly on the target, substituting the staged binary:
   `sudo sh 02_Tools/scripts/install-nethelper.sh /path/to/nilo-nethelper nilo`.
   It installs the helper service and independent recovery timer. Device-network
   forwarding requires `nftables`; absent tooling is not working forwarding.
4. Give the service account its writable `/var/lib/nilo` data directory and
   access to the helper socket. The helper alone owns the TLS private key under
   `/var/lib/nilo-nethelper`; Bench Core receives only bounded signing service.
   Keep runtime state outside the GUI directory. Trust the issuing CA on clients.
5. Bootstrap the administrator once as the service account:
   `nilo admin bootstrap --data-dir /var/lib/nilo --username administrator`.
   The development-administrator convenience is simulation-only.
6. Start the installed helper once so it derives the printed permanent name and
   generates the appliance-specific TLS key. The key remains under
   `/var/lib/nilo-nethelper` and is not copied into the image, GUI, or Bench Core
   state.
7. Initialize the appliance management listener:

```sh
nilo serve --profile appliance \
  --data-dir /var/lib/nilo \
  --management-interface wlan0 --management-interface nilo0 \
  --management-port 8443 \
  --gui-dir /opt/nilo/gui \
  --nethelper-socket /run/nilo/nethelper.sock
```

8. Configure the long-running service to use the same data directory with
   `--management-port 8443` rather than repeating the bootstrap interface list.
   The persisted selection then remains authoritative across restarts.

The administrator-only management selector on **Network > Overview** lists
detected names, type, link state and current addresses below the compact status.
Enable `wlan0` and `nilo0`; leave `eth0` disabled. The panel identifies the
current browser path, requires explicit confirmation before removing it, and
offers keep/restore actions while a configuration is pending. It presents
`https://ddp-remote.local:8443` for local Wi-Fi and
`https://10.205.148.11:8443` for WireGuard. `.local` discovery works only on the
local network. The numeric Wi-Fi DHCP address is deliberately shown only as
current telemetry, not as a durable URL.

The Overview header owns View activity, safe repurposing help and interface
refresh. Management read progress, failure, state and localized recovery outcome
share one mutually exclusive status line. Interfaces without observed addresses
remain valid inventory entries and use empty address arrays on the HTTP contract.

Both management and device-side selectors retain unavailable adapters in view,
disable only the conflicting role choice, and show the owning role and saved
device-network name where applicable. A management adapter remains valid as a
device network's local ingress. **How to repurpose an adapter** gives the safe
sequence: establish and confirm a replacement management path, remove and
confirm the old management binding, then assign the now-unclaimed adapter as
device-facing. There is no one-click role swap.

For a sigrok-enabled delivery, also follow the bundle instructions in
[Sigrok integration](Sigrok_Integration.md). Verify service startup, authenticated
access and recovery on the target; a successful build is not deployment evidence.

Missing WireGuard tools can be installed through the acknowledged NILO workflow
on supported Debian 13/Ubuntu 24.04 targets. Installation is a bounded helper
operation; it does not request a distribution upgrade, kernel replacement or
reboot. Inspect its operation result and readiness afterwards.

## Appliance power controls

On the live appliance profile, Operator and Administrator browser sessions can
choose **Restart NILO** or **Power off NILO** from the account menu. Each action
has a separate confirmation. Viewer sessions do not see the actions, and bearer
tokens are refused even when their stored scope set contains
`system.power.manage`. Sign out remains available to every browser session.

The browser sends exactly `POST /api/v1/system/power` with either
`{"action":"restart"}` or `{"action":"power_off"}` and the session CSRF
header. Bench Core persists `system.power.requested` before dispatch and returns
202 only when the privileged helper accepts the request. A connection loss is
reported as unconfirmed, not converted into success.

The peer-authenticated helper accepts only the empty `system.restart` and
`system.power_off` envelopes. As root, it calls systemd-logind on the system bus
with `Manager.Reboot(false)` or `Manager.PowerOff(false)`; no request data becomes
a command, argument, executable path or environment value. The simulation and
browser mock have only an unavailable controller.

Restart and power-off checks on the local bench Pi follow
[Local Hardware Access](Local_Hardware_Access.md#27-power-control-checks). The
restart can be observed remotely. The person at the bench owns the power-off and
power-restoration sequence because a powered-off board has no remote recovery
path. These observations are useful development evidence, not repository gates.

## First-boot provisioning and commissioning

A new appliance has nobody at a console to name interfaces, and an installer
cannot know in advance which adapter becomes the permanent management path. The
appliance therefore records one lifecycle state, `provisioning` or
`commissioned`, and behaves differently in each.

### What the state means and where it comes from

The schema migration that creates the state seeds it from what the database
already held when it ran: a database with no appliance identity is a clean
installation and records `provisioning`; one that already had an identity is an
existing installation and records `commissioned`. Migrations run before the
first identity row is written, so this holds for both interface-managed and
legacy `--bind` deployments without either being detected separately. An upgrade
never re-enters first-boot provisioning. Bench Core reads the state on every
open and refuses to start when the recorded value is absent or unrecognized,
rather than guessing in either direction.

### While provisioning

The management allowlist is every eligible physical interface currently
present, not a choice an administrator made. An interface is eligible when the
provider observed a physical adapter of an Ethernet or Wi-Fi kind carrying a
stable adapter identity. The privileged helper records an adapter as physical
only when it reports a permanent hardware address **and** the kernel presents it
on a real bus, which is what keeps a container veth, a bridge, a tun device and
the loopback device out: NetworkManager reports built-in Ethernet and a
container veth through the same wired device API, so neither the device type nor
the interface name separates them.

The allowlist is recomputed on every reconciliation, so an adapter connected
during first boot begins carrying management access within one sampling interval
and one removed stops, with no restart. It is persisted, so a restart resumes
exactly what was serving. With no eligible adapter present the appliance
persists nothing, installs no firewall rule and admits nothing; the first
adapter to appear is applied by the ordinary reconciliation.

Explicit management-access updates are refused while provisioning: the allowlist
is derived there, and the only way to change it is to commission.

`serve` accepts no listener option at all while the appliance is provisioning,
and serves on the first-boot HTTPS port. The same command is refused on an
appliance that is already commissioned, naming `--bind` and `--management-port`.

### Discovery and addressing

The appliance answers one local name on exactly the interfaces currently
carrying management access, published through the privileged helper rather than
by Bench Core. The advertisement carries a host label and an interface list and
no address, because the responder publishes whatever each interface currently
holds - which is what lets one URL keep working across a DHCP renewal. The
publication is renewed only when the set of serving interfaces changes, and a
publication that fails is reported and retried rather than being fatal or
silent: an appliance whose name is not answered is still reachable by address.

The helper manages only responder configuration it wrote itself, recognized by
its own marker, and refuses to replace any other, so an appliance that already
runs a responder is never taken over.

A `.local` name is answered only on the local network the client is attached to.
Addresses shown in the interface come from the network and change; they are not
permanent identifiers.

For a direct cable with no DHCP server, a wired adapter accepts `link-local` as
its IPv4 addressing method, rendered as NetworkManager's own link-local method.
The standard probe reaches a different address on each adapter by construction,
which is why NILO allocates none itself and why a fixed conventional address per
adapter is deliberately not used: two adapters carrying one would create route
and ARP ambiguity. The method is refused with a static address or gateway and on
a wireless adapter. An adapter holding only a link-local address is a usable
management path like any other.

### First contact and the published credential

A freshly imaged appliance is signed into with a credential that is published
rather than secret: user name `admin`, password `admin_password`. It is the same
in every image, it appears here and in the delivery documentation, and anyone
holding an image holds it. It exists so that somebody who has just imaged an
appliance can reach it and commission it without being handed anything and
without a console step.

Two things bound what that costs.

The exposure ends at commissioning, and is made to end there. Completing
commissioning is refused while this password is still in place, with a refusal
naming the change that has to happen first; replace the password and the same
request succeeds. First boot is already the widest the appliance's management
access ever gets, and commissioning is the moment that widening ends, so binding
the two means the weak credential cannot outlive the wide window. An appliance
whose administrator was created some other way is not affected by the rule.

The published password is ordinary in every other respect. It is stored as an
Argon2id verifier like any other, and the replacement is held to the ordinary
password rule: nothing about shipping a known first credential relaxes what any
later password has to satisfy.

**Expect a certificate warning, and expect it as the first step.** Open
`https://nilo.local:8443/` or the printed permanent `.local` URL. The
browser will object to the certificate, and that objection is part of the
procedure rather than a fault:

- No publicly trusted certificate can exist for these identities. `.local` is
  reserved for multicast DNS and no public authority will issue for a reserved
  name or a private address, so the helper-generated certificate is signed by
  nobody the browser already trusts.
- The helper-generated certificate carries every currently advertised local
  name, so the expected objection is to its self-signed issuer rather than a
  name mismatch.
- The image contains no appliance private key. Each helper generates a distinct
  P-256 key after the printed identity is known and exposes only signing.
- Friendly-name changes may rotate the public certificate. The browser may ask
  for trust again; compare the permanent printed name before proceeding.

### Completing commissioning

Commissioning is administrator-only and one way. It refuses a selection the
appliance could not be reached through: the selection must validate, every
interface in it must be one that may carry management access, at least one must
be usable at that moment, and the interface the request arrived on may not be
given up unless that is confirmed explicitly. A second attempt is refused.

The order of the transition is deliberate. Under the role coordinator the
appliance applies the firewall, persists the record and stops deriving, and
records the lifecycle state **last** - because that is the only step whose
failure can be undone completely. If it fails, the allowlist in force is still a
valid provisioning allowlist, so the previous one is restored and the appliance
remains provisioning, still following the hardware. A transition that does not
finish therefore costs nothing.

After commissioning the configured allowlist alone governs. A newly attached
eligible adapter does not join it and is refused at the socket gate; the
allowlist changes only through an ordinary management-access request. Freeing an
adapter for a device network is exactly what commissioning onto a smaller
allowlist does, and the exclusive-role rule then applies: the role coordinator
refuses an adapter that is still in the management allowlist.

### What first boot does not weaken

TLS, authentication, authorization, CSRF, session cookies and same-origin
handling are unchanged. The three enforcement layers are unchanged: the helper's
`inet nilo_management` nftables table, the socket gate that rejects a disallowed
destination before TLS, and the per-request recheck outside the application
handler. The unprivileged process gains no network-administration capability;
the helper accepts a validated interface list, a port, and a validated host
label, never firewall or responder syntax. No response, log or export carries a
Wi-Fi password, a WireGuard key or any other secret.

The appliance listener uses the helper's current public certificate and sends
each handshake signature back through the constrained helper protocol. Friendly
and provisioning names are covered before they are published. The legacy
file-backed pair remains available when both `--tls-cert` and `--tls-key` are
provided, and cannot be combined with the helper-backed identity. Browsers warn
about the signer for the reasons set out under first contact above.

The published first credential does not weaken any of this either. It is one
ordinary administrator identity, subject to the same roles, sessions, throttling
and audit as any other, and it cannot survive commissioning.

## Acceptance still requiring explicit evidence

Do not infer that these scenarios passed from the existence of code or the
removal of old reports. Reconcile existing test/journal evidence before scheduling:

- Core/helper/NetworkManager crashes, reboot during a pending apply, offline
  startup and migration with existing data on a booted appliance.
- Real-radio Wi-Fi join, USB adapter hotplug and power-cycle recovery.
- First-boot Mini PC acceptance: first access through each built-in eligible
  adapter, direct Ethernet access with no DHCP server through the link-local
  fallback, Wi-Fi DHCP renewal and reconnection, USB Ethernet hotplug during and
  after commissioning, safe transition of adapters to device-facing roles, loss
  and recovery of the active management path, persistence across a full reboot,
  and no NILO management exposure on a commissioned device-facing network.
- Dual-path Raspberry Pi acceptance: Wi-Fi DHCP renewal and reconnect,
  `nilo0` returning as `10.205.148.11`, access through both stable URLs,
  refusal on the PLC-facing `eth0`, unchanged PLC communication, and a full
  appliance reboot.
- Stable-role Raspberry Pi early signal: retain DDP Wi-Fi and `nilo0` as
  management paths, use the USB Ethernet adapter only on an isolated device
  segment, exercise accepted/refused assignments, replug and restart, and
  confirm `https://10.205.148.11:8443/` remains reachable.
- PLC register-map validation and vendor download using an authorized tool.
- Real-backend Activity browser coverage, connection selection when an adapter
  has no remembered connection, and usability on the physical seven-inch panel.
- Event coverage for adapter appearance/disappearance/rebinding, handshake
  changes, clock synchronization and service lifecycle.

These are carried-forward verification/work items, not a claim that each is
unimplemented. Detailed historical observations remain in Git and task journals.
