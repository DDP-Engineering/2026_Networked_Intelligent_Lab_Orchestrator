<!--
Copyright (c) 2026 DDP Engineering
This software is licensed under the PolyForm Noncommercial License 1.0.0.
Commercial use requires a separate commercial license.
See the LICENSE.txt in the .agentic folder for full license details.
SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
-->

# REQ-FUNC-5fd2806f — The identity and token administration HTTP surface

This spec is the wire contract for the seven administration routes the
browser Authentication page needs. The routes are written once here rather
than repeated across the requirements that each own one of them, because
they share an envelope, an error shape and one authorization rule.

The access model these routes sit inside — roles, permissions, credential
kinds, cookie modes, CSRF, and the pre-existing route table — is
`.agentic/specs/REQ-SEC-5d0cfe7e.md`. Nothing here changes it.

## Behavior

### Routes and permissions

| Route | Required permission | Success |
|---|---|---|
| `GET /api/v1/users` | `users.read` | `200` identity listing |
| `POST /api/v1/users` | `users.manage` | `201` identity |
| `PUT /api/v1/users/{username}/role` | `users.manage` | `204` |
| `PUT /api/v1/users/{username}/enabled` | `users.manage` | `204` |
| `PUT /api/v1/users/{username}/password` | `users.manage` | `204` |
| `GET /api/v1/tokens` | `tokens.read.all` | `200` token listing |
| `POST /api/v1/tokens` | `tokens.manage.all` | `201` token and secret |
| `POST /api/v1/tokens/{id}/revoke` | `tokens.manage.all` | `204` |

`GET /api/v1/users` already existed; the other seven are new. Every one of
them sits behind the same credential resolution as every other protected
route: no usable credential is `401`, a usable credential without the
route's permission is `403`, and a session-authenticated request that
changes state must carry `X-NILO-CSRF`.

A mutation that produces information the caller cannot otherwise obtain —
a new identity, a new token secret — returns it. A mutation that only
changes existing state answers `204` and the caller re-reads the listing.
That keeps the serialized surface as small as the screens actually need.

### Acting identity

The acting administrator is the identity that authenticated the request,
read from the request context. No request body field names an actor, and a
body that carries `actor`, `username`, `role` or any other identifying
field beyond the ones tabulated below is ignored rather than honored. The
identity service re-reads that actor inside its own transaction, so an
actor demoted between authentication and commit is refused there too.

### Failure

Every failure is a JSON object with a stable `error` code and, for a
client error that names a submitted value, a fixed `message`:

```json
{"error": "invalid_role", "message": "role must be one of administrator, operator or viewer"}
```

| Status | `error` | When |
|---|---|---|
| `400` | `invalid_request` | the body is not a JSON object of the expected shape |
| `400` | `invalid_username` | the username is outside the accepted form |
| `400` | `invalid_password` | the password is outside the accepted length |
| `400` | `invalid_role` | the role is not one of the three |
| `400` | `invalid_scope` | a scope names no defined permission |
| `400` | `invalid_expiry` | the expiry is unparseable or not in the future |
| `400` | `invalid_enabled` | the enabled field is missing or not a boolean |
| `401` | `unauthorized` | no usable credential |
| `403` | `forbidden` | a usable credential without the permission |
| `404` | `user_not_found` | no identity carries that username |
| `404` | `token_not_found` | no active token carries that identifier |
| `409` | `username_taken` | a different identity already uses that username |
| `409` | `last_administrator` | the change would leave no enabled administrator |
| `500` | `internal_error` | anything else, with no `message` |

Messages are fixed strings chosen per code. They never quote a submitted
password, a stored credential, or the text of an internal error, and they
never echo the submitted value back — naming the field and its accepted
form is what the caller needs to correct it.

A refused request changes nothing. Validation that can be decided from the
request alone happens before the identity service is called; the
invariants the service owns — a duplicate username, the last enabled
administrator, an expiry that is not in the future — are decided inside
its transaction and mapped to the codes above.

## Data / interface

All timestamps are RFC 3339 in UTC. Every response body carries
`"api_version": "nilo.http/1"`.

### Identity representation

```json
{
  "id": "0199...",
  "username": "jane",
  "role": "operator",
  "enabled": true,
  "created_at": "2026-09-02T09:00:00Z"
}
```

`GET /api/v1/users` answers `{"api_version": ..., "items": [ ... ]}`
ordered by username. It carries no password verifier and no session
information.

### Requests

```
POST /api/v1/users
{"username": "jane", "role": "operator", "password": "<at least 12 bytes>"}

PUT /api/v1/users/{username}/role
{"role": "viewer"}

PUT /api/v1/users/{username}/enabled
{"enabled": false}

PUT /api/v1/users/{username}/password
{"password": "<at least 12 bytes>"}
```

`{username}` is the normalized username; it is lower case and limited to
letters, digits, `-`, `_` and `.`, so it needs no escaping.

### Token representation

```json
{
  "id": "0199...",
  "name": "ci",
  "owner": "jane",
  "scopes": ["inventory.read", "status.read"],
  "created_at": "2026-09-02T09:00:00Z",
  "expires_at": "2026-10-02T09:00:00Z",
  "revoked": false,
  "revoked_at": null
}
```

`scopes` is always present and is `[]` for an unnarrowed token, which
inherits its owner's whole role. `expires_at` and `revoked_at` are `null`
when the token never expires and has not been revoked. `revoked` is the
derived flag the interface renders, so a client never has to interpret a
timestamp to decide whether a token is usable.

`GET /api/v1/tokens` answers `{"api_version": ..., "items": [ ... ]}`
ordered by creation time. No listing ever carries a secret or a verifier.

```
POST /api/v1/tokens
{"owner": "jane", "name": "ci", "scopes": ["status.read"], "expires_at": "2026-10-02T09:00:00Z"}
```

`scopes` may be absent, `null` or `[]`, all meaning unnarrowed.
`expires_at` may be absent or `null`, meaning the token does not expire.

The `201` answer is the only place a secret appears:

```json
{"api_version": "nilo.http/1", "token": { ...token representation... }, "secret": "nilo_..."}
```

```
POST /api/v1/tokens/{id}/revoke
```

carries no body. Revoking a token that does not exist, or that is already
revoked, answers `404 token_not_found`: the store records a revocation
only for a token that is currently active, and the caller learns the same
thing either way — there is nothing left to revoke.

## Examples

Creating an identity as an administrator:

```http
POST /api/v1/users
Cookie: nilo_session=...
X-NILO-CSRF: ...
Content-Type: application/json

{"username":"jane","role":"operator","password":"correct horse battery"}

HTTP/1.1 201 Created
{"api_version":"nilo.http/1","id":"0199...","username":"jane",
 "role":"operator","enabled":true,"created_at":"2026-09-02T09:00:00Z"}
```

The same request with a role nobody defined:

```http
{"username":"jane","role":"superuser","password":"correct horse battery"}

HTTP/1.1 400 Bad Request
{"error":"invalid_role","message":"role must be one of administrator, operator or viewer"}
```

Demoting the only enabled administrator:

```http
PUT /api/v1/users/admin/role
{"role":"viewer"}

HTTP/1.1 409 Conflict
{"error":"last_administrator",
 "message":"the last enabled administrator must remain an enabled administrator"}
```

The same request with a session cookie but no `X-NILO-CSRF` header answers
`403 forbidden` before any of the above is evaluated.

## Acceptance

- Each route answers `401` without a usable credential, `403` with a
  usable credential lacking its tabulated permission, and its tabulated
  success status with that permission.
- A change is attributed in the audit history to the authenticated
  identity, whatever the request body claims.
- A refused request leaves the identity and token listings unchanged.
- A created token's secret appears in its creation response and in no
  other response, ever.
- No response body contains a password verifier, a token verifier or a
  session verifier.
