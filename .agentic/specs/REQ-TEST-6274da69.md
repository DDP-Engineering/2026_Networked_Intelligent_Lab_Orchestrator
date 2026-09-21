# REQ-TEST-6274da69 — Verify every release artifact without network access

## Behavior

The release gate validates structure first, then runs target lifecycle tests in
network-none containers or documented architecture emulation. Results are
target- and check-specific; emulation/static checks cannot satisfy physical
hardware rows.

## Data / interface

`verify-release.sh <release-root>` writes `acceptance.json` only when every
attempted check has evidence. The matrix covers build reproducibility, package
policy/lifecycle, installer clean install/reinstall, injected rollback,
removal/purge, status/GUI/helper/catalogue, integration install/upgrade/health,
image structure and external/internal checksums. Raspberry Pi boot, expansion,
identity, admin consumption, networking, power and GPIO plus mini-PC USB remain
`not_run` without named hardware evidence.

Build reproducibility is executable acceptance: the fixed verifier consumes a
reviewed candidate-build-input document and performs two complete builds in
fresh roots. External artifact directories and caller-authored receipts are not
accepted as proof.

## Examples

An amd64 container can pass Ubuntu package lifecycle but not mini-PC USB. QEMU
can pass arm64 package execution and image structure but not Pi power/GPIO.

## Acceptance

The gate detects a default route, DNS/HTTP attempt, artifact mutation, lifecycle
failure and false hardware promotion. Clean fixture artifacts pass every
hardware-independent row with no network interface beyond loopback.

## Assumptions and open decisions

Virtual Ubuntu is supported for packaging lifecycle evidence when it matches
the exact OS; USB passthrough remains hardware-specific acceptance.
