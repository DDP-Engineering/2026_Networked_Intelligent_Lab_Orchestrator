# NILO offline delivery operator guide

Use only the artifact for the exact operating-system target named in
`manifest.json`. Before installation, run `sha256sum -c SHA256SUMS` from the
release directory and keep the release metadata with the installed system.

## Existing Ubuntu or Raspberry Pi OS installation

Run the target `.run` file as root from local storage. The installer verifies
its embedded package set, refuses a mismatched OS or architecture, requires an
active NetworkManager service, and uses APT with all network sources disabled.
It snapshots existing NILO state before an upgrade and automatically restores
the prior packages and state if migration, startup, or health verification
fails. A retained root-only transaction under
`/var/lib/nilo-installer/transactions` with phase
`recovery-required` needs local administrator investigation before retrying.
The release acceptance record is valid only when the repository-owned verifier
executes `build-candidate-set.sh` twice in fresh output roots from the reviewed
candidate-build-input document. Each run starts from the identified Git
revision and lock-bound Go, npm and target-runtime caches, rebuilding binaries,
the production GUI, integration bundles and payloads before final packaging.
Each invocation identity is established before
its builders run; its embedded receipts bind the builder program, command,
artifact path, inputs, source revision and input lock; and all resulting bytes
must match the candidate. Caller-supplied copies and receipts are not accepted
as reproducibility proof. Lifecycle acceptance also requires already-local,
digest-pinned exact-target container images configured through
`NILO_ACCEPTANCE_IMAGE_UBUNTU_24_04_AMD64` and
`NILO_ACCEPTANCE_IMAGE_RPI_DEBIAN13_ARM64`; every destructive row receives a
fresh `--network none` system container. Evidence records the configured image
reference and observed image ID, kernel, OS release and architecture. The matrix
covers successful upgrade, clean rollback, real package-operation faults, each
mutation phase, and interrupted recovery at every durable marker.
Installer rollback evidence additionally binds the separately built prior
version used by the fault-injection check.

## Raspberry Pi appliance image

Write `nilo-pi4-<version>.img.xz` to the whole target device with a trusted
image writer that verifies the compressed-file checksum. First boot expands
the filesystem and consumes the protected initial-administrator credential.
That source remains root-owned mode `0600` under the root-only
`/var/lib/nilo-installer` directory until the root one-shot copies it to an
ephemeral service-user handoff under `/run`. The handoff is removed on every
exit and the source only after successful seeding; do not change their
ownership or permissions.
The GPIO recovery function is disabled by default; enable it only after the
documented wiring and physical acceptance checks have passed.

## Removal and recovery

Removing the Debian package preserves `/var/lib/nilo`. To delete application
state, use `nilo maintenance purge --data-dir /var/lib/nilo --confirm-local`;
the command refuses directories that do not contain a verified NILO database.
Run that command locally before removing the package: it creates the verified
sibling backup `/var/lib/nilo-purge-backup`, asks the privileged helper to
remove only resources backed by durable ownership records, and deletes state
only after cleanup succeeds. A retry reuses that backup only while it still
verifies against unchanged bytes and complete file and directory modes,
ownership, ACLs and extended attributes; after a successful purge, move
it to protected recovery storage. Non-interactive package removal stops Bench Core,
cleans resources that cannot affect an active management tunnel, preserves the
application state and any possible sole management path, then stops the helper.
Foreign or merely similarly named NetworkManager and nftables objects are
reported and retained.
Do not manually delete an in-progress transaction. Inspect its journal and the
system journal for `nilo-recovery.service`, correct the reported local cause,
and rerun the packaged recovery helper.

Hardware acceptance entries marked `not_run` are not certification claims.
They must be executed on the named Raspberry Pi or Ubuntu mini-PC before public
distribution.
