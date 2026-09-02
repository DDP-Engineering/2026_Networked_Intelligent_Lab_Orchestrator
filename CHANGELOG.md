# Changelog

All notable changes to NILO are documented in this file. The repository carries
one project-level Semantic Version, declared in the root `VERSION` file.

## [Unreleased]

### Added

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
