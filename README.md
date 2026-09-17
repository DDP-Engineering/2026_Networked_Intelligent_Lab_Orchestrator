# NILO — Networked Intelligent Lab Orchestrator

**An electronics bench appliance for humans and AI agents.**

NILO brings instrument access, network management and electronics tools into
one application. Work through a browser GUI, automate through the CLI, or give
an AI agent access through MCP. Shared services manage permissions, approvals,
queued operations and captured evidence across these interfaces.

Plugins extend the appliance with specialist tools. The first integration uses
sigrok for logic-analyzer capture and decoding; the longer-term direction
includes CAN analysis, embedded debugging, firmware flashing and PLC tooling.
Installed and enabled plugins are configured per appliance.

[Quick start](#quick-start) · [Documentation](#documentation) ·
[Roadmap](#roadmap) · [Changelog](CHANGELOG.md)

## Project status

NILO is in active development, with Raspberry Pi ARM64 as the first appliance
target. The repository includes a working browser interface, authenticated CLI
and MCP endpoints, managed networking, and an initial sigrok integration.
Available commands and device support depend on the installed integration.

The current sigrok adapter supports ATK-DL16 discovery, finite digital captures
and UART decoding of saved captures. A full PulseView-style browser waveform
workspace is **planned**, not yet implemented. PWM outputs are excluded.

The project uses one shared version, declared in [VERSION](VERSION). See the
[changelog](CHANGELOG.md) for delivered changes. Simulation and bench smoke
tests do not establish electrical accuracy or production hardware acceptance.

## Capabilities

| Area | Available today |
|---|---|
| Browser interface | Tabbed workspaces, responsive layouts, contextual help, light/dark/system themes, English/German/Romanian |
| Human and agent access | Browser sessions, CLI and MCP tools backed by shared authorization and services |
| Access control | Administrator, Operator and Viewer roles; scoped API tokens; operation approval policies |
| Plugin management | Curated bundles with explicit install, enable, disable, restart and uninstall operations |
| Instrument operations | Durable transaction IDs, bounded queues, device ownership, cancellation, deadlines and retained artifacts |
| Networking | Adapter configuration, exclusive management/device roles by stable hardware identity, Wi-Fi scan/join, managed WireGuard profiles and local/VPN device access through the Linux helper |
| Activity | Filtered audit history, export and retention controls |

For exact commands, limits and permissions, use the linked guides below.

## How it fits together

```text
Browser GUI          NILO CLI          AI agents via MCP
     \                   |                   /
      +---------- Shared Bench Core --------+
                  Identity and policy
                  Jobs, leases and audit
                  Capture artifacts
                    /           \
          Plugin supervisor    Linux network helper
                  |                     |
          Instrument tools       Appliance networking
```

The Go Bench Core owns orchestration and persistence. Integrations run as
supervised external processes; the current integration SDK is Python. The
TypeScript browser interface connects to the same appliance services used by
automation clients. Privileged network changes are isolated in a Linux helper.
On an appliance, one HTTPS Bench Core may serve several explicitly allowed
management interfaces. A connection is admitted by interface-aware socket and
request gates and by a helper-owned nftables allowlist; the listener is never
made generally reachable merely by binding a wildcard address. PLC-facing
interfaces remain excluded unless an administrator deliberately enables them.

## Quick start

The default development setup uses a **Windows browser GUI with the backend
and integration runners under WSL2 Linux**. Start from the repository root with
an initialized WSL2 distribution:

```powershell
# Prepare the pinned Windows and WSL2 development tools.
.\02_Tools\bootstrap\bootstrap-development.ps1

# Start the backend and browser interface in simulation mode.
.\02_Tools\scripts\nilo-dev.ps1 start
```

Open the browser URL printed by the launcher and sign in with:

| Development username | Development password |
|---|---|
| `admin` | `admin_password` |

These credentials are a simulation-only development convenience. An appliance
uses its own administrator setup. The bootstrap does not install or convert
WSL unless explicitly requested; see [tool setup](02_Tools/README.md) for that
option and distribution selection.

For manual startup, browser tests and simulation scenarios, see the
[development guide](06_SoftwareDocu/Development_and_Simulation_Flow.md).

## Build environment

There are two development paths, and they serve different purposes.

| Path | Use it for | Entry point |
|---|---|---|
| Host-local interactive development | Day-to-day coding: run the app, iterate on the GUI, use a headed browser | `.\02_Tools\bootstrap\bootstrap-development.ps1` then `.\02_Tools\scripts\nilo-dev.ps1 start` |
| Reproducible OCI build/test boundary | The build and test result that counts: lint, unit, contract, cross-architecture and headless browser gates | `.\02_Tools\bootstrap\bootstrap-build-environment.ps1` then `.\02_Tools\env\with-build-environment.ps1 test` |

Use the host-local path while you are working. Use the container path before
you rely on a result, because it is the same locked Linux image on every
supported host and it runs offline.

The container path needs **Git and Git LFS on the machine before you clone**:
the multipart image archive is stored through Git LFS, and setup deliberately
never contacts a forge itself. Native Ubuntu 24.04 x86-64 uses the matching
`.sh` entry points.

Commands, recovery paths, export and publication are in
[repository tools](02_Tools/README.md).

## Appliance deployment

Linux ARM64 is the first release target; Linux x86_64 is the additional
appliance target. Windows is a development host, not an appliance platform.

Appliance startup uses HTTPS, a dedicated service account, explicit
administrator initialization and either the compatible legacy literal `--bind`
mode or a persisted management-interface allowlist. Network and management
firewall changes require the separately installed privileged helper; Bench Core
remains unprivileged. Sigrok bundles also require a compatible target runtime
and backend.

- [Appliance setup and network operations](06_SoftwareDocu/Network_Operations.md)
- [Sigrok bundle preparation and deployment](06_SoftwareDocu/Sigrok_Integration.md)
- [Delivery documentation](07_Delivery/README.md)

## Documentation

| Guide | Purpose |
|---|---|
| [Find your NILO](06_SoftwareDocu/Find_Your_NILO.md) | Plain-language appliance names and ways to find NILO on common networks |
| [Development and simulation](06_SoftwareDocu/Development_and_Simulation_Flow.md) | Daily workflow, environments and verification levels |
| [Repository tools](02_Tools/README.md) | Bootstrap, pinned toolchains, build and diagnostic commands |
| [CLI and authentication](07_Delivery/NILO_CLI_and_Authentication.md) | Command-line access, identities, roles and tokens |
| [Network operations](06_SoftwareDocu/Network_Operations.md) | Network workflows, recovery, isolated tests and appliance setup |
| [Sigrok integration](06_SoftwareDocu/Sigrok_Integration.md) | Current operations, limits, shared interfaces and packaging |
| [Code style](01_Documentation/Code_Style.md) | Production-code and test conventions |

## Roadmap

The next major instrument-interface work is a native browser waveform workspace
with capability-driven device settings, triggers, digital and analog traces,
decoder tracks, named saved captures, markers, measurements and search.
Semantic operations will also be available through CLI and MCP where useful.

The [waveform workspace plan](00_Planning/Sigrok_Waveform_Workspace/README.md)
contains the PulseView reference analysis, proposed contracts and 15
implementation work packages. Broader product direction is in
[Planning](00_Planning/README.md).

Future plugin families include CAN analyzers, J-Link/STM32/ESP flashing and
debugging, and PLC automation tools. These are roadmap items, not installed
capabilities of the current application.

## Repository layout

```text
00_Planning/       Product direction and upcoming work
01_Documentation/  Project conventions
02_Tools/          Setup, build, verification and deployment tools
03_Hardware/       Hardware assets
04_HardwareDocu/   Hardware documentation
05_Software/       Go core, browser GUI, integrations and tests
06_SoftwareDocu/   Current operational and developer guides
07_Delivery/       Delivery and user-facing documentation
```
