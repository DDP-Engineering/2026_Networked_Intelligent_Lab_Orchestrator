# Dual-path NILO management access verification

## Candidate and scope

This report covers the dual-path management-access candidate in workspace
`F:\NILO_dual_path_management_access`, branch
`ddp/nilo-dual-path-management`, based on local `origin/master` commit
`ef561f21fe249dc0a698e20160ae015eddf89922`.

An attempted remote refresh was refused by the configured SSH remote, so the
candidate's base could not be compared with the server's current `master`.
Nothing was pushed. No live Raspberry Pi, DCP server, WireGuard peer, Wi-Fi
profile, PLC or production certificate was modified.

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

## Raspberry Pi acceptance still required

Deployment requires explicit authorization after reviewing this candidate. On
the Pi, preserve console or independent recovery access and verify:

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
10. A full Raspberry Pi reboot preserves both enabled interfaces, the single
    state directory, helper firewall enforcement and PLC communication.

Record the service logs, interface state, helper-owned nftables table, HTTP
results and non-destructive PLC connectivity results without printing secret
material. Raspberry Pi acceptance is not implied by the local automated tests.
