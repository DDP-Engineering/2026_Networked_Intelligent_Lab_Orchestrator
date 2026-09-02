# Browser GUI foundation

## Purpose

The browser GUI foundation provides NILO's native Windows development
interface while keeping the deployed appliance boundary static. It gives
developers a locked, testable React and TypeScript project in which a human
signs in with a username and password over the server-managed browser session,
reads the backend status with the simulated integration and device inventory,
and — as an administrator — administers identities and API tokens, without ever
suggesting that a simulated result came from physical equipment.

## Public interface

The project root is `05_Software/nilo_gui`. Developers use its npm scripts
through `02_Tools/env/with-tools.ps1`: `dev` serves the interface against a real
backend, `dev:mock` serves it against the built-in contract mock, `format:check`
and `lint` enforce source quality, `test:ci` runs deterministic component tests,
`test:e2e` runs the integrated browser test against a real WSL2 backend with
`test:e2e:headed` as its diagnosis variant, and `build` (or `build:mock`) emits
static production assets into ignored `dist/`.

`*.test.ts` belongs to the component runner and `*.spec.ts` to the browser
runner; `vite.config.ts` and `playwright.config.ts` each name their own files, so
neither collects the other's.

Three build-time environment variables configure the endpoint:
`VITE_NILO_API_MODE` (`live` or `mock`), `VITE_NILO_API_BASE_URL` (an absolute
http or https URL, `http://127.0.0.1:8090` by default), and
`VITE_NILO_MOCK_SCENARIO`, one of `healthy`, `unavailable-inventory`, `empty`,
`unauthorized`, `unavailable`, `malformed-status`, `malformed-integrations`, or
`malformed-devices`, with `malformed` still accepted as an alias of
`malformed-status`. They are read from the committed `.env` and `.env.mock`
files, neither of which holds a credential.

The GUI's own module boundary is `src/api`: `createBenchCoreClient` returns a
`BenchCoreClient` whose methods take a `Credential` — either
`{kind:'session', csrfToken}` or `{kind:'bearer', token}` — and cover the three
bench resources, the sign-in, sign-out and current-session routes, and the
identity and token administration routes. Every failure arrives as a
`BenchCoreError` whose `failure` field is `unauthorized`, `forbidden`,
`rejected`, `unavailable`, or `malformed`, with the stable code Bench Core sent
in `code`. They all share one private `exchange`, so the classification cannot
drift between routes. A session credential sends no `Authorization` header at
all — its secret is in an `HttpOnly` cookie the page cannot read — and adds
`X-NILO-CSRF` on every state-changing request; a bearer credential does the
reverse, and its token is only ever a header value.

The configured client's `baseUrl` is always the empty same-origin base, so it
requests origin-relative paths — the three entries of `BENCH_CORE_PATHS`, the
`/api/v1/auth/*` session routes, and the `/api/v1/users` and `/api/v1/tokens`
administration routes.
`VITE_NILO_API_BASE_URL`
is the endpoint the development server forwards `/api` to and the label the
sign-in screen shows, never a URL the browser fetches. This is what keeps the page
and the backend one origin: an `Authorization` header is not CORS-safelisted, so
a direct cross-origin call would be preflighted, and Bench Core answers no
`OPTIONS` request and sends no CORS headers. An appliance serves the built
assets from Bench Core itself, where the same paths are already same-origin.

## Key behaviors

- `REQ-PLAT-e1a2a591` - pins direct and transitive dependencies with exact
  versions, integrity metadata, and a static Vite production build.
- `REQ-TEST-031b1658` - includes frontend formatting, linting, tests, and build
  in the Windows repository gates.
- `REQ-UX-188a4860` - keeps the NILO development identity, the SIMULATION
  runtime profile, and the physical-equipment boundary statement visible on
  every screen.
- `REQ-FUNC-8a5491d8` - requests the versioned status resource with the entered
  bearer token and returns the reported profile, status, simulation flag, and
  build identity as typed values.
- `REQ-FUNC-3d412917` - separates a rejected token, an unreachable backend, and
  a response outside the supported contract into three distinct causes.
- `REQ-SEC-ed66f6ed` - holds the bearer token in volatile application memory
  only, never in `localStorage`, `sessionStorage`, or a cookie.
- `REQ-UX-40473e43` - presents the sign-in screen's development bearer-token
  form and contacts nothing until a non-empty token is submitted.
- `REQ-UX-66636ac9` - renders in-progress, rejected-token, unreachable,
  unsupported-response, and connected as five separately identifiable screens.
- `REQ-UX-0018c647` - keeps the primary screen inside one viewport, with the
  workspace as the only scroll region.
- `REQ-PLAT-f852c5b2` - resolves the API mode and Bench Core endpoint from the
  environment, failing loudly on an unusable value.
- `REQ-PLAT-b55586c0` - addresses the backend only through same-origin paths and
  forwards them from the development server, so no authenticated request is ever
  preflighted.
- `REQ-TEST-76b94249` - answers from a mock built on the same contract the
  production client parses and routed by API path, so the interface runs with no
  backend and one scenario can spoil a single resource.
- `REQ-FUNC-539355e0` - requests the versioned integration inventory with the
  entered bearer token and returns the scenario and each integration's identity,
  display name, state, simulation flag, and operations as typed values.
- `REQ-FUNC-72bffd89` - requests the versioned device inventory with the entered
  bearer token and returns the scenario and each device's identity, owning
  integration, display name, kind, availability, simulation flag, and
  capabilities as typed values.
- `REQ-UX-8fa0c15e` - presents the backend status, every integration with its
  state, and every device with its owning integration and availability on one
  authenticated screen.
- `REQ-UX-afb79ae0` - states that an inventory is empty when the backend
  legitimately returns no entries, rather than showing a failure.
- `REQ-UX-8169ea1d` - names which resource returned an unusable response and
  keeps the resources that were read successfully on screen.
- `REQ-TEST-0a4f49af` - drives a real browser against a real Bench Core under
  WSL2 and confirms the bearer-token connection, the simulation identity, the
  healthy status and both deterministic inventories.
- `REQ-TEST-0f02f4b8` - stops that backend mid-session, confirms the unreachable
  screen, restarts it and recovers through the interface's own retry control.
- `REQ-TEST-3c4487d6` - fails a browser run and names the repository command
  that supplies each missing prerequisite, instead of skipping.
- `REQ-REL-f04744fa` - waits a bounded time for each endpoint and names the one
  that stayed silent.
- `REQ-REL-f0c8b07c` - stops only the processes that run started, confirms the
  removal of only the directory it created, and fails the run when any part of
  that release did not succeed.
- `REQ-SEC-4884bc91` - keeps the development token in one private field with no
  accessor, and redacts it out of anything a failing action throws.
- `REQ-PLAT-59993b78` - takes free loopback ports and keeps its temporary Bench
  Core state on the WSL Linux filesystem.
- `REQ-UX-f479d8b9` - offers one headless browser command and a headed variant
  for diagnosis.
- `REQ-FUNC-a6c14c23` - signs a human in with a username and password and shows
  the resulting identity and role.
- `REQ-FUNC-e0da3a06` - restores the signed-in state after a reload from the
  current-session resource, because the session secret is in a cookie the page
  cannot read.
- `REQ-FUNC-5ffce0e4` - ends the session server-side on sign-out.
- `REQ-FUNC-5d80ae6c` - keeps the development bearer-token connection available
  without requiring it for a human sign-in.
- `REQ-UX-c7a277c5` - offers the Authentication navigation entry exactly while
  the identity holds the administration permissions, opening one page whose two
  sections list their entries or say they have none.
- `REQ-UX-c4a0c9e7` - keeps that page inside one viewport, with each listing in
  its own scroll compartment.
- `REQ-UX-ae7baff0` - requires an explicit confirmation before a disable, a
  password reset or a revocation.
- `REQ-UX-e60cf375` - discloses a new token secret once, with a copy action, and
  never again.
- `REQ-UX-96852626` - tells an ended session apart from a refused permission.
- `REQ-UX-38017d29` - states that a token cannot exceed its owner's current role.
- `REQ-UX-b849ddb2` - shows every administrative action in progress and then its
  success or the reason Bench Core refused it.
- `REQ-SEC-66b84b2c` - keeps an entered password, a created token secret and the
  session's request-forgery token out of web storage, request URLs and the
  console.
- `REQ-TEST-68dcb172` - drives the administration page against a real Bench Core
  under WSL2 with a real bootstrapped administrator.
- `REQ-UX-5503457a` - carries the wordmark, the tabs and every control on one
  46-pixel row, with no second tab strip and no page title above the workspace.
- `REQ-UX-f300b346` - activates, closes and keyboard-navigates the open page
  tabs, falling back to the nearest remaining neighbour.
- `REQ-UX-30821068` - splits the navigation into the working entries and the
  bottom-anchored Network, Plugins, Authentication, Settings, About cluster.
- `REQ-UX-6da44516` - gives the active page the whole workspace, with no
  decorative background and no document-level scrolling.
- `REQ-UX-0697c0ab` and `REQ-UX-2d00dbdb` - Light, Dark and System over one
  token set, with System following the device in CSS and reacting to a change.
- `REQ-UX-adbe5465` and `REQ-NFR-b3f90f07` - one catalogue per language over one
  key set, selected from the top row, leaving identifiers untranslated.
- `REQ-UX-811bdde9` - reports appliance network state only where the backend
  reported it, and says so plainly where it did not.
- `REQ-DATA-1a14a1dc` - persists exactly the theme and the language, and nothing
  that authenticates.
- `REQ-TEST-842f1943` - proves the shell, a theme and a language across a real
  reload in a browser against a real WSL2 backend.

## Architecture

```mermaid
flowchart LR
    app["App: credential, identity and connection state"]
    signin["SignInScreen: password sign-in and development token"]
    shell["AppShell: one grid, four cells"]
    toprow["TopRow: tabs, network, SIM, appearance, language, account"]
    sidenav["SideNav: working entries and the core cluster"]
    pages["Bench, Network, Authentication, About, integration pages"]
    tokens["styles/tokens.css: one semantic palette, three themes"]
    catalogue["i18n: en, de, ro over one key set"]
    prefs["preferences/store: nilo.theme and nilo.language"]
    admin["AuthenticationPage: users and API tokens"]
    modal["Modal: focused administrative forms"]
    env["config/environment: mode and base URL"]
    client["api/client: resources, session and administration"]
    contract["api/contract: nilo.http/1 types and parsers"]
    mock["api/mock: path-routed contract scenarios and an administration store"]
    proxy["Vite dev server: /api forwarding"]
    backend["WSL2 Bench Core /api/v1/*"]
    app --> signin
    app --> shell
    shell --> toprow
    shell --> sidenav
    shell --> pages
    pages --> admin
    admin --> modal
    toprow --> tokens
    toprow --> catalogue
    tokens --> prefs
    catalogue --> prefs
    env --> client
    env -- "mock mode" --> mock
    client --> contract
    mock --> contract
    app --> client
    admin --> client
    client -- "live mode, same-origin /api/v1/*" --> proxy
    proxy -- "VITE_NILO_API_BASE_URL" --> backend
    client -- "mock mode" --> mock
    harness["e2e/topology: ports, WSL2 backend, Vite, in-memory token, bootstrapped administrator"]
    prereq["e2e/prerequisites: fail, never skip"]
    spec["e2e/bench-core.spec: the browser journey"]
    adminspec["e2e/authentication.spec: sign-in and administration"]
    browser["Chromium from 02_Tools/runtime"]
    harness --> prereq
    harness -- "starts, stops, removes its own" --> backend
    harness -- "starts with the chosen backend port" --> proxy
    spec --> harness
    adminspec --> harness
    spec --> browser
    adminspec --> browser
    browser --> proxy
```

`App` owns the whole conversation: which credential is in use, who it belongs
to, and which page is open. Its shell is `restoring`, `signed-out`,
`connecting`, `refused`, or `active`. Every page load starts in `restoring`,
because the only way to learn whether somebody is signed in is to ask Bench Core
who the browser's cookies belong to. Nothing outside `api/contract.ts` knows the
wire field names, and only `AuthenticationPage` calls the network besides the
shell.

Hiding the Authentication navigation entry is presentation, never a permission:
Bench Core authorizes every request on its own, so an identity that reaches the
page another way is refused there and told so.

The three resources are requested concurrently on one token. A status that
cannot be read settles the whole screen as `failed`, because without it neither
the token nor the backend nor the contract can be trusted for anything else.
Once the status is in, each inventory is carried as its own `Resource<T>` —
value or failure — which is what lets an unusable integration inventory be told
from an unusable device inventory while the status and the other inventory stay
visible.

## Files

- `05_Software/nilo_gui/package.json` and `package-lock.json` - exact commands,
  dependency versions, transitive resolution, and integrity hashes.
- `05_Software/nilo_gui/.env` and `.env.mock` - committed build-time endpoint
  configuration for the WSL2-backed and mock-only development modes.
- `05_Software/nilo_gui/src/api/contract.ts` - the `nilo.http/1` constants, the
  origin-relative paths, the status, integration, device, session, identity and
  token types, and the strict parsers shared by the client and the mock.
- `05_Software/nilo_gui/src/api/client.ts` - the typed client's fourteen methods
  over one shared exchange, the `Credential` union, and `BenchCoreError` with
  its five causes.
- `05_Software/nilo_gui/src/api/csrf.ts` - reads the readable `nilo_csrf`
  cookie, the only part of the session credential the page can see.
- `05_Software/nilo_gui/src/api/permissions.ts` - whether a session has any
  reason to open the Authentication page.
- `05_Software/nilo_gui/src/api/mock.ts` - path-routed, contract-built answers
  for the eight development scenarios, plus an in-memory identity and token
  store enforcing the same role matrix, CSRF rule and last-administrator rule
  the backend does; the ready and unavailable inventories reproduce the Go
  simulation catalog exactly.
- `05_Software/nilo_gui/src/config/environment.ts` - environment reader,
  `GuiConfigurationError`, and the configured-client factory.
- `05_Software/nilo_gui/src/App.tsx` - session restoration, the held credential,
  the concurrent three-resource request, and the unauthenticated and
  not-yet-connected screens.
- `05_Software/nilo_gui/src/styles/tokens.css` - the one semantic palette,
  defined for light, for dark, and for the system theme's dark branch.
- `05_Software/nilo_gui/src/styles/shell.css` - the shell grid and every
  component in it, declaring no literal colour.
- `05_Software/nilo_gui/src/preferences/store.ts` - the only durable browser
  storage this interface uses, and the two keys it holds.
- `05_Software/nilo_gui/src/theme/` and `src/i18n/` - the theme and language
  providers, their contexts, and the three message catalogues.
- `05_Software/nilo_gui/src/components/shell/` - `AppShell`, `TopRow`,
  `TabStrip`, `SideNav`, `NetworkSummary`, `Menu`, `Icon`, the page registry and
  the tab state.
- `05_Software/nilo_gui/src/components/pages/` - `BenchPage`, `NetworkPage` and
  the About, not-yet-available and integration pages.
- `05_Software/nilo_gui/src/components/SignInScreen.tsx` - the unauthenticated
  screen with both forms.
- `05_Software/nilo_gui/src/components/AuthenticationPage.tsx`,
  `AdministrationForms.tsx` and `Modal.tsx` - the two bounded administration
  panels, the dialog bodies for every change, and the focused dialog itself with
  its focus handling and Escape dismissal.
- `05_Software/nilo_gui/src/App.css` and `src/index.css` - single-viewport
  layout, the inventory scroll compartment, and the CSS custom properties every
  colour is expressed through.
- `05_Software/nilo_gui/src/App.test.tsx`,
  `src/components/AuthenticationPage.test.tsx`, `src/api/client.test.ts`,
  `src/api/contract.test.ts`, `src/api/mock.test.ts`,
  `src/config/environment.test.ts`, and `src/layout.test.ts` - the shell,
  administration-page, client, contract, mock, configuration, and layout tests.
- `05_Software/nilo_gui/src/test/setup.ts` - jest-dom matchers and the
  Testing Library cleanup Vitest does not register without injected globals.
- `05_Software/nilo_gui/vite.config.ts` - local development server, the `/api`
  forwarding for both serve and preview, Vitest, and static build configuration.
- `05_Software/nilo_gui/src/config/devServer.test.ts` - asserts that forwarding
  is declared and takes its target from the environment.
- `05_Software/nilo_gui/playwright.config.ts` - the integrated browser runner:
  headless, serial, no retries, and no retained trace, video or screenshot.
- `05_Software/nilo_gui/e2e/topology.ts` - one run's own Bench Core under WSL2
  and interface server on Windows: free loopback ports, a `mktemp` directory on
  the WSL Linux filesystem, the in-memory token, bounded readiness waits, and a
  teardown that signals only its own handles and removes only its own directory.
- `05_Software/nilo_gui/e2e/prerequisites.ts` - the distribution, checkout and
  browser checks, each failing with the repository command that supplies it.
- `05_Software/nilo_gui/e2e/bench-core.spec.ts` - the bearer-token browser
  journey, in one serial session: connect, status, simulation identity, both
  inventories, the unreachable screen, and recovery.
- `05_Software/nilo_gui/e2e/authentication.spec.ts` - the human browser journey
  against a real administrator the harness bootstraps through the real command
  line: password sign-in, identity creation, a role change, a token with its
  one-time secret, restoration across a reload, revocation, the
  last-administrator refusal, the entry a Viewer does not get, and sign-out.
- `05_Software/nilo_gui/e2e/token-redaction.spec.ts` - the forced-failure
  regression: a real browser action made to fail after its locator resolves,
  asserting the redaction wrapper removes the value and that the unwrapped call
  would still have leaked it.
- `05_Software/nilo_gui/e2e/topology.test.ts`, `e2e/prerequisites.test.ts` and
  `e2e/commands.test.ts` - the harness, prerequisite and command-surface tests,
  which run in the component suite and start no process of their own.
- `05_Software/nilo_gui/public/favicon.svg` - repository-owned browser icon.
- `05_Software/nilo_gui/README.md` - exact local restore, development, quality,
  build, configuration, and authentication commands.
- `05_Software/scripts/aegis_test.py` - Windows repository-gate integration.
