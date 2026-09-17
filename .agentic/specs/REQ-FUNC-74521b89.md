# REQ-FUNC-74521b89 - Manage a collision-safe friendly appliance name

## Behavior

Friendly name is optional alias/presentation state and never replaces the
serial, UUID, system hostname or DNS-SD SRV target. Reads use the authenticated
`network.read` permission; mutation uses the existing administrator-only
`network.manage` permission and CSRF enforcement.

Change order is: validate; prepare and activate certificate coverage for the
old and candidate names; publish the candidate in separate per-interface Avahi
groups; persist the friendly name; finalize the helper transaction; withdraw
the old alias and its surplus certificate coverage. If any interface reports a
collision, every candidate group is withdrawn and prior certificate coverage
is restored. Replacement keeps the old alias until the new value is durable.
Clear persists absence before withdrawing the old alias. The serial/service
and `nilo.local` groups are never part of this transaction.

Helper staging is restart-reconcilable. Collision or pre-commit failure frees
only the candidate group and preserves prior certificate, friendly alias and
serial route. Restart converges on the SQLite friendly value after restoring
certificate coverage.

## Data / interface

```http
GET /api/v1/appliance/identity
PUT /api/v1/appliance/friendly-name  {"friendly_name":"bench-west"}
DELETE /api/v1/appliance/friendly-name
```

The PUT body is closed. The label is 1-63 lowercase ASCII characters, begins
and ends `[a-z0-9]`, uses only `[a-z0-9-]`, and cannot equal `nilo`, the serial
name or a reserved service label. Invalid input is
`400 invalid_friendly_name`; link collision is
`409 friendly_name_conflict` with only the candidate and safe interface names.
Identical PUT is `200`; deleting absence is `204`.

## Examples

Replacing `bench-east` with `bench-west` temporarily covers serial, recovery,
old and new names, commits `bench-west`, then withdraws `bench-east`. A link
collision for `bench-west` leaves `bench-east` and the serial route usable.

## Acceptance

- Permission, CSRF and closed-body checks match existing mutations.
- Invalid names are refused, never silently normalized.
- Certificate coverage precedes visibility.
- Every injected failure and restart preserves or restores the serial path.
- Collision detail is bounded and secret-free.

## Resolved design decisions

`network.manage` is the existing administrator-only mutation boundary in the
role matrix. Helper transaction identifiers and the SQLite desired value make
prepare, rollback and restart reconciliation idempotent.
