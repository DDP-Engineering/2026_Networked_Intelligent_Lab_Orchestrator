# REQ-DATA-0b4ecb84 — Emit complete release evidence

## Behavior

Release assembly creates one canonical evidence tree whose references and
digests close over every artifact and redistributed component. Missing
licences, source obligations, inputs or verification results make release
verification fail rather than produce a complete index.

## Data / interface

The release root contains `manifest.json`, `SHA256SUMS`, `provenance.json`,
`sbom.spdx.json`, `THIRD_PARTY_NOTICES/`, `source-offer/` and target artifact
directories. JSON is UTF-8 canonical/sorted with schema `1`. Manifest entries
contain path, media type, target, architecture, version, size and SHA-256.
Provenance contains source commit/tree status, source epoch, build-environment
digest, target definition digest, input-lock digest and command identity.
Acceptance records use `passed`, `failed` or `not_run` with evidence references.

## Examples

The Pi image record points at the arm64 `.deb`, installer, base-image lock and
static verification while physical checks remain `not_run`. An SBOM package
without a matching notice/source classification fails closure.

## Acceptance

Schema and semantic verification recomputes all digests, validates cross-links
and rejects undeclared files. SPDX validation covers application, language
dependencies, integration runtimes, shared libraries, OS packages, firmware
and base-image identity.

## Assumptions and open decisions

Publication timestamp and signatures are separate non-deterministic records.
No claim of public distribution readiness is made without signing evidence.
