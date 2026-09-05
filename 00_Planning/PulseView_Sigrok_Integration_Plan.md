# PulseView / sigrok integration and the NILO plugin platform

Planning proposal, 2026-09-05. Prepared outside AEGIS at the user's request.
This document proposes future behavior; it does not mark implementation work
as complete or authorize installation, deployment, or hardware operations.

## 1. Recommended direction

Make **Sigrok Instruments** the first real NILO integration. Use the modified
sigrok stack from `F:\2026_PulseView` for acquisition and decoding, and offer
PulseView as its optional desktop viewer. NILO's browser workbench, CLI, and MCP
tools should all operate through the same Bench Core services.

The product is an extensible electronics and automation appliance: networking
provides access to the bench; integration packages provide its instruments and
software tools. A plugin adds capabilities to NILO, including human and agent
access, rather than merely adding a program launcher.

```text
Human browser GUI       Human / automation CLI       AI client
        |                         |                      |
        +------ authenticated HTTP API ------+       NILO MCP
                                            |           |
                                  Bench Core services
                           identity, policy, operation registry
                           jobs, device leases, artifacts, audit
                                            |
                                 integration supervisor
                                            |
                                   nilo.integration/*
                                            |
                              Sigrok integration runner
                                 /                  \
                        acquisition              offline analysis
                         sigrok-cli         sigrok-cli / typed helper
                              |                  |
                          libsigrok        libsigrokdecode
                              |                  |
                        USB instrument     saved capture

Stored capture ---> NILO browser viewer / download ---> PulseView desktop
```

Keep instrument drivers, decoding details, and vendor commands in their
integration. Keep authorization, ownership, scheduling, storage, and networking
policy in NILO. This also follows the existing [planning overview](Overview.md).

## 2. What exists, and what still needs connecting

The inspected NILO checkout is `e78080e`, with project version `0.1.0`. Source
code takes precedence over older README descriptions of the initial increment.

| Area | Observed today | Work needed for this integration |
|---|---|---|
| Integration boundary | Manifest/schema validation, supervised processes, NDJSON `nilo.integration/1`, Python SDK, fake package, and contract suite exist. | Connect installed packages to the running application and add a real sigrok runner. |
| Operations | Policy resolution, immutable requests, approvals, sessions, leases, evidence, and audit exist as foundation services. | Compose the operation service into the server with a real validator, package registry, evidence adapter, and API routes. |
| Inventory | HTTP inventory is supplied by `simulation.Catalog`. | Persist enrolled devices and merge their configuration with actual runner discovery and availability. |
| Discovery protocol | The SDK's `DiscoverResult` currently returns type/status only. | Define actual device and capability data. Method names alone do not provide discovery. |
| Jobs | Execution runs synchronously under the caller's context; events are collected until execution finishes. | Add server-owned background execution, durable result/progress access, and reconnect semantics. |
| MCP | Exactly three authenticated network reads are registered. | Add integration discovery, capture/analysis and job/artifact access, with authorization per tool and resource. |
| CLI / GUI | Remote CLI patterns and an authenticated browser shell exist; Bench displays inventory. | Add operational commands, capture forms, job views, evidence browsing, and a signal viewer. |
| Storage | Confined, digest-verified artifact storage exists; app composition sets a 64 MiB per-artifact maximum. | Add suitable capture quotas, staging reservations, input-artifact access and bounded waveform retrieval. |
| Plugin lifecycle | The TODO still calls for install, enable, compatibility, update, rollback and removal workflows. | Start with a curated package installed explicitly; add a downloadable catalog after the first integration is proven. |

Source entry points:

- [Application composition](../05_Software/nilo_assistant/internal/app/app.go),
  [operation service](../05_Software/nilo_assistant/internal/operation/service.go),
  [HTTP server](../05_Software/nilo_assistant/internal/httpapi/server.go).
- [Current MCP implementation](../05_Software/nilo_assistant/internal/mcp/server.go),
  [permissions](../05_Software/nilo_assistant/internal/auth/role.go),
  [simulated inventory](../05_Software/nilo_assistant/internal/simulation/catalog.go).
- [Public schemas](../05_Software/schemas/integration/v1/manifest.schema.json),
  [SDK models](../05_Software/integration_sdk/python/src/nilo_integration/models.py),
  [reference package](../05_Software/integrations/fake_python/manifest.json),
  [contract suite](../05_Software/tests/contract/README.md).

### The local PulseView fork

The inspected source checkout is `bcf7e93`. Its delivery directory contains
packages identified as source revision `66540df`, including an ARM64 Debian
package and a self-contained aarch64 AppImage with a `--cli` entry point.
The SHA-256 hashes of those two actual files were checked against the local
`SHA256SUMS` and matched. This is an integrity check, not a fresh execution test.

The fork carries `hantek-dso3062l` and `atk-dl16` drivers. Its build records
describe successful Pi installation, CLI/decoder startup and GUI startup. That
same record explicitly says physical capture on that Pi had not been exercised
because neither instrument was attached. The user reports a tested deployment;
newer physical-capture evidence and the currently installed package are still
to be established. Some build-ledger entries refer to an earlier dirty revision,
so the future integration must record the exact deployed bytes and provenance.

The underlying ATK-DL16 driver also has PWM controls. **The user has explicitly
excluded PWM outputs from NILO:** they must not appear in GUI, CLI, MCP,
capability discovery, or configurable operations. Its discovery uses a USB
port path and does not populate a serial number in the inspected scan function.
These facts affect NILO's capability allowlist and device identity model.

Local evidence is in `F:\2026_PulseView`:

- `README.md`, `07_Delivery/README-RaspberryPi-deb.txt`,
  `07_Delivery/README-AppImage-aarch64.txt`, `07_Delivery/SHA256SUMS`;
- `02_Tools/build/STATUS.md`, especially the aarch64 hardware observations;
- `05_Software/libsigrok/src/hardware/atk-dl16/api.c`;
- `05_Software/sigrok-cli/doc/sigrok-cli.1` and `decode.c`.

## 3. A plugin model that can accommodate other software

Use these concepts consistently:

| Concept | Meaning | Example |
|---|---|---|
| Package | Installable adapter, schemas, runtime dependencies and provenance. | `sigrok` package with the approved DDP runtime. |
| Integration instance | Configured use of an installed package. | Sigrok running on this appliance. |
| Device or target | Enrolled physical resource or configured endpoint. | A particular ATK-DL16, a debug probe, a PLC endpoint. |
| Capability | Typed function supported by that device/backend combination. | Logic capture, UART decode, flash verification. |
| Operation / job | One requested action and its durable execution session. | Capture one million samples using a named analyzer. |
| Artifact | Stored input/output with identity, provenance and access rules. | Capture, decoder results, firmware image, programming log. |
| Presentation | NILO forms/results or a specialist viewer. | Browser waveform, PulseView, a later debugger panel. |

Separate package availability, installation, enablement, compatibility, runner
health, and device presence. An installed package with no attached instrument
should say so; it should not disappear or claim that the instrument is ready.

Each package needs identity, supported host architectures, runtime dependencies,
operation schemas, configuration schema, capability information, limits,
permissions, resource claims, health checks, cleanup behavior, artifact types,
and a deterministic simulation/contract fixture. Keep GUI hints declarative
initially: field labels, units, help, grouping, enum choices and result types.

The existing manifest rejects additional properties. Dependency constraints,
GUI hints, richer capabilities and dynamic resources are **proposed contract
work**, not fields that can be inserted into today's package unchanged. Define
them once for the platform, updating public and embedded schemas, Go types,
Python models and shared fixtures together. Discovery needs matching changes.
Keep `nilo.integration/1` only for genuinely compatible evolution; otherwise
introduce a new protocol deliberately. The development policy calls for direct
replacement of superseded development contracts, without compatibility aliases.

Use one NILO-owned MCP endpoint with capability-specific tools. A plugin does
not need to run another public MCP server. If future vendor software only
provides MCP, its NILO adapter can be a private MCP client while the core still
owns authorization, jobs and evidence.

Start with backend packages plus generic forms/results. Build the first rich
sigrok workbench into the NILO GUI. Independently downloadable custom frontend
code is a later extension: it needs a defined UI contract and isolation from
NILO's authenticated page. Do not make a general frontend extension loader a
prerequisite for the first instrument.

## 4. How to use the sigrok stack

### First adapter

Create `05_Software/integrations/sigrok/`, using the existing Python SDK. Keep
the PulseView source and its build system in its current repository. NILO
consumes a pinned runtime artifact rather than copying that source tree.

For the first vertical slice, launch the verified `sigrok-cli` with an explicit
executable and argument array. Capture bounded data first, then decode the
saved capture. Keep NILO protocol traffic on the runner's stdout; drain child
stdout/stderr separately and bound both. Use argument allowlists and typed
configuration; neither humans nor agents receive an arbitrary sigrok command
string, shell command, decoder search path, or filesystem path parameter.

The fork already supports device scanning, driver information, bounded capture,
saved-file input, decoder selection, sample-number annotations and exports.
Qualify these against the pinned binary rather than assuming all upstream
options and every driver behave alike.

### Structured discovery and decoding

Do not expose raw CLI prose as the long-term NILO API. For the small initial
device/decoder set, use version-pinned parsers with recorded fixtures for
discovery and annotations. Require exact channel mapping, integer sample
positions, decoder/class identifiers, and explicit failures for malformed data.

The existing `--protocol-decoder-jsontrace` is not an adequate canonical event
format: the inspected implementation produces begin/end visualization events,
converts sample indices to floating-point microseconds, and interpolates
annotation text into JSON without escaping. It therefore cannot guarantee
lossless timing or valid JSON for arbitrary annotation content.

Before committing to parser expansion, test multi-device identity, complete
capability discovery, special characters, error annotations and stacked decoders.
If those cannot be represented reliably, add a small native helper alongside
the sigrok fork that uses libsigrok/libsigrokdecode and emits versioned,
properly serialized structured data. Ship it in the pinned runtime and keep
the NILO runner/API unchanged. Do not move C bindings into the Go core.

Annotations and protocol transactions are distinct. Normalize UART bytes,
I2C transfers and SPI frames only where the decoder exposes enough information
and fixtures prove the interpretation. Retain original annotations and mark
unsupported structured fields explicitly. Do not infer missing bytes from
human-readable labels and call them measured data.

### First supported hardware

Recommend the **ATK-DL16 logic analyzer**, subject to its availability, with
UART first, then I2C and SPI. Its capture path proves the common model without
also introducing oscilloscope coupling, vertical ranges and analog calibration.
Support the Hantek through the same package as a later capability increment.

Initially allow finite digital acquisition only. PWM outputs are excluded from
this integration and its planned increments. Filter PWM channels
and configuration keys out of public capabilities and reject attempts to pass
them through any operation or generic configuration route. Use acquisition
settings that leave PWM disabled, and verify that behavior against the driver
and hardware. Continuous acquisition and arbitrary driver settings are also
outside the initial capture operation. Acquisition configures and arms hardware
even when the probes only observe signals; its policy metadata must describe
that accurately.
Discovery itself must be limited to approved drivers/transports because some
drivers open or initialize devices while scanning.

## 5. The same workflow through GUI, CLI and MCP

All names below are proposed interfaces, not commands or tools available today.
Names inside a package use the existing underscore operation convention;
the registry maps package names to NILO's internal integration/device IDs.

| User goal | GUI | CLI example | MCP tool example |
|---|---|---|---|
| Find available capabilities | Integrations and Devices | `nilo integration describe sigrok --json` | `integration_describe` |
| Discover/enroll an analyzer | Scan, identify, name, enable | `nilo device discover --integration sigrok --json` | `device_discover` |
| Acquire signals | Logic Analyzer capture form | `nilo logic capture --device <id> --config capture.json --json` | `logic_capture` |
| Follow or cancel work | Job progress and Cancel | `nilo job get <id> --json`, `nilo job cancel <id>` | `job_get`, `job_cancel` |
| Decode a stored capture | Add UART / I2C / SPI decoder | `nilo logic decode --capture <id> --config decode.json --json` | `logic_decode` |
| Inspect a bounded result | Waveform, table, measurement panel | `nilo logic query --capture <id> --config query.json --json` | `logic_query` |
| Retrieve evidence | Download/export | `nilo artifact download <id>` | `artifact_describe`, bounded artifact reads |

Shared discovery, job and artifact operations belong to the core. Sigrok owns
capture, decode and signal-query semantics. MCP tools call the same application
services used by HTTP; CLI talks to the running server rather than spawning
another hardware-owning backend. A generic typed operation route can support
future packages, while familiar CLI groups and MCP tools improve discoverability.

### Human workflow

1. Open Integrations, enable Sigrok Instruments, and inspect runtime health.
2. Discover the analyzer, confirm its physical identity, and give it a bench
   name. Show model, connection, supported functions and current owner.
3. Open its workbench, choose digital channels, assign signal names, select an
   allowed sample rate, finite sample count and supported trigger.
4. Validate the request, including size estimate and wiring/profile constraints.
   Apply the configured NILO approval policy, then acquire under a device lease.
5. Display job state and elapsed time; report unknown progress honestly when
   the device cannot report a fraction. Cancel uses backend cleanup.
6. Show the stored waveform, add decoders, inspect timestamped events, measure
   a selected region, and download a capture for deeper PulseView inspection.

Initially provide a useful browser viewer: digital channels, pan/zoom, cursors,
decoder overlays or aligned rows, an event table, and artifact download. Serve
bounded time/sample windows and resolution-appropriate summaries. At wide zoom,
indicate transitions/activity instead of decimating samples in a way that hides
narrow pulses. Recreating every PulseView feature is a separate project.

### Agent workflow

An agent discovers the device and limits, submits `logic_capture` with typed
parameters, receives a job ID or pending-approval state, checks that job, decodes
the capture, and queries only relevant time ranges or protocol errors. It gets
structured measurements and evidence references that a human can open in the
same workbench. Raw megabytes do not belong in the model context.

Authorization must cover each tool, target, job and artifact. The current MCP
transport checks only `network.read`; that permission must not grant instrument
execution. Add appropriate scopes and carry authenticated actor identity into
the operation service. Agents must not approve their own pending requests.
Approval binds the exact target, parameters and limits using the existing policy
snapshot/digest model. Ask/Auto/Full Access behavior remains a core decision.

### PulseView's role

The minimum reliable bridge is **download a `.sr` and open it in the matching
PulseView build**. PWM exclusion applies to NILO's integration surfaces; a
separately run, unmodified PulseView has its own controls. NILO must not offer
live device access through that viewer if it would expose excluded PWM controls.
Display the download action accurately: a browser download does not
by itself start an application on the user's computer. A registered local
launcher could improve that later.

For users who need PulseView on the appliance, a later authenticated desktop
session can provide it. The bench's existing RustDesk installation is a
development convenience, not the product's embedded browser interface. A remote
desktop is also not a substitute for typed CLI/MCP capture and analysis.

Initially PulseView should inspect files without direct device access while
NILO owns instruments. NILO leases do not prevent an independently launched
PulseView process from opening USB hardware. A future live desktop mode needs
an explicit handoff: finish/quiesce NILO jobs, reserve the device for that desktop
session, enforce device access, and reclaim ownership after process exit or
session failure. Do not advertise simultaneous control without that mechanism.

## 6. Jobs, device ownership and evidence

### Durable execution

Expose a submitted job immediately; let the server own its execution lifetime
and deadline. A closed browser, MCP request timeout or VPN interruption must
not silently cancel or repeat a capture. Add idempotency keys so a retried
submission resolves to its existing request. Persist terminal results and a
bounded progress/event history for reconnection.

Reuse the existing session states: `pending_approval`, `queued`, `running`,
`completed`, `failed`, `cancelled`, `incomplete`. Stages such as waiting for a
trigger and saving data can be progress metadata. On restart, reconcile work
truthfully; do not automatically repeat hardware actions.

Cancellation requests graceful stop, waits for cleanup, then terminates the
process tree if required. Hold leases until the instrument is released or marked
unavailable for recovery. A cancelled/interrupted capture may have useful partial
data, but it must not become a successful complete capture. Today's evidence
collection on successful completion needs explicit extension for partial data.

### Physical ownership

Use stable NILO device IDs linked to verified physical locators. Prefer a serial
number when available; for the ATK-DL16, use enrollment plus USB topology and
identity checks. Replugging to another port can require remapping. USB bus/address
or the ordinal position in a scan is not persistent identity.

Resolve leases from the selected physical resource in the core, before starting
work and again after an approval delay. Current manifests only contain static
resource strings: dynamic device claims need an explicit implementation. Offline
decoding claims compute/storage capacity and does not lock the acquisition
device. Until device-specific claims are complete, one conservative lock for
all sigrok acquisition is an acceptable first-slice limitation.

The shared model must eventually arbitrate across packages as well: a UART
monitor and an ESP flasher can conflict over the same serial adapter, and two
different debug probes may control the same target board. Add named target
relationships and multi-resource claims when that second integration requires
them; acquire claims atomically in a consistent order.

### Artifact contract

Each capture should record:

- artifact/capture ID, producing job and actor, physical device identity;
- raw data and digest, requested and actual acquisition configuration;
- channel index/name mapping, sample rate with units, actual sample count,
  trigger position when available, start time and clock meaning;
- NILO version, adapter/runtime source revision and digests, driver version,
  decoder IDs/options/versions for each derived result;
- completion status, truncation/overflow indicators, and warnings.

Store `.sr` as the initial portable capture format and verify it opens in the
matching PulseView build. Keep exports and decode results as derived artifacts
linked to the original capture. Use properly serialized JSON/NDJSON for event
data, with integer sample positions preserved exactly across Python, Go and
JavaScript; use decimal strings where numbers could exceed JSON clients' safe
integer range. Derive display time from the recorded sample rate. Never imply
sample-level synchronization between different instruments from wall-clock time.

The runner needs a defined, confined per-job staging directory and read-only
access to authorized input artifacts. Current artifact declarations contain
identity, size and digest, not a complete file-transfer protocol. Specify the
ID-to-staged-file binding and atomic finalization; validate paths, size, digest,
ownership and format before serving an artifact. Decompression and imported
capture processing need limits too. Do not transport waveforms in NDJSON control
messages, whose existing line limit is 1 MiB.

At 16 packed digital channels and 10 MS/s, raw data is about 20 MB/s; ten seconds
is about 200 MB before compression/format overhead. Enabling fewer channels may
not shrink a driver's sample word. Base admission on actual packing and worst
case storage, reserve room for staging plus finalization and derived files,
bound decoding CPU/RAM, and keep management/network services responsive. Start
below the current 64 MiB artifact limit; introduce configurable quotas before
larger captures. Measure throughput on the Pi before promising rate/duration
combinations or moving to continuous acquisition.

## 7. Packaging, networking and operational boundaries

For the first bench trial, reuse the existing `pulseview-ddp` installation after
checking its provenance. The inspected package installs a private runtime under
`/opt/pulseview-ddp` and standard launchers. Its wrapper establishes the bundled
runtime environment, so do not assume invoking an inner ELF binary is equivalent.
The package conflicts with distro `pulseview` and `sigrok-cli` packages.

For repeatable NILO delivery, consume a checksum-pinned runtime payload in a
package-owned location. Preserve its launcher/library/decoder configuration.
Keep the runtime's embedded decoder Python separate from the NILO Python SDK
environment. A later headless runtime bundle can omit Qt/PulseView; it must use
the same driver/decoder source baseline as the optional viewer.

### Installation and deployment with NILO

Support both delivery paths through the same package manager and registry:

- **Bundled with NILO:** the installer or appliance image includes the selected
  integration adapters, manifests, schemas and required redistributable runtime
  dependencies. Sigrok is the first such integration. Record exact package
  digests and platform requirements in the NILO release inventory, and qualify
  that complete bundle together. The device should not need to download sigrok
  on first use when it was included in its NILO delivery.
- **Installed later:** an administrator supplies a compatible package from an
  offline bundle or, once implemented, a trusted catalog. Apply the same
  integrity, dependency, configuration and health checks used for bundled
  packages. An independently installed package has the same operational
  lifecycle and interfaces as a bundled one.

Installation and enablement are separate. Recommend installing bundled
instrument integrations disabled until initial setup explicitly enables them
and enrolls the intended devices. Persist that choice across restarts and NILO
upgrades. An upgrade must not silently re-enable a disabled integration or
restore an optional package that the administrator removed. Installation of
software does not itself grant permission to perform hardware operations.

### Lifecycle operations

Provide one core lifecycle service, surfaced through the Integrations GUI, CLI
and appropriately authorized MCP tools. Package administration requires its own
permissions; permission to capture signals does not grant install or restart
authority. Record lifecycle requests and outcomes in the shared audit history.

| Action | Required behavior |
|---|---|
| Install | Verify and stage package/dependencies, register configuration and capabilities, and leave activation to an explicit enable decision. Failed installation leaves the previous usable state intact. |
| Enable | Validate compatibility, configuration and device permissions; run health checks; allow new operations when ready. Report missing hardware separately from a broken installation. |
| Disable | Immediately block new work, then drain existing jobs or explicitly cancel where supported. Complete cleanup before releasing hardware. Keep package, configuration and evidence. Report disabling while work remains. |
| Restart | Restart the selected integration instance/runner, without restarting NILO. Block new work, drain or safely cancel active work, stop and clean up, start/revalidate, then restore readiness. If recovery fails, remain unavailable and report the reason. Do not enable a disabled integration as a side effect. |
| Uninstall | Disable and quiesce first, then remove registration and package-owned runtime files. Check dependent integrations and shared runtime ownership before deletion. Preserve captures, audit and job history; retain inactive configuration for reinstall unless the administrator separately requests its removal. |
| Update / rollback | Quiesce, stage and validate a compatible candidate, then activate it atomically. Retain the prior package/configuration for rollback. Never replace binaries underneath an active job. |

Enabled does not require a permanently running process: the current supervisor
can launch runners for individual operations. For an idle integration using
that model, restart resets/revalidates its runtime state and health; it does
not invent a resident daemon. Keep desired enablement separate from observed
states such as idle, starting, busy, stopping, or failed.

The lifecycle API must handle repeated requests and concurrent submissions
without admitting new jobs during shutdown. Non-interruptible phases of future
flash/PLC operations can delay disable, restart, update or uninstall; report
the blocking job and its safe completion point instead of treating process
termination as successful hardware cleanup.

Package qualification runs the contract suite before shipment; installation
performs bounded host/runtime health checks without initiating a physical
capture or other hardware-changing test. A checksum detects changed bytes;
authenticating a
publisher for downloadable packages needs signatures or another trusted delivery
mechanism. Review exact vendor redistribution terms for proprietary runtimes
before including them in a NILO image; record the fork's existing license and
source notices with its packages.

Retain NILO's one project-level version in `VERSION` and its required mirrors.
First-party adapters shipped in that release follow that project version;
backend source revisions, vendor tool versions and package digests are separate
provenance. Do not create independently versioned NILO components. Future
third-party package versions and compatibility requirements need a catalog
policy before independent distribution.

Run the sigrok runner without root and grant only the required device access.
The existing USB rules use `plugdev`/`uaccess`; verify access for the actual
service account, not just an interactive desktop user. Process supervision is
not an OS sandbox: production installation must add appropriate filesystem,
device, resource and network restrictions for runners.

Local USB acquisition does not require a public sigrok port or USB-over-IP.
The GUI/CLI/MCP reach NILO through its managed LAN/VPN access, while the runner
remains beside the hardware. Network-connected instruments use enrolled
endpoints and explicit reachability permissions. Privileged route/firewall
changes stay with NILO's network service/helper, not individual plugins.
Account for plugin-originated egress separately from forwarded network traffic.

Later Windows-only vendor software can run behind a narrowly scoped authenticated
bridge on a Windows host, with stable remote device IDs, job status and evidence
transfer. Validate each vendor runtime on ARM64/amd64/Windows before promising
support. Loss of the bridge during flashing must produce an unknown/incomplete
outcome and reconciliation, not an automatic retry.

## 8. Delivery increments and acceptance

These are capability milestones, not assigned release numbers or calendar
estimates. Each milestone should produce a usable, testable outcome.

| Increment | Deliverable | Evidence required to call it done |
|---|---|---|
| 0. Confirm the runtime | Identify Pi model/OS, installed runtime and attached analyzer; exercise finite CLI capture into `.sr`. | Exact package digest/version, real device identity, actual sample count, file readback in PulseView, clean release/reopen. Record separately from prior startup-only evidence. |
| 1. Connect one real operation | Curated package registration, production validator/evidence adapter, server-owned jobs, basic discovery, physical leases, HTTP/CLI/MCP submission and a minimal GUI capture/results form. Start with the fake runner and then sigrok finite capture. | The three interfaces use the same authorization and job path; collision, cancellation, timeout, server restart, disconnect and idempotent retry have truthful outcomes. No large capture quota expansion yet. |
| 2. Useful logic analysis | UART, then I2C/SPI decoding; normalized events; browser waveform/time-window queries; exports and PulseView download workflow. | Known captures decode to expected samples/events; special characters survive serialization; unsupported interpretations are explicit; browser handles larger-than-viewport data without hiding pulses. |
| 3. Qualify the appliance | Controlled ARM64 installation, bundled and later-installed packages, service permissions, resource limits, enable/disable/restart/uninstall/update/rollback and fault recovery. | Reproducible hardware tests, real capture performance, unplug/replug, full disk, no trigger, process failure and network responsiveness during load; lifecycle operations preserve evidence and persisted enablement, including through NILO upgrades. |
| 4. Prove reuse | Add one narrow second family, preferably verified flashing on a disposable development target or read-only CAN reception. | Common job/permission/resource/artifact machinery is reused; the second adapter needs no vendor-specific behavior in the core. |
| 5. Broaden the ecosystem | More drivers, analog scope functions, additional tools, package catalog, optional specialist desktop/remote bridges, and later workflows. | Per-device/per-platform qualification and compatibility records; downloadable package trust and UI extension contracts precede open third-party installation. |

Do not hold the first useful integration for a marketplace, continuous live
streaming, complete PulseView browser parity, or a universal workflow editor.
The first end-to-end result is a real finite capture requested through NILO,
stored durably, and inspectable by both a human and an agent.

### Verification strategy

Follow the existing [development topology](../06_SoftwareDocu/Development_and_Simulation_Flow.md):
Windows browser/tools, WSL2 backend/SQLite/runners, deterministic simulated
devices, and runtime state on the WSL Linux filesystem. Use saved signal fixtures
for software tests. Repository-owned setup and verification scripts go under
`02_Tools/` and use pinned local tools.

Exercise the common integration contract and real core services first. Add
meaningful failure cases: identical analyzers, moved ports, absent driver or
decoder, permission denial, busy device, invalid channels/rate, unsupported
trigger, no-trigger deadline, malformed decoder output, full storage, crashed
runner, cancellation during saving, partial files, and unauthorized artifact
reads. Verify PWM is absent from discovery/forms/tools, PWM-related parameters
are rejected by the backend, and acquisition does not activate PWM outputs.
Show approval, failure and recovery states in GUI/CLI/MCP parity tests.

Use the device from [Local Hardware Access](../06_SoftwareDocu/Local_Hardware_Access.md)
for early ARM64 and physical-I/O observations. Resolve its current address from
its documented identity rather than hardcoding a historical IP. That disposable
bench Pi is not the controlled release hardware gate. Normal tests must remain
usable without contacting it. Release hardware evidence needs a reproducible
appliance configuration, known signal source and recorded wiring.

### When to connect the ATK-DL16

Connect the analyzer by USB to the network-accessible bench Pi early, before
the first real-adapter implementation is considered complete. USB alone permits
enumeration, stable-identity and service-account permission checks. A finite
capture from a known signal source then checks the acquisition path, timing,
data format and decoder assumptions. Wiring the signal source is a separate
human step; keep PWM outputs unused and excluded.

Use these early observations to resolve driver and ARM64 assumptions while
the adapter is still small. Continue simulation and saved-capture development
when the hardware is unavailable. At final testing, repeat acquisition,
decoding and failure recovery through the full GUI/CLI/MCP path on the controlled
appliance configuration. Connecting the bench Pi early does not replace that
final qualification or make normal software tests depend on its availability.

## 9. How later integrations fit

| Family | Specialist adapter/backend | Reused NILO features | Additional contract concerns |
|---|---|---|---|
| CAN analysis | SocketCAN or a qualified vendor analyzer runtime | Device enrollment, capture jobs, artifacts, GUI traces, typed MCP results | Bus/channel identity, receive versus transmit, frame timing, bus-off and DBC provenance. A sigrok CAN decoder over sampled pins is different from a CAN interface. |
| ESP programming | A qualified esptool runtime | Target selection, firmware artifacts, approvals, progress, logs | Boot/reset control, serial ownership, image/target match, flash verification, interrupted writes. |
| STM32 programming | Qualified STM32/OpenOCD/probe tooling | Same programming workflow and evidence | Backend-specific support, SWD/JTAG and reset ownership, readout protection, erase scope. |
| SEGGER J-Link | Qualified vendor runtime, local or bridged | Probe inventory, jobs, artifacts, access policy | Runtime licensing/distribution, target support, RTT/debug sessions, safe cancellation phases. |
| PLC software | Vendor/service adapter near the PLC | Managed endpoint access, approvals, audit and file evidence | CPU identity, machine state, download/run/stop semantics and verification. |
| SCPI instruments | SCPI/VISA adapter | Typed configuration, measurements, leases and evidence | Units, instrument limits, output state, timeouts and safe shutdown. |

A capability can be a finite job, a long-lived session or a managed background
service. Prove finite jobs with sigrok first; introduce session/service lifecycle
extensions when CAN monitoring, RTT or PLC communication needs them. Flashing
must be able to declare phases where immediate cancellation cannot leave a
known safe state. A universal process kill is not a universal hardware cleanup.

Add multi-step workflows only after two families are proven. A future
flash-reset-capture-decode workflow references existing operation contracts,
artifacts and resource claims; it should not embed a free-form shell script.

## 10. Recommended next scope and remaining decisions

The next implementation scope should be **one ATK-DL16 finite capture, with a
durable NILO job, basic GUI/CLI/MCP access, a verified `.sr` artifact and optional
offline UART decoding**. Expand I2C/SPI and browser analysis in the next usable
increment. If the analyzer is unavailable, retain the same contracts and start
with deterministic saved captures while arranging real hardware qualification.

Resolve these before the work they affect:

- Confirm the first physical instrument and a known signal source; ATK-DL16 is
  the recommendation, not a claim that it is currently attached.
- Verify the Pi's installed runtime and actual capture behavior. The local
  delivery files have verified hashes; the device was not contacted for this plan.
- Run the focused discovery/decoding spike to choose whether pinned parsers
  suffice or the native structured helper is required.
- Measure RAM/storage throughput to set initial acquisition limits; choose a
  runtime digest from verified evidence, not whichever source HEAD is newest.
- Confirm whether file-based PulseView inspection satisfies the first release.
  Live desktop control would add device handoff and desktop-session work.

## Upstream references

Upstream establishes the component boundaries; the local fork is the authority
for its added drivers, runtime and output behavior.

- [PulseView](https://sigrok.org/wiki/PulseView) describes the Qt graphical frontend.
- [sigrok-cli](https://www.sigrok.org/wiki/Sigrok-cli) documents acquisition,
  file-based processing and decoding, and recommends separating capture from
  processing for demanding acquisition.
- [libsigrokdecode](https://sigrok.org/wiki/Libsigrokdecode) describes the decoding library.
- [PulseView manual](https://www.sigrok.org/doc/pulseview/0.4.2/manual.pdf),
  chapter 8, documents opening saved captures and session settings from the CLI.
