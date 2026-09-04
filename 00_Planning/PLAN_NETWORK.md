# NILO Network Service — Implementation Plan

**Intended document:** `F:\2026_Networked_Intelligent_Lab_Orchestrator\00_Planning\Network_Service_Implementation_Plan.md`

**Status:** Implementation in progress under `TASK-943403e2`; the current audit and remaining work are recorded in [Network service implementation status](../06_SoftwareDocu/Network_Service_Implementation_Status.md).

**Planned extension (2026-09-03):** [Device networks and audit: Claude Code implementation plan](PLAN_DEVICE_NETWORKS_AND_AUDIT.md) defines the next scope: configurable interfaces, generic local/VPN device access, duplicate-site address translation, complete live WireGuard profiles, and per-NILO audit/diagnostics. Its [Claude Code entry prompt](CLAUDE_DEVICE_NETWORKS_AND_AUDIT_PROMPT.md) starts the handoff. This is planned work, not a claim that this document's excluded features are already implemented; the extension reconciles those exclusions before implementation.

## 1. Outcome and agreed scope

Deliver Network as NILO’s first complete managed service, available through the GUI and CLI, with read-only MCP access.

The service will provide:

- LAN/Wi-Fi interface status, statistics, enable and disable.
- WireGuard installation detection and administrator-triggered installation.
- Import and management of multiple named WireGuard profiles, with **one NILO-managed tunnel active at a time**.
- VPN enable/disable and an explicit **Connect at startup** option.
- **24-hour persisted traffic statistics**.
- Administrator acknowledgement before changes, with **automatic rollback after two minutes** for connectivity-changing actions unless an administrator confirms the result.

LAN/Wi-Fi toggles affect runtime connectivity; they do not rewrite saved IP, DHCP or Wi-Fi credentials. Reboot follows the existing operating-system configuration. VPN profiles persist, but autostart is off by default.

Excluded: Wi-Fi scanning and credential configuration, static-IP editing, DHCP servers, PLC routing/NAT, firewall management, VPN-server provisioning, other VPN providers, and instrument operations.

## 2. Backend, platform and safety

### Shared service and live execution

- Add one built-in Network service used by HTTP, CLI and MCP. Network remains a core capability, not a plugin.
- Keep the existing simulation profile entirely simulated: enabling an interface, installing WireGuard or activating a profile there must never change the host.
- Add an explicit Linux `appliance` runtime profile for real networking. Instrument integrations remain disabled in this milestone; do not populate live inventory with simulated devices.
- Support Debian 13/Raspberry Pi OS with systemd and NetworkManager for live management. Other network managers remain observable where possible but report management as unsupported.
- Use NetworkManager’s D-Bus API for managed Ethernet, Wi-Fi and WireGuard connections. Do not mix NetworkManager and `wg-quick` ownership of the same tunnel.
- Add same-origin serving of the built GUI and TLS for explicitly configured private listeners. Preserve loopback-only development defaults; do not introduce public listeners or implicit TLS bypasses.

### Privileged operations

Run Bench Core without root privileges. Provide a narrowly scoped, root-owned Network helper:

- Reachable only through a restricted Unix socket with caller-identity checks.
- Accepts typed, allowlisted operations—not arbitrary commands, paths or shell scripts.
- Owns privileged network changes, protected profile storage, installation and recovery.
- Installed explicitly through repository tooling under `02_Tools`; never silently installed by application startup.
- Reports unavailable capabilities clearly when absent or unsupported.

Interface disable means disconnecting that device and preventing automatic reconnection during the current runtime. Enable restores its previous connection, or requires selection of an existing compatible saved connection if none is known. It does not create credentials or globally switch every Wi-Fi adapter’s radio off. Loopback and unsupported infrastructure interfaces are not controllable.

### Administrator permissions and acknowledgement

Add fixed permissions:

- `network.read`: status and statistics; available to all existing roles.
- `network.manage`: interface controls; Administrator only.
- `vpn.manage`: profile import/removal, activation and autostart; Administrator only.
- `vpn.install`: install WireGuard tooling; Administrator only.

Enforce permissions server-side, including the current owner role and token scopes. Disabling or demoting an administrator invalidates their ability to complete pending changes. MCP exposes no mutation tools, even for an administrator token.

For every mutation, present the exact target and proposed action before acknowledgement. Bind acknowledgement to that action and its current revision; reject stale previews and duplicate execution. Audit the actor, action, target and outcome without recording secrets.

### Connectivity rollback

For interface toggles and VPN activation, deactivation or switching:

1. Validate and preview the proposed change.
2. Receive administrator acknowledgement.
3. Establish a recovery checkpoint before changing connectivity.
4. Apply the change and start a **120-second confirmation window**.
5. Keep the change only after an explicit, authenticated confirmation.
6. Otherwise restore the previous connection, routing and DNS state.

Use NetworkManager checkpoints for timed restoration, with helper-owned recovery state independent of the browser and Bench Core. NetworkManager provides an automatic rollback mechanism for this purpose. [NetworkManager checkpoint API](https://networkmanager.pages.freedesktop.org/NetworkManager/NetworkManager/gdbus-org.freedesktop.NetworkManager.html)

Allow only one connectivity change at a time. Refuse an operation if recovery cannot be armed. Persist enough recovery information to reconcile an interrupted change after helper/service restart or reboot. Never report “restored” without verifying the observed state; expose recovery failure prominently.

### WireGuard installation and profiles

- Check NetworkManager availability, kernel support, installed tools and their versions.
- Install only the required WireGuard tooling through the supported distribution’s signed package repositories, after administrator acknowledgement.
- Do not upgrade the distribution, replace the kernel, reboot automatically, install another network manager, or replace DNS tooling.
- Return asynchronous installation progress and a verified final result. An already-installed result is idempotent; package-lock and offline failures are actionable errors.
- Import standard `.conf` profiles into NILO-owned NetworkManager connections, initially inactive and without autostart.
- Support IPv4/IPv6, split/full tunnels, DNS, MTU, endpoints, peer keys, allowed addresses and keepalive settings.
- Reject executable hooks and unsupported directives explicitly. Do not execute imported configuration as a script: `wg-quick` profiles can contain shell hooks. [WireGuard configuration semantics](https://raw.githubusercontent.com/WireGuard/wireguard-tools/master/src/man/wg-quick.8)
- Store private and preshared keys only in protected root-owned profile storage. Never return them through status, logs, audit, MCP or profile downloads.
- Permit removal of inactive NILO-owned profiles. Require deactivation before removing an active profile.
- Treat externally managed tunnels, including the bench’s existing `wg0`, as read-only. Do not silently adopt, stop or overwrite them.
- Support one startup profile. Changing the startup selection is explicit; runtime VPN disable does not silently erase that setting.

## 3. GUI, statistics and public interfaces

### Network page

Keep Network in the lower core-navigation cluster and reuse the approved shell, themes and English/German/Romanian translations.

Provide three compact areas:

- **Interfaces:** Ethernet/Wi-Fi identity, addresses, connection state, available link capacity, RX/TX rates and administrator controls.
- **WireGuard:** installation readiness, named profiles, active tunnel, handshake information, import, enable/disable and startup selection.
- **Statistics:** selected-interface traffic chart with 15-minute, one-hour and 24-hour views.

Use confirmation dialogs and a visible rollback countdown with **Keep change** and **Restore previous state** actions. Viewers and Operators see permitted telemetry without management controls.

The top-row indicator uses the same Network data as the page. Keep these facts separate:

- Interface connection state.
- Tunnel activation.
- Last successful WireGuard handshake.
- Browser reachability of Bench Core.

Do not present an activated tunnel or recent handshake as proof that a remote PLC is reachable.

### Statistics

- Sample counters every five seconds; calculate rates using elapsed monotonic time.
- Persist one-minute aggregates in SQLite and retain a rolling 24 hours.
- Store traffic counts and available error/drop counters, not packet contents.
- Preserve history across application restarts.
- Start a new measurement interval after counter resets or device replacement; missing samples remain gaps, not zero traffic.
- Identify the measurement source explicitly: simulated environment or actual backend host.
- Bound history responses and mark stale observations. Refresh failure must not leave old values appearing current.

### HTTP and CLI

Add a versioned `/api/v1/network` API family covering status, statistics, WireGuard readiness, safe profile metadata and typed management operations.

Management operations return an operation identifier with progress and final outcome. Provide operation lookup, confirmation and rollback routes. Use the existing authentication, CSRF, error and audit conventions.

Implement this CLI surface:

```text
nilo network status
nilo network statistics --interface ID --period 15m|1h|24h
nilo network interface enable ID
nilo network interface disable ID

nilo network vpn check
nilo network vpn install
nilo network vpn profiles list
nilo network vpn profiles import --file PATH --name NAME
nilo network vpn profiles remove ID
nilo network vpn enable ID
nilo network vpn disable
nilo network vpn autostart ID --enabled true|false

nilo network operation status ID
nilo network operation confirm ID
nilo network operation rollback ID
```

CLI commands call the same authenticated API, including when used locally. Support `--server`, `--token-file`, `--ca-file` and `--json`; never accept token secrets as command-line arguments.

Interactive mutations require confirmation. Noninteractive execution requires an explicit acknowledgement flag, but it cannot bypass the connectivity rollback window. A post-change confirmation is always a separate action.

### Read-only MCP

Implement `/mcp` using the official Go SDK and Streamable HTTP. Pin the selected SDK version in the repository. [Official Go MCP SDK](https://github.com/modelcontextprotocol/go-sdk)

Expose only:

- `network_get_status`
- `network_get_statistics`
- `network_get_wireguard_status`

Use existing scoped NILO bearer tokens and require `network.read`. Validate each request, including token revocation and Origin handling. Return bounded structured results without private keys or raw profiles.

This milestone supports clients that can supply a configured bearer token; OAuth discovery and interactive OAuth login are not included. Mark tools read-only, but enforce that property in their implementations rather than relying on annotations.

## 4. Implementation sequence

Complete these checkpoints sequentially on the current branch, without committing or pushing:

1. **Contracts and authorization:** update TASK-556d32bb from monitoring-only to the agreed scope; define requirements, API/CLI/MCP contracts, permissions and deterministic scenarios.
2. **Telemetry and history:** implement the shared service, Linux reads, simulation provider and 24-hour storage migration.
3. **Managed operations:** implement the privileged helper, rollback/recovery, WireGuard installation and profile lifecycle.
4. **User interfaces:** connect the Network page, compact indicator, administrator dialogs and CLI.
5. **Live runtime and MCP:** enable the explicit Linux runtime, private TLS serving and authenticated read-only MCP.
6. **Verification and documentation:** complete automated checks and opt-in Raspberry Pi testing; update operational guidance and the changelog.

Each checkpoint must leave its covered behavior runnable and tested. Keep scripts focused on setup and test execution; do not turn them into a general process-management framework.

## 5. Verification and completion criteria

### Automated checks

Cover:

- Administrator versus Operator/Viewer access, narrowed tokens, revoked credentials and CSRF.
- Acknowledgement rejection, stale requests, duplicate submissions and concurrent changes.
- Confirmation, timeout rollback, client disconnect, service/helper restart, reboot recovery and rollback failure.
- Profile validation, prohibited hooks, secret redaction, single-active-tunnel enforcement and autostart.
- WireGuard already installed, missing, unsupported, installation failure and package-manager contention.
- Counter reset, missing devices, stale readings, restart continuity and 24-hour expiry.
- GUI themes, languages, keyboard navigation and bounded scrolling.
- CLI output/exit behavior and MCP discovery, structured responses, authorization and absence of write tools.

Run deterministic browser tests against WSL2 simulation, plus disposable Linux system tests with NetworkManager. Do not disable the developer’s real WSL network during ordinary tests. Verify ARM64/AMD64 builds and preserve existing portability checks.

### Raspberry Pi bench testing

Use the device documented in `06_SoftwareDocu/Local_Hardware_Access.md`:

- Rediscover and verify `ddp-remote` before testing; do not assume its recorded IP addresses are current.
- Record its OS, NetworkManager, WireGuard, interfaces and initial connectivity.
- Preserve the existing `wg0` profile, keys, DNS setup and unrelated services.
- Test Ethernet and Wi-Fi one at a time while retaining a verified alternative access path.
- Use temporary WireGuard profiles and disposable test peers, not the existing external VPN credentials.
- Exercise acknowledged changes, confirmation, deliberate timeout recovery, profile switching, DNS restoration and traffic statistics.
- Test installation from a missing-package state in a disposable Linux environment; do not uninstall the Pi’s existing WireGuard merely to reproduce that case.
- Restore the bench baseline and remove only task-created test resources afterward.

These are recorded **real-hardware bench observations**, not a mandatory clean-clone repository gate or production certification.

**Done means:** an administrator can check/install WireGuard, import a profile, safely control interfaces and VPN connectivity through GUI and CLI, inspect persistent statistics, and read the same status through MCP; non-administrators cannot mutate networking; unconfirmed connectivity changes recover automatically; and the working behavior has both automated evidence and documented Raspberry Pi bench results.
