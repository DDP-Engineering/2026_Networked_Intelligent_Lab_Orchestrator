# Development and Simulation Flow

## 1. Purpose

This document defines the default NILO development and simulation environment
used before deployment to a Raspberry Pi.

The approved development topology is:

- the browser GUI and its development tooling run natively on Windows;
- Bench Core, its HTTP and MCP services, SQLite, and integration runners run
  under WSL2 Linux;
- electronics, instruments, PLCs, programmers, and failure conditions are
  represented by deterministic simulated integrations;
- a Windows browser connects to the WSL2 backend through a loopback development
  endpoint.

This arrangement is a development environment, not an additional product
deployment target. NILO appliances remain Linux systems, with Raspberry Pi 4
Linux ARM64 as the first MVP target.

## 2. Goals and boundaries

The flow is intended to provide:

- a fast native Windows GUI edit, reload, and browser-debugging loop;
- Linux behavior for the backend, integration runners, paths, permissions, and
  process supervision;
- repeatable end-to-end tests without physical equipment;
- the same authenticated API, policy, approval, session, artifact, and audit
  paths used by a real appliance;
- a clear route from desktop simulation to VM, ARM64, and physical-device
  acceptance.

It does not attempt to prove:

- USB, serial, GPIO, or other physical I/O behavior;
- Raspberry Pi timing, thermal, storage, or power characteristics;
- ARM64-native dependency compatibility;
- udev rules or real device discovery;
- electrical safety behavior or physical interlocks;
- successful flashing or debugging of actual equipment.

Those properties require later Linux VM, ARM64, and hardware-in-the-loop gates.

## 3. Development topology

```text
Windows 11 development host

  Windows browser
       |
       | HTTP on an explicitly configured loopback development port
       v
  WSL2 Linux x86_64
    Bench Core
    HTTP API and MCP server
    authentication, policy, approvals, sessions, audit, artifacts
    SQLite database
    integration supervisor
       |
       | nilo.integration/1
       v
    simulated Python integrations
      simulated instruments, PLCs, programmers, artifacts, and failures

  Windows TypeScript toolchain
    GUI development server
    component tests
    browser end-to-end tests
```

During early GUI work, the Windows GUI may also use a contract-validated mock
API. Integration and end-to-end testing must connect the same GUI to the real
WSL2 Bench Core.

## 4. Responsibility by environment

### 4.1 Windows host

Windows owns:

- editing the shared repository;
- the TypeScript GUI development server and fast reload loop;
- browser developer tools;
- GUI unit and component tests;
- mock API scenarios;
- browser end-to-end test orchestration;
- presentation of the application in the user's normal browser.

The frontend build toolchain must be version-pinned and bootstrapped through
`02_Tools/` when it is introduced. A system-wide Node.js or other JavaScript
runtime must not become an undocumented prerequisite.

Windows is not the production backend environment. Windows portability builds
remain useful, but production-only Linux behavior must not be weakened to make
Windows a NILO appliance target.

### 4.2 WSL2 Linux

WSL2 owns:

- the long-running Go Bench Core process;
- the HTTP and MCP service implementations;
- SQLite persistence;
- Python integration runners;
- the integration supervisor and process lifecycle;
- simulated devices and operations;
- backend unit, contract, integration, and acceptance tests.

Runtime data must be stored on the WSL Linux filesystem, not under `/mnt/c`,
`/mnt/f`, or another Windows-mounted filesystem. This provides more realistic
Linux filesystem semantics and avoids placing a live SQLite database on DrvFS.
The runtime data location must be separate from source code and ignored build
outputs.

### 4.3 Shared source

The Windows repository at
`F:\2026_Networked_Intelligent_Lab_Orchestrator` is visible from WSL as
`/mnt/f/2026_Networked_Intelligent_Lab_Orchestrator` with the normal WSL drive
mapping.

Source may be shared through that path. Generated runtime state, databases,
artifacts, sockets, and temporary runner files belong on the WSL Linux
filesystem.

`02_Tools` separates downloaded runtimes, executable shims, uv caches, managed
Python installations, and project virtual environments by host platform:

```text
02_Tools/runtime/windows-x86_64/
02_Tools/runtime/linux-x86_64/
02_Tools/runtime/linux-arm64/
```

The shared download cache keeps checksum-verified archives, while extracted or
generated platform content remains isolated.

## 5. Simulation profile

Bench Core must provide an explicit simulation profile. The intended command
shape is:

```text
nilo serve --profile simulation
```

This command is a planned interface and does not exist in Increment 1.

Simulation must be obvious and fail closed:

- the GUI permanently displays a prominent `SIMULATION` indicator;
- the backend reports simulation mode through its authenticated status API;
- simulation uses a dedicated configuration and SQLite database;
- simulated integrations and devices use unmistakable IDs and display names;
- real hardware packages are not enabled in the simulation profile;
- no real automation or equipment network is contacted;
- operations are still authenticated, authorized, approved, audited, and
  persisted through the normal Bench Core services;
- session results and artifacts record that they were simulated;
- switching between simulation and real-hardware profiles requires an explicit
  restart and configuration selection.

Simulation is not a privileged mode. It must not bypass validation, policy,
approval, resource locking, artifact integrity, or audit behavior.

## 6. Simulation scenarios

The simulation catalog should grow from the existing fake Python integration.
It should provide deterministic scenarios covering at least:

- healthy appliance and integration status;
- device discovery and stable device identity;
- read-only operations and measurements;
- an operation requiring approval;
- queued, running, completed, failed, cancelled, and incomplete sessions;
- progress and structured events;
- artifact creation and authenticated download;
- malformed runner output;
- runner crash, hang, timeout, and restart;
- device-busy resource conflicts;
- simulated disconnect during an operation;
- storage or artifact persistence failure;
- denied and expired approvals;
- revoked authentication credentials.

Scenarios must be selectable by stable identifiers and accept deterministic
seeds or fake clocks where time affects their output. GUI and end-to-end tests
must not depend on random failures or wall-clock timing when a controlled
alternative is possible.

## 7. API and GUI contract

Bench Core is authoritative for authentication, validation, policy, approvals,
sessions, resource locks, evidence, and audit. The GUI must not invoke
integration processes, edit the database, or access simulated devices
directly.

The GUI may operate in two development modes:

1. **Mock mode:** a Windows-hosted mock implements predefined UI scenarios.
   It is used for fast component work and difficult visual states.
2. **Integrated simulation mode:** the GUI uses the real authenticated Bench
   Core API in WSL2, which supervises real integration processes operating on
   simulated devices.

The mock must be derived from, generated from, or validated against the same
versioned API schemas used by Bench Core. Mock-only fields or responses are not
allowed in production GUI code.

The WSL2 service binds to an explicitly configured loopback development
interface and port. It must not bind to all interfaces by default. Development
mode may seed test identities, but it must not introduce an unauthenticated
production route or bypass authorization checks.

## 8. Daily development flow

The intended daily flow, once the GUI and long-running server exist, is:

1. Bootstrap or verify the pinned Windows and WSL2 tools from PowerShell:

   ```powershell
   .\02_Tools\bootstrap\bootstrap-development.ps1
   ```

2. If WSL or the selected distribution is not ready, review the reported host
   change and explicitly rerun with `-InstallWSL`; complete any required reboot
   or first-launch initialization, then rerun the ordinary command.
3. Start Bench Core in WSL2 with the simulation profile and a WSL-native data
   directory.
4. Start the GUI development server on Windows.
5. Open the GUI in the Windows browser.
6. Use mock mode for isolated visual states or integrated simulation mode for
   real backend behavior.
7. Run focused GUI or backend tests while developing.
8. Run browser end-to-end tests against the WSL2 backend before considering a
   cross-component change complete.
9. Run the repository's broader validation gate in the environment required by
   the changed components.

Exact application-server, GUI, and port defaults will be added only when their
corresponding interfaces are implemented. Documentation must not present a
planned application command as currently executable.

## 9. Verification levels

| Level | Environment | What it proves |
|---|---|---|
| GUI unit/component | Windows, mock API | Rendering, interaction, accessibility, and deterministic UI states |
| GUI contract | Windows | Frontend requests and responses conform to versioned API schemas |
| Backend unit/contract | WSL2 | Go services, Python SDK, protocol, packages, persistence, and failure handling |
| Integrated browser E2E | Windows browser plus WSL2 backend | Real GUI-to-core-to-simulated-runner workflows |
| Linux system test | Linux VM | Installation, systemd, permissions, networking, firewall, and service recovery |
| ARM64 smoke | ARM64 emulator or native runner | Build, packaging, startup, and dependency architecture compatibility |
| Hardware-in-the-loop | Raspberry Pi and real equipment | Actual device discovery, I/O, flashing, timing, cleanup, and safety behavior |

Passing a lower level never substitutes for a required higher-level release
gate. In particular, WSL2 simulation cannot certify Raspberry Pi hardware or a
real PLC flashing path.

## 10. Current repository state

Increment 1 currently provides the Go foundation, SQLite stores, policy and
session services, runner supervision, Python SDK, fake integration, and
contract/acceptance tests. It does not yet provide a long-running HTTP or MCP
service or a browser GUI.

The existing Windows foundation gate is:

```powershell
.\05_Software\scripts\test-increment-1.ps1
```

`nilo serve-foundation-check` is a short-lived composition check. It is not the
future simulation server.

The environment bootstrap is executable now. The complete application flow
becomes executable after the following foundations are added:

1. a long-running `nilo serve` command;
2. a versioned authenticated HTTP API;
3. an explicit simulation profile and isolated data directory;
4. a TypeScript GUI project and pinned frontend toolchain;
5. contract-validated mock scenarios;
6. browser end-to-end tests against WSL2.

## 11. Relationship to product delivery

This development flow changes how NILO is built and tested, not where NILO is
delivered. The platform distinction is:

- **Windows 11 x86_64:** supported development host for the GUI and browser
  tests, plus portable compile checks;
- **WSL2 Linux x86_64:** supported local simulation backend for development and
  automated integration testing;
- **Linux x86_64 VM:** system and network test environment;
- **Raspberry Pi 4 Linux ARM64:** first MVP appliance and final hardware
  acceptance environment.

No result from simulation may be represented as evidence that a physical
operation occurred.
