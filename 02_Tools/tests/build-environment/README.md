# Build-environment acceptance evidence

## What "done" means, in seven distinct states

These are different claims. `evidence.local.json` records them in `maturity`,
`verify-acceptance.py` refuses a later state while an earlier one is unclaimed,
and `fully_accepted` additionally requires every BE gate to have passed.

| State | Means | Proven by |
|---|---|---|
| `implemented` | The source, definition, lock and scripts exist and their contracts pass | the build-environment test module |
| `locally_integrity_checked` | The local manifest, parts and materialized inputs match their recorded digests | `inspect-parts`, `verify-materialized-inputs` |
| `round_trip_verified` | The exported archive re-imported to the exact image digest and the complete offline suite passed | `verify-archive-round-trip.sh` |
| `clean_host_verified` | Clean Windows/WSL2 **and** clean native Ubuntu hosts ran setup successfully | BE-03 on real clean hosts |
| `published` | Every LFS object of the generation is on the authoritative Gitea remote | human publication, then BE-09 readback |
| `independently_backed_up` | A second, independent Git+LFS copy exists and was read back | BE-09 backup readback |
| `fully_accepted` | All of the above, with BE-01 through BE-10 passed | `verify-acceptance.py` exit 0 |

Integrity is not authenticity: these digests prove content matches the reviewed
manifest in this repository, not that a particular publisher produced it. There
is no signed manifest or external trust root, and image/definition scanning is
not implemented. See `../../README.md` for the full statement.

Run source contract tests first, then execute the real gates in order:

1. validate the policy and require a complete, consumed reconstruction lock
   before passing the lock gate (BE-01/BE-02);
2. on clean supported hosts, verify Windows 11 x86-64 plus Ubuntu 24.04 WSL2
   through PowerShell and native Ubuntu 24.04 x86-64 through POSIX; explicitly
   install at most the selected engine from the shared host-bootstrap archive
   only when authorized (BE-03);
3. materialize the recipe's hash-locked upstream records, build the OCI image with
   `--network=none`, and preserve inventory/SBOM/licence/provenance logs,
   including the candidate-tree SHA-256 and base `HEAD` (BE-04);
4. run `with-build-environment.ps1 test`, which disables container networking
   and runs the unprivileged offline suite; privileged appliance/browser
   acceptance runs separately through `tests/appliance/check.sh` (BE-05);
5. build the Sigrok worker for ARM64 and run `verify-arm64-abi.sh` against the
   authorized appliance (BE-06);
6. export, run `verify-archive-failures.sh`, then run
   `verify-archive-round-trip.sh --engine docker --manifest
   02_Tools/archives/build-environment/current.json --archive-path
   02_Tools/archives/build-environment` to import, execute the offline suite,
   compare digests, preserve its log, and promote the manifest status; export
   must stop if current candidate content no longer matches the image label
   (BE-07);
7. record measured complete archive size plus Gitea and backup headroom; no
   content may be omitted and each part must be at most 2 GB (BE-08);
8. publish only by human action, acquire Gitea and backup content into distinct
   clean caches, and verify every OID; do the same for GitHub only if a complete
   replica is declared (BE-09);
9. run `verify-clean-clone.ps1` on already-acquired clean Gitea and GitHub
   clones and reconcile the runbook with observed behavior (BE-10).

Build twice on independent clean runners and compare with `verify-rebuild.sh`.
Different digests mean locked repeatable, not bit-reproducible. Copy
`evidence.example.json` to ignored `evidence.local.json`; fill a gate only from
the named logs/artifacts. `verify-acceptance.py` exits nonzero and reports all
pending/failed gates until every BE-01 through BE-10 record is supported.

An uncommitted reviewed candidate is valid build input: preserve the generated
source-tree SHA-256, then after approval and the human commit run
`build_environment.py verify-committed-source --repository-root .
--expected-tree-sha256 <digest>`. This command requires a clean relevant tree
and identical content; the base commit is context, not a claim that dirty bytes
already equal `HEAD`.

The setup entry point never acquires repository content. Clone/LFS acquisition
from a forge is deliberately a human prerequisite performed before these
commands, and clean-clone verification accepts only an existing local path.

The offline container mounts the checkout and Git metadata read-only, its
isolated runtime cache and declared outputs writable, and optional kernel
evidence read-only. On a Linux executor whose preflight tests require the host
module inventory, explicitly set `NILO_KERNEL_MODULES_PATH=/lib/modules` for
`with-build-environment.sh`, `verify-build-environment.sh` or the archive
round-trip command. Record `uname -r` and the evidence path with the run log.
An absent explicit directory is refused; ordinary runs add no kernel mount.
This evidence mount grants no capabilities and does not run privileged
appliance or browser acceptance.

On WSL2, also set `NILO_WSL_HOST_VOLUME` to an existing directory on the
observed Windows volume backing the distribution. An empty directory is enough:
container checks receive it read-only at `/work/host-volume` and measure its
filesystem free space. The same setting supports host-side restore preflight.
Record the backing-volume observation and chosen evidence directory with the log.

The isolated source copy retains archive control metadata (including the host
bootstrap lock and current image manifest), excludes multipart payloads and
joined runtime archives, and removes empty directories left by excluded
runtime trees. No host directory is removed or changed by this copy step.

The offline frontend suite uses two Vitest workers. It runs every component test
with the normal five-second per-test deadline; bounded concurrency avoids CPU
contention on the supported Windows/WSL2 development host.
