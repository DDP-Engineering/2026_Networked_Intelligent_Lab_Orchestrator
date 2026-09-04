# Network service implementation status

Audited 2026-09-04 against `00_Planning/PLAN_NETWORK.md` and
`00_Planning/PLAN_DEVICE_NETWORKS_AND_AUDIT.md`, on `master`, under
`TASK-943403e2`. This is an implementation checkpoint, not appliance acceptance.
No staging, commit, push or version change was made.

## Device networks and audit extension: phase state

The extension plan runs P0 to P11. Everything below was proven on Windows with
the pinned toolchain, on isolated Linux in the disposable NetworkManager
container (`02_Tools/tests/network/check.sh`, twelve runs this task), or - where
it says so - observed on the bench Raspberry Pi. None of it is hardware
acceptance on a controlled device, and no production device network exists.

| Phase | State |
|---|---|
| P0 reconcile scope | Done. 40 requirements, `.agentic/specs/REQ-FUNC-2f8d63b5.md`, section-13 acceptance IDs mapped, defaults recorded as RULING-006 |
| P1 durable operations and audit | Done. Appliance and boot identity, audit envelope, persisted sequence allocator, durable network operations, reattachment route, refusal when audit is unavailable |
| P2 WireGuard profile lifecycle | Done on isolated Linux. Root-owned keystore reached over the helper socket, NetworkManager tunnel connections (`nilo-wg-*` on `nilo0`), import, rename, remove, activation and switching under a system-wide checkpoint, startup connect with retry, restart reconciliation against the helper's record. Two profiles survive a restart and are switched, put back explicitly and by the timer, with no key outside the root-only store |
| P3 interface configuration and Wi-Fi | Done on isolated Linux for wired addressing; Wi-Fi join unverified on real radio. Settings bound to the adapter's permanent hardware identity, NILO-owned `nilo-if-*` profiles, static/automatic addressing, DNS, management protection, never-default, management-path trace refusing the last path, Wi-Fi scan and join with the password staged in the keystore and never returned |
| P4 device-network contracts and planning | Done. Model, validation, pure planner, persistence, CRUD through the acknowledged lifecycle, computed status (disabled/active/waiting/conflict), connection check facts, access instructions, HTTP routes, CLI, simulation and appliance managers |
| P5 local forwarding and recovery | Done on isolated Linux. The helper renders one owned nftables table from a durable inventory written before any rule, owns the forwarding switch only where it turned it on, recovers a device-network change from its inventory snapshot, checks its table every ten seconds, and refuses a foreign table. Container proof: a client reaches a Modbus server behind the appliance with only a route to it, the unauthorized source is refused, the timer restoration removes everything, and an established session ends at the removal |
| P6 VPN forwarding and translation | Done on isolated Linux. Return-route validation against the profile's AllowedIPs; container proof with a disposable hub, a remote client and two sites whose devices share 192.168.0.130, reached independently at 10.7.1.130 and 10.7.2.130, never through the internal address or from an unauthorized source, with an 8 MiB transfer and tunnel loss leaving the other site untouched |
| P7 Activity, events and diagnostics | Done. `activity.read`, `activity.export`, `diagnostics.manage`; server-paginated filtered history with retained window and logging health; JSONL/CSV export with metadata, bounds and formula neutralization; retention by age and quota in bounded batches with watermarks; timed debug sessions with monotonic expiry and restart interruption; login, access-denial, export, policy, session and reconcile events |
| P8 integrated GUI | Done against the contract mock and the component tests. Device networks table and editor, adapter configuration and Wi-Fi dialog, WireGuard rename, Activity page with diagnostics controls, activity links from the Network page, reattachment after a reload, en/de/ro |
| P9 appliance startup and system acceptance | Partly done. `nilo-nethelper.service`, `nilo-nethelper-recovery.timer` and the HTTPS appliance service are installed, enabled and running on the bench Pi; ARM64 and x86-64 builds complete. No VM crash, pending-operation reboot or offline-startup scenario was run |
| P10 Raspberry Pi and device acceptance | Partly observed on the bench Pi 4 (Debian 13, NetworkManager 1.52.1). NILO assigned `192.168.0.1/24` to the wired adapter, enabled a local-only Device Network, installed its `inet nilo` nftables rules and routed the Windows client at `192.168.1.13` to the ILC151GSM at `192.168.0.130` with three ICMP replies and no loss. TCP 502 and 503 were refused, while TCP 504 returned a valid Modbus function-3 response from both Pi and Windows through NILO: unit 1, holding register 0, value 1. USB hotplug, power-cycle and vendor download remain unverified |
| P11 final reconciliation | Done for this checkpoint; see the journal's Stage report |

## What each phase added

### P2 - WireGuard profiles on the appliance

`internal/nethelper/keystore.go` remains the only place key material is written.
The staging RPC exists: `wireguard.secrets.stage` is the one operation that may
carry a `SecretPayload`, the response type has nowhere to put a secret (proved
structurally), and a staged handle is bound to a profile identity on
acknowledgement or swept when abandoned. `wireguard.profile.create` turns the
public profile and the promoted key material into a NetworkManager connection
named `nilo-wg-<id>` on interface `nilo0` with system-owned secret flags;
`wireguard.inventory` lists every WireGuard connection with its public
configuration and marks the ones NILO did not create as external, which are
never activated or deleted through NILO. Activation and deactivation run under a
system-wide checkpoint whose snapshot digests every non-tunnel device's
connection, address, route and DNS state, so a restoration is verified against
the whole appliance. At most one NILO-owned tunnel is up; a switch deactivates
the previous one first. The startup selection is connected at boot by the
system actor and retried every thirty seconds while the uplink is missing.
Rename is a profile change; removal refuses an active profile and one a device
network depends on.

### P3 - adapter configuration and Wi-Fi

`internal/network/interface_settings.go` binds a configuration to the adapter's
permanent hardware address, so it follows the adapter across renames and
replugs and is reported as unattached when the adapter is absent. The helper
writes a NILO-owned `nilo-if-<mac>` profile with `ipv6.method=link-local`
(IPv4 addressing is what is managed) and activates it; the operating system's
own profile is untouched and comes back on restoration. The management-path
trace (`internal/network/management.go`) refuses a change that would leave the
caller's own path or the last management path unusable, using the request
origin the boundary attaches to every change. Wi-Fi scan classifies open,
WPA2-personal and WPA3-personal as joinable and everything else as unsupported;
a join stages the password in the keystore, sends only a handle onward, and the
helper reads it once and discards it. A virtual adapter without a permanent
address is identified by its current one.

### P4 - device networks

`internal/network/device_network_manager.go` owns the saved decisions and keeps
the installed forwarding in step with them behind two seams: an environment
(adapters bound to saved settings, profiles) and a forwarder. Status is computed,
never stored - intent from the record, the plan from what is present now,
applied from what the forwarder holds - so an enabled network whose tunnel is
down reads "waiting", not "on" or "off". A connection check reports separate
facts (saved, forwarding installed, target probed) and never turns an
inconclusive probe into "offline". Access instructions are per machine and carry
no secret. Migration `009` stores the decisions; `GET|POST
/api/v1/network/device-networks[/preview|/{id}|/{id}/checks|/{id}/access-instructions]`
and `nilo network device-networks ...` expose them.

### P5 and P6 - forwarding

`internal/nethelper/forwarding.go` keeps `forwarding.json`, the inventory of the
one table NILO owns (`inet nilo`, marked by its comment) and of the forwarding
switch it turned on. The inventory is saved before a rule is written, the table
is installed before the switch goes on and the switch goes off before the table
goes away. `nftables_render.go` writes one script per inventory applied as one
transaction: replies of admitted flows pass, every client packet passes the
grants, a translated grant matches the address the client used
(`ct original ip daddr`), the internal address is dropped on the device link,
IPv6 is dropped toward it, and the forward chain's policy is `drop` only where
NILO owns the switch - before NILO nothing was forwarded either. A device-network
change is recovered from a snapshot of the inventory, not a NetworkManager
checkpoint; the helper repairs a vanished table every ten seconds and reports a
foreign one instead of replacing it. The planner refuses VPN sources the
selected profile cannot route back.

### P7 - Activity and diagnostics

`internal/activity` answers `GET /api/v1/activity` (category, function, target,
actor, severity, outcome, operation, action, time window, cursor, limit) with the
retained window per category, the watermark retention left and the logging
health from a monitor on every audit append; `GET /api/v1/activity/export`
streams JSONL or CSV with schema version, appliance identity, time range,
retained window, redaction policy and a trailer that says whether the bound was
hit. `Housekeeper.Sweep` purges by age then by quota in batches of 500, keeps
the evidence of a pending operation, records watermarks and one event per sweep
that removed something. `internal/diagnostics` runs timed sessions for fixed
function scopes (5/15/60 minutes, monotonic expiry, one per scope and target,
interrupted on restart) and writes debug records only while a scope is raised,
rate-limited per scope with drops counted. Coverage added: login success and
failure, access denial, operation stages as debug detail, runtime re-plans of
device networks, retention sweeps, exports, session lifecycle, policy changes.

### P8 - the interface

`src/api/{devicenetworks,activity,settings}.ts` parse the new resources;
`DeviceNetworksPanel`, `InterfaceSettingsDialog`, the WireGuard rename control
and `ActivityPage` present them; the `activity` core page sits behind
`activity.read`; Network is a collapsible group with focused Overview, Network
adapters, Device access, VPN profiles and Diagnostics pages; each page picks up
an operation still applying or
awaiting confirmation after a reload from `GET /api/v1/network/operations`, and
links to Activity already narrowed. The contract mock answers every new route
through the same acknowledged lifecycle, so the interface behaves identically
against the mock and against the simulation profile.

### P9 - startup and independent recovery

`nilo-nethelper recover --state-dir` is one pass over the saved record and
inventory, run by the root-owned `nilo-nethelper-recovery.timer` twenty seconds
after boot and every thirty seconds; it takes the helper's lock without waiting
and does nothing while the helper is alive. `install-nethelper.sh` installs and
enables it beside the service and warns when `nft` is absent.
`02_Tools/scripts/build-appliance.sh` builds both binaries for `linux/arm64` and
`linux/amd64`.

## What runs

Simulation and appliance profiles compose the same service, operation manager,
device-network manager, activity service, diagnostics service and maintenance
loop. The simulation keeps device networks and adapter settings in SQLite and
forwards in memory; the appliance forwards through the helper. The maintenance
loop samples every five seconds, re-plans device networks on the same cadence,
expires unconfirmed changes and debug sessions every second, retries the
startup tunnel every thirty seconds and sweeps retention every five minutes.

## Remaining work

| Area | Remaining |
|---|---|
| systemd and VM | The service and timer run on the bench Pi; run core/helper/NetworkManager crash, reboot during a pending apply, offline startup and migration with prior data |
| Wi-Fi | Join a real personal network on a real radio; the container has no wireless device |
| Hardware | USB adapter hotplug, power-cycle recovery, broader Modbus validation against the PLC's documented register map, and vendor application download with an authorized tool |
| Events | Adapter appeared/disappeared/rebound, handshake availability transitions, time-synchronization changes, service start/stop |
| Interface | Browser tests for the Activity page against the real development topology; the compatible-connection chooser for enabling an adapter with no previous connection; acceptance of the compact field layouts on the physical seven-inch panel |
| Requirements | The ones whose verification names hardware, a VM or a browser stay `draft` |

## Compact field interface (2026-09-04, TASK-9ca3f883)

The Network interface was re-laid out for the seven-inch appliance display under a
separate GUI task; no backend, contract, permission or confirmation behaviour
changed. Network is now one collapsible navigation group with separate Overview,
Network adapters, Device access, VPN profiles and Diagnostics workspace tabs.
Overview shows the uplink, device side, active profile and what needs attention;
the three configuration pages each
render as a table with the essential columns and a labelled-card reflow below
56rem, with every secondary value in an explicit Details region and every action
in one compact per-row Actions dialog above the page. The dialog names the
object type and current status, states why an entry is withheld, separates
destructive choices and cannot be clipped by the table's scroll area. The
device-network editor is grouped Identity, Device side, Access path, Addressing,
Services, Advanced (collapsed) and Review, validates address, prefix, source and
port formats beside the field before any preview, and keeps ports editable with
502 as an example only. Settings carry a help popover that works without hover,
persistent guidance under the fields that need it, and a How to configure help
for the device-network and adapter workflows that names the external route a
management computer, router or VPN hub still needs and says a probe from NILO
does not prove remote access. Diagnostics owns WireGuard readiness and traffic
history. Its traffic chart labels RX and TX, byte volume per measured minute
and time, keeps missing samples as gaps, and presents exact values for a minute
selected by pointer, touch or keyboard. Readiness labels and values use compact
horizontal facts, while the Statistics title, interface and period controls
share one header, so Diagnostics needs no initial vertical scroll at 1024x600
and larger. `e2e/field-display.spec.ts` proves the five destinations at the four
viewports in both themes; the physical panel is still unverified.

## CLI examples

```text
nilo network status --server URL --token-file TOKEN_FILE
nilo network interface settings ID --method static --address 192.168.0.1/24 --never-default --server URL --token-file TOKEN_FILE
nilo network wifi scan ID --server URL --token-file TOKEN_FILE
nilo network vpn profiles rename ID --name "Head office" --server URL --token-file TOKEN_FILE
nilo network device-networks create --file line-3.json --server URL --token-file TOKEN_FILE
nilo network device-networks enable ID --server URL --token-file TOKEN_FILE
nilo network device-networks check ID --tcp-port 502 --server URL --token-file TOKEN_FILE
nilo network operation list --server URL --token-file TOKEN_FILE
nilo network operation confirm OPERATION_ID --server URL --token-file TOKEN_FILE
nilo admin user list --server URL --token-file TOKEN_FILE
nilo admin token delete --id TOKEN_ID --confirm-delete --server URL --token-file TOKEN_FILE
nilo activity list --category audit --operation OPERATION_ID --server URL --token-file TOKEN_FILE
nilo activity export --format csv --out activity.csv --server URL --token-file TOKEN_FILE
nilo diagnostics start --scope network.wireguard --target office --minutes 15 --server URL --token-file TOKEN_FILE
nilo diagnostics settings --retention audit=90,event=14,debug=1 --quota audit=268435456,event=134217728,debug=67108864 --server URL --token-file TOKEN_FILE
```

Mutations print a preview, then need `yes` or `--acknowledge`; connectivity
changes still need `network operation confirm OPERATION_ID` before their
deadline. Acknowledgement never implies confirmation. `network operation list`
is the same listing the browser uses to reattach to a pending change after a
reload; it takes no filters because the route offers none. The `admin` group
uses the same `--server` and `--token-file` options in its remote mode; a
permanent deletion there needs the subject typed back or `--confirm-delete`,
and `--acknowledge` is not an admin option.

## Verification entry points

- Isolated Linux: `wsl -d Ubuntu-24.04 -- sh -lc 'cd /mnt/f/<checkout> && NILO_NETWORK_RACE=1 sh 02_Tools/tests/network/check.sh'`.
  The container needs `NET_ADMIN`, `SYS_ADMIN` and writable `/proc/sys`, which
  `check.sh` grants; it stays disposable and mounts the checkout read-only.
- Go module: `02_Tools/env/with-go.ps1 go test ./... -count=1`, `go vet ./...`,
  `$env:GOOS='linux'; go vet ./...`.
- Interface: `02_Tools/env/with-tools.ps1 npm.cmd run test:ci`, `lint`,
  `format:check`; `npm.cmd run dev:mock` for the mock walkthrough.
- Configured gate, from the repository root: `python .agentic/aegis.py check --task TASK-943403e2 --strict --run`.
- Raspberry Pi observations: see the task journal's P10 entry; nothing on the Pi
  is a gate.
