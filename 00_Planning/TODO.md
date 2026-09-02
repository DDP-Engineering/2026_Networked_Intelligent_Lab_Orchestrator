# TODO

## GUI

- [ ] Provide light, dark, and system theme options. The system option should
  follow the operating-system or browser color-scheme preference.
- [ ] Design each page or subsystem to fit within one monitor viewport without
  page-level scrolling.
- [ ] Define the main GUI navigation with one tab or page per subsystem:
  - **Settings** - application, appliance, development, and user preferences.
  - **Authentication and access** - users, roles, permissions, sessions, and
    API-token administration.
  - **Integrations and plugins** - installed integrations and tools, including
    examples such as PulseView, USB cameras, and flashing tools.
  - **About** - NILO version, build information, compatibility information,
    and relevant license notices.

## Authentication and authorization

The backend foundation landed in development slice 7. What remains is the
interface for it, plus the deliberately deferred pieces below.

- [x] Add human authentication for one or more users. Backend only: HTTP
  login, logout and current-session routes with expiring, server-side
  revocable browser sessions.
- [x] Add role-based access control (RBAC) with permissions scoped per NILO
  function. Fixed Administrator, Operator and Viewer roles map to a
  compiled-in permission set. Deliberately **not** a custom-role editor.
- [x] Add user creation, role assignment, disabling and password reset —
  through the `nilo admin` command line. Still open: the same workflows in the
  GUI, arbitrary permission editing (not planned), and hard deletion
  (deferred so audit records keep resolving to a real subject).
- [x] Add API-token lifecycle management: create, name, scope, expire and
  revoke. Still open: rotation, deletion, and self-service token management
  for an Operator without command-line access.
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
