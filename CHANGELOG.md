# Changelog

All notable changes to NILO are documented in this file. The repository carries
one project-level Semantic Version, declared in the root `VERSION` file.

## [Unreleased]

### Added

- Interface shell following the saved design direction in
  `00_Planning/GUI_Design/`: one compact 46-pixel top row carrying the NILO
  wordmark, the open page tabs and the network, simulation, appearance,
  language and account controls, with a labelled left navigation whose
  bottom-anchored cluster is Network, Plugins, Authentication, Settings and
  About. There is no second tab strip and no page title above the workspace.
- Open pages as tabs, with predictable activation, closing, arrow, Home and End
  keyboard operation, and a fallback to the nearest remaining neighbour when the
  active tab closes.
- Light, Dark and System themes over one set of semantic design tokens. System
  follows the device colour-scheme preference in CSS and changes with it without
  a reload. The selection is persisted.
- English, German and Romanian interface languages from one message catalogue,
  selectable in the top row and persisted. Identifiers, permission names,
  protocol names, API values and reported instrument data are left untranslated.
- Network page and compact network summary, both stating honestly that appliance
  network reporting is not yet available: Bench Core exposes no interface,
  address, link-speed or traffic data, so nothing is shown as measured and the
  reachability of Bench Core over HTTP is labelled as exactly that, never as
  LAN, Wi-Fi or VPN state.
- About page reporting the build identity Bench Core actually returns, and
  instrument entries built from the integrations it actually reports.
- Browser end-to-end coverage of the shell, `e2e/shell.spec.ts`, which proves the
  single-row header, the lower Network placement, tab opening and closing, and
  that a theme and a language survive a real page reload together with the
  session.

- Browser sign-in: a human signs in to the interface with a username and
  password and gets the server-managed `HttpOnly` session cookie. The signed-in
  state survives a page reload, because the interface asks
  `GET /api/v1/auth/session` who the browser's cookies belong to rather than
  keeping anything itself; sign-out ends the session server-side. The
  development bearer-token connection is kept beside the password form and is
  never required for a person to sign in. No password, session secret,
  cross-site request forgery token or new API token secret reaches
  `localStorage`, `sessionStorage`, a request URL or the console.
- Browser Authentication page, administrator-only, administering identities and
  API tokens: create an identity with one of the three fixed roles, change a
  role, disable and re-enable, reset a password, and create, list and revoke
  named API tokens with scopes and an optional expiry. Disabling, resetting a
  password and revoking each require an explicit confirmation naming the
  subject, and a new token's secret is shown once with a copy action and is not
  shown again after it is dismissed or the page is reloaded. Tab and Shift+Tab
  stay inside an open dialog, so a keyboard user cannot reach and operate the
  page the dialog is covering.
- Identity and token administration HTTP routes: `POST /api/v1/users`,
  `PUT /api/v1/users/{username}/role`, `PUT /api/v1/users/{username}/enabled`,
  `PUT /api/v1/users/{username}/password`, `GET /api/v1/tokens`,
  `POST /api/v1/tokens` and `POST /api/v1/tokens/{id}/revoke`. Each declares its
  permission beside the existing routes, takes its acting administrator from the
  authenticated request rather than any request body field, requires the
  `X-NILO-CSRF` header when a session cookie authenticates it, and answers a
  refusal with a stable code and a fixed message that quotes no internal detail.
  An API token with a past expiry is now refused rather than created unusable.
- Main navigation with Bench and Authentication entries, the second shown only
  to an identity holding the administration permissions. Hiding it is not a
  permission: Bench Core authorizes every request on its own, and the interface
  tells an ended session apart from a refused one.
- Browser end-to-end coverage of the human path, `e2e/authentication.spec.ts`,
  which bootstraps a real administrator through the real command line and drives
  password sign-in, identity and token administration, session restoration
  across a reload and sign-out against a real Bench Core under WSL2, with no
  mocked authorization answer anywhere in it.
- Identity and access control in Bench Core: fixed Administrator, Operator and
  Viewer roles mapped to an explicit permission set, enforced server-side on
  every protected HTTP route. An unauthenticated request is answered `401`, a
  request from an actor without the permission `403`.
- Browser login: `POST /api/v1/auth/login`, `POST /api/v1/auth/logout` and
  `GET /api/v1/auth/session`, with absolute session expiry, server-side
  invalidation, an `HttpOnly` cookie, a required `X-NILO-CSRF` header on
  state-changing requests, a failed-login brake, and the explicit
  `nilo serve --session-cookies development-localhost|https` mode.
- `nilo admin` local administration commands: bootstrap the first
  administrator, list, create, re-role, disable, enable and reset the password
  of an identity, and create, list and revoke named API tokens. Passwords are
  read from standard input and never printed. These commands open the data
  directory without starting the appliance, so running one beside a live
  server leaves that server's running operations and their resource leases
  untouched.
- Named API tokens with scopes and expiry. The secret is shown once at
  creation and only its verifier is stored; effective permissions are the
  scopes intersected with the owner's current role, so a token can never
  exceed its owner.
- `GET /api/v1/users`, the authenticated identity listing, requiring the
  administrator `users.read` permission.
- Audit records for identity creation, role change, disable, enable, password
  reset, and token creation and revocation, each with an attributable actor
  and no plaintext secret. Each change and its audit record are written in one
  database transaction, so a change that cannot be recorded is not applied,
  and that transaction also enforces the last-enabled-administrator rule
  against concurrent changes made from another process. A password reset also
  defeats a login that verified the replaced password but had not yet
  established its session.
- Official documentation `07_Delivery/NILO_CLI_and_Authentication.md`, the
  administrator's reference for the command line, the role and permission
  model, first-run setup, the authentication routes, audit coverage, and
  upgrade behavior.

### Changed

- The interface workspace now uses the full width and height left by the top row
  and the navigation. The decorative background grid, the connection-status
  panel and the card nesting are gone, and the physical-equipment boundary
  statement moved from a 47-pixel footer into a 24-pixel strip under the
  workspace, where it is still present on every screen.
- Durable browser storage now holds exactly two preference keys, `nilo.theme`
  and `nilo.language`. No password, session secret, request-forgery token or API
  token secret is stored beside them, and the tests assert the written keys
  rather than merely that nothing was written.

- Neither an administrator role nor a broadly scoped API token bypasses an
  integration operation's required approvals or safety limits. The planning
  documents now separate identity authorization, which gates what an actor may
  *request*, from integration operation policy, which gates whether an
  operation may *run* without a human approval.
- SQLite migration `004_identity_access_control` adds the identity, token and
  browser-session columns and tables. Existing identities and tokens are
  preserved; every existing identity becomes a Viewer and every existing token
  keeps an empty, unnarrowed scope list, so development tokens keep working
  and gain no administrative rights.

- One-command Windows development start, `02_Tools/scripts/nilo-dev.ps1
  start|status`, which brings up Bench Core under WSL2 and the browser
  interface on Windows, each in its own window, and reports whether they
  answer. It supervises nothing and never signals a process.
- Integrated browser test, `npm run test:e2e` in `05_Software/nilo_gui`, which
  drives a real browser against the Windows interface and a real Bench Core
  under WSL2 on the simulation profile, covering login, status, simulation
  marking, both inventories, and backend-unavailable recovery. On Windows the
  repository gate now runs it, so the browser-to-WSL2 path is gated evidence.
- Repository-local browser runtime, installed below
  `02_Tools/runtime/windows-x86_64/playwright/` by
  `02_Tools/bootstrap/bootstrap-browser.ps1`, so no test run downloads a browser
  and no per-user cache is written.

## [0.1.0] - 2026-09-01

### Added

- Portable Go Bench Core: configuration paths, operation and session domain,
  transactional SQLite foundation, authentication, permission and approval
  policy, and durable audit and artifact stores.
- Integration boundary: package manifests, wire protocol, Python integration
  SDK, and supervised external integration runners.
- Repository-local toolchain and Windows/WSL2 development bootstrap, with the
  Increment 1 acceptance gate.
- Authenticated simulation status server, deterministic simulation inventory,
  and the repository-local GUI foundation.
