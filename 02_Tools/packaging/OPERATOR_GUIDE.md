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
fails. A retained transaction under `/var/lib/nilo/transactions` with phase
`recovery-required` needs local administrator investigation before retrying.

## Raspberry Pi appliance image

Write `nilo-pi4-<version>.img.xz` to the whole target device with a trusted
image writer that verifies the compressed-file checksum. First boot expands
the filesystem and consumes the protected initial-administrator credential.
The GPIO recovery function is disabled by default; enable it only after the
documented wiring and physical acceptance checks have passed.

## Removal and recovery

Removing the Debian package preserves `/var/lib/nilo`. To delete application
state, use `nilo maintenance purge --data-dir /var/lib/nilo --confirm-local`;
the command refuses directories that do not contain a verified NILO database.
Do not manually delete an in-progress transaction. Inspect its journal and the
system journal for `nilo-recovery.service`, correct the reported local cause,
and rerun the packaged recovery helper.

Hardware acceptance entries marked `not_run` are not certification claims.
They must be executed on the named Raspberry Pi or Ubuntu mini-PC before public
distribution.
