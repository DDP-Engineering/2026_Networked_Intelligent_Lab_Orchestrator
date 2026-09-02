# TODO

## GUI

- [x] Provide light, dark, and system theme options. The system option should
  follow the operating-system or browser color-scheme preference. All three ship
  over one set of semantic tokens; System follows the device in CSS and reacts
  to a change without a reload.
- [x] Design each page or subsystem to fit within one monitor viewport without
  page-level scrolling. Holds for the Bench and Authentication pages: each
  listing scrolls inside its own compartment. Below roughly 64rem the
  administration panels stack and scroll in the workspace rather than clip.
- [ ] Define the main GUI navigation with one tab or page per subsystem. The
  navigation and its two clusters exist; three entries still name subsystems
  that are not built, and say so rather than showing an empty page:
  - **Bench** - backend status and the integration and device inventory. Done.
  - **Instruments** - one entry per integration the backend reports, showing its
    identity, state, declared operations and devices. Done to the limit of what
    Bench Core reports; no instrument control surface exists.
  - **Network** - appliance LAN, Wi-Fi and VPN state. The page exists and states
    honestly that Bench Core reports none of it yet. Still open: the backend
    telemetry itself, VPN provisioning and network configuration.
  - **Authentication and access** - users, roles and API-token administration.
    Done for administrators. Sessions are ended by disabling an identity or
    resetting its password; a session browser is not built.
  - **Settings** - application, appliance, development, and user preferences.
    Not built; appearance and language are chosen from the top row.
  - **Plugins** - plugin management. Not built.
  - **About** - NILO version and build information. Done from the reported
    build identity; compatibility information is not yet reported.
- [ ] Add appliance network telemetry to Bench Core: interfaces, addresses,
  link speed and RX/TX. The interface has the page and will show it when the
  backend reports it.
- [ ] Translate the interface beyond English, German and Romanian if further
  languages are required.

## Authentication and authorization

The backend foundation landed in development slice 7 and its browser interface
in slice 8. What remains are the deliberately deferred pieces below.

- [x] Add human authentication for one or more users. HTTP login, logout and
  current-session routes with expiring, server-side revocable browser sessions,
  and a browser sign-in that uses them: an `HttpOnly` session cookie, session
  restoration after a reload, and sign-out.
- [x] Add role-based access control (RBAC) with permissions scoped per NILO
  function. Fixed Administrator, Operator and Viewer roles map to a
  compiled-in permission set. Deliberately **not** a custom-role editor.
- [x] Add user creation, role assignment, disabling and password reset —
  through the `nilo admin` command line and the browser Authentication page.
  Still open: arbitrary permission editing (not planned) and hard deletion
  (deferred so audit records keep resolving to a real subject).
- [x] Add API-token lifecycle management: create, name, scope, expire and
  revoke — through the command line and the browser Authentication page, both
  administrator-only. Still open: rotation, deletion, and self-service token
  management for an Operator.
- [x] Ensure API tokens can receive narrower permissions than their owner. A
  token's effective permissions are its scopes intersected with its owner's
  current role, so a scope can only ever remove one.
- [x] Audit role, password-reset and token lifecycle changes without recording
  plaintext credentials or tokens. Still open: auditing login attempts
  themselves, and an authenticated way to read the audit history.
- [ ] Add SSO, multi-factor authentication and email-based recovery.
- [ ] Make the failed-login brake durable; it currently resets when the Bench
  Core process restarts.

## Integrations, plugins, and updates

- [ ] Define a plugin system in which integrations and tool adapters can be
  installed independently from the NILO core application.
- [ ] Define plugin discovery, download, installation, enable/disable,
  compatibility checking, update, rollback, and removal workflows.
- [ ] Define package signing, provenance verification, permissions, isolation,
  and trust policy before allowing downloadable plugins.
- [ ] Provide a safe update mechanism for the NILO core application, separate
  from plugin updates, with compatibility checks and rollback support.

## Command-line access

- [ ] Consider an official NILO CLI, similar in role to the GitLab CLI, for
  authentication, status, administration, API-token management, integrations,
  plugins, and supported automation workflows.
- [ ] Design CLI authentication so interactive human sessions and scoped
  automation tokens follow the same RBAC and audit rules as the GUI and MCP
  interfaces.
