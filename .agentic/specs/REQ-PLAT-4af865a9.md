# REQ-PLAT-4af865a9 — Resolve complete target release inputs

## Behavior

Connected resolution starts from one exact target and a repository-owned input
declaration. It emits a canonical lock proposal and content-addressed objects.
The proposal becomes `complete: true` only when it covers the target's Go and
GUI build inputs, production integration runtimes, OS dependency pool and
repository metadata, release tools, and—for Raspberry Pi—the exact base image.
The release engineer reviews the generated versions, origins, licences and
redistribution classifications before adopting the lock.

## Data / interface

`config/release-inputs.json` names logical inputs and resolver types; it never
contains credentials. `resolve-inputs <target> --output <proposal>` runs only in
the connected phase. Each proposed `inputs.lock.json` entry contains `name`,
`scope`, `kind`, `version`, `architecture`, `url`, `license`, `redistribution`,
`sha256`, `size` and optional package/base-image provenance. Entries and
incomplete reasons are sorted. Cache paths remain digest-derived.

## Examples

An Ubuntu proposal includes every `.deb` in its resolved local APT pool. A Pi
proposal additionally names one dated 64-bit Raspberry Pi OS Lite image based
on Debian 13. A Sigrok binary without reviewable redistribution evidence keeps
the proposal incomplete.

## Acceptance

Fixture resolvers produce stable complete proposals for both targets. Missing
input classes, repository-signature failure, changed bytes, architecture drift
and unreviewed redistribution all fail closed. Offline commands never invoke a
resolver.

## Assumptions and open decisions

The exact Pi image release and digest are selected from the official Raspberry
Pi download metadata during connected acquisition and then pinned; mutable
"latest" URLs never enter an active lock.
