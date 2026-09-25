# Browser GUI foundation

## Routing

Select this page for the React shell, pages, API client, accessibility and
localization. Start at `src/components/shell/AppShell.tsx`; focused tests sit
beside each page. Instrument lifecycle and transaction views start at
`src/components/pages/PluginsPage.tsx` and `PluginsPage.test.tsx`. Core scheduling,
permissions and runner behavior belong to [Managed bench integrations](bench-integrations.md).

## Purpose and boundaries

The browser GUI foundation provides NILO's native Windows development
interface while keeping the deployed appliance boundary static. It gives
developers a locked, testable React and TypeScript project in which a human
signs in with a username and password over the server-managed browser session,
reads backend status and the selected runtime's integration and device inventory,
and — as an administrator — administers identities and API tokens, including
their permanent deletion, without ever
suggesting that a simulated result came from physical equipment.

## Public interface

The project root is `05_Software/frontend`. Developers use its npm scripts
through `02_Tools/env/with-tools.ps1`: `dev` serves the interface against a real
backend, `dev:mock` serves it against the built-in contract mock, `format:check`
and `lint` enforce source quality, `test:ci` runs deterministic component tests,
and `build` (or `build:mock`) emits static production assets into ignored
`dist/`. The `test:e2e` scripts are internal runner surfaces for the privileged
appliance verification stage; the supported acceptance entry point is
`02_Tools/tests/appliance/check.sh`, which builds the assets and drives the
appliance-profile HTTPS origin from its pinned container browser.

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
`VITE_NILO_API_BASE_URL` is the endpoint the development server forwards `/api`
to, never a URL the browser fetches directly. This keeps the page and backend
one origin: an `Authorization` header is not CORS-safelisted, so a direct
cross-origin call would be preflighted, and Bench Core answers no `OPTIONS`
request and sends no CORS headers. An appliance serves the built assets from
Bench Core itself, where the same paths are already same-origin.

## Runtime presentation

Built assets served by Bench Core receive a `nilo-runtime-profile` HTML meta tag.
Authenticated status remains authoritative for the workspace runtime badge and
reported inventory. The metadata contains no network inventory and grants no
permissions. Before authentication, the product identity uses the project
version from the checked-in GUI package mirror and does not request protected
runtime status.

## Invariants

- While the appliance reports it is provisioning, the Network page shows the
  first-boot commissioning workflow in place of the management allowlist editor,
  never both: every change the editor could make is refused by the appliance
  until commissioning ends. An appliance that does not report the state at all is
  treated as commissioned.
- Page-scoped Activity actions use `.page-title-actions` inside `.page-head`:
  title first, authorized Activity button next, supporting description below.
  This shared wrapping, left-aligned pattern applies to Network, Authentication
  and future pages offering that action (REQ-UX-b75fbbc3). Keep resource-create
  and refresh actions in their existing list toolbar and record actions local.
- All live requests use the same-origin authenticated client and its CSRF rules.
- Simulation labels stay explicit; browser state never proves physical accuracy.
- Instrument submission retries preserve a command key until a response arrives;
  history reattaches by transaction ID and polling never submits work.
- The server authorizes every action even when UI controls are hidden or disabled.

## Key behaviors

- `REQ-UX-b75fbbc3` - associates page-scoped Activity with the title using the
  shared responsive header group, while preserving existing Activity context.
- `REQ-UX-dd4b2c16` - keeps the whole first Activity record inside a 1024x600
  viewport with the disclosures closed.
- `REQ-UX-3e01a5b6` - the Activity header is the title and, for an exporter,
  Export at its right, and no further descriptive text.
- `REQ-UX-c5976f3b` - reports only the recording conditions that qualify the
  Activity listing, and nothing while neither holds.
- `REQ-UX-f8c58c4c` - lays Activity scope, filter fields and More filters out as
  one wrapping line where the panel is shown.
- `REQ-UX-ba46f6d3` - keeps Activity scope selection outside the filter
  disclosure where the viewport collapses the panel.
- `REQ-UX-0291ba55` - places the administrator Activity retention disclosure
  between the header and the filters.
- `REQ-UX-7781cc4e` - offers 10, 25, 50, 100 and all records a page and opens
  Activity at 10.
- `REQ-PLAT-e1a2a591` - pins direct and transitive dependencies with exact
  versions, integrity metadata, and a static Vite production build.
- `REQ-TEST-031b1658` - includes frontend formatting, linting, tests, and build
  in the Windows repository gates.
- `REQ-UX-5e428717` - offers the appliance password policy on the Authentication
  page, for a session holding `password.policy.manage`, as a disclosure that
  starts closed; inside it the minimum length and four class checkboxes share one
  wrapping line. The bounds stay the appliance's: a refusal is shown as the
  appliance wrote it and the shown policy returns to what is stored.
- `REQ-UX-c117cbf9` - the Authentication identities page carries no standing
  paragraph about the signed-in actor or the row actions; that guidance stays in
  each control's contextual help. The API tokens page keeps its own description.
- `REQ-UX-188a4860` - keeps one persistent runtime badge after authentication.
- `REQ-FUNC-8a5491d8` - requests the versioned authenticated status resource
  and returns build identity plus the optional bounded Linux, architecture and
  NILO-data-filesystem capacity snapshot as typed values.
- `REQ-FUNC-3d412917` - separates a rejected token, an unreachable backend, and
  a response outside the supported contract into three distinct causes.
- `REQ-SEC-ed66f6ed` - holds the bearer token in volatile application memory
  only, never in `localStorage`, `sessionStorage`, or a cookie.
- `REQ-UX-40473e43` - presents the NILO product identity, labelled human
  credentials, sign-in action, and actionable authentication feedback.
- `REQ-UX-8a1ddb96` - displays the canonical project version before sign-in
  from the checked-in GUI package mirror.
- `REQ-UX-10182ba0` - keeps the password masked initially and offers an
  accessible, value-preserving reveal and hide control.
- `REQ-UX-fdb7c574` - keeps the complete sign-in surface readable and operable
  from an 800x480 field display through a 1920x1080 desktop viewport.
- `REQ-UX-66636ac9` - distinguishes in-progress, ended-session, unreachable,
  unsupported-response, and connected authenticated resource states.
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
- `REQ-TEST-0a4f49af` is retired. `REQ-TEST-68eedecb` replaces its simulation
  journey with a real browser driving a real appliance-profile Bench Core,
  privileged helper and NetworkManager through password authentication.
- `REQ-TEST-b1177191` - drives the built assets from Bench Core's own HTTPS
  origin without a Vite server or proxy in the acceptance topology.
- `REQ-TEST-0f02f4b8` - stops that backend mid-session, confirms the unreachable
  screen, restarts it and recovers through the interface's own retry control.
- `REQ-TEST-3c4487d6` - fails a browser run and names the repository command
  that supplies each missing prerequisite, instead of skipping.
- `REQ-REL-f04744fa` - bounds appliance-core readiness and names the endpoint
  that stayed silent.
- `REQ-REL-f0c8b07c` - stops only the processes that run started, confirms the
  removal of only the directory it created, and fails the run when any part of
  that release did not succeed.
- The appliance harness keeps fixture passwords behind its redacted input
  boundary and retains no browser traces, screenshots or videos.
- Each run uses disposable container state and an isolated browser network
  namespace, then removes only the resources it created.
- `REQ-FUNC-a6c14c23` - signs a human in with a username and password and shows
  the resulting identity and role.
- `REQ-FUNC-e0da3a06` - restores the signed-in state after a reload from the
  current-session resource, because the session secret is in a cookie the page
  cannot read.
- `REQ-FUNC-5ffce0e4` - ends the session server-side on sign-out.
- `REQ-FUNC-5d80ae6c` - offers one username-and-password credential submission
  path before sign-in.
- `REQ-UX-c7a277c5`, `REQ-UX-93e1b747` - make Authentication an expandable,
  permission-aware System group whose Users and API tokens destinations own
  distinct workspace tabs and resource reads.
- `REQ-UX-7c42d7ba`, `REQ-UX-833c9cd3` - keep primary user and token facts in
  responsive records, secondary safe metadata under Details, and mutations in
  record-aware Actions dialogs that explain unavailable choices.
- `REQ-UX-c4a0c9e7` - keeps each focused listing in normal workspace flow and
  converts the semantic table to labelled cards below 56rem without a nested
  list scrollbar.
- `REQ-UX-ae7baff0` - requires an explicit confirmation before a disable, a
  password reset or a revocation.
- Permanent deletion of an identity or a token sits in the destructive part of
  the same Actions dialog, textually distinct from Disable and from Revoke. Its
  dialog states what is removed, keeps a warning that it cannot be undone
  visible while the field is being filled, and sends nothing until the exact
  username or token name has been typed back. Dismissing it sends nothing at
  all. The dialog is presentation: Bench Core enforces the last-enabled-
  administrator invariant, the owned-token refusal and the audit record inside
  the transaction that applies the removal. An unavailable deletion states its
  reason beside the control, and an administrator deleting their own account is
  returned to sign-in because the session applying it stops authenticating.
  After a successful deletion only the focused listing is re-read.
- `REQ-UX-e60cf375` - discloses a new token secret once, with a copy action, and
  never again.
- `REQ-UX-96852626` - tells an ended session apart from a refused permission.
- `REQ-UX-38017d29` - states that a token cannot exceed its owner's current role.
- `REQ-UX-ed735941`, `REQ-UX-2b0b6d59`, `REQ-UX-4570c081` - reuse the shared
  Field, help popover and workflow-help contracts for authentication while
  keeping validation, impact and irreversible warnings persistently visible.
- `REQ-UX-d697e8ec` - opens Activity from either destination with the identity
  component filter already applied.
- `REQ-UX-b849ddb2` - shows every administrative action in progress and then its
  success or the reason Bench Core refused it.
- `REQ-SEC-66b84b2c` - keeps an entered password, a created token secret and the
  session's request-forgery token out of web storage, request URLs and the
  console.
- `REQ-TEST-68dcb172` - drives the administration page against the real
  appliance-stage Bench Core with a real bootstrapped administrator.
- `REQ-UX-5503457a` - carries the wordmark, the tabs and every control on one
  46-pixel row, with no second tab strip and no page title above the workspace.
- `REQ-UX-f300b346` - activates, closes and keyboard-navigates the open page
  tabs, falling back to the nearest remaining neighbour.
- `REQ-UX-30821068` - splits the navigation into the working entries and the
  bottom-anchored Plugins, Network, Authentication, Activity, About cluster.
- `REQ-UX-b718864e` - expands Network into four focused, separately tabbed
  destinations and keeps the group usable in the rail and drawer.
- `REQ-UX-b555166c` - combines the decision summary and adapter configuration on
  Overview, keeps the other Network concerns focused, and requests traffic
  history only from Diagnostics.
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
- `REQ-UX-144bf035` - About places permanent identity beside its title and the
  friendly-name/Identify controls first, then groups software/API/protocol,
  Linux/kernel/architecture/storage and management facts into three accessible
  regions. Its scroll rail stays at the workspace edge, and hardware addresses
  appear only inside the exact printable label embedded beside the facts.
- `REQ-TEST-842f1943` - proves the shell, a theme and a language across a real
  reload in the appliance-stage browser.

## Change map

| Change requested | Start with | Focused verification |
| --- | --- | --- |
| Page-title Activity layout | `.page-title-actions` in shell.css; UsersPage, ApiTokensPage and NetworkPage headers | AuthenticationPage.test.tsx; field-display.spec.ts |
| Activity page furniture: header, filter line, retention, page size | `pages/ActivityPage.tsx`; `.activity-head`, `.activity-filters` in shell.css | ActivityPage.test.tsx; layout.test.ts; field-display.spec.ts |
| Contextual Activity navigation and permissions | AppShell.tsx `openActivity` | shell.test.tsx |
| Networking inventory and managed changes | NetworkPage.tsx and network/ | NetworkPage.test.tsx; network.spec.ts |
| Appliance identity, system facts, embedded label, friendly name and Identify | SimplePages.tsx `AboutPage`; api/appliance.ts; api/contract.ts status parser | AboutPage.test.tsx; api/client.test.ts; api/appliance.test.ts; field-display.spec.ts |
| Identity/token forms | authentication/ and AdministrationForms.tsx | AuthenticationPage.test.tsx; authentication.spec.ts |
| Appliance password policy | authentication/PasswordPolicyPanel.tsx; `.admin-policy*` in shell.css; `getPasswordPolicy`/`setPasswordPolicy` in api/client.ts | AuthenticationPage.test.tsx |

## Architecture

```mermaid
flowchart LR
    app["App: credential, identity and connection state"]
    signin["SignInScreen: product version, password sign-in and visibility"]
    shell["AppShell: one grid, four cells"]
    toprow["TopRow: tabs, network, SIM, appearance, language, account"]
    sidenav["SideNav: working entries plus keyed Network and Authentication groups"]
    pages["Bench, four Network views, Activity, focused Authentication views, About, integrations"]
    tokens["styles/tokens.css: one semantic palette, three themes"]
    catalogue["i18n: en, de, ro over one key set"]
    prefs["preferences/store: nilo.theme and nilo.language"]
    admin["AuthenticationPage(view): fetch-free dispatcher"]
    users["UsersPage: identities and user mutations"]
    apitokens["ApiTokensPage: safe token records and one-time disclosure"]
    netpage["NetworkPage(view): shared operations plus one focused concern"]
    modal["Modal: focused administrative forms"]
    env["config/environment: mode and base URL"]
    client["api/client: resources, session and administration"]
    contract["api/contract: nilo.http/1 types and parsers"]
    mock["api/mock: path-routed contract scenarios and an administration store"]
    proxy["Vite dev server: /api forwarding"]
    backend["Development Bench Core /api/v1/*"]
    appliance["Appliance stage: NetworkManager, helper, appliance-profile core"]
    app --> signin
    app --> shell
    shell --> toprow
    shell --> sidenav
    shell --> pages
    pages --> instruments["PluginsPage: lifecycle, acquisition, transaction detail"]
    instruments --> client
    pages --> admin
    admin --> users
    admin --> apitokens
    pages --> netpage
    netpage --> client
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
    harness["e2e/topology: appliance core, bounded lifecycle, bootstrapped administrator"]
    prereq["e2e/prerequisites: fail, never skip"]
    spec["e2e/bench-core.spec: the browser journey"]
    adminspec["e2e/authentication.spec: sign-in and administration"]
    browser["Pinned Chromium in isolated appliance browser namespace"]
    harness --> prereq
    harness -- "starts, stops, removes its own" --> appliance
    spec --> harness
    adminspec --> harness
    spec --> browser
    adminspec --> browser
    browser -- "built HTTPS origin" --> appliance
```

`AppShell` binds the Network read/change callbacks and maps four page descriptors
to one view-discriminated `NetworkPage`. `SideNav` renders Network as an expanded
group by default, with Overview, Device access, VPN profiles and Diagnostics
opening independent workspace tabs. Overview puts the decision summary first,
then one shared `NetworkInterfacesTable` with management selectors, adapter
observations, and Details/Actions. Secondary URL guidance is collapsed; the
management outcome occupies one status line. The table keys
physical rows by stable identity, labels absent adapters with that identity, and
keeps an absent adapter's reused name from selecting its replacement. An absent
selection with a reused name stays checked and disabled independently of the
replacement until its name is unambiguous; active and rollback ownership share
one row per physical identity.
`NetworkPage` reattaches and
polls applying, confirmation, rollback and rollback-failed operations on every
view, requests traffic history only on Diagnostics, renders a countdown and uses
`NetworkConfirmation` for native modal focus containment.
`WireGuardPanel` proposes profile changes without
applying them; acknowledgement is separate. `api/wireguard.ts` validates the
wire contract and exposes only safe profile metadata. `e2e/network.spec.ts`
also covers `readiness_only`: the panel reports readiness without offering profile
controls or claiming an empty inventory, and the summary keeps tunnel and handshake
state unreported. The marker is optional for older/full responses. The same spec
also verifies acknowledged tool installation, `InstallationProgress` phases,
disabled competing actions and terminal success. An optional operation `progress`
string is parsed at the API boundary; no raw package command output is displayed.
The install capability is independent of profile-management readiness, including
when kernel evidence is unknown. Actual APT behavior has separate Linux tests.
The spec exercises the real appliance-stage browser flow including the full
120-second rollback window. The browser reacquires the appliance's authoritative
operation after an uncertain acknowledgement and after reload. A matching
unfinished operation replaces any older terminal notice, preserving a rollback
failure until the backend reports verified recovery.

`App` owns the whole conversation: which credential is in use, who it belongs
to, and which page is open. Its shell is `restoring`, `signed-out`,
`connecting`, `refused`, or `active`. Every page load starts in `restoring`,
because the only way to learn whether somebody is signed in is to ask Bench Core
who the browser's cookies belong to. The API adapters own wire parsing; pages receive typed values and callbacks.
Network changes carry only their typed request parameters to those adapters.

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

- `src/components/network/CommissioningPanel.tsx` - the first-boot workflow: the standing temporary-exposure alert, the eligible adapters with their stable identities and addresses, the current browser path, the usable URLs and their scope, and the completion control with its confirmation step.

- `src/api/wireguard.ts` - bounded readiness/profile parsing and typed change requests.
- `src/components/network/{WireGuardPanel,NetworkConfirmation}.tsx` - profile controls and native acknowledgement modal.
- `e2e/network.spec.ts` - isolated real-backend Network browser scenarios.

- `05_Software/frontend/package.json` and `package-lock.json` - exact commands,
  dependency versions, transitive resolution, and integrity hashes.
- `05_Software/frontend/.env` and `.env.mock` - committed build-time endpoint
  configuration for the WSL2-backed and mock-only development modes.
- `05_Software/frontend/src/api/contract.ts` - the `nilo.http/1` constants, the
  origin-relative paths, the status, integration, device, session, identity and
  token types, and the strict parsers shared by the client and the mock.
- `05_Software/frontend/src/api/client.ts` - the typed client's fourteen methods
  over one shared exchange, the `Credential` union, and `BenchCoreError` with
  its five causes.
- `05_Software/frontend/src/api/csrf.ts` - reads the readable `nilo_csrf`
  cookie, the only part of the session credential the page can see.
- `05_Software/frontend/src/version.ts` - exposes the checked-in GUI package
  version for the pre-authentication product identity.
- `05_Software/frontend/src/api/permissions.ts` - whether a session has any
  reason to open the Authentication page.
- `05_Software/frontend/src/api/mock.ts` - path-routed, contract-built answers
  for the eight development scenarios, plus an in-memory identity and token
  store enforcing the same role matrix, CSRF rule and last-administrator rule
  the backend does; the ready and unavailable inventories reproduce the Go
  simulation catalog exactly.
- `05_Software/frontend/src/config/environment.ts` - environment reader,
  `GuiConfigurationError`, and the configured-client factory.
- `05_Software/frontend/src/App.tsx` - session restoration, the held credential,
  the concurrent three-resource request, and the unauthenticated and
  not-yet-connected screens.
- `05_Software/frontend/src/styles/tokens.css` - the one semantic palette,
  defined for light, for dark, and for the system theme's dark branch.
- `05_Software/frontend/src/styles/shell.css` - the shell grid and every
  component in it, declaring no literal colour.
- `05_Software/frontend/src/preferences/store.ts` - the only durable browser
  storage this interface uses, and the two keys it holds.
- `05_Software/frontend/src/theme/` and `src/i18n/` - the theme and language
  providers, their contexts, and the three message catalogues.
- `05_Software/frontend/src/components/shell/` - `AppShell`, `TopRow`,
  `TabStrip`, `SideNav`, `NetworkSummary`, `Menu`, `Icon`, the page registry and
  the tab state.
- `05_Software/frontend/src/components/pages/` - `BenchPage`, `NetworkPage` and
  the About, not-yet-available and integration pages. About reads the
  authenticated appliance resource and presents permanent/friendly names,
  management addresses, the printable label, friendly-name administration and
  the permission-gated Identify action. Identify decodes the versioned
  `supported` response and absolute `active_until` deadline; About controls and
  management links retain 44-pixel targets at field-display sizes.
- `05_Software/frontend/src/components/SignInScreen.tsx` - the unauthenticated
  screen with both forms.
- `05_Software/frontend/src/components/AuthenticationPage.tsx`,
  `src/components/authentication/`, `AdministrationForms.tsx` and `Modal.tsx` -
  the fetch-free view dispatcher, focused Users and API tokens resource owners,
  shared dialog bodies for every change, and the focused dialog itself with its
  focus handling and Escape dismissal. `authentication/PasswordPolicyPanel.tsx`
  sits on the Users view for a session that may administer the policy, as a
  closed `<details>` following the Activity retention disclosure; its number
  field carries no min or max, because the bounds belong to the appliance and a
  browser restating them would drift from it.
- `05_Software/frontend/src/App.css` and `src/index.css` - single-viewport
  layout, the inventory scroll compartment, and the CSS custom properties every
  colour is expressed through.
- `05_Software/frontend/src/App.test.tsx`,
  `src/components/AuthenticationPage.test.tsx`, `src/api/client.test.ts`,
  `src/api/contract.test.ts`, `src/api/mock.test.ts`,
  `src/config/environment.test.ts`, and `src/layout.test.ts` - the shell,
  administration-page, client, contract, mock, configuration, and layout tests.
- `05_Software/frontend/src/test/setup.ts` - jest-dom matchers and the
  Testing Library cleanup Vitest does not register without injected globals.
- `05_Software/frontend/vite.config.ts` - local development server, the `/api`
  forwarding for both serve and preview, Vitest, and static build configuration.
- `05_Software/frontend/src/config/devServer.test.ts` - asserts that forwarding
  is declared and takes its target from the environment.
- `05_Software/frontend/playwright.config.ts` - the integrated browser runner:
  headless, serial, no retries, and no retained trace, video or screenshot.
- `05_Software/frontend/e2e/topology.ts` - one run's appliance-profile Bench
  Core, privileged helper boundary, generated TLS material, temporary database,
  bootstrapped administrator, bounded readiness and shutdown waits, and teardown
  limited to its own child processes and state.
- `05_Software/frontend/e2e/prerequisites.ts` - the appliance-stage binaries,
  built assets and runtime configuration checks, each failing with the stage
  action that supplies it.
- `05_Software/frontend/e2e/bench-core.spec.ts` - the password-authenticated
  appliance journey: status, empty fresh-appliance inventories, the unreachable
  screen, and recovery.
- `05_Software/frontend/e2e/authentication.spec.ts` - the human browser journey
  against a real administrator the harness bootstraps through the real command
  line: password sign-in, identity creation, a role change, a token with its
  one-time secret, restoration across a reload, revocation, the
  last-administrator refusal, the entry a Viewer does not get, and sign-out.
- `05_Software/frontend/e2e/token-redaction.spec.ts` - the forced-failure
  regression: a real browser action made to fail after its locator resolves,
  asserting the redaction wrapper removes the value and that the unwrapped call
  would still have leaked it.
- `05_Software/frontend/e2e/topology.test.ts`, `e2e/prerequisites.test.ts` and
  `e2e/commands.test.ts` - the harness, prerequisite and command-surface tests,
  which run in the component suite and start no process of their own.
- `05_Software/frontend/public/favicon.svg` - repository-owned browser icon.
- `05_Software/frontend/README.md` - exact local restore, development, quality,
  build, configuration, and authentication commands.
- `02_Tools/scripts/aegis_test.py` - Windows repository-gate integration.

## First-boot commissioning (2026-09-12, TASK-38d62913)

The management-access payload carries `provisioning`, and the browser parser
reads it as `body.provisioning === true` so an appliance that omits it is
treated as commissioned - the safe direction, because it shows the ordinary
allowlist editor rather than offering to commission an appliance that already
is. `NetworkPage` renders `CommissioningPanel` when that flag is set and
suppresses the allowlist editor in exactly the same condition; the two are
mutually exclusive because while provisioning the appliance refuses every change
the editor could make.

The panel opens with a standing alert naming the exposure as temporary, then
lists each adapter that may carry management access with its name, stable
adapter identity, type, connection state and current addresses, marking the one
this browser session arrived on. An interface that cannot carry management
access is listed with the appliance's own reason and offers no control at all.
The URL section shows the usable appliance URLs together with the facts that the
local name is answered only on the local network and that addresses are not
permanent identifiers.

Completing commissioning sends the chosen adapters. An empty choice is refused
in the browser before anything is sent. Giving up the adapter the session
arrived on raises a second, explicit button naming it, so the consequence is
read before it is accepted. A refusal from the appliance is shown with the
selection kept, so the administrator corrects rather than restarts. The control
is withheld from an identity without the manage permission, and
`completeCommissioning` is withheld entirely under the simulation profile, which
has no eligible physical interface to commission.

## Compact Network overview (2026-09-03)

The authenticated shell uses one SIM/LIVE badge, without a footer disclaimer. Network lists show addresses and gateways, plus WireGuard configured addresses and peer endpoints. The API parser retains only public profile metadata. DNS, routes, handshake and readiness diagnostics are collapsed; Import profile opens the form. Completed notices are dismissible; pending confirmation, rollback failures and stale data remain explicit. Gateway fields are additive; an older running backend leaves them unreported until restarted. Do not restart a user simulation session merely to populate these fields: simulated profiles are in memory.

## Device networks, adapter configuration and Activity (2026-09-04)

- `src/api/devicenetworks.ts`, `src/api/activity.ts`, `src/api/settings.ts`
  parse the device-network, activity/diagnostics and adapter-settings resources;
  `contract.ts` gained `adapter_id`, `ssid` and `settings` on an interface, the
  operations listing, and the `activity.read`, `activity.export` and
  `diagnostics.manage` permissions; `wireguard.ts` gained the `rename` change.
  The client exposes one method per route and `exportActivity` returns the
  export as the text the appliance wrote.
- `src/api/mock.ts` answers every new route through the same acknowledged
  lifecycle, keeps device networks, sessions, the policy and an activity
  history in memory, and gives device-network and Wi-Fi changes a confirmation
  window as the appliance does.
- `components/network/DeviceNetworksPanel.tsx`: table (intent and observation
  are separate columns), editor dialog, enable/disable/delete through previews,
  check and instructions dialogs, activity link narrowed to the network.
  `InterfaceSettingsDialog.tsx`: addressing form and, on a wireless adapter,
  scan and join with a password that is typed once and never displayed.
  `WireGuardPanel.tsx`: rename in the profile details.
- `pages/ActivityPage.tsx`: scope, filters and the More filters disclosure on
  one wrapping line, server paging by cursor, the recording conditions that
  qualify the listing, JSONL/CSV export from the page header, and the
  administrator retention policy above the filters. `shell/pages.ts` places the `activity` core page below
  Authentication, shown to an identity holding `activity.read`; `AppShell` keeps the filter a Network
  page link chose and remounts the page with it.
- `pages/NetworkPage.tsx` reads `GET /api/v1/network/operations` on mount and
  picks up a change still applying, awaiting confirmation, rolling back or
  blocked in rollback failure; it keeps polling the failure until verified
  recovery. An uncertain acknowledgement reacquires the matching authoritative
  operation and replaces any older terminal notice, while a refused preview is
  shown with the appliance's reason rather than as an outage.
- Catalogues: 154 keys added to en, de and ro; `messages.test.ts` lists the
  words the languages share.

## Compact field interface and contextual help (2026-09-04, TASK-9ca3f883)

The interface is sized for a seven-inch appliance display (`1024x600` primary,
`800x480` fallback) as well as a desktop monitor, and no backend, contract,
permission or confirmation behaviour changed with it.

- `REQ-UX-c8c16d33` - one semantic type scale in `styles/tokens.css`
  (`--text-title` 22px, `--text-section` 18px, `--text-body` 16px,
  `--text-dense` 15px, `--text-label` 14px with their line heights); every
  `font-size` in `styles/shell.css` names a token, nothing operator-facing is
  below 14px, and no heading is set in capitals with widened tracking.
- `REQ-UX-c5f7c0cd` - `--control-height` and `--target-min` are 44px; one base
  `button, .button` rule, 44px inputs and selects, 22px checkboxes inside
  44px labels, and a tab close control the height of the row.
- `REQ-UX-2687f591` - `shell/useViewportClass` reads the same two media queries
  the stylesheet uses and returns `wide | compact | constrained`. Compact
  narrows the rail to `--nav-width-compact` and keeps every label; constrained
  hides the rail, shows the `Menu` control and opens `shell/NavDrawer` (a
  `Modal` with `variant="drawer"`), and `TopRow` folds the appearance and
  language menus into the account menu while keeping the tabs, the network
  summary and the one `SIM`/`LIVE` indicator.
- `REQ-UX-2de87216` - `Modal` is a head (title and `Close`), a scrolling
  `.modal-body`, and a `.modal-actions` bar stuck to its bottom; below 56rem or
  640px it is a full-screen sheet. `components/layers.ts` keeps the stack of
  dismissible layers so Escape closes only the top one: a help popover over an
  editor closes the popover.
- `REQ-UX-2b0b6d59` - `help/HelpPopover` opens on click, tap, Enter, hover and
  focus, exposes `aria-expanded` and `aria-controls`, closes on Escape, outside
  activation and its own control with focus returned, closes any other help
  through one document event, and positions itself from the trigger's rectangle
  so it stays in the viewport. No explanation is left in a `title` attribute.
- `REQ-UX-4570c081` - `help/Field` renders label, help, control, persistent
  guidance and the validation message in reading order, tied to the control by
  `aria-describedby`. The confirmation window, countdown, rollback failure,
  refusal and route prerequisites stay in notices and dialogs.
- `REQ-UX-1860069d` - `help/WorkflowHelp` opens "How to configure" over `Modal`
  with six catalogue sections (`help.dn.*`, `help.if.*`): goal, values needed,
  addressing example, what NILO changes, what must exist elsewhere, verify and
  recover.
- `REQ-UX-cde0b9e2` - `network/NetworkOverview` states the uplink and its
  address, the device side, the active profile or None, and what needs
  attention, computed from the status, the device networks, the WireGuard status
  and the running operation. `network/addressing.ts` holds `uplinkOf`,
  `isAddress`, `isPrefix` and `subnetOf`.
- `REQ-UX-ff989933` - one `table.data-table.record-table` per list with
  `data-label` on each cell; below 56rem the stylesheet turns rows into labelled
  cards. Secondary values live in an explicit details row announced as a region;
  `components/RowActions.tsx` gives each row one Actions control that opens a
  compact `Modal` attached to the document body. It identifies the object type
  and status, gives every choice a full-width target, keeps disabled entries and
  their reasons visible, separates destructive actions, and cannot be clipped
  by the table scroll compartment.
- `REQ-UX-8859ce12` - the device-network editor is Identity, Device side, Access
  path, Addressing, Services, a collapsed Advanced and a Review that names the
  adapter and address, the authorized sources and target, whether WireGuard is
  involved, whether source NAT or preservation is used and whether the
  management path is affected. Format validation (address, prefix, source list,
  port range) runs before any preview and appears after the field; ports stay
  editable with 502 as an example.
- `REQ-UX-f30891b4` - Bench, Activity, Authentication and About use the same
  classes; Activity keeps time, outcome, action, subject and actor in the row
  and moves category, function, actor identity and the raw record into its
  details.
- `REQ-NFR-aa1a4734` - 134 catalogue keys added to en, de and ro.
- `REQ-TEST-5da12a09` - `e2e/field-display.spec.ts` drives 1920x1080, 1366x768,
  1024x600 and 800x480 in both themes and asserts readable text, 44-pixel field
  targets, no document overflow, no `title`-only explanation, the drawer, the
  sheet, help by pointer and keyboard, the editors' final actions, validation
  and a pending confirmation. It is CSS-pixel proof; the physical seven-inch
  panel is still unverified.

## Focused Network navigation (2026-09-12, TASK-8f18c927)

Network is one collapsible System group whose root opens Overview and whose
children open Device access, VPN profiles and Diagnostics. Each destination owns
one workspace tab and one concern: Overview contains the decision summary,
management access and interface configuration; Device access contains forwarding
definitions; VPN profiles contains WireGuard lifecycle and import; and Diagnostics
contains WireGuard readiness plus traffic history. The existing `network` page
id remains Overview; the children use internal
`network:*` ids. Collapsing navigation never closes a tab, Activity remains a
peer page, and common operation refusal, confirmation and recovery notices stay
available on every Network view.

`TrafficChart` labels received (RX) and sent (TX) traffic, scales bytes per
measured minute against a visible time axis, and positions bars by timestamp so
an unmeasured minute remains a gap. One field-sized listbox maps pointer
coordinates to the nearest measured minute, avoiding overlapping targets in
dense one-hour and one-day histories; its options expose exact values, a visible
marker preserves measured zeroes, and arrow/Home/End keys move the active
selection. Hover, touch or keyboard selection shows the minute's time, RX, TX
and total while only the chart itself enters the tab order. Expanded WireGuard
readiness renders as a compact grid of left-label/right-value facts, and the
Statistics title shares one responsive header with the interface and period
controls. This keeps the initial Diagnostics chart in the workspace without
vertical scrolling at 1024x600 and larger; 800x480 may scroll vertically.

## Adapter role selection (2026-09-11, TASK-8f18c927)

`management-access.ts` strictly parses stable adapter identity, the fixed
`management`/`device` role vocabulary and validated 409 conflict details.
`NetworkPage` owns the one management-access observation shared by its focused
views. The management checkbox and device-side option stay visible but disabled
for an opposite claim, with a localized persistent explanation. Device-side
assignment also fails closed while management inventory is reading or failed;
defaults exclude known management claims. The same adapter remains available as
local ingress. `WorkflowHelp` exposes the three confirmed repurposing steps from
both selectors. The development mock and browser fixture return the same
role-aware contract. Management transition notices render only fixed typed
outcomes through the locale catalog, never backend diagnostic prose. A valid
conflict notice raised during preview, acknowledgement, or management update
survives unchanged polling and is superseded only when the corresponding
observed role claim disappears; malformed structured conflicts use a fixed
localized fallback and never retain backend wording.

## Account-menu appliance power controls (2026-09-08, TASK-a3a3d7b2)

On a live appliance, `TopRow` adds Restart NILO and Power off NILO beside Sign
out only when the current browser session carries `system.power.manage`.
Simulation, Viewer and bearer-token states render neither action. Power remains
outside `BENCH_CORE_TOKEN_SCOPES`, although it is part of the complete permission
union used to parse session responses.

`PowerConfirmation` owns the action-specific warning and a single in-flight
submission. It distinguishes accepted responses, typed server refusals and an
unconfirmed transport loss; it never interprets the intended disappearance of
the appliance as success. `AppShell` owns the selected action and binds the
dialog to `BenchCoreClient.requestPower`, while sign-out keeps its existing
session behavior.

## Managed instrument workspace (TASK-f1f2d836)

`REQ-UX-30e2e009` adds Plugins and the sigrok integration tab through the same
`PluginsPage`. The page serializes refreshes, polls summaries and selected detail,
retains uncertain submission keys, renders digital channels and a bounded preview,
and exposes session-only approval, cancellation, capture download and saved UART
submission. Management and execution controls follow the current session scopes.
English, German and Romanian share the instrument key set.

- `src/api/bench.ts`, `bench.test.ts` — job/plugin wire parsing and API boundary.
- `src/api/client.ts` — lifecycle, discovery, submit/list/detail/actions/download.
- `src/components/pages/PluginsPage.tsx`, `PluginsPage.test.tsx` — workspace and
  retry/permission/reconnect regressions.
- `src/components/shell/pages.ts`, `AppShell.tsx` — installed workspace routing.
- `src/i18n/{en,de,ro}.ts`, `src/index.css` — translated controls and responsive layout.

The browser mock retains its pre-existing scope; exercise instruments against a
real simulation-profile runner. Source paths above are relative to `05_Software/frontend/`.

## Core navigation order without Settings (2026-09-05, TASK-17d71acd)

`CORE_PAGES` in `shell/pages.ts` is the top-to-bottom order the lower cluster
renders, because `SideNav` maps the array as it stands: Plugins, Network with
its three `network:*` children, Authentication with API tokens, Activity, About.
The `settings` descriptor, its `AppShell` placeholder branch and the obsolete
placeholder catalogue entries are gone, so no Settings destination exists to
open; `Icon`'s `settings` glyph remains available in the glyph catalogue. The Plugins
entry remains available through the managed instrument workspace added by
TASK-f1f2d836.

## Activity page furniture (2026-09-15, TASK-29673ddb)

The Activity page is two lines of furniture and then records. `.activity-head`
is one row: the page title, and the Export menu at its right edge
(REQ-UX-3e01a5b6). `ActivityHealthBanner` renders only the conditions that
qualify the listing - audit writes failing, and each retained window entries
were removed from - and renders nothing at all while neither holds
(REQ-UX-c5976f3b). The administrator retention disclosure sits between the
header and the filters, collapsed (REQ-UX-0291ba55).

`.activity-filters` is a wrapping flex line rather than a grid of rows: the six
fields share a `1 1 12rem` basis, and More filters is an `aria-expanded` button
whose three fields join the same flow through
`.activity-filters-more-fields:not([hidden]) { display: contents }`
(REQ-UX-f8c58c4c). The category chips are one element in one of two positions,
decided by `useViewportClass`: inside the form and first on the line while the
panel is inline, and above the closed drawer while the viewport collapses it,
so a bench panel keeps scope selection one tap away (REQ-UX-ba46f6d3).

The listing offers 10, 25, 50, 100 and all records a page and opens at 10
(REQ-UX-7781cc4e); Bench Core accepts any limit up to its maximum, so the
default is a client decision. Together these keep the whole first record inside
a 1024x600 viewport before anything is scrolled (REQ-UX-dd4b2c16).
