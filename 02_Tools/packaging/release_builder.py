#!/usr/bin/env python3
"""Build and verify deterministic offline NILO delivery artifacts."""

from __future__ import annotations

import argparse
import datetime
import hashlib
import io
import json
import lzma
import os
import re
import shutil
import stat
import subprocess
import sys
import tarfile
import tempfile
from pathlib import Path, PurePosixPath
from typing import Any, Iterable


TARGET_ARCHITECTURES = {
    "rpi-debian13-arm64": "arm64",
    "ubuntu-24.04-amd64": "amd64",
}
MUTABLE_PREFIXES = ("var/lib/nilo", "var/log/nilo", "run/nilo")
DEFAULT_POLICY = Path(__file__).with_name("config") / "forbidden-release-content.json"
DEBIAN_ROOT = Path(__file__).with_name("debian")
IMAGE_BUILDER = Path(__file__).with_name("image") / "build-pi-image.sh"
IMAGE_VERIFIER = Path(__file__).with_name("verify") / "verify-image.sh"
OPERATOR_GUIDE = Path(__file__).with_name("OPERATOR_GUIDE.md")
SEMVER = re.compile(r"^[0-9]+\.[0-9]+\.[0-9]+(?:[-+][0-9A-Za-z.-]+)?$")
DEBIAN_PACKAGE_NAME = re.compile(r"^[A-Za-z0-9][A-Za-z0-9.+_:%~-]*\.deb$")
DEBIAN_BINARY_PACKAGE = re.compile(r"^[a-z0-9][a-z0-9+.-]*$")
DEBIAN_VERSION = re.compile(r"^[A-Za-z0-9][A-Za-z0-9.+:~_-]*$")
SOFTWARE_ACCEPTANCE_CHECKS = (
    "build-reproducibility",
    "package-policy",
    "package-install",
    "package-reinstall",
    "installer-clean-install",
    "installer-rollback",
    "removal-purge",
    "core-status-gui-helper-catalogue",
    "integration-install-upgrade-health",
    "checksums",
    "offline-boundary",
)
REQUIRED_RELEASE_INPUTS = {
    "shared": {"go-module-cache", "gui-npm-cache", "source-offers"},
    "rpi-debian13-arm64": {"base-image", "os-package-pool", "sigrok-runtime"},
    "ubuntu-24.04-amd64": {"os-package-pool", "sigrok-runtime"},
}


class ReleaseError(ValueError):
    """Report a release-policy or artifact-integrity refusal."""


def canonical_json(value: object) -> bytes:
    """Encode one stable JSON document."""
    return (json.dumps(value, indent=2, sort_keys=True) + "\n").encode("utf-8")


def sha256_file(path: Path) -> str:
    """Hash a regular file without reading it all into memory."""
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        while chunk := stream.read(1024 * 1024):
            digest.update(chunk)
    return digest.hexdigest()


def sha256_bytes(value: bytes) -> str:
    """Hash an in-memory artifact."""
    return hashlib.sha256(value).hexdigest()


def _safe_relative(path: Path, root: Path) -> str:
    relative = path.relative_to(root).as_posix()
    pure = PurePosixPath(relative)
    if not relative or pure.is_absolute() or ".." in pure.parts:
        raise ReleaseError(f"unsafe payload path: {relative!r}")
    return relative


def _load_policy(path: Path = DEFAULT_POLICY) -> dict[str, Any]:
    try:
        policy = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as error:
        raise ReleaseError(f"cannot load release scan policy {path}: {error}") from error
    if policy.get("schema") != 1:
        raise ReleaseError("release scan policy schema must be 1")
    return policy


def _iter_tree(root: Path) -> Iterable[tuple[Path, os.stat_result, str]]:
    if not root.is_dir():
        raise ReleaseError(f"release tree is not a directory: {root}")
    for path in sorted(root.rglob("*"), key=lambda item: item.relative_to(root).as_posix()):
        try:
            details = path.lstat()
        except OSError as error:
            raise ReleaseError(f"cannot inspect release path {path}: {error}") from error
        relative = _safe_relative(path, root)
        if stat.S_ISLNK(details.st_mode):
            raise ReleaseError(f"symbolic link is forbidden in a release: {relative}")
        if not (stat.S_ISDIR(details.st_mode) or stat.S_ISREG(details.st_mode)):
            raise ReleaseError(f"special file is forbidden in a release: {relative}")
        yield path, details, relative


def scan_release_tree(root: Path, policy_path: Path = DEFAULT_POLICY) -> dict[str, object]:
    """Refuse test fixtures, secrets, host paths and undeclared special files."""
    root = Path(root).resolve()
    policy = _load_policy(Path(policy_path))
    path_rules = [
        (entry["id"], re.compile(entry["pattern"]))
        for entry in policy.get("path_rules", [])
    ]
    content_rules = [
        (entry["id"], re.compile(entry["pattern"].encode("utf-8"), re.IGNORECASE))
        for entry in policy.get("content_rules", [])
    ]
    checked = 0
    for path, details, relative in _iter_tree(root):
        checked += 1
        for rule_id, pattern in path_rules:
            if pattern.search(relative):
                raise ReleaseError(f"forbidden content rule {rule_id}: {relative}")
        if not stat.S_ISREG(details.st_mode):
            continue
        chunk_bytes = int(policy.get("content_scan_max_bytes", 4 * 1024 * 1024))
        if chunk_bytes < 4096:
            raise ReleaseError("release scan content chunk must be at least 4096 bytes")
        tail = b""
        with path.open("rb") as stream:
            while chunk := stream.read(chunk_bytes):
                data = tail + chunk
                for rule_id, pattern in content_rules:
                    if pattern.search(data):
                        raise ReleaseError(f"forbidden content rule {rule_id}: {relative}")
                tail = data[-4096:]
    return {"schema": 1, "files_checked": checked, "status": "pass"}


def _validate_payload_path(relative: str) -> None:
    for prefix in MUTABLE_PREFIXES:
        if relative == prefix or relative.startswith(prefix + "/"):
            raise ReleaseError(f"mutable state is forbidden in a package payload: {relative}")


def _tar_info(name: str, mode: int, size: int, epoch: int, *, directory: bool) -> tarfile.TarInfo:
    info = tarfile.TarInfo(name + ("/" if directory and not name.endswith("/") else ""))
    info.type = tarfile.DIRTYPE if directory else tarfile.REGTYPE
    info.mode = mode
    info.size = 0 if directory else size
    info.mtime = epoch
    info.uid = 0
    info.gid = 0
    info.uname = ""
    info.gname = ""
    return info


def build_payload(
    source: Path,
    output: Path,
    *,
    target: str,
    version: str,
    source_date_epoch: int,
    offline_receipt: Path,
) -> dict[str, object]:
    """Create one canonical target payload and return its receipt."""
    if target not in TARGET_ARCHITECTURES:
        raise ReleaseError(f"unsupported target: {target}")
    if not SEMVER.fullmatch(version):
        raise ReleaseError(f"invalid project version: {version}")
    if source_date_epoch < 0:
        raise ReleaseError("SOURCE_DATE_EPOCH must not be negative")
    _validate_offline_receipt(offline_receipt, target)
    source = Path(source).resolve()
    output = Path(output).resolve()
    if output == source or source in output.parents:
        raise ReleaseError("payload output must be outside the source tree")
    scan_release_tree(source)

    entries: list[tuple[str, Path | None, int, int, bool]] = []
    files: list[dict[str, object]] = []
    for path, details, relative in _iter_tree(source):
        _validate_payload_path(relative)
        archive_name = f"payload/{relative}"
        if stat.S_ISDIR(details.st_mode):
            entries.append((archive_name, None, 0o755, 0, True))
            continue
        executable_path = relative.startswith(("usr/bin/", "usr/libexec/"))
        mode = 0o755 if executable_path or details.st_mode & 0o111 else 0o644
        digest = sha256_file(path)
        entries.append((archive_name, path, mode, details.st_size, False))
        files.append(
            {
                "mode": f"{mode:04o}",
                "path": relative,
                "sha256": digest,
                "size": details.st_size,
            }
        )
    manifest = {
        "architecture": TARGET_ARCHITECTURES[target],
        "files": files,
        "schema": 1,
        "source_date_epoch": source_date_epoch,
        "target": target,
        "version": version,
    }
    manifest_bytes = canonical_json(manifest)
    entries.append(("payload-manifest.json", None, 0o644, len(manifest_bytes), False))
    entries.sort(key=lambda entry: entry[0])

    output.parent.mkdir(parents=True, exist_ok=True)
    descriptor, temporary_name = tempfile.mkstemp(prefix=f".{output.name}.", suffix=".tmp", dir=output.parent)
    os.close(descriptor)
    temporary = Path(temporary_name)
    try:
        with tarfile.open(temporary, "w:xz", format=tarfile.PAX_FORMAT, preset=9) as archive:
            for name, path, mode, size, directory in entries:
                info = _tar_info(name, mode, size, source_date_epoch, directory=directory)
                if name == "payload-manifest.json":
                    archive.addfile(info, io.BytesIO(manifest_bytes))
                elif directory:
                    archive.addfile(info)
                else:
                    assert path is not None
                    with path.open("rb") as stream:
                        archive.addfile(info, stream)
        os.replace(temporary, output)
    except Exception:
        temporary.unlink(missing_ok=True)
        raise
    return {
        "architecture": TARGET_ARCHITECTURES[target],
        "artifact": output.name,
        "schema": 1,
        "sha256": sha256_file(output),
        "size": output.stat().st_size,
        "target": target,
        "version": version,
    }


def _archive_bytes(entries: Iterable[tuple[str, bytes, int]], epoch: int) -> bytes:
    output = io.BytesIO()
    with tarfile.open(fileobj=output, mode="w:xz", format=tarfile.PAX_FORMAT, preset=9) as archive:
        for name, contents, mode in sorted(entries, key=lambda entry: entry[0]):
            info = _tar_info(name, mode, len(contents), epoch, directory=False)
            archive.addfile(info, io.BytesIO(contents))
    return output.getvalue()


def _payload_entries(payload: Path) -> tuple[dict[str, Any], list[tuple[str, bytes, int]]]:
    try:
        archive = tarfile.open(payload, "r:xz")
    except (OSError, tarfile.TarError) as error:
        raise ReleaseError(f"cannot open target payload: {error}") from error
    with archive:
        members = archive.getmembers()
        names = [member.name for member in members]
        if names != sorted(names) or names.count("payload-manifest.json") != 1:
            raise ReleaseError("payload archive is not canonical")
        manifest_stream = archive.extractfile("payload-manifest.json")
        if manifest_stream is None:
            raise ReleaseError("payload manifest is missing")
        try:
            manifest = json.load(manifest_stream)
        except (UnicodeDecodeError, json.JSONDecodeError) as error:
            raise ReleaseError(f"payload manifest is invalid: {error}") from error
        if set(manifest) != {"architecture", "files", "schema", "source_date_epoch", "target", "version"} or manifest.get("schema") != 1:
            raise ReleaseError("payload manifest fields are invalid")
        declared_files = manifest.get("files")
        if not isinstance(declared_files, list):
            raise ReleaseError("payload manifest files are invalid")
        entries: list[tuple[str, bytes, int]] = []
        observed_files: list[dict[str, object]] = []
        observed_names: set[str] = set()
        for member in members:
            pure = PurePosixPath(member.name)
            if pure.is_absolute() or ".." in pure.parts or member.issym() or member.islnk():
                raise ReleaseError(f"unsafe payload archive member: {member.name}")
            if member.name == "payload-manifest.json" or member.isdir():
                continue
            if not member.isfile() or not member.name.startswith("payload/"):
                raise ReleaseError(f"unexpected payload archive member: {member.name}")
            stream = archive.extractfile(member)
            if stream is None:
                raise ReleaseError(f"cannot read payload member: {member.name}")
            relative = member.name.removeprefix("payload/")
            _validate_payload_path(relative)
            if relative in observed_names:
                raise ReleaseError(f"duplicate payload archive member: {relative}")
            observed_names.add(relative)
            contents = stream.read()
            mode = stat.S_IMODE(member.mode)
            entries.append((f"./{relative}", contents, mode))
            observed_files.append(
                {
                    "mode": f"{mode:04o}",
                    "path": relative,
                    "sha256": sha256_bytes(contents),
                    "size": len(contents),
                }
            )
        if observed_files != declared_files:
            raise ReleaseError("payload manifest digest mismatch")
    return manifest, entries


def _maintainer_entries(version: str, architecture: str, installed_size: int) -> list[tuple[str, bytes, int]]:
    control = f"""Package: nilo
Version: {version}
Section: science
Priority: optional
Architecture: {architecture}
Maintainer: DDP Engineering
Depends: adduser, systemd
Installed-Size: {installed_size}
Description: Networked Intelligent Lab Orchestrator
 NILO coordinates bench integrations through a local appliance service.
""".encode("utf-8")
    postinst = b"""#!/bin/sh
set -e
if ! getent group nilo >/dev/null 2>&1; then addgroup --system nilo; fi
if ! id nilo >/dev/null 2>&1; then adduser --system --ingroup nilo --home /var/lib/nilo --no-create-home nilo; fi
install -d -o nilo -g nilo -m 0750 /var/lib/nilo /var/log/nilo /run/nilo
service_uid=$(id -u nilo)
service_gid=$(id -g nilo)
umask 027
printf 'NILO_CALLER_UID=%s\nNILO_CALLER_GID=%s\n' "$service_uid" "$service_gid" >/etc/nilo/nethelper.env
chown root:root /etc/nilo/nethelper.env
chmod 0640 /etc/nilo/nethelper.env
if [ -d /run/systemd/system ]; then
  systemctl daemon-reload || true
  deb-systemd-helper enable nilo-nethelper.socket nilo-recovery.service nilo-firstboot.service nilo.service >/dev/null || true
  if [ ! -x /usr/sbin/policy-rc.d ] || /usr/sbin/policy-rc.d start nilo.service; then
    deb-systemd-invoke start nilo-nethelper.socket nilo-recovery.service nilo-firstboot.service nilo.service >/dev/null || true
  fi
fi
exit 0
"""
    prerm = b"""#!/bin/sh
set -e
if [ "$1" = remove ] && [ -d /run/systemd/system ]; then
  deb-systemd-invoke stop nilo.service nilo-firstboot.service nilo-recovery.service nilo-nethelper.service >/dev/null || true
fi
exit 0
"""
    postrm = b"""#!/bin/sh
set -e
if [ -d /run/systemd/system ]; then systemctl daemon-reload || true; fi
# Application state is intentionally preserved. Use `nilo maintenance purge`
# for an explicit ownership-checked deletion.
exit 0
"""
    return [
        ("./conffiles", b"/etc/nilo/nilo.env\n/etc/nilo/firstboot.env\n", 0o644),
        ("./control", control, 0o644),
        ("./postinst", postinst, 0o755),
        ("./postrm", postrm, 0o755),
        ("./prerm", prerm, 0o755),
    ]


def _ar_member(name: str, contents: bytes, epoch: int) -> bytes:
    encoded_name = (name + "/").encode("ascii")
    if len(encoded_name) > 16:
        raise ReleaseError(f"ar member name is too long: {name}")
    fields = [
        encoded_name.ljust(16, b" "),
        str(epoch).encode("ascii").ljust(12, b" "),
        b"0".ljust(6, b" "),
        b"0".ljust(6, b" "),
        b"100644".ljust(8, b" "),
        str(len(contents)).encode("ascii").ljust(10, b" "),
        b"`\n",
    ]
    body = b"".join(fields) + contents
    return body + (b"\n" if len(contents) % 2 else b"")


def build_deb(
    payload: Path,
    output: Path,
    *,
    target: str,
    version: str,
    source_date_epoch: int,
    offline_receipt: Path,
) -> dict[str, object]:
    """Build one deterministic architecture-specific Debian binary package."""
    if target not in TARGET_ARCHITECTURES:
        raise ReleaseError(f"unsupported target: {target}")
    if not SEMVER.fullmatch(version):
        raise ReleaseError(f"invalid project version: {version}")
    _validate_offline_receipt(offline_receipt, target)
    manifest, data_entries = _payload_entries(Path(payload))
    if manifest.get("target") != target:
        raise ReleaseError("payload target mismatch")
    if manifest.get("version") != version:
        raise ReleaseError("payload version mismatch")

    known_names = {name for name, _, _ in data_entries}
    for unit in sorted((DEBIAN_ROOT / "units").iterdir(), key=lambda item: item.name):
        if unit.suffix not in {".service", ".socket", ".timer"} or not unit.is_file():
            continue
        name = f"./usr/lib/systemd/system/{unit.name}"
        if name in known_names:
            raise ReleaseError(f"payload duplicates package-owned unit: {name}")
        data_entries.append((name, unit.read_bytes(), 0o644))
    recovery_helper = DEBIAN_ROOT / "nilo-installer-recover"
    data_entries.append(
        ("./usr/libexec/nilo/nilo-installer-recover", recovery_helper.read_bytes(), 0o755)
    )
    if "./etc/nilo/firstboot.env" not in known_names:
        data_entries.append(("./etc/nilo/firstboot.env", b"NILO_FIRST_ADMIN_USERNAME=admin\n", 0o640))
    installed_size = (sum(len(contents) for _, contents, _ in data_entries) + 1023) // 1024
    control_archive = _archive_bytes(
        _maintainer_entries(version, TARGET_ARCHITECTURES[target], installed_size),
        source_date_epoch,
    )
    data_archive = _archive_bytes(data_entries, source_date_epoch)
    package = b"!<arch>\n" + b"".join(
        [
            _ar_member("debian-binary", b"2.0\n", source_date_epoch),
            _ar_member("control.tar.xz", control_archive, source_date_epoch),
            _ar_member("data.tar.xz", data_archive, source_date_epoch),
        ]
    )
    output = Path(output).resolve()
    output.parent.mkdir(parents=True, exist_ok=True)
    descriptor, temporary_name = tempfile.mkstemp(prefix=f".{output.name}.", suffix=".tmp", dir=output.parent)
    try:
        with os.fdopen(descriptor, "wb") as stream:
            stream.write(package)
            stream.flush()
            os.fsync(stream.fileno())
        os.replace(temporary_name, output)
    except Exception:
        Path(temporary_name).unlink(missing_ok=True)
        raise
    return {
        "architecture": TARGET_ARCHITECTURES[target],
        "artifact": output.name,
        "schema": 1,
        "sha256": sha256_file(output),
        "size": output.stat().st_size,
        "target": target,
        "version": version,
    }


def _read_ar(path: Path) -> dict[str, bytes]:
    try:
        data = path.read_bytes()
    except OSError as error:
        raise ReleaseError(f"cannot read Debian package {path}: {error}") from error
    if not data.startswith(b"!<arch>\n"):
        raise ReleaseError(f"Debian package is not an ar archive: {path}")
    offset = 8
    members: dict[str, bytes] = {}
    while offset < len(data):
        header = data[offset : offset + 60]
        if len(header) != 60 or header[58:] != b"`\n":
            raise ReleaseError(f"invalid ar header in Debian package: {path}")
        try:
            name = header[:16].decode("ascii").rstrip().rstrip("/")
            size = int(header[48:58].decode("ascii").strip())
        except (UnicodeDecodeError, ValueError) as error:
            raise ReleaseError(f"invalid ar metadata in Debian package: {path}") from error
        offset += 60
        if size < 0 or offset + size > len(data) or name in members:
            raise ReleaseError(f"invalid ar member in Debian package: {path}")
        members[name] = data[offset : offset + size]
        offset += size + (size % 2)
    if list(members) != ["debian-binary", "control.tar.xz", "data.tar.xz"]:
        raise ReleaseError(f"unexpected Debian package members: {path}")
    return members


def _deb_control(path: Path) -> dict[str, str]:
    members = _read_ar(path)
    try:
        with tarfile.open(fileobj=io.BytesIO(members["control.tar.xz"]), mode="r:xz") as archive:
            stream = archive.extractfile("./control")
            if stream is None:
                raise ReleaseError("Debian package control file is missing")
            text = stream.read().decode("utf-8")
    except (tarfile.TarError, UnicodeDecodeError) as error:
        raise ReleaseError(f"invalid Debian control archive in {path}: {error}") from error
    fields: dict[str, str] = {}
    for line in text.splitlines():
        if not line or line.startswith(" "):
            continue
        key, separator, value = line.partition(":")
        if not separator:
            raise ReleaseError(f"invalid Debian control line in {path}")
        fields[key] = value.strip()
    return fields


def _installer_driver(target: str, version: str, packages: list[dict[str, object]]) -> bytes:
    architecture = TARGET_ARCHITECTURES[target]
    if target == "ubuntu-24.04-amd64":
        os_ids = "ubuntu"
        version_id = "24.04"
    else:
        os_ids = "raspbian debian"
        version_id = "13"
    checksums = "\n".join(f"{item['sha256']}  packages/{item['name']}" for item in packages)
    package_ids = " ".join(str(item["package"]) for item in packages)
    template = """#!/bin/sh
set -eu
expected_target='@TARGET@'
expected_arch='@ARCHITECTURE@'
expected_os_ids='@OS_IDS@'
expected_version_id='@VERSION_ID@'
version='@VERSION@'
package_ids='@PACKAGE_IDS@'
root=${NILO_INSTALL_ROOT:-/}
if [ "$(id -u)" -ne 0 ]; then echo 'installer must run as root' >&2; exit 2; fi
cd "$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
printf '%s\\n' '@CHECKSUMS@' | sha256sum -c - >/dev/null
if [ "$root" != / ]; then echo 'alternate roots are for isolated acceptance only' >&2; fi
for command in apt-get awk df dpkg dpkg-deb dpkg-query flock grep sed sha256sum systemctl; do
  command -v "$command" >/dev/null 2>&1 || { echo "required command is unavailable: $command" >&2; exit 2; }
done
mkdir -p "$root/run/lock"
exec 9>"$root/run/lock/nilo-installer.lock"
flock -n 9 || { echo 'another NILO installation is active' >&2; exit 2; }
arch=$(dpkg --print-architecture)
[ "$arch" = "$expected_arch" ] || { echo "architecture mismatch: $arch" >&2; exit 2; }
os_release="$root/etc/os-release"
[ -r "$os_release" ] || { echo 'cannot identify operating system' >&2; exit 2; }
ID= VERSION_ID=
. "$os_release"
case " $expected_os_ids " in *" $ID "*) ;; *) echo "operating system mismatch: $ID" >&2; exit 2 ;; esac
[ "$VERSION_ID" = "$expected_version_id" ] || { echo "release mismatch: $VERSION_ID" >&2; exit 2; }
[ ! -d /run/systemd/system ] || systemctl is-active NetworkManager.service >/dev/null || {
  echo 'NetworkManager must be active before NILO installation' >&2; exit 2;
}
[ ! -x /usr/libexec/nilo/nilo-installer-recover ] || /usr/libexec/nilo/nilo-installer-recover
dpkg --audit
nilo_package="packages/nilo_@VERSION@_@ARCHITECTURE@.deb"
apt_offline() {
  apt-get --no-download --no-install-recommends --no-remove \
    -o Dir::Etc::sourcelist=/dev/null -o Dir::Etc::sourceparts=- \
    -o Acquire::http::Proxy=false -o Acquire::https::Proxy=false \
    -o Acquire::ftp::Proxy=false "$@"
}
simulation=$(apt_offline --simulate --yes install "$nilo_package" packages/*.deb)
printf '%s\\n' "$simulation" | grep '^Remv ' >/dev/null && {
  echo 'offline dependency plan would remove an installed package' >&2; exit 2;
}
available_kib=$(df -Pk "$root/var/lib" | awk 'NR==2 {print $4}')
required_kib=$(du -sk packages | awk '{print $1}')
[ "$available_kib" -ge $((required_kib + 262144)) ] || { echo 'insufficient free space' >&2; exit 2; }
txid="install-$(date -u +%Y%m%dT%H%M%SZ)-$$"
txdir="$root/var/lib/nilo/transactions/$txid"
snapshot="$root/var/lib/nilo/snapshots/$txid"
mkdir -p "$txdir/prior-packages"
prior_debs="$txdir/prior-packages.list"
new_packages="$txdir/new-packages.list"
: >"$prior_debs"
: >"$new_packages"
for package_id in $package_ids; do
  installed=$(dpkg-query -W -f='${Status}' "$package_id" 2>/dev/null || true)
  if [ "$installed" = 'install ok installed' ]; then
    prior_version=$(dpkg-query -W -f='${Version}' "$package_id")
    case "$prior_version" in *[!A-Za-z0-9.+:~_-]*) echo "installed version is unsafe: $package_id" >&2; exit 2 ;; esac
    prior_arch=$(dpkg-query -W -f='${Architecture}' "$package_id")
    prior_deb="/var/cache/apt/archives/${package_id}_${prior_version}_${prior_arch}.deb"
    [ -f "$prior_deb" ] && [ ! -L "$prior_deb" ] || {
      echo "prior package is unavailable for rollback: $prior_deb" >&2; exit 2;
    }
    cp -- "$prior_deb" "$txdir/prior-packages/"
    printf '%s %s\\n' "$package_id" "$txdir/prior-packages/${prior_deb##*/}" >>"$prior_debs"
  else
    printf '%s\\n' "$package_id" >>"$new_packages"
  fi
done
phase() {
  value=$1
  printf '{"schema":2,"phase":"%s","target":"%s","version":"%s","prior_debs":"%s","new_packages":"%s","snapshot":"%s"}\\n' \
    "$value" "$expected_target" "$version" "$prior_debs" "$new_packages" "$snapshot" >"$txdir/transaction.json.tmp"
  mv -f "$txdir/transaction.json.tmp" "$txdir/transaction.json"
  sync "$txdir/transaction.json"
}
rollback() {
  trap - EXIT HUP INT TERM
  phase rolling_back
  ok=1
  if [ -f "$txdir/policy-created" ]; then
    rm -f /usr/sbin/policy-rc.d "$txdir/policy-created" || ok=0
  fi
  while read -r package_id prior_deb; do
    [ -n "$package_id" ] || continue
    dpkg -i "$prior_deb" >/dev/null 2>&1 || ok=0
  done <"$prior_debs"
  while read -r package_id; do
    [ -n "$package_id" ] || continue
    dpkg --purge "$package_id" >/dev/null 2>&1 || ok=0
  done <"$new_packages"
  if [ -d "$snapshot" ]; then
    /usr/bin/nilo maintenance restore --data-dir "$root/var/lib/nilo" --snapshot "$snapshot" \
      --failed-target-version "$version" >/dev/null 2>&1 || ok=0
  fi
  if [ -d /run/systemd/system ]; then
    systemctl daemon-reload >/dev/null 2>&1 || ok=0
    systemctl start nilo-nethelper.socket nilo.service >/dev/null 2>&1 || ok=0
  fi
  if [ "$ok" -ne 1 ]; then phase recovery-required; echo 'rollback failed; recovery is required' >&2; exit 1; fi
  phase rolled_back
  echo "installation rolled back; journal retained at $txdir" >&2
  exit 1
}
phase preflight
trap rollback EXIT HUP INT TERM
if [ -d /run/systemd/system ]; then
  systemctl stop nilo.service nilo-nethelper.service || true
  systemctl is-active nilo.service >/dev/null 2>&1 && { echo 'NILO did not quiesce' >&2; exit 1; }
fi
phase quiesced
if [ -f "$root/var/lib/nilo/nilo.db" ]; then
  /usr/bin/nilo maintenance snapshot --data-dir "$root/var/lib/nilo" --transaction "$txid" --target-version "$version"
fi
phase snapshotted
phase dependencies
policy_created=0
if [ ! -e /usr/sbin/policy-rc.d ]; then
  : >"$txdir/policy-created"
  printf '#!/bin/sh\\nexit 101\\n' >/usr/sbin/policy-rc.d
  chmod 0755 /usr/sbin/policy-rc.d
  policy_created=1
fi
apt_offline --yes install "$nilo_package" packages/*.deb
if [ "$policy_created" -eq 1 ]; then
  rm -f /usr/sbin/policy-rc.d "$txdir/policy-created"
fi
phase package
/usr/bin/nilo maintenance inventory --data-dir "$root/var/lib/nilo" >/dev/null
phase migrated
if [ -d /run/systemd/system ]; then
  systemctl daemon-reload
  systemctl enable nilo-nethelper.socket nilo-recovery.service nilo-firstboot.service nilo.service >/dev/null
  systemctl start nilo-nethelper.socket nilo-firstboot.service nilo.service
fi
phase started
/usr/bin/nilo version | grep -F "$version" >/dev/null
[ ! -d /run/systemd/system ] || systemctl is-active nilo-nethelper.socket nilo.service >/dev/null
[ ! -d /run/systemd/system ] || systemctl is-enabled nilo-recovery.service >/dev/null
phase verified
phase committed
trap - EXIT HUP INT TERM
exit 0
"""
    replacements = {
        "@ARCHITECTURE@": architecture,
        "@CHECKSUMS@": checksums,
        "@OS_IDS@": os_ids,
        "@PACKAGE_IDS@": package_ids,
        "@TARGET@": target,
        "@VERSION@": version,
        "@VERSION_ID@": version_id,
    }
    for marker, value in replacements.items():
        template = template.replace(marker, value)
    return template.encode("utf-8")


def _installer_launcher(archive_digest: str) -> bytes:
    return f"""#!/bin/sh
set -eu
expected_sha256='{archive_digest}'
payload_line=$(awk '/^__NILO_PAYLOAD_BELOW__$/ {{print NR + 1; exit}}' "$0")
[ -n "$payload_line" ] || {{ echo 'embedded payload marker is missing' >&2; exit 2; }}
temporary=$(mktemp -d "${{TMPDIR:-/tmp}}/nilo-installer.XXXXXX")
trap 'rm -rf -- "$temporary"' EXIT HUP INT TERM
archive="$temporary/payload.tar.xz"
tail -n +"$payload_line" "$0" >"$archive"
actual_sha256=$(sha256sum "$archive" | awk '{{print $1}}')
[ "$actual_sha256" = "$expected_sha256" ] || {{ echo 'embedded payload digest mismatch' >&2; exit 2; }}
if ! tar -tJf "$archive" | awk '
  $0 ~ "^/" {{bad=1}}
  {{n=split($0, part, "/"); for (i=1; i<=n; i++) if (part[i] == "..") bad=1}}
  END {{exit bad ? 1 : 0}}'; then
  echo 'archive member path is unsafe' >&2; exit 2
fi
if ! tar -tvJf "$archive" | awk 'substr($1,1,1) != "-" && substr($1,1,1) != "d" {{exit 1}}'; then
  echo 'archive contains a link or special file' >&2; exit 2
fi
mkdir "$temporary/root"
tar -xJf "$archive" -C "$temporary/root" --no-same-owner --no-same-permissions
if [ "${{1:-}}" = --inspect ]; then cat "$temporary/root/manifest.json"; exit 0; fi
exec sh "$temporary/root/install.sh" "$@"
__NILO_PAYLOAD_BELOW__
""".encode("utf-8")


def build_installer(
    package: Path,
    pool: Path,
    output: Path,
    *,
    target: str,
    version: str,
    source_date_epoch: int,
    offline_receipt: Path,
) -> dict[str, object]:
    """Build one target-specific, self-verifying offline installer."""
    if target not in TARGET_ARCHITECTURES:
        raise ReleaseError(f"unsupported target: {target}")
    _validate_offline_receipt(offline_receipt, target)
    package = Path(package).resolve()
    fields = _deb_control(package)
    if fields.get("Package") != "nilo":
        raise ReleaseError("installer package is not nilo")
    if fields.get("Architecture") != TARGET_ARCHITECTURES[target]:
        raise ReleaseError("Debian package architecture mismatch")
    if fields.get("Version") != version:
        raise ReleaseError("Debian package version mismatch")
    pool = Path(pool).resolve()
    if not pool.is_dir():
        raise ReleaseError(f"dependency pool is not a directory: {pool}")
    package_paths = [package]
    for candidate in sorted(pool.iterdir(), key=lambda item: item.name):
        details = candidate.lstat()
        if candidate.suffix != ".deb":
            raise ReleaseError(f"dependency pool contains a non-deb file: {candidate.name}")
        if DEBIAN_PACKAGE_NAME.fullmatch(candidate.name) is None:
            raise ReleaseError(f"unsafe Debian package name: {candidate.name!r}")
        if not stat.S_ISREG(details.st_mode) or stat.S_ISLNK(details.st_mode):
            raise ReleaseError(f"dependency must be a regular non-symlink file: {candidate.name}")
        package_paths.append(candidate)
    seen: set[str] = set()
    seen_packages: set[str] = set()
    package_records: list[dict[str, object]] = []
    entries: list[tuple[str, bytes, int]] = []
    for candidate in package_paths:
        if candidate.name in seen:
            raise ReleaseError(f"duplicate package name: {candidate.name}")
        seen.add(candidate.name)
        control = _deb_control(candidate)
        package_id = control.get("Package", "")
        package_version = control.get("Version", "")
        package_architecture = control.get("Architecture", "")
        if DEBIAN_BINARY_PACKAGE.fullmatch(package_id) is None:
            raise ReleaseError(f"invalid Debian package identity: {candidate.name}")
        if package_id in seen_packages:
            raise ReleaseError(f"duplicate Debian package identity: {package_id}")
        if DEBIAN_VERSION.fullmatch(package_version) is None:
            raise ReleaseError(f"invalid Debian package version: {candidate.name}")
        if package_architecture not in {TARGET_ARCHITECTURES[target], "all"}:
            raise ReleaseError(f"dependency architecture mismatch: {candidate.name}")
        seen_packages.add(package_id)
        contents = candidate.read_bytes()
        record = {
            "architecture": package_architecture,
            "name": candidate.name,
            "package": package_id,
            "sha256": sha256_bytes(contents),
            "size": len(contents),
            "version": package_version,
        }
        package_records.append(record)
        entries.append((f"packages/{candidate.name}", contents, 0o644))
    package_records.sort(key=lambda item: str(item["name"]))
    manifest = {
        "architecture": TARGET_ARCHITECTURES[target],
        "packages": package_records,
        "schema": 1,
        "target": target,
        "version": version,
    }
    entries.extend(
        [
            ("install.sh", _installer_driver(target, version, package_records), 0o755),
            ("manifest.json", canonical_json(manifest), 0o644),
        ]
    )
    archive = _archive_bytes(entries, source_date_epoch)
    launcher = _installer_launcher(sha256_bytes(archive))
    output = Path(output).resolve()
    output.parent.mkdir(parents=True, exist_ok=True)
    descriptor, temporary_name = tempfile.mkstemp(prefix=f".{output.name}.", suffix=".tmp", dir=output.parent)
    try:
        with os.fdopen(descriptor, "wb") as stream:
            stream.write(launcher)
            stream.write(archive)
            stream.flush()
            os.fsync(stream.fileno())
        os.chmod(temporary_name, 0o755)
        os.replace(temporary_name, output)
    except Exception:
        Path(temporary_name).unlink(missing_ok=True)
        raise
    return {
        "architecture": TARGET_ARCHITECTURES[target],
        "artifact": output.name,
        "schema": 1,
        "sha256": sha256_file(output),
        "size": output.stat().st_size,
        "target": target,
        "version": version,
    }


def _require_regular_input(path: Path, description: str) -> Path:
    path = Path(path)
    try:
        details = path.lstat()
    except OSError as error:
        raise ReleaseError(f"cannot inspect {description}: {error}") from error
    if not stat.S_ISREG(details.st_mode) or stat.S_ISLNK(details.st_mode):
        raise ReleaseError(f"{description} must be a regular non-symlink file")
    return path.resolve()


def build_pi_image(
    base_image: Path,
    package: Path,
    pool: Path,
    output: Path,
    *,
    target: str,
    version: str,
    source_date_epoch: int,
    admin_password_file: Path,
    offline_receipt: Path,
    runner=subprocess.run,
) -> dict[str, object]:
    """Validate image inputs and invoke the privileged, offline Pi builder."""
    if target != "rpi-debian13-arm64":
        raise ReleaseError("image construction requires the Raspberry Pi target")
    _validate_offline_receipt(offline_receipt, target)
    if not SEMVER.fullmatch(version):
        raise ReleaseError(f"invalid project version: {version}")
    base_image = _require_regular_input(base_image, "base image")
    package = _require_regular_input(package, "NILO package")
    password_file = _require_regular_input(admin_password_file, "administrator password file")
    if os.name != "nt" and password_file.stat().st_mode & 0o077:
        raise ReleaseError("administrator password file must not grant group or other permissions")
    fields = _deb_control(package)
    if fields.get("Package") != "nilo" or fields.get("Architecture") != "arm64":
        raise ReleaseError("Raspberry Pi image requires the arm64 NILO package")
    if fields.get("Version") != version:
        raise ReleaseError("Debian package version mismatch")
    try:
        with lzma.open(base_image, "rb") as stream:
            header = stream.read(512)
            if len(header) != 512 or header[510:512] != b"\x55\xaa":
                raise ReleaseError("base image lacks an MBR signature")
    except lzma.LZMAError as error:
        raise ReleaseError(f"base image is not valid xz data: {error}") from error
    pool = Path(pool).resolve()
    if not pool.is_dir() or pool.is_symlink():
        raise ReleaseError("image dependency pool must be a directory")
    for candidate in pool.iterdir():
        if candidate.suffix != ".deb":
            raise ReleaseError(f"image dependency pool contains a non-deb file: {candidate.name}")
        if DEBIAN_PACKAGE_NAME.fullmatch(candidate.name) is None:
            raise ReleaseError(f"unsafe Debian package name: {candidate.name!r}")
        _require_regular_input(candidate, f"image dependency {candidate.name}")
        dependency = _deb_control(candidate)
        if dependency.get("Architecture") not in {"arm64", "all"}:
            raise ReleaseError(f"dependency architecture mismatch: {candidate.name}")
    if not IMAGE_BUILDER.is_file():
        raise ReleaseError(f"Raspberry Pi image builder is missing: {IMAGE_BUILDER}")
    output = Path(output).resolve()
    output.parent.mkdir(parents=True, exist_ok=True)
    environment = os.environ.copy()
    environment["SOURCE_DATE_EPOCH"] = str(source_date_epoch)
    try:
        runner(
            ["sh", str(IMAGE_BUILDER), str(base_image), str(package), str(pool), str(password_file), str(output)],
            env=environment,
            check=True,
        )
    except subprocess.CalledProcessError as error:
        raise ReleaseError(f"Raspberry Pi image builder failed with exit {error.returncode}") from error
    output = _require_regular_input(output, "Raspberry Pi image output")
    try:
        with lzma.open(output, "rb") as stream:
            if not stream.read(1):
                raise ReleaseError("Raspberry Pi image output is empty")
    except lzma.LZMAError as error:
        raise ReleaseError(f"Raspberry Pi image output is not valid xz data: {error}") from error
    if not IMAGE_VERIFIER.is_file():
        raise ReleaseError(f"Raspberry Pi image verifier is missing: {IMAGE_VERIFIER}")
    try:
        runner(["sh", str(IMAGE_VERIFIER), str(output)], env=environment, check=True)
    except subprocess.CalledProcessError as error:
        raise ReleaseError(f"Raspberry Pi image structural verification failed with exit {error.returncode}") from error
    return {
        "architecture": "arm64",
        "artifact": output.name,
        "hardware_acceptance": "not_run",
        "schema": 1,
        "sha256": sha256_file(output),
        "size": output.stat().st_size,
        "structural_verification": "passed",
        "target": target,
        "version": version,
    }


def _read_json(path: Path, description: str) -> dict[str, Any]:
    try:
        value = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, UnicodeDecodeError, json.JSONDecodeError) as error:
        raise ReleaseError(f"cannot read {description} {path}: {error}") from error
    if not isinstance(value, dict) or value.get("schema") != 1:
        raise ReleaseError(f"{description} schema must be 1")
    return value


def _validate_offline_receipt(path: Path, target: str, *, lock_path: Path | None = None) -> dict[str, Any]:
    """Require the isolated, verified-cache admission for one target build."""
    path = _require_regular_input(path, "offline preparation receipt")
    receipt = _read_json(path, "offline preparation receipt")
    target_record = receipt.get("target")
    offline = receipt.get("offline")
    inputs = receipt.get("inputs")
    if not isinstance(target_record, dict) or target_record.get("id") != target or target_record.get("architecture") != TARGET_ARCHITECTURES[target]:
        raise ReleaseError("offline preparation receipt target mismatch")
    if not isinstance(inputs, list) or not inputs:
        raise ReleaseError("offline preparation receipt has no verified inputs")
    if not isinstance(offline, dict) or offline != {
        "interfaces": ["lo"],
        "ipv4_default_route": False,
        "ipv6_default_route": False,
        "platform": "linux",
        "verified": True,
    }:
        raise ReleaseError("offline preparation receipt does not prove network isolation")
    lock_digest = receipt.get("lock_sha256")
    if not isinstance(lock_digest, str) or re.fullmatch(r"[0-9a-f]{64}", lock_digest) is None:
        raise ReleaseError("offline preparation receipt lock digest is invalid")
    if lock_path is not None and sha256_file(lock_path) != lock_digest:
        raise ReleaseError("offline preparation receipt does not match the release input lock")
    return receipt


def _expected_artifacts(version: str) -> dict[str, str]:
    return {
        f"nilo_{version}_amd64.deb": "ubuntu-24.04-amd64",
        f"nilo-{version}-ubuntu-24.04-amd64.run": "ubuntu-24.04-amd64",
        f"nilo_{version}_arm64.deb": "rpi-debian13-arm64",
        f"nilo-{version}-rpi-debian13-arm64.run": "rpi-debian13-arm64",
        f"nilo-pi4-{version}.img.xz": "rpi-debian13-arm64",
    }


def assemble_release(
    artifact_dir: Path,
    output: Path,
    *,
    version: str,
    source_date_epoch: int,
    lock_path: Path,
    targets_path: Path,
    source_revision: str,
    acceptance_path: Path | None = None,
    source_offers_path: Path | None = None,
    offline_receipts: list[Path] | None = None,
) -> dict[str, object]:
    """Close artifacts and their provenance into one canonical release tree."""
    if not SEMVER.fullmatch(version):
        raise ReleaseError(f"invalid project version: {version}")
    if not source_revision.strip() or any(character.isspace() for character in source_revision):
        raise ReleaseError("source revision must be one non-empty token")
    lock_path = _require_regular_input(lock_path, "input lock")
    lock = _read_json(lock_path, "input lock")
    if lock.get("complete") is not True or lock.get("incomplete_reasons"):
        raise ReleaseError("input lock is incomplete")
    inputs = lock.get("inputs")
    if not isinstance(inputs, list) or not inputs:
        raise ReleaseError("complete input lock has no inputs")
    required_input_fields = (
        "architecture", "artifact_type", "license", "name", "provenance",
        "redistribution", "scope", "sha256", "size", "url", "version",
    )
    for item in inputs:
        if not isinstance(item, dict) or any(item.get(field) in {None, ""} for field in required_input_fields):
            raise ReleaseError("input lock entry lacks notice/provenance metadata")
        if item["redistribution"] != "permitted" or not re.fullmatch(r"[0-9a-f]{64}", str(item["sha256"])):
            raise ReleaseError("input lock entry lacks redistribution or digest authority")
    observed_inputs = {(str(item["scope"]), str(item["name"])) for item in inputs}
    required_inputs = {(scope, name) for scope, names in REQUIRED_RELEASE_INPUTS.items() for name in names}
    if observed_inputs != required_inputs:
        raise ReleaseError("input lock does not contain the exact required release input classes")
    if not offline_receipts or len(offline_receipts) != len(TARGET_ARCHITECTURES):
        raise ReleaseError("one offline preparation receipt per target is required")
    receipt_targets: set[str] = set()
    for receipt_path in offline_receipts:
        receipt_preview = _read_json(_require_regular_input(receipt_path, "offline preparation receipt"), "offline preparation receipt")
        receipt_target = receipt_preview.get("target", {}).get("id") if isinstance(receipt_preview.get("target"), dict) else None
        if receipt_target not in TARGET_ARCHITECTURES or receipt_target in receipt_targets:
            raise ReleaseError("offline preparation receipt set has a duplicate or unknown target")
        _validate_offline_receipt(receipt_path, str(receipt_target), lock_path=lock_path)
        receipt_targets.add(str(receipt_target))
    if receipt_targets != set(TARGET_ARCHITECTURES):
        raise ReleaseError("offline preparation receipt set is incomplete")
    targets_path = _require_regular_input(targets_path, "target registry")
    targets = _read_json(targets_path, "target registry")
    target_entries = targets.get("targets")
    if not isinstance(target_entries, list):
        raise ReleaseError("target registry has no targets")
    hardware: dict[str, list[str]] = {}
    for target in target_entries:
        if not isinstance(target, dict) or target.get("id") not in TARGET_ARCHITECTURES:
            raise ReleaseError("target registry contains an unsupported target")
        checks = target.get("hardware_acceptance")
        if not isinstance(checks, list) or not all(isinstance(check, str) and check for check in checks):
            raise ReleaseError("target registry hardware acceptance is invalid")
        hardware[str(target["id"])] = checks

    if acceptance_path is None:
        raise ReleaseError("executed software acceptance evidence is required")
    acceptance_path = _require_regular_input(acceptance_path, "software acceptance evidence")
    acceptance = _read_json(acceptance_path, "software acceptance evidence")
    acceptance_checks = acceptance.get("checks")
    if not isinstance(acceptance_checks, list):
        raise ReleaseError("software acceptance evidence has no checks")
    observed: set[tuple[str, str]] = set()
    acceptance_files: list[tuple[Path, Path]] = []
    closed_checks: list[dict[str, object]] = []
    for check in acceptance_checks:
        if not isinstance(check, dict):
            raise ReleaseError("software acceptance evidence contains an invalid check")
        target_id = check.get("target")
        name = check.get("check")
        kind = check.get("kind")
        status = check.get("status")
        key = (str(target_id), str(name))
        if target_id not in TARGET_ARCHITECTURES or key in observed:
            raise ReleaseError("software acceptance evidence contains a duplicate or unknown target")
        observed.add(key)
        if kind == "software":
            if set(check) != {"check", "evidence", "kind", "status", "target"} or status != "passed":
                raise ReleaseError(f"software acceptance check did not pass with evidence: {target_id}/{name}")
            evidence = check.get("evidence")
            if not isinstance(evidence, dict) or set(evidence) != {"path", "sha256"}:
                raise ReleaseError("software acceptance evidence reference is invalid")
            relative = PurePosixPath(str(evidence.get("path", "")))
            if relative.is_absolute() or not relative.parts or ".." in relative.parts:
                raise ReleaseError("software acceptance evidence path is unsafe")
            source = _require_regular_input(
                acceptance_path.parent.joinpath(*relative.parts),
                f"software acceptance evidence {target_id}/{name}",
            )
            if sha256_file(source) != evidence.get("sha256"):
                raise ReleaseError(f"software acceptance evidence digest mismatch: {target_id}/{name}")
            destination = Path("evidence") / "acceptance" / str(target_id) / f"{name}.json"
            acceptance_files.append((destination, source))
            closed_checks.append(
                {
                    "check": name,
                    "evidence": {"path": destination.as_posix(), "sha256": evidence["sha256"]},
                    "kind": kind,
                    "status": status,
                    "target": target_id,
                }
            )
        elif kind == "physical":
            if set(check) != {"check", "kind", "reason", "status", "target"} or status != "not_run" or not isinstance(check.get("reason"), str) or not check["reason"]:
                raise ReleaseError("physical acceptance lacks a not-run reason")
            closed_checks.append(dict(check))
        if kind not in {"software", "physical"}:
            raise ReleaseError("software acceptance evidence contains an invalid check kind")
    required_software = {
        (target_id, check)
        for target_id in TARGET_ARCHITECTURES
        for check in SOFTWARE_ACCEPTANCE_CHECKS
    }
    required_software.add(("rpi-debian13-arm64", "image-structure"))
    observed_software = {
        (str(check.get("target")), str(check.get("check")))
        for check in acceptance_checks
        if isinstance(check, dict) and check.get("kind") == "software"
    }
    if observed_software != required_software:
        missing = sorted(required_software - observed_software)
        extra = sorted(observed_software - required_software)
        raise ReleaseError(f"software acceptance evidence mismatch; missing={missing}, extra={extra}")
    required_physical = {
        (target_id, check)
        for target_id, checks in hardware.items()
        for check in checks
    }
    observed_physical = {
        (str(check.get("target")), str(check.get("check")))
        for check in acceptance_checks
        if isinstance(check, dict) and check.get("kind") == "physical"
    }
    if observed_physical != required_physical:
        raise ReleaseError("physical acceptance evidence does not match the target registry")

    if source_offers_path is None:
        raise ReleaseError("corresponding-source offer directory is required")
    source_offers_path = Path(source_offers_path).resolve()
    source_entries = list(_iter_tree(source_offers_path))
    source_files = [relative for _, details, relative in source_entries if stat.S_ISREG(details.st_mode)]
    if "SOURCE_OFFER.txt" not in source_files or not any(path.startswith("LICENSES/") for path in source_files):
        raise ReleaseError("source offer must contain SOURCE_OFFER.txt and LICENSES content")
    scan_release_tree(source_offers_path)
    if not OPERATOR_GUIDE.is_file():
        raise ReleaseError("operator guide is missing")

    artifact_dir = Path(artifact_dir).resolve()
    if not artifact_dir.is_dir() or artifact_dir.is_symlink():
        raise ReleaseError("artifact input must be a regular directory")
    expected = _expected_artifacts(version)
    discovered = {path.name: path for path in artifact_dir.iterdir()}
    if set(discovered) != set(expected):
        missing = sorted(set(expected) - set(discovered))
        extra = sorted(set(discovered) - set(expected))
        raise ReleaseError(f"artifact set mismatch; missing={missing}, extra={extra}")
    for path in discovered.values():
        _require_regular_input(path, f"release artifact {path.name}")
    scan_release_tree(artifact_dir)

    output = Path(output).resolve()
    if output.exists():
        raise ReleaseError(f"release output already exists: {output}")
    output.parent.mkdir(parents=True, exist_ok=True)
    temporary = Path(tempfile.mkdtemp(prefix=f".{output.name}.", dir=output.parent))
    try:
        artifact_records: list[dict[str, object]] = []
        for name, target in sorted(expected.items(), key=lambda item: (item[1], item[0])):
            relative = Path(target) / name
            destination = temporary / relative
            destination.parent.mkdir(parents=True, exist_ok=True)
            shutil.copyfile(discovered[name], destination)
            architecture = TARGET_ARCHITECTURES[target]
            media_type = (
                "application/vnd.debian.binary-package" if name.endswith(".deb")
                else "application/x-xz" if name.endswith(".img.xz")
                else "application/x-nilo-installer"
            )
            artifact_records.append(
                {
                    "architecture": architecture,
                    "media_type": media_type,
                    "path": relative.as_posix(),
                    "sha256": sha256_file(destination),
                    "size": destination.stat().st_size,
                    "target": target,
                    "version": version,
                }
            )

        provenance = {
            "build": {
                "network": "disabled-after-acquisition",
                "release_builder_sha256": sha256_file(Path(__file__)),
                "source_date_epoch": source_date_epoch,
                "source_revision": source_revision,
            },
            "input_lock": {"path": "inputs.lock.json", "sha256": sha256_file(lock_path)},
            "target_registry": {"path": "targets.json", "sha256": sha256_file(targets_path)},
            "acceptance_source_sha256": sha256_file(acceptance_path),
            "artifacts": artifact_records,
            "schema": 1,
            "targets": [
                {"architecture": TARGET_ARCHITECTURES[target], "id": target}
                for target in sorted(TARGET_ARCHITECTURES)
            ],
            "version": version,
        }
        sbom = {
            "SPDXID": "SPDXRef-DOCUMENT",
            "creationInfo": {
                "created": datetime.datetime.fromtimestamp(
                    source_date_epoch, datetime.timezone.utc
                ).strftime("%Y-%m-%dT%H:%M:%SZ"),
                "creators": ["Tool: NILO release_builder.py"],
            },
            "dataLicense": "CC0-1.0",
            "documentNamespace": f"https://nilo.local/releases/{version}/{source_revision}",
            "files": [
                {
                    "SPDXID": f"SPDXRef-Artifact-{index}",
                    "checksums": [{"algorithm": "SHA256", "checksumValue": item["sha256"]}],
                    "fileName": item["path"],
                }
                for index, item in enumerate(artifact_records, start=1)
            ],
            "name": f"NILO-{version}",
            "packages": [
                {
                    "SPDXID": f"SPDXRef-Input-{index}",
                    "checksums": [{"algorithm": "SHA256", "checksumValue": item["sha256"]}],
                    "downloadLocation": item["url"],
                    "licenseConcluded": item["license"],
                    "name": item["name"],
                    "versionInfo": item["version"],
                }
                for index, item in enumerate(sorted(inputs, key=lambda value: (str(value.get("scope")), str(value.get("name")))), start=1)
            ],
            "spdxVersion": "SPDX-2.3",
        }
        notice_lines = ["NILO third-party notices", ""]
        for item in sorted(inputs, key=lambda value: (str(value.get("name")), str(value.get("version")))):
            required = ("name", "version", "license", "url", "sha256", "provenance", "redistribution")
            if not isinstance(item, dict) or any(not item.get(field) for field in required):
                raise ReleaseError("input lock entry lacks notice/provenance metadata")
            notice_lines.append(
                f"{item['name']} {item['version']} | {item['license']} | {item['url']} | provenance:{item['provenance']} | redistribution:{item['redistribution']} | sha256:{item['sha256']}"
            )
        notice_lines.append("")
        closed_acceptance = {
            "checks": sorted(closed_checks, key=lambda check: (str(check["target"]), str(check["kind"]), str(check["check"]))),
            "schema": 1,
        }
        evidence = {
            "THIRD_PARTY_NOTICES/inputs.txt": ("\n".join(notice_lines)).encode("utf-8"),
            "OPERATOR_GUIDE.md": OPERATOR_GUIDE.read_bytes(),
            "acceptance.json": canonical_json(closed_acceptance),
            "inputs.lock.json": lock_path.read_bytes(),
            "provenance.json": canonical_json(provenance),
            "sbom.spdx.json": canonical_json(sbom),
            "targets.json": targets_path.read_bytes(),
        }
        for name, contents in evidence.items():
            destination = temporary / name
            destination.parent.mkdir(parents=True, exist_ok=True)
            destination.write_bytes(contents)

        copied_records: list[dict[str, object]] = []
        for relative, source in sorted(acceptance_files, key=lambda item: item[0].as_posix()):
            destination = temporary / relative
            destination.parent.mkdir(parents=True, exist_ok=True)
            shutil.copyfile(source, destination)
            copied_records.append(
                {"path": relative.as_posix(), "sha256": sha256_file(destination), "size": destination.stat().st_size, "target": "evidence"}
            )
        for source, details, relative_name in source_entries:
            if not stat.S_ISREG(details.st_mode):
                continue
            relative = Path("source-offer") / relative_name
            destination = temporary / relative
            destination.parent.mkdir(parents=True, exist_ok=True)
            shutil.copyfile(source, destination)
            copied_records.append(
                {"path": relative.as_posix(), "sha256": sha256_file(destination), "size": destination.stat().st_size, "target": "release"}
            )

        checksum_records = artifact_records + copied_records + [
            {
                "path": name,
                "sha256": sha256_bytes(contents),
                "size": len(contents),
                "target": "release",
            }
            for name, contents in sorted(evidence.items())
        ]
        checksum_records.sort(key=lambda item: str(item["path"]))
        checksums = "".join(f"{item['sha256']}  {item['path']}\n" for item in checksum_records).encode("utf-8")
        (temporary / "SHA256SUMS").write_bytes(checksums)
        manifest = {
            "artifacts": artifact_records,
            "checksum_file": {"path": "SHA256SUMS", "sha256": sha256_bytes(checksums)},
            "distribution": {"internal_unsigned": True, "public_distribution": "blocked"},
            "files": checksum_records,
            "schema": 1,
            "source_date_epoch": source_date_epoch,
            "version": version,
        }
        (temporary / "manifest.json").write_bytes(canonical_json(manifest))
        os.replace(temporary, output)
    except Exception:
        shutil.rmtree(temporary, ignore_errors=True)
        raise
    return verify_release(output)


def verify_release(root: Path) -> dict[str, object]:
    """Verify the semantic digest closure of one assembled release tree."""
    root = Path(root).resolve()
    manifest = _read_json(root / "manifest.json", "release manifest")
    records = manifest.get("files")
    if not isinstance(records, list) or not records:
        raise ReleaseError("release manifest has no files")
    verified: list[dict[str, object]] = []
    for record in records:
        if not isinstance(record, dict) or not isinstance(record.get("path"), str):
            raise ReleaseError("release manifest file record is invalid")
        pure = PurePosixPath(record["path"])
        if pure.is_absolute() or ".." in pure.parts:
            raise ReleaseError(f"unsafe release manifest path: {record['path']}")
        path = root.joinpath(*pure.parts)
        path = _require_regular_input(path, f"release file {record['path']}")
        actual = sha256_file(path)
        if actual != record.get("sha256") or path.stat().st_size != record.get("size"):
            raise ReleaseError(f"release digest mismatch: {record['path']}")
        verified.append(record)
    checksum = manifest.get("checksum_file")
    if not isinstance(checksum, dict) or checksum.get("path") != "SHA256SUMS":
        raise ReleaseError("release checksum-file record is invalid")
    if sha256_file(root / "SHA256SUMS") != checksum.get("sha256"):
        raise ReleaseError("release digest mismatch: SHA256SUMS")
    expected_lines = [f"{item['sha256']}  {item['path']}" for item in records]
    actual_lines = (root / "SHA256SUMS").read_text(encoding="utf-8").splitlines()
    if actual_lines != expected_lines:
        raise ReleaseError("SHA256SUMS does not match the release manifest")
    acceptance = _read_json(root / "acceptance.json", "acceptance index")
    for check in acceptance.get("checks", []):
        if check.get("kind") == "physical" and check.get("status") != "not_run":
            raise ReleaseError("physical acceptance lacks independently supplied evidence")
        if check.get("kind") == "software":
            evidence = check.get("evidence")
            if check.get("status") != "passed" or not isinstance(evidence, dict):
                raise ReleaseError("software acceptance lacks retained evidence")
            evidence_relative = PurePosixPath(str(evidence.get("path", "")))
            if evidence_relative.is_absolute() or not evidence_relative.parts or ".." in evidence_relative.parts:
                raise ReleaseError("retained acceptance evidence path is unsafe")
            evidence_path = root.joinpath(*evidence_relative.parts)
            if sha256_file(_require_regular_input(evidence_path, "retained acceptance evidence")) != evidence.get("sha256"):
                raise ReleaseError("retained acceptance evidence digest mismatch")
    artifacts = [item for item in verified if item.get("target") in TARGET_ARCHITECTURES]
    expected_paths = {"manifest.json", "SHA256SUMS", *(str(item["path"]) for item in records)}
    actual_paths = {
        path.relative_to(root).as_posix()
        for path in root.rglob("*")
        if path.is_file()
    }
    if actual_paths != expected_paths:
        raise ReleaseError(
            f"release tree has undeclared files: {sorted(actual_paths - expected_paths)}"
        )
    return {"artifacts": artifacts, "files_verified": len(verified), "schema": 1, "status": "pass"}


def _write_receipt(path: Path | None, value: object) -> None:
    if path is None:
        return
    path.parent.mkdir(parents=True, exist_ok=True)
    temporary = path.with_name(f".{path.name}.tmp")
    temporary.write_bytes(canonical_json(value))
    os.replace(temporary, path)


def _parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description=__doc__)
    subcommands = parser.add_subparsers(dest="command", required=True)
    payload = subcommands.add_parser("payload", help="build a deterministic target payload")
    payload.add_argument("--source", type=Path, required=True)
    payload.add_argument("--output", type=Path, required=True)
    payload.add_argument("--target", choices=sorted(TARGET_ARCHITECTURES), required=True)
    payload.add_argument("--version", required=True)
    payload.add_argument("--source-date-epoch", type=int, required=True)
    payload.add_argument("--offline-receipt", type=Path, required=True)
    payload.add_argument("--receipt", type=Path)
    deb = subcommands.add_parser("deb", help="build a deterministic Debian package")
    deb.add_argument("--payload", type=Path, required=True)
    deb.add_argument("--output", type=Path, required=True)
    deb.add_argument("--target", choices=sorted(TARGET_ARCHITECTURES), required=True)
    deb.add_argument("--version", required=True)
    deb.add_argument("--source-date-epoch", type=int, required=True)
    deb.add_argument("--offline-receipt", type=Path, required=True)
    deb.add_argument("--receipt", type=Path)
    installer = subcommands.add_parser("installer", help="build a self-extracting offline installer")
    installer.add_argument("--package", type=Path, required=True)
    installer.add_argument("--pool", type=Path, required=True)
    installer.add_argument("--output", type=Path, required=True)
    installer.add_argument("--target", choices=sorted(TARGET_ARCHITECTURES), required=True)
    installer.add_argument("--version", required=True)
    installer.add_argument("--source-date-epoch", type=int, required=True)
    installer.add_argument("--offline-receipt", type=Path, required=True)
    installer.add_argument("--receipt", type=Path)
    image = subcommands.add_parser("image", help="build a sanitized Raspberry Pi image")
    image.add_argument("--base-image", type=Path, required=True)
    image.add_argument("--package", type=Path, required=True)
    image.add_argument("--pool", type=Path, required=True)
    image.add_argument("--admin-password-file", type=Path, required=True)
    image.add_argument("--output", type=Path, required=True)
    image.add_argument("--target", choices=sorted(TARGET_ARCHITECTURES), required=True)
    image.add_argument("--version", required=True)
    image.add_argument("--source-date-epoch", type=int, required=True)
    image.add_argument("--offline-receipt", type=Path, required=True)
    image.add_argument("--receipt", type=Path)
    release = subcommands.add_parser("release", help="assemble canonical release evidence")
    release.add_argument("--artifacts", type=Path, required=True)
    release.add_argument("--output", type=Path, required=True)
    release.add_argument("--version", required=True)
    release.add_argument("--source-date-epoch", type=int, required=True)
    release.add_argument("--lock", type=Path, required=True)
    release.add_argument("--targets", type=Path, required=True)
    release.add_argument("--source-revision", required=True)
    release.add_argument("--acceptance", type=Path, required=True)
    release.add_argument("--source-offers", type=Path, required=True)
    release.add_argument("--offline-receipt", type=Path, required=True, action="append")
    release.add_argument("--receipt", type=Path)
    verify = subcommands.add_parser("verify", help="verify a closed release tree")
    verify.add_argument("--root", type=Path, required=True)
    verify.add_argument("--receipt", type=Path)
    scan = subcommands.add_parser("scan", help="scan a staged production tree")
    scan.add_argument("--root", type=Path, required=True)
    scan.add_argument("--policy", type=Path, default=DEFAULT_POLICY)
    scan.add_argument("--receipt", type=Path)
    return parser


def main(arguments: list[str] | None = None) -> int:
    parser = _parser()
    options = parser.parse_args(arguments)
    try:
        if options.command == "payload":
            result = build_payload(
                options.source,
                options.output,
                target=options.target,
                version=options.version,
                source_date_epoch=options.source_date_epoch,
                offline_receipt=options.offline_receipt,
            )
        elif options.command == "deb":
            result = build_deb(
                options.payload,
                options.output,
                target=options.target,
                version=options.version,
                source_date_epoch=options.source_date_epoch,
                offline_receipt=options.offline_receipt,
            )
        elif options.command == "installer":
            result = build_installer(
                options.package,
                options.pool,
                options.output,
                target=options.target,
                version=options.version,
                source_date_epoch=options.source_date_epoch,
                offline_receipt=options.offline_receipt,
            )
        elif options.command == "image":
            result = build_pi_image(
                options.base_image,
                options.package,
                options.pool,
                options.output,
                target=options.target,
                version=options.version,
                source_date_epoch=options.source_date_epoch,
                admin_password_file=options.admin_password_file,
                offline_receipt=options.offline_receipt,
            )
        elif options.command == "release":
            result = assemble_release(
                options.artifacts,
                options.output,
                version=options.version,
                source_date_epoch=options.source_date_epoch,
                lock_path=options.lock,
                targets_path=options.targets,
                source_revision=options.source_revision,
                acceptance_path=options.acceptance,
                source_offers_path=options.source_offers,
                offline_receipts=options.offline_receipt,
            )
        elif options.command == "verify":
            result = verify_release(options.root)
        else:
            result = scan_release_tree(options.root, options.policy)
        _write_receipt(options.receipt, result)
        print(json.dumps(result, sort_keys=True))
        return 0
    except (OSError, ReleaseError, tarfile.TarError) as error:
        print(f"release builder refused: {error}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
