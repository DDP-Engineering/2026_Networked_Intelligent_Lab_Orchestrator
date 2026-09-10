# REQ-FUNC-a29c7664 — Restore the build image from local multipart content

## Behavior

Setup reads the tracked `current.json` and lock before inspecting only local
archive content. An already imported image is accepted only after canonical
digest and inventory verification. Otherwise restore validates the exact
ordered set of parts, joins them outside the checkout, verifies the complete
archive, imports it, verifies the imported digest/inventory, and removes only
the verified temporary joined file.

## Data / interface

PowerShell: `bootstrap-build-environment.ps1 [-Distribution NAME]
[-Engine docker|podman] [-InstallEngine] [-EngineArchivePath PATH]
[-ArchivePath PATH] [-Offline] [-RebuildFromPinnedInputs]`.

Linux: `bootstrap-build-environment.sh --engine ENGINE [--archive-path PATH]
[--offline|--rebuild-from-pinned-inputs]`.

Ordinary setup cannot combine offline and rebuild. `--archive-path` is a local
directory, never a URL. Missing content output includes filename and LFS OID
and gives manual LFS acquisition and explicit rebuild instructions.

## Examples

A pointer beginning with `version https://git-lfs.github.com/spec/v1` is
reported as unmaterialized. An extra `part999`, duplicate manifest filename,
reordered ordinal, part over 2 GB, wrong SHA-256, wrong joined length/hash, or
wrong imported digest is fatal before container execution.

## Acceptance

Synthetic tests cover successful local restore and all named failures. Source
inspection and injected command tests prove ordinary setup contains no forge
acquisition path. Clean-clone proof from both configured forges is manual and
must be recorded separately.

## Assumptions and open decisions

Engine import inspection differs between Docker and Podman but must normalize
to the same canonical digest string. Real first-generation archive values stay
pending until export is performed.
