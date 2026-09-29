# Repository Tools Simplification Plan

**Date:** 2026-09-29\
**Status:** Closed for repository-local delivery; implemented and locally validated\
**Scope:** Developer and operator tooling under `02_Tools`, using the current
Go/Python/React stack\
**Workflow:** Repository tooling procedures; no AEGIS workflow, as requested

## 1. Goal

Make ordinary development understandable from one short help page and one
command entry point per host shell. Reduce duplicate implementations and retire
superseded launchers after updating their callers. Preserve required validation,
offline boundaries, platform support and explicit host/deployment actions.

This refactor is independent of the proposed Go-to-Python migration.

## 2. Observed starting point

The tracked inventory contains 105 `.ps1`, `.sh` and `.py` files:

| Location | Files | Role |
|---|---:|---|
| `bootstrap/` | 11 | Runtime, WSL, container and frontend setup |
| `env/` | 6 | Toolchain and canonical container invocation |
| `lib/` | 2 | Shared Windows/WSL and development helpers |
| `scripts/` | 27 | Mixed developer commands, builders and diagnostics |
| `packaging/` | 22 | Input acquisition, preparation, assembly and verification |
| `tests/` | 37 | Tests and disposable acceptance fixtures |

There are 11 PowerShell/shell pairs. Platform-specific invocation is sometimes
necessary; duplicate substantive logic is the consolidation target. Test count
is not an appropriate measure of developer-facing complexity.

Examples requiring caller analysis:

- `env/with-go.*` overlaps `env/with-tools.*`, but network and appliance fixtures
  still invoke it. Update these callers before retiring the Go-only wrappers.
- `scripts/build-host-bootstrap.sh` delegates to
  `scripts/build-wsl-host-bootstrap.sh`; select one canonical builder and update
  its documentation/tests.
- `scripts/build-appliance.sh` and container `build-all.sh` have different current
  output responsibilities. Reconcile them before presenting one `build` contract.
- Two Pi image scripts have different roles: a Python-builder frontend under
  `packaging/build/` and privileged disk construction under `packaging/image/`.
  Similar names alone do not establish duplication.

## 3. Public interface

Provide `02_Tools/nilo-tools.ps1` on Windows and `02_Tools/nilo-tools.sh` on
supported native Linux. The launchers reuse existing bootstrap, development,
container and packaging orchestration.

| Command | User-visible responsibility |
|---|---|
| `setup` | Prepare the required development toolchains and canonical build environment; retain explicit offline/rebuild, WSL and engine-install options. |
| `dev` | Enter the existing real simulation development flow. Preserve Windows `start`/`status`, visible consoles, profile fences and credentials. Define native Linux behavior explicitly rather than silently inventing process ownership. |
| `doctor` | Report platform, pinned runtime, dependencies and container readiness with useful recovery instructions. |
| `test` | Run canonical offline validation by default; name focused and privileged acceptance routes explicitly. |
| `build` | Produce the documented application/GUI/integration outputs using pinned inputs and declared output paths. |
| `package` | Prepare or assemble an exact-target delivery from complete verified inputs, with distinct actions and honest readiness reporting. |

Help must show examples, supported hosts, outputs and consequential options.
Unknown commands/options fail before side effects. Preserve arguments, quoting,
exit codes and cancellation across shell/WSL boundaries.

Specialist image/engine maintenance, connected input acquisition, archive export,
replica readback and installation remain clearly named advanced actions. Normal
`setup`, `test` or `build` must not silently trigger them. In particular, the
current container `package` action performs preparation, not a finished release;
the new help must preserve that distinction.

## 4. Implementation sequence

1. **Map commands and callers.** Classify each existing script as public command,
   platform adapter, internal helper, test fixture or obsolete candidate. Record
   inputs, outputs, platform, privilege/network behavior and callers in scripts,
   CI, docs, Containerfiles and configured verification commands.
2. **Define and wire the public commands.** Fix command contracts and examples;
   reuse working implementations. A facade is useful only if ordinary workflows
   become discoverable without selecting another entry point manually.
3. **Consolidate actual duplication.** Replace Go-only wrapper callers with the
   shared toolchain route; consolidate redundant launchers and repeated dispatch
   logic. Keep substantive bootstrap behavior usable before managed Python exists.
4. **Organize internal helpers.** Move files only when the move improves ownership
   and discovery. Update every caller in the same candidate. Retire superseded
   public launchers rather than retaining a permanent alias layer.
5. **Reconcile documentation and verify.** Put quick-start/help examples first in
   `02_Tools/README.md`; link specialist runbooks. Update affected development,
   packaging and CI references, then run the gates below.

Start with command discovery and wrapper consolidation. Handle packaging/helper
internals in a later coherent increment if they require changes to artifact
contracts. Do not mechanically combine independent checks into a monolithic
script or rewrite working shell implementations solely to standardize language.

## 5. Verification and completion

- Help, usage refusals, argument forwarding, paths with spaces and exit codes
  work on Windows/WSL and supported native Ubuntu.
- Existing development startup/status and refusal behavior remain covered.
- Existing tooling, dependency-lock, build, packaging and isolation checks pass;
  meaningful regressions cover any changed dispatch or side-effect boundary.
- Offline execution stays offline; source mounts and output permissions retain
  their contracts. Privileged acceptance remains an explicit separate route.
- Input locks, source-tree provenance and canonical OCI contents remain consistent.
  Script changes can affect the relevant source digest: rebuild and verify a new
  image generation when required instead of treating the old digest as acceptance
  of the changed tooling. Preserve archive/publication maturity distinctions.
- Gitea offline validation and artifact upload use the selected canonical route.
  Do not change shared runner settings as part of this refactor.
- Ordinary developer instructions use the six commands, duplicate implementations
  have been removed where proven equivalent, and retired paths have no live callers.
- Record achieved reductions in public entry points and duplicated logic. The
  total script count may remain substantial because tests and focused helpers remain.

## 6. Boundaries

No AEGIS stages/journals for this scripts work. Existing configured callers of
`aegis_test.py` still need to be preserved or deliberately updated; the filename
does not require using the AEGIS workflow for the refactor.

Keep product behavior and the current language stack outside this tooling change.
The earlier Sigrok exclusion applies to the backend migration plan; this tooling
refactor does not delete integration product code.

Git mutations/publication and host installation continue to require the specific
user instructions defined by repository rules. Preserve unrelated planning work.

References: [tools guide](../02_Tools/README.md),
[development flow](../06_SoftwareDocu/Development_and_Simulation_Flow.md),
[packaging plan](Packaging_and_Delivery_Plan.md), and
[Gitea workflow](../.gitea/workflows/ci.yaml).

## 7. Implemented outcome â€” 2026-09-29

- Ordinary work has six public commands and one help page per host shell.
  Windows preserves the existing visible-console development lifecycle; native
  Linux exposes a foreground simulation backend with explicit Linux state storage.
- Removed `env/with-go.ps1`, `env/with-go.sh` and
  `scripts/build-wsl-host-bootstrap.sh` after updating their active callers.
  Go-only execution now selects only pinned Go through `with-tools.*`;
  `build-host-bootstrap.sh` owns the host archive generator directly.
- Container builds publish CLI/helper binaries for both architectures alongside
  the GUI. Container `run` preserves argument boundaries; callers requesting
  compound commands invoke `sh -c` explicitly.
- Local and Gitea offline validation enter the same public Linux test route.
  The existing artifact upload directory includes the new binaries.
- Windows setup and container commands share the registry-based WSL backing
  volume lookup. Container validation receives that free-space evidence and
  observed kernel inventory read-only; development discovery excludes Docker
  Desktop utility distributions.
- Kept specialist packaging, archive and acceptance helpers with distinct
  contracts, including the incremental checks with optional race coverage.
  No mechanical directory moves or product-language migration were needed.

The inventory remains **105 script files**: 67 launchers/helpers and 38 files
under `tests/`. Three duplicate files were retired; two public launchers and one
regression-test module were added. The reduction is in duplicate implementations
and choices required for ordinary work, rather than in necessary test coverage.

Toolchain, dependency locks and image-contained inputs are unchanged. Validation
uses the accepted runtime image against an isolated copy of the candidate source.
This does not relabel the archived image's original source identity or constitute
acceptance/publication of a new archive generation.

The ignored local acceptance record `tests/build-environment/evidence.local.json`
is preserved on the host and excluded from copied source. A direct Windows
tooling run found its pre-existing mismatch between the BE05 detail and pending
gate list; clean-checkout/container validation uses the tracked evidence template.

Nested Python/Ruff cache directories are excluded from all three source-copy
routes. This also removes empty remnants of the retired Python contract fixture
from the isolated copy while preserving generated host files.

Verification completed on Windows with Ubuntu WSL2 and Docker:

- `nilo-tools.ps1 test -Distribution Ubuntu` exited **0** through the public
  Windows â†’ WSL â†’ canonical network-disabled container route. It passed 218
  tooling tests (35 platform/acceptance skips), Go amd64 tests and arm64
  compilation checks, 111 integration/contract Python tests, and 589 frontend
  tests across 48 files. Lint and the production GUI build also passed.
- The 16 command regression cases passed across Windows and Linux: six execute
  on Windows and ten on Linux, with the other platform's cases explicitly skipped.
- `doctor tools` passed for the pinned Windows and WSL toolchains.
- The host-isolation gate passed; container preparation left host dependency
  directories and sentinel content unchanged. Real-copy regression checks pass
  for local, environment-verification and Gitea CI source-copy commands.
- Published all four static Linux CLI/helper executables and the GUI under
  `02_Tools/cache/containers/outputs/`; inspected amd64/arm64 executable types
  and ran the amd64 CLI's `version` command successfully.
- PowerShell parsing, shell syntax, Python compilation, new-test Ruff checks
  and `git diff --check` passed. Ruff's UP009 was excluded to retain the
  encoding header required by repository code style.

Logs are retained under ignored `02_Tools/cache/` as
`tools-simplification-offline.log`, `tools-simplification-isolation.log` and
`tools-simplification-doctor.log`. Earlier failed runs are retained separately.
The GUI build retains its existing bundle-size warning.

No shared runner settings, runtime/dependency locks, Git state or archive
generation were changed. The Gitea route is wired to the shared command and
its source-copy behavior is tested; no remote run was triggered for this
uncommitted candidate. Physical appliance acceptance remains a separate gate.

## 8. Verification after the master merge — 2026-09-29

Master `59fb237` was merged into `feature/38-add_ci` as `0d18497`, preserving
this uncommitted tooling candidate. The merged production source is NILO 0.12.0.
The offline suite passed against the pinned, network-disabled runtime: 218
tooling tests (35 skips), Go amd64 tests and arm64 compilation checks, 112
Python integration/contract tests, and 596 frontend tests across 49 files.
Lint, formatting and production builds passed. Evidence is retained in
`02_Tools/cache/rpi-smoke-20260929/build-merged.log`.

Master changes the frontend lockfile's project version and recorded SHA-256.
Consequently, the merged environment lock differs from the accepted image's
embedded lock. The public `test --in-container` route retains its exact-match
refusal; it was not weakened. Post-merge validation invoked the existing
`verify-offline.sh` suite directly inside the digest-verified image, verifying
the merged source's locked input coverage. This does not certify a matching
archive generation or a passing Gitea run. The accepted image and archive
metadata need a separate refresh before that public/CI route can run this
merged lock.

## 9. Close-out — 2026-09-29

The user requested task closure and a local commit of the prepared files.
Tooling simplification and the current-source Raspberry Pi deployment are
complete. This delivery includes the script changes, regression coverage,
updated guides, deployment evidence and the previously prepared Python/Modbus
plans. Those plans remain proposals; their implementations were not performed.

The master merge and successful Pi deployment are recorded in
[the deployment report](../07_Delivery/NILO_0.12.0_RPi_Current_20260929.md).
The accepted build-image/archive refresh identified in section 8 remains a
separate follow-up. No push, new remote CI run or archive publication is part
of this local close-out. Ignored runtimes, cache files, payloads and private
appliance state are excluded from the commit.

## 10. CI compatibility with version updates — 2026-09-29

The user requested that routine version updates stop requiring an image refresh.
The shared CI/local container entry point now requires isolated source and
readable image evidence without comparing the source environment lock against
the image's build-time copy. The outer controller still verifies the pinned OCI
digest, and offline preparation verifies current dependency coverage and package
integrity. New dependencies absent from the verified cache still need updated
environment inputs.

This supersedes the version-only image-refresh follow-up in sections 8 and 9.
It does not publish or relabel an archive generation. Regression tests cover
changed project metadata and continued refusal of host or missing-image context.
The change is local; existing failed Gitea runs are historical results.

The public `nilo-tools.ps1 test -Distribution Ubuntu` command passed with exit
0 using the existing digest-verified, network-disabled image and the current
0.12.0 source. It covered 220 tooling cases (35 skipped), Go amd64 tests and
arm64 compilation checks, 112 Python integration/contract tests, and 596 frontend
tests across 49 files. Lint and production builds passed. All 18 command
regression cases passed across Windows and Linux with platform-specific skips.
Shell syntax, new-test Ruff checks and `git diff --check` passed. The full log
is retained under ignored `02_Tools/cache/ci-version-compatibility-offline.log`.
