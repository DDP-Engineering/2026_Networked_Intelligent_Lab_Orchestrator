# NILO Gitea CI

`workflows/ci.yaml` runs on push, same-repository pull requests and manual
dispatch, selecting the shared instance runner's `linux-amd64` label. Fork pull
requests are skipped because this runner provides host Docker access.

The connected Ubuntu controller checks out source without LFS payloads or
persistent credentials. It verifies the locally provisioned canonical image's
OCI digest using a pinned Skopeo container, then streams the checkout into a
per-run Docker volume. This supports a checkout inside a runner job container
without treating its path as a host directory.

The inner test container uses the image selected by
`02_Tools/archives/build-environment/current.json`, networking disabled,
capabilities dropped, no-new-privileges, the named `ubuntu` account (UID/GID
1000), two CPU cores,
6 GiB memory without extra swap and 1,024 processes. `/lib/modules` is an
explicit read-only host evidence mount. Source is copied into the container's
writable work directory; the original checkout remains unchanged. The complete
offline suite has a 45-minute deadline with a 30-second termination grace.

CI does not compare the current project version or source-lock file hash with
the image's build-time copy. Version bumps and their lockfile metadata changes
use the existing provisioned image. Offline preparation checks the current
source's dependency coverage and package integrity; dependencies absent from
the image's verified input cache still require updated environment inputs.
The provisioned OCI image digest remains pinned independently of the project
version.

`02_Tools/cache/ci/` holds the suite log, result JSON and production GUI output.
The workflow attempts artifact upload on success and failure, retaining evidence
for 14 days. Containers and volumes created by the helper are removed in its
cleanup; their unique names keep cleanup scoped to the current run.

Provision the canonical image and Skopeo tool image on DCPServer through
`2025_DCP_Server/5_Software/23_nilo_ci_setup.sh` and its `nilo-ci/` companions.
An archive-generation change requires explicit provisioning of its verified
local parts. CI fails preflight when the accepted image is absent or stale; it
does not silently rebuild, fetch an environment archive or replace host images.

Actions must be enabled in the repository. Checkout uses `http://gitea:3000`
on `home_net`; the host's published LAN address is not reachable from these
containers under the current firewall policy. Action versions are pinned to
commits; upload-artifact v3 is selected for act_runner 0.2.13 compatibility.

The local nested-container acceptance exercises the actual helper and offline
suite. Remote event dispatch, authenticated checkout and artifact transport
require the committed workflow to be published to Gitea and a real Actions run.
