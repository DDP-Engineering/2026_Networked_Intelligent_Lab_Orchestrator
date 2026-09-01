# Development simulation vertical slices

## Purpose

This plan turns the approved Windows-GUI and WSL2-backend development flow into
small, runnable slices. It is an enabling implementation sequence across the
product increments, not a replacement for `System_Build_Plan.md` or the MVP
increment boundaries.

Each slice must leave one useful workflow executable and tested before the next
slice begins. Simulation remains explicit and never represents evidence from
physical equipment.

## Task list

### Slice 1 - Simulation backend and authenticated status

- Add `nilo serve --profile simulation` with explicit data and bind settings.
- Restrict the simulation server to loopback addresses.
- Add a versioned authenticated status endpoint that reports `simulation`.
- Add development-only issuance of a persisted, revocable bearer token.
- Shut down cleanly on process cancellation or operating-system signals.
- Verify the API and command behavior with Go tests.

Exit gate: a developer can run Bench Core under WSL2, authenticate from Windows,
and receive an unambiguous simulation status response.

### Slice 2 - Deterministic simulated inventory (implemented)

- Add stable simulated integration and device identities.
- Expose authenticated service and device inventory endpoints.
- Provide healthy and unavailable deterministic scenarios.
- Keep real hardware packages disabled in the simulation profile.

Exit gate: the real backend returns predictable device and service state without
contacting equipment networks.

### Slice 3 - Repository-local frontend toolchain (implemented)

- Pin Node.js and the package manager in `02_Tools` with checksums.
- Bootstrap the Windows frontend runtime without changing the persistent host
  environment.
- Scaffold the TypeScript browser application, formatting, linting, unit tests,
  and production build.

Exit gate: a fresh supported Windows checkout can build and test the GUI using
only repository bootstrap commands.

### Slice 4 - GUI login and status shell

- Add the API client and authentication flow.
- Add the application shell, service status, device list, and prominent
  `SIMULATION` indicator.
- Add contract-validated mock scenarios for isolated component development.
- Cover loading, unauthorized, unavailable, and healthy states.

Exit gate: the Windows GUI works with both its contract mock and the real WSL2
backend.

### Slice 5 - One-command development lifecycle

- Add Windows entry points for start, stop, status, and logs.
- Keep SQLite, artifacts, and runtime state on the WSL Linux filesystem.
- Detect port conflicts and stale processes without killing unrelated work.
- Print the browser URL and the commands needed for diagnosis.

Exit gate: a developer can reliably start and stop the complete local topology
from PowerShell.

### Slice 6 - Cross-component browser verification

- Add browser end-to-end tests against the real WSL2 backend.
- Exercise authentication, status, simulation marking, device inventory, and
  backend-unavailable recovery.
- Update the development-flow document with the exact executable commands.

Exit gate: the repository gate proves the browser-to-WSL simulation path and
clearly separates that proof from Raspberry Pi and hardware acceptance.

## Deferred product behavior

These slices do not pull PLC flashing, hardware debugging, MCP operations,
WireGuard enrollment, appliance packaging, or physical-device acceptance ahead
of their product increments. They establish the development runway on which
those behaviors can be implemented and verified.
