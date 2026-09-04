<!--
Copyright (c) 2026 DDP Engineering
This software is licensed under the PolyForm Noncommercial License 1.0.0.
Commercial use requires a separate commercial license.
See the LICENSE.txt in the .agentic folder for full license details.
SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
-->

# REQ-DATA-cffc3af2 - Audit identity and token lifecycle changes

## Behavior

Every successful administrative identity mutation produces one and only one
record in the repository's existing durable Activity stream. The mutation and
append share the existing SQLite administrative transaction: both commit, or
both roll back. A validation refusal, authorization refusal, no-op role or
enabled-state request, and ordinary list/read request produces none of these
mutation records. Existing login and shared access-denied records are separate
common Activity behaviors and are not renamed or duplicated here.

## Data / interface

All mutation records use the common current audit envelope:

- `category`: `audit`
- `component`: `identity`
- `outcome`: `applied`
- `severity`: `info`
- `actor_kind`: `user` for session/CLI identity actors or `token` for API-token actors
- `actor_id`: authenticated identity ID for a user actor or authenticated token ID for a token actor
- `subject`: the affected user ID for user mutations or token ID for token mutations
- `action`: exactly one canonical action from the table below
- appliance, boot, event, sequence, occurrence, recorded-at and time-source fields: composed by the existing common audit mechanism, not by an identity-specific substitute

| Mutation | Canonical action | Safe details |
|---|---|---|
| create user | `identity.user.created` | username, role, enabled |
| change role | `identity.user.role_changed` | previous role, new role |
| enable user | `identity.user.enabled` | previous enabled state, new enabled state |
| disable user | `identity.user.disabled` | previous enabled state, new enabled state |
| reset password | `identity.user.password_reset` | no credential material |
| create API token | `identity.token.created` | name, owner ID, scopes, expiration |
| revoke API token | `identity.token.revoked` | token ID and safe access-impact context |

Details never contain a plaintext password, token secret, password verifier,
token verifier, session verifier, Authorization header, cookie or CSRF token.
Errors and logs must not interpolate the secret-bearing request or response.

The identity service boundary accepts authenticated actor attribution rich
enough to retain both kind and identifier. HTTP handlers derive it exclusively
from the request authentication context. Local CLI calls use user-kind identity
attribution. Client-supplied actor fields are ignored.

The common event composer may be separated from its default repository binding
or rebound to the transaction-owned repository, but the final append must still
pass through the common envelope rules and the transaction's audit repository;
an identity-only event store or post-commit side channel is forbidden.

## Examples

A session authenticated as user `u-admin` changes `u-operator` from Operator
to Viewer. The transaction writes the role and one record with actor kind
`user`, actor ID `u-admin`, subject `u-operator`, action
`identity.user.role_changed`, and safe previous/new role details.

An API token `t-automation`, narrowed to the required administration scope,
revokes token `t-old`. The record has actor kind `token`, actor ID
`t-automation`, subject `t-old` and action `identity.token.revoked`. The owner
identity may appear as safe context but does not replace the token actor ID.

If the database rejects the audit insert while a password reset is attempted,
the password verifier and all existing sessions remain unchanged and no success
record is visible.

## Acceptance

All seven mutation actions are observable through both Activity listing and
export when filtered with `category=audit&component=identity`; every successful
mutation has exactly one complete record; user and token callers have distinct
actor kind and identifier; canary credentials are absent from every stored and
serialized field; and forced audit failures leave identity, token and session
state exactly as it was before each request.

## Assumptions and open decisions

Empty token scopes continue to mean the token is not narrowed and receives its
owner's current role permissions. Exact Go types and whether composition is a
public composer or a transaction-bound writer are design decisions, provided
the common-envelope and atomic single-append invariants above hold.
