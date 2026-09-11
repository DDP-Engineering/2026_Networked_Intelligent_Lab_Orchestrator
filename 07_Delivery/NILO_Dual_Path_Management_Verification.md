# Dual-path NILO management access verification

## Candidate and scope

This report covers the dual-path management-access candidate in workspace
`F:\NILO_dual_path_management_access`, branch
`ddp/nilo-dual-path-management`, at merge commit
`04ccc12209c4062855fc28702221f5a0d0f190d6`. The branch includes remote
`master` commit `81dbe0e937e4f7c92fd1e33babaa23a4022c3b1c`, including the final login
interface refinement from `9ba1a07`.

Nothing was pushed. No PLC, production certificate, Wi-Fi credential or
WireGuard credential was modified. The authorized Ubuntu mini-PC development
appliance and Raspberry Pi development appliance described below were updated.

## Implemented contract

- One unprivileged Bench Core, one wildcard HTTPS socket and one state directory
  serve all enabled paths. Starting multiple servers on the same database is
  neither required nor supported.
- The saved policy contains validated Linux interface names, management port and
  stable certificate identities. Current DHCP addresses are refreshed runtime
  observations, not persisted identifiers.
- Socket admission rejects local destination addresses not owned by an enabled
  interface before TLS or HTTP. Every HTTP request is checked again so a reused
  connection cannot outlive an allowlist change.
- The privileged helper owns a dedicated `inet nilo_management` nftables table:
  enabled input interfaces are accepted on the management port and the port is
  dropped on all other interfaces. Bench Core sends a typed request and receives
  no broad network-administration capability.
- The administrator API and GUI use existing authorization, session cookie,
  CSRF and same-origin machinery. Their response schema contains interface
  observations and configuration state only, never Wi-Fi passwords, WireGuard
  private keys or other secret material.
- Management updates preserve a last-known-good selection. Empty and unusable
  configurations are refused, removing the browser's current interface requires
  explicit confirmation, and a pending update is restored after its deadline,
  explicit recovery or a restart before confirmation.
- A differing console override requires `--management-reconfigure`, retains the
  saved list as its recovery target, and cannot be staged without a currently
  usable path. Ordinary persisted startup still tolerates temporary total link
  loss and begins serving as soon as an allowed interface returns.
- Existing literal `--bind` deployments are unchanged until interface mode is
  deliberately initialized.
- Production certificates are read and checked, never silently generated or
  replaced. Missing configured DNS/IP SANs produce an actionable warning.

## Automated evidence

| Acceptance behavior | Automated evidence |
|---|---|
| Legacy single bind | CLI validation and existing authenticated simulation serve test |
| `wlan0` and `nilo0` share one instance | management service dual-interface test and listener tests |
| `eth0` rejected before application handling | allowlist listener test, request recheck test and nftables rule test |
| DHCP address changes without restart | provider refresh test |
| Either allowed link may disappear and return | independent Wi-Fi and WireGuard lifecycle tests |
| Restart and simulated reboot | SQLite round-trip, service restart and interrupted-pending recovery tests |
| Empty/unusable list safety | configuration and update refusal tests |
| Current browser warning | backend confirmation test and GUI component interaction test |
| TLS/authentication/CSRF/cookies | SAN tests, administrator-only API test, CSRF refusal test and existing HTTP suite |
| No secrets in API/GUI | response-shape test and UI contract; repository documentation contains no field secret values |
| Applied/pending/failed/recovered states | service recovery tests and GUI component coverage |

Completed local commands:

```text
go test ./... -count=1
go vet ./...
uv run pytest -q
uv run ruff check .
uv run ruff format --check .
npm run format:check
npm run lint
npm run test:ci
npm run build
npm run test:e2e
02_Tools/tests/network/check.sh
```

The Go command passed every package after restoring the ignored, pinned fake
Python integration environment in this new worktree, and `go vet` was clean.
The Python SDK passed 32 tests, the fake integration passed 7 with its one
declared skip, and the integration contract passed 10; Ruff lint and format
checks passed. The GUI suite passed 441 tests in 33 files. The production build
passed with the existing Vite chunk-size warning. The integrated browser run
passed all 38 tests against its real simulation backend. The isolated Debian 13 network harness passed its real
NetworkManager checkpoint/restart, WireGuard lifecycle, nftables forwarding,
authenticated helper, unprivileged HTTPS composition, and management-firewall
tests inside a disposable Docker network namespace.

The repository's optional Windows race branch was not available because the
pinned Go environment reported `CGO_ENABLED=0` and no GCC. No required test was
reported as passed without running it.

After merging the newer login-interface work from remote `master`, the complete
Go suite passed again. GUI lint passed, all 436 current GUI tests passed, and the
production GUI build completed with only the existing Vite chunk-size warning.

## Ubuntu mini-PC deployment evidence

The dual-path candidate at `11f4644a4df251749cff251871c14f8876b63d7e` was
deployed on 2026-09-10 to the documented
`ddp-remote-linux` Ubuntu x86-64 development appliance. The existing
`/var/lib/nilo` state directory, administrator data and certificate were
preserved. Before installation, the previous core binary, helper, service unit,
GUI and state directory were copied to
`/var/backups/nilo/pre-dual-path-11f4644` for local rollback.

The deployed core and helper SHA-256 hashes matched the staged Linux/amd64
artifacts. `nilo version` reported `0.2.0` and protocol
`nilo.integration/1`. `nilo.service`, `nilo-nethelper.service` and
`nilo-nethelper-recovery.timer` were active after installation.

The mini-PC-specific management policy enables `wlp6s0` and `ddp_nilo` on port
8443 and excludes PLC-facing `eno1`. Initial console arguments created the
persistent record; the long-running unit was then changed to
`--management-port 8443` without repeated interface arguments. A service
restart reloaded both saved interfaces and their live addresses from
`/var/lib/nilo`. The helper-owned `inet nilo_management` nftables input chain
accepted established traffic and new TCP 8443 traffic only on `wlp6s0` or
`ddp_nilo`, with a final TCP 8443 drop rule for every other ingress interface.

External LAN verification through
`https://ddp-remote-linux.local:8443` returned HTTP 200. Unauthenticated
requests to `/api/v1/status` and `/api/v1/network/management-access` both
returned HTTP 401. The served JavaScript bundle contained the new Management
access interfaces UI. The unchanged certificate was inspected before
deployment and covers `DNS:ddp-remote-linux.local` and
`IP:10.205.148.11`; NILO did not generate or replace it. Temporary transfer and
extraction files were removed after hash verification.

The development Windows host has no route to `10.205.148.0/24`, and key-based
access to the DCP hub was unavailable. Consequently this deployment verified
that the live listener and firewall include `ddp_nilo`, but did not independently
complete an HTTPS request from a separate WireGuard peer. No host reboot was
performed. Those two checks remain mini-PC follow-up evidence rather than being
reported as passed.

## Raspberry Pi deployment evidence

The dual-path candidate was deployed with explicit authorization on 2026-09-11
to the Raspberry Pi 4 development appliance `ddp-remote`. The existing
`/var/lib/nilo` state, administrator data, TLS material, Wi-Fi profiles and
WireGuard profiles were preserved. A root-only rollback archive was created at
`/var/backups/nilo/pre-dual-path-11f4644-pi-20260911-184541/files-and-state.tar`
before replacing the core, privileged helper, GUI and service unit.

The installed ARM64 core and helper SHA-256 hashes matched the staged build.
`nilo version` reported `0.2.0` and protocol `nilo.integration/1`.
`nilo.service`, `nilo-nethelper.service` and
`nilo-nethelper-recovery.timer` were active and enabled after installation and
again after a Raspberry Pi reboot.

Because `wlan0` and `nilo0` were both offline during deployment, the temporary
development allowlist is `eth0`, `wlan0` and `nilo0`. This preserves a recovery
path over the directly attached development Ethernet link; `eth0` must be
removed from the management allowlist before that interface is reconnected to
the PLC network. The long-running unit supplies only `--management-port 8443`
and reloads the saved interface policy from `/var/lib/nilo`. Both a service
restart and a full reboot restored the three-interface policy. The helper-owned
`inet nilo_management` chain accepted TCP 8443 only on those interfaces and
ended with a drop rule for that port on all other ingress interfaces.

An external HTTPS request through `eth0` returned HTTP 200 before and after the
reboot. Unauthenticated requests to `/api/v1/status` and
`/api/v1/network/management-access` returned HTTP 401. The unchanged
certificate contains `DNS:ddp-remote.local` and `IP:10.205.148.11`; NILO did
not generate or replace it. Temporary relay addressing and the relay copy of
the deployment package were removed after verification. The staged package and
rollback archive remain on the Pi for recovery.

At the end of verification, only `eth0` was connected at `192.168.0.1/24`.
The saved `DDP` Wi-Fi profile remained configured for autoconnect, but no
matching hotspot was available. The saved `nilo0` WireGuard connection was not
active and its NetworkManager autoconnect property was `no`; this deployment
did not silently change that pre-existing setting.

Later on 2026-09-11, the `DDP` hotspot became available. The saved profile
connected as `10.44.118.42/24`, `nilo0` was started at `10.205.148.11/32`, and
the hub reported a current handshake. The Windows WireGuard client at
`10.205.148.7` established TCP 8443 to the Pi and received HTTP 200 from
`https://10.205.148.11:8443`; the protected status API still returned HTTP 401
without authentication.

The Pi GUI was then refreshed from combined branch commit
`04ccc12209c4062855fc28702221f5a0d0f190d6`. Before replacement, the previous
runtime was archived at
`/var/backups/nilo/pre-master-04ccc12-20260911-1954/runtime.tar`. The deployed
JavaScript SHA-256 matched the locally tested production build, NILO returned
HTTP 200 over WireGuard, and the protected status API returned HTTP 401. The
backend and helper hashes were unchanged by the remote `master` merge.

## Raspberry Pi acceptance still required

Preserve console or independent recovery access and complete these checks when
the `DDP` hotspot, a separate WireGuard peer and the PLC are available:

1. The `DDP` Wi-Fi profile reconnects automatically and `wlan0` receives a DHCP
   address without changing NILO's saved configuration.
2. `nilo0` returns as `10.205.148.11/32` with a current WireGuard handshake.
3. A different device on `DDP`, without WireGuard, reaches
   `https://ddp-remote.local:8443` and completes authenticated browser actions.
4. A remote WireGuard peer reaches `https://10.205.148.11:8443` and completes
   authenticated browser actions.
5. The management port is unreachable through PLC-facing `eth0` at
   `192.168.0.1`, while the PLC at `192.168.0.130` remains reachable through the
   approved device-network services.
6. Wi-Fi DHCP renewal, hotspot replacement and `wlan0` disconnect/reconnect do
   not interrupt the VPN path or require a NILO restart.
7. `nilo0` disconnect/reconnect does not interrupt the local Wi-Fi path.
8. The deployed certificate validates for both `ddp-remote.local` and
   `10.205.148.11`, and client trust is correct.
9. A pending management change that is not confirmed restores the last-known-good
   paths, including across service restart.
10. After `wlan0` and `nilo0` are usable, remove `eth0` from the management
    allowlist, confirm the change, and verify that `192.168.0.1:8443` is
    unreachable before reconnecting the PLC.

Record the service logs, interface state, helper-owned nftables table, HTTP
results and non-destructive PLC connectivity results without printing secret
material. Raspberry Pi acceptance is not implied by the local automated tests.
