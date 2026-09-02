# NILO browser GUI

This project is the native Windows development frontend for NILO. It builds to
static assets for later serving by Bench Core; Node.js is a development and
build dependency, not an appliance runtime dependency.

A human signs in with a username and password and gets a server-managed
`HttpOnly` session cookie. The interface then shows the backend status with the
simulated integration and device inventory, and - for an administrator - an
Authentication page that administers identities and API tokens. A development
bearer token remains available beside the password form, for reaching a backend
with no identity bootstrapped yet.

From the repository root, restore the locked dependency graph:

```powershell
.\02_Tools\bootstrap\bootstrap-frontend.ps1
```

Then run commands through the repository-local tool wrapper:

```powershell
.\02_Tools\env\with-tools.ps1 npm.cmd --prefix .\05_Software\nilo_gui run dev
.\02_Tools\env\with-tools.ps1 npm.cmd --prefix .\05_Software\nilo_gui run dev:mock
.\02_Tools\env\with-tools.ps1 npm.cmd --prefix .\05_Software\nilo_gui run format:check
.\02_Tools\env\with-tools.ps1 npm.cmd --prefix .\05_Software\nilo_gui run lint
.\02_Tools\env\with-tools.ps1 npm.cmd --prefix .\05_Software\nilo_gui run test:ci
.\02_Tools\env\with-tools.ps1 npm.cmd --prefix .\05_Software\nilo_gui run build
```

The development server binds to `127.0.0.1` and uses Vite's default port 5173
unless an explicit `-- --port <port>` argument is supplied. Generated
`node_modules/`, `coverage/`, `dist/`, and `test-results/` directories are
ignored and must not be committed.

## Integrated browser test

One command drives a real browser against the real Windows interface and a real
Bench Core running under WSL2 on the simulation profile:

```powershell
.\02_Tools\bootstrap\bootstrap-browser.ps1
.\02_Tools\env\with-tools.ps1 npm.cmd --prefix .\05_Software\nilo_gui run test:e2e
```

The first command installs the pinned browser once, below the ignored
`02_Tools/runtime/windows-x86_64/playwright/`; the combined development
bootstrap already runs it. A test run downloads nothing.

The suite is headless and retains no trace, video, screenshot, database, or
token. The one thing a passing run leaves is Playwright's own
`test-results/.last-run.json`, a pass/fail marker it writes after the run ends;
it is ignored by Git. To watch a failure happen, run the same suite with the
browser visible:

```powershell
.\02_Tools\env\with-tools.ps1 npm.cmd --prefix .\05_Software\nilo_gui run test:e2e:headed
```

Each run picks its own free loopback ports, so it does not disturb a
`nilo-dev.ps1` topology already running on the defaults, and it creates its own
temporary Bench Core data directory under `$HOME/.local/state/nilo-e2e/` inside
the distribution. It stops only the processes it started and removes only that
directory, on the failing path as well as the passing one; it never frees a
port and never matches a process by name. Select the distribution explicitly
with `$env:NILO_E2E_DISTRIBUTION` when more than one is installed.

Cleanup is checked, not assumed. The release script confirms the directory is
gone rather than trusting the removal command's exit status, and a release that
does not fully succeed fails the run and names the directory that may survive -
a distribution that could not delete the database has not deleted it.

The token never leaves the harness. There is no accessor for it: the browser
suite hands the login field to the harness, which types the value in behind a
redaction wrapper. That wrapper is why the suite is safe to fail - a browser
action that fails _after_ its locator resolves repeats its argument in the error
it throws, and that message reaches the console and the error-context note
regardless of trace, video, and screenshot being off. `e2e/token-redaction.spec.ts`
provokes exactly that failure with a dummy marker and asserts the value is gone,
with a companion test asserting the unwrapped call still leaks it, so the wrapper
is never dropped on a guess.

A missing prerequisite fails the run and names the repository command that
provides it. Nothing is ever skipped: a skipped browser test reported as a pass
would claim a browser reached WSL2 when none did.

`*.test.ts` belongs to the component runner (`test:ci`) and `*.spec.ts` to the
browser runner (`test:e2e`); neither collects the other's files.

## How the browser reaches Bench Core

The interface only ever requests origin-relative paths - `/api/v1/status`,
`/api/v1/integrations`, `/api/v1/devices`, the `/api/v1/auth/*` session routes,
and the `/api/v1/users` and `/api/v1/tokens` administration routes. It never
addresses Bench Core directly: neither a bearer credential nor a
`SameSite=Strict` session cookie survives a cross-origin call, which would be
preflighted anyway, and Bench Core answers no preflight and exposes no CORS
headers.

In development the Vite server forwards `/api` to the configured Bench Core
endpoint, so the page at `127.0.0.1:5173` and the backend at `127.0.0.1:8090`
stay one origin as far as the browser is concerned. On an appliance Bench Core
serves the built assets itself, where those paths are already same-origin.

## Backend configuration

| Variable                  | Values                     | Default                 |
| ------------------------- | -------------------------- | ----------------------- |
| `VITE_NILO_API_MODE`      | `live`, `mock`             | `live`                  |
| `VITE_NILO_API_BASE_URL`  | absolute http or https URL | `http://127.0.0.1:8090` |
| `VITE_NILO_MOCK_SCENARIO` | one of the scenarios below | `healthy`               |

The mock routes on the request path, so one scenario can leave the status
healthy while a single inventory is unusable:

| Scenario                 | What the mock answers                                           |
| ------------------------ | --------------------------------------------------------------- |
| `healthy`                | status healthy; ready integration and device inventory          |
| `unavailable-inventory`  | backend answers; both inventories report themselves unavailable |
| `empty`                  | backend answers; both inventories are valid and empty           |
| `unauthorized`           | every request is refused, sign-in included                      |
| `unavailable`            | nothing answers at all                                          |
| `malformed-status`       | status outside the contract; both inventories fine              |
| `malformed-integrations` | integration inventory outside the contract; the rest fine       |
| `malformed-devices`      | device inventory outside the contract; the rest fine            |

`malformed` is still accepted and means `malformed-status`. The `healthy` and
`unavailable-inventory` bodies reproduce the deterministic catalog the Go
simulation profile serves, so a mock session and a WSL2 session show the same
integration and device identities.

`VITE_NILO_API_BASE_URL` is the endpoint the development server forwards `/api`
to and the label the sign-in screen shows; it is not fetched from the browser.

`.env` holds the live defaults for a Windows browser talking to Bench Core under
WSL2. `.env.mock` is loaded by `dev:mock` and `build:mock`, which pass
`--mode mock` to Vite. Neither file contains a credential. Override values in an
ignored `.env.local` or `.env.mock.local` rather than editing the committed
files; an unusable value stops startup with an error naming the variable.

## Signing in

Create the first administrator once, at the console, against the same data
directory Bench Core serves (see
`06_SoftwareDocu/Development_and_Simulation_Flow.md`, section 8.1). There is no
self-registration and no remote bootstrap route:

```bash
printf 'change-this-password\nchange-this-password\n' | \
  ../../02_Tools/env/with-tools.sh go run ./cmd/nilo admin bootstrap \
    --data-dir "$HOME/.local/state/nilo-development" --username admin
```

Then sign in on the page with that username and password. Bench Core sets an
`HttpOnly` session cookie the page cannot read, and a readable `nilo_csrf`
cookie the page echoes in `X-NILO-CSRF` on every state-changing request.
Reloading the page keeps you signed in: the interface asks
`GET /api/v1/auth/session` who the browser's cookies belong to. **Sign out**
ends the session server-side.

Nothing about a credential is written to `localStorage`, `sessionStorage`, a
request URL, or the console.

To reach a backend with no identity yet, issue a development token instead:

```bash
../../02_Tools/env/with-tools.sh go run ./cmd/nilo development issue-token \
  --data-dir "$HOME/.local/state/nilo-development"
```

Paste the printed `token` into the **Development bearer token** field. That
identity is a least-privileged Viewer, so it reads the bench and is refused by
the administration routes. The token is held in memory for the lifetime of the
page only, so a reload returns to the sign-in screen.

In mock mode the seeded identities are `admin`, `operator` and `viewer`, all
with the password `simulation-password`, and a bearer token authenticates as the
Viewer. The mock enforces the same role matrix, the same CSRF rule and the same
last-administrator rule the backend does, so a screen that works there is not
relying on the mock being lenient. Its store is in memory, so reloading the page
in mock mode signs you out.

## What the signed-in screens show

**Bench** carries the backend status, the integrations and their state, and the
devices with the integration that owns each one and its availability, inside one
viewport. A status that cannot be read settles the whole screen, as one of a
rejected token, a refused permission, an unreachable backend, or an unsupported
status response. Once the status is in, each inventory carries its own outcome -
entries, an explicit empty statement, or a message naming which inventory could
not be read while the other one and the status stay visible. **Refresh** repeats
all three requests with the credential already held.

**Authentication** appears in the navigation only for an identity holding the
administration permissions. It administers identities on one side and API tokens
on the other. Identities can be created with one of the three fixed roles,
re-roled, disabled, re-enabled, and have their password reset; they are never
deleted, so audit records keep naming a real subject. Tokens can be created for
an existing identity with a name, optional scopes and an optional expiry, and
revoked. Disabling, resetting a password and revoking each ask for an explicit
confirmation naming the subject.

A new token's secret is shown once, with a copy action and a statement that it
cannot be retrieved later. Dismissing it or reloading the page does not bring it
back, because Bench Core stores only a one-way verifier of it.

Every screen keeps the `SIMULATION` indicator and the statement that simulated
results never represent evidence from physical equipment. Each listing scrolls
inside its own compartment; the page itself never scrolls.

Hidden navigation is not a permission. Bench Core authorizes every request on
its own, answering `401` for a credential it does not accept and `403` for one
that lacks the permission, and the interface tells those two apart: the first
returns to the sign-in screen, the second does not.

## Source layout

- `src/api/contract.ts` - the versioned `nilo.http/1` wire contract, its
  origin-relative paths, and the strict parsers shared by the client and the
  mock.
- `src/api/client.ts` - the typed Bench Core client for the bench resources, the
  session routes and the administration routes; the `Credential` union that
  chooses between a session cookie and a bearer header; and `BenchCoreError`
  with its `unauthorized`, `forbidden`, `rejected`, `unavailable` and
  `malformed` causes.
- `src/api/csrf.ts` - reads the readable `nilo_csrf` cookie, the only part of the
  session credential the page can see.
- `src/api/permissions.ts` - whether a signed-in identity has any reason to open
  the Authentication page. Presentation only; the server decides.
- `src/api/mock.ts` - path-routed, contract-built mock answers plus an in-memory
  identity and token store, for development without a backend.
- `src/config/environment.ts` - resolves the mode and base URL above.
- `src/App.tsx` - the shell: session restoration, the credential in use, and
  which page is open.
- `src/components/` - `SignInScreen`, `WorkspaceHeader`, `StatusPanel`,
  `InventoryPanel`, `AuthenticationPage`, `AdministrationForms` and `Modal`.
- `playwright.config.ts` - the integrated browser runner: headless, serial, no
  retries, and no retained trace, video, or screenshot.
- `e2e/topology.ts` - the run's own Bench Core under WSL2 and interface server
  on Windows: free ports, WSL-native temporary state, the in-memory token,
  bounded readiness waits, and a teardown limited to what it started.
- `e2e/prerequisites.ts` - the checks that fail a run, with the repository
  command that supplies each missing prerequisite.
- `e2e/bench-core.spec.ts` - the bearer-token browser journey: connect, status,
  simulation identity, both inventories, the unreachable screen, and recovery.
- `e2e/authentication.spec.ts` - the human browser journey against a real
  administrator bootstrapped through the real command line: password sign-in,
  identity creation, a role change, a token with its one-time secret, session
  restoration across a reload, revocation, the last-administrator refusal, the
  navigation entry a Viewer does not get, and sign-out.
- `e2e/token-redaction.spec.ts` - the forced-failure regression that keeps the
  token out of a failing action's error.

Simulation is never evidence that a physical operation occurred; the interface
states this on every screen.
