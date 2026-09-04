# NILO — Command Line and Authentication

Official NILO documentation. This is the administrator's reference for the
`nilo` command line and for how NILO decides who a request belongs to and what
that request is allowed to ask for.

It describes the capabilities present in this release. Where a capability is
deliberately absent, section 8 says so plainly rather than leaving you to
discover it in production.

## Contents

1. [Two decisions, not one](#1-two-decisions-not-one)
2. [Roles and permissions](#2-roles-and-permissions)
3. [The `nilo admin` command line](#3-the-nilo-admin-command-line)
4. [First-run setup](#4-first-run-setup)
5. [HTTP authentication](#5-http-authentication)
6. [The browser Authentication page](#6-the-browser-authentication-page)
7. [Audit](#7-audit)
8. [Upgrading an existing installation](#8-upgrading-an-existing-installation)
9. [Not yet available](#9-not-yet-available)

## 1. Two decisions, not one

Every request meets two independent checks, and both must pass.

**Identity authorization** decides whether an actor may *request* an action.
It comes from the actor's fixed role, narrowed further by the scopes of the API
token in use, if any. That is what this document describes.

**Operation policy** decides whether an instrument operation may *run* without
a human approval. It belongs to the integration app and its declared
operations, and is configured separately.

Identity authorization sits in front of operation policy and never replaces it.

> **The Administrator role is not a safety bypass.** An administrator
> submitting an operation still meets Ask / Auto / Full Access resolution,
> still meets required approvals, and still meets the enforced voltage,
> current, duration, channel, target-identity, firmware-allowlist, armed-state
> and interlock limits. An API token carrying every scope inherits no approval
> automation whatsoever. Relaxing an approval requirement is a separate,
> explicit change to the integration policy configuration, and it is audited
> there.

## 2. Roles and permissions

NILO has three fixed roles. There is no custom-role editor, no permission
editor, and no policy language: the table below is built into NILO.

| Permission | Meaning | Viewer | Operator | Administrator |
|---|---|---|---|---|
| `status.read` | Read the backend status | yes | yes | yes |
| `inventory.read` | Read the integration and device inventories | yes | yes | yes |
| `tokens.read.self` | List one's own API tokens | no | yes | yes |
| `tokens.manage.self` | Create and revoke one's own API tokens | no | yes | yes |
| `users.read` | List identities, their roles and their state | no | no | yes |
| `users.manage` | Create identities, change roles, disable, enable, reset passwords | no | no | yes |
| `tokens.read.all` | List every identity's API tokens | no | no | yes |
| `tokens.manage.all` | Revoke any identity's API token | no | no | yes |

The set covers the functionality this release exposes. Hardware operations,
firmware flashing, MCP access and artifact retrieval receive their permissions
when those interfaces ship.

One caveat worth knowing before you assign roles: the two `tokens.*.self`
permissions are enforced wherever token management is reachable, but both
surfaces in this release - the command line in section 3 and the browser page in
section 6 - are administrator-only. An Operator therefore holds those
permissions without yet having an interface for them. They are kept because they
are what a token scope narrows against.

### How effective permissions are calculated

```
signed-in user  ->  permissions of the user's role
API token       ->  permissions of the owner's role  ∩  the token's scopes
```

An empty scope list means "not narrowed" and gives the owner's full role.
The calculation runs against stored state on **every** request, which has two
consequences worth relying on:

- A role change takes effect on the very next request. Nothing needs
  re-issuing, and no session or token needs to be revoked to apply it.
- A scope can only ever *remove* a permission, never add one. Granting a token
  a scope its owner does not hold gives that token nothing.

## 3. The `nilo admin` command line

### What it is, and what it is not

The administration commands are the **local** administration boundary. They do
not authenticate the operator running them, because write access to the NILO
data directory is already equivalent to full control of its database; a
password prompt there would add ceremony without adding a boundary.

`--actor` names an existing **enabled administrator** whose identity is
recorded as the audit actor. It is attribution, not authentication. A
non-administrator or disabled `--actor` is refused.

Protect the data directory with filesystem permissions accordingly. Anyone who
can write to it can administer NILO.

### Rules that hold for every command

- `--data-dir` is required and must be an absolute path.
- Passwords are read from **standard input**, never from a command argument, so
  they never enter a process listing or shell history.
- No command prints a password back.
- Output is JSON on standard output; diagnostics go to standard error.
- Exit `0` on success, `1` on a rejected or failed operation, `2` on a usage
  error.

### Command reference

```text
nilo admin bootstrap      --data-dir D --username U
nilo admin user list      --data-dir D
nilo admin user create    --data-dir D --actor A --username U --role R
nilo admin user set-role  --data-dir D --actor A --username U --role R
nilo admin user disable   --data-dir D --actor A --username U
nilo admin user enable    --data-dir D --actor A --username U
nilo admin user reset-password --data-dir D --actor A --username U
nilo admin token create   --data-dir D --actor A --owner U --name N [--scopes a,b] [--expires-in 720h]
nilo admin token list     --data-dir D
nilo admin token revoke   --data-dir D --actor A --id T
```

`R` is `administrator`, `operator` or `viewer`. Usernames are folded to lower
case and may contain letters, digits, `-`, `_` and `.`.

`bootstrap`, `user create` and `user reset-password` read a password from
standard input. Each reads it twice and refuses a mismatch. A password must be
at least 12 bytes.

### Invariants the commands enforce

- **Bootstrap closes permanently.** `nilo admin bootstrap` is the only route to
  administrative rights that requires no existing administrator, so it refuses
  to run as soon as one enabled administrator exists. It is also the only
  command with no `--actor`: it attributes the creation to the administrator it
  creates.
- **The last administrator survives.** Disabling the last enabled
  administrator, or changing its role away from Administrator, is refused and
  leaves the identity unchanged. You cannot lock yourself out this way.
- **Identities are disabled, never deleted.** Audit records must keep resolving
  to a real subject.
- **Disabling or resetting invalidates sessions.** Both immediately revoke
  every browser session that identity holds, server-side, including a session
  a login that checked the replaced password had not yet finished creating.
- **Safe to run beside a running server.** An administration command opens the
  data directory without starting the appliance, so it leaves running
  operations and the equipment they hold exactly as it found them.
- **A token secret is shown once.** `token create` is the only command that
  ever prints a secret. NILO stores only a one-way verifier of it, so no later
  command, listing or audit record can reveal it. If a secret is lost, revoke
  the token and issue a new one.

## 4. First-run setup

There is no default account and no default password. A new installation has no
administrator until you create one.

Create the first administrator:

```bash
printf 'correct-horse-battery\ncorrect-horse-battery\n' | \
  nilo admin bootstrap --data-dir /var/lib/nilo --username admin
```

```json
{"user_id":"0199...","username":"admin","role":"administrator"}
```

Add an operator:

```bash
printf 'bench-operator-secret\nbench-operator-secret\n' | \
  nilo admin user create --data-dir /var/lib/nilo --actor admin --username operator --role operator
```

Issue a narrowed token for an automated client:

```bash
nilo admin token create --data-dir /var/lib/nilo --actor admin --owner operator \
  --name continuous-integration --scopes status.read,inventory.read --expires-in 720h
```

```json
{"token_id":"0199...","name":"continuous-integration","owner":"operator",
 "scopes":["inventory.read","status.read"],"expires_at":"2026-10-02T09:00:00Z",
 "token":"nilo_..."}
```

Store `token` in your secret manager now; it is never shown again. A listing
carries the metadata and never the secret:

```bash
nilo admin token list --data-dir /var/lib/nilo
```

```json
{"items":[{"id":"0199...","name":"continuous-integration","owner":"operator",
 "scopes":["inventory.read","status.read"],"expires_at":"2026-10-02T09:00:00Z",
 "revoked":false,"created_at":"2026-09-02T09:00:00Z"}]}
```

Revoke it when the client is retired:

```bash
nilo admin token revoke --data-dir /var/lib/nilo --actor admin --id 0199...
```

Bootstrap is the only step that has to happen at the console. Once one
administrator exists, everything above can be done from the browser
Authentication page in section 6 instead, by that administrator signing in with
the password you just set.

## 5. HTTP authentication

### Routes

| Route | Credential | Required permission |
|---|---|---|
| `POST /api/v1/auth/login` | none | none |
| `POST /api/v1/auth/logout` | session | none; CSRF token required |
| `GET /api/v1/auth/session` | session or bearer | none |
| `GET /api/v1/status` | session or bearer | `status.read` |
| `GET /api/v1/integrations` | session or bearer | `inventory.read` |
| `GET /api/v1/devices` | session or bearer | `inventory.read` |
| `GET /api/v1/users` | session or bearer | `users.read` |
| `POST /api/v1/users` | session or bearer | `users.manage` |
| `PUT /api/v1/users/{username}/role` | session or bearer | `users.manage` |
| `PUT /api/v1/users/{username}/enabled` | session or bearer | `users.manage` |
| `PUT /api/v1/users/{username}/password` | session or bearer | `users.manage` |
| `GET /api/v1/tokens` | session or bearer | `tokens.read.all` |
| `POST /api/v1/tokens` | session or bearer | `tokens.manage.all` |
| `POST /api/v1/tokens/{id}/revoke` | session or bearer | `tokens.manage.all` |

Every route that changes something and is authenticated by a session cookie
must also carry the `X-NILO-CSRF` header. A creation returns what the caller
cannot otherwise obtain — the new identity, or the new token together with the
one and only disclosure of its secret. Every other change answers `204 No
Content`, and the caller re-reads the listing.

A refusal that the caller can act on carries a stable code and a fixed message,
for example `{"error":"invalid_role","message":"role must be one of
administrator, operator or viewer"}`. The codes are `invalid_request`,
`invalid_username`, `invalid_password`, `invalid_role`, `invalid_name`,
`invalid_scope`, `invalid_expiry`, `invalid_enabled`, `user_not_found`,
`token_not_found`, `username_taken` and `last_administrator`. Anything NILO did
not expect is `500` with `internal_error` and no message: a message built from
an internal error would hand a caller the shape of the store behind it.

### Two rejections, deliberately distinct

- **`401 Unauthorized`**, with `WWW-Authenticate: Bearer` and body
  `{"error":"unauthorized"}` — no credential was presented, or the one
  presented cannot be used.
- **`403 Forbidden`**, with body `{"error":"forbidden"}` and no challenge — a
  usable credential that lacks the route's permission. Repeating the same
  credential will not help, which is why no challenge is offered.

"Cannot be used" covers a missing, malformed, unknown, expired or revoked
token, a token whose owner is disabled, and an expired, revoked or unknown
session. The client learns only that the credential did not work, never which
of those it was.

Login answers a wrong password and an unknown username identically, and
verifies an unknown username against a fixed placeholder so that response time
does not reveal whether an account exists.

Repeated failures are braked: after 5 failed logins for one username within 15
minutes, further attempts are answered `429` until the window elapses, even
with the correct password. This counter lives in the running process and resets
when NILO restarts — it is a brake against online guessing, not durable lockout
state.

### Browser sessions versus API tokens

|  | Browser session | API token |
|---|---|---|
| Established by | password login | `nilo admin token create` |
| Carried in | `nilo_session` cookie | `Authorization: Bearer` header |
| Lifetime | 12 hours, absolute | none, or `--expires-in` |
| Revocable server-side | yes, per session or per identity | yes, per token |
| CSRF token required | yes, on state-changing requests | no |
| Readable by page scripts | no; the cookie is `HttpOnly` | not applicable |

A browser attaches a cookie automatically, so a session needs a second
credential the browser will not attach on its own. Login returns a CSRF token
in the response body and in a readable `nilo_csrf` cookie; NILO stores only a
one-way verifier of it. Every state-changing request authenticated by a session
must repeat that token in the `X-NILO-CSRF` header. Bearer requests skip this
check, because nothing attaches a bearer header on a user's behalf.

### Cookie security mode

The mode is chosen explicitly when the server starts and is never inferred:

```text
nilo serve --profile simulation --session-cookies development-localhost|https --data-dir D --bind ADDR
```

| | `development-localhost` (default) | `https` |
|---|---|---|
| `Secure` | absent | present |
| `HttpOnly` | present | present |
| `SameSite` | `Strict` | `Strict` |

A `Secure` cookie is never sent over plain HTTP, so a plain-HTTP loopback
installation needs a deliberate exception.

> **Any installation reachable over a network must be served over HTTPS and
> started with `--session-cookies https`.** NILO does not infer this from the
> bind address, because that would silently produce an insecure cookie the day
> the server is placed behind a proxy.

### Signing in and out

```powershell
$session = New-Object Microsoft.PowerShell.Commands.WebRequestSession
$login = Invoke-RestMethod -Method Post -Uri http://127.0.0.1:8090/api/v1/auth/login `
    -ContentType application/json -WebSession $session `
    -Body '{"username":"admin","password":"correct-horse-battery"}'
$login.role
$login.permissions
```

The response carries the role and the effective permissions, so a client can
present only what the signed-in user may actually do. Read a permitted route,
then sign out with the CSRF token:

```powershell
Invoke-RestMethod -Uri http://127.0.0.1:8090/api/v1/users -WebSession $session
Invoke-RestMethod -Method Post -Uri http://127.0.0.1:8090/api/v1/auth/logout `
    -WebSession $session -Headers @{ "X-NILO-CSRF" = $login.csrf_token }
```

Sign-out fails with `403` without that header, and the session cookie stops
authenticating the moment it succeeds.

To confirm authorization is enforced, authenticate as a Viewer and read an
administrator route:

```powershell
$headers = @{ Authorization = "Bearer <viewer-token>" }
Invoke-RestMethod -Uri http://127.0.0.1:8090/api/v1/status -Headers $headers   # 200
Invoke-RestMethod -Uri http://127.0.0.1:8090/api/v1/users  -Headers $headers   # 403
```

## 6. The browser Authentication page

An administrator signing in to the browser interface gets an expandable
**Authentication** group in the System navigation. **Users** and **API tokens**
open as distinct workspace tabs and each fetches only the resource it owns
(API tokens also reads identities for the owner selector). The destinations are
shown only when the session can read them, but the interface enforces nothing
itself: NILO authorizes every request server-side, so an identity that reaches a
page some other way is refused there rather than being relied on not to look.

**Users.** Every identity with its username, role and enabled state; creation
time is under **Details**. The row **Actions** dialog explains unavailable
choices. You can create a user, change a role, disable and re-enable, and reset
a password.
Identities are never deleted, and the last enabled administrator can be neither
disabled nor demoted. Disabling and resetting a password each ask for an
explicit confirmation naming the identity, because both take access away
immediately and neither is undone by repeating the control.

**API tokens.** Every token's name, owner, effective state and expiry; scopes
and creation time are under **Details**. Revoked takes precedence over expired,
which takes precedence over active. Safe metadata never includes a secret,
which NILO does not store. You can create a named token for an existing identity
with optional scopes and an optional expiry, and revoke an active token after
an explicit confirmation. The creation form states that a token's
permissions are its scopes narrowed by its owner's current role, so selecting
every scope for a Viewer's token still yields a Viewer's token.

A new token's secret is shown once, with a copy action and the warning that it
cannot be retrieved later. Closing the disclosure clears the value from
interface state and removes it from the document; dismissing it, navigating
away or reloading does not bring it back.

Field help and **How to configure** guidance describe the role boundary,
empty-scope meaning, access impact, external coordination and verification.
Warnings and confirmation text remain visible without opening help. **View
activity** opens Activity with `component=identity` already applied.

Token administration in this release is administrator-only. An Operator holds
the `tokens.read.self` and `tokens.manage.self` permissions and still has no
interface for them.

## 7. Audit

Each of these changes appends exactly one common Activity audit event naming the
authenticated actor and affected subject. A browser session records actor kind
`user` and its user ID; a bearer request records actor kind `token` and its token
ID. The client request body cannot supply attribution. Events use category
`audit`, component `identity`, outcome `applied` and severity `info`, with the
appliance, boot and time-source envelope completed by the common audit writer.
No event contains a password, token secret or verifier. The change and its event
are written in one database transaction: a change whose event cannot be written
does not happen at all. Reads and rejected/no-op mutations add no mutation event.

| Action | Subject |
|---|---|
| `identity.user.created` | user |
| `identity.user.role_changed` | user |
| `identity.user.disabled` | user |
| `identity.user.enabled` | user |
| `identity.user.password_reset` | user |
| `identity.token.created` | api_token |
| `identity.token.revoked` | api_token |

Activity can list and export these events with `component=identity`. Login
attempts use their separate access-audit action and are not mutation events.

## 8. Upgrading an existing installation

An installation created before role-based access control is migrated forward
automatically the first time the new version opens its data directory. The
migration is forward-only; earlier migrations are never rewritten.

What to expect:

- Every existing identity and token is preserved.
- Every existing identity becomes a **Viewer**, so nothing silently acquires
  administrative rights. Promote the accounts that need it with
  `nilo admin user set-role`.
- Every existing token keeps an empty scope list, which means "not narrowed"
  and follows its owner's role. Existing automated clients keep working and
  gain nothing.
- Because no pre-existing identity is an administrator, `nilo admin bootstrap`
  remains available after the upgrade until you create one.

Take a backup of the data directory before upgrading, as with any schema
change.

## 9. Not yet available

Do not plan around these; they are not in this release.

- Self-service token management for an Operator, in the browser or anywhere
  else.
- Custom roles, arbitrary permission editing, single sign-on, multi-factor
  authentication, and email-based account recovery.
- Token rotation, and permanent deletion of identities or tokens.
- MCP transport and operation endpoints.
- Suppressed terminal echo while typing a password. NILO never echoes or prints
  a password itself, but your terminal does while you type it. Prefer piping
  from a secret manager over interactive typing where that matters.
- Durable login-attempt state; the failed-login brake resets when NILO
  restarts.
