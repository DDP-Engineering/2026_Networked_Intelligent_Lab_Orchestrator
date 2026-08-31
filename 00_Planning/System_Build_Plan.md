# NILO System Build Plan

**Product:** Networked Intelligent Lab Orchestrator (NILO)
**Status:** Approved Stage 1 plan of record for implementation
**Date:** 2026-08-31
**Relationship to other documents:** `Overview.md` is the long-term product
vision. This document is the Stage 1 architecture and build-order authority.
`Remote_Bench_MVP_PRD.md` defines the first product slice within Stage 1, and
increment implementation plans decompose that slice. Lower-level documents may
narrow scope but may not contradict this plan's architecture or safety boundaries.

## 1. What NILO is

NILO is a Linux appliance (Raspberry Pi 4 first, mini PC later) that gives AI
clients — Claude Code, Codex Desktop, and other MCP clients — controlled, audited
access to physical equipment. A human remains responsible for wiring, safety, and
approval of consequential actions.

NILO serves two product lines from one core:

1. **Electronics bench** — logic analyzers, oscilloscopes, debug probes, flash
   programmers, cameras, relays. The AI collects evidence (captures, decodes,
   logs, measurements), reasons about it, and helps debug embedded systems.
2. **Automation gateway** — the appliance mounted in an automation cabinet as a
   network hub/gateway: monitoring PLCs and industrial devices, providing gated
   remote access for engineering tools, and letting an agent supervise and update
   a remote installation over VPN.

The product is *not* an autonomous robot lab and *not* a raw command proxy. The
AI never gets shell, SCPI, OpenOCD, or arbitrary network access — only typed,
policy-checked operations that produce durable evidence.

## 2. Context and constraints

- **Single developer**, fluent in Python and embedded C, developing with AI
  coding agents (Claude Code / Codex Desktop). The developer must be able to
  read and field-debug everything that matters; the core stays small and boring.
- **Target: a product for external users.** Architecture discipline (strict
  plugin contract, audit, versioned schemas) applies from the first commit.
  Product packaging (installer, appliance image, full GUI) comes late, after the
  system proves itself in daily use.
- **Self-contained development environment.** A public source checkout carries
  pinned, checksum-verified bootstrap and environment scripts in `02_Tools`.
  Downloaded runtimes and caches are ignored by Git; a separate offline
  development-kit archive may include them without bloating source history.
- **Bench hardware available today:** Raspberry Pi 4, Hantek DSO-3062L
  oscilloscope, ATK-DL16 logic analyzer, SEGGER J-Link v9, ST-Link/STM32
  programmers, assorted STM32 targets.
- **Automation hardware available today:** Phoenix Contact ILC 151 ETH and
  ILC 151 GSM/GPRS. These are programmed with PC Worx, which is Windows-only —
  a fact that shapes the gateway feature set (see §5).

## 3. Architecture decisions

### 3.1 Decision log

| # | Decision | Rationale |
|---|----------|-----------|
| D1 | **Bench Core in Go** | Static single binary for the appliance, strong typing and concurrency for the safety-critical session/lease state machine, trivial cross-compilation (linux/arm64, linux/amd64). Go reads close to C, which the developer knows. Mitigation for the unfamiliar language: keep the core small, idiomatic, and covered by table-driven tests the developer can read. |
| D2 | **Plugins in Python, out-of-process** | Python is the native language of every instrument ecosystem in scope (sigrok tooling, pylink/pyocd, pymodbus, opencv) and the developer's strongest language. Out-of-process runners give crash isolation, dependency isolation per plugin, and language freedom later. In-process/dynamic loading is rejected. |
| D3 | **Language-neutral plugin wire protocol** | Versioned, schema-validated NDJSON over stdin/stdout; stderr reserved for bounded diagnostics. Plugins may later be written in any language without touching the core. |
| D4 | **SQLite for persistence** | Single-writer transactional store, zero operational overhead on an appliance, pure-Go driver keeps CGO out of the build. |
| D5 | **MCP is the AI-facing API** | Typed, goal-level tools generated from plugin operation descriptors. The GUI and admin CLI are separate consumers of the same core services. |
| D6 | **Read-only by default; writes are gated** | Safety is enforced by the core and hardware defaults, never by trusting the AI client. See §6. |
| D7 | **Evidence is the product** | Every operation yields durable, addressable results: artifact IDs, raw files (`.sr`, CSV, VCD, logs), structured measurements, and model-friendly summaries/renderings. MCP results are compact and reference artifacts rather than embedding bulk data. |
| D8 | **Gated network access grants are a first-class capability** | Time-boxed, audited tunnels from an engineer's machine through the appliance to exactly one device:port (e.g., PC Worx → ILC 151). Motivated by Windows-only vendor tooling; cheap to build on top of WireGuard + firewall + audit. |
| D9 | **Admin CLI before GUI; approval surface early** | A full configuration GUI is deferred. The one UI piece needed early is a minimal authenticated approval page/notification for gated actions. |
| D10 | **Linux only for now** | Windows-only vendor software is reached through gated tunnels (D8), not by porting NILO to Windows. |
| D11 | **Repository-local build tools** | Go, uv, managed Python, verification entry points, and their version/checksum metadata live under `02_Tools`; scripts change only the launched process environment. Public source clones bootstrap online, while offline development kits carry the populated runtime and dependency cache. |

### 3.2 System shape

```text
Claude Code / Codex ──MCP (HTTP, token)──┐
Admin CLI ───────────────────────────────┼── Bench Core (Go)
Approval page / later GUI ──HTTP─────────┘        │
                                                  │ supervises (NDJSON/stdio)
                              ┌───────────────────┼───────────────────┐
                              │                   │                   │
                        sigrok plugin       jlink plugin        modbus plugin ...
                          (Python)            (Python)            (Python)
                              │                   │                   │
                        scope / LA           J-Link + STM32       ILC 151 etc.
```

The Bench Core owns everything cross-cutting: configuration, authentication and
MCP tokens, policy and approvals, immutable operation requests, sessions,
exclusive device leases, plugin supervision, artifact storage, append-only
audit, and startup reconciliation after a crash. Plugins own only
device-specific behavior behind typed semantic operations (`capture`, `decode`,
`flash`, `read_registers` — never `send_cmd`).

### 3.3 The hard part: sessions and leases

The correctness-critical core is the operation lifecycle:

- immutable, schema-validated operation requests;
- session states: pending approval → queued → running → terminal
  (completed / failed / cancelled / incomplete);
- exclusive per-device leases so conflicting hardware operations cannot race;
- cancellation, timeouts, and safe-shutdown propagated to runners;
- crash/restart reconciliation: interrupted `running` sessions become
  `incomplete`, their leases released, nothing reported as success without
  durable evidence.

This state machine is designed and exhaustively tested against a simulated
plugin before any real instrument is integrated, because the plugin contract
must carry these semantics from day one.

## 4. Plugin contract (summary)

Every plugin package provides:

- a manifest: identity, version, protocol range, supported platforms, runner
  entry point (executable + argument array, never a shell string);
- JSON Schemas for its configuration and for each operation's input and result;
- operation descriptors: stable ID, whether it changes physical state, default
  policy, resource claims, timeout bounds, cancellability;
- lifecycle handlers: describe, discover, configure, health, execute, cancel,
  safe shutdown;
- normalized events: progress, measurements, domain events, warnings, bounded
  logs, artifact declarations;
- contract tests runnable against simulated devices.

The core validates everything a plugin says and everything a client sends; a
plugin crash never takes down the core or another plugin.

## 5. Automation gateway capabilities

Realism about industrial equipment: most PLCs (including the ILC 151) can only
be *programmed* from Windows-only vendor IDEs. NILO therefore provides three
levels of automation access:

1. **Data operations from Linux** — Modbus TCP, later OPC UA: typed read/write
   register operations through a normal plugin, subject to the same policy and
   audit as bench instruments.
2. **Gated access grants (D8)** — an administrator or approved request opens a
   time-boxed, audited tunnel from a named source (the engineer's laptop on the
   VPN) to one destination device:port in the cabinet segment. Vendor tools like
   PC Worx work unchanged; NILO records who reached what, when.
3. **Appliance self-management** — health telemetry, logs, and safe remote
   update of NILO itself, so a remote cabinet installation stays maintainable.

Networking: the appliance joins a WireGuard VPN as an outbound client (usable
behind NAT), binds its services to explicitly configured private interfaces
only, and can route to an isolated PLC segment on a second Ethernet interface
(routed + source-NAT, never bridged). Only explicitly allowed destinations are
reachable; an AI client never gets unrestricted access to an industrial subnet.

## 6. Safety and permission model

- Read-only operations are the default and may run automatically once a device
  is configured.
- State-changing operations (flash, erase, reset, relay/power switching,
  register writes, tunnel grants) are **gated**: they require human approval
  unless the administrator has explicitly whitelisted that operation on that
  device.
- Approvals bind to one immutable request; changing any parameter voids the
  approval.
- Plugins declare per-operation risk; the administrator's policy configuration
  decides; the core enforces. Limits (voltage/current/duration/allowlisted
  firmware paths and targets) are enforced in the core-validated parameters.
- Relay and power defaults must be safe after boot, service failure, or network
  loss.
- Every request, decision, operation, cancellation, and terminal result is
  recorded in an append-only audit log correlated by request/session/device.

The exact policy vocabulary (modes, per-operation overrides, persistence rules)
is specified in detail during Step 2, informed by the simulated plugin.

## 7. Build order

Each step is a permanent increment of the real system — no throwaway demo code.
A step is done when its deliverables work end-to-end, its tests pass on the dev
machine, and the Go core cross-compiles for linux/arm64 and linux/amd64.

| Step | Deliverable | Done when |
|------|-------------|-----------|
| 1. Foundation | Repository-local toolchain bootstrap, Go module, `nilo` binary (version/health), validated config, SQLite store + migrations, structured logging, systemd unit, verification scripts | A fresh checkout bootstraps pinned tools without global installation; skeleton builds and runs on dev machine and Pi; cross-compile checks green |
| 2. Domain core | Operation requests, session state machine, device leases, policy engine, append-only audit, artifact store | Exhaustive state/lease/policy tests pass; restart reconciliation proven |
| 3. Plugin host | Manifest + schema validation, NDJSON protocol, supervisor with crash isolation, Python plugin SDK, one simulated plugin (permanent CI fixture), contract test suite | Simulated plugin completes the full lifecycle incl. approval, cancel, crash, restart |
| 4. MCP + admin CLI | Token-authenticated MCP server (discover/submit/status/cancel/fetch-artifact), admin CLI for setup, tokens, policy | Claude Code and Codex Desktop drive the simulated plugin over the LAN |
| 5. Approval surface | Minimal authenticated web page + notification for pending gated actions | A gated operation submitted by an AI is approved by a human and executes |
| 6. sigrok plugin | Discovery, bounded capture, decode/export, `.sr` + rendered evidence for the supported bench devices | Real capture from AI request to downloadable artifact; device released cleanly on success/cancel/disconnect |
| 7. J-Link/STM32 plugin | Target discovery, verified flash (gated, allowlisted firmware), reset, RTT log streaming as evidence | Flash + RTT session driven by an AI with human approval |
| 8. Gateway | WireGuard enrollment/status, interface binding, PLC segment routing, gated access grants (D8) | Remote AI session over VPN; PC Worx reaches the ILC 151 through an audited time-boxed grant |
| 9. Automation plugins | Modbus TCP plugin; ILC 151 monitoring profile; OPC UA later | Register reads (auto) and writes (gated) against the ILC 151 with full audit |
| 10. Appliance-ization | Installer, Pi image, self-update with rollback, backup/restore; full GUI grows from here | Clean-machine install passes acceptance checks |

Step 6/7 order may swap based on spike (a) below.

## 8. Spikes before/during Step 1

Half-day checks on the actual Pi; (a) is decisive for the Step 6/7 order:

- **(a) sigrok device support:** `sigrok-cli --scan` (and a trial capture) with
  the Hantek DSO-3062L and ATK-DL16 attached, on ARM64. If neither device has a
  working driver, the J-Link plugin becomes Step 6 and sigrok moves behind it.
- **(b) J-Link on ARM64:** SEGGER Linux ARM64 tools + pylink flashing a bench
  STM32 from the Pi.
- **(c) MCP remote transport:** confirm Claude Code and Codex Desktop can each
  consume a token-authenticated HTTP MCP endpoint over the LAN, or identify
  which needs a local stdio bridge.

## 9. Risks and open items

- **sigrok driver coverage** for the specific bench devices is unverified
  (spike a).
- **ILC 151 data access path** (Modbus TCP availability depends on the loaded
  PLC project and firmware; alternatives: web/FTP interfaces, Windows-side OPC)
  — verify during Step 9.
- **Approval latency/UX**: approvals must reach the human where they are;
  notification channel choices are decided in Step 5.
- **Solo-developer Go**: mitigated by small core, strict linting, exhaustive
  readable tests, and agent-assisted development; revisit if the core stops
  being auditable by its owner.
- **Evidence volume on Pi storage**: artifact retention/quota policy needed by
  Step 6.
- **Development-tool redistribution and offline size:** source Git tracks only
  bootstrap metadata and scripts; platform-labelled offline development kits
  must carry third-party notices, verified archives, and dependency caches.
