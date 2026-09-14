# Bench Core foundation

## Routing

Select this page for the Go Bench Core under `05_Software/nilo_assistant/`:
the `nilo` command line (`serve`, `admin` in local and remote mode, `network`,
`activity`, `diagnostics`), the authenticated `/api/v1` HTTP routes and their
permissions, identities, sessions, API tokens and their audit events, network
operations, the simulation catalog, and the SQLite repositories. Start at
`cmd/nilo/main.go` for a command, `internal/httpapi/server.go` for a route
(the one permission table), `internal/identity/service.go` for an identity
rule, and `internal/network/operation.go` for a managed change; the focused
tests sit beside each file. The browser interface belongs to *Browser GUI
foundation*; the privileged NetworkManager helper and its system tests are in
`internal/nethelper/` and `02_Tools/tests/network/`, and the appliance
verification stage's proof is `cmd/nilo/appliance_stage_linux_test.go`, run by
`02_Tools/tests/appliance/`; the Python integration packages belong to
*Integration platform*.

## Purpose and boundaries

The Go Bench Core owns validated configuration, immutable operation state, identity and access control, policy and approvals, durable sessions and evidence, application startup reconciliation, and the authenticated HTTP boundary. It exposes read-only Network MCP tools; live interface networking and WireGuard readiness use a separate privileged NetworkManager helper; managed sigrok instrument operations use the bench integration service.

Two authorization decisions live here and stay separate. `identity.Service` decides whether an actor may *request* an action, from its fixed role and the scopes of the API token in use. `policy.Resolver` decides whether an operation may *run* without a human approval, from the integration app's declared operation policy. Neither the Administrator role nor a fully scoped token touches the second decision.

Managed plugin lifecycle, asynchronous instrument jobs and the `plugin`/`bench`
CLI groups route to [Managed bench integrations](bench-integrations.md).

## Public interface

`nilo network` calls the authenticated `/api/v1/network` interface for status,
statistics, interfaces, WireGuard, device networks, the operation listing and
separate operation acknowledgement, confirmation and rollback. `/mcp` offers
three network read tools over the same service, plus eight bench tools when the integration manager is configured. WireGuard reads expose safe
metadata only.

`nilo admin user|token` run in exactly one of two modes: local (`--data-dir`,
write access is the authority, `--actor` is attribution, no deletion) or remote
(`--server` with `--token-file`, an ordinary `internal/apiclient` client of the
identity administration routes, where the appliance authorizes the token on
every request and `user delete` / `token delete` exist behind a typed
confirmation or `--confirm-delete`). Ambiguous or incomplete mode options exit 2
before anything is opened or contacted. The root usage names every group.

`cmd/nilo` provides version reporting, the `serve-foundation-check` composition smoke command, development token issuance, the `nilo admin` local and remote administration commands, the `network`, `activity` and `diagnostics` API clients, and explicit simulation/appliance servers. Protected HTTP contracts provide login, logout, the current session, status, deterministic integration and device inventory for the selected simulation scenario, the identity listing, and the identity and token administration routes the browser Authentication page drives. Internal consumers compose the foundation with `app.Open` and authorize through `app.IdentityService()`, submit work through `operation.Service`, resolve policy through `policy.Resolver`, and use the narrow repository interfaces owned by the domain packages. Only a starting server composes with `app.New`, which is `app.Open` followed by startup reconciliation; every other command opens the appliance without touching the operations recorded in it.

## Invariants

- Every protected route declares its permission in `server.go` or `bench.go`; the command
  line is an API client and adds no authorization or identity rule of its own.
- A token's permissions are its owner's current role intersected with its
  scopes, resolved from stored state on every request.
- No credential is accepted on a command line: tokens come from `--token-file`,
  passwords from standard input; no response, log, audit event, export or error
  message carries a password, secret or verifier.
- An identity change and its audit event commit in one `BEGIN IMMEDIATE`
  transaction; one enabled administrator always remains; a refused change
  leaves the subject untouched.
- Permanent deletion is explicit (typed confirmation or `--confirm-delete`),
  distinct from disable and revoke, and every attempt, applied or refused, is
  on record with labels that outlive the row.
- Network changes go through preview, acknowledgement, confirmation and
  rollback; a noninteractive mutation needs `--acknowledge`, which is never
  consent for anything else.
- Whether a network interface may carry appliance management access is decided
  from what a provider observed about the hardware - the device kind, a physical
  adapter observation and the permanent adapter identity - and never from the
  interface name. A reading that carries no hardware observation is ineligible.
- The appliance records exactly one commissioning lifecycle state, `provisioning`
  or `commissioned`. It is seeded by the schema migration that creates it and
  read on every open; an absent or unrecognized value stops the start rather
  than resolving to either state.
- While the appliance is provisioning, the management allowlist is every
  eligible physical interface currently present, recomputed on each reconcile
  and persisted. Once commissioned, the persistent allowlist is the configured
  set alone. A Pi hardware request may add one non-persistent, deadline-bounded
  recovery interface without changing that record.
- The privileged helper publishes only process-owned Avahi D-Bus entries. It
  receives validated host/service/alias values and interface lists, never raw
  responder configuration, and does not replace daemon policy. Permanent
  discovery advertises only the configured TLS-covered system hostname; an
  Avahi collision suffix is refused rather than turned into an uncertified URL.
- Raspberry Pi button events are stored by the helper before exposure with the
  release time; recovery-duration events additionally carry the connected-link
  snapshot while reset events do not. Events are coalesced with reset precedence
  and acknowledged only after core handles them. Nonterminal recovery ownership
  is restored before a later event; an idle highest-generation tombstone stops
  acknowledgement failures from replaying an action. A long press atomically changes the live
  lifecycle and management admission to provisioning without restarting the
  process or touching identities, credentials, TLS, audit, integrations, or
  device configuration. For a recovery-duration event, already-connected-link
  refusal audits complete before core persists the handled generation, so an
  audit failure leaves the helper event available for a safe retry.
- A GPIO read discontinuity cancels the gesture in progress and requires a
  stable released baseline before another press. Recovery-profile
restoration proves the exact durable UUID, adapter/interface/MAC binding,
  volatile state and non-routing settings before deletion. NetworkManager's
  generated empty `proxy` section is part of the normalized owned profile;
  populated proxy settings or any other unexpected setting remain foreign.
  Post-mutation audit
  failures retain a durable ordered queue covering every required audit that
  can outlive a state advance or network effect, and never repeat or delay
  expiry cleanup of the network effect.
- Commissioning is one way and is completed only onto an allowlist the appliance
  can still be reached through. A transition that does not finish restores the
  allowlist that was serving and leaves the appliance provisioning.

## Key behaviors

- `REQ-FUNC-880eac8d` - preserves immutable operation requests and defined session transitions.
- `REQ-DATA-b2e24314` - atomically activates and reloads one digest-checked configuration version.
- `REQ-DATA-4357c6f2` - acquires complete resource claim sets or returns typed busy.
- `REQ-DATA-f9b0b55d` and `REQ-DATA-9608cfd8` - persists attributable audit history and verified artifacts.
- `REQ-SEC-0ecda0e7`, `REQ-SEC-f9fd49ff`, `REQ-SEC-035a8f94`, `REQ-SEC-50599a7e`, and `REQ-SEC-ae30d292` - enforce the foundation authentication, policy, and approval model.
- `REQ-REL-32b81ca8`, `REQ-REL-fe4a58f2` and `REQ-REL-e223f4b5` - reconcile interrupted sessions when the server starts, leave them untouched for every other command, and avoid false completion.
- `REQ-FUNC-4b3d9fd7` - exposes the versioned authenticated simulation status contract.
- `REQ-FUNC-649c67f8` and `REQ-SEC-f9fd49ff` - issue development tokens through non-login identities while retaining one-way verifier storage.
- `REQ-SEC-7fd3869a` - rejects missing, malformed, unknown, and revoked HTTP Bearer tokens uniformly.
- `REQ-PLAT-6f93c06b` - confines development simulation listeners to literal loopback addresses.
- `REQ-REL-fce79da4` - drains and closes the HTTP server after cancellation or termination.
- `REQ-FUNC-22f4f69c` and `REQ-FUNC-346ffa07` - return stable authenticated simulated integration and device inventories.
- `REQ-FUNC-9835998b` and `REQ-FUNC-7f4afa6b` - select only supported deterministic simulation scenarios and fail closed on unknown identifiers.
- `REQ-PLAT-30618e85` - keeps every simulation inventory entry unmistakably separate from physical equipment.
- `REQ-FUNC-7ae21313` - names which observed interfaces may carry management access, from recorded hardware facts rather than the interface name.
- `REQ-DATA-72876d18` - seeds the commissioning lifecycle state from whether the database already held an appliance identity when the migration ran.
- `REQ-REL-d9bfd8ae` - reports the recorded lifecycle state unchanged across restart and refuses to assume one.
- `REQ-FUNC-243c38a6` - derives the first-boot management allowlist from the eligible interfaces present and follows them as they appear and leave.
- `REQ-SEC-a1012bc5` - freezes the allowlist once commissioned so a newly attached adapter never joins it.
- `REQ-PLAT-b1abf074` - starts a first-boot appliance with no console listener option, and refuses one that is already commissioned.
- `REQ-FUNC-9a19ad36` - accepts link-local as a wired adapter's IPv4 method so direct commissioning needs no DHCP server and no guessed address.
- `REQ-FUNC-554b9bbd` - answers the appliance's local name on exactly the interfaces carrying management access and renews it as they change.
- `REQ-SEC-83deb5c6` - takes a validated advertisement rather than responder configuration, manages only configuration it wrote, and reports an appliance it cannot publish on.
- `REQ-FUNC-6334add6` - permanently advertises one stable `_nilo._tcp.local` instance per commissioned appliance with identity, configured hostname and usable HTTPS URL, withdrawing the shared alias before replacement.
- `REQ-FUNC-d2d75f06` - gives every role-safe first-boot Ethernet adapter an exactly inventoried, volatile, non-routing IPv4LL commissioning profile without inferring ownership from its name.
- `REQ-FUNC-2b5bfab7`, `REQ-FUNC-4249d837` and `REQ-SEC-281ef5e4` - arm a bounded post-transition recovery window, select only the first doubly verified unassigned Ethernet adapter, and exactly restore its temporary effects.
- `REQ-PLAT-c6a335b0`, `REQ-UX-8062385f` and `REQ-PLAT-ca44573d` - confine the BCM17/BCM27 button and indication path to explicitly enabled Raspberry Pi deliveries.
- `REQ-FUNC-fb666359` and `REQ-DATA-2c5b4a4d` - return only network access to commissioning while preserving appliance identity, credentials, TLS, audit and integration/device configuration.
- `REQ-FUNC-b49d7866` - fixes the management allowlist to what an administrator chose, records the appliance commissioned, and refuses a selection it could not be reached through.
- `REQ-REL-fa990acb` - restores the allowlist that was serving when a commissioning transition does not finish, leaving the appliance provisioning.
- `REQ-PLAT-18084875` - records a device as a physical adapter only when it reports a permanent hardware address and the kernel presents it on a real bus.
- `REQ-SEC-5d0cfe7e` and `REQ-SEC-74b46e3e` - map the three fixed roles to explicit permissions and bound a token by its scopes intersected with its owner's current role.
- `REQ-SEC-62e54172` - authorizes every protected route server-side and separates unauthenticated from forbidden.
- `REQ-FUNC-00305f82`, `REQ-SEC-8e89944b` and `REQ-FUNC-519eff58` - restrict first-administrator bootstrap, preserve one enabled administrator, and administer identities from the command line.
- `REQ-FUNC-db6fb2fa`, `REQ-FUNC-ec3d2e54` and `REQ-FUNC-7d80be16` - establish, end and report expiring browser sessions.
- `REQ-SEC-b5502ddb`, `REQ-SEC-49fe784b`, `REQ-SEC-d8026dbe` and `REQ-SEC-e14c8f51` - confine session cookies to the declared deployment mode, require a session CSRF token, brake repeated failed logins, and invalidate sessions on disable or password reset.
- `REQ-FUNC-ae14f9d3`, `REQ-FUNC-e325f08e` and `REQ-SEC-01c18c7c` - disclose a token secret once, list and revoke tokens without exposing secrets, and reject expired, revoked and disabled-owner credentials.
- `REQ-DATA-cffc3af2` and `REQ-DATA-d8d5a701` - append one complete common
  Activity audit envelope for every applied identity and token mutation without
  secrets, and migrate existing identities and tokens forward as least-privileged.
- `REQ-FUNC-5fd2806f`, `REQ-FUNC-27c1db30`, `REQ-FUNC-1fac821d` and `REQ-FUNC-907003de` - create an identity, change its role, disable and re-enable it, and replace its password over the authenticated HTTP interface.
- `REQ-FUNC-795b54d0`, `REQ-FUNC-f50eb6d3` and `REQ-FUNC-71c3c908` - list every token's safe metadata, create a named token whose secret is returned exactly once, and revoke one.
- `REQ-FUNC-938c5f85` - refuse an administrative request carrying an unknown role or scope, a bad username, a short password, a past expiry or a taken username, naming the field and changing nothing.
- `REQ-SEC-7595665c`, `REQ-SEC-89034a8a` and `REQ-SEC-6e3ae992` - authorize
  every administration route server-side, attribute every change to the actual
  request credential (user ID for a session, token ID for bearer) rather than
  any body field, and serialize no verifier of any kind.
- `REQ-FUNC-a1cefd8f` - `nilo network operation list` reads the operation
  listing the browser reattaches with; the route has no filters, so the command
  offers none.
- `REQ-FUNC-6590e0c5`, `REQ-SEC-7861a0b2` and `REQ-UX-3f48f396` - administer
  identities and tokens remotely over the administration routes with the token
  read only from a file, in exactly one mode per command, with readable or
  `--json` output, and name every command group in the root usage.
- `REQ-SEC-bfd1cf86` - a command-line permanent deletion states its subject and
  proceeds only on the subject typed back or `--confirm-delete`; no other flag
  is consent.
- `REQ-DATA-b4cdd70a` and `REQ-DATA-20645b45` - record every refused deletion
  as a denied event with its reason, and write applied deletion events with the
  subject's label and the actor's and owner's usernames so they read after the
  row is gone.

## Architecture

```mermaid
flowchart TD
    cli(["cmd/nilo"])
    admincli(["nilo admin"])
    netcli(["nilo network"])
    http(["HTTP /api/v1/status"])
    inventory(["HTTP inventory routes"])
    authroutes(["HTTP auth and user routes"])
    adminroutes(["HTTP identity and token administration routes"])
    netroutes(["HTTP /api/v1/network routes"])
    mcproutes(["MCP /mcp read tools"])
    middleware["identity resolution"]
    authorize["permission check"]
    catalog["simulation.Catalog"]
    network["network.Service"]
    management["network.ManagementAccessService"]
    hwrecovery["network.HardwareRecoveryService"]
    mdns["network.MDNSAnnouncer"]
    operations["network.OperationManager"]
    provider["network.Provider: simulation or helper client"]
    changer["network.ConnectivityChanger: simulation or helper client"]
    sampler["network sampler"]
    helper["root nethelper: peer-authenticated Unix RPC"]
    nm["NetworkManager D-Bus and independent checkpoint timer"]
    avahi["Avahi process-owned D-Bus group"]
    gpio["Pi GPIO v2 button and LED"]
    recovery[("root-only recovery records")]
    networkrecovery[("SQLite network_recovery")]
    provider --> helper
    changer --> helper
    helper --> nm
    helper --> avahi
    helper --> gpio
    helper --> recovery
    apiclient["internal/apiclient"]
    app["app.New"]
    identity["identity.Service"]
    roles["auth role matrix"]
    op["operation.Service"]
    policy["policy.Resolver"]
    session["session.Service"]
    runner(["integration supervisor"])
    artifact["artifact.Store"]
    audit["audit.Repository"]
    sqlite[("SQLite repositories")]
    files[("artifact root")]
    cli --> app
    cli --> http
    cli --> catalog
    admincli --> identity
    http --> middleware
    inventory --> middleware
    authroutes --> middleware
    adminroutes --> middleware
    adminroutes --> identity
    middleware --> identity
    middleware --> authorize
    authorize --> roles
    authorize --> catalog
    identity --> roles
    identity --> audit
    identity --> sqlite
    app --> identity
    app --> sqlite
    app --> op
    op --> policy
    op --> session
    op --> runner
    op --> artifact
    op --> audit
    policy --> sqlite
    session --> sqlite
    audit --> sqlite
    artifact --> sqlite
    artifact --> files
    netcli --> apiclient
    apiclient --> netroutes
    netroutes --> middleware
    netroutes --> network
    mcproutes --> middleware
    mcproutes --> network
    netroutes --> operations
    network --> provider
    network --> sqlite
    app --> management
    app --> hwrecovery
    management --> helper
    hwrecovery --> management
    hwrecovery --> helper
    hwrecovery --> networkrecovery
    mdns --> helper
    operations --> changer
    operations --> provider
    sampler --> network
    sampler --> sqlite
```

The runtime shares one `network.Simulation` across observations, changes,
WireGuard metadata and counters. `RunMaintenance` writes minute history and
expires unconfirmed operations. `ChangeController` carries validated import and
startup parameters; public operations contain a revision digest rather than raw
configuration. Mutation I/O is serialized independently of readable progress.
Failure to restore or discard recovery cannot become a successful outcome.

The explicit Linux appliance profile runs Bench Core unprivileged and uses
`internal/nethelper` over a restricted Unix socket. The root helper validates peer
UIDs, bounds JSON requests and binds one saved interface change to durable recovery
before arming a target-specific NetworkManager checkpoint. NM owns the independent
120-second timeout; startup reconciliation verifies connection/autoconnect and
normalized IP/route/DNS state before allowing further management. No saved network
profile is rewritten for interface toggles. The explicit systemd installer is in
`02_Tools/scripts/install-nethelper.sh`; its service and recovery timer run on the
bench Raspberry Pi, while reboot-during-change recovery remains unverified.

`wireguard.check` reads NetworkManager availability/version without D-Bus auto-start,
loaded/running-kernel WireGuard evidence and `/usr/bin/wg --version` with a fixed
environment, two-second timeout and 4 KiB output limit. Unknown probes carry
`reported: false` and fixed diagnostics, never raw command output. Live results
carry `readiness_only: true`; they do not inventory profiles or claim tunnel state.
No module loading, profile file reads, package installation or shell occurs on this
read path. Missing required tooling remains an explicit `wg` row with no version.

`internal/network/install.go` adds an optional asynchronous installation controller:
acknowledgement reserves the mutation slot and the maintenance loop polls progress.
`internal/nethelper/install*.go` owns the job, private durable last-job record,
supported-distribution/source checks and fixed APT transaction. The version-2
pre-install guard is also a fixed `nilo-nethelper` process mode, so the actual
locked transaction cannot change unrelated packages after an earlier simulation.
Only wireguard-tools is installed; readiness is probed again before success.
Request cancellation does not own the worker; service shutdown cancels and joins
it, and restart reports interruption without replay. `probe.go` shares bounded
metadata reads between readiness and installation. `install_system_linux_test.go`
exercises real signed installation, locks, offline errors and idempotence on the
pinned Debian 13 and Ubuntu 24.04 disposable fixtures. The separate HTTPS process
test exercises the already-installed path through a real unprivileged core.

Appliance serving requires a built GUI and TLS certificate/key, forces secure
session cookies, confines a legacy literal listener to a private address, and
guards the provisioning wildcard listener by the derived interface allowlist.
It labels the GUI with safe runtime metadata before authentication. Instrument inventory is empty. WireGuard
has no live fallback. NetworkManager pseudo-devices or hot-unplugged adapters
without sysfs counters produce a zero counter sample instead of stopping the
service. See `06_SoftwareDocu/Network_Service_Implementation_Status.md` for the
remaining hardware and reboot acceptance work.

`identity.Service` decides whether an actor may *request* an action. `policy.Resolver` independently decides whether an operation may *run* without a human approval. Neither the Administrator role nor a fully scoped API token changes the second decision, so identity authorization can never become an approval or safety bypass.

Identities and API tokens can also be removed permanently, which is a distinct
operation from disabling and from revoking rather than a stronger word for
either. `DELETE /api/v1/users/{username}` (`users.manage`) removes the identity
row and, in the same transaction, every browser session row it holds; it is
refused with `last_administrator` while the identity is the last enabled
administrator and with `user_has_tokens` while it owns any API token, revoked
and expired ones included, because a cascade would destroy credential records
on the strength of a decision that was about a person. `DELETE
/api/v1/tokens/{id}` (`tokens.manage.all`) removes the token row in any state:
revoking keeps the record and stops the secret, deleting removes the record,
which also stops the secret because the row a bearer secret is matched against
is gone. Each successful removal appends exactly one `identity.user.deleted` or
`identity.token.deleted` event carrying the safe description of what was
removed — for a user the username, role and prior enabled state; for a token the
name, owner ID and owner username, scopes, expiry and prior revoked/expired
state — with the subject's name as the event's target label and the acting
administrator's username in the details, because the record itself no longer
exists to be looked up afterwards. A refused deletion (not an administrator,
last enabled administrator, owned tokens, unknown user or token) appends one
`identity.user.delete_refused` or `identity.token.delete_refused` event with
outcome `denied`, the requested subject as label (`invalid` or `unknown` when
none was usable; the unusable input stays out of the record) and the reason
code, written through the ordinary writer after the refusing transaction has
rolled back. The refusal never depends on that record, but a record that
cannot be written is joined to the returned error (`errors.Join`, so
`errors.Is` still finds the reason and the HTTP code is unchanged); a store
failure inside the transaction is not a refusal and is not recorded. The remote
command line treats the listing reads around a change as informational: a 403
on the pre-deletion listing skips the description, and a failed re-read after
an applied change is reported with exit 0. A credential lacking the route permission is refused earlier by the
HTTP access guard as `identity.access.denied`. There is no soft deletion and no
second mutation path.

Every administrative identity change runs inside one `BEGIN IMMEDIATE`
transaction supplied by the identity repository, which also supplies the audit
repository bound to it. `audit.Writer.Compose` supplies the same appliance,
boot, clock, actor, category, outcome and severity envelope used by other common
Activity events, then the transaction-bound repository appends it before
commit. The write lock is taken before the change reads anything, so the
last-enabled-administrator count is read and enforced without another
connection or another process changing it in between — for a deletion exactly as
for a demotion or a disable, so two concurrent removals through separate
connections can never both be applied — and a change whose audit event is
rejected is rolled back with it rather than surviving unattributed, which for a
deletion means the identity, its sessions and the token stay.
Successful reads and rejected or no-op changes do not create mutation events;
a refused permanent deletion creates a denied event instead. A
browser session is inserted by a statement conditional on the identity still
carrying the password verifier the login checked and still being enabled, so a
completed password reset cannot be overtaken by a login already in flight.

The simulation catalog is deliberately dependency-free and cannot discover or contact physical equipment; runner-backed discovery remains later work.

## Files

- `internal/nethelper/`, `cmd/nilo-nethelper/` - bounded privileged D-Bus interface operations, root-only durable recovery/event records, process-owned Avahi publication, and the explicitly enabled Pi GPIO path.
- `cmd/nilo/network_runtime*.go`, `internal/httpapi/serving.go` - explicit live provider selection, TLS and runtime-aware static GUI.
- `02_Tools/tests/network/` - disposable real-NetworkManager and unprivileged HTTPS process tests (opt-in, separate from the ordinary suite).

- `internal/network/` (under the Go module) - operation safety, simulation, profile validation, sampling and statistics, plus the device-network model and the pure forwarding planner.
- `internal/network/commissioning.go` - the one-way transition out of first boot: it checks the appliance can still be reached through the chosen interfaces, applies and persists them, stops deriving, and restores what was serving if any step does not finish.
- `internal/appliance/commissioner.go` - adapts the durable lifecycle writer to the narrow interface the network service calls, returning nil when there is nothing to record through.
- `internal/httpapi/commissioning.go` - the administrator-only route, mapping each refusal onto a conflict rather than a service failure.
- `internal/network/mdns.go`, `mdns_announcer.go` - validated provisioning aliases and commissioned DNS-SD records, plus restart-aware reconciliation against the helper's actual publication.
- `internal/nethelper/avahi_plan.go`, `mdns.go`, `avahi_linux.go` - the typed publication plan, privileged operation, and Linux Avahi D-Bus entry-group implementation; no daemon configuration file is replaced.
- `internal/network/commissioning_link.go`, `internal/nethelper/networkmanager_commissioning_linux.go` - role-safe first-boot IPv4LL intent and the exact owned volatile NetworkManager profile lifecycle.
- `internal/network/hardware_recovery.go`, `cmd/nilo/hardware_recovery.go` - durable arm/link/expiry/reset coordination and live runtime composition.
- `internal/nethelper/direct_recovery.go`, `networkmanager_recovery_linux.go`, `recovery_hardware.go`, `gpio_*.go` - exact temporary-link ownership/restoration and durable Pi button/indicator behavior.
- `internal/store/sqlite/hardware_recovery_repository.go`, `migrations/014_network_recovery.sql` - durable core recovery interlock and handled hardware generation.
- `cmd/nilo/listener_mode.go` - the two decisions `serve` makes about its listener: which mode the options ask for, and whether first-boot serving is permitted by the appliance's recorded state. Pure, so both are asserted without a subprocess.
- `internal/appliance/commissioning.go` - the lifecycle vocabulary, its validity rule and the load that refuses an unusable recorded value. It reads nothing itself; `internal/store/sqlite/commissioning_repository.go` satisfies its store interface.
- `internal/store/sqlite/migrations/013_commissioning.sql` - the singleton `commissioning` table and the seeding rule, expressed in SQL because it is the only code that runs before the first appliance identity row exists.
- `internal/network/eligibility.go` - the single rule naming which observed interfaces may carry management access. Pure: it reads a reading's kind, physical observation and adapter identity and nothing else.
- `internal/nethelper/adapter_identity.go` - the physical-adapter decision and the `/sys/class/net/<name>/device` probe. Both are portable and untagged so they are tested on any development host; only the D-Bus read that feeds them lives in `networkmanager_linux.go`.
- `internal/network/{devicenetwork,forwarding_plan}.go` - the saved device-network decision and the owned resource set it implies. Both are pure: their only imports are `errors`, `fmt`, `sort`, `strings` and `net/netip`, so a plan cannot reach a host. Nothing applies a plan yet.
- `internal/network/persistence.go`, `internal/store/sqlite/network_operation_repository.go` - the durable record of a managed change, holding public metadata only and refusing a record whose parameters name a secret.
- `internal/httpapi/network*.go`, `internal/mcp/`, `internal/apiclient/` - shared authenticated Network adapters.
- `cmd/nilo/network*.go` - Network CLI with interactive or explicit acknowledgement; confirmation remains separate.
- `cmd/nilo/admin.go`, `cmd/nilo/admin_remote.go` - the administration commands: mode resolution and the local data-directory path in the first, the remote API client, readable renderings and the deletion confirmation in the second. `admin_remote_test.go` proves the requests against a recording stub; `admin_live_test.go` proves roles, scopes and credential state against a real `nilo serve` subprocess.

- `05_Software/nilo_assistant/cmd/nilo/` - command entry point and CLI contract tests.
- `05_Software/nilo_assistant/internal/app/` - foundation composition and startup reconciliation.
- `05_Software/nilo_assistant/internal/config/` - process configuration and physically confined paths.
- `05_Software/nilo_assistant/internal/domain/` - UUID identities, immutable requests, artifacts, and session states.
- `05_Software/nilo_assistant/internal/operation/` - policy-to-runner operation orchestration.
- `05_Software/nilo_assistant/internal/policy/` - policy matrix, immutable snapshots, and approvals.
- `05_Software/nilo_assistant/internal/session/` - session transitions, lease ownership, and reconciliation service.
- `05_Software/nilo_assistant/internal/auth/` - password, role/permission, browser-session and API-token primitives, plus the failed-login brake.
- `05_Software/nilo_assistant/internal/identity/` - identity, session, token and authorization service shared by the command line and the HTTP boundary.
- `05_Software/nilo_assistant/internal/audit/` - the event contract, its envelope, and the writer that fills the appliance, boot, category, severity and clock fields so a caller supplies only what it knows. Append, list and bounded retention; no operation can change a stored event.
- `05_Software/nilo_assistant/internal/appliance/` - the stable appliance identity and the per-boot identity, provisioned while the application opens and therefore before any service that could record an event exists.
- `05_Software/nilo_assistant/internal/artifact/` - bounded, confined, durable artifact placement with OS adapters.
- `05_Software/nilo_assistant/internal/store/sqlite/` - database opening, migrations, and all foundation repository implementations and tests.
- `05_Software/nilo_assistant/internal/buildinfo/` - build metadata reported by the command.
- `05_Software/nilo_assistant/internal/httpapi/` - loopback-confined simulation listener, credential resolution and per-route permission checks, cookie and CSRF handling, the authentication and status contracts, and graceful lifecycle. `identity.go` holds the middleware and the sign-in, sign-out and current-session handlers; `administration.go` holds the identity and token administration handlers, their validation and the one map from a typed service failure to a documented client error; `server.go` holds the single route table where every route declares its permission.
- `05_Software/nilo_assistant/internal/simulation/` - immutable healthy and unavailable integration/device catalogs with stable simulation identity.
- `05_Software/nilo_assistant/go.mod` and `05_Software/nilo_assistant/go.sum` - pinned Go module graph.
- `05_Software/nilo_assistant/README.md` - portable core usage and boundaries.

### Network gateway observations

Network interface readings expose Gateway4 and Gateway6 from the active NetworkManager IP configurations. HTTP adds gateway4/gateway6 strings (empty when absent); the deterministic simulation catalog includes both families for its wired device. The disposable dual-stack NetworkManager fixture checks these observations and recovery with default gateways present. No routing configuration is edited by this read path.
- `cmd/nilo/appliance_stage_linux_test.go` - the appliance verification stage's
  proof: starts the root helper and an unprivileged appliance-profile core with
  generated TLS material and a built interface directory, then asserts the
  listening record, the authenticated boundary, and that every adapter Bench
  Core reports is one NetworkManager detected in the stage's own namespace.

## Device networks, forwarding, Activity and diagnostics (2026-09-04)

Added under TASK-943403e2, P2 to P9. Packages and the seams between them:

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
  `Monitor` (audit health); `internal/diagnostics`: timed sessions and the
  `Trace` debug sink the operation manager calls. `internal/store/sqlite`
  migrations 008 (interface settings), 009 (device networks), 010 (activity
  policy, retention watermarks, diagnostic sessions).
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

## Exclusive physical-adapter roles (2026-09-11, TASK-8f18c927)

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

## Management-eligible interfaces (2026-09-12, TASK-2873801a)

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

## Appliance commissioning lifecycle (2026-09-12, TASK-ac1e9003)

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

## First-boot management serving (2026-09-12, TASK-20c64f67)

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

## First-boot local-name discovery (2026-09-12, TASK-878ab570)

While an appliance is being commissioned it answers one local name on exactly
the interfaces carrying management access, so an installer can type one URL and
reach it from whichever network they are on. `network.MDNSAdvertisement` is a
validated host/service/alias object and an interface list before it leaves the
unprivileged process. Invalid labels, loopback or duplicate interfaces, unknown
service fields, and configuration separators are refused. During provisioning
the object contains only the scoped `nilo.local` alias; after commissioning it
contains only one `_nilo._tcp.local` service with stable appliance UUID,
configured hostname and current HTTPS URL. A temporary recovery overlay adds
`nilo.local` only on the selected recovery interface without widening the
permanent service.

`network.MDNSAnnouncer` compares desired publication with both its local cache
and `mdns.status` from the helper. An unchanged healthy entry group is left
alone; a helper or Avahi restart, failed publication, interface change, port
change, or lifecycle change causes replacement or withdrawal on the next
reconcile. An empty provisioning interface set withdraws the alias. Discovery
failure is reported and retried while address reachability remains available.

The privileged helper accepts only the typed `mdns.publish` and `mdns.status`
operations and owns one entry group on its Avahi D-Bus connection. It publishes
the current addresses of the requested interface indexes and never rewrites
daemon configuration or another publisher's group. The configured system
hostname is the permanent service target; if Avahi reports a collision-renamed
effective host, publication fails closed so NILO never advertises a URL outside
the installed certificate. A helper with no reachable responder reports the
publication unavailable rather than appearing to succeed.

## Completing commissioning (2026-09-12, TASK-bd56e123)

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

A selection the configuration rejects is answered as a conflict, not a service
failure, so an administrator's mistake is never dressed up as an outage.
Reassigning a freed adapter to a device network is the existing device-network
workflow: the role coordinator already refuses an adapter that is still in the
management allowlist, and commissioning onto a smaller allowlist is what frees
one.

## Whole-appliance power control (2026-09-08, TASK-a3a3d7b2)

`POST /api/v1/system/power` accepts only `restart` or `power_off`. The route is
guarded by `system.power.manage`, held by Operator and Administrator, then
requires a browser-session credential in addition to CSRF; Viewer and bearer
credentials never reach the controller. A `system.power.requested` audit record
must persist before dispatch or the request fails closed.

`internal/power.Controller` is the consuming interface. Simulation composes its
inert `Unavailable` implementation. Appliance composition uses the existing
peer-authenticated helper client, whose closed protocol adds only empty
`system.restart` and `system.power_off` envelopes. The root helper maps those to
systemd-logind `Manager.Reboot(false)` and `Manager.PowerOff(false)` over system
D-Bus. No shell, executable name or caller-supplied argument crosses the
boundary. The local Pi restart and human-operated power cycle are early hardware
observations rather than gates.

### Managed instrument composition

`app.StartBench` composes the durable manager for `serve` only. Migration 011 adds
plugin state and jobs linked to existing sessions. `operation.Service.Prepare`,
`Run` and `ApproveQueued` preserve the synchronous Submit/Approve wrappers while
separating durable admission from execution. `AppendInTransaction` lets queued
settlement and its audit envelope share one SQLite transaction. Focused proof:
`internal/operation/queue_test.go`, `internal/bench/jobs_test.go`, and the shared
HTTP/MCP/CLI bench tests listed in bench-integrations.md.
