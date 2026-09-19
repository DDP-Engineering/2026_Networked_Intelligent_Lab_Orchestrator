# REQ-REL-6040e6f3 — Create and restore consistent upgrade snapshots

## Behavior

Snapshot creation waits for jobs, stops core before helper, checkpoints and
integrity-checks SQLite, copies the defined irreplaceable set with metadata,
writes a digest manifest, fsyncs the staging tree and atomically renames it.
Restore validates every digest and requires matching application and schema
versions before replacing state while services are stopped.

## Data / interface

Snapshots live by default under `/var/lib/nilo/snapshots/<transaction-id>/` and
retain the last two successful upgrades plus active state. `snapshot.json`
contains schema, transaction ID, source/target versions, database schema,
created source epoch and sorted file records. Preflight requires free space for
the package transaction, one full snapshot and the larger of 256 MiB or ten
percent headroom.

## Examples

A valid snapshot restores identities, integration selection and configuration
with their modes. A snapshot for schema 15 is not started with binaries that
declare schema 14. An interrupted `.staging-*` directory is never a candidate.

## Acceptance

Tests cover WAL checkpoint, database corruption, busy jobs, space refusal,
metadata preservation, fsync/rename failure, digest damage, version mismatch,
retention and interrupted staging.

## Assumptions and open decisions

The first packaged release declares no historical package source version; the
generic mechanism is fixture-tested and later manifests explicitly enumerate
supported sources.
