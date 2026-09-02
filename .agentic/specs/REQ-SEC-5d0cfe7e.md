<!--
Copyright (c) 2026 DDP Engineering
This software is licensed under the PolyForm Noncommercial License 1.0.0.
Commercial use requires a separate commercial license.
See the LICENSE.txt in the .agentic folder for full license details.
SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
-->

# REQ-SEC-5d0cfe7e — Map fixed roles to explicit permissions

This spec is the reconciled access model for the whole identity and
access-control foundation. The role matrix is one requirement, but the
matrix is unreadable without the surrounding model, so the model is
recorded here once instead of being repeated across nineteen files.

## Two independent decisions

NILO now answers two different questions about every request, in order,
and both must say yes.

1. **Identity authorization.** May this actor *request* this action at
   all? Decided from the actor's fixed role and, for an API token, from
   the token's declared scopes.
2. **Integration operation policy.** May this *operation* run on this
   *device* now, or does it need a human approval first? Decided from the
   integration app's declared operation policy, the administrator's
   per-operation overrides, and the persisted device mode.

The second decision is unchanged by this work and remains owned by the
integration app, exactly as the MVP PRD describes. Identity
authorization is a gate placed *in front of* it, never a replacement for
it and never a way around it.

Concretely: the Administrator role is not a bypass. An administrator
submitting an operation still meets Ask/Auto/Full Access resolution,
still meets required approvals, and still meets the core-enforced
voltage, current, duration, channel, target-identity, firmware-allowlist,
armed-state and interlock limits. A token carrying every scope is
likewise only an actor with full *request* rights; it inherits no
approval automation whatsoever. Removing an approval requirement remains
an administrator action performed against the integration policy
configuration, and it is audited there.

The planning statement that permission policy "is not attached to a
client or token" was written before human identities existed and is now
narrowed rather than reversed: *operation* policy is not attached to a
client or token; *request* authorization is.

## Roles and permissions

Three fixed roles. There is no custom-role editor, no permission editor,
and no policy language: the matrix below is compiled into Bench Core.

| Permission | Meaning | Viewer | Operator | Administrator |
|---|---|---|---|---|
| `status.read` | Read the backend status resource | yes | yes | yes |
| `inventory.read` | Read the integration and device inventories | yes | yes | yes |
| `tokens.read.self` | List one's own API tokens | no | yes | yes |
| `tokens.manage.self` | Create and revoke one's own API tokens | no | yes | yes |
| `users.read` | List identities, their roles and their state | no | no | yes |
| `users.manage` | Create identities, change roles, disable, enable, reset passwords | no | no | yes |
| `tokens.read.all` | List every identity's API tokens | no | no | yes |
| `tokens.manage.all` | Revoke any identity's API token | no | no | yes |

The set is deliberately limited to functionality that exists today.
Hardware operations, PLC flashing, MCP transport and artifact retrieval
get their permissions when they get their endpoints; inventing them now
would produce a matrix that no test can exercise.

One honest caveat: the two `tokens.*.self` permissions are enforced
wherever token management is reachable, but token management is reachable
only from the local administration command line and from the browser
Authentication page, both of which are administrator-only by design. So an
Operator holds those permissions and still has no interface through which
to use them. They are kept rather than dropped because they are what a
token scope narrows against, and because collapsing them would make
Operator identical to Viewer. The self-service surface remains deferred.

Every authenticated actor may read its own session regardless of role;
that is identity, not authorization, so it carries no permission.

## Effective permissions

```
sessionPermissions(user)  = rolePermissions(user.role)
tokenPermissions(token)   = rolePermissions(owner.role) ∩ scopes(token)
```

An empty scope list means "not narrowed" and resolves to the owner's full
role permissions. This is what keeps API tokens created before this work
usable: their stored scope list is empty, so they follow their owner's
role and gain nothing.

The intersection is recomputed from the *current* stored role on every
request. A role change therefore takes effect on the next request for
both sessions and tokens, with no re-issue and no cache to invalidate.
Scopes can only ever remove permissions, so a scope list is never a
privilege-escalation path.

## Credentials

Two credential kinds, deliberately kept apart.

**Browser session.** Established by password login, carried in a cookie,
short-lived, revocable server-side, and unusable for automation because
its cookie is `HttpOnly` and its state-changing use additionally requires
a CSRF token.

**API token.** Created by an administrator, from the administration
command line or the browser Authentication page: named,
optionally scoped, optionally expiring, long-lived, and presented in the
`Authorization: Bearer` header. It has no cookie and is not subject to
CSRF, because it is never attached by a browser automatically.

### Session cookies

| | Development-localhost mode | HTTPS deployment mode |
|---|---|---|
| `Secure` | absent | present |
| `HttpOnly` | present | present |
| `SameSite` | `Strict` | `Strict` |
| `Path` | `/` | `/` |

The mode is chosen explicitly at server start, never inferred. The
development topology serves plain HTTP on loopback, where a `Secure`
cookie would never be sent at all; any non-loopback deployment must
select the HTTPS mode. Selecting the development mode with a
non-loopback bind address is refused.

### CSRF

Login returns a CSRF token in the response body and in a readable
(non-`HttpOnly`) `nilo_csrf` cookie. Bench Core stores only its SHA-256
verifier alongside the session. Every state-changing request
authenticated by a session cookie must repeat the token in the
`X-NILO-CSRF` header, and the header value is compared in constant time
against the stored verifier of *that* session. Bearer-authenticated
requests skip this check.

### Login abuse

Failed password logins are counted per lowercased username in memory,
with an injected clock. After 5 consecutive failures inside 15 minutes,
further attempts for that username are refused as throttled until the
window elapses, whether or not the password is correct. A successful
login clears the counter. An unknown username is verified against a
fixed dummy Argon2id verifier so that its response time does not reveal
whether the account exists.

The counter is in memory only: it is a brute-force brake for a running
process, not durable state, and it is documented as resetting on
restart.

## Persistence

Forward migration `004_identity_access_control.sql`; earlier migrations
are not rewritten.

```sql
ALTER TABLE users ADD COLUMN role TEXT NOT NULL DEFAULT 'viewer'
    CHECK (role IN ('administrator', 'operator', 'viewer'));
ALTER TABLE users ADD COLUMN disabled_at TEXT;

ALTER TABLE mcp_tokens ADD COLUMN name TEXT NOT NULL DEFAULT '';
ALTER TABLE mcp_tokens ADD COLUMN scopes TEXT NOT NULL DEFAULT '';
ALTER TABLE mcp_tokens ADD COLUMN expires_at TEXT;

CREATE TABLE IF NOT EXISTS user_sessions (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id),
    verifier BLOB NOT NULL UNIQUE,
    csrf_verifier BLOB NOT NULL,
    created_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    revoked_at TEXT
);

CREATE INDEX IF NOT EXISTS user_sessions_by_user ON user_sessions(user_id);
```

Existing rows keep their data. Every existing identity becomes a Viewer,
which is what keeps a pre-existing development token from silently
acquiring administrative rights. `scopes` is a comma-separated permission
list; empty means unnarrowed. Identities are disabled, never deleted, so
audit records keep resolving to a real subject; hard deletion is
deliberately deferred.

The browser session table is `user_sessions` because `sessions` already
names operation sessions, which are a different thing entirely.

### Atomicity

Three identity invariants are enforced by the database, not by the order
of statements around it.

**One administrative change, one transaction.** Every mutating identity
or token operation runs inside a single `BEGIN IMMEDIATE` transaction
that also carries its audit event. `IMMEDIATE` takes the write lock
before the transaction reads anything, so the actor check, the last
enabled administrator count, and the change they guard cannot be
interleaved with another connection or another process — the two cases a
process-local lock would miss, given that the command line and the server
are separate processes over one database file. If the audit event cannot
be written, the change is rolled back with it: an identity change nobody
can be held to account for is not an identity change worth keeping.

**A session belongs to the password that was checked.** Session creation
is one conditional insert:

```sql
INSERT INTO user_sessions (...)
SELECT ?, ?, ?, ?, ?, ?
WHERE EXISTS (SELECT 1 FROM users
              WHERE id = ? AND password_hash = ? AND disabled_at IS NULL);
```

The verifier the login checked is a condition of the statement that
writes the row, so a password reset or a disable that commits while a
login is in flight leaves that login with no session at all. Re-reading
the password before an unconditional insert would only narrow the window;
this closes it.

**Only the server reconciles.** Startup reconciliation declares persisted
running sessions incomplete and releases their leases. It belongs to a
starting server alone: `app.New` performs it, `app.Open` does not, and
every command that is not the server — the administration commands and
development token issuance — uses `app.Open`, because a live server may
own the operations recorded in that data directory.

## HTTP interface

| Route | Credential | Required permission |
|---|---|---|
| `POST /api/v1/auth/login` | none | none |
| `POST /api/v1/auth/logout` | session | none, CSRF required |
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

The eight administration routes carry the same authorization rule as
everything above them; their request and response bodies, their error
codes and their validation are specified once in
`.agentic/specs/REQ-FUNC-5fd2806f.md` rather than repeated here.

Rejection is uniform in shape and distinct in meaning:

- no credential, or an unusable one → `401` with `WWW-Authenticate: Bearer`
  and body `{"error":"unauthorized"}`;
- a valid credential lacking the permission → `403` with body
  `{"error":"forbidden"}` and no `WWW-Authenticate` header.

"Unusable" covers a missing, malformed, unknown, expired or revoked
token, a token whose owner is disabled, and an expired, revoked or
unknown session. The client learns only that the credential did not
work, never which of those it was.

## Command line

Every command takes `--data-dir <absolute-path>`. Passwords are read
from standard input, never from an argument, and are never printed.
Mutating commands take `--actor <username>`, naming an existing enabled
administrator recorded as the audit actor; write access to the data
directory is already full control of the database, so the flag is
attribution, not a security boundary.

```
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

`bootstrap` is the one command with no `--actor`: it attributes the
creation to the administrator it creates. It refuses to run once any
enabled administrator exists, which is what stops it being a permanent
unauthenticated escalation path.

All output is JSON on standard output. `token create` is the only
command that ever prints a secret, and it prints it once.

## Examples

```console
$ printf 'correct horse battery\ncorrect horse battery\n' | \
    nilo admin bootstrap --data-dir /var/lib/nilo --username admin
{"user_id":"0199...","username":"admin","role":"administrator"}

$ nilo admin token create --data-dir /var/lib/nilo --actor admin \
    --owner admin --name ci --scopes status.read,inventory.read --expires-in 720h
{"token_id":"0199...","name":"ci","owner":"admin","scopes":["status.read","inventory.read"],
 "expires_at":"2026-10-02T09:00:00Z","token":"nilo_..."}

$ nilo admin token list --data-dir /var/lib/nilo
{"items":[{"id":"0199...","name":"ci","owner":"admin","scopes":["status.read","inventory.read"],
 "expires_at":"2026-10-02T09:00:00Z","revoked":false}]}
```

A Viewer reading `/api/v1/users`:

```http
GET /api/v1/users
Cookie: nilo_session=...

HTTP/1.1 403 Forbidden
Content-Type: application/json

{"error":"forbidden"}
```

The same request with no cookie at all:

```http
HTTP/1.1 401 Unauthorized
WWW-Authenticate: Bearer
Content-Type: application/json

{"error":"unauthorized"}
```

## Acceptance

- Each role resolves to exactly the permission set tabulated above, and
  the model defines no permission outside that table.
- A token's effective permissions equal its scopes intersected with its
  owner's current role permissions, and an empty scope list resolves to
  the owner's full set.
- The protected routes answer 401 without a usable credential and 403
  with a usable credential that lacks the route's permission.
- Nothing in this model changes how integration operation policy,
  required approvals, or configured safety limits are resolved.
