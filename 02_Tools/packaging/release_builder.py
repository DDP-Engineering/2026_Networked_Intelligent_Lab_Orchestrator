#!/usr/bin/env python3
# -*- coding: utf-8 -*-
# -----------------------------------------------------------------------------
# Copyright (c) 2026 DDP Engineering
# SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
# -----------------------------------------------------------------------------
"""@file release_builder.py
@brief Build and verify deterministic offline NILO delivery artifacts.
"""

# IMPORTS

from __future__ import annotations

import argparse
import datetime
import hashlib
import importlib.util
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
from typing import Any, Callable, Iterable


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
ACCEPTANCE_RUNNER = Path(__file__).parents[1] / "tests" / "packaging" / "release-acceptance.sh"
ACCEPTANCE_LIFECYCLE_RUNNER = Path(__file__).parents[1] / "tests" / "packaging" / "target-lifecycle-check.sh"
TARGET_ACCEPTANCE_HARNESS = Path(__file__).parents[1] / "tests" / "packaging" / "run-target-acceptance.sh"
BUILD_CANDIDATE_SET = Path(__file__).with_name("build") / "build-candidate-set.sh"
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
    "installer-upgrade",
    "installer-clean-rollback",
    "installer-rollback-dependencies",
    "installer-rollback-package",
    "installer-rollback-migrated",
    "installer-rollback-started",
    "installer-rollback-verified",
    "installer-interruption-preflight",
    "installer-interruption-quiesced",
    "installer-interruption-snapshotted",
    "installer-interruption-dependencies",
    "installer-interruption-package",
    "installer-interruption-migrated",
    "installer-interruption-started",
    "installer-interruption-verified",
    "installer-interruption-committed",
    "installer-interruption-rolling_back",
    "installer-interruption-rolled_back",
    "installer-interruption-recovery-required",
    "removal-purge",
    "core-status-gui-helper-catalogue",
    "integration-install-upgrade-health",
    "checksums",
    "offline-boundary",
)
LIFECYCLE_ACCEPTANCE_CHECKS = {
    "package-install",
    "package-reinstall",
    "installer-clean-install",
    "installer-upgrade",
    "installer-clean-rollback",
    "installer-rollback-dependencies",
    "installer-rollback-package",
    "installer-rollback-migrated",
    "installer-rollback-started",
    "installer-rollback-verified",
    "installer-interruption-preflight",
    "installer-interruption-quiesced",
    "installer-interruption-snapshotted",
    "installer-interruption-dependencies",
    "installer-interruption-package",
    "installer-interruption-migrated",
    "installer-interruption-started",
    "installer-interruption-verified",
    "installer-interruption-committed",
    "installer-interruption-rolling_back",
    "installer-interruption-rolled_back",
    "installer-interruption-recovery-required",
    "removal-purge",
    "core-status-gui-helper-catalogue",
    "integration-install-upgrade-health",
}
PRIOR_INSTALLER_CHECKS = {
    "installer-upgrade",
    "installer-rollback-dependencies",
    "installer-rollback-package",
    "installer-rollback-migrated",
    "installer-rollback-started",
    "installer-rollback-verified",
    "installer-interruption-preflight",
    "installer-interruption-quiesced",
    "installer-interruption-snapshotted",
    "installer-interruption-dependencies",
    "installer-interruption-package",
    "installer-interruption-migrated",
    "installer-interruption-started",
    "installer-interruption-verified",
    "installer-interruption-committed",
    "installer-interruption-rolling_back",
    "installer-interruption-rolled_back",
    "installer-interruption-recovery-required",
}
REQUIRED_RELEASE_INPUTS = {
    "shared": {"go-module-cache", "gui-npm-cache", "source-offers"},
    "rpi-debian13-arm64": {"base-image", "os-package-pool", "sigrok-runtime", "camera-build-input"},
    "ubuntu-24.04-amd64": {"os-package-pool", "sigrok-runtime", "camera-build-input"},
}


class ReleaseError(ValueError):
    """Report a release-policy or artifact-integrity refusal."""


def _validate_release_input_contract(lock_path: Path, targets_path: Path) -> tuple[Any, Any]:
    """Reuse the canonical target/lock parser instead of a weaker release schema."""
    module_path = Path(__file__).with_name("package_inputs.py")
    specification = importlib.util.spec_from_file_location("nilo_release_input_contract", module_path)
    if specification is None or specification.loader is None:
        raise ReleaseError("cannot load canonical dependency provenance contract")
    module = importlib.util.module_from_spec(specification)
    sys.modules[specification.name] = module
    try:
        specification.loader.exec_module(module)
        registry = module.load_targets(targets_path)
        lock = module.load_lock(lock_path, registry, require_complete=True)
    except Exception as error:
        raise ReleaseError(f"dependency provenance contract is invalid: {error}") from error
    return module, lock


def _revalidate_signed_dependency_pools(
    module: Any,
    lock: Any,
    receipts: dict[str, dict[str, Any]],
    signature_verifier: Callable[[Path, Path, list[str]], str] | None,
) -> None:
    """Re-run signed repository and dependency closure validation at release time."""
    verifier = signature_verifier or module._default_apt_signature_verifier
    for item in lock.inputs:
        if item.artifact_type != "debian-pool-tar":
            continue
        receipt = receipts.get(item.scope)
        if receipt is None:
            raise ReleaseError(f"dependency provenance receipt is missing for {item.scope}")
        record = next(
            (entry for entry in receipt["inputs"] if entry.get("scope") == item.scope and entry.get("name") == item.name),
            None,
        )
        if record is None:
            raise ReleaseError(f"dependency provenance object is missing for {item.scope}")
        cache_root = Path(receipt["cache_root"])
        object_path = cache_root.joinpath(*PurePosixPath(record["cache_object"]).parts)
        try:
            observed = module._validate_resolved_artifact(
                object_path,
                item,
                apt_policy=item.apt,
                signature_verifier=verifier,
            )
        except Exception as error:
            raise ReleaseError(f"dependency provenance revalidation failed for {item.scope}: {error}") from error
        if observed != item.apt:
            raise ReleaseError(f"dependency provenance changed for {item.scope}")


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


def _acceptance_runner_digest() -> str:
    closure = [
        {"name": path.name, "sha256": sha256_file(_require_regular_input(path, "repository-owned acceptance runner"))}
        for path in (ACCEPTANCE_RUNNER, ACCEPTANCE_LIFECYCLE_RUNNER, TARGET_ACCEPTANCE_HARNESS)
    ]
    return sha256_bytes(canonical_json(closure))


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
    input_lock: Path,
) -> dict[str, object]:
    """Create one canonical target payload and return its receipt."""
    if target not in TARGET_ARCHITECTURES:
        raise ReleaseError(f"unsupported target: {target}")
    if not SEMVER.fullmatch(version):
        raise ReleaseError(f"invalid project version: {version}")
    if source_date_epoch < 0:
        raise ReleaseError("SOURCE_DATE_EPOCH must not be negative")
    _validate_offline_receipt(offline_receipt, target, lock_path=input_lock)
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
Depends: adduser, curl, dpkg-repack, e2fsprogs, parted, systemd, util-linux
Installed-Size: {installed_size}
Description: Networked Intelligent Lab Orchestrator
 NILO coordinates bench integrations through a local appliance service.
""".encode("utf-8")
    postinst = b"""#!/bin/sh
set -e
if ! getent group nilo >/dev/null 2>&1; then addgroup --system nilo; fi
if ! id nilo >/dev/null 2>&1; then adduser --system --ingroup nilo --home /var/lib/nilo --no-create-home nilo; fi
if ! getent group video >/dev/null 2>&1; then addgroup --system video; fi
adduser nilo video
install -d -o nilo -g nilo -m 0750 /var/lib/nilo /var/log/nilo /run/nilo
install -d -o root -g root -m 0700 /var/lib/nilo-installer /var/lib/nilo-installer/transactions
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
  deb-systemd-invoke stop nilo.service nilo-firstboot.service nilo-recovery.service >/dev/null || true
  /usr/bin/nilo maintenance remove --data-dir /var/lib/nilo --nethelper-socket /run/nilo/nethelper.sock
  deb-systemd-invoke stop nilo-nethelper.service nilo-nethelper.socket >/dev/null || true
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
    input_lock: Path,
) -> dict[str, object]:
    """Build one deterministic architecture-specific Debian binary package."""
    if target not in TARGET_ARCHITECTURES:
        raise ReleaseError(f"unsupported target: {target}")
    if not SEMVER.fullmatch(version):
        raise ReleaseError(f"invalid project version: {version}")
    _validate_offline_receipt(offline_receipt, target, lock_path=input_lock)
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
    firstboot_helper = DEBIAN_ROOT / "nilo-firstboot"
    data_entries.append(
        ("./usr/libexec/nilo/nilo-firstboot", firstboot_helper.read_bytes(), 0o755)
    )
    image_firstboot = DEBIAN_ROOT / "nilo-image-firstboot"
    data_entries.append(
        ("./usr/libexec/nilo/nilo-image-firstboot", image_firstboot.read_bytes(), 0o755)
    )
    if "./etc/nilo/firstboot.env" not in known_names:
        data_entries.append(("./etc/nilo/firstboot.env", b"NILO_FIRST_ADMIN_USERNAME=admin\n", 0o640))
    package_manifest = canonical_json(
        {
            "files": [
                {
                    "mode": f"{mode:04o}",
                    "path": name.removeprefix("./"),
                    "sha256": sha256_bytes(contents),
                    "size": len(contents),
                }
                for name, contents, mode in sorted(data_entries, key=lambda item: item[0])
            ],
            "schema": 1,
            "target": target,
            "version": version,
        }
    )
    package_manifest_name = "./usr/share/nilo/package-manifest.json"
    if package_manifest_name in {name for name, _, _ in data_entries}:
        raise ReleaseError(f"payload duplicates package manifest: {package_manifest_name}")
    data_entries.append((package_manifest_name, package_manifest, 0o644))
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


def _verify_deb_structure(path: Path, target: str, version: str) -> None:
    """Authenticate generated Debian metadata and its complete data inventory."""
    members = _read_ar(path)
    if members["debian-binary"] != b"2.0\n":
        raise ReleaseError(f"Debian package format marker is invalid: {path.name}")
    try:
        with tarfile.open(fileobj=io.BytesIO(members["control.tar.xz"]), mode="r:xz") as archive:
            control_members = archive.getmembers()
            control_entries = {
                item.name: (archive.extractfile(item).read(), stat.S_IMODE(item.mode))
                for item in control_members
                if item.isfile() and archive.extractfile(item) is not None
            }
        with tarfile.open(fileobj=io.BytesIO(members["data.tar.xz"]), mode="r:xz") as archive:
            data_members = archive.getmembers()
            if any(not item.isfile() for item in data_members):
                raise ReleaseError(f"Debian data archive contains non-file members: {path.name}")
            data_entries = {
                item.name: (archive.extractfile(item).read(), stat.S_IMODE(item.mode))
                for item in data_members
                if archive.extractfile(item) is not None
            }
    except (tarfile.TarError, lzma.LZMAError, KeyError) as error:
        raise ReleaseError(f"Debian package archive is invalid: {path.name}: {error}") from error
    if len(control_entries) != len(control_members) or len(data_entries) != len(data_members):
        raise ReleaseError(f"Debian package contains duplicate or unreadable members: {path.name}")
    if list(control_entries) != sorted(control_entries) or list(data_entries) != sorted(data_entries):
        raise ReleaseError(f"Debian package members are not canonical: {path.name}")

    fields = _deb_control(path)
    try:
        installed_size = int(fields["Installed-Size"])
    except (KeyError, ValueError) as error:
        raise ReleaseError(f"Debian package installed size is invalid: {path.name}") from error
    calculated_size = (sum(len(contents) for contents, _ in data_entries.values()) + 1023) // 1024
    if installed_size != calculated_size:
        raise ReleaseError(f"Debian package installed size does not match its data: {path.name}")
    expected_control = {
        name: (contents, mode)
        for name, contents, mode in _maintainer_entries(
            version, TARGET_ARCHITECTURES[target], installed_size
        )
    }
    if control_entries != expected_control:
        raise ReleaseError(f"Debian maintainer metadata is not repository-generated: {path.name}")

    manifest_name = "./usr/share/nilo/package-manifest.json"
    encoded_manifest = data_entries.get(manifest_name)
    if encoded_manifest is None or encoded_manifest[1] != 0o644:
        raise ReleaseError(f"Debian package manifest is missing: {path.name}")
    try:
        package_manifest = json.loads(encoded_manifest[0])
    except (UnicodeDecodeError, json.JSONDecodeError) as error:
        raise ReleaseError(f"Debian package manifest is invalid: {path.name}") from error
    if (
        set(package_manifest) != {"files", "schema", "target", "version"}
        or package_manifest.get("schema") != 1
        or package_manifest.get("target") != target
        or package_manifest.get("version") != version
    ):
        raise ReleaseError(f"Debian package manifest identity is invalid: {path.name}")
    observed = [
        {
            "mode": f"{mode:04o}",
            "path": name.removeprefix("./"),
            "sha256": sha256_bytes(contents),
            "size": len(contents),
        }
        for name, (contents, mode) in data_entries.items()
        if name != manifest_name
    ]
    if package_manifest.get("files") != observed:
        raise ReleaseError(f"Debian package manifest does not bind its data archive: {path.name}")

    repository_files = {
        **{
            f"./usr/lib/systemd/system/{unit.name}": (unit.read_bytes(), 0o644)
            for unit in sorted((DEBIAN_ROOT / "units").iterdir(), key=lambda item: item.name)
            if unit.is_file() and unit.suffix in {".service", ".socket", ".timer"}
        },
        "./usr/libexec/nilo/nilo-installer-recover": (
            (DEBIAN_ROOT / "nilo-installer-recover").read_bytes(), 0o755
        ),
        "./usr/libexec/nilo/nilo-firstboot": (
            (DEBIAN_ROOT / "nilo-firstboot").read_bytes(), 0o755
        ),
        "./usr/libexec/nilo/nilo-image-firstboot": (
            (DEBIAN_ROOT / "nilo-image-firstboot").read_bytes(), 0o755
        ),
    }
    for name, expected in repository_files.items():
        if data_entries.get(name) != expected:
            raise ReleaseError(f"Debian package-owned file is not repository-generated: {name}")


def _verify_installer_structure(
    path: Path, target: str, version: str
) -> list[dict[str, object]]:
    contents = path.read_bytes()
    marker = b"__NILO_PAYLOAD_BELOW__\n"
    if marker not in contents:
        raise ReleaseError(f"installer structure is invalid: {path.name}")
    launcher, archive_bytes = contents.rsplit(marker, 1)
    matched = re.search(rb"expected_sha256='([0-9a-f]{64})'", launcher)
    if matched is None or matched.group(1).decode("ascii") != sha256_bytes(archive_bytes):
        raise ReleaseError(f"installer embedded archive digest mismatch: {path.name}")
    try:
        with tarfile.open(fileobj=io.BytesIO(archive_bytes), mode="r:xz") as archive:
            members = archive.getmembers()
            names = [member.name for member in members]
            if len(names) != len(set(names)):
                raise ReleaseError(f"installer archive has duplicate members: {path.name}")
            if names != sorted(names) or any(not member.isfile() for member in members):
                raise ReleaseError(f"installer archive structure is invalid: {path.name}")
            manifest_stream = archive.extractfile("manifest.json")
            if manifest_stream is None:
                raise ReleaseError(f"installer manifest is missing: {path.name}")
            manifest = json.load(manifest_stream)
            if (
                manifest.get("schema") != 1
                or manifest.get("target") != target
                or manifest.get("version") != version
                or manifest.get("architecture") != TARGET_ARCHITECTURES[target]
            ):
                raise ReleaseError(f"installer manifest identity mismatch: {path.name}")
            package_records = manifest.get("packages")
            if not isinstance(package_records, list) or not package_records:
                raise ReleaseError(f"installer package manifest is invalid: {path.name}")
            declared: dict[str, dict[str, object]] = {}
            identities: set[str] = set()
            for item in package_records:
                if (
                    not isinstance(item, dict)
                    or set(item) != {"architecture", "name", "package", "sha256", "size", "version"}
                    or not isinstance(item.get("name"), str)
                    or DEBIAN_PACKAGE_NAME.fullmatch(str(item["name"])) is None
                    or DEBIAN_BINARY_PACKAGE.fullmatch(str(item.get("package", ""))) is None
                    or item.get("architecture") not in {TARGET_ARCHITECTURES[target], "all"}
                    or DEBIAN_VERSION.fullmatch(str(item.get("version", ""))) is None
                    or re.fullmatch(r"[0-9a-f]{64}", str(item.get("sha256", ""))) is None
                    or type(item.get("size")) is not int
                    or int(item["size"]) < 1
                ):
                    raise ReleaseError(f"installer package manifest is invalid: {path.name}")
                member_name = f"packages/{item['name']}"
                package_id = str(item["package"])
                if member_name in declared or package_id in identities:
                    raise ReleaseError(f"installer package manifest has duplicate identities: {path.name}")
                declared[member_name] = item
                identities.add(package_id)
            actual = {name for name in names if name.startswith("packages/")}
            if actual != set(declared):
                raise ReleaseError(f"installer package manifest mismatch: {path.name}")
            if set(names) != {"install.sh", "manifest.json", *actual}:
                raise ReleaseError(f"installer archive has unexpected members: {path.name}")
            for name in actual:
                stream = archive.extractfile(name)
                if stream is None:
                    raise ReleaseError(f"installer package is unreadable: {name}")
                package_bytes = stream.read()
                record = declared[name]
                if len(package_bytes) != record.get("size") or sha256_bytes(package_bytes) != record.get("sha256"):
                    raise ReleaseError(f"installer package digest mismatch: {name}")
            driver_stream = archive.extractfile("install.sh")
            if driver_stream is None or driver_stream.read() != _installer_driver(
                target, version, [dict(item) for item in package_records]
            ):
                raise ReleaseError(f"installer driver is not the repository-generated implementation: {path.name}")
            expected_files: list[tuple[str, str, int]] = []
            manifest_bytes = b""
            for member in members:
                stream = archive.extractfile(member)
                if stream is None:
                    raise ReleaseError(f"installer member is unreadable: {member.name}")
                member_bytes = stream.read()
                expected_files.append((member.name, sha256_bytes(member_bytes), len(member_bytes)))
                if member.name == "manifest.json":
                    manifest_bytes = member_bytes
            expected_launcher = _installer_launcher(
                sha256_bytes(archive_bytes),
                sha256_bytes(manifest_bytes),
                names,
                expected_files,
            )
            if contents != expected_launcher + archive_bytes:
                raise ReleaseError(f"installer launcher is not the repository-generated implementation: {path.name}")
            return [dict(item) for item in package_records]
    except (tarfile.TarError, lzma.LZMAError, json.JSONDecodeError, KeyError, TypeError) as error:
        raise ReleaseError(f"installer structure is invalid: {path.name}: {error}") from error


def _verify_release_artifact_structure(
    path: Path, target: str, version: str
) -> list[dict[str, object]] | None:
    if path.name.endswith(".deb"):
        _verify_deb_structure(path, target, version)
        fields = _deb_control(path)
        if (
            fields.get("Package") != "nilo"
            or fields.get("Version") != version
            or fields.get("Architecture") != TARGET_ARCHITECTURES[target]
        ):
            raise ReleaseError(f"Debian artifact identity mismatch: {path.name}")
        return None
    if path.name.endswith(".run"):
        return _verify_installer_structure(path, target, version)
    if path.name.endswith(".img.xz"):
        try:
            with lzma.open(path, "rb") as stream:
                header = stream.read(512)
                if len(header) != 512 or header[510:512] != b"\x55\xaa":
                    raise ReleaseError(f"image structure lacks an MBR signature: {path.name}")
        except lzma.LZMAError as error:
            raise ReleaseError(f"image structure is invalid: {path.name}: {error}") from error
        return None
    raise ReleaseError(f"release artifact type is unsupported: {path.name}")


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
    dependency_paths = " ".join(
        f"packages/{item['name']}" for item in packages if item["package"] != "nilo"
    )
    repack_packages = [
        f"packages/{item['name']}" for item in packages if item["package"] == "dpkg-repack"
    ]
    repack_package = repack_packages[0] if len(repack_packages) == 1 else ""
    template = """#!/bin/sh
set -eu
export LC_ALL=C
expected_target='@TARGET@'
expected_arch='@ARCHITECTURE@'
expected_os_ids='@OS_IDS@'
expected_version_id='@VERSION_ID@'
version='@VERSION@'
package_ids='@PACKAGE_IDS@'
dependency_paths='@DEPENDENCY_PATHS@'
repack_package='@REPACK_PACKAGE@'
root=${NILO_INSTALL_ROOT:-/}
if [ "$(id -u)" -ne 0 ]; then echo 'installer must run as root' >&2; exit 2; fi
umask 077
cd "$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
printf '%s\\n' '@CHECKSUMS@' | sha256sum -c - >/dev/null
if [ "$root" != / ]; then echo 'alternate roots are for isolated acceptance only' >&2; fi
for command in apt-get awk curl df dpkg dpkg-deb dpkg-query find flock grep sed sha256sum sort sync systemctl; do
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
simulation=$(apt_offline --simulate --reinstall --yes install "$nilo_package" packages/*.deb)
printf '%s\\n' "$simulation" | grep '^Remv ' >/dev/null && {
  echo 'offline dependency plan would remove an installed package' >&2; exit 2;
}
changed_packages_preview=$(printf '%s\\n' "$simulation" | awk '$1 == "Inst" {print $2}' | sort -u)
for changed_package in $changed_packages_preview; do
  case " $package_ids " in *" $changed_package "*) ;; *)
    echo "offline dependency plan changes an undeclared package: $changed_package" >&2; exit 2 ;;
  esac
done
available_kib=$(df -Pk "$root/var/lib" | awk 'NR==2 {print $4}')
required_kib=$(du -sk packages | awk '{print $1}')
[ "$available_kib" -ge $((required_kib + 262144)) ] || { echo 'insufficient free space' >&2; exit 2; }
[ -n "$repack_package" ] && [ -f "$repack_package" ] || {
  echo 'installer dependency pool has no private dpkg-repack tool' >&2; exit 2;
}
had_prior_data_dir=0
if [ -d "$root/var/lib/nilo" ]; then
  had_prior_data_dir=1
  [ -f "$root/var/lib/nilo/nilo.db" ] || {
    echo 'existing NILO data directory has no authoritative database' >&2; exit 2;
  }
fi
txid="install-$(date -u +%Y%m%dT%H%M%SZ)-$$"
install -d -o 0 -g 0 -m 0700 "$root/var/lib/nilo-installer"
install -d -o 0 -g 0 -m 0700 "$root/var/lib/nilo-installer/transactions"
txdir="$root/var/lib/nilo-installer/transactions/$txid"
snapshot="$root/var/lib/nilo/snapshots/$txid"
preflight_cleanup() {
  rm -rf -- "$txdir"
  rmdir "$root/var/lib/nilo-installer/transactions" "$root/var/lib/nilo-installer" 2>/dev/null || true
}
trap preflight_cleanup EXIT HUP INT TERM
mkdir -p "$txdir/prior-packages"
chmod 0700 "$txdir" "$txdir/prior-packages"
sync "$root/var/lib/nilo-installer/transactions"
mkdir "$txdir/repack-tool"
dpkg-deb -x "$repack_package" "$txdir/repack-tool"
repack_command=$(find "$txdir/repack-tool" -type f -name dpkg-repack -print)
set -- $repack_command
[ "$#" -eq 1 ] && [ -x "$1" ] || { echo 'private dpkg-repack tool is invalid' >&2; exit 2; }
repack_command=$1
prior_debs="$txdir/prior-packages.list"
new_packages="$txdir/new-packages.list"
changed_packages="$txdir/changed-packages.list"
: >"$prior_debs"
: >"$new_packages"
: >"$changed_packages"
printf '%s\\n' "$changed_packages_preview" | sed '/^$/d' >"$changed_packages"
had_prior_nilo=0
for package_id in $changed_packages_preview; do
  installed=$(dpkg-query -W -f='${Status}' "$package_id" 2>/dev/null || true)
  if [ "$installed" = 'install ok installed' ]; then
    prior_version=$(dpkg-query -W -f='${Version}' "$package_id")
    case "$prior_version" in *[!A-Za-z0-9.+:~_-]*) echo "installed version is unsafe: $package_id" >&2; exit 2 ;; esac
    before=$(find "$txdir/prior-packages" -maxdepth 1 -type f -name '*.deb' | wc -l)
    (cd "$txdir/prior-packages" && "$repack_command" "$package_id" >/dev/null)
    after=$(find "$txdir/prior-packages" -maxdepth 1 -type f -name '*.deb' | wc -l)
    [ "$after" -eq $((before + 1)) ] || { echo "could not retain prior package: $package_id" >&2; exit 2; }
    prior_deb=$(find "$txdir/prior-packages" -maxdepth 1 -type f -name "${package_id}_*.deb" | sort | tail -n 1)
    [ -n "$prior_deb" ] && [ ! -L "$prior_deb" ] || { echo "prior package is unavailable for rollback: $package_id" >&2; exit 2; }
    prior_digest=$(sha256sum "$prior_deb" | awk '{print $1}')
    sync "$prior_deb"
    printf '%s %s %s %s\\n' "$package_id" "$prior_version" "$prior_digest" "$prior_deb" >>"$prior_debs"
    [ "$package_id" != nilo ] || had_prior_nilo=1
  else
    printf '%s\\n' "$package_id" >>"$new_packages"
  fi
done
sync "$prior_debs" "$new_packages" "$changed_packages" "$txdir/prior-packages" "$txdir"
prior_debs_sha256=$(sha256sum "$prior_debs" | awk '{print $1}')
new_packages_sha256=$(sha256sum "$new_packages" | awk '{print $1}')
changed_packages_sha256=$(sha256sum "$changed_packages" | awk '{print $1}')
phase() {
  value=$1
  printf '{"schema":4,"phase":"%s","target":"%s","version":"%s","had_prior_nilo":%s,"had_prior_data_dir":%s,"prior_debs":"%s","prior_debs_sha256":"%s","new_packages":"%s","new_packages_sha256":"%s","changed_packages":"%s","changed_packages_sha256":"%s","snapshot":"%s"}\\n' \
    "$value" "$expected_target" "$version" "$had_prior_nilo" "$had_prior_data_dir" "$prior_debs" "$prior_debs_sha256" \
    "$new_packages" "$new_packages_sha256" "$changed_packages" "$changed_packages_sha256" "$snapshot" >"$txdir/transaction.json.tmp"
  sync "$txdir/transaction.json.tmp"
  mv -f "$txdir/transaction.json.tmp" "$txdir/transaction.json"
  sync "$txdir/transaction.json" "$txdir"
  if [ "${NILO_ACCEPTANCE_INTERRUPT_PHASE:-}" = "$value" ]; then
    kill -KILL $$
  fi
  if [ "${NILO_ACCEPTANCE_FAIL_PHASE:-}" = "$value" ]; then
    echo "acceptance fault injected after phase: $value" >&2
    return 97
  fi
}
rollback() {
  trap - EXIT HUP INT TERM
  phase rolling_back
  ok=1
  if [ -d /run/systemd/system ]; then
    systemctl stop nilo.service nilo-firstboot.service nilo-nethelper.service nilo-nethelper.socket >/dev/null 2>&1 || ok=0
    systemctl is-active nilo.service nilo-nethelper.service >/dev/null 2>&1 && ok=0
  fi
  if [ -f "$txdir/policy-created" ]; then
    rm -f /usr/sbin/policy-rc.d "$txdir/policy-created" || ok=0
  fi
  set --
  while read -r package_id prior_version prior_digest prior_deb; do
    [ -n "$package_id" ] || continue
    [ "$(sha256sum "$prior_deb" | awk '{print $1}')" = "$prior_digest" ] || { ok=0; continue; }
    set -- "$@" "$prior_deb"
  done <"$prior_debs"
  [ "$#" -eq 0 ] || dpkg -i "$@" >/dev/null 2>&1 || ok=0
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
    [ "$had_prior_nilo" -ne 1 ] || systemctl start nilo-nethelper.socket nilo.service >/dev/null 2>&1 || ok=0
  fi
  [ "${NILO_ACCEPTANCE_FORCE_RECOVERY_REQUIRED:-}" != 1 ] || ok=0
  if [ "$ok" -eq 1 ] && [ "$had_prior_data_dir" -eq 0 ]; then
    rm -rf -- "$root/var/lib/nilo" || ok=0
    sync "$root/var/lib" || ok=0
  fi
  if [ "$ok" -ne 1 ]; then
    [ ! -d /run/systemd/system ] || systemctl stop nilo.service nilo-nethelper.service nilo-nethelper.socket >/dev/null 2>&1 || true
    phase recovery-required
    echo 'rollback failed; services are stopped and recovery is required' >&2
    exit 1
  fi
  phase rolled_back
  completed="$root/var/lib/nilo-installer/completed"
  install -d -o 0 -g 0 -m 0700 "$completed"
  [ -d "$completed" ] && [ ! -L "$completed" ] || {{ echo 'unsafe completed transaction root' >&2; exit 1; }}
  completed_txdir="$completed/${txdir##*/}"
  [ ! -e "$completed_txdir" ] || {{ echo 'completed transaction identity collision' >&2; exit 1; }}
  mv "$txdir" "$completed_txdir"
  sync "$completed_txdir" "$completed" "$root/var/lib/nilo-installer/transactions" "$root/var/lib/nilo-installer"
  if [ "$had_prior_data_dir" -eq 0 ]; then
    echo 'clean installation rolled back to absent application state' >&2
  else
    echo "installation rolled back; journal retained at $completed_txdir" >&2
  fi
  exit 1
}
phase preflight
trap rollback EXIT HUP INT TERM
if [ -d /run/systemd/system ]; then
  systemctl stop nilo.service nilo-nethelper.service nilo-nethelper.socket || true
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
case "${NILO_ACCEPTANCE_FAIL_OPERATION:-}" in
  dependencies)
    fault_dependency=$(printf '%s\\n' $dependency_paths | sed -n '1p')
    [ -n "$fault_dependency" ] || { echo 'no dependency package is available for fault injection' >&2; exit 2; }
    dpkg --unpack "$fault_dependency"
    echo 'acceptance fault injected inside dependency installation' >&2
    exit 97
    ;;
  package)
    [ -z "$dependency_paths" ] || apt_offline --reinstall --yes install $dependency_paths
    dpkg --unpack "$nilo_package"
    echo 'acceptance fault injected inside NILO package installation' >&2
    exit 97
    ;;
  '') apt_offline --reinstall --yes install "$nilo_package" packages/*.deb ;;
  *) echo 'unsupported installer operation fault' >&2; exit 2 ;;
esac
if [ "$policy_created" -eq 1 ]; then
  rm -f /usr/sbin/policy-rc.d "$txdir/policy-created"
fi
phase package
/usr/bin/nilo maintenance migrate --data-dir "$root/var/lib/nilo" >/dev/null
phase migrated
if [ -d /run/systemd/system ]; then
  systemctl daemon-reload
  systemctl enable nilo-nethelper.socket nilo-recovery.service nilo-firstboot.service nilo.service >/dev/null
  systemctl start nilo-nethelper.socket nilo-firstboot.service nilo.service
fi
phase started
/usr/bin/nilo version | grep -F "$version" >/dev/null
[ ! -d /run/systemd/system ] || systemctl is-active nilo-nethelper.socket nilo-nethelper.service nilo.service >/dev/null
[ ! -d /run/systemd/system ] || systemctl is-enabled nilo-recovery.service >/dev/null
[ ! -d /run/systemd/system ] || [ -S /run/nilo/nethelper.sock ]
[ ! -d /run/systemd/system ] || curl --fail --silent --show-error --max-time 10 http://127.0.0.1:8091/ | grep -F "$version" >/dev/null
[ -s /usr/share/nilo/gui/index.html ]
catalogue_found=0
for manifest in /usr/lib/nilo/integration-bundles/*/manifest.json; do
  [ ! -f "$manifest" ] || catalogue_found=1
done
[ "$catalogue_found" -eq 1 ]
phase verified
phase committed
trap - EXIT HUP INT TERM
exit 0
"""
    replacements = {
        "@ARCHITECTURE@": architecture,
        "@CHECKSUMS@": checksums,
        "@DEPENDENCY_PATHS@": dependency_paths,
        "@OS_IDS@": os_ids,
        "@PACKAGE_IDS@": package_ids,
        "@REPACK_PACKAGE@": repack_package,
        "@TARGET@": target,
        "@VERSION@": version,
        "@VERSION_ID@": version_id,
    }
    for marker, value in replacements.items():
        template = template.replace(marker, value)
    return template.encode("utf-8")


def _installer_launcher(
    archive_digest: str,
    manifest_digest: str = "0" * 64,
    expected_members: list[str] | None = None,
    expected_files: list[tuple[str, str, int]] | None = None,
) -> bytes:
    member_lines = "\n".join(expected_members or [])
    file_lines = "\n".join(
        f"{digest} {size} {name}" for name, digest, size in (expected_files or [])
    )
    return f"""#!/bin/sh
set -eu
expected_sha256='{archive_digest}'
expected_manifest_sha256='{manifest_digest}'
payload_line=$(awk '/^__NILO_PAYLOAD_BELOW__$/ {{print NR + 1; exit}}' "$0")
[ -n "$payload_line" ] || {{ echo 'embedded payload marker is missing' >&2; exit 2; }}
temporary=$(mktemp -d "${{TMPDIR:-/tmp}}/nilo-installer.XXXXXX")
trap 'rm -rf -- "$temporary"' EXIT HUP INT TERM
archive="$temporary/payload.tar.xz"
tail -n +"$payload_line" "$0" >"$archive"
actual_sha256=$(sha256sum "$archive" | awk '{{print $1}}')
[ "$actual_sha256" = "$expected_sha256" ] || {{ echo 'embedded payload digest mismatch' >&2; exit 2; }}
members="$temporary/members"
tar -tJf "$archive" >"$members"
if awk 'seen[$0]++ {{duplicate=1}} END {{exit duplicate ? 0 : 1}}' "$members"; then
  echo 'archive member is duplicated' >&2; exit 2
fi
if ! awk '
  $0 ~ "^/" {{bad=1}}
  {{n=split($0, part, "/"); for (i=1; i<=n; i++) if (part[i] == "..") bad=1}}
  END {{exit bad ? 1 : 0}}' "$members"; then
  echo 'archive member path is unsafe' >&2; exit 2
fi
expected_members="$temporary/expected-members"
cat >"$expected_members" <<'NILO_EXPECTED_MEMBERS'
{member_lines}
NILO_EXPECTED_MEMBERS
cmp -s "$members" "$expected_members" || {{ echo 'archive inventory mismatch' >&2; exit 2; }}
manifest="$temporary/manifest.json"
tar -xOJf "$archive" manifest.json >"$manifest"
[ "$(sha256sum "$manifest" | awk '{{print $1}}')" = "$expected_manifest_sha256" ] || {{
  echo 'installer manifest digest mismatch' >&2; exit 2;
}}
if ! tar -tvJf "$archive" | awk 'substr($1,1,1) != "-" && substr($1,1,1) != "d" {{exit 1}}'; then
  echo 'archive contains a link or special file' >&2; exit 2
fi
mkdir "$temporary/root"
tar -xJf "$archive" -C "$temporary/root" --no-same-owner --no-same-permissions
while read -r expected_digest expected_size relative; do
  [ -n "$relative" ] || continue
  candidate="$temporary/root/$relative"
  [ -f "$candidate" ] && [ ! -L "$candidate" ] || {{ echo 'installer payload file is unsafe' >&2; exit 2; }}
  [ "$(wc -c <"$candidate" | tr -d ' ')" = "$expected_size" ] || {{ echo 'installer payload file size mismatch' >&2; exit 2; }}
  [ "$(sha256sum "$candidate" | awk '{{print $1}}')" = "$expected_digest" ] || {{ echo 'installer payload file digest mismatch' >&2; exit 2; }}
done <<'NILO_EXPECTED_FILES'
{file_lines}
NILO_EXPECTED_FILES
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
    input_lock: Path,
) -> dict[str, object]:
    """Build one target-specific, self-verifying offline installer."""
    if target not in TARGET_ARCHITECTURES:
        raise ReleaseError(f"unsupported target: {target}")
    receipt = _validate_offline_receipt(offline_receipt, target, lock_path=input_lock)
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
        _require_admitted_input(candidate, receipt, f"installer dependency {candidate.name}")
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
    manifest_bytes = canonical_json(manifest)
    entries.extend(
        [
            ("install.sh", _installer_driver(target, version, package_records), 0o755),
            ("manifest.json", manifest_bytes, 0o644),
        ]
    )
    archive = _archive_bytes(entries, source_date_epoch)
    sorted_entries = sorted(entries, key=lambda item: item[0])
    launcher = _installer_launcher(
        sha256_bytes(archive),
        sha256_bytes(manifest_bytes),
        [name for name, _, _ in sorted_entries],
        [(name, sha256_bytes(contents), len(contents)) for name, contents, _ in sorted_entries],
    )
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
    input_lock: Path,
    runner=subprocess.run,
) -> dict[str, object]:
    """Validate image inputs and invoke the privileged, offline Pi builder."""
    if target != "rpi-debian13-arm64":
        raise ReleaseError("image construction requires the Raspberry Pi target")
    receipt = _validate_offline_receipt(offline_receipt, target, lock_path=input_lock)
    if not SEMVER.fullmatch(version):
        raise ReleaseError(f"invalid project version: {version}")
    base_image = _require_regular_input(base_image, "base image")
    _require_admitted_input(base_image, receipt, "Raspberry Pi base image")
    package = _require_regular_input(package, "NILO package")
    password_file = _require_regular_input(admin_password_file, "administrator password file")
    if os.name != "nt" and password_file.stat().st_mode & 0o077:
        raise ReleaseError("administrator password file must not grant group or other permissions")
    fields = _deb_control(package)
    if fields.get("Package") != "nilo" or fields.get("Architecture") != "arm64":
        raise ReleaseError("Raspberry Pi image requires the arm64 NILO package")
    if fields.get("Version") != version:
        raise ReleaseError("Debian package version mismatch")
    _verify_deb_structure(package, target, version)
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
        _require_admitted_input(candidate, receipt, f"image dependency {candidate.name}")
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


def _read_json(path: Path, description: str, *, expected_schema: int = 1) -> dict[str, Any]:
    try:
        value = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, UnicodeDecodeError, json.JSONDecodeError) as error:
        raise ReleaseError(f"cannot read {description} {path}: {error}") from error
    if not isinstance(value, dict) or value.get("schema") != expected_schema:
        raise ReleaseError(f"{description} schema must be {expected_schema}")
    return value


def _validate_offline_receipt(path: Path, target: str, *, lock_path: Path | None = None) -> dict[str, Any]:
    """Require the isolated, verified-cache admission for one target build."""
    if lock_path is None:
        raise ReleaseError("artifact construction requires an explicit trusted input lock")
    path = _require_regular_input(path, "offline preparation receipt")
    receipt = _read_json(path, "offline preparation receipt")
    target_record = receipt.get("target")
    offline = receipt.get("offline")
    inputs = receipt.get("inputs")
    cache_root_value = receipt.get("cache_root")
    if not isinstance(target_record, dict) or target_record.get("id") != target or target_record.get("architecture") != TARGET_ARCHITECTURES[target]:
        raise ReleaseError("offline preparation receipt target mismatch")
    if not isinstance(inputs, list) or not inputs:
        raise ReleaseError("offline preparation receipt has no verified inputs")
    if not isinstance(cache_root_value, str) or not Path(cache_root_value).is_absolute():
        raise ReleaseError("offline preparation receipt cache root is invalid")
    cache_root = Path(cache_root_value)
    try:
        cache_details = cache_root.lstat()
    except OSError as error:
        raise ReleaseError(f"cannot inspect offline cache root: {error}") from error
    if not stat.S_ISDIR(cache_details.st_mode) or stat.S_ISLNK(cache_details.st_mode):
        raise ReleaseError("offline cache root must be a regular non-symlink directory")
    expected_input_fields = {
        "apt", "architecture", "artifact_type", "cache_object", "license", "name",
        "provenance", "redistribution", "scope", "sha256", "size", "version",
    }
    for index, item in enumerate(inputs):
        if not isinstance(item, dict) or set(item) != expected_input_fields:
            raise ReleaseError(f"offline preparation input {index} has invalid fields")
        digest = item.get("sha256")
        size = item.get("size")
        cache_object = item.get("cache_object")
        if not isinstance(digest, str) or re.fullmatch(r"[0-9a-f]{64}", digest) is None:
            raise ReleaseError(f"offline preparation input {index} digest is invalid")
        if not isinstance(size, int) or isinstance(size, bool) or size < 0:
            raise ReleaseError(f"offline preparation input {index} size is invalid")
        expected_relative = PurePosixPath("objects", "sha256", digest[:2], digest)
        if cache_object != expected_relative.as_posix():
            raise ReleaseError(f"offline preparation input {index} cache path is invalid")
        object_path = _require_regular_input(
            cache_root.joinpath(*expected_relative.parts),
            f"offline cache object {item.get('name', index)}",
        )
        try:
            object_path.relative_to(cache_root.resolve())
        except ValueError as error:
            raise ReleaseError("offline cache object escapes its cache root") from error
        if object_path.stat().st_size != size:
            raise ReleaseError(f"offline cache object size mismatch: {item.get('name', index)}")
        if sha256_file(object_path) != digest:
            raise ReleaseError(f"offline cache object digest mismatch: {item.get('name', index)}")
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
    lock_path = _require_regular_input(lock_path, "trusted input lock")
    if sha256_file(lock_path) != lock_digest:
        raise ReleaseError("offline preparation receipt does not match the release input lock")
    lock = _read_json(lock_path, "release input lock")
    if lock.get("complete") is not True or not isinstance(lock.get("inputs"), list):
        raise ReleaseError("release input lock is incomplete")
    expected = [
        {key: value for key, value in item.items() if key != "url"}
        for item in lock["inputs"]
        if isinstance(item, dict) and item.get("scope") in {"shared", target}
    ]
    observed = [
        {key: value for key, value in item.items() if key != "cache_object"}
        for item in inputs
    ]
    sort_key = lambda item: (str(item.get("scope")), str(item.get("name")))
    if sorted(observed, key=sort_key) != sorted(expected, key=sort_key):
        raise ReleaseError("offline preparation receipt does not match the release input lock")
    return receipt


def _require_admitted_input(path: Path, receipt: dict[str, Any], description: str) -> None:
    """Bind an external build input to bytes revalidated from the offline cache."""
    path = _require_regular_input(path, description)
    digest = sha256_file(path)
    size = path.stat().st_size
    admitted = []
    for item in receipt["inputs"]:
        admitted.append(item)
        apt = item.get("apt")
        if isinstance(apt, dict) and isinstance(apt.get("packages"), list):
            admitted.extend(package for package in apt["packages"] if isinstance(package, dict))
    if not any(item.get("sha256") == digest and item.get("size") == size for item in admitted):
        raise ReleaseError(f"{description} is not admitted by the offline receipt")


def _admitted_cache_object(
    inputs: list[dict[str, Any]],
    receipts: dict[str, dict[str, Any]],
    *,
    scope: str,
    name: str,
) -> Path:
    """Resolve one lock entry to bytes revalidated by an offline receipt."""
    locked = [item for item in inputs if item.get("scope") == scope and item.get("name") == name]
    if len(locked) != 1:
        raise ReleaseError(f"release input is not unique: {scope}/{name}")
    target_receipts = receipts.values() if scope == "shared" else [receipts[scope]]
    for receipt in target_receipts:
        matches = [
            item for item in receipt["inputs"]
            if item.get("scope") == scope and item.get("name") == name
        ]
        if len(matches) != 1:
            continue
        record = matches[0]
        if record.get("sha256") != locked[0]["sha256"] or record.get("size") != locked[0]["size"]:
            continue
        cache_root = Path(str(receipt["cache_root"])).resolve()
        relative = PurePosixPath(str(record["cache_object"]))
        return _require_regular_input(
            cache_root.joinpath(*relative.parts), f"admitted input {scope}/{name}"
        )
    raise ReleaseError(f"release input is absent from offline admission: {scope}/{name}")


def _archive_file_inventory(path: Path, description: str) -> list[dict[str, object]]:
    """Return the exact confined regular-file inventory of a deterministic tar.xz."""
    try:
        with tarfile.open(path, mode="r:xz") as archive:
            members = archive.getmembers()
            names = [member.name for member in members]
            if len(names) != len(set(names)) or any(not member.isfile() for member in members):
                raise ReleaseError(f"{description} contains duplicate or non-file members")
            inventory = []
            for member in members:
                pure = PurePosixPath(member.name)
                if pure.is_absolute() or not pure.parts or ".." in pure.parts:
                    raise ReleaseError(f"{description} contains an unsafe path")
                stream = archive.extractfile(member)
                if stream is None:
                    raise ReleaseError(f"{description} contains an unreadable member")
                contents = stream.read()
                inventory.append(
                    {
                        "path": member.name,
                        "sha256": sha256_bytes(contents),
                        "size": len(contents),
                    }
                )
            return sorted(inventory, key=lambda item: str(item["path"]))
    except (tarfile.TarError, lzma.LZMAError) as error:
        raise ReleaseError(f"{description} is not a valid tar.xz archive: {error}") from error


def _expected_artifacts(version: str) -> dict[str, str]:
    return {
        f"nilo_{version}_amd64.deb": "ubuntu-24.04-amd64",
        f"nilo-{version}-ubuntu-24.04-amd64.run": "ubuntu-24.04-amd64",
        f"nilo_{version}_arm64.deb": "rpi-debian13-arm64",
        f"nilo-{version}-rpi-debian13-arm64.run": "rpi-debian13-arm64",
        f"nilo-pi4-{version}.img.xz": "rpi-debian13-arm64",
    }


def _observe_offline_network() -> dict[str, object]:
    """Observe the Linux network namespace used to execute acceptance checks."""
    if not sys.platform.startswith("linux"):
        raise ReleaseError("software acceptance must execute in a Linux network namespace")
    try:
        interfaces = sorted(path.name for path in Path("/sys/class/net").iterdir())
        ipv4_lines = Path("/proc/net/route").read_text(encoding="ascii").splitlines()[1:]
        ipv6_lines = Path("/proc/net/ipv6_route").read_text(encoding="ascii").splitlines()
    except OSError as error:
        raise ReleaseError(f"cannot observe the acceptance network namespace: {error}") from error
    ipv4_default = any(
        len(fields) >= 8 and fields[1] == "00000000" and fields[7] == "00000000"
        for fields in (line.split() for line in ipv4_lines)
    )
    ipv6_default = False
    for fields in (line.split() for line in ipv6_lines):
        if len(fields) < 10 or fields[0] != "0" * 32 or fields[1] != "00":
            continue
        try:
            flags = int(fields[8], 16)
        except ValueError as error:
            raise ReleaseError("acceptance namespace has malformed IPv6 route metadata") from error
        if flags & 0x0200 == 0:
            ipv6_default = True
            break
    return {
        "interfaces": interfaces,
        "ipv4_default_route": ipv4_default,
        "ipv6_default_route": ipv6_default,
        "platform": "linux",
        "verified": True,
    }


def _validated_offline_observation(observer: Callable[[], dict[str, object]]) -> dict[str, object]:
    """Require evidence that acceptance is running with loopback only and no route."""
    observation = observer()
    expected_fields = {
        "interfaces", "ipv4_default_route", "ipv6_default_route", "platform", "verified",
    }
    if not isinstance(observation, dict) or set(observation) != expected_fields:
        raise ReleaseError("acceptance network observation is invalid")
    interfaces = observation.get("interfaces")
    if (
        observation.get("platform") != "linux"
        or observation.get("verified") is not True
        or interfaces != ["lo"]
        or observation.get("ipv4_default_route") is not False
        or observation.get("ipv6_default_route") is not False
    ):
        raise ReleaseError("software acceptance requires a loopback-only Linux namespace with no default route")
    return dict(observation)


def _completed_output(value: object, description: str) -> bytes:
    """Normalize captured subprocess output without accepting arbitrary objects."""
    if value is None:
        return b""
    if isinstance(value, bytes):
        return value
    if isinstance(value, str):
        return value.encode("utf-8")
    raise ReleaseError(f"software acceptance returned invalid {description}")


def _validate_artifact_receipt_document(
    document: dict[str, object],
    *,
    artifact: str,
    artifact_dir: Path,
    binding: dict[str, object],
    invocation: str,
    input_lock_sha256: str,
    source_revision: str,
) -> None:
    """Validate the builder metadata embedded in one build-set receipt."""
    version = Path(__file__).parents[2].joinpath("VERSION").read_text(encoding="utf-8").strip()
    target = _expected_artifacts(version).get(artifact)
    command = "image" if artifact.endswith(".img.xz") else "installer" if artifact.endswith(".run") else "deb"
    inputs = document.get("inputs")
    if (
        target is None
        or document.get("schema") != 2
        or document.get("artifact") != artifact
        or document.get("artifact_path") != str((artifact_dir / artifact).resolve())
        or document.get("sha256") != binding["sha256"]
        or document.get("size") != binding["size"]
        or document.get("builder_sha256") != sha256_file(Path(__file__))
        or document.get("input_lock_sha256") != input_lock_sha256
        or document.get("source_revision") != source_revision
        or document.get("build_invocation_id") != invocation
        or document.get("command") != command
        or document.get("target") != target
        or document.get("version") != version
        or not isinstance(inputs, list)
        or not inputs
    ):
        raise ReleaseError("artifact-builder receipt does not bind its exact builder metadata")
    required_names = {
        "deb": {"input_lock", "offline_receipt", "payload"},
        "installer": {"input_lock", "offline_receipt", "package"},
        "image": {"base_image", "input_lock", "offline_receipt", "package"},
    }[command]
    observed_names: set[str] = set()
    for item in inputs:
        if (
            not isinstance(item, dict)
            or set(item) != {"name", "path", "sha256", "size"}
            or not isinstance(item.get("name"), str)
            or not isinstance(item.get("path"), str)
            or not Path(str(item["path"])).is_absolute()
            or re.fullmatch(r"[0-9a-f]{64}", str(item.get("sha256", ""))) is None
            or type(item.get("size")) is not int
            or int(item["size"]) < 0
            or item["name"] in observed_names
        ):
            raise ReleaseError("artifact-builder receipt has an invalid input binding")
        input_path = _require_regular_input(
            Path(str(item["path"])), f"artifact-builder input {item['name']}"
        )
        if input_path.stat().st_size != item["size"] or sha256_file(input_path) != item["sha256"]:
            raise ReleaseError("artifact-builder receipt input binding does not match current bytes")
        observed_names.add(str(item["name"]))
    if command in {"installer", "image"}:
        if (
            not required_names.issubset(observed_names)
            or not any(name.startswith("pool/") for name in observed_names)
            or any(
            name not in required_names and not name.startswith("pool/") for name in observed_names
            )
        ):
            raise ReleaseError(f"artifact-builder {command} receipt has an invalid input set")
    elif observed_names != required_names:
        raise ReleaseError("artifact-builder receipt has an invalid input set")


def _validate_build_set_receipt(
    path: Path,
    artifact_dir: Path,
    artifact_bindings: list[dict[str, object]],
    *,
    expected_lock_sha256: str | None = None,
    expected_source_revision: str | None = None,
) -> dict[str, object]:
    """Validate one repository-builder receipt for a complete artifact set."""
    receipt = _read_json(
        _require_regular_input(path, "independent build receipt"),
        "independent build receipt",
        expected_schema=3,
    )
    if receipt.get("schema") != 3 or set(receipt) != {
        "artifact_receipts", "artifact_root", "artifacts", "builder_sha256",
        "input_lock_sha256", "invocation_id", "process_id", "schema", "source_revision",
    }:
        raise ReleaseError("independent build receipt has invalid fields")
    root_value = receipt.get("artifact_root")
    invocation = receipt.get("invocation_id")
    if (
        not isinstance(root_value, str)
        or not Path(root_value).is_absolute()
        or Path(root_value).resolve() != artifact_dir
        or not isinstance(invocation, str)
        or re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9._-]{7,127}", invocation) is None
        or receipt.get("builder_sha256") != sha256_file(Path(__file__))
        or receipt.get("artifacts") != artifact_bindings
        or not isinstance(receipt.get("process_id"), int)
        or int(receipt["process_id"]) < 1
        or re.fullmatch(r"[0-9a-f]{64}", str(receipt.get("input_lock_sha256", ""))) is None
        or not isinstance(receipt.get("source_revision"), str)
        or not receipt["source_revision"]
    ):
        raise ReleaseError("independent build receipt does not bind the artifact build")
    if expected_lock_sha256 is not None and receipt["input_lock_sha256"] != expected_lock_sha256:
        raise ReleaseError("independent build receipt does not bind the release input lock")
    if expected_source_revision is not None and receipt["source_revision"] != expected_source_revision:
        raise ReleaseError("independent build receipt does not bind the release source revision")
    artifact_receipts = receipt.get("artifact_receipts")
    if not isinstance(artifact_receipts, list) or len(artifact_receipts) != len(artifact_bindings):
        raise ReleaseError("independent build receipt lacks artifact-builder receipts")
    for item in artifact_receipts:
        if (
            not isinstance(item, dict)
            or set(item) != {"artifact", "receipt", "receipt_sha256"}
            or not isinstance(item.get("artifact"), str)
            or re.fullmatch(r"[0-9a-f]{64}", str(item.get("receipt_sha256", ""))) is None
            or not isinstance(item.get("receipt"), dict)
        ):
            raise ReleaseError("independent build receipt has an invalid artifact receipt binding")
        document = item["receipt"]
        artifact = item["artifact"]
        binding = next((value for value in artifact_bindings if value["name"] == artifact), None)
        if (
            binding is None
            or sha256_bytes(canonical_json(document)) != item["receipt_sha256"]
        ):
            raise ReleaseError("independent build receipt contains an invalid artifact-builder receipt")
        try:
            _validate_artifact_receipt_document(
                document,
                artifact=artifact,
                artifact_dir=artifact_dir,
                binding=binding,
                invocation=str(invocation),
                input_lock_sha256=str(receipt["input_lock_sha256"]),
                source_revision=str(receipt["source_revision"]),
            )
        except ReleaseError as error:
            raise ReleaseError(f"independent build receipt contains an invalid artifact-builder receipt: {error}") from error
    if sorted(str(item["artifact"]) for item in artifact_receipts) != sorted(
        str(item["name"]) for item in artifact_bindings
    ):
        raise ReleaseError("independent build receipt artifact-builder set is incomplete")
    return dict(receipt)


def create_build_set_receipt(
    artifact_dir: Path,
    artifact_receipts: list[Path],
    output: Path,
    *,
    lock_path: Path,
    source_revision: str,
) -> dict[str, object]:
    """Emit a build-invocation receipt from the builders' own artifact receipts."""
    artifact_dir = Path(artifact_dir).resolve()
    if not artifact_dir.is_dir() or artifact_dir.is_symlink():
        raise ReleaseError("build-set artifact root is invalid")
    lock_path = _require_regular_input(lock_path, "build-set input lock")
    if not source_revision or any(character.isspace() for character in source_revision):
        raise ReleaseError("build-set source revision must be one token")
    artifacts = [
        {"name": path.name, "sha256": sha256_file(path), "size": path.stat().st_size}
        for path in sorted(artifact_dir.iterdir(), key=lambda value: value.name)
        if path.is_file() and not path.is_symlink()
    ]
    expected_names = {item["name"] for item in artifacts}
    observed_names: set[str] = set()
    invocation_id: str | None = None
    receipt_bindings: list[dict[str, object]] = []
    for receipt_path in artifact_receipts:
        receipt_path = _require_regular_input(receipt_path, "artifact-builder receipt")
        document = _read_json(receipt_path, "artifact-builder receipt", expected_schema=2)
        artifact = document.get("artifact")
        if (
            not isinstance(artifact, str)
            or artifact not in expected_names
            or artifact in observed_names
            or re.fullmatch(
                r"[A-Za-z0-9][A-Za-z0-9._-]{7,127}",
                str(document.get("build_invocation_id", "")),
            )
            is None
        ):
            raise ReleaseError("artifact-builder receipt does not bind its builder invocation and artifact output path")
        current_invocation = str(document["build_invocation_id"])
        if invocation_id is None:
            invocation_id = current_invocation
        elif invocation_id != current_invocation:
            raise ReleaseError("artifact-builder receipts do not share one pre-established builder invocation")
        binding = next(item for item in artifacts if item["name"] == artifact)
        _validate_artifact_receipt_document(
            document,
            artifact=artifact,
            artifact_dir=artifact_dir,
            binding=binding,
            invocation=current_invocation,
            input_lock_sha256=sha256_file(lock_path),
            source_revision=source_revision,
        )
        observed_names.add(artifact)
        receipt_bindings.append(
            {
                "artifact": artifact,
                "receipt": document,
                "receipt_sha256": sha256_bytes(canonical_json(document)),
            }
        )
    if observed_names != expected_names:
        raise ReleaseError("artifact-builder receipt set is incomplete")
    result: dict[str, object] = {
        "artifact_receipts": sorted(receipt_bindings, key=lambda item: item["artifact"]),
        "artifact_root": str(artifact_dir),
        "artifacts": artifacts,
        "builder_sha256": sha256_file(Path(__file__)),
        "input_lock_sha256": sha256_file(lock_path),
        "invocation_id": invocation_id,
        "process_id": os.getpid(),
        "schema": 3,
        "source_revision": source_revision,
    }
    _write_receipt(Path(output), result)
    return result


def _load_candidate_build_inputs(
    path: Path, *, expected_lock_sha256: str | None = None
) -> dict[str, Path]:
    """Validate the fixed inputs from which acceptance must rebuild a candidate."""
    document = _read_json(
        _require_regular_input(path, "candidate build inputs"),
        "candidate build inputs",
        expected_schema=1,
    )
    file_fields = {
        "admin_password", "base_image", "go_module_cache", "input_lock", "npm_cache",
        "pi_receipt", "pi_sigrok_runtime", "pi_camera_build_input",
        "ubuntu_receipt", "ubuntu_sigrok_runtime", "ubuntu_camera_build_input",
    }
    directory_fields = {
        "pi_pool", "source_root", "ubuntu_pool",
    }
    if set(document) != {"schema", *file_fields, *directory_fields}:
        raise ReleaseError("candidate build inputs have invalid fields")
    result: dict[str, Path] = {}
    for field in sorted(file_fields | directory_fields):
        value = document.get(field)
        if not isinstance(value, str) or not Path(value).is_absolute():
            raise ReleaseError(f"candidate build inputs require an absolute {field} path")
        candidate = Path(value)
        if candidate.is_symlink():
            raise ReleaseError(f"candidate build input {field} must not be a symbolic link")
        if field in file_fields:
            candidate = _require_regular_input(candidate, f"candidate build input {field}")
        elif not candidate.is_dir():
            raise ReleaseError(f"candidate build input {field} must be a regular directory")
        result[field] = candidate.resolve()
    if (
        expected_lock_sha256 is not None
        and sha256_file(result["input_lock"]) != expected_lock_sha256
    ):
        raise ReleaseError("candidate build inputs do not bind the release input lock")
    lock = _read_json(result["input_lock"], "candidate build input lock", expected_schema=1)
    locked_inputs = lock.get("inputs")
    if not isinstance(locked_inputs, list):
        raise ReleaseError("candidate build input lock has no inputs")
    locked_fields = {
        "base_image": ("rpi-debian13-arm64", "base-image"),
        "go_module_cache": ("shared", "go-module-cache"),
        "npm_cache": ("shared", "gui-npm-cache"),
        "pi_sigrok_runtime": ("rpi-debian13-arm64", "sigrok-runtime"),
        "ubuntu_sigrok_runtime": ("ubuntu-24.04-amd64", "sigrok-runtime"),
        "pi_camera_build_input": ("rpi-debian13-arm64", "camera-build-input"),
        "ubuntu_camera_build_input": ("ubuntu-24.04-amd64", "camera-build-input"),
    }
    for field, identity in locked_fields.items():
        matches = [
            item for item in locked_inputs
            if isinstance(item, dict) and (item.get("scope"), item.get("name")) == identity
        ]
        if (
            len(matches) != 1
            or matches[0].get("sha256") != sha256_file(result[field])
            or matches[0].get("size") != result[field].stat().st_size
        ):
            raise ReleaseError(f"candidate build input {field} is not bound by the release input lock")
    return result


def _extract_locked_archive(path: Path, output: Path, description: str) -> None:
    """Extract one lock-bound cache archive without links or path traversal."""
    if output.exists():
        raise ReleaseError(f"{description} extraction root already exists")
    output.mkdir(parents=True)
    try:
        with tarfile.open(path, mode="r:*") as archive:
            members = archive.getmembers()
            names = [member.name for member in members]
            if len(names) != len(set(names)):
                raise ReleaseError(f"{description} contains duplicate members")
            for member in members:
                pure = PurePosixPath(member.name)
                if pure.is_absolute() or not pure.parts or ".." in pure.parts:
                    raise ReleaseError(f"{description} contains an unsafe path")
                destination = output.joinpath(*pure.parts)
                if member.isdir():
                    destination.mkdir(parents=True, exist_ok=True)
                    destination.chmod(stat.S_IMODE(member.mode) & 0o777)
                    continue
                if not member.isfile():
                    raise ReleaseError(f"{description} contains a non-regular member")
                destination.parent.mkdir(parents=True, exist_ok=True)
                stream = archive.extractfile(member)
                if stream is None:
                    raise ReleaseError(f"{description} contains an unreadable member")
                with destination.open("xb") as target:
                    shutil.copyfileobj(stream, target)
                destination.chmod(stat.S_IMODE(member.mode) & 0o777)
    except (tarfile.TarError, lzma.LZMAError) as error:
        raise ReleaseError(f"{description} is not a valid cache archive: {error}") from error


def verify_reproducible_candidate(
    artifact_dir: Path,
    build_inputs_path: Path,
    *,
    source_revision: str,
    source_date_epoch: int,
    expected_lock_sha256: str | None = None,
    runner: Callable[..., object] = subprocess.run,
) -> dict[str, object]:
    """Perform two acceptance-owned fixed builds and compare their complete outputs."""
    artifact_dir = Path(artifact_dir).resolve()
    if not artifact_dir.is_dir() or artifact_dir.is_symlink():
        raise ReleaseError("reproducibility artifact directory is invalid")
    if (
        not source_revision
        or any(character.isspace() for character in source_revision)
        or type(source_date_epoch) is not int
        or source_date_epoch < 0
    ):
        raise ReleaseError("reproducibility source identity is invalid")
    inputs = _load_candidate_build_inputs(
        Path(build_inputs_path), expected_lock_sha256=expected_lock_sha256,
    )
    lock_sha256 = sha256_file(inputs["input_lock"])
    version = Path(__file__).parents[2].joinpath("VERSION").read_text(encoding="utf-8").strip()
    expected_names = set(_expected_artifacts(version))
    if {path.name for path in artifact_dir.iterdir()} != expected_names:
        raise ReleaseError("reproducibility candidate does not contain the exact artifact set")
    artifact_bindings = [
        {"name": path.name, "sha256": sha256_file(_require_regular_input(path, "candidate artifact")), "size": path.stat().st_size}
        for path in sorted(artifact_dir.iterdir(), key=lambda value: value.name)
    ]
    builder = _require_regular_input(BUILD_CANDIDATE_SET, "fixed candidate builder")
    builds: list[dict[str, object]] = []
    with tempfile.TemporaryDirectory(prefix=".nilo-repro-", dir=artifact_dir.parent) as temporary:
        temporary_root = Path(temporary)
        extracted_inputs: dict[str, Path] = {}
        for field in (
            "go_module_cache", "npm_cache", "ubuntu_sigrok_runtime", "pi_sigrok_runtime",
            "ubuntu_camera_build_input", "pi_camera_build_input",
        ):
            extracted = temporary_root / f"input-{field}"
            _extract_locked_archive(inputs[field], extracted, field.replace("_", " "))
            extracted_inputs[field] = extracted
        for number in (1, 2):
            output = temporary_root / f"build-{number}"
            command = [
                "sh", str(builder), str(output),
                str(inputs["source_root"]), str(extracted_inputs["go_module_cache"]),
                str(extracted_inputs["npm_cache"]), str(inputs["ubuntu_pool"]),
                str(inputs["ubuntu_receipt"]), str(extracted_inputs["ubuntu_sigrok_runtime"]),
                str(inputs["pi_pool"]), str(inputs["pi_receipt"]),
                str(extracted_inputs["pi_sigrok_runtime"]),
                str(inputs["base_image"]), str(inputs["admin_password"]),
                str(inputs["input_lock"]),
                str(extracted_inputs["ubuntu_camera_build_input"]),
                str(extracted_inputs["pi_camera_build_input"]),
            ]
            environment = dict(os.environ)
            environment["SOURCE_DATE_EPOCH"] = str(source_date_epoch)
            environment["NILO_SOURCE_REVISION"] = source_revision
            try:
                completed = runner(
                    command,
                    capture_output=True,
                    check=False,
                    cwd=Path(__file__).parents[2].resolve(),
                    env=environment,
                    timeout=3600,
                )
            except (OSError, subprocess.TimeoutExpired) as error:
                raise ReleaseError(f"independent candidate build {number} could not execute: {error}") from error
            if type(getattr(completed, "returncode", None)) is not int:
                raise ReleaseError("independent candidate build returned no integer exit code")
            if completed.returncode != 0:
                raise ReleaseError(f"independent candidate build {number} failed with exit {completed.returncode}")
            if not output.is_dir() or output.is_symlink() or {path.name for path in output.iterdir()} != expected_names:
                raise ReleaseError("independent candidate build produced an invalid artifact set")
            rebuilt_bindings = [
                {"name": path.name, "sha256": sha256_file(_require_regular_input(path, "rebuilt artifact")), "size": path.stat().st_size}
                for path in sorted(output.iterdir(), key=lambda value: value.name)
            ]
            if rebuilt_bindings != artifact_bindings:
                raise ReleaseError("independently rebuilt artifacts are not byte-reproducible")
            receipt = _validate_build_set_receipt(
                Path(str(output) + ".build.json"),
                output.resolve(),
                rebuilt_bindings,
                expected_lock_sha256=lock_sha256,
                expected_source_revision=source_revision,
            )
            builds.append(
                {"artifacts": rebuilt_bindings, "invocation_id": receipt["invocation_id"]}
            )
    if builds[0]["invocation_id"] == builds[1]["invocation_id"]:
        raise ReleaseError("independent candidate builds reused one invocation")
    return {"artifacts": artifact_bindings, "builds": builds, "schema": 1, "status": "pass"}


def _artifact_receipt_inputs(options: argparse.Namespace) -> list[dict[str, object]]:
    """Bind non-secret builder inputs without recording credential material."""
    paths: list[tuple[str, Path]] = [
        ("input_lock", options.lock),
        ("offline_receipt", options.offline_receipt),
    ]
    if options.command == "deb":
        paths.append(("payload", options.payload))
    elif options.command == "installer":
        paths.append(("package", options.package))
        paths.extend(
            (f"pool/{path.name}", path)
            for path in sorted(Path(options.pool).iterdir(), key=lambda item: item.name)
        )
    elif options.command == "image":
        paths.extend((("base_image", options.base_image), ("package", options.package)))
        paths.extend(
            (f"pool/{path.name}", path)
            for path in sorted(Path(options.pool).iterdir(), key=lambda item: item.name)
        )
    return [
        {
            "name": name,
            "path": str(_require_regular_input(path, f"builder receipt input {name}")),
            "sha256": sha256_file(Path(path)),
            "size": Path(path).stat().st_size,
        }
        for name, path in paths
    ]


def _bind_artifact_builder_receipt(
    result: dict[str, object], options: argparse.Namespace
) -> dict[str, object]:
    if options.command not in {"deb", "installer", "image"}:
        return result
    invocation_id = getattr(options, "build_invocation", None)
    source_revision = getattr(options, "source_revision", None)
    if not isinstance(invocation_id, str) or re.fullmatch(
        r"[A-Za-z0-9][A-Za-z0-9._-]{7,127}", invocation_id
    ) is None:
        raise ReleaseError("artifact receipt requires a pre-established builder invocation")
    if not isinstance(source_revision, str) or not source_revision or any(
        character.isspace() for character in source_revision
    ):
        raise ReleaseError("artifact receipt requires one source revision token")
    bound = dict(result)
    bound.update(
        {
            "artifact_path": str(Path(options.output).resolve()),
            "build_invocation_id": invocation_id,
            "builder_sha256": sha256_file(Path(__file__)),
            "command": options.command,
            "input_lock_sha256": sha256_file(_require_regular_input(options.lock, "artifact input lock")),
            "inputs": _artifact_receipt_inputs(options),
            "schema": 2,
            "source_revision": source_revision,
        }
    )
    return bound


def _execute_acceptance_check(
    check: dict[str, object],
    *,
    artifact_dir: Path,
    runner: Callable[..., object],
    network_observer: Callable[[], dict[str, object]],
    expected_lock_sha256: str | None = None,
    expected_source_revision: str | None = None,
    expected_source_date_epoch: int | None = None,
) -> tuple[dict[str, object], list[tuple[Path, bytes]]]:
    """Execute one strict acceptance command and derive its retained evidence."""
    if "command" in check:
        raise ReleaseError("software acceptance must use the repository-owned acceptance runner")
    target_id = str(check["target"])
    name = str(check["check"])
    expected_fields = {"check", "kind", "target", "timeout_seconds"}
    if name == "build-reproducibility":
        expected_fields.add("candidate_build_inputs")
        if not expected_fields.issubset(check):
            raise ReleaseError("reproducibility acceptance requires candidate build inputs")
    if name in PRIOR_INSTALLER_CHECKS:
        expected_fields.add("prior_installer")
    if name in LIFECYCLE_ACCEPTANCE_CHECKS:
        expected_fields.add("target_image")
    if set(check) != expected_fields:
        raise ReleaseError("software acceptance command plan has invalid fields")
    timeout_seconds = check.get("timeout_seconds")
    if type(timeout_seconds) is not int or not 1 <= timeout_seconds <= 3600:
        raise ReleaseError("software acceptance command plan is invalid")
    target_image = check.get("target_image")
    if name in LIFECYCLE_ACCEPTANCE_CHECKS and (
        not isinstance(target_image, str)
        or re.fullmatch(r"[^\s@]+@sha256:[0-9a-f]{64}", target_image) is None
    ):
        raise ReleaseError("lifecycle acceptance requires one reviewed digest-pinned target image")
    executable_path = (
        TARGET_ACCEPTANCE_HARNESS if name in LIFECYCLE_ACCEPTANCE_CHECKS else ACCEPTANCE_RUNNER
    )
    executable = _require_regular_input(executable_path, "repository-owned acceptance runner")
    artifact_dir = Path(artifact_dir).resolve()
    if not artifact_dir.is_dir() or artifact_dir.is_symlink():
        raise ReleaseError("software acceptance artifact directory is invalid")
    command = {
        "argv": [
            "sh", str(executable), "--target", target_id, "--check", name,
            "--artifacts", str(artifact_dir),
        ],
        "cwd": str(Path(__file__).parents[2].resolve()),
        "timeout_seconds": timeout_seconds,
    }
    artifact_bindings = [
        {"name": path.name, "sha256": sha256_file(path), "size": path.stat().st_size}
        for path in sorted(artifact_dir.iterdir(), key=lambda value: value.name)
        if path.is_file() and not path.is_symlink()
    ]
    reproduced_bindings: list[dict[str, object]] = []
    build_receipts: list[dict[str, object]] = []
    prior_artifacts: list[dict[str, object]] = []
    if name == "build-reproducibility":
        inputs_value = check.get("candidate_build_inputs")
        if not isinstance(inputs_value, str) or not Path(inputs_value).is_absolute():
            raise ReleaseError("reproducibility acceptance requires absolute candidate build inputs")
        build_inputs = _require_regular_input(Path(inputs_value), "candidate build inputs")
        if expected_source_revision is None or expected_source_date_epoch is None:
            raise ReleaseError("reproducibility acceptance requires the release source identity")
        if expected_lock_sha256 is None:
            raise ReleaseError("reproducibility acceptance requires the release input lock identity")
        command["argv"].extend(
            [
                "--candidate-build-inputs", str(build_inputs),
                "--source-revision", expected_source_revision,
                "--source-date-epoch", str(expected_source_date_epoch),
                "--expected-lock-sha256", expected_lock_sha256,
            ]
        )
    if name in PRIOR_INSTALLER_CHECKS:
        prior_value = check.get("prior_installer")
        if not isinstance(prior_value, str) or not Path(prior_value).is_absolute():
            raise ReleaseError("rollback acceptance requires an absolute prior installer")
        prior_path = _require_regular_input(Path(prior_value), "prior release installer")
        suffix = f"-{target_id}.run"
        if not prior_path.name.startswith("nilo-") or not prior_path.name.endswith(suffix):
            raise ReleaseError("rollback acceptance prior installer identity is invalid")
        prior_version = prior_path.name.removeprefix("nilo-").removesuffix(suffix)
        if SEMVER.fullmatch(prior_version) is None:
            raise ReleaseError("rollback acceptance prior installer version is invalid")
        current_packages = [
            path for path in artifact_dir.iterdir()
            if path.name.endswith(f"_{TARGET_ARCHITECTURES[target_id]}.deb")
        ]
        if len(current_packages) != 1:
            raise ReleaseError("rollback acceptance cannot resolve the candidate package")
        current_version = _deb_control(current_packages[0]).get("Version")
        if prior_version == current_version:
            raise ReleaseError("rollback acceptance requires a distinct prior version")
        _verify_installer_structure(prior_path, target_id, prior_version)
        prior_artifacts = [{
            "name": prior_path.name,
            "sha256": sha256_file(prior_path),
            "size": prior_path.stat().st_size,
            "version": prior_version,
        }]
        command["argv"].extend(["--prior-installer", str(prior_path)])

    before = _validated_offline_observation(network_observer)
    environment = dict(os.environ)
    environment["NILO_ACCEPTANCE_TARGET"] = target_id
    environment["NILO_ACCEPTANCE_CHECK"] = name
    if name in LIFECYCLE_ACCEPTANCE_CHECKS:
        environment["NILO_ACCEPTANCE_EXPECTED_IMAGE"] = str(target_image)
    try:
        completed = runner(
            command["argv"],
            capture_output=True,
            check=False,
            cwd=Path(str(command["cwd"])),
            env=environment,
            timeout=timeout_seconds,
        )
    except subprocess.TimeoutExpired as error:
        raise ReleaseError(f"software acceptance timed out: {target_id}/{name}") from error
    except OSError as error:
        raise ReleaseError(f"software acceptance could not execute {target_id}/{name}: {error}") from error
    after = _validated_offline_observation(network_observer)
    return_code = getattr(completed, "returncode", None)
    stdout = _completed_output(getattr(completed, "stdout", None), "stdout")
    stderr = _completed_output(getattr(completed, "stderr", None), "stderr")
    if type(return_code) is not int:
        raise ReleaseError("software acceptance returned no integer exit code")
    if return_code != 0:
        raise ReleaseError(f"software acceptance failed with exit {return_code}: {target_id}/{name}")

    if name == "build-reproducibility":
        try:
            reproducibility = json.loads(stdout.decode("utf-8", errors="strict"))
        except (UnicodeDecodeError, json.JSONDecodeError) as error:
            raise ReleaseError("reproducibility acceptance returned invalid JSON evidence") from error
        if (
            not isinstance(reproducibility, dict)
            or set(reproducibility) != {"artifacts", "builds", "schema", "status"}
            or reproducibility.get("schema") != 1
            or reproducibility.get("status") != "pass"
            or reproducibility.get("artifacts") != artifact_bindings
            or not isinstance(reproducibility.get("builds"), list)
            or len(reproducibility["builds"]) != 2
        ):
            raise ReleaseError("reproducibility acceptance returned invalid build evidence")
        for build in reproducibility["builds"]:
            if (
                not isinstance(build, dict)
                or set(build) != {"artifacts", "invocation_id"}
                or build.get("artifacts") != artifact_bindings
                or re.fullmatch(
                    r"[A-Za-z0-9][A-Za-z0-9._-]{7,127}", str(build.get("invocation_id", ""))
                ) is None
            ):
                raise ReleaseError("reproducibility acceptance returned invalid build evidence")
        if reproducibility["builds"][0]["invocation_id"] == reproducibility["builds"][1]["invocation_id"]:
            raise ReleaseError("reproducibility acceptance reused one build invocation")
        reproduced_bindings = list(artifact_bindings)
        build_receipts = list(reproducibility["builds"])

    target_environment: dict[str, str] | None = None
    if name in LIFECYCLE_ACCEPTANCE_CHECKS:
        prefixes = {
            "NILO_ACCEPTANCE_IMAGE_REFERENCE": "image_reference",
            "NILO_ACCEPTANCE_IMAGE_ID": "image_id",
            "NILO_ACCEPTANCE_KERNEL": "kernel",
            "NILO_ACCEPTANCE_OS_ID": "os_id",
            "NILO_ACCEPTANCE_OS_VERSION": "os_version",
            "NILO_ACCEPTANCE_ARCHITECTURE": "architecture",
        }
        observed: dict[str, str] = {}
        try:
            output_lines = stdout.decode("utf-8", errors="strict").splitlines()
        except UnicodeDecodeError as error:
            raise ReleaseError("target acceptance identity output is not UTF-8") from error
        for line in output_lines:
            for prefix, field in prefixes.items():
                marker = prefix + "="
                if line.startswith(marker):
                    if field in observed:
                        raise ReleaseError("target acceptance identity contains a duplicate field")
                    observed[field] = line.removeprefix(marker)
        expected_architecture = TARGET_ARCHITECTURES[target_id]
        if (
            set(observed) != set(prefixes.values())
            or observed.get("image_reference") != target_image
            or re.fullmatch(r"sha256:[0-9a-f]{64}", observed.get("image_id", "")) is None
            or re.fullmatch(r"[A-Za-z0-9._+~-]{1,255}", observed.get("kernel", "")) is None
            or observed.get("architecture") != expected_architecture
            or not observed.get("os_id")
            or not observed.get("os_version")
        ):
            raise ReleaseError("target acceptance identity does not match the reviewed exact target")
        target_environment = observed

    base = Path("evidence") / "acceptance" / target_id / name
    stdout_path = base.with_suffix(".stdout")
    stderr_path = base.with_suffix(".stderr")
    plan_digest = sha256_bytes(canonical_json(command))
    result = {
        "acceptance_runner_sha256": _acceptance_runner_digest(),
        "check": name,
        "artifacts": artifact_bindings,
        "build_receipts": build_receipts,
        "command_plan_sha256": plan_digest,
        "exit_code": return_code,
        "network_after": after,
        "network_before": before,
        "prior_artifacts": prior_artifacts,
        "reproduced_artifacts": reproduced_bindings,
        "schema": 1,
        "stderr": {
            "path": stderr_path.as_posix(), "sha256": sha256_bytes(stderr), "size": len(stderr),
        },
        "stdout": {
            "path": stdout_path.as_posix(), "sha256": sha256_bytes(stdout), "size": len(stdout),
        },
        "target": target_id,
        "target_environment": target_environment,
    }
    result_bytes = canonical_json(result)
    result_path = base.with_suffix(".json")
    return result, [
        (result_path, result_bytes),
        (stdout_path, stdout),
        (stderr_path, stderr),
    ]


def _validate_sbom_relationships(sbom: dict[str, object]) -> None:
    """Require exact SPDX edges from deliverables to admitted component closure."""
    packages = sbom.get("packages")
    files = sbom.get("files")
    relationships = sbom.get("relationships")
    if not isinstance(packages, list) or not isinstance(files, list) or not isinstance(relationships, list):
        raise ReleaseError("SBOM relationship graph is missing")
    package_ids: dict[str, dict[str, object]] = {}
    for package in packages:
        if not isinstance(package, dict) or not isinstance(package.get("SPDXID"), str):
            raise ReleaseError("SBOM relationship package identity is invalid")
        identifier = str(package["SPDXID"])
        if identifier in package_ids:
            raise ReleaseError("SBOM relationship graph has duplicate package identities")
        package_ids[identifier] = package
    file_ids: dict[str, dict[str, object]] = {}
    for file in files:
        if not isinstance(file, dict) or not isinstance(file.get("SPDXID"), str):
            raise ReleaseError("SBOM relationship file identity is invalid")
        identifier = str(file["SPDXID"])
        if identifier in file_ids:
            raise ReleaseError("SBOM relationship graph has duplicate file identities")
        file_ids[identifier] = file

    input_ids: dict[tuple[str, str], str] = {}
    deliverables: list[tuple[str, str, str]] = []
    component_owners: list[tuple[str, str, str]] = []
    for identifier, package in package_ids.items():
        comment = package.get("packageComment")
        if not isinstance(comment, str):
            continue
        input_match = re.fullmatch(r"admitted input scope=([^\s]+) name=([^\s]+)", comment)
        deliverable_match = re.fullmatch(r"deliverable target=([^\s]+) path=(.+)", comment)
        component_match = re.fullmatch(
            r"(?:OS package|runtime component) scope=([^\s]+) input=([^\s]+)", comment
        )
        if input_match:
            identity = (input_match.group(1), input_match.group(2))
            if identity in input_ids:
                raise ReleaseError("SBOM relationship graph has duplicate admitted inputs")
            input_ids[identity] = identifier
        elif deliverable_match:
            deliverables.append((identifier, deliverable_match.group(1), deliverable_match.group(2)))
        elif component_match:
            component_owners.append((identifier, component_match.group(1), component_match.group(2)))

    file_by_name = {
        str(file.get("fileName")): identifier for identifier, file in file_ids.items()
    }
    expected: set[tuple[str, str, str]] = set()
    for deliverable_id, target, path in deliverables:
        file_id = file_by_name.get(path)
        if file_id is None:
            raise ReleaseError("SBOM relationship deliverable has no matching artifact file")
        expected.add(("SPDXRef-DOCUMENT", "DESCRIBES", deliverable_id))
        expected.add((deliverable_id, "CONTAINS", file_id))
        matching_inputs = [
            input_id for (scope, _), input_id in input_ids.items() if scope in {"shared", target}
        ]
        if not matching_inputs:
            raise ReleaseError("SBOM relationship deliverable has no admitted dependencies")
        expected.update((deliverable_id, "DEPENDS_ON", input_id) for input_id in matching_inputs)
    for component_id, scope, input_name in component_owners:
        owner = input_ids.get((scope, input_name))
        if owner is None:
            raise ReleaseError("SBOM relationship component has no admitted owning input")
        expected.add((owner, "CONTAINS", component_id))
    if len(deliverables) != len(files) or not input_ids or not component_owners:
        raise ReleaseError("SBOM relationship graph is incomplete")

    actual: set[tuple[str, str, str]] = set()
    for relationship in relationships:
        if not isinstance(relationship, dict) or set(relationship) != {
            "relatedSpdxElement", "relationshipType", "spdxElementId"
        }:
            raise ReleaseError("SBOM relationship declaration is invalid")
        edge = (
            str(relationship["spdxElementId"]),
            str(relationship["relationshipType"]),
            str(relationship["relatedSpdxElement"]),
        )
        if edge in actual:
            raise ReleaseError("SBOM relationship graph has duplicate edges")
        actual.add(edge)
    if actual != expected:
        raise ReleaseError("SBOM relationship graph does not match deliverable dependency closure")


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
    runner: Callable[..., object] = subprocess.run,
    network_observer: Callable[[], dict[str, object]] = _observe_offline_network,
    apt_signature_verifier: Callable[[Path, Path, list[str]], str] | None = None,
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
        if "apt" not in item:
            raise ReleaseError("input lock entry lacks dependency provenance metadata")
    observed_inputs = {(str(item["scope"]), str(item["name"])) for item in inputs}
    required_inputs = {(scope, name) for scope, names in REQUIRED_RELEASE_INPUTS.items() for name in names}
    if observed_inputs != required_inputs:
        raise ReleaseError("input lock does not contain the exact required release input classes")
    if not offline_receipts or len(offline_receipts) != len(TARGET_ARCHITECTURES):
        raise ReleaseError("one offline preparation receipt per target is required")
    receipt_targets: set[str] = set()
    receipt_documents: dict[str, dict[str, Any]] = {}
    for receipt_path in offline_receipts:
        receipt_preview = _read_json(_require_regular_input(receipt_path, "offline preparation receipt"), "offline preparation receipt")
        receipt_target = receipt_preview.get("target", {}).get("id") if isinstance(receipt_preview.get("target"), dict) else None
        if receipt_target not in TARGET_ARCHITECTURES or receipt_target in receipt_targets:
            raise ReleaseError("offline preparation receipt set has a duplicate or unknown target")
        receipt_documents[str(receipt_target)] = _validate_offline_receipt(
            receipt_path, str(receipt_target), lock_path=lock_path,
        )
        receipt_targets.add(str(receipt_target))
    if receipt_targets != set(TARGET_ARCHITECTURES):
        raise ReleaseError("offline preparation receipt set is incomplete")
    targets_path = _require_regular_input(targets_path, "target registry")
    input_contract, validated_lock = _validate_release_input_contract(lock_path, targets_path)
    _revalidate_signed_dependency_pools(
        input_contract, validated_lock, receipt_documents, apt_signature_verifier,
    )
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

    artifact_dir = Path(artifact_dir).resolve()
    if not artifact_dir.is_dir() or artifact_dir.is_symlink():
        raise ReleaseError("artifact input must be a regular directory")
    expected = _expected_artifacts(version)
    discovered = {path.name: path for path in artifact_dir.iterdir()}
    if set(discovered) != set(expected):
        missing = sorted(set(expected) - set(discovered))
        extra = sorted(set(discovered) - set(expected))
        raise ReleaseError(f"artifact set mismatch; missing={missing}, extra={extra}")
    for name, path in discovered.items():
        _require_regular_input(path, f"release artifact {name}")
    installer_packages = {
        expected[name]: records
        for name, path in discovered.items()
        if (records := _verify_release_artifact_structure(path, expected[name], version))
        is not None
    }
    for target_id in TARGET_ARCHITECTURES:
        apt_input = next(
            item
            for item in inputs
            if item["scope"] == target_id and item["name"] == "os-package-pool"
        )
        apt = apt_input["apt"]
        expected_dependencies = sorted(
            (
                {
                    "architecture": package["architecture"],
                    "name": PurePosixPath(package["filename"]).name,
                    "package": package["package"],
                    "sha256": package["sha256"],
                    "size": package["size"],
                    "version": package["version"],
                }
                for package in apt["packages"]
            ),
            key=lambda item: str(item["name"]),
        )
        records = installer_packages.get(target_id, [])
        actual_dependencies = sorted(
            (record for record in records if record["package"] != "nilo"),
            key=lambda item: str(item["name"]),
        )
        if actual_dependencies != expected_dependencies:
            raise ReleaseError(
                f"installer dependency set does not match signed pool: {target_id}"
            )
        nilo_records = [record for record in records if record["package"] == "nilo"]
        deb_name = f"nilo_{version}_{TARGET_ARCHITECTURES[target_id]}.deb"
        if len(nilo_records) != 1 or nilo_records[0] != {
            "architecture": TARGET_ARCHITECTURES[target_id],
            "name": deb_name,
            "package": "nilo",
            "sha256": sha256_file(discovered[deb_name]),
            "size": discovered[deb_name].stat().st_size,
            "version": version,
        }:
            raise ReleaseError(f"installer does not embed the exact NILO package: {target_id}")
    scan_release_tree(artifact_dir)

    if acceptance_path is None:
        raise ReleaseError("executed software acceptance evidence is required")
    acceptance_path = _require_regular_input(acceptance_path, "software acceptance evidence")
    acceptance = _read_json(
        acceptance_path, "software acceptance command plan", expected_schema=2,
    )
    if acceptance.get("schema") != 2 or set(acceptance) != {"checks", "schema"}:
        raise ReleaseError("software acceptance command plan must use schema 2")
    acceptance_checks = acceptance.get("checks")
    if not isinstance(acceptance_checks, list):
        raise ReleaseError("software acceptance command plan has no checks")
    observed: set[tuple[str, str]] = set()
    acceptance_files: list[tuple[Path, bytes]] = []
    closed_checks: list[dict[str, object]] = []
    for check in acceptance_checks:
        if not isinstance(check, dict):
            raise ReleaseError("software acceptance evidence contains an invalid check")
        target_id = check.get("target")
        name = check.get("check")
        kind = check.get("kind")
        key = (str(target_id), str(name))
        if target_id not in TARGET_ARCHITECTURES or key in observed:
            raise ReleaseError("software acceptance evidence contains a duplicate or unknown target")
        observed.add(key)
        if kind == "software":
            result, files = _execute_acceptance_check(
                check,
                artifact_dir=artifact_dir,
                runner=runner,
                network_observer=network_observer,
                expected_lock_sha256=sha256_file(lock_path),
                expected_source_revision=source_revision,
                expected_source_date_epoch=source_date_epoch,
            )
            acceptance_files.extend(files)
            destination, result_bytes = files[0]
            closed_checks.append(
                {
                    "check": name,
                    "evidence": {"path": destination.as_posix(), "sha256": sha256_bytes(result_bytes)},
                    "kind": kind,
                    "status": "passed",
                    "target": target_id,
                }
            )
        elif kind == "physical":
            if set(check) != {"check", "kind", "reason", "target"} or not isinstance(check.get("reason"), str) or not check["reason"]:
                raise ReleaseError("physical acceptance lacks a not-run reason")
            closed_checks.append({**check, "status": "not_run"})
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
    if (
        "SOURCE_OFFER.txt" not in source_files
        or "source-index.json" not in source_files
        or not any(path.startswith("LICENSES/") for path in source_files)
    ):
        raise ReleaseError("source offer must contain its offer, source index and LICENSES content")
    scan_release_tree(source_offers_path)
    source_inventory = sorted(
        [
            {
                "path": relative,
                "sha256": sha256_file(source),
                "size": details.st_size,
            }
            for source, details, relative in source_entries
            if stat.S_ISREG(details.st_mode)
        ],
        key=lambda item: str(item["path"]),
    )
    admitted_source_offer = _admitted_cache_object(
        inputs, receipt_documents, scope="shared", name="source-offers"
    )
    if _archive_file_inventory(admitted_source_offer, "admitted source offer") != source_inventory:
        raise ReleaseError("source offer directory is not bound to the admitted source offer bytes")
    source_index = _read_json(source_offers_path / "source-index.json", "source obligation index")
    if set(source_index) != {"entries", "schema"} or not isinstance(source_index.get("entries"), list):
        raise ReleaseError("source obligation index is invalid")
    indexed_inputs: set[tuple[str, str]] = set()
    for entry in source_index["entries"]:
        if (
            not isinstance(entry, dict)
            or set(entry) != {"license_file", "name", "scope", "source_path"}
            or not isinstance(entry.get("license_file"), str)
            or entry["license_file"] not in source_files
            or entry.get("source_path") is not None
            and entry["source_path"] not in source_files
        ):
            raise ReleaseError("source obligation index has an invalid relationship")
        identity = (str(entry.get("scope")), str(entry.get("name")))
        if identity in indexed_inputs:
            raise ReleaseError("source obligation index has duplicate input relationships")
        indexed_inputs.add(identity)
    if indexed_inputs != observed_inputs:
        raise ReleaseError("source obligation index does not cover every admitted input")
    if not OPERATOR_GUIDE.is_file():
        raise ReleaseError("operator guide is missing")

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
                "acceptance_runner_sha256": _acceptance_runner_digest(),
                "release_builder_sha256": sha256_file(Path(__file__)),
                "source_date_epoch": source_date_epoch,
                "source_revision": source_revision,
            },
            "input_lock": {"path": "inputs.lock.json", "sha256": sha256_file(lock_path)},
            "target_registry": {"path": "targets.json", "sha256": sha256_file(targets_path)},
            "acceptance_plan_sha256": sha256_file(acceptance_path),
            "artifacts": artifact_records,
            "schema": 1,
            "targets": [
                {"architecture": TARGET_ARCHITECTURES[target], "id": target}
                for target in sorted(TARGET_ARCHITECTURES)
            ],
            "version": version,
        }
        sorted_inputs = sorted(
            inputs, key=lambda value: (str(value.get("scope")), str(value.get("name")))
        )
        input_ids: dict[tuple[str, str], str] = {}
        sbom_packages: list[dict[str, object]] = []
        relationships: list[dict[str, str]] = []
        for index, item in enumerate(sorted_inputs, start=1):
            input_id = f"SPDXRef-Input-{index}"
            scope = str(item["scope"])
            name = str(item["name"])
            input_ids[(scope, name)] = input_id
            sbom_packages.append(
                {
                    "SPDXID": input_id,
                    "checksums": [{"algorithm": "SHA256", "checksumValue": item["sha256"]}],
                    "downloadLocation": item["url"],
                    "licenseConcluded": item["license"],
                    "name": name,
                    "packageComment": f"admitted input scope={scope} name={name}",
                    "versionInfo": item["version"],
                }
            )
        for index, artifact in enumerate(artifact_records, start=1):
            deliverable_id = f"SPDXRef-Deliverable-{index}"
            file_id = f"SPDXRef-Artifact-{index}"
            target = str(artifact["target"])
            sbom_packages.append(
                {
                    "SPDXID": deliverable_id,
                    "downloadLocation": "NOASSERTION",
                    "licenseConcluded": "NOASSERTION",
                    "name": str(artifact["path"]),
                    "packageComment": f"deliverable target={target} path={artifact['path']}",
                    "versionInfo": version,
                }
            )
            relationships.extend(
                [
                    {"spdxElementId": "SPDXRef-DOCUMENT", "relationshipType": "DESCRIBES", "relatedSpdxElement": deliverable_id},
                    {"spdxElementId": deliverable_id, "relationshipType": "CONTAINS", "relatedSpdxElement": file_id},
                ]
            )
            for item in sorted_inputs:
                scope = str(item["scope"])
                if scope in {"shared", target}:
                    relationships.append(
                        {
                            "spdxElementId": deliverable_id,
                            "relationshipType": "DEPENDS_ON",
                            "relatedSpdxElement": input_ids[(scope, str(item["name"]))],
                        }
                    )
        os_package_index = 1
        for item in sorted_inputs:
            apt = item.get("apt")
            if apt is None:
                continue
            if not isinstance(apt, dict) or not isinstance(apt.get("packages"), list):
                raise ReleaseError("OS dependency input lacks per-package provenance")
            repository = apt.get("repository")
            if not isinstance(repository, str) or not repository.startswith("https://"):
                raise ReleaseError("OS dependency repository provenance is invalid")
            for package in apt["packages"]:
                if not isinstance(package, dict) or any(
                    field not in package for field in ("filename", "package", "sha256", "version")
                ):
                    raise ReleaseError("OS dependency package provenance is invalid")
                package_id = f"SPDXRef-OSPackage-{os_package_index}"
                sbom_packages.append(
                    {
                        "SPDXID": package_id,
                        "checksums": [
                            {"algorithm": "SHA256", "checksumValue": package["sha256"]},
                        ],
                        "downloadLocation": f"{repository.rstrip('/')}/{package['filename']}",
                        "licenseConcluded": item["license"],
                        "name": package["package"],
                        "packageComment": f"OS package scope={item['scope']} input={item['name']}",
                        "versionInfo": package["version"],
                    }
                )
                relationships.append(
                    {
                        "spdxElementId": input_ids[(str(item["scope"]), str(item["name"]))],
                        "relationshipType": "CONTAINS",
                        "relatedSpdxElement": package_id,
                    }
                )
                os_package_index += 1
        runtime_component_index = 1
        for item in sorted_inputs:
            if item.get("artifact_type") != "sigrok-runtime-tar":
                continue
            runtime_archive = _admitted_cache_object(
                inputs,
                receipt_documents,
                scope=str(item["scope"]),
                name=str(item["name"]),
            )
            try:
                with tarfile.open(runtime_archive, mode="r:xz") as archive:
                    manifest_member = archive.getmember("runtime-manifest.json")
                    stream = archive.extractfile(manifest_member)
                    if stream is None:
                        raise ReleaseError("Sigrok runtime manifest is unreadable")
                    runtime_manifest = json.load(stream)
            except (KeyError, tarfile.TarError, lzma.LZMAError, json.JSONDecodeError) as error:
                raise ReleaseError(f"Sigrok runtime manifest is invalid: {error}") from error
            components = runtime_manifest.get("components") if isinstance(runtime_manifest, dict) else None
            if not isinstance(components, list) or not components:
                raise ReleaseError("Sigrok runtime has no component-complete SBOM inventory")
            for component in components:
                if (
                    not isinstance(component, dict)
                    or set(component) != {"license", "name", "source", "version"}
                    or not all(isinstance(component.get(field), str) and component[field] for field in component)
                    or not component["source"].startswith("https://")
                ):
                    raise ReleaseError("Sigrok runtime component provenance is invalid")
                component_id = f"SPDXRef-RuntimeComponent-{runtime_component_index}"
                sbom_packages.append(
                    {
                        "SPDXID": component_id,
                        "downloadLocation": component["source"],
                        "licenseConcluded": component["license"],
                        "name": component["name"],
                        "packageComment": f"runtime component scope={item['scope']} input={item['name']}",
                        "versionInfo": component["version"],
                    }
                )
                relationships.append(
                    {
                        "spdxElementId": input_ids[(str(item["scope"]), str(item["name"]))],
                        "relationshipType": "CONTAINS",
                        "relatedSpdxElement": component_id,
                    }
                )
                runtime_component_index += 1
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
            "packages": sbom_packages,
            "relationships": sorted(
                relationships,
                key=lambda item: (
                    item["spdxElementId"], item["relationshipType"], item["relatedSpdxElement"]
                ),
            ),
            "spdxVersion": "SPDX-2.3",
        }
        _validate_sbom_relationships(sbom)
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
        for relative, contents in sorted(acceptance_files, key=lambda item: item[0].as_posix()):
            destination = temporary / relative
            destination.parent.mkdir(parents=True, exist_ok=True)
            destination.write_bytes(contents)
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
        scan_release_tree(temporary)
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
    try:
        sbom = json.loads((root / "sbom.spdx.json").read_text(encoding="utf-8"))
    except (OSError, UnicodeDecodeError, json.JSONDecodeError) as error:
        raise ReleaseError(f"release SBOM is invalid: {error}") from error
    if not isinstance(sbom, dict):
        raise ReleaseError("release SBOM is invalid")
    _validate_sbom_relationships(sbom)
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
            result = _read_json(evidence_path, "retained acceptance result")
            required_result_fields = {
                "acceptance_runner_sha256", "artifacts", "build_receipts", "check", "command_plan_sha256", "exit_code", "network_after", "network_before",
                "prior_artifacts", "reproduced_artifacts", "schema", "stderr", "stdout", "target", "target_environment",
            }
            if (
                set(result) != required_result_fields
                or result.get("schema") != 1
                or result.get("exit_code") != 0
                or result.get("check") != check.get("check")
                or result.get("target") != check.get("target")
                or result.get("acceptance_runner_sha256") != _acceptance_runner_digest()
                or not re.fullmatch(r"[0-9a-f]{64}", str(result.get("command_plan_sha256", "")))
            ):
                raise ReleaseError("retained acceptance result is invalid")
            if (
                result.get("check") in LIFECYCLE_ACCEPTANCE_CHECKS
                and not isinstance(result.get("target_environment"), dict)
            ):
                raise ReleaseError("retained lifecycle acceptance lacks exact target identity")
            if (
                result.get("check") not in LIFECYCLE_ACCEPTANCE_CHECKS
                and result.get("target_environment") is not None
            ):
                raise ReleaseError("non-lifecycle acceptance unexpectedly claims target identity")
            bindings = result.get("artifacts")
            if not isinstance(bindings, list) or not bindings:
                raise ReleaseError("retained acceptance result lacks artifact bindings")
            for binding in bindings:
                if not isinstance(binding, dict) or set(binding) != {"name", "sha256", "size"}:
                    raise ReleaseError("retained acceptance artifact binding is invalid")
                matches = list(root.glob(f"*/{binding['name']}"))
                if len(matches) != 1:
                    raise ReleaseError("retained acceptance artifact binding is unresolved")
                bound = _require_regular_input(matches[0], "retained acceptance artifact")
                if bound.stat().st_size != binding["size"] or sha256_file(bound) != binding["sha256"]:
                    raise ReleaseError("retained acceptance artifact binding mismatch")
            reproduced_bindings = result.get("reproduced_artifacts")
            if check.get("check") == "build-reproducibility":
                if reproduced_bindings != bindings:
                    raise ReleaseError("retained reproducibility evidence does not bind identical artifacts")
                build_receipts = result.get("build_receipts")
                if (
                    not isinstance(build_receipts, list)
                    or len(build_receipts) != 2
                    or any(not isinstance(receipt, dict) for receipt in build_receipts)
                    or build_receipts[0].get("artifacts") != bindings
                    or build_receipts[1].get("artifacts") != bindings
                    or build_receipts[0].get("invocation_id") == build_receipts[1].get("invocation_id")
                ):
                    raise ReleaseError("retained reproducibility evidence lacks independent build receipts")
            elif reproduced_bindings != []:
                raise ReleaseError("unrelated acceptance result contains reproducibility evidence")
            elif result.get("build_receipts") != []:
                raise ReleaseError("unrelated acceptance result contains build receipts")
            prior_artifacts = result.get("prior_artifacts")
            if check.get("check") in PRIOR_INSTALLER_CHECKS:
                if (
                    not isinstance(prior_artifacts, list)
                    or len(prior_artifacts) != 1
                    or not isinstance(prior_artifacts[0], dict)
                    or set(prior_artifacts[0]) != {"name", "sha256", "size", "version"}
                    or re.fullmatch(r"[0-9a-f]{64}", str(prior_artifacts[0].get("sha256", ""))) is None
                    or SEMVER.fullmatch(str(prior_artifacts[0].get("version", ""))) is None
                    or type(prior_artifacts[0].get("size")) is not int
                    or int(prior_artifacts[0]["size"]) < 1
                ):
                    raise ReleaseError("retained rollback evidence lacks a prior installer binding")
            elif prior_artifacts != []:
                raise ReleaseError("unrelated acceptance result contains a prior installer binding")
            for boundary in ("network_before", "network_after"):
                _validated_offline_observation(lambda value=result[boundary]: value)
            for stream_name in ("stdout", "stderr"):
                stream = result.get(stream_name)
                if not isinstance(stream, dict) or set(stream) != {"path", "sha256", "size"}:
                    raise ReleaseError("retained acceptance output reference is invalid")
                relative = PurePosixPath(str(stream.get("path", "")))
                if relative.is_absolute() or not relative.parts or ".." in relative.parts:
                    raise ReleaseError("retained acceptance output path is unsafe")
                output_path = _require_regular_input(root.joinpath(*relative.parts), "retained acceptance output")
                if output_path.stat().st_size != stream.get("size") or sha256_file(output_path) != stream.get("sha256"):
                    raise ReleaseError("retained acceptance output digest mismatch")
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
    descriptor, temporary_name = tempfile.mkstemp(
        prefix=f".{path.name}.", suffix=".tmp", dir=path.parent
    )
    try:
        with os.fdopen(descriptor, "wb") as stream:
            stream.write(canonical_json(value))
            stream.flush()
            os.fsync(stream.fileno())
        os.replace(temporary_name, path)
        if os.name != "nt":
            directory = os.open(path.parent, os.O_RDONLY)
            try:
                os.fsync(directory)
            finally:
                os.close(directory)
    except Exception:
        Path(temporary_name).unlink(missing_ok=True)
        raise


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
    payload.add_argument("--lock", type=Path, required=True)
    payload.add_argument("--receipt", type=Path)
    deb = subcommands.add_parser("deb", help="build a deterministic Debian package")
    deb.add_argument("--payload", type=Path, required=True)
    deb.add_argument("--output", type=Path, required=True)
    deb.add_argument("--target", choices=sorted(TARGET_ARCHITECTURES), required=True)
    deb.add_argument("--version", required=True)
    deb.add_argument("--source-date-epoch", type=int, required=True)
    deb.add_argument("--offline-receipt", type=Path, required=True)
    deb.add_argument("--lock", type=Path, required=True)
    deb.add_argument("--receipt", type=Path)
    deb.add_argument("--build-invocation")
    deb.add_argument("--source-revision")
    installer = subcommands.add_parser("installer", help="build a self-extracting offline installer")
    installer.add_argument("--package", type=Path, required=True)
    installer.add_argument("--pool", type=Path, required=True)
    installer.add_argument("--output", type=Path, required=True)
    installer.add_argument("--target", choices=sorted(TARGET_ARCHITECTURES), required=True)
    installer.add_argument("--version", required=True)
    installer.add_argument("--source-date-epoch", type=int, required=True)
    installer.add_argument("--offline-receipt", type=Path, required=True)
    installer.add_argument("--lock", type=Path, required=True)
    installer.add_argument("--receipt", type=Path)
    installer.add_argument("--build-invocation")
    installer.add_argument("--source-revision")
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
    image.add_argument("--lock", type=Path, required=True)
    image.add_argument("--receipt", type=Path)
    image.add_argument("--build-invocation")
    image.add_argument("--source-revision")
    build_set = subcommands.add_parser(
        "build-set-receipt", help="close one artifact-build invocation"
    )
    build_set.add_argument("--artifacts", type=Path, required=True)
    build_set.add_argument("--artifact-receipt", type=Path, action="append", required=True)
    build_set.add_argument("--output", type=Path, required=True)
    build_set.add_argument("--lock", type=Path, required=True)
    build_set.add_argument("--source-revision", required=True)
    reproduce = subcommands.add_parser(
        "reproduce", help="execute two fixed fresh builds and verify reproducibility"
    )
    reproduce.add_argument("--artifacts", type=Path, required=True)
    reproduce.add_argument("--build-inputs", type=Path, required=True)
    reproduce.add_argument("--source-revision", required=True)
    reproduce.add_argument("--source-date-epoch", type=int, required=True)
    reproduce.add_argument("--expected-lock-sha256", required=True)
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
                input_lock=options.lock,
            )
        elif options.command == "deb":
            result = build_deb(
                options.payload,
                options.output,
                target=options.target,
                version=options.version,
                source_date_epoch=options.source_date_epoch,
                offline_receipt=options.offline_receipt,
                input_lock=options.lock,
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
                input_lock=options.lock,
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
                input_lock=options.lock,
            )
        elif options.command == "build-set-receipt":
            result = create_build_set_receipt(
                options.artifacts,
                options.artifact_receipt,
                options.output,
                lock_path=options.lock,
                source_revision=options.source_revision,
            )
        elif options.command == "reproduce":
            result = verify_reproducible_candidate(
                options.artifacts,
                options.build_inputs,
                source_revision=options.source_revision,
                source_date_epoch=options.source_date_epoch,
                expected_lock_sha256=options.expected_lock_sha256,
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
        if getattr(options, "receipt", None) is not None:
            result = _bind_artifact_builder_receipt(result, options)
        _write_receipt(getattr(options, "receipt", None), result)
        print(json.dumps(result, sort_keys=True))
        return 0
    except (OSError, ReleaseError, tarfile.TarError) as error:
        print(f"release builder refused: {error}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
