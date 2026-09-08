# REQ-REL-37fe088c — Export and verify complete multipart OCI archives

## Behavior

The exporter writes an OCI archive stream with deterministic gzip
metadata, calculates the joined size and SHA-256, splits bytes sequentially,
and calculates each part's size, SHA-256, and LFS OID (`sha256:<hex>`). It
emits `current.json` with pending round-trip status. A separate explicit command
imports that archive, executes the offline suite, stores its evidence log, and
only then promotes the manifest to locally round-trip complete.

## Data / interface

`export-build-environment.sh --engine ENGINE --image REF --generation NAME
[--part-size BYTES] [--output DIR]`. Default part size is 1073741824 bytes;
the accepted range is 1 through 2000000000 bytes. There is no total-size cap.

`current.json` schema 1 records generation, media/compression, expected image
digest, joined archive size/hash, ordered part records, source base commit,
deterministic candidate-tree SHA-256, clean/dirty status, lock-manifest digest,
creation command/builder/platform, SBOM/licence/provenance paths, verification
commands/date, and Gitea/backup/GitHub replica evidence. The content identity
includes relevant tracked and non-ignored untracked paths, file bytes,
deletions, symlink targets, and applicable executable modes. Export rejects an
image whose source-tree label is missing or differs from the current candidate.
After approval, an explicit check requires the eventual clean committed tree to
match the approved candidate digest.
Unknown external evidence is represented as `pending`, never `verified`.

## Examples

A 2.5 GiB archive with default settings becomes three ordered parts. A GitHub
replica declaration is either `complete` with every identical OID/readback or
`absent`; `partial` is invalid. Gitea and independent backup evidence must be
complete for BE-08/BE-09.

## Acceptance

Synthetic deterministic streams round-trip byte-for-byte and failure tests
cover missing, duplicate, unexpected, reordered, corrupt, pointer, oversized,
wrong joined hash, and incomplete-replica cases. Real generation checksums are
accepted only after the image is actually exported and imported.

## Assumptions and open decisions

None about archive limits or replica authority. Measured first-generation size,
Gitea headroom, backup headroom, and optional GitHub allowance remain observed
maintenance evidence rather than design constants.
