# Appliance Network operations and commissioning

## Routing

Select this page for appliance networking, managed device networks, forwarding,
management access, first-boot commissioning, discovery and recovery. The
implementation paths below are relative to `05_Software/core/`. Start at
`internal/network/appliance.go` for core composition and
`internal/nethelper/engine.go` for privileged application. The shared core
boundary belongs to [Bench Core foundation](bench-core-foundation.md);
browser flows belong to [Network GUI](network-gui.md).

## Purpose and boundaries

Bench Core validates and records Network intent before the privileged helper
changes NetworkManager, forwarding, discovery or recovery state. The pure
planning model has no host effect; the device-network manager applies validated
plans through the helper and reconciles observed state.

## Network gateway observations

Network interface readings expose Gateway4 and Gateway6 from the active NetworkManager IP configurations. HTTP adds gateway4/gateway6 strings (empty when absent); the deterministic simulation catalog includes both families for its wired device. The disposable dual-stack NetworkManager fixture checks these observations and recovery with default gateways present. No routing configuration is edited by this read path.
- `cmd/nilo/appliance_stage_linux_test.go` - the appliance verification stage's
  proof: starts the root helper and an unprivileged appliance-profile core with
  generated TLS material and a built interface directory, then asserts the
  listening record, the authenticated boundary, and that every adapter Bench
  Core reports is one NetworkManager detected in the stage's own namespace.

## Device networks and forwarding

The packages and the seams between them are:

- `internal/network/profile_manager.go`, `tunnel.go`: the saved WireGuard
  inventory and the `TunnelDriver` seam the helper client implements
  (`CreateTunnel`, `DeleteTunnel`, `Tunnels`, `ConnectTunnel`); `Startup` runs
  the startup selection as the system actor and retries from the maintenance
  loop. `appliance.go` composes the helper client (`ApplianceBase`) with the
  profile manager, the settings intent store and the device-network manager;
  `serve` hands one value to the service, the operation manager and the sampler.
- `internal/network/interface_settings.go`, `management.go`, `wifi.go`:
  configuration bound to the adapter's permanent hardware identity, the
  management-path trace (`ValidateManagementChange` with `Change.Origin`), and
  Wi-Fi scan/join with the password staged through `SecretStager.StagePassword`.
- `internal/network/devicenetwork.go`, `forwarding_plan.go`,
  `device_network_manager.go`, `device_network_check.go`: the pure model and
  planner (`ForwardingContext{DeviceLink, TunnelDevice, LocalLink}`), the manager
  over the `Forwarder` and `DeviceNetworkEnvironment` seams, snapshot/restore of
  the saved decisions, computed status, checks and instructions. Simulation and
  appliance both use the manager; the simulation forwards in memory. A candidate
  adapter is treated as the device side while its first definition is validated,
  and saving that disabled definition does not arm or apply forwarding recovery.
- `internal/nethelper/forwarding.go`, `nftables_render.go`,
  `nftables_linux.go`: the durable `ForwardingInventory`, the engine's
  `ApplyForwarding`/`RemoveForwarding`/`AppliedForwarding`/`VerifyForwarding`,
  the one-transaction nft script and the Linux adapter (`nft` at a pinned path,
  `/proc/sys/net/ipv4/ip_forward`). A device-network change is prepared with
  `Record.Forwarding` and no NetworkManager checkpoint. `recovery.go`
  `RecoverOnce` is what `nilo-nethelper recover` and the systemd timer run.
- `internal/nethelper/tunnel_engine.go`, `settings_engine.go`,
  `networkmanager_wireguard_linux.go`, `networkmanager_settings_linux.go`: the
  NetworkManager adapters for tunnels (system-wide checkpoint, `SystemSnapshot`
  digest) and adapter settings (`nilo-if-<mac>` profiles, `ipv6.method=link-local`,
  scan classification, join).
- `internal/activity`: `Service.Query`/`Export`, `Policy`, `Housekeeper`,
  `Monitor` (audit health); `internal/store/sqlite` migrations 008
  (interface settings), 009 (device networks), 010 (activity policy and
  retention watermarks).
- HTTP: `network_operations.go` (settings, Wi-Fi and device-network previews with
  the request origin), `device_networks.go`, `activity.go` (`accessGuard` records
  refusals; exports and policy changes are recorded with the requesting actor).
- Invariants kept: durable record before any privileged mutation; secrets only
  in the helper's root-only store and never in a response, log, audit row,
  database row or export; disabled debug never disables audit; the forward chain
  closes what NILO did not grant only where NILO turned forwarding on.

Verification: the disposable container (`02_Tools/tests/network/check.sh`) runs
the NetworkManager, WireGuard, settings, local forwarding (three namespaces and a
Modbus responder), VPN topology (hub, remote client, two sites) and independent
recovery system tests; the Windows suite covers everything portable.

## Exclusive physical-adapter roles

`internal/network/adapter_roles.go` is an unprivileged, process-local
coordinator over stable adapter IDs. Management and device-network managers
publish durable claim snapshots and perform validation plus persistence/helper
work under one coordinator lock. A manager publishes its persisted claim state
before releasing that lock; failed firewall recovery retains and reports every
physical ID whose live ownership is uncertain. Pending recoverable device
changes persist their pre-change inventory and old/proposed claim union in the
first applying record, before helper recovery is prepared or device state
mutates. Creates also persist a deterministic created ID. Startup can therefore
settle a checkpoint-less create from durable inventory and can fail and restore
another pre-helper attempt without refusing to start. If the helper is prepared
but its checkpoint cannot be recorded, Bench Core restores it with a fresh
bounded context before settling the operation. If that restoration fails or
cannot be observed, the operation remains durably unfinished and owns the
connectivity interlock after restart. Adoption restores that interlock for
every unfinished connectivity state (`applying`, confirmation, rolling back,
or rollback failed) while the helper still owns recovery. Runtime maintenance
checks a matching helper record before the checkpoint-less fallback, including
when the core could not persist its checkpoint ID. Nonterminal helper phases
retain the interlock; an unverified restoration requires a matching restored
phase to clear it. Adopted helper
state retains both IDs until reconciliation settles. A restored helper phase
restores the device store and cached claims; an unverified recovery keeps the
union reserved.
Periodic reconciliation preserves the same fail-closed order as startup:
operation recovery must succeed before management reconciliation, and
management reconciliation must succeed before device forwarding is refreshed.
Simulation rollback also restores through the device manager so its
cached claims match the restored store. Virtual interfaces and
local device-network ingress do not claim a physical role. Unknown roles are
rejected.

Management configuration persists `{adapter_id,last_known_name}` objects in the
existing JSON columns and dual-decodes legacy name arrays. The effective
firewall allowlist and HTTPS destination gate admit only observed
interfaces matching those bindings. An absent physical adapter retains its
role claim, but its last-known name cannot admit a replacement adapter. Distinct
physical bindings may share a last-known name. Name-only updates retain an absent
binding while another adapter uses that name; an unambiguous selection permits
its removal. Inspection receives change parameters without process-local recovery
correlation, while apply retains that correlation. The
typed firewall request supports an empty effective list to deny new management
connections when all saved interfaces disappear; administrator configuration
still requires a selection. Startup attaches the
device store without forwarding, adopts and reconciles unfinished device
recovery, then resolves management bindings. It gives persisted device intent
precedence only when another usable management route survives, applies the safe
firewall, and then reconciles forwarding and starts serving.
All-conflicting state fails before the listener or forwarding opens. HTTP maps
typed collisions to the secret-free 409
`network_interface_role_conflict` envelope. Management transition diagnostics
persist a fixed outcome code and safe adapter/resource labels alongside the
console detail; legacy plain details still load, while unknown typed outcomes
are not published as browser metadata. The management-status mapper normalizes
every interface's absent IPv4 and IPv6 observations to empty JSON arrays, so a
disconnected or addressless adapter remains valid strict-client inventory.

## Management-eligible interfaces

`network.Interface` carries `Physical`, whether the provider that produced the
reading saw a physical network adapter. The privileged helper sets it from two
kernel facts read together: NetworkManager's permanent hardware address for the
device, and the presence of `/sys/class/net/<name>/device`, the entry the kernel
creates only for a device on a real bus. Both are required, because
NetworkManager reports built-in Ethernet and a container veth through the same
wired device API and a veth carries a current hardware address but no permanent
one. `busDevicePresent` accepts one ordinary path element and refuses an empty
name, `.`, `..`, and any name carrying a path separator or NUL, so an interface
name stays inside the root it is joined to.

`network.EligibleForManagement` is the one rule every provisioning decision
reads: an Ethernet or Wi-Fi device that the provider recorded as physical and
that carries a non-empty canonical adapter identity. The zero value of
`Physical` is false, so a reading from a provider that records no hardware
observation, and a response from an older helper that omits the field, are both
ineligible. `Physical` crosses the helper socket inside
`nethelper.Device.Interface` and takes part in no state comparison: `sameState`
reads named fields only.

The simulated catalog carries both answers - `enp1s0` and `wlan0` are physical,
and a connected container veth `veth9f2a41` and an addressed bridge `br-lab0`
are not - so the exclusions are asserted by the ordinary suite on any host. The
classification reaches no HTTP response: `networkInterfacePayload` in
`internal/httpapi/network.go` is an explicit struct and names no such field.

## Appliance commissioning lifecycle

The appliance records one lifecycle state in a singleton `commissioning` table:
`provisioning` for a clean installation that has not completed commissioning,
`commissioned` otherwise. Migration `013_commissioning.sql` creates the table
and seeds it in the same statement, choosing `commissioned` when the database
already holds an appliance identity row and `provisioning` when it does not.

That discriminator works because `sqlite.Open` applies the migration set and
`app.Open` calls `appliance.Provision` afterwards, so the migration always runs
before the first identity row is written. A database carried forward from an
earlier release therefore takes `commissioned` from the identity it already
holds, which covers the interface-managed and the legacy single `--bind`
deployment without detecting either separately.

`appliance.CommissioningState` owns the vocabulary and refuses anything but the
two values, including the empty one; there is no zero-value default, because a
guessed `commissioned` would skip first boot on an appliance that needs it and a
guessed `provisioning` would widen management exposure on one that does not.
`app.Open` loads the state immediately after provisioning the identity and fails
the open when the recorded value is absent or unrecognized, exposing it through
`App.Commissioning()`. Nothing writes the state, and the table carries no
timestamp: `appliance_identity.commissioned_at` is the identity creation time and
means something else.

## First-boot management serving

`ManagementAccessService` has a provisioning mode. While the appliance's
recorded lifecycle state is `provisioning`, the allowlist is derived from every
interface `EligibleForManagement` accepts rather than from an administrator's
choice: `NewManagementAccessService` derives it instead of requiring a console
option or a saved record, and `refresh` re-derives on every reconcile, so an
adapter connected during first boot begins carrying management access within one
sampling interval and one removed stops. Each derived change travels the same
path an administrator's would - helper firewall, then persistence, then the
socket allowlist - so it inherits the same ordering and failure handling. The
derived allowlist is persisted, so a restart resumes exactly what was serving.

With no eligible interface present the service persists nothing, installs no
firewall rule and admits nothing, and the first adapter to appear is applied by
the ordinary reconcile. That is deliberate: a configuration with an empty
allowlist cannot be validated or rendered as an nftables interface set, and
admitting everything instead would leave the wildcard listener unguarded.

`Update` and `Confirm` return `ErrManagementAccessDerived` while provisioning,
because the allowlist is not an administrator's to set there. Once the appliance
is commissioned, derivation stops entirely: the configured set alone governs, and
a newly detected eligible adapter is refused at the socket gate.

`serve` reaches provisioning only when no listener option is given. The
first-boot wildcard address is adopted before the database is opened, because
`config.Load` only records `BindAddress`, and `checkProvisioningPermitted`
judges it immediately after the open, so a commissioned appliance started with
no options is refused for its real reason rather than by a syntax rule.
`httpapi.Serve` still validates the wildcard listener against a non-nil gate and
TLS material, so the three enforcement layers are unchanged.

A wired adapter may also take `link-local` as its IPv4 addressing method,
rendered as NetworkManager's own link-local method, so an appliance reached over
a direct cable needs neither a DHCP server nor a guessed address. The standard
probe reaches a different address on each adapter, which is why NILO allocates
none itself. The method is refused with a static address or gateway and on a
wireless adapter. The allowlist needs no special case: it admits every address an
enabled interface reports, so a 169.254 address is already a usable management
path.

## Local-name discovery

The permanent printed serial name answers and carries the version-2
`_nilo._tcp.local` service on exactly the interfaces carrying management
access, during both provisioning and commissioned operation. Its SRV target and
HTTPS URL always use that immutable serial host. Provisioning independently
adds the scoped `nilo.local` recovery alias; commissioning withdraws that alias
without withdrawing the serial service. A temporary recovery overlay can add
`nilo.local` back only on the selected recovery interface, and a friendly name
is another independent address alias rather than a service target.

`network.MDNSAdvertisement` validates the serial host, closed service and TXT
allowlist, independent aliases and interface lists before they leave the
unprivileged process. Invalid labels, loopback or duplicate interfaces, unknown
service fields, URL/name disagreements and configuration separators are
refused.

`network.MDNSAnnouncer` compares desired publication with both its local cache
and `mdns.status` from the helper. An unchanged healthy entry group is left
alone; a helper or Avahi restart, failed publication, interface change, port
change, or lifecycle change causes replacement or withdrawal on the next
reconcile. An empty provisioning interface set withdraws the alias. Discovery
failure is reported and retried while address reachability remains available.

The privileged helper accepts only the typed `mdns.publish` and `mdns.status`
operations. It owns one Avahi D-Bus entry group per interface and purpose, with
permanent serial groups committed before optional alias groups, so an alias
collision or reset cannot withdraw the serial service. Each group publishes the
current addresses of its requested interface and never rewrites daemon
configuration or another publisher's group. When Avahi's effective global host
already equals the serial, the group reuses it; a preserved custom system host
instead receives the explicit no-reverse serial address and the serial remains
the SRV target. A helper with no reachable responder reports the publication
unavailable rather than appearing to succeed.

## Completing commissioning

`POST /api/v1/commissioning/complete` is the administrator-only step that ends
first boot. `ManagementAccessService.Commission` refuses a selection the
appliance could not be reached through: it must validate, every interface must
be one that may carry management access, at least one must be usable now, and
the interface the request itself arrived on may not be given up unless that was
confirmed. A second completion is refused, because the step is one way.

The order is the safety argument. Under the role coordinator the service applies
the firewall, persists the record and stops deriving; the durable lifecycle
state is recorded **last**, because it is the only step whose failure can be
undone completely - the allowlist in force is still a valid provisioning
allowlist, so restoring the previous one returns the appliance exactly to where
it was. `restoreProvisioning` does that on any failure, and the appliance keeps
following the hardware afterwards, which is what proves it really is still
provisioning.

Before any of that, commissioning settles one property of the appliance rather
than of the selection: whether the credential published with the image still
opens it. While a stored verifier under `admin` still matches `admin_password` - however
that identity was created, and whether or not it is currently disabled, because
disabling is reversible - completion is refused with
`ErrPublishedCredentialUnchanged`, answered as `409 published_password_unchanged`
naming the change. The check sits ahead of every read and every apply, so a
refusal has nothing to undo. A service that cannot answer - no checker, or a
checker that failed - refuses with `ErrPublishedCredentialUnknown` and a `503`,
because a gate that cannot answer must not silently pass.

`internal/network` declares the `PublishedCredential` interface and
`internal/appliance` satisfies it over `identity.Service.PasswordVerifierMatches`, so the
network service never learns what an identity is, exactly as `Commissioner`
already works. The interface answers one boolean and never the credential.

A selection the configuration rejects is answered as a conflict, not a service
failure, so an administrator's mistake is never dressed up as an outage.
Reassigning a freed adapter to a device network is the existing device-network
workflow: the role coordinator already refuses an adapter that is still in the
management allowlist, and commissioning onto a smaller allowlist is what frees
one.
