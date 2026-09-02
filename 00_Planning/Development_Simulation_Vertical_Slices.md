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

### Slice 1 - Simulation backend and authenticated status (implemented)

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

### Slice 4 - GUI login and status shell (implemented)

- Add the API client and authentication flow. (implemented)
- Add the application shell, service status, and prominent `SIMULATION`
  indicator. (implemented)
- Add contract-validated mock scenarios for isolated component development.
  (implemented for the status, integration, and device resources)
- Cover loading, unauthorized, unavailable, malformed-response, and healthy
  states. (implemented)
- Add the integration and device inventory views. (implemented)

Exit gate: met. The Windows GUI works with both its contract mock and the real
WSL2 backend for status, integrations, and devices. The mock serves all three
resources by path across eight scenarios, and a live browser session against
`nilo serve --profile simulation` showed the status with both inventories in the
`healthy` and `unavailable` backend scenarios, an unreachable backend, recovery
by retry, and a single unusable inventory named without hiding the rest.

That live session was manual integration evidence recorded in the task journal.
Slice 6 has since made this path part of the repository gate.

### Slice 5 - One-command development start

- Add one Windows entry point, `02_Tools/scripts/nilo-dev.ps1`, offering `start`
  and `status`.
- Launch each component into its own visible console window; the developer
  stops one by pressing Ctrl+C in it or closing it.
- Keep SQLite, artifacts, and runtime state on the WSL Linux filesystem; refuse
  a home directory that resolves under `/mnt/`.
- Detect a port held by unrelated work, report it, and start nothing.
- Print the browser URL, the token-issuance command, and the status command; no
  token is ever stored or printed.

The slice deliberately does **not** supervise processes. It records no process
identifiers, keeps no lifecycle state in the repository, and never signals or
terminates anything, so there is no `stop` and no `logs` command. Owning
long-lived processes across Windows and a WSL2 distribution means handling
identifier reuse and process trees in two namespaces; a development convenience
that terminates nothing cannot terminate the wrong thing.

Exit gate: a developer can bring the whole local topology up from PowerShell in
one command and see, from that same command, whether it is answering.

### Slice 6 - Cross-component browser verification (implemented)

- Add browser end-to-end tests against the real WSL2 backend. (implemented)
- Exercise authentication, status, simulation marking, device inventory, and
  backend-unavailable recovery. (implemented)
- Update the development-flow document with the exact executable commands.
  (implemented)

The suite runs from `npm run test:e2e` in `05_Software/nilo_gui`, headless by
default, with `test:e2e:headed` for diagnosis. It starts its own topology: it
compiles and runs Bench Core inside a WSL2 distribution on the simulation
profile with its database under `$HOME/.local/state/nilo-e2e/`, starts its own
Windows interface server, issues its own development token, and drives a real
browser against them. Ports are taken from what is free at the moment it starts,
so it runs beside a `nilo-dev.ps1` topology without disturbing it.

`nilo-dev.ps1` is deliberately not the supervisor. It owns no process by design,
so it could neither stop the backend mid-run nor guarantee it released only its
own work; the suite manages its own short-lived processes instead and removes
them and its temporary directory on the failing path as well as the passing one.

The browser test is pinned and repository-local: the test package is fixed by
exact version in the interface lockfile, and its one browser engine is installed
below the ignored `02_Tools/runtime/windows-x86_64/playwright/` by
`bootstrap-browser.ps1`. A test run downloads nothing and retains no trace,
screenshot, video, token, or database.

Exit gate: met. On a Windows host the repository gate runs the browser suite
after the interface component tests and the production build, so the
browser-to-WSL simulation path is gated evidence rather than a manual check; a
missing prerequisite fails the gate and names its remedy instead of skipping.
That proof covers the simulated path only and certifies nothing about Raspberry
Pi hardware, real device I/O, or PLC flashing, which remain the later Linux VM,
ARM64, and hardware-in-the-loop gates.

### Slice 7 - Identity and access-control foundation (implemented)

- Reconcile the access model: identity authorization gates which actions an
  actor may *request*; integration operation policy independently gates whether
  an operation may *run* without a human approval. (implemented)
- Add fixed Administrator, Operator and Viewer roles mapped to a compiled-in
  permission set, with no custom-role editor and no policy language.
  (implemented)
- Add `nilo admin` local administration: bootstrap the first administrator, and
  list, create, re-role, disable, enable and reset the password of an identity.
  (implemented)
- Add HTTP login, logout and current-session routes, with absolute session
  expiry, server-side invalidation, an `HttpOnly` cookie, a CSRF token, a
  failed-login brake, and an explicit development-localhost versus HTTPS cookie
  mode. (implemented)
- Add named, scoped, expiring API tokens whose secret is disclosed once and
  whose effective permissions never exceed their owner's current role.
  (implemented)
- Enforce authorization server-side on every protected route, answering `401`
  without a usable credential and `403` with a usable one that lacks the
  permission. (implemented)
- Audit identity and token lifecycle changes with attributable actors and no
  plaintext secrets. (implemented)

Neither an administrator role nor a fully scoped token bypasses an approval, an
operation marked Denied or Always prompt, or a configured equipment-protection
limit; those remain the integration app's operation policy, enforced separately.

The forward SQLite migration preserves existing identities and tokens: every
pre-existing identity becomes a Viewer, and every pre-existing token keeps an
empty scope list, which means "not narrowed" and follows its owner's role. The
existing development-token flow therefore keeps working and gains nothing.

Exit gate: met. An administrator can be created on a fresh data directory
without a default credential, users and scoped tokens can be managed from the
command line, a browser can log in and out against Bench Core, and a request
from an under-privileged actor is refused at the HTTP boundary with a status
distinct from an unauthenticated one. Proof is Go tests through the real HTTP
handler, the real command binary, and a reopened SQLite data directory; the
existing Windows-to-WSL2 browser suite still passes unchanged.

This slice implements the backend only. There is no administration or login
GUI yet, and no MCP transport.

### Slice 8 - Identity and access-control interface (delivered)

- Added the GUI sign-in screen, browser session handling over the server-managed
  `HttpOnly` cookie, session restoration after a reload, sign-out, and the
  administrator-only Authentication page for users, roles and API tokens.
- Added the Bench Core identity and token administration routes those screens
  drive, each behind the existing per-route permission check.
- Added the main navigation this implies, with the Bench and Authentication
  entries only. The About and Settings surfaces are deliberately not added: an
  empty page is worse than no entry, and neither subsystem exists yet.
- Deferred to a later slice: authenticated audit browsing for identity and token
  changes, self-service token management for an Operator, durable
  login-attempt state, terminal echo suppression for command-line password
  entry, and token rotation.

## Deferred product behavior

These slices do not pull PLC flashing, hardware debugging, MCP operations,
WireGuard enrollment, appliance packaging, or physical-device acceptance ahead
of their product increments. They establish the development runway on which
those behaviors can be implemented and verified.
