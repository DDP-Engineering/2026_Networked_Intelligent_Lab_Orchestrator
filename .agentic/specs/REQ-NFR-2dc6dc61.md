# REQ-NFR-2dc6dc61 — Build deterministic target payloads offline

## Behavior

The offline builder receives only an identified source tree, one verified
target cache, an empty staging directory and an empty output directory. It
normalizes project-owned files and archives so repeated builds have identical
bytes. Publication time and optional signatures are outside deterministic
payloads and are recorded separately. Release acceptance, rather than its
caller, creates both fresh rebuild roots and invokes the fixed complete-candidate
builder twice.

## Data / interface

`build-release.sh <target> <output>` requires `SOURCE_DATE_EPOCH`, `UTC`, the C
locale, numeric owners, fixed modes and sorted archive members. The source time
comes from `git show -s --format=%ct HEAD`; a source snapshot without Git must
carry the same value in reviewed provenance. The builder rejects a dirty-tree
release policy mismatch, version disagreement or build-host path in output.

## Examples

Two empty staging roots produce identical `.deb` data/control archives and
installer payloads. A human `published_at` value may differ in an external
release record without changing artifact bytes.

## Acceptance

Rebuild comparison covers binaries, GUI, integration bundles, `.deb`, `.run`,
manifests, SBOM and notices. Pi image filesystem identifiers that must be
regenerated at first boot are zeroed rather than normalized to a shared live
identity. Caller-created artifact copies and receipts cannot satisfy the
rebuild comparison.

## Assumptions and open decisions

Compression tools and levels are pinned by the verified build environment.
