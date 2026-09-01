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

The first executable form of this command now requires an explicit WSL-native
data directory and loopback listener:

```text
nilo serve --profile simulation [--scenario healthy|unavailable] --data-dir <absolute-path> --bind <loopback-host:port>
```

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
   directory, using the commands in section 8.1.
4. Start the GUI development server on Windows using the commands in section
   8.2.
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

### 8.1 Current backend-only workflow

From a WSL2 shell, use the shared source checkout but keep state in Linux:

```bash
cd /mnt/f/2026_Networked_Intelligent_Lab_Orchestrator/05_Software/nilo_assistant
mkdir -p "$HOME/.local/state/nilo-development"
../../02_Tools/env/with-tools.sh go run ./cmd/nilo development issue-token \
  --data-dir "$HOME/.local/state/nilo-development"
../../02_Tools/env/with-tools.sh go run ./cmd/nilo serve \
  --profile simulation \
  --scenario healthy \
  --data-dir "$HOME/.local/state/nilo-development" \
  --bind 127.0.0.1:8090
```

Copy the `token` value printed by the first command. While the server remains
running, verify it from Windows PowerShell:

```powershell
$headers = @{ Authorization = "Bearer <paste-token-here>" }
Invoke-RestMethod -Uri http://127.0.0.1:8090/api/v1/status -Headers $headers
Invoke-RestMethod -Uri http://127.0.0.1:8090/api/v1/integrations -Headers $headers
Invoke-RestMethod -Uri http://127.0.0.1:8090/api/v1/devices -Headers $headers
```

The status response must report `profile` as `simulation` and `simulation` as
`true`. Inventory responses must use unmistakable `simulation.` identities and
`SIMULATION - ` display names. Restart with `--scenario unavailable` to retain
the same identities while reporting unavailable inventory. Stop the server with
Ctrl+C. Issuing another token creates another development identity; it does not
reveal any earlier token.

This is the backend verification flow. The native Windows GUI flow is separate
below.

### 8.2 Current Windows frontend workflow

The combined bootstrap now installs the checksum-pinned Windows Node.js runtime
and restores the committed GUI lockfile. To restore only frontend dependencies,
run:

```powershell
.\02_Tools\bootstrap\bootstrap-frontend.ps1
```

Start the GUI against the WSL2 backend from the repository root:

```powershell
.\02_Tools\env\with-tools.ps1 npm.cmd --prefix .\05_Software\nilo_gui run dev
```

Open `http://127.0.0.1:5173/` in the Windows browser. The page displays a
prominent `SIMULATION` status in every state and a permanent statement that
simulated results never represent evidence from physical equipment.

Paste the `token` value printed by `nilo development issue-token` in section 8.1
into the login field and select **Connect**. The GUI then calls
`GET /api/v1/status`, `GET /api/v1/integrations`, and `GET /api/v1/devices`
together, with that token as the bearer credential on each, and displays:

- the reported profile, service status, simulation flag, build version, API
  version, and integration protocol;
- every integration, with its identifier, state, and supported operations;
- every device, with its identifier, the integration that owns it, its
  availability, its kind, and its capabilities.

Each inventory names the deterministic scenario it came from and how many
entries it is showing. The token is held in memory for that page only: it is
never written to `localStorage`, `sessionStorage`, a cookie, or a request URL, so
reloading the page returns to the login screen.

The GUI distinguishes five whole-screen outcomes, each with its own screen: the
requests are in progress, the token was rejected, Bench Core was unreachable,
Bench Core replied with a status response this interface does not understand,
and Bench Core reported its status. Beyond those, each inventory carries its own
outcome: entries, an explicit empty statement, or a message naming which of the
two inventories could not be read, with the status and the other inventory still
on screen. **Refresh** repeats all three requests with the token already held,
and **Use a different token** discards it and returns to the login screen.

The whole screen stays inside one viewport; a bench with more entries than fit
scrolls inside the inventory compartment rather than scrolling the page.

To develop the interface with no WSL2 backend at all, use mock mode:

```powershell
.\02_Tools\env\with-tools.ps1 npm.cmd --prefix .\05_Software\nilo_gui run dev:mock
```

Mock mode answers from the repository's own contract-aligned mock, accepts any
non-empty token, and contacts nothing. The mock routes on the request path, so
one scenario can leave the status healthy while an inventory is unusable. Select
the answer by setting `VITE_NILO_MOCK_SCENARIO` in
`05_Software/nilo_gui/.env.mock`, or in an ignored `.env.mock.local`:

| Scenario                 | What the mock answers                                          |
| ------------------------ | -------------------------------------------------------------- |
| `healthy`                | status healthy; ready integration and device inventory          |
| `unavailable-inventory`  | backend answers; both inventories report themselves unavailable |
| `empty`                  | backend answers; both inventories are valid and empty           |
| `unauthorized`           | every resource rejects the token                                |
| `unavailable`            | nothing answers at all                                          |
| `malformed-status`       | status outside the contract; both inventories fine              |
| `malformed-integrations` | integration inventory outside the contract; the rest fine       |
| `malformed-devices`      | device inventory outside the contract; the rest fine            |

`malformed` is still accepted and means `malformed-status`. The `healthy` and
`unavailable-inventory` bodies reproduce the deterministic catalog the Go
simulation profile serves, so a mock session and a WSL2 session show the same
integration and device identities.

The endpoint itself comes from `05_Software/nilo_gui/.env`:
`VITE_NILO_API_MODE` selects `live` or `mock`, and `VITE_NILO_API_BASE_URL` must
match the `--bind` address passed to `nilo serve --profile simulation`
(`http://127.0.0.1:8090` by default). Override either locally in an ignored
`.env.local` rather than editing the committed defaults; an unrecognised value
fails the startup with an explicit error naming the variable.

The browser never addresses that endpoint directly. It requests origin-relative
paths such as `/api/v1/status`, and the Vite development server forwards `/api`
to `VITE_NILO_API_BASE_URL`. This is deliberate: an `Authorization` header is not
CORS-safelisted, so a direct call from port 5173 to port 8090 would be
preflighted, and the loopback simulation server answers no `OPTIONS` request and
sends no CORS headers. Forwarding keeps one origin in development without giving
Bench Core a cross-origin surface, and matches the appliance, where Bench Core
serves the built assets itself. A `502` from the development server with
`ECONNREFUSED` in its log means the WSL2 backend is not running or is bound to a
different port.

To see the backend's other deterministic scenario in the browser, restart Bench
Core with `--scenario unavailable` and reload the page: the status stays healthy
while both inventories report their entries as unavailable.

Run the complete focused frontend gate with:

```powershell
.\02_Tools\env\with-tools.ps1 npm.cmd --prefix .\05_Software\nilo_gui run format:check
.\02_Tools\env\with-tools.ps1 npm.cmd --prefix .\05_Software\nilo_gui run lint
.\02_Tools\env\with-tools.ps1 npm.cmd --prefix .\05_Software\nilo_gui run test:ci
.\02_Tools\env\with-tools.ps1 npm.cmd --prefix .\05_Software\nilo_gui run build
```

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

The repository currently provides the Go foundation, SQLite stores, policy and
session services, runner supervision, Python SDK, fake integration,
contract/acceptance tests, a long-running authenticated simulation status API,
deterministic integration and device inventory, a repository-local Windows
Node.js toolchain, and a browser GUI that authenticates with a development
bearer token and displays the backend status together with the simulated
integration and device inventory. The repository does not yet provide an MCP
service.

The existing Windows foundation gate is:

```powershell
.\05_Software\scripts\test-increment-1.ps1
```

`nilo serve-foundation-check` remains a short-lived composition check. `nilo
serve --profile simulation` is the long-running development server.

The environment bootstrap, backend flow, and the authenticated GUI status and
inventory flow are executable now. The complete GUI application flow requires
the remaining foundations:

1. one-command Windows start, stop, status, and log entry points for the whole
   local topology;
2. browser end-to-end tests against WSL2, covering authentication, status,
   simulation marking, device inventory, and backend-unavailable recovery.

Until that second item lands, the browser-to-WSL2 path is proven only by a
manual browser check recorded in the task journal, never by the repository gate.

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
