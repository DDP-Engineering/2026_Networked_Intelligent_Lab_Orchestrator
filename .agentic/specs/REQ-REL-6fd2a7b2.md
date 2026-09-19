# REQ-REL-6fd2a7b2 — Upgrade installed integrations atomically

## Behavior

Upgrade compares installed and catalogue version/digest, waits up to 60 seconds
for related jobs, stages and validates the new bundle, runs an optional
versioned state migration, atomically switches the active directory, performs
the health check and records the outcome. Busy timeout refuses without force.
Any post-staging failure restores the old active directory and state snapshot.

## Data / interface

`nilo plugin upgrade --plugin <id>` is an authenticated API client like other
plugin commands; the HTTP lifecycle action is `upgrade`. Installed metadata
records bundle digest, version, enabled state, mode and state schema. The active
path changes through a same-filesystem rename; staging and rollback paths are
confined beneath the integration root. New catalogue entries remain uninstalled.

## Examples

An enabled Ask-mode Sigrok 1.0 becomes Sigrok 1.1 and remains enabled in Ask
mode. A failed 1.1 health check leaves 1.0 active and records a rollback audit
outcome. An active capture that does not finish in 60 seconds returns busy.

## Acceptance

Success, no-op, busy, invalid package, migration failure, health failure,
rename interruption and audit failure cases leave exactly one usable active
version and a truthful durable state.

## Assumptions and open decisions

Integration state migrations are package-declared forward steps paired with a
rollback snapshot; arbitrary downgrade migrations are unsupported.
