# REQ-PLAT-c98ee5fa — Build lifecycle-safe Debian packages

## Behavior

The builder stages only the declared target payload into Debian filesystem
ownership, generates canonical metadata and constructs the package without
root. Maintainer scripts create the locked service user and mutable directories
idempotently, write the helper caller UID/GID environment, respect
`policy-rc.d`, and never seed identities or start services in an image chroot.

## Data / interface

Artifacts are `nilo_<VERSION>_<amd64|arm64>.deb`. Immutable paths are
`/usr/bin/nilo`, `/usr/libexec/nilo/nilo-nethelper`, `/usr/share/nilo/gui`,
`/usr/lib/nilo/integration-bundles`, units under `/usr/lib/systemd/system`,
defaults under `/etc/nilo`, and notices under `/usr/share/doc/nilo`.
`/var/lib/nilo`, `/var/log/nilo` and `/run/nilo` are created at install time
but contain no packaged mutable data. The package depends only on the exact
target dependency-pool contract.

## Examples

Installing twice keeps local configuration and one `nilo` system identity.
Installing in a chroot with `policy-rc.d` returns success without a NILO
process. Removing preserves state; package-manager purge invokes no interactive
or guessed network cleanup.

## Acceptance

`dpkg-deb --info/--contents`, package-policy checks and lifecycle containers
cover both architectures. Paths, modes, units, conffiles, dependencies,
maintainer idempotence and absence of mutable/development content are checked.

## Assumptions and open decisions

The package version is the existing canonical project version; this task does
not assign another version.
