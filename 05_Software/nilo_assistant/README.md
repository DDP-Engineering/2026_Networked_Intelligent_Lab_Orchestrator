# NILO Assistant Go Bench Core

This Go module is the NILO Bench Core. External Python integrations connect
through the versioned integration boundary rather than becoming part of the
core process. The module provides the portable Increment 1 foundation plus a
loopback-confined development simulation server. It does not yet provide
hardware behavior, simulated inventory, a browser GUI, or an MCP service.

Linux ARM64 is the first target and Linux x86_64 is the later appliance target.
Windows x86_64 is verified only as a portable compile target.

The currently implemented application commands are:

```text
nilo version
nilo serve-foundation-check --data-dir <absolute-path>
nilo development issue-token --data-dir <absolute-path>
nilo serve --profile simulation [--scenario healthy|unavailable] [--session-cookies development-localhost|https] --data-dir <absolute-path> --bind <loopback-address>

nilo admin bootstrap --data-dir <absolute-path> --username <name>
nilo admin user list --data-dir <absolute-path>
nilo admin user create --data-dir <absolute-path> --actor <administrator> --username <name> --role administrator|operator|viewer
nilo admin user set-role --data-dir <absolute-path> --actor <administrator> --username <name> --role administrator|operator|viewer
nilo admin user disable --data-dir <absolute-path> --actor <administrator> --username <name>
nilo admin user enable --data-dir <absolute-path> --actor <administrator> --username <name>
nilo admin user reset-password --data-dir <absolute-path> --actor <administrator> --username <name>
nilo admin token create --data-dir <absolute-path> --actor <administrator> --owner <name> --name <label> [--scopes a,b] [--expires-in 720h]
nilo admin token list --data-dir <absolute-path>
nilo admin token revoke --data-dir <absolute-path> --actor <administrator> --id <token-id>

nilo admin user list|create|set-role|enable|disable|reset-password|delete ... --server <url> --token-file <path> [--ca-file <pem>] [--json] [--confirm-delete]
nilo admin token list|create|revoke|delete ... --server <url> --token-file <path> [--ca-file <pem>] [--json] [--confirm-delete]

nilo network ... --server <url> --token-file <path>
nilo activity list|export ... --server <url> --token-file <path>
nilo diagnostics ... --server <url> --token-file <path>
```

The simulation server exposes `POST /api/v1/auth/login`, and the authenticated
`POST /api/v1/auth/logout`, `GET /api/v1/auth/session`, `GET /api/v1/status`,
`GET /api/v1/integrations`, `GET /api/v1/devices`, and `GET /api/v1/users`
routes using the `nilo.http/1` contract. The optional scenario is deterministic
and defaults to `healthy`; `unavailable` preserves stable identities while
changing readiness. Development token issuance creates a non-login identity
with the least-privileged Viewer role, stores only the one-way token verifier,
and prints the plaintext token once. The simulation listener rejects wildcard,
hostname, LAN, and other non-loopback addresses.

Identity and access control live in `internal/identity`: fixed Administrator,
Operator and Viewer roles, browser sessions, and scoped API tokens, enforced
server-side on every protected route. Identities and tokens can also be removed
permanently over `DELETE /api/v1/users/{username}` and
`DELETE /api/v1/tokens/{id}`, which is distinct from disabling and from
revoking: the record itself goes, with the identity's browser sessions, in the
one transaction that also appends the audit event. The last enabled
administrator cannot be deleted, and an identity that still owns any API token
is refused rather than cascaded. The `nilo admin` commands administer identities
locally with `--data-dir`, where they deliberately offer no deletion, or
remotely with `--server` and `--token-file`, where they are an ordinary client
of the same routes and offer `user delete` and `token delete` behind a typed
confirmation or `--confirm-delete`. A refused deletion is recorded as a denied
audit event with its reason. The role and permission model, the command
reference, the authentication routes and the upgrade behavior are documented in
`../../07_Delivery/NILO_CLI_and_Authentication.md`.

For the complete Windows-browser and WSL2-backend workflow, including exact
commands, see `../../06_SoftwareDocu/Development_and_Simulation_Flow.md`.

Run the complete foundation gate from the repository root with
`05_Software\scripts\test-increment-1.ps1` on Windows or
`./05_Software/scripts/test-increment-1.sh` on Linux. See
`05_Software/INCREMENT_1_ACCEPTANCE.md` for observed evidence and the mandatory
Raspberry Pi ARM64 gate.
