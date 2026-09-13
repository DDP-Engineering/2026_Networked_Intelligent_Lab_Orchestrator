# Changelog

All notable changes to NILO are documented in this file. The repository carries
one project-level Semantic Version, declared in the root `VERSION` file.

## [Unreleased]

## [0.3.1] - 2026-09-13

### Changed

- Replaced the simulated integrated-browser topology with a disposable
  appliance-profile stage exercising the production GUI, NetworkManager,
  privileged network helper, firewall, and WireGuard paths.
- Aligned browser acceptance, repository validation, and development guidance
  with production appliance behavior and Raspberry Pi hardware acceptance.

### Fixed

- Dense and zero-value network traffic samples remain pointer-selectable and
  keyboard accessible without overlapping interactive chart targets.
- Appliance browser credentials are kept out of command lines and redacted from
  runner errors and retained diagnostics.

## [0.3.0] - 2026-09-13

### Added

- First-boot provisioning: a new appliance records a commissioning lifecycle
  state, serves management access on every eligible physical adapter until an
  administrator commissions it, follows adapters as they appear and leave, and
  freezes the allowlist once commissioned. Upgrades are never returned to it.
- Appliance local-name discovery during first boot, published on exactly the
  interfaces carrying management access through the privileged helper, which
  manages only responder configuration it wrote itself.
- Link-local IPv4 addressing for wired adapters, so direct commissioning needs
  neither a DHCP server nor a guessed address.
- A first-boot browser workflow that states the temporary exposure, lists each
  eligible adapter with its stable identity and addresses, marks the current
  browser path, and completes commissioning onto a confirmed reachable path.
- Exclusive stable-identity roles for physical management and device-facing
  adapters, with fail-closed conflicts, restart recovery and guided GUI choices.
- Persistent, administrator-managed HTTPS interface allowlists with dynamic
  Wi-Fi/WireGuard address tracking, last-known-good recovery and stable URLs.
- Defense-in-depth management-port filtering through the privileged network
  helper, plus certificate identity validation and GUI configuration states.
- Confirmed whole-appliance Restart and Power off controls for Operator and
  Administrator browser sessions, dispatched through the privileged helper.
- Reproducible Linux amd64 OCI development/build environment with a complete
  locked offline input closure, shared Windows/WSL2 and native Ubuntu 24.04
  orchestration, multipart LFS escrow, and acceptance evidence tooling.
- Structured Sigrok worker feasibility slice with direct mixed-signal datafeed and decoder callbacks, bounded crash isolation, and machine-readable coverage evidence.

### Changed

- Standardized page-scoped View activity beside Network and Authentication titles.
- Renamed the management-access apply button to clearly describe its purpose.
- Unified Overview's adapter inventory and management controls in one compact,
  responsive table; secondary access guidance is expandable.
- Network Overview now leads with appliance status and continues directly with
  management access and interfaces; the redundant Network adapters page is gone.
- The signed-out interface now presents NILO's version and human sign-in in a
  responsive layout, with an accessible password visibility control.
- Updated the installed AEGIS workflow to 0.12.0 for focused verification and
  leaner records, preserving project configuration and engineering history.

- Build-environment reconstruction now acquires and digest-verifies the pinned
  base and BuildKit frontend images in an explicit networked phase, so a clean
  host with no cached images can rebuild offline.
- Container build and test mount the checkout read-only and work from an
  isolated copy, so a run can no longer replace host `node_modules`, create
  Linux virtual environments in the checkout, or overwrite Windows GUI
  dependencies. Declared artifacts are published to `/outputs`.
- Archive restore streams one verified pass instead of writing two full-size
  temporaries, checks free space before importing - including the Windows
  volume backing a WSL virtual disk - and accepts an explicit temporary
  directory.
- Explicit host engine installation calculates and displays the exact package
  transaction first and refuses downgrades and architecture replacements unless
  they are explicitly authorized.
- Acceptance evidence records seven ordered maturity states, so implemented,
  round-trip verified, published and fully accepted can no longer be conflated.

### Fixed

- Saved management identities survive interface-name reuse and ordinary saves;
  WireGuard installation acknowledgement excludes internal recovery metadata.
- Removing the current management path requires confirmation even when an
  unplugged adapter retains the same interface name.
- Addressless management adapters now remain valid GUI inventory entries, and
  unresolved checkpoint cleanup stays interlocked across restarts until the
  helper verifies restoration.
- Documentation now states the exact supported host boundary, the Git LFS
  prerequisite, and that checksums prove integrity rather than publisher
  authenticity; image scanning and real Podman verification are recorded as
  not yet performed.

## [0.2.0] - 2026-09-05

### Added

- Managed sigrok plugins with install, enable, disable, restart and uninstall across GUI, CLI and MCP.
- Digital ATK capture and saved UART jobs with bounded queues, transaction IDs, delayed replies, deadlines and retained artifacts.
- `nilo network operation list`, the listing of recent and pending managed
  changes the browser uses to reattach after a reload.
- Remote mode for `nilo admin user` and `nilo admin token` over `--server` and
  `--token-file`, calling the identity administration routes with the same
  permissions as the browser; the local `--data-dir` commands are unchanged.
- `nilo admin user delete` and `nilo admin token delete` in remote mode, which
  print exactly what will be removed and need the subject typed back on a
  terminal or the dedicated `--confirm-delete` flag without one.
- `identity.user.delete_refused` and `identity.token.delete_refused` denied
  audit records naming the actor, the requested subject and the reason.
- Permanent deletion of identities and API tokens, over
  `DELETE /api/v1/users/{username}` (`users.manage`) and
  `DELETE /api/v1/tokens/{id}` (`tokens.manage.all`).
- Delete in the destructive part of the row Actions dialog, distinct from
  Disable and Revoke, confirmed by typing the exact username or token name.
- `identity.user.deleted` and `identity.token.deleted` Activity audit events,
  appended in the same transaction as the removal. Both stay listable and
  exportable through `component=identity` once their subject is gone.

### Changed

- Deletion audit records carry the subject's name as their target label and
  the actor's and, for a token, the owner's username, so they read after the
  row is gone without a join.
- The root `nilo` usage names every current command group, including `activity`
  and both `admin` modes.
- Activity now opens newest-first with server-side sorting on every data column,
  numbered 25/50/100/all pagination, and a filtered record total.
- Activity component filtering includes an `All network functions` choice that
  matches the complete `network.*` component family.
- The Activity export control now shares the filter action row, and the page's
  scroll rail aligns with the workspace edge.
- Deleting an identity removes its browser sessions with it, and is refused
  with `last_administrator` for the last enabled administrator and
  `user_has_tokens` while it owns any token, revoked and expired included.
- Deleting a token removes the record in any state and stops its secret;
  revoking still keeps the record.
- Authentication administration is now an expandable System group with focused
  Users and API tokens workspaces, responsive record actions, shared field and
  workflow help, and direct links to identity-filtered Activity.
- User and API-token mutations now write the common Activity audit envelope in
  the same transaction as the change, including the real session or bearer-token
  actor and no credential material.

### Removed

- Temporary debug sessions, their session history, generated and historical
  debug records, API and CLI controls, Activity filter, retention category and
  storage policy. Activity retention remains for Audit and Events only.

### Added

- Compact field interface for the seven-inch appliance display: one semantic
  type scale with no operator-facing text below 14 pixels, 44-pixel controls and
  touch targets, a navigation rail that keeps its labels and a labelled drawer
  on a constrained viewport, and dialogs that fit the viewport with a reachable
  title, close control and sticky actions, becoming full-screen sheets on a
  small display.
- Progressive help: persistent guidance under the fields that need it, an
  accessible help popover that opens by click, tap and keyboard and never
  requires hover, and a How to configure help for the device-network and adapter
  workflows with an addressing example and the external routes still needed
  elsewhere. Safety, validation, confirmation and rollback information stays
  outside help.
- Network overview naming the uplink, the device side, the active WireGuard
  profile and whatever needs attention; Interfaces, WireGuard profiles and
  Device networks as responsive records with explicit Details and one Actions
  dialog per row that stays above table scrolling, identifies the object and
  status, explains disabled choices and separates destructive actions; and a
  device-network editor grouped by task with format
  validation beside each field and a plain-language review before the preview.
- `e2e/field-display.spec.ts`: browser coverage at 1920x1080, 1366x768, 1024x600
  and 800x480 in both themes, asserting readable text, 44-pixel field targets,
  no document-level horizontal scrolling and the help, drawer and sheet
  behaviour.

- Interface shell following the saved design direction in
  `00_Planning/GUI_Design/`: one compact 46-pixel top row carrying the NILO
  wordmark, the open page tabs and the network, simulation, appearance,
  language and account controls, with a labelled left navigation whose
  bottom-anchored cluster is Plugins, Network, Authentication, Activity and
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
- Stateful Network simulation with interface and WireGuard controls, persisted
  traffic history, separate acknowledgement and two-minute rollback.
- Network GUI, authenticated CLI and read-only MCP tools over the shared service.
- Linux interface control through a restricted NetworkManager helper, with durable
  recovery and an independent two-minute rollback timer.
- Explicit appliance startup serving the GUI and authenticated API over HTTPS.
- Live WireGuard readiness checks with kernel and tooling diagnostics, without
  offering unimplemented profile controls or implying an observed tunnel state.
- Acknowledged WireGuard tool installation on Debian 13 and Ubuntu 24.04, with
  progress, signed-repository checks and actionable package-manager errors.
- Stable appliance identity, created once at first initialization, and a
  separate identifier for each boot, so events can be attributed and correlated
  across restarts.
- Audit events now carry an appliance, boot, category, component, actor kind,
  outcome, severity and correlated operation. Events stored before this change
  keep their original meaning and read back with those fields empty.
- Audit ordering comes from a persisted allocator, so retention deleting every
  retained record can no longer make the next event reuse a sequence number.
- Network changes are recorded durably before they are applied and are listed by
  `GET /api/v1/network/operations`, so an interface reloaded mid-change finds the
  pending operation and a restarted appliance still honours its window.
- A network change that cannot be recorded is refused before any side effect,
  while reads and an in-flight rollback continue to work.
- WireGuard profiles on the appliance: staged key material crosses the helper
  socket once, becomes a NetworkManager connection NILO owns (`nilo-wg-*` on
  `nilo0`), and is activated, switched and put back under a system-wide
  checkpoint; several profiles are saved, one is active, one may connect at
  startup, external tunnels are listed and never touched, and a profile can be
  renamed. `nilo network vpn profiles rename`.
- Adapter configuration bound to the adapter's permanent hardware identity:
  static or automatic IPv4 addressing, DNS, management protection and
  never-default, applied through a NILO-owned profile the operating system's
  own profile survives; a change that would leave no usable management path is
  refused. Wi-Fi scan and join for open, WPA2- and WPA3-personal networks, with
  the password sent once and never returned. `nilo network interface settings`,
  `nilo network wifi scan|join`.
- Device networks: save which adapter faces a device or a whole device subnet,
  what is exposed at its original or a translated address, from which local or
  VPN sources and for which services; enable, disable, edit and delete through
  the acknowledged lifecycle; observed state kept apart from intent (waiting,
  conflict, active); bounded connection checks reporting separate facts; access
  instructions per machine. `GET|POST /api/v1/network/device-networks...`,
  `nilo network device-networks ...`.
- Forwarding on the appliance in one nftables table NILO owns, rendered from a
  durable inventory written before any rule, with source translation by
  default, translated exposure matched on the address the client used, the
  internal address refused, and the forwarding switch given back when the last
  device network goes. A device-network change is recovered from its inventory
  snapshot; the helper repairs a vanished table and refuses a foreign one.
  Proven in the isolated container for local access and, with a disposable hub
  and two sites sharing one device address, for VPN access.
- Activity: `GET /api/v1/activity` with filters, server paging, the retained
  window and logging health; `GET /api/v1/activity/export` as JSONL or CSV with
  schema version, appliance identity, time range, retained window and redaction
  policy, bounded and formula-neutralized; retention by age and quota with
  watermarks that disclose gaps; login, access-denial, export, policy and runtime
  re-plan events. `GET|PUT /api/v1/activity/retention` and
  `nilo activity list|export|retention`. Permissions `activity.read`,
  `activity.export` and `activity.retention.manage`, Administrator only.
- Interface: Device networks table and editor, adapter configuration and Wi-Fi
  dialog, WireGuard rename, the shared Activity page with retention controls,
  activity links from the Network page already narrowed, reattachment to a
  change still in flight after a reload; English, German and Romanian.
- Independent recovery: `nilo-nethelper recover` and the root-owned
  `nilo-nethelper-recovery.timer`, installed by `install-nethelper.sh`, put an
  interrupted change back and reinstall the owned table when neither Bench Core
  nor the helper is alive. `02_Tools/scripts/build-appliance.sh` builds ARM64
  and x86-64 binaries.
- Root-owned key storage in the privileged helper. An imported profile's key
  material is staged under an opaque single-use handle, promoted only on
  acknowledgement, expired when an import is abandoned, and never crosses the
  helper socket.
- Saved VPN profiles persist as public metadata only, with unique names, a
  reference to their key material rather than the material itself, and zero or
  one startup selection enforced by the schema.
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

- Split Network into an expandable System group with focused Overview, Network
  adapters, Device access, VPN profiles and Diagnostics workspace tabs.
- Clarified the Diagnostics traffic chart with RX/TX legend, byte and time
  scales, preserved measurement gaps and an exact selectable-minute readout.
- Compacted Diagnostics readiness and statistics controls so its initial chart
  fits without vertical scrolling at 1024x600 and larger.

- Simplified Network tables with IPs, gateways and WireGuard endpoints; moved diagnostics and import behind compact controls and kept one runtime badge.

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

### Fixed

- NetworkManager pseudo-devices and hot-unplugged adapters without sysfs counters no longer stop the appliance service.
- A newly configured device-side adapter may use its own assigned subnet, and saving the disabled Device Network no longer requires a nonexistent forwarding checkpoint.

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
