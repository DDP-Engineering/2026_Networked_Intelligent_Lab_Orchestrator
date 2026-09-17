# NILO — Command Line and Authentication

Official NILO documentation. This is the administrator's reference for the
`nilo` command line and for how NILO decides who a request belongs to and what
that request is allowed to ask for.

It describes the capabilities present in this release. Where a capability is
deliberately absent, section 8 says so plainly rather than leaving you to
discover it in production.

## Find an appliance before its address is known

`nilo discover` queries mDNS/DNS-SD and SSDP on each eligible local interface
for a bounded interval and merges observations by appliance UUID. Add `--json`
for schema-versioned output, or `--label <permanent-or-friendly-name>` to emit
one selected printable SVG label to standard output. `nilo open <name>` opens
the unique advertised HTTPS URL in the platform browser.

Every remote command's `--server` accepts either its existing explicit URL or
an exact permanent name / unique friendly name found by the same discovery
boundary. Explicit URLs bypass discovery. Missing and ambiguous names send no
request. Multicast is link-local; router or VLAN forwarding is not implied.

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
| `users.manage` | Create identities, change roles, disable, enable, reset passwords, delete permanently | no | no | yes |
| `tokens.read.all` | List every identity's API tokens | no | no | yes |
| `tokens.manage.all` | Revoke or permanently delete any identity's API token | no | no | yes |
| `password.policy.manage` | Read and change the appliance password policy | no | no | yes |

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

`nilo admin` administers identities and API tokens in one of two modes. A
command uses exactly one of them:

| | Local mode | Remote mode |
|---|---|---|
| Selected by | `--data-dir D` | `--server URL --token-file PATH` |
| Runs | on the appliance, against its data directory | anywhere that reaches the appliance's HTTPS listener |
| Authority | write access to the data directory | the token owner's role, narrowed by the token's scopes, checked by NILO on every request |
| Actor recorded | `--actor`, an enabled administrator named for attribution | the token that made the request; there is no `--actor` |
| Permanent deletion | not offered | `user delete` and `token delete`, behind an explicit confirmation |
| Output | JSON | readable by default; the appliance's own JSON document with `--json` |

Combining `--data-dir` with `--server` or `--token-file`, or giving one remote
option without the other, is a usage error (exit `2`) that opens nothing and
contacts nothing. So is `--actor` in remote mode, or `--json`, `--ca-file` and
`--confirm-delete` in local mode. `nilo admin bootstrap` is local only.

### Local mode: what it is, and what it is not

The local commands are the **local** administration boundary. They do not
authenticate the operator running them, because write access to the NILO data
directory is already equivalent to full control of its database; a password
prompt there would add ceremony without adding a boundary.

`--actor` names an existing **enabled administrator** whose identity is
recorded as the audit actor. It is attribution, not authentication. A
non-administrator or disabled `--actor` is refused.

Protect the data directory with filesystem permissions accordingly. Anyone who
can write to it can administer NILO.

### Remote mode: what it is

The remote commands are an ordinary API client. They call the same identity
administration routes the browser Authentication page calls (section 5), with
the bearer token read from `--token-file`, and NILO decides every request the
way it decides a browser's: the token must be usable, its owner's role
narrowed by its scopes must hold the route's permission, and every identity
rule in section 5 applies inside the appliance. The command adds no rule of
its own and can obtain nothing the same token could not obtain with `curl`.

The token is read from the file named by `--token-file` and from nowhere else.
There is no flag that accepts a token value, because a command line is
readable by every other process on the host and survives in shell history. A
missing, empty or unreadable token file is reported before any request is
made. `--ca-file` pins the certificate authority of a private listener.

### Rules that hold for every command

- Passwords are read from **standard input**, never from a command argument,
  in both modes. `user create` and `user reset-password` read the password
  twice and refuse a mismatch; the appliance holds the password to its
  configured password policy, described below.
- No command prints a password back, in any mode, on any stream.

**The appliance password policy.** The appliance holds one policy: a minimum
length in bytes, and four independent requirements - a lowercase letter, an
uppercase letter, a digit, and a special character, meaning any character that
is neither a letter nor a digit. Each requirement demands at least one such
character. An appliance that has recorded no policy applies a minimum of 12
bytes requiring no class, which is what NILO has always enforced.

The policy governs a password being **set** - bootstrap, create, reset, on every
interface - and is never consulted when a stored password is **checked**.
Tightening it therefore locks nobody out: an identity created under an earlier
policy keeps signing in and meets the new policy the next time its password is
set. The fixed maximum of 1024 bytes is not part of the policy; it bounds the
work an unauthenticated login attempt can ask the appliance to perform and
applies on both paths whatever the policy says.

A configured minimum may be set no lower than the absolute floor of **8 bytes**,
which is compiled in, and no higher than the maximum. A value outside that range
is refused with `invalid_password_policy` and a message naming the bound, and
the stored policy is left as it was. The four requirements need no bounds: each
is on or off, and a password missing any that are on is refused with a message
naming every requirement it missed. Only
`password.policy.manage`, which is administrator-only, may read or change the
policy, and every applied change appends one Activity audit event carrying the
policy before and after and no credential.
- Data goes to standard output; prompts, confirmations, warnings and errors go
  to standard error. Local output is JSON. Remote output is readable by
  default and, with `--json`, the appliance's own document and nothing else on
  standard output, so a script reads exactly what the API said.
- Exit `0` on success, `1` on a rejected or failed operation (including a
  refusal by the appliance, an unreachable appliance and a cancelled
  confirmation), `2` on a usage error.

### Local command reference

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

### Remote command reference

Every remote command takes `--server URL --token-file PATH [--ca-file PATH]
[--json]`, written here as `...`:

```text
nilo admin user list           ...
nilo admin user create         ... --username U --role R
nilo admin user set-role       ... --username U --role R
nilo admin user enable         ... --username U
nilo admin user disable        ... --username U
nilo admin user reset-password ... --username U
nilo admin user delete         ... --username U [--confirm-delete]
nilo admin token list          ...
nilo admin token create        ... --owner U --name N [--scopes a,b] [--expires-in 720h]
nilo admin token revoke        ... --id T
nilo admin token delete        ... --id T [--confirm-delete]
nilo admin password-policy     ... [--minimum-length N --require lower,upper,digit,special|none]
```

| Command | Request | On success |
|---|---|---|
| `user list` | `GET /api/v1/users` | the listing |
| `user create` | `POST /api/v1/users` | the new identity |
| `user set-role` | `PUT /api/v1/users/{u}/role` | the re-read listing |
| `user enable`, `user disable` | `PUT /api/v1/users/{u}/enabled` | the re-read listing |
| `user reset-password` | `PUT /api/v1/users/{u}/password` | the re-read listing |
| `user delete` | `DELETE /api/v1/users/{u}` | the re-read listing |
| `token list` | `GET /api/v1/tokens` | the listing |
| `token create` | `POST /api/v1/tokens` | the token and, once, its secret |
| `token revoke` | `POST /api/v1/tokens/{id}/revoke` | the re-read listing |
| `token delete` | `DELETE /api/v1/tokens/{id}` | the re-read listing |
| `password-policy` with no values | `GET /api/v1/password-policy` | the current policy |
| `password-policy` with a length and `--require` | `PUT /api/v1/password-policy` | the stored policy |

A refusal exits `1` and prints the appliance's stable error code on standard
error, for example `the appliance refused the request: last_administrator
(HTTP 409)`. The codes are those of section 5; `forbidden` is a usable token
without the route's permission and `unauthorized` is a token that cannot be
used at all (revoked, expired, unknown, or whose owner is disabled).

### Administering a Raspberry Pi appliance remotely

The appliance is served over HTTPS at `https://nilo.local`. On the appliance
itself, once, issue an administrator token in local mode and copy it to the
administration workstation with the permissions of a private key:

```bash
sudo nilo admin token create --data-dir /var/lib/nilo --actor admin \
  --owner admin --name workstation-admin --expires-in 720h
```

```bash
install -m 0600 /dev/null ~/.config/nilo/admin.token
# paste the token value printed once by the command above into that file
```

From the workstation, list the identities:

```bash
nilo admin user list --server https://nilo.local --token-file ~/.config/nilo/admin.token
```

```text
USERNAME  ROLE           STATE    ID
admin     administrator  enabled  0199...
operator  operator       enabled  0199...
```

Add an operator, with the password piped in from a secret manager rather than
typed:

```bash
printf '%s\n%s\n' "$OPERATOR_PASSWORD" "$OPERATOR_PASSWORD" | \
  nilo admin user create --username line-3 --role operator \
    --server https://nilo.local --token-file ~/.config/nilo/admin.token
```

Issue a narrowed token for a script and store the secret at once; it is shown
exactly once and the warning on standard error says so:

```bash
nilo admin token create --owner line-3 --name line-3-monitor \
  --scopes status.read,network.read --expires-in 720h \
  --server https://nilo.local --token-file ~/.config/nilo/admin.token --json
```

```json
{"api_version":"nilo.http/1","token":{"id":"0199...","name":"line-3-monitor","owner":"line-3",
 "scopes":["network.read","status.read"],"created_at":"2026-09-04T09:00:00Z",
 "expires_at":"2026-10-04T09:00:00Z","revoked":false,"revoked_at":null},"secret":"nilo_..."}
```

Disable an identity, then revoke a token, from a private listener with its own
certificate authority:

```bash
nilo admin user disable --username line-3 \
  --server https://nilo.local --token-file ~/.config/nilo/admin.token --ca-file ~/.config/nilo/nilo-ca.pem
nilo admin token revoke --id 0199... \
  --server https://nilo.local --token-file ~/.config/nilo/admin.token --ca-file ~/.config/nilo/nilo-ca.pem
```

### Permanent deletion from the command line

`user delete` and `token delete` are the only destructive commands, and they
are explicit twice over. First the command reads the listing and states on
standard error exactly what it is about to remove:

```text
This permanently deletes user "line-3" (id 0199..., role operator, disabled). Its browser sessions are removed with it.
This permanently deletes API token 0199... (name "line-3-monitor", owner line-3, revoked, expires 2026-10-04T09:00:00Z). The record cannot be recovered.
```

Then it needs consent, and only one of two things counts as consent:

- **On a terminal**, the username or token ID typed back exactly:

  ```text
  $ nilo admin user delete --username line-3 --server https://nilo.local --token-file ~/.config/nilo/admin.token
  This permanently deletes user "line-3" (id 0199..., role operator, disabled). Its browser sessions are removed with it.
  Type the username to confirm: line-3
  deleted user "line-3"
  ```

  Anything else prints `deletion cancelled; nothing deleted` and exits `1`.
- **Without a terminal** (a script, a pipe), the dedicated `--confirm-delete`
  flag. Without it the command prints `noninteractive deletion requires
  --confirm-delete; nothing deleted` and exits `2`. The flag is also accepted on
  a terminal, as the same consent given up front.

  ```bash
  nilo admin token delete --id 0199... --confirm-delete \
    --server https://nilo.local --token-file ~/.config/nilo/admin.token --json </dev/null
  ```

No other flag is consent. `--acknowledge`, which acknowledges a previewed
network change, is not an admin option at all and is refused as unknown. A
refused or cancelled deletion has made exactly one request, the listing read.

That listing read is a courtesy, not a gate. A token that holds `users.manage`
or `tokens.manage.all` without the matching read permission is answered `403`
on it; the command then says the subject cannot be described here and still
asks for consent and sends the deletion, because the appliance, not the
command, decides what the token may do. Any other failure of that read stops
the command with exit `1` before anything destructive is attempted. Usernames
are folded to lower case the way the appliance folds them, so
`--username Operator` describes, deletes and confirms `operator`.

After any applied change, the command re-reads the listing so you see the
resulting state. If that re-read fails, the change has still been applied: the
command says so on standard error and exits `0` rather than reporting a
success as a failure.

The rules of section 5 hold unchanged: the last enabled administrator cannot be
deleted (`last_administrator`), an identity that still owns API tokens cannot
be deleted until they are deleted (`user_has_tokens`), an already-deleted
subject answers `user_not_found` or `token_not_found`, and a subject the
listing does not know is still sent so that the appliance, not the command,
gives that answer. Every attempt, applied or refused, is on record (section 7).

### Invariants the commands enforce

- **Bootstrap closes permanently.** `nilo admin bootstrap` is the only route to
  administrative rights that requires no existing administrator, so it refuses
  to run as soon as one enabled administrator exists. It is also the only
  command with no `--actor`: it attributes the creation to the administrator it
  creates.
- **The last administrator survives.** Disabling the last enabled
  administrator, changing its role away from Administrator, or deleting it, is
  refused and leaves the identity unchanged, including when two such changes
  race through separate connections. You cannot lock yourself out this way.
- **The local commands disable; they never delete.** There is no local
  `user delete` or `token delete`: the confirmation a deletion needs is asked
  for in remote mode, over the HTTP API in section 5 and the browser page in
  section 6.
- **Disabling or resetting invalidates sessions.** Both immediately revoke
  every browser session that identity holds, server-side, including a session
  a login that checked the replaced password had not yet finished creating.
- **Safe to run beside a running server.** A local administration command opens
  the data directory without starting the appliance, so it leaves running
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
| `DELETE /api/v1/users/{username}` | session or bearer | `users.manage` |
| `GET /api/v1/tokens` | session or bearer | `tokens.read.all` |
| `POST /api/v1/tokens` | session or bearer | `tokens.manage.all` |
| `POST /api/v1/tokens/{id}/revoke` | session or bearer | `tokens.manage.all` |
| `DELETE /api/v1/tokens/{id}` | session or bearer | `tokens.manage.all` |
| `GET /api/v1/password-policy` | session or bearer | `password.policy.manage` |
| `PUT /api/v1/password-policy` | session or bearer | `password.policy.manage` |
| `GET /api/v1/network/management-access` | session or bearer | `network.manage` |
| `PUT /api/v1/network/management-access` | session or bearer | `network.manage` |
| `POST /api/v1/network/management-access/confirm` | session or bearer | `network.manage` |
| `POST /api/v1/network/management-access/recover` | session or bearer | `network.manage` |

Every route that changes something and is authenticated by a session cookie
must also carry the `X-NILO-CSRF` header. A creation returns what the caller
cannot otherwise obtain — the new identity, or the new token together with the
one and only disclosure of its secret. Identity changes otherwise answer `204
No Content`, and the caller re-reads the listing. Management-access mutations
return the safe current status so a client can present pending/recovered state;
they never return network credentials or key material.

A refusal that the caller can act on carries a stable code and a fixed message,
for example `{"error":"invalid_role","message":"role must be one of
administrator, operator or viewer"}`. The codes are `invalid_request`,
`invalid_username`, `invalid_password`, `invalid_role`, `invalid_name`,
`invalid_scope`, `invalid_expiry`, `invalid_enabled`, `invalid_password_policy`,
`user_not_found`,
`token_not_found`, `username_taken`, `last_administrator` and
`user_has_tokens`. Anything NILO did not expect is `500` with `internal_error`
and no message: a message built from an internal error would hand a caller the
shape of the store behind it.

### The two permanent deletions

`DELETE /api/v1/users/{username}` removes the identity row itself. It carries no
request body and answers `204 No Content`. Three things happen in the one
transaction that applies it: every browser session that identity holds is
removed, the identity is removed, and the audit event is appended. It is refused
with `409` and `last_administrator` while the identity is the last enabled
administrator, and with `409` and `user_has_tokens` while the identity owns any
API token — revoked and expired ones included. Owned tokens are never swept
along: delete them first, deliberately. An administrator may delete their own
account while another enabled administrator remains; the session doing it stops
authenticating at once, which is what returns a browser to sign-in.

`DELETE /api/v1/tokens/{id}` removes the token record itself and likewise
answers `204 No Content`. It is deliberately not revocation:

| | Revoke | Delete |
|---|---|---|
| Route | `POST /api/v1/tokens/{id}/revoke` | `DELETE /api/v1/tokens/{id}` |
| The secret | stops working | stops working |
| The record | kept, marked revoked | removed |
| Accepted state | active only | active, expired or revoked |
| Repeating it | `404 token_not_found` | `404 token_not_found` |

Neither deletion can be undone, and neither has a soft-deleted form to recover
from. What survives is the Activity history, which keeps naming the subject
after the subject is gone.

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
| Established by | password login | `nilo admin token create` or the API tokens page |
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

### Appliance management interfaces

Existing appliance installations remain compatible with one literal listener:

```text
nilo serve --profile appliance --data-dir D --bind IP:PORT --gui-dir GUI --tls-cert CERT --tls-key KEY
```

To serve the same Bench Core on local Wi-Fi and WireGuard without exposing the
PLC adapter, initialize a persistent interface allowlist instead:

```text
nilo serve --profile appliance --data-dir /var/lib/nilo \
  --management-interface wlan0 --management-interface nilo0 \
  --management-port 8443 \
  --management-tls-name ddp-remote.local \
  --management-tls-name 10.205.148.11 \
  --gui-dir /opt/nilo/gui --tls-cert CERT --tls-key KEY
```

The two modes cannot be combined. Interface names, rather than their current
addresses, are saved in the existing NILO state database. Later service starts
use `--management-port 8443` with the same data directory to load that state.
An allowed interface may be temporarily absent and begins serving automatically
when it returns. Only one `nilo serve` process may use the state directory.

Repeating the saved bootstrap options is harmless only while they still match.
To change an existing list from the appliance console, add
`--management-reconfigure`; NILO requires a currently usable path and stages the
new list as pending while retaining the previous list for recovery. Without
that explicit flag, differing startup options are refused rather than silently
overwriting a GUI selection or its recovery path.

The wildcard socket used internally by allowlist mode is not an authorization
shortcut. NILO rejects a connection by its local destination before TLS/HTTP
handling and repeats the check for requests on reused connections. The
privileged network helper independently accepts the management port only on the
configured interface names and drops it elsewhere. The unprivileged server is
not granted network-administration capabilities.

The administrator-only **Management access interfaces** control is under
**Network > Overview**. It reports detected names, types, current addresses and link
state, marks the interface used by the browser, and shows stable local/VPN URLs.
Removing the current path requires an explicit second confirmation. A change
must retain a usable interface, remains pending until confirmed through a newly
enabled path, and otherwise restores the last-known-good selection. Pending
state found after a restart is recovered rather than trusted.

For the field configuration, the certificate must contain at least
`DNS:ddp-remote.local` and `IP:10.205.148.11`. NILO never silently generates or
replaces a production certificate; it reports an actionable warning naming any
missing configured identity. Local clients use
`https://ddp-remote.local:8443`; `.local` discovery is local-network-only.
Remote WireGuard clients use `https://10.205.148.11:8443`. A changing Wi-Fi
DHCP address is telemetry, not a persistent identifier.

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
choices. You can create a user, change a role, disable and re-enable, reset a
password, and delete the identity permanently. The last enabled administrator
can be neither disabled, nor demoted, nor deleted, and the dialog says so beside
each control it has to withhold. Disabling and resetting a password each ask for
an explicit confirmation naming the identity, because both take access away
immediately and neither is undone by repeating the control.

**Disable or delete.** Disabling keeps the identity and stops it signing in; it
can be undone by enabling it again. **Delete permanently** removes the record,
and its browser sessions with it. Because nothing can bring it back, that dialog
asks for the username to be typed back exactly, states what will be removed, and
keeps its warning visible while you type. Nothing is sent until the typed name
matches, and the confirmation is presentation only: every rule it states is
enforced by NILO inside the transaction that applies the removal. An identity
that owns API tokens cannot be deleted, and the refusal says to delete those
tokens on the API tokens page first — revoking them is not enough.

**API tokens.** Every token's name, owner, effective state and expiry; scopes
and creation time are under **Details**. Revoked takes precedence over expired,
which takes precedence over active. Safe metadata never includes a secret,
which NILO does not store. You can create a named token for an existing identity
with optional scopes and an optional expiry, revoke an active token after an
explicit confirmation, and delete any token permanently. The creation form
states that a token's permissions are its scopes narrowed by its owner's current
role, so selecting every scope for a Viewer's token still yields a Viewer's
token.

**Revoke or delete.** Both are offered in the destructive part of the Actions
dialog and they are not the same thing. **Revoke** stops the secret and keeps
the record, and is offered only while the token is active. **Delete
permanently** removes the record whatever state it is in — active, expired or
already revoked — and asks for the token name to be typed back exactly, with the
same persistent warning as an identity deletion.

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
| `identity.user.deleted` | user |
| `identity.token.deleted` | api_token |

The two deletion events carry the safe description of what was removed, because
the record itself no longer exists to be looked up, and they are written so that
they read on their own: the subject's name is the event's target label (the
username, or the token name), and the details carry the acting administrator's
username. `identity.user.deleted` carries the username, the role and the
enabled state the identity had. `identity.token.deleted` carries the token name,
the owner's user ID and username, the scopes, the expiry and whether the token
was already revoked or expired. Neither carries a secret or a verifier, and both
remain listable and exportable after their subject is gone, without joining the
event to any table.

A **refused** deletion is on record too. It is not a mutation event, because
nothing changed; it is a denied decision, written after the refusing
transaction has rolled back:

| Action | Subject | Outcome | Reason codes |
|---|---|---|---|
| `identity.user.delete_refused` | username | `denied`, severity `warning` | `last_administrator`, `user_has_tokens`, `user_not_found`, `not_administrator`, `invalid_username` |
| `identity.token.delete_refused` | api_token | `denied`, severity `warning` | `token_not_found`, `not_administrator` |

Each carries the requested subject as its target label (`invalid` or
`unknown` when the request named no usable subject, whose input is kept out of
the record), the actor as for any other identity event, and `reason` and
`actor_username` in its details. If the refusal record itself cannot be
written, the deletion is still refused and the error the caller receives says
that the attempt could not be recorded. A
credential that lacks the route's permission never reaches this decision: the
HTTP boundary refuses it with `403` and records `identity.access.denied` with
the permission and route, exactly as for every other protected route.

Activity can list and export these events with `component=identity`; the
refusals are the ones with `outcome=denied`. Login attempts use their separate
access-audit action and are not mutation events.

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
- Token rotation, and permanent deletion in local mode: the `--data-dir`
  commands disable and revoke only. Permanent deletion is in the HTTP API, the
  browser page and the remote command line.
- MCP transport and operation endpoints.
- Suppressed terminal echo while typing a password. NILO never echoes or prints
  a password itself, but your terminal does while you type it. Prefer piping
  from a secret manager over interactive typing where that matters.
- Durable login-attempt state; the failed-login brake resets when NILO
  restarts.
