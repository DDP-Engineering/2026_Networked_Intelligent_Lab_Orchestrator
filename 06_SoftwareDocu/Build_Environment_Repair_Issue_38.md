# Offline build environment repair — issue 38

The environment repair closes the current Go, Python and npm source locks against
677 checksum-verified reconstruction inputs. It includes the Camera and Serial
dependencies, pinned pyrav4l2 source and build backend, and iproute2. Offline
preparation checks source-lock coverage before replacing environments. The
isolated source copy retains archive control metadata while excluding downloaded
payloads and host runtime trees. Frontend CI uses two workers with the existing
five-second per-test deadlines.

This is environment work, so no AEGIS task journal or stage workflow was opened.

## Candidate and archive identity

| Item | Value |
| --- | --- |
| Base commit | `a89db2fa89b0fdf8a77f24833555b617122cb344` |
| Candidate source SHA-256 | `1543b8c06bb7f72b639c26ae3ed8930f8356933fe762404b9015c28d52bf2df9` |
| Generation | `noble-20260928-01` |
| OCI digest | `sha256:84578c0371424ee15cf43bb78083cadd45e9bda2ba2bdd6926f0c00b1d0f0568` |
| Reconstruction manifest SHA-256 | `9ce4d99ef0789c5da2f275fd93dff5b73d46bb8c78ac67099c45e5e16ef0f21a` |
| Lock SHA-256 | `11d621f3a761f7e051d4920cda6fee332ce45caa651698f9e2e74b7b4ca63349` |
| Compressed archive | 2,907,371,163 bytes |
| Archive SHA-256 | `386b338aa687273b8db64a182f3741834fa69700440cc5f17f7335edc882ad31` |
| Uncompressed OCI tar | 5,636,498,432 bytes |

The archive uses deterministic gzip headers. Its manifest preserves the OCI
digest and includes measured SBOM, licence and provenance artifacts. Independent
clean-runner bit reproducibility has not been established.

| Part | Bytes | SHA-256 / LFS OID suffix |
| --- | ---: | --- |
| `part000` | 1,073,741,824 | `07bf15245a9a54481b513dabb12a88dd03e45076e506d7bb53dd7ea3c2181649` |
| `part001` | 1,073,741,824 | `c28a2c7fbe9e4da211da3875e859b7e811ee4351cb5717c86e6a671fd320373e` |
| `part002` | 759,887,515 | `7cbc9b87aaabf25a17986811e9128749a79fce696a51bc58401ebc5b51a8a51a` |

## Local validation

- All 677 materialized inputs passed size and SHA-256 validation; exact lock
  coverage and ten focused lock-preparation tests passed.
- Canonical network-disabled image build and inventory passed.
- Software tooling: 202 tests reported, comprising 173 passes and 29 platform
  skips, with no failures.
- Go amd64 tests passed; ARM64 test packages compiled using `-exec=/bin/true`.
  This does not establish physical ARM64 execution or appliance acceptance.
- Python integration suites: 111 tests passed.
- Frontend: 589 tests in 48 files passed with two workers and unchanged deadlines;
  production build passed.
- Real checkout isolation passed: host node_modules and platform virtual
  environments were unchanged.
- Missing, reordered and corrupt archive tests passed; all three exported parts
  passed checksum validation.
- Real archive import and complete offline suite passed on the existing WSL
  executor at `2026-09-28T16:29:40+00:00`. The imported OCI digest matched before
  and after verification. `02_Tools/archives/build-environment/current.json`
  now selects this locally verified generation. Because archive inspection
  rejects extra parts, the prior parts, manifest and metadata are preserved in
  ignored `02_Tools/cache/issue-38/previous-archive/`. The old parts are removed
  from the current escrow candidate; their existing remote objects and historical
  Git records are unchanged.

Logs and local gate evidence are under `02_Tools/cache/issue-38/` and
`02_Tools/tests/build-environment/evidence.local.json`. These are ignored local
records. Successful round-trip evidence is also embedded in archive metadata.

## Authorized WSL engine setup

The existing Ubuntu 24.04 WSL2 distribution now runs native Docker Engine 29.8.0,
containerd 2.3.4, runc 1.5.1, buildx 0.37, Compose 5.5.1 and skopeo 1.13.3.
Docker is active under systemd and stores its data on the Linux filesystem.
The interrupted package configuration was repaired before installation; no
package downgrade was performed and the final dpkg audit was clean.

The obsolete Linux Docker `desktop.exe` credential-store setting was removed
after backing up the configuration privately to
`/home/dpasare/.docker/config.before-native-issue38.json`. Other settings were
preserved. Default Docker access and the pinned Ubuntu pull were verified.

Validation mounts `/lib/modules` read-only and an empty directory on the observed
C: backing volume read-only for space measurement. At import preflight, that
backing volume had approximately 36 GiB free. Neither a new WSL distribution nor
Docker Desktop was installed.

## Shared Gitea CI

The shared Gitea instance runner is registered as ID 1, `dcpserver-runner`,
with instance scope (`owner_id=0`, `repo_id=0`). Actions-enabled repositories
select `linux-amd64`, `ubuntu-24.04` or `ubuntu-latest`; one job runs at a time.
Shared job Docker access was explicitly approved, deployed and smoke-tested.
DCPServer's numbered scripts 22 and 23 and their companions reproduce the
runner settings and canonical NILO image provisioning.

`.gitea/workflows/ci.yaml` handles pushes, same-repository pull requests and
manual dispatch. Its connected controller streams source into owned Docker
volumes and verifies the accepted OCI digest through a pinned Skopeo container.
The test container uses the image's named `ubuntu` account, UID/GID 1000;
recovery tooling requires `id -un` and an arbitrary unnamed UID is insufficient.
Observed Docker settings were network `none`, capabilities `ALL` dropped,
no-new-privileges, 2 CPUs, 6 GiB memory with no additional swap and 1,024 PIDs.
`/workspace` and `/lib/modules` were read-only; `/outputs` was writable.

The workflow plan passed act_runner 0.2.13 dry-run validation. The final nested
suite passed with exit 0, no OOM event and a 615.92-second helper duration:
202 tooling tests (29 platform skips), Go amd64 tests and ARM64 compilation,
111 Python tests, 589 frontend tests across 48 files and the production build.
The GUI artifact contains four files. This run's controller, seed and test
containers and named volumes were removed after evidence collection.
Final suite evidence is retained by the numbered setup script under
`/home/dpasare/apps/nilo-ci-setup/validation-1c64a38ee3ad4b7e824fe6ef8bbc05e8/02_Tools/cache/ci/`.
The result JSON and readable host logs are mirrored in DCPServer's
`6_SoftwareDocu/nilo-ci-{result,provision,validation}-2026-09-28.*`.
Remote event dispatch, authenticated Actions checkout and artifact transport
remain unverified until the workflow is published. See `.gitea/README.md`.

## Remaining acceptance and human publication

Clean Windows/WSL and native Linux host bootstrap acceptance, physical ARM64 and
privileged browser acceptance, authoritative Gitea LFS publication, independent
backup and clean-cache readback remain external gates. Existing-host engine
installation and local archive verification do not satisfy those gates.

The user authorized task closure and local commits on 2026-09-28. Local delivery
carries the repair, archive generation, 55 executable mode corrections, CI
workflow and this evidence report. Unrelated Modbus planning and its task journal
are excluded. No branch creation, Git/LFS publication or pull request is included.

Verify the committed source identity against the manifest before publishing:

```powershell
python .\02_Tools\scripts\build_environment.py verify-committed-source `
  --repository-root . `
  --expected-tree-sha256 1543b8c06bb7f72b639c26ae3ed8930f8356933fe762404b9015c28d52bf2df9
```

The only
configured remote is `origin`, pointing to personal Gitea. With measured free
capacity values, the human publication preflight is:

```powershell
.\02_Tools\scripts\mirror-build-environment-lfs.ps1 `
  -ManifestPath .\02_Tools\archives\build-environment\current.json `
  -GiteaRemote origin `
  -GiteaCapacityBytes <measured-free-bytes> `
  -BackupCapacityBytes <measured-free-bytes>
```

Only the human may add `-ExecutePublication` to publish the complete LFS OID set.
Git ref publication is a separate human-authorized step. Complete checksum
readback from Gitea and an independent backup are required before marking
publication complete. GitHub has no archive replica, which satisfies the
complete-or-absent policy.
