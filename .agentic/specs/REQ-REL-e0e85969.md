# REQ-REL-e0e85969 — Roll back a failed offline installation

## Behavior

The installer persists a transaction record before each mutation. Any failure
after mutation starts invokes reverse steps: stop new services, restore the
prior package set from retained local package bytes, restore the matching state
snapshot, integrity-check it, and start only the previous version. If a reverse
step cannot be verified, services remain stopped and the recovery record names
the exact operator action and retained paths.

## Data / interface

`transaction.json` has a closed phase vocabulary: `preflight`, `quiesced`,
`snapshotted`, `dependencies`, `package`, `migrated`, `started`, `verified`,
`committed`, `rolling_back`, `rolled_back`, `recovery_required`. Each record
contains target/version/snapshot/package digests and is atomically replaced and
fsynced. Startup with a nonterminal record resumes rollback, never forward
installation.

## Examples

A failed new-core health check restores the old `.deb` and snapshot. A power
loss after package installation is observed as a nonterminal record and rolls
back. A damaged prior package leaves services stopped at `recovery_required`.

## Acceptance

Failure injection and interruption tests cover every phase and show either the
complete old state or committed new state—never mixed binaries/database—and
secret-free retained recovery instructions.

## Assumptions and open decisions

No `--force` skips rollback verification. Active jobs time out after 60 seconds
and fail before snapshot/mutation.
