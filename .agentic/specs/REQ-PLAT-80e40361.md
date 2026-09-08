# REQ-PLAT-80e40361 — Build locked OCI environment

## Behavior

The environment is a single Linux amd64 OCI image built from one reviewed
Containerfile. Ubuntu packages are selected at exact versions from the locked
Ubuntu 24.04 snapshot and the base image is referenced by manifest digest.
Language archives and browser/npm dependencies are pinned by a complete external
input manifest. Before the engine starts, the materializer verifies all 650
records by size and SHA-256. The build receives only that named input context and
runs with networking disabled. It fails when lock data, materialized content, or
the inventory disagree.

## Data / interface

`02_Tools/containers/build/build-environment.lock.json` is schema 1. It records
the generation, image repository/tag, base reference and digest, snapshot URL,
the apt roots, expected tool version command/output, and the path, digest, and
count of `reconstruction-inputs.json`. That manifest records every transitive
apt package, language/runtime archive, npm tarball, Go proxy object, Python wheel,
and browser payload with its source, consumer, licence, size, and SHA-256.
`Containerfile` accepts only lock-derived build arguments;
`build-build-environment.sh` verifies/materializes the inputs before invoking the
engine with `--network=none`.

The image contains `/opt/nilo/environment/lock.json`, `toolchain.versions`,
`inventory.json`, `sbom.spdx.json`, `provenance.json`, and `licenses/`. The
runtime user is unprivileged; a caller-selected UID/GID is supplied when the
container runs rather than embedded as host identity.

## Examples

`build-build-environment.sh --engine docker` materializes the exact upstream
closure, resolves the locally available digest-pinned base, builds the generation
tag without network access, records its canonical image digest, and runs the
inventory verifier. A missing/tampered payload, package absent from the manifest,
or tag-only base causes a validation error before the engine build.

## Acceptance

The definition check, clean input materialization and network-disabled build,
exact inventory, native and ARM64
compiler/linker smokes, Go cross builds, offline dependency restoration, and
headless browser gate all pass. Two independent clean builds are compared;
until their OCI digests match, documentation says locked repeatable rather than
bit-reproducible.

## Assumptions and open decisions

The source reconstruction closure and a local network-disabled inventory build
are complete. The first generation is not fully accepted until the canonical
archive digest, independent two-build comparison, and hosted storage evidence
are recorded. Source machinery may be complete while those external proofs
remain pending.
