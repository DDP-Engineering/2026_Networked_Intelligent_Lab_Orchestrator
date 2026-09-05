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

### 2.1 Development-stage compatibility policy

While NILO remains in active development, superseded functionality is removed
directly. Do not add backward-compatibility aliases, parallel old/new API or CLI
contracts, integration shims, or state/data conversion solely to preserve an
earlier development build. Development state is disposable and should be reset
or deleted when its owning feature is removed.

An exception requires an explicit user decision to preserve a particular
contract or dataset. Cleanup that deletes obsolete development data is allowed;
it must not retain or translate that data into the replacement design.

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

- the GUI keeps one persistent `SIM` badge with an accessible simulation label;
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

### 8.0 Starting the whole thing, and signing in

One command starts the real Go backend under WSL2 and the browser interface on
Windows:

```powershell
.\02_Tools\scripts\nilo-dev.ps1 start
```

Sign in with the fixed development administrator:

| | |
|---|---|
| Username | `admin` |
| Password | `admin_password` |

This is the whole point of the credential: the goal is to reach the running
application as fast as possible and look at real behaviour, not to run a
bootstrap step and issue a token first. The same pair signs in to mock mode, so
there is one username and one password to remember whichever mode is running.

It is created by `serve --development-administrator`, which `nilo-dev.ps1`
passes for you. The fences on it are:

- it exists only on the `simulation` profile, and the command refuses the flag
  on any other profile;
- it creates the administrator when the data directory has none, and resets the
  password when the existing administrator is this same `admin` identity — so a
  data directory carried over from an earlier session keeps working;
- an administrator with any **other** username is somebody's real account and is
  never created over, never reset, and never disturbed;
- the server prints on every start which of those actually happened.

**It is a development-phase convenience and is expected to be removed.** A
deployed appliance bootstraps its first administrator once, at the console,
with a password nobody publishes. Nothing outside `cmd/nilo/development.go`,
the flag in `serve`, and the mock's seeded identities depends on this pair.

A development bearer token still exists and is still useful, but it resolves to
a Viewer: it reads the bench and carries no management permission, so the
Network page shows telemetry and no controls. Use the administrator above when
the point is to exercise a change.

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

### 8.0 One-command lifecycle

The whole topology now starts from one Windows entry point, which runs Bench
Core under WSL2 and the interface development server on Windows. Stopping is
manual: each component gets its own window, and you close it or press Ctrl+C
there. The entry point never signals a process, so there is no `stop` command:

```powershell
.\02_Tools\scripts\nilo-dev.ps1 start
```

Each component is launched into **its own visible console window**. Its output
is in that window, and you stop it by pressing Ctrl+C there or by closing it.
There is no `stop` command and no `logs` command, because closing a window and
reading a window replace both.

A successful start prints the backend endpoint, the browser URL, the selected
distribution and scenario, the exact command that issues a development token,
and the exact command that reports status again. It reports success only once
Bench Core answers on its loopback endpoint and the interface answers on its
port. Select the topology explicitly when the defaults do not suit:

```powershell
.\02_Tools\scripts\nilo-dev.ps1 start -Distribution Ubuntu-24.04 -Scenario unavailable -BackendPort 9090 -FrontendPort 5200
```

Check it again at any time:

```powershell
.\02_Tools\scripts\nilo-dev.ps1 status
```

`status` reports one of stopped, healthy, partially running, or port conflict,
decided purely from what the two endpoints answer, and each state has its own
exit code so an automated caller can tell them apart. The full option and
exit-code tables are in `02_Tools/README.md`.

The command deliberately supervises nothing. It records no process identifiers,
keeps no state files anywhere in the repository, and never signals or terminates
a process. Owning long-lived processes across Windows and a WSL2 distribution
means handling identifier reuse and process trees in two operating system
namespaces, and a development convenience that terminates nothing cannot
terminate the wrong thing. `start` therefore starts only what is not already
answering, so running it twice is harmless and running it against a half-up
topology completes it rather than duplicating it.

A port that is held while its component does not answer is reported as a
conflict naming the port and the holding process, and nothing is started; that
process is left running.

Bench Core's SQLite database and artifacts are written to
`$HOME/.local/state/nilo-development/<instance>` inside the distribution, never
under `/mnt/`, and a home directory that resolves to a Windows drive mount is
refused rather than used.

The command never issues, stores, or prints a bearer token. It prints the
`nilo development issue-token` command, already carrying the right data
directory, and the developer runs it.

The manual commands in sections 8.1 and 8.2 remain correct and are still the
right tool for driving one component on its own.

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

A development identity is created with the least-privileged **Viewer** role, so
its token can read status and both inventories and nothing else. Reading
`/api/v1/users` with it returns `403 Forbidden` — a usable credential without
the permission — as distinct from the `401 Unauthorized` returned when no
credential is presented at all.

To exercise the human login path instead of a token, create an administrator
once against the same data directory and log in over HTTP:

```powershell
wsl.exe -- bash -lc "cd /mnt/f/2026_Networked_Intelligent_Lab_Orchestrator/05_Software/nilo_assistant && \
  printf 'change-this-password\nchange-this-password\n' | \
  ../../02_Tools/env/with-tools.sh go run ./cmd/nilo admin bootstrap \
    --data-dir `$HOME/.local/state/nilo-development --username admin"

$login = Invoke-RestMethod -Method Post -Uri http://127.0.0.1:8090/api/v1/auth/login `
    -ContentType application/json -SessionVariable session `
    -Body '{"username":"admin","password":"change-this-password"}'
Invoke-RestMethod -Uri http://127.0.0.1:8090/api/v1/users -WebSession $session
Invoke-RestMethod -Method Post -Uri http://127.0.0.1:8090/api/v1/auth/logout `
    -WebSession $session -Headers @{ "X-NILO-CSRF" = $login.csrf_token }
```

Three details in that first command are easy to get wrong. `$HOME` is escaped
with a backtick so PowerShell hands it to bash unexpanded; without the escape
PowerShell expands it on the Windows side and the data directory becomes a
Windows path. The directory must be the same `nilo-development` one the running
server opened, or the administrator is created in a database that server never
reads and the login below fails against it. And the command uses the default WSL
distribution — pass `-d <name>` if that is not the one holding the checkout.

On a Windows host under a Constrained Language Mode policy, `New-Object` is
refused for a web session type, which is why the session above is created by
`-SessionVariable` instead.

The bootstrap password is passed on standard input, never as an argument, and
no command prints it back. The login response carries the role and the
effective permissions; the session cookie is `HttpOnly`, and the logout above
fails without the `X-NILO-CSRF` header. `nilo serve` runs in the
`development-localhost` cookie mode by default, which omits the `Secure`
attribute because the local topology is plain HTTP on loopback; any HTTPS
deployment must pass `--session-cookies https`.

The complete command and authentication reference is the shipped document
`../07_Delivery/NILO_CLI_and_Authentication.md`.

The identity created by `nilo admin bootstrap` above is the one you sign in with
in the browser; section 8.2 describes that flow. Bootstrap stays a local console
operation with no HTTP route, so create it against the data directory the server
serves before opening the interface.

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
single persistent `SIM` badge with an accessible simulation label after sign-in.
The sign-in screen explains that simulated results are not physical evidence;
authenticated pages do not repeat that statement.

**Signing in as a human.** Enter the username and password of the administrator
created with `nilo admin bootstrap` in section 8.1 and select **Sign in**. Bench
Core answers with a session cookie that is `HttpOnly` — the page cannot read it,
and neither can an injected script — and a readable `nilo_csrf` cookie the page
repeats in the `X-NILO-CSRF` header on every state-changing request. Reloading
the page keeps you signed in: the interface asks `GET /api/v1/auth/session` who
the browser's cookies belong to, and only shows the sign-in screen when the
answer is that nobody is. **Sign out** ends the session server-side, so the
cookie stops working for whoever holds it.

The top bar names the signed-in identity and its role, and offers **Bench** and,
for an identity holding the administration permissions, **Authentication**.

**Connecting with a development token instead.** Paste the `token` value printed
by `nilo development issue-token` in section 8.1 into the **Development bearer
token** field and select **Connect**. That identity is a least-privileged
Viewer, so it reads the bench and is refused by the administration routes. The
token is held in memory for that page only — never in `localStorage`,
`sessionStorage`, a cookie, or a request URL — so reloading the page returns to
the sign-in screen. No password, session secret, cross-site request forgery
token or newly created API token secret is written to any of those either.

**The shell.** One compact row across the top carries the NILO wordmark, the
open page tabs, and the network, `SIM`, appearance, language and account
controls; there is no second tab strip and no page title above the workspace.
The left navigation lists the working entries and the integrations Bench Core
actually reports, with a bottom-anchored cluster of **Plugins, Network,
Authentication, Activity** and **About**. Network is a collapsible
built-in group with **Overview, Network adapters, Device access, VPN profiles**
and **Diagnostics**. Choosing the group root opens Overview; choosing any entry
opens it as a tab; tabs activate on click, move with the arrow, Home and End
keys, close from their own control, and fall back to the nearest remaining
neighbour. Authentication is a second collapsible group whose permission-aware
**Users** and **API tokens** destinations open as distinct workspace tabs.

The shell has three layouts, decided by the viewport and used by the stylesheet
and the interface alike. Above 72rem it is the full rail. At or below 72rem —
the seven-inch panel's usual 1024x600 — the rail narrows but every entry keeps
its visible label. At or below 56rem wide, or 520 pixels high, the rail gives
way to a **Menu** control that opens a labelled navigation drawer; Escape closes
it and focus returns to the control. The appearance and language selections
move into the account menu there, while the tabs, the network summary and the
single `SIM`/`LIVE` indicator stay in the row.

**Appearance and language.** The appearance menu offers **Light**, **Dark** and
**System**; System follows the operating system or browser preference and
changes with it without a reload. The language menu offers **English**,
**Deutsch** and **Română**. Both selections are stored in `localStorage` under
`nilo.theme` and `nilo.language`, which are the only two keys this interface
ever writes: no password, session secret, request-forgery token or API token
secret is stored beside them. Identifiers, permission names, protocol names,
API values and everything Bench Core reports stay as sent, whatever the
language.

**Network.** The real simulation server reports simulated interfaces and
WireGuard profiles, applies acknowledged in-memory changes and samples traffic
into minute history in SQLite. Connectivity changes must be confirmed within
120 seconds or are restored. The GUI polls the actual operation result and
reports browser reachability separately. Overview summarizes the next decision;
Network adapters, Device access and VPN profiles own configuration; Diagnostics
owns WireGuard readiness and traffic history. The history chart identifies RX
and TX, labels its byte and time scales, preserves unmeasured gaps and exposes
the exact selected minute by pointer, touch or keyboard. Live NetworkManager
management, root key storage and crash/reboot recovery are unfinished; the browser-only mock
does not implement the full new lifecycle. See
[Network implementation status](Network_Service_Implementation_Status.md).

**Plugins** names a subsystem that is not built and says so; the navigation
marks it. **About** is real, and reports the build identity Bench Core returns.

**The Bench page** calls `GET /api/v1/status`, `GET /api/v1/integrations`, and
`GET /api/v1/devices` together with the credential in use, and displays:

- the reported profile, service status, simulation flag, build version, API
  version, and integration protocol;
- every integration, with its identifier, state, and supported operations;
- every device, with its identifier, the integration that owns it, its
  availability, its kind, and its capabilities.

Each inventory names the deterministic scenario it came from and how many
entries it is showing.

The GUI distinguishes the whole-screen outcomes with their own screens: the
session is being restored, the requests are in progress, the token was rejected,
this identity is not permitted, Bench Core was unreachable, Bench Core replied
with a status response this interface does not understand, and Bench Core
reported its status. An ended session and a refused permission are deliberately
different: the first returns to the sign-in screen, the second keeps you signed
in and says an administrator has to widen the role or the token scopes. Beyond
those, each inventory carries its own outcome: entries, an explicit empty
statement, or a message naming which of the two inventories could not be read,
with the status and the other inventory still on screen. **Refresh** repeats all
three requests with the credential already held.

**Authentication** is split into focused, permission-aware **Users** and **API
tokens** destinations. Each owns its loading, empty and failure state and does
not load the other listing; API tokens also reads identities for its owner
selector:

- identities are listed with their role and enabled state, and can be created
  with one of the three fixed roles, re-roled, disabled, re-enabled, have their
  password reset, and deleted permanently. Creation time is under **Details**.
  Disabling keeps the record and stops the identity signing in; deleting removes
  the record and every browser session it holds, and cannot be undone. The last
  enabled administrator cannot be disabled, demoted or deleted, and an identity
  that owns API tokens cannot be deleted at all — the tokens are never swept
  along, and the refusal says to delete them first;
- tokens are listed with their name, owner, effective state and expiry; scopes
  and creation time are under **Details**. State precedence is revoked, expired,
  then active. Tokens
  and can be created for an existing identity with optional scopes and an
  optional expiry, revoked, and deleted permanently. Revoking stops the secret
  and keeps the record, and is offered only while the token is active; deleting
  removes the record whatever state it is in. A token's permissions are its
  scopes narrowed by its owner's current role, which the creation form states,
  so a token can never exceed that role.

Row **Actions** dialogs identify the record and explain unavailable operations,
with the two permanent removals in the destructive group beneath them.
Disabling an identity, resetting a password and revoking a token each ask for an
explicit confirmation naming the subject. Each permanent deletion asks for more:
it states what is removed, keeps a warning visible that it cannot be undone, and
sends nothing until the identity's username or the token's name has been typed
back exactly. That is presentation only. Bench Core enforces the last-enabled-
administrator invariant, the owned-token refusal and the audit record inside the
one transaction that applies the removal, so a client that skips the dialog
gains nothing. Field popovers and workflow help
explain role and scope semantics, access impact, external coordination and
verification without hiding persistent warnings. A new token's secret is shown once,
with a copy action and a statement that it cannot be retrieved later; dismissing
it or reloading the page does not bring it back, because Bench Core stores only
a one-way verifier of it; closing the disclosure clears interface state and the
document subtree. **View activity** opens Activity filtered to the `identity`
component.

Hiding the navigation entry is not a permission. Bench Core authorizes every
administration request on its own — `401` without a usable credential, `403`
with one that lacks the permission — so an identity that reaches the page
another way is refused there.

Every page stays inside one viewport at a normal desktop width. Bench inventory
retains its bounded list compartment. A focused user or token listing follows
normal workspace page flow, avoiding a second nested scrollbar; below 56rem its
single semantic table becomes labelled record cards.

### The compact field interface

The interface is sized for a seven-inch appliance display as well as a desktop
monitor. `1024x600` is the primary field viewport and `800x480` the constrained
fallback; both are read at 100% browser zoom.

One semantic type scale lives in `src/styles/tokens.css` and nothing else
declares a size: page titles are 22px, section titles 18px, body and control
text 16px, dense table values and navigation labels 15px, and labels, help and
secondary status 14px. No operator-facing text is smaller than 14px, in either
theme, and no table is made to fit by shrinking it. Every button, input, select,
tab, navigation entry and row action is at least 44 by 44 pixels, and a checkbox
sits inside a label of that height.

Each list — Interfaces, WireGuard profiles, Device networks, Activity, the bench
inventories, the identity and token listings — is one markup that renders as a
table on a wide display and as a stack of labelled cards below 56rem. The
essential columns stay in the row; every other value is one **Details** control
away, in an expansion that is announced as a region, and several actions on one
item are offered from a single **Actions** control. It opens a compact dialog
above the page, where the object type and current status remain visible, every
action is a large button, disabled entries state why, destructive choices are
separated, and Cancel returns focus without moving the page. Because the dialog
is attached to the document rather than the table, a table scrollbar cannot
clip it. Nothing is silently dropped at a narrow width.

Dialogs are bounded to the viewport: a head with the title and a close control,
a body that scrolls, and the action bar stuck to the bottom of that body. At or
below 56rem wide, or 640 pixels high, a configuration dialog becomes a
full-screen sheet. Escape closes the topmost layer only, so a help popover over
an editor closes the popover and leaves the editor open.

**Help has three levels.** Persistent guidance sits under a field when the
operator needs it to choose a valid value, and it never disappears. A help
control beside a setting's label opens a popover with a short definition and an
example; it opens on click, tap and keyboard, exposes `aria-expanded` and the
panel, closes on Escape, outside activation or its own close control with focus
returned, allows one open at a time, and repositions to stay in view. Hover is
an extra, never a requirement, and no explanation lives only in a `title`
attribute. **How to configure** in the device-network editor and the adapter
dialog opens the workflow help: the goal, the values to have ready, an
addressing example, what NILO will change, what must already be configured on
another computer, router, VPN hub or device, and how to verify and recover.

Safety information stays outside help. The confirmation window and automatic
rollback, the countdown, a failed rollback, a refused change, field validation
and the external-route prerequisite are all persistent text beside the control
or in the confirmation dialog.

The Network group opens with one Overview page — management or uplink adapter
with its address, device side with its address, the active WireGuard profile or
None, and whatever needs attention. Network adapters, Device access, VPN profiles
and Diagnostics then expose only their assigned concern. The device-network
editor is grouped as Identity,
Device side, Access path, Addressing, Services, a collapsed Advanced, and a
Review that states in plain language which adapter carries which address, which
sources may reach which target, whether WireGuard is involved, whether source
NAT is used and whether the management connection could be affected. Ports stay
editable: 502 is shown as an example because a device may be set to another
port such as 504.

To develop the interface with no WSL2 backend at all, use mock mode:

```powershell
.\02_Tools\env\with-tools.ps1 npm.cmd --prefix .\05_Software\nilo_gui run dev:mock
```

Mock mode answers from the repository's own contract-aligned mock and contacts
nothing. It seeds the identities `admin`, `operator` and `viewer`, all with the
password `simulation-password`, and authenticates any non-empty bearer token as
the Viewer. It enforces the same role matrix, the same CSRF rule and the same
last-administrator rule the backend does, so a screen that works there is not
relying on the mock being lenient; its store is in memory, so reloading the page
in mock mode signs you out. The mock routes on the request path, so one scenario
can leave the status healthy while an inventory is unusable. Select the answer
by setting `VITE_NILO_MOCK_SCENARIO` in `05_Software/nilo_gui/.env.mock`, or in
an ignored `.env.mock.local`:

| Scenario                 | What the mock answers                                          |
| ------------------------ | -------------------------------------------------------------- |
| `healthy`                | status healthy; ready integration and device inventory          |
| `unavailable-inventory`  | backend answers; both inventories report themselves unavailable |
| `empty`                  | backend answers; both inventories are valid and empty           |
| `unauthorized`           | every request is refused, sign-in included                       |
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

### 8.3 Integrated browser test

The browser-to-WSL2 path is proven by one command. It needs no running topology
and no token: it builds and starts its own Bench Core inside the distribution on
the simulation profile, starts its own interface server on Windows, issues its
own development token, and drives a real browser against them.

Install the pinned browser once — the combined development bootstrap in section
8 already does this — and then run the suite:

```powershell
.\02_Tools\bootstrap\bootstrap-browser.ps1
.\02_Tools\env\with-tools.ps1 npm.cmd --prefix .\05_Software\nilo_gui run test:e2e
```

The browser binaries live under the ignored
`02_Tools/runtime/windows-x86_64/playwright/`, beside the other pinned runtimes.
No persistent host environment is changed and a test run downloads nothing.

`e2e/bench-core.spec.ts` covers, in one browser session against one backend:
submitting a development token on the sign-in screen; the authenticated status
request completing; the persistent simulation identity and the
development boundary before sign-in, without repeated authenticated notices; the reported
`simulation` profile and `healthy` service status; the deterministic integration
inventory; the deterministic device inventory with the integration that owns
each device; Bench Core presented as unreachable once its process is stopped;
and recovery to the connected screen through the interface's own retry control,
with no page reload and no second token.

`e2e/shell.spec.ts` covers what only a browser settles about the shell: that
the header really is one row within the agreed height, that the lower cluster is
the core entries with the nested Network group among them, that focused Network
destinations open separate tabs, that tabs close with the documented fallback,
that the Network pages report nothing they were not told, that
a theme and a language survive a real reload together with the session, and that
a narrow viewport narrows the navigation without clipping or naming nothing.

`e2e/field-display.spec.ts` covers the compact field interface at `1920x1080`,
`1366x768`, `1024x600` and `800x480` in the light and dark themes: the labelled
rail and the navigation drawer, the Network overview, the details of an
interface, a profile and a device network on their focused pages, Diagnostics,
the row action dialogs, field help
opened and closed by pointer and by keyboard and kept inside the viewport, the
adapter and device-network editors reaching their final actions, the full-screen
sheet, an invalid address and an invalid port refused beside their fields, a
change awaiting confirmation named in the overview and restored, and the other
pages. At every viewport it asserts that no visible text renders below 14
pixels, that nothing scrolls the document sideways, that no explanation lives
only in a `title` attribute, and — at the two field sizes — that no interactive
target is below 44 by 44 pixels. It is proof in CSS pixels; acceptance on the
physical seven-inch panel is a separate, still-outstanding step. Run it with
`NILO_FIELD_REVIEW=1` to keep one screenshot per viewport and theme under
`test-results/field/`.

`e2e/authentication.spec.ts` covers the human path against its own backend, with
a real administrator it bootstraps through the real command line, passing the
password on standard input: password sign-in; creating an identity; changing its
role; creating an API token with a scope and reading its one-time secret;
reloading the page and finding the session restored and the secret gone; a
revocation that only happens after an explicit confirmation; the refusal to
disable the last enabled administrator; the Authentication entry a Viewer does
not get; and a sign-out that a reload confirms was server-side. Nothing in it is
mocked: every refusal is Bench Core's own decision over its own database.

It is headless and retains nothing: no trace, no video, no screenshot, no
database, and no token. To watch a failure, run the same suite with the browser
visible:

```powershell
.\02_Tools\env\with-tools.ps1 npm.cmd --prefix .\05_Software\nilo_gui run test:e2e:headed
```

Each run takes loopback ports that are free at the moment it starts, so it does
not collide with a `nilo-dev.ps1` topology already using the defaults, and it
keeps its Bench Core database and artifacts in a fresh temporary directory under
`$HOME/.local/state/nilo-e2e/` inside the distribution — never under `/mnt/`. At
the end of the run, passing or failing, it stops only the processes it started
and removes only that directory. It never frees a port and never matches a
process by name.

That removal is confirmed rather than assumed: the release script checks the
directory is absent afterwards instead of trusting the removal command's exit
status, and a release that does not fully succeed fails the run and names the
directory that may survive. A distribution that could not delete the database
has not deleted it, and reporting that as a clean pass would be the opposite of
the guarantee.

Every secret the suites type — the development token and the fixture passwords —
reaches the browser only through a harness method that redacts the value out of
anything the action throws, and the administrator's password is written to the
bootstrap command's standard input rather than its command line.

The development token is held in one private field with no accessor and reaches
the browser only through the harness method that types it into a supplied field.
That method redacts the value out of anything the action throws, which matters
because a browser action failing *after* its locator has resolved repeats its
argument in the error message — and that message reaches the console reporter
and the error-context note whether or not trace, video, and screenshot are off.
A forced-failure test provokes that exact case with a dummy marker and asserts
the value is absent, paired with a test asserting the unwrapped call still leaks
it, so the protection is only ever removed on evidence.

`02_Tools/scripts/nilo-dev.ps1` is deliberately **not** the supervisor for this
suite. That entry point is for visible manual development windows: it owns no
process, records no identifier, and signals nothing, so it could neither stop
the backend mid-run nor guarantee it cleaned up only its own work.

Select the distribution explicitly when more than one is installed:

```powershell
$env:NILO_E2E_DISTRIBUTION = 'Ubuntu-24.04'
```

A missing prerequisite — no WSL2 distribution, a checkout the distribution
cannot see, or no repository-local browser — fails the run and names the
repository command that provides it. The suite never skips itself, because a
skipped browser test reported as a pass would claim a browser reached Bench Core
under WSL2 when nothing was observed.

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

The local development bench has an optional Raspberry Pi ARM64 device that can
answer some ARM64, packaging, and physical I/O questions the simulated topology
cannot; it is described in [Local Hardware Access](Local_Hardware_Access.md). It
is a convenience for early signal only. No repository gate contacts it, it may
be powered off or off the network at any time, and an observation made on it is
not the hardware-in-the-loop level in the table above.

## 10. Current repository state

The repository currently provides the Go foundation, SQLite stores, policy and
session services, runner supervision, Python SDK, fake integration,
contract/acceptance tests, a long-running authenticated simulation status API,
deterministic integration and device inventory, identity and access control with
its HTTP administration routes, a repository-local Windows Node.js and browser
toolchain, a browser GUI in which a human signs in with a password and
administers identities and API tokens — including their permanent deletion —
beside the backend status and the simulated inventory, and integrated browser
tests that drive that GUI against a real WSL2 backend. The `/mcp` endpoint now provides authenticated read-only
Network status, statistics and WireGuard status tools; it shares the HTTP
identity service. Appliance TLS startup remains unfinished.

The existing Windows foundation gate is:

```powershell
.\05_Software\scripts\test-increment-1.ps1
```

`nilo serve-foundation-check` remains a short-lived composition check. `nilo
serve --profile simulation` is the long-running development server.

The environment bootstrap, backend flow, the authenticated GUI status and
inventory flow, the one-command development start, and the integrated browser
test are all executable now.

On a Windows host the repository gate runs that browser test after the GUI
component tests and the production build, so the browser-to-WSL2 simulation path
is repository-gated evidence rather than a manual check recorded in a task
journal. A missing prerequisite fails the gate and names its remedy; it is never
skipped and then presented as proof.

That gate proves the simulated path only. It certifies nothing about Raspberry
Pi hardware, real device I/O, or PLC flashing, all of which remain the later
Linux VM, ARM64, and hardware-in-the-loop gates in section 9.

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

The Raspberry Pi on the local development bench is not that acceptance
environment. It shares the architecture and the operating system, which is what
makes it useful early, but its configuration is not recorded in this repository
and cannot be rebuilt from it. See
[Local Hardware Access](Local_Hardware_Access.md).

No result from simulation may be represented as evidence that a physical
operation occurred.

## Live Network development checkpoint

`serve --profile appliance` is now available on explicitly prepared Linux test
machines, with a root NetworkManager helper and an unprivileged HTTPS Bench Core.
It is separate from the default Windows/WSL2 simulation topology and does not
change the simulation launcher or its fixed development administrator.
[Live Network development](Live_Network_Development.md) gives the isolated Docker
test command, deployment prerequisites, startup commands and remaining limits.
The system test never joins the host network namespace or system D-Bus.
