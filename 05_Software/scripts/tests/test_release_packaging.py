#!/usr/bin/env python3
"""Verify deterministic, offline NILO delivery artifacts."""

from __future__ import annotations

import importlib.util
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
import unittest
from pathlib import Path


REPOSITORY_ROOT = Path(__file__).resolve().parents[3]
PACKAGING_ROOT = REPOSITORY_ROOT / "02_Tools" / "packaging"
MODULE_PATH = PACKAGING_ROOT / "release_builder.py"
SIGROK_BUILDER_PATH = REPOSITORY_ROOT / "02_Tools" / "scripts" / "build-sigrok-bundle.py"


def load_builder():
    """Load the release builder from its repository path."""
    if not MODULE_PATH.is_file():
        raise AssertionError(f"the release builder is required at {MODULE_PATH}")
    spec = importlib.util.spec_from_file_location("nilo_release_builder", MODULE_PATH)
    if spec is None or spec.loader is None:
        raise AssertionError(f"cannot load {MODULE_PATH}")
    module = importlib.util.module_from_spec(spec)
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


def load_sigrok_builder():
    """Load the target-runtime Sigrok bundle builder."""
    spec = importlib.util.spec_from_file_location("nilo_sigrok_bundle_builder", SIGROK_BUILDER_PATH)
    if spec is None or spec.loader is None:
        raise AssertionError(f"cannot load {SIGROK_BUILDER_PATH}")
    module = importlib.util.module_from_spec(spec)
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


def fixture_payload(root: Path) -> Path:
    """Create the smallest representative immutable filesystem payload."""
    source = root / "source"
    files = {
        "usr/bin/nilo": b"nilo-binary\n",
        "usr/libexec/nilo/nilo-nethelper": b"helper-binary\n",
        "usr/share/nilo/gui/index.html": b"<title>NILO</title>\n",
        "usr/lib/nilo/integration-bundles/sigrok/manifest.json": b'{"schema":1}\n',
        "usr/lib/udev/rules.d/60-nilo-instruments.rules": b'SUBSYSTEM=="video4linux", GROUP="video", MODE="0660"\n',
        "etc/nilo/nilo.env": b"NILO_GPIO_RECOVERY=disabled\n",
    }
    for relative, contents in files.items():
        path = source / relative
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_bytes(contents)
        if relative in {"usr/bin/nilo", "usr/libexec/nilo/nilo-nethelper"}:
            path.chmod(0o755)
    return source


def offline_receipt(
    root: Path,
    target: str,
    lock_path: Path | None = None,
    admitted_paths: list[Path] | None = None,
) -> Path:
    architecture = "arm64" if target == "rpi-debian13-arm64" else "amd64"
    path = root / f"{target}-preparation.json"
    cache_root = root / "packaging-cache"
    inputs = []
    declared = []
    if lock_path:
        lock = json.loads(lock_path.read_text(encoding="utf-8"))
        for item in lock["inputs"]:
            if item["scope"] not in {"shared", target}:
                continue
            fixture = lock_path.parent / "lock-fixtures" / f"{item['scope']}--{item['name']}"
            contents = fixture.read_bytes() if fixture.is_file() else f"{item['scope']}/{item['name']}".encode()
            declared.append((item["name"], contents, {key: value for key, value in item.items() if key != "url"}))
    else:
        declared.append(("fixture", f"verified input for {target}\n".encode(), None))
    declared.extend((source.name, source.read_bytes(), None) for source in admitted_paths or [])
    for name, contents, locked in declared:
        digest = hashlib.sha256(contents).hexdigest()
        cache_object = Path("objects") / "sha256" / digest[:2] / digest
        object_path = cache_root / cache_object
        object_path.parent.mkdir(parents=True, exist_ok=True)
        object_path.write_bytes(contents)
        record = (
            locked
            if locked is not None
            else {
                "apt": None,
                "architecture": architecture,
                "artifact_type": "fixture",
                "license": "MIT",
                "name": name,
                "provenance": "test fixture",
                "redistribution": "allowed",
                "scope": target,
                "sha256": digest,
                "size": len(contents),
                "version": "1",
            }
        )
        record = {**record, "cache_object": cache_object.as_posix()}
        if record["sha256"] != digest or record["size"] != len(contents):
            raise AssertionError("test lock does not describe its fixture bytes")
        inputs.append(record)
    if lock_path is None:
        lock_path = offline_lock(root)
        locked_inputs = [
            {
                **{key: value for key, value in item.items() if key != "cache_object"},
                "url": f"https://example.invalid/fixtures/{item['scope']}/{item['name']}",
            }
            for item in inputs
        ]
        lock_path.write_text(
            json.dumps(
                {
                    "complete": True,
                    "incomplete_reasons": [],
                    "inputs": locked_inputs,
                    "schema": 1,
                },
                indent=2,
                sort_keys=True,
            )
            + "\n",
            encoding="utf-8",
        )
    lock_digest = hashlib.sha256(lock_path.read_bytes()).hexdigest()
    path.write_text(
        json.dumps(
            {
                "cache_root": str(cache_root.resolve()),
                "inputs": inputs,
                "lock_sha256": lock_digest,
                "offline": {
                    "interfaces": ["lo"],
                    "ipv4_default_route": False,
                    "ipv6_default_route": False,
                    "platform": "linux",
                    "verified": True,
                },
                "schema": 1,
                "target": {"architecture": architecture, "id": target},
            },
            indent=2,
            sort_keys=True,
        )
        + "\n",
        encoding="utf-8",
    )
    return path


def offline_lock(root: Path) -> Path:
    """Return the explicit trusted fixture lock paired with an offline receipt."""
    return root / "fixture-inputs.lock.json"


def read_ar(path: Path) -> dict[str, bytes]:
    """Read the simple System V ar form emitted for Debian packages."""
    data = path.read_bytes()
    if not data.startswith(b"!<arch>\n"):
        raise AssertionError("not an ar archive")
    offset = 8
    members: dict[str, bytes] = {}
    while offset < len(data):
        header = data[offset : offset + 60]
        if len(header) != 60 or header[58:] != b"`\n":
            raise AssertionError("invalid ar member header")
        name = header[:16].decode("ascii").rstrip().rstrip("/")
        size = int(header[48:58].decode("ascii").strip())
        offset += 60
        members[name] = data[offset : offset + size]
        offset += size + (size % 2)
    return members


def elf_fixture(architecture: str) -> bytes:
    header = bytearray(64)
    header[:6] = b"\x7fELF\x02\x01"
    header[18:20] = (62 if architecture == "amd64" else 183).to_bytes(2, "little")
    return bytes(header) + b"runtime fixture payload"


def dependency_deb(builder, architecture: str) -> bytes:
    """Build the signed-pool dependency fixture embedded by each installer."""
    control = (
        f"Package: fixture-dependency\nVersion: 1.0\nArchitecture: {architecture}\n"
        "Description: release dependency fixture\n"
    ).encode()
    return (
        b"!<arch>\n"
        + builder._ar_member("debian-binary", b"2.0\n", 1_700_000_000)
        + builder._ar_member(
            "control.tar.xz",
            builder._archive_bytes([("./control", control, 0o644)], 1_700_000_000),
            1_700_000_000,
        )
        + builder._ar_member(
            "data.tar.xz", builder._archive_bytes([], 1_700_000_000), 1_700_000_000
        )
    )


def write_runtime_manifest(runtime_root: Path, architecture: str) -> None:
    files = sorted([
        {
            "path": path.relative_to(runtime_root).as_posix(),
            "sha256": hashlib.sha256(path.read_bytes()).hexdigest(),
            "size": path.stat().st_size,
        }
        for path in sorted(runtime_root.rglob("*"))
        if path.is_file() and path.name != "runtime-manifest.json"
    ], key=lambda item: item["path"])
    manifest = {
        "architecture": architecture,
        "components": [
            {
                "license": "GPL-3.0-or-later",
                "name": "sigrok-runtime-fixture",
                "source": "https://example.invalid/sigrok-source",
                "version": "1.0",
            }
        ],
        "files": files,
        "health_checks": [
            {"arguments": ["bin/python3", "--version"], "id": "python-version"},
            {"arguments": ["bin/sigrok-cli", "--version"], "id": "sigrok-version"},
            {"arguments": ["bin/sigrok-cli", "--scan"], "id": "sigrok-driver-scan"},
        ],
        "schema": 1,
    }
    (runtime_root / "runtime-manifest.json").write_text(
        json.dumps(manifest, indent=2, sort_keys=True) + "\n", encoding="utf-8"
    )


class DeterministicPayloadTests(unittest.TestCase):
    def test_offline_receipt_revalidates_admitted_cache_bytes(self) -> None:
        builder = load_builder()
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            source = fixture_payload(root)
            receipt = offline_receipt(root, "ubuntu-24.04-amd64")
            record = json.loads(receipt.read_text(encoding="utf-8"))
            cache_object = Path(record["cache_root"]) / record["inputs"][0]["cache_object"]
            cache_object.write_bytes(b"x" * cache_object.stat().st_size)
            with self.assertRaisesRegex(builder.ReleaseError, "cache object digest mismatch"):
                builder.build_payload(
                    source,
                    root / "payload.tar.xz",
                    target="ubuntu-24.04-amd64",
                    version="0.8.0",
                    source_date_epoch=1,
                    offline_receipt=receipt,
                    input_lock=offline_lock(root),
                )

    def test_payload_is_byte_reproducible_and_manifested(self) -> None:
        builder = load_builder()
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            source = fixture_payload(root)
            first = root / "first.tar.xz"
            second = root / "second.tar.xz"
            first_receipt = builder.build_payload(
                source,
                first,
                target="ubuntu-24.04-amd64",
                version="0.8.0",
                source_date_epoch=1_700_000_000,
                offline_receipt=offline_receipt(root, "ubuntu-24.04-amd64"),
                input_lock=offline_lock(root),
            )
            os.utime(source / "usr/bin/nilo", (1_800_000_000, 1_800_000_000))
            second_receipt = builder.build_payload(
                source,
                second,
                target="ubuntu-24.04-amd64",
                version="0.8.0",
                source_date_epoch=1_700_000_000,
                offline_receipt=offline_receipt(root, "ubuntu-24.04-amd64"),
                input_lock=offline_lock(root),
            )
            self.assertEqual(first.read_bytes(), second.read_bytes())
            self.assertEqual(first_receipt["sha256"], second_receipt["sha256"])
            self.assertEqual(first_receipt["target"], "ubuntu-24.04-amd64")
            self.assertEqual(first_receipt["version"], "0.8.0")

            with tarfile.open(first, "r:xz") as archive:
                names = archive.getnames()
                self.assertEqual(names, sorted(names))
                manifest = json.load(archive.extractfile("payload-manifest.json"))
                binary = archive.getmember("payload/usr/bin/nilo")
                self.assertEqual(stat.S_IMODE(binary.mode), 0o755)
                self.assertEqual(binary.uid, 0)
                self.assertEqual(binary.gid, 0)
                self.assertEqual(binary.mtime, 1_700_000_000)
            self.assertEqual(manifest["schema"], 1)
            self.assertEqual(manifest["target"], "ubuntu-24.04-amd64")
            self.assertTrue(all(item["sha256"] for item in manifest["files"]))

    def test_payload_refuses_links_special_files_and_mutable_state(self) -> None:
        builder = load_builder()
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            source = fixture_payload(root)
            mutable = source / "var/lib/nilo/nilo.db"
            mutable.parent.mkdir(parents=True)
            mutable.write_bytes(b"state")
            with self.assertRaisesRegex(builder.ReleaseError, "mutable state"):
                builder.build_payload(
                    source,
                    root / "payload.tar.xz",
                    target="ubuntu-24.04-amd64",
                    version="0.8.0",
                    source_date_epoch=1,
                    offline_receipt=offline_receipt(root, "ubuntu-24.04-amd64"),
                    input_lock=offline_lock(root),
                )
            mutable.unlink()
            link = source / "usr/share/nilo/gui/escape"
            try:
                link.symlink_to(root / "outside")
            except OSError as error:
                self.skipTest(f"symlinks unavailable: {error}")
            with self.assertRaisesRegex(builder.ReleaseError, "symbolic link"):
                builder.build_payload(
                    source,
                    root / "payload.tar.xz",
                    target="ubuntu-24.04-amd64",
                    version="0.8.0",
                    source_date_epoch=1,
                    offline_receipt=offline_receipt(root, "ubuntu-24.04-amd64"),
                    input_lock=offline_lock(root),
                )

    def test_release_scanner_names_forbidden_production_content(self) -> None:
        builder = load_builder()
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            clean = fixture_payload(root)
            builder.scan_release_tree(clean)
            forbidden = clean / "usr/lib/nilo/integration-bundles/sigrok/fake_python"
            forbidden.write_text("test fixture", encoding="utf-8")
            with self.assertRaisesRegex(builder.ReleaseError, "test-runtime"):
                builder.scan_release_tree(clean)

    def test_release_scanner_checks_large_and_binary_files(self) -> None:
        builder = load_builder()
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            source = fixture_payload(root)
            large = source / "usr/share/nilo/gui/large.dat"
            large.write_bytes(b"x" * (4 * 1024 * 1024 + 1) + b"admin_password")
            with self.assertRaisesRegex(builder.ReleaseError, "development-credential"):
                builder.scan_release_tree(source)
            large.unlink()
            binary = source / "usr/bin/embedded"
            binary.write_bytes(b"\x00prefix /home/release-user/private suffix")
            with self.assertRaisesRegex(builder.ReleaseError, "host-path"):
                builder.scan_release_tree(source)


class DebianPackageTests(unittest.TestCase):
    def test_debian_package_is_deterministic_and_policy_safe(self) -> None:
        builder = load_builder()
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            source = fixture_payload(root)
            payload = root / "payload.tar.xz"
            builder.build_payload(
                source,
                payload,
                target="ubuntu-24.04-amd64",
                version="0.8.0",
                source_date_epoch=1_700_000_000,
                offline_receipt=offline_receipt(root, "ubuntu-24.04-amd64"),
                input_lock=offline_lock(root),
            )
            first = root / "nilo-first.deb"
            second = root / "nilo-second.deb"
            builder.build_deb(
                payload,
                first,
                target="ubuntu-24.04-amd64",
                version="0.8.0",
                source_date_epoch=1_700_000_000,
                offline_receipt=offline_receipt(root, "ubuntu-24.04-amd64"),
                input_lock=offline_lock(root),
            )
            builder.build_deb(
                payload,
                second,
                target="ubuntu-24.04-amd64",
                version="0.8.0",
                source_date_epoch=1_700_000_000,
                offline_receipt=offline_receipt(root, "ubuntu-24.04-amd64"),
                input_lock=offline_lock(root),
            )
            self.assertEqual(first.read_bytes(), second.read_bytes())
            members = read_ar(first)
            self.assertEqual(
                list(members), ["debian-binary", "control.tar.xz", "data.tar.xz"]
            )
            self.assertEqual(members["debian-binary"], b"2.0\n")
            with tarfile.open(fileobj=io.BytesIO(members["control.tar.xz"]), mode="r:xz") as control:
                names = control.getnames()
                self.assertEqual(names, sorted(names))
                metadata = control.extractfile("./control").read().decode("utf-8")
                postinst = control.extractfile("./postinst").read().decode("utf-8")
                prerm = control.extractfile("./prerm").read().decode("utf-8")
                conffiles = control.extractfile("./conffiles").read().decode("utf-8")
            self.assertIn("Package: nilo\n", metadata)
            self.assertIn("Version: 0.8.0\n", metadata)
            self.assertIn("Architecture: amd64\n", metadata)
            self.assertIn("policy-rc.d", postinst)
            self.assertIn("adduser nilo video", postinst)
            self.assertIn("maintenance remove", prerm)
            self.assertLess(prerm.index("maintenance remove"), prerm.index("stop nilo-nethelper.service"))
            self.assertIn("/etc/nilo/nilo.env", conffiles)
            with tarfile.open(fileobj=io.BytesIO(members["data.tar.xz"]), mode="r:xz") as data:
                names = data.getnames()
                self.assertEqual(names, sorted(names))
                self.assertIn("./usr/bin/nilo", names)
                self.assertIn("./usr/lib/udev/rules.d/60-nilo-instruments.rules", names)
                self.assertIn("./usr/lib/systemd/system/nilo.service", names)
                self.assertIn("./usr/lib/systemd/system/nilo-nethelper.socket", names)
                self.assertIn("./usr/lib/systemd/system/nilo-firstboot.service", names)
                self.assertIn("./usr/lib/systemd/system/nilo-image-firstboot.service", names)
                self.assertIn("./usr/libexec/nilo/nilo-installer-recover", names)
                self.assertIn("./usr/libexec/nilo/nilo-image-firstboot", names)
                self.assertNotIn("./var/lib/nilo/nilo.db", names)
                core_unit = data.extractfile("./usr/lib/systemd/system/nilo.service").read().decode("utf-8")
                helper_unit = data.extractfile("./usr/lib/systemd/system/nilo-nethelper.service").read().decode("utf-8")
                recovery = data.extractfile("./usr/libexec/nilo/nilo-installer-recover").read().decode("utf-8")
                recovery_unit = data.extractfile("./usr/lib/systemd/system/nilo-recovery.service").read().decode("utf-8")
            self.assertNotIn("--management-port", core_unit)
            self.assertIn("--local-status-bind 127.0.0.1:8091", core_unit)
            self.assertIn("Requires=nilo-image-firstboot.service", core_unit)
            self.assertIn("--allow-uid ${NILO_CALLER_UID}", helper_unit)
            self.assertIn("--socket-gid ${NILO_CALLER_GID}", helper_unit)
            self.assertIn("maintenance restore", recovery)
            self.assertIn("dpkg -i", recovery)
            self.assertIn("had_prior_nilo", recovery)
            self.assertIn('sync "$journal.tmp"\n  mv -f "$journal.tmp" "$journal"', recovery)
            recovery_rollback = recovery[recovery.index("ok=1") :]
            self.assertLess(recovery_rollback.index("systemctl stop"), recovery_rollback.index("dpkg -i"))
            self.assertIn('[ "$had_prior_nilo" -ne 1 ] || systemctl start', recovery)
            self.assertIn("nilo-installer-recover", recovery_unit)
            self.assertIn("/var/lib/dpkg", recovery_unit)
            if shutil.which("dpkg-deb"):
                inspected = subprocess.run(
                    ["dpkg-deb", "--info", str(first)],
                    text=True,
                    capture_output=True,
                    check=False,
                )
                self.assertEqual(inspected.returncode, 0, inspected.stderr)

    def test_debian_package_refuses_target_or_payload_mismatch(self) -> None:
        builder = load_builder()
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            payload = root / "payload.tar.xz"
            builder.build_payload(
                fixture_payload(root),
                payload,
                target="ubuntu-24.04-amd64",
                version="0.8.0",
                source_date_epoch=1,
                offline_receipt=offline_receipt(root, "ubuntu-24.04-amd64"),
                input_lock=offline_lock(root),
            )
            with self.assertRaisesRegex(builder.ReleaseError, "target mismatch"):
                builder.build_deb(
                    payload,
                    root / "wrong.deb",
                    target="rpi-debian13-arm64",
                    version="0.8.0",
                    source_date_epoch=1,
                    offline_receipt=offline_receipt(root, "rpi-debian13-arm64"),
                    input_lock=offline_lock(root),
                )

    def test_debian_package_refuses_payload_bytes_not_bound_by_the_manifest(self) -> None:
        builder = load_builder()
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            payload = root / "payload.tar.xz"
            builder.build_payload(
                fixture_payload(root),
                payload,
                target="ubuntu-24.04-amd64",
                version="0.8.0",
                source_date_epoch=1,
                offline_receipt=offline_receipt(root, "ubuntu-24.04-amd64"),
                input_lock=offline_lock(root),
            )
            with tarfile.open(payload, "r:xz") as source_archive:
                members = source_archive.getmembers()
                contents = {
                    member.name: source_archive.extractfile(member).read()
                    for member in members
                    if member.isfile()
                }
            contents["payload/usr/bin/nilo"] += b"tampered"
            with tarfile.open(payload, "w:xz", format=tarfile.PAX_FORMAT) as changed:
                for member in members:
                    if member.isfile():
                        member.size = len(contents[member.name])
                        changed.addfile(member, io.BytesIO(contents[member.name]))
                    else:
                        changed.addfile(member)
            with self.assertRaisesRegex(builder.ReleaseError, "payload manifest digest mismatch"):
                builder.build_deb(
                    payload,
                    root / "tampered.deb",
                    target="ubuntu-24.04-amd64",
                    version="0.8.0",
                    source_date_epoch=1,
                    offline_receipt=offline_receipt(root, "ubuntu-24.04-amd64"),
                    input_lock=offline_lock(root),
                )

    def test_release_verifier_rejects_replaced_debian_maintainer_logic(self) -> None:
        builder = load_builder()
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            payload = root / "payload.tar.xz"
            builder.build_payload(
                fixture_payload(root), payload,
                target="ubuntu-24.04-amd64", version="0.8.0",
                source_date_epoch=1_700_000_000,
                offline_receipt=offline_receipt(root, "ubuntu-24.04-amd64"),
                input_lock=offline_lock(root),
            )
            package = root / "nilo_0.8.0_amd64.deb"
            builder.build_deb(
                payload, package,
                target="ubuntu-24.04-amd64", version="0.8.0",
                source_date_epoch=1_700_000_000,
                offline_receipt=offline_receipt(root, "ubuntu-24.04-amd64"),
                input_lock=offline_lock(root),
            )
            members = read_ar(package)
            with tarfile.open(fileobj=io.BytesIO(members["control.tar.xz"]), mode="r:xz") as archive:
                entries = [
                    (
                        member.name,
                        b"#!/bin/sh\nexit 0\n" if member.name == "./postinst" else archive.extractfile(member).read(),
                        member.mode,
                    )
                    for member in archive.getmembers()
                ]
            changed_control = builder._archive_bytes(entries, 1_700_000_000)
            package.write_bytes(
                b"!<arch>\n"
                + builder._ar_member("debian-binary", members["debian-binary"], 1_700_000_000)
                + builder._ar_member("control.tar.xz", changed_control, 1_700_000_000)
                + builder._ar_member("data.tar.xz", members["data.tar.xz"], 1_700_000_000)
            )
            with self.assertRaisesRegex(builder.ReleaseError, "maintainer metadata"):
                builder._verify_release_artifact_structure(
                    package, "ubuntu-24.04-amd64", "0.8.0"
                )


class OfflineInstallerTests(unittest.TestCase):
    def _dependency_deb(self, builder, path: Path) -> None:
        epoch = 1_700_000_000
        control = (
            b"Package: dependency\nVersion: 1.0\nArchitecture: amd64\n"
            b"Description: installer fixture\n"
        )
        control_archive = builder._archive_bytes(
            [("./control", control, 0o644)], epoch
        )
        data_archive = builder._archive_bytes([], epoch)
        path.write_bytes(
            b"!<arch>\n"
            + builder._ar_member("debian-binary", b"2.0\n", epoch)
            + builder._ar_member("control.tar.xz", control_archive, epoch)
            + builder._ar_member("data.tar.xz", data_archive, epoch)
        )

    def _deb(self, builder, root: Path) -> Path:
        payload = root / "payload.tar.xz"
        builder.build_payload(
            fixture_payload(root),
            payload,
            target="ubuntu-24.04-amd64",
            version="0.8.0",
            source_date_epoch=1_700_000_000,
            offline_receipt=offline_receipt(root, "ubuntu-24.04-amd64"),
            input_lock=offline_lock(root),
        )
        package = root / "nilo_0.8.0_amd64.deb"
        builder.build_deb(
            payload,
            package,
            target="ubuntu-24.04-amd64",
            version="0.8.0",
            source_date_epoch=1_700_000_000,
            offline_receipt=offline_receipt(root, "ubuntu-24.04-amd64"),
            input_lock=offline_lock(root),
        )
        return package

    def test_offline_installer_is_self_verifying_and_deterministic(self) -> None:
        builder = load_builder()
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            package = self._deb(builder, root)
            pool = root / "pool"
            pool.mkdir()
            self._dependency_deb(builder, pool / "dependency_1.0_amd64.deb")
            first = root / "nilo-first.run"
            second = root / "nilo-second.run"
            for output in (first, second):
                builder.build_installer(
                    package,
                    pool,
                    output,
                    target="ubuntu-24.04-amd64",
                    version="0.8.0",
                    source_date_epoch=1_700_000_000,
                    offline_receipt=offline_receipt(
                        root, "ubuntu-24.04-amd64", admitted_paths=list(pool.iterdir())
                    ),
                    input_lock=offline_lock(root),
                )
            self.assertEqual(first.read_bytes(), second.read_bytes())
            if os.name != "nt":
                self.assertTrue(first.stat().st_mode & stat.S_IXUSR)
            launcher, archive = first.read_bytes().split(
                b"__NILO_PAYLOAD_BELOW__\n", 1
            )
            launcher_text = launcher.decode("utf-8")
            self.assertIn(builder.sha256_bytes(archive), launcher_text)
            self.assertIn("archive member path is unsafe", launcher_text)
            self.assertNotIn("http://", launcher_text)
            self.assertNotIn("https://", launcher_text)
            with tarfile.open(fileobj=io.BytesIO(archive), mode="r:xz") as payload:
                names = payload.getnames()
                self.assertEqual(names, sorted(names))
                self.assertEqual(
                    names,
                    [
                        "install.sh",
                        "manifest.json",
                        "packages/dependency_1.0_amd64.deb",
                        "packages/nilo_0.8.0_amd64.deb",
                    ],
                )
                manifest = json.load(payload.extractfile("manifest.json"))
                driver = payload.extractfile("install.sh").read().decode("utf-8")
            self.assertEqual(manifest["target"], "ubuntu-24.04-amd64")
            self.assertTrue(all(item["sha256"] for item in manifest["packages"]))
            self.assertIn("--no-download", driver)
            self.assertIn("--reinstall", driver)
            self.assertIn("export LC_ALL=C", driver)
            self.assertIn("flock -n", driver)
            self.assertIn("is-active NetworkManager", driver)
            self.assertIn("Acquire::http::Proxy=false", driver)
            self.assertIn("dpkg --purge", driver)
            self.assertIn("systemctl is-active", driver)
            self.assertIn("maintenance snapshot", driver)
            self.assertIn("maintenance restore", driver)
            self.assertIn("maintenance migrate", driver)
            self.assertNotIn("maintenance inventory", driver)
            self.assertIn("had_prior_nilo", driver)
            self.assertNotIn("/var/cache/apt/archives", driver)
            self.assertIn("changed-packages.list", driver)
            self.assertIn("repack-tool", driver)
            self.assertNotIn("command -v dpkg-repack", driver)
            self.assertIn("had_prior_data_dir", driver)
            self.assertIn('rm -rf -- "$root/var/lib/nilo"', driver)
            self.assertIn('sync "$txdir/transaction.json" "$txdir"', driver)
            self.assertIn("curl --fail --silent --show-error", driver)
            self.assertIn("[ -S /run/nilo/nethelper.sock ]", driver)
            rollback = driver[driver.index("rollback() {") : driver.index("phase preflight")]
            self.assertLess(rollback.index("systemctl stop"), rollback.index('dpkg -i "$@"'))
            self.assertIn('[ "$had_prior_nilo" -ne 1 ] || systemctl start', driver)
            for phase in (
                "preflight",
                "quiesced",
                "snapshotted",
                "dependencies",
                "package",
                "migrated",
                "started",
                "verified",
                "committed",
                "rolling_back",
                "rolled_back",
                "recovery-required",
            ):
                self.assertIn(phase, driver)

    @unittest.skipIf(os.name == "nt", "recovery helper execution requires a POSIX host")
    def test_recovery_helper_executes_underscore_and_clean_terminal_phases(self) -> None:
        recovery = PACKAGING_ROOT / "debian" / "nilo-installer-recover"
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            stub_root = root / "stubs"
            stub_root.mkdir()
            systemctl = stub_root / "systemctl"
            systemctl.write_text(
                "#!/bin/sh\n[ \"${1:-}\" != is-active ]\n", encoding="utf-8"
            )
            systemctl.chmod(0o755)

            for phase in ("rolling_back", "rolled_back"):
                with self.subTest(phase=phase):
                    state = root / "var" / "lib" / "nilo-installer"
                    transaction = state / "transactions" / f"install-{phase}"
                    transaction.mkdir(parents=True)
                    state.chmod(0o700)
                    (state / "transactions").chmod(0o700)
                    transaction.chmod(0o700)
                    data = root / "var" / "lib" / "nilo"
                    data.mkdir(parents=True, exist_ok=True)
                    (data / "candidate-state").write_text("new\n", encoding="utf-8")
                    metadata = {}
                    for name in (
                        "prior-packages.list", "new-packages.list", "changed-packages.list"
                    ):
                        path = transaction / name
                        path.write_bytes(b"")
                        path.chmod(0o600)
                        metadata[name] = hashlib.sha256(b"").hexdigest()
                    journal = transaction / "transaction.json"
                    journal.write_text(
                        json.dumps(
                            {
                                "schema": 4,
                                "phase": phase,
                                "target": "ubuntu-24.04-amd64",
                                "version": "0.8.0",
                                "had_prior_nilo": 0,
                                "had_prior_data_dir": 0,
                                "prior_debs": str(transaction / "prior-packages.list"),
                                "prior_debs_sha256": metadata["prior-packages.list"],
                                "new_packages": str(transaction / "new-packages.list"),
                                "new_packages_sha256": metadata["new-packages.list"],
                                "changed_packages": str(transaction / "changed-packages.list"),
                                "changed_packages_sha256": metadata["changed-packages.list"],
                                "snapshot": str(data / "snapshots" / transaction.name),
                            },
                            separators=(",", ":"),
                            sort_keys=True,
                        )
                        + "\n",
                        encoding="utf-8",
                    )
                    journal.chmod(0o600)
                    environment = dict(os.environ)
                    environment.update(
                        {
                            "NILO_ACCEPTANCE_DISPOSABLE_TARGET": "test-fixture",
                            "NILO_RECOVERY_ROOT": str(root),
                            "PATH": f"{stub_root}{os.pathsep}{environment['PATH']}",
                        }
                    )
                    completed = subprocess.run(
                        ["sh", str(recovery)],
                        capture_output=True,
                        text=True,
                        check=False,
                        env=environment,
                    )
                    self.assertEqual(completed.returncode, 0, completed.stderr)
                    self.assertFalse(data.exists())
                    retired = state / "completed" / transaction.name / "transaction.json"
                    self.assertTrue(retired.is_file())
                    recovered = json.loads(retired.read_text(encoding="utf-8"))
                    self.assertEqual(recovered["phase"], "rolled_back")
                    data.mkdir(parents=True)
                    (data / "later-valid-state").write_text("keep\n", encoding="utf-8")
                    repeated = subprocess.run(
                        ["sh", str(recovery)],
                        capture_output=True,
                        text=True,
                        check=False,
                        env=environment,
                    )
                    self.assertEqual(repeated.returncode, 0, repeated.stderr)
                    self.assertTrue((data / "later-valid-state").is_file())
                    shutil.rmtree(state)

    def test_installer_archive_rejects_duplicate_members(self) -> None:
        builder = load_builder()
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            package = self._deb(builder, root)
            output = root / "nilo.run"
            pool = root / "pool"
            pool.mkdir()
            builder.build_installer(
                package,
                pool,
                output,
                target="ubuntu-24.04-amd64",
                version="0.8.0",
                source_date_epoch=1_700_000_000,
                offline_receipt=offline_receipt(root, "ubuntu-24.04-amd64"),
                input_lock=offline_lock(root),
            )
            _, archive = output.read_bytes().split(b"__NILO_PAYLOAD_BELOW__\n", 1)
            entries = []
            with tarfile.open(fileobj=io.BytesIO(archive), mode="r:xz") as payload:
                for member in payload.getmembers():
                    entries.append((member.name, payload.extractfile(member).read(), member.mode))
            entries.append(entries[0])
            duplicated = builder._archive_bytes(entries, 1_700_000_000)
            output.write_bytes(builder._installer_launcher(builder.sha256_bytes(duplicated)) + duplicated)
            with self.assertRaisesRegex(builder.ReleaseError, "duplicate"):
                builder._verify_installer_structure(output, "ubuntu-24.04-amd64", "0.8.0")

    @unittest.skipIf(os.name == "nt", "generated POSIX launcher requires a POSIX host")
    def test_generated_installer_launcher_rejects_duplicate_members_before_extraction(self) -> None:
        builder = load_builder()
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            package = self._deb(builder, root)
            output = root / "nilo.run"
            pool = root / "pool"
            pool.mkdir()
            builder.build_installer(
                package,
                pool,
                output,
                target="ubuntu-24.04-amd64",
                version="0.8.0",
                source_date_epoch=1_700_000_000,
                offline_receipt=offline_receipt(root, "ubuntu-24.04-amd64"),
                input_lock=offline_lock(root),
            )
            launcher, archive = output.read_bytes().split(b"__NILO_PAYLOAD_BELOW__\n", 1)
            entries = []
            with tarfile.open(fileobj=io.BytesIO(archive), mode="r:xz") as payload:
                for member in payload.getmembers():
                    entries.append((member.name, payload.extractfile(member).read(), member.mode))
            entries.append(entries[0])
            duplicated = builder._archive_bytes(entries, 1_700_000_000)
            launcher_text = launcher.decode("utf-8")
            launcher_text = re.sub(
                r"expected_sha256='[0-9a-f]{64}'",
                f"expected_sha256='{builder.sha256_bytes(duplicated)}'",
                launcher_text,
                count=1,
            )
            tampered = root / "duplicated.run"
            tampered.write_bytes(launcher_text.encode("utf-8") + b"__NILO_PAYLOAD_BELOW__\n" + duplicated)
            tampered.chmod(0o755)
            completed = subprocess.run(
                [str(tampered), "--inspect"], capture_output=True, text=True, check=False
            )
            self.assertNotEqual(completed.returncode, 0)
            self.assertIn("duplicate", completed.stderr)

    def test_installer_structure_rejects_a_replaced_root_driver(self) -> None:
        builder = load_builder()
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            package = self._deb(builder, root)
            pool = root / "pool"
            pool.mkdir()
            output = root / "nilo.run"
            builder.build_installer(
                package,
                pool,
                output,
                target="ubuntu-24.04-amd64",
                version="0.8.0",
                source_date_epoch=1_700_000_000,
                offline_receipt=offline_receipt(root, "ubuntu-24.04-amd64"),
                input_lock=offline_lock(root),
            )
            _, archive = output.read_bytes().split(b"__NILO_PAYLOAD_BELOW__\n", 1)
            entries = []
            with tarfile.open(fileobj=io.BytesIO(archive), mode="r:xz") as payload:
                for member in payload.getmembers():
                    contents = payload.extractfile(member).read()
                    if member.name == "install.sh":
                        contents = b"#!/bin/sh\nexit 0\n"
                    entries.append((member.name, contents, member.mode))
            changed = builder._archive_bytes(entries, 1_700_000_000)
            output.write_bytes(builder._installer_launcher(builder.sha256_bytes(changed)) + changed)
            with self.assertRaisesRegex(builder.ReleaseError, "installer driver"):
                builder._verify_installer_structure(
                    output, "ubuntu-24.04-amd64", "0.8.0"
                )

    def test_offline_installer_refuses_mismatched_or_unsafe_inputs(self) -> None:
        builder = load_builder()
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            package = self._deb(builder, root)
            pool = root / "pool"
            pool.mkdir()
            with self.assertRaisesRegex(builder.ReleaseError, "architecture mismatch"):
                builder.build_installer(
                    package,
                    pool,
                    root / "wrong.run",
                    target="rpi-debian13-arm64",
                    version="0.8.0",
                    source_date_epoch=1,
                    offline_receipt=offline_receipt(root, "rpi-debian13-arm64"),
                    input_lock=offline_lock(root),
                )
            outside = root / "outside.deb"
            outside.write_bytes(b"outside")
            try:
                (pool / "linked.deb").symlink_to(outside)
            except OSError as error:
                self.skipTest(f"symlinks unavailable: {error}")
            with self.assertRaisesRegex(builder.ReleaseError, "regular non-symlink"):
                builder.build_installer(
                    package,
                    pool,
                    root / "unsafe.run",
                    target="ubuntu-24.04-amd64",
                    version="0.8.0",
                    source_date_epoch=1,
                    offline_receipt=offline_receipt(root, "ubuntu-24.04-amd64"),
                    input_lock=offline_lock(root),
                )

    def test_offline_installer_refuses_shell_active_package_names(self) -> None:
        builder = load_builder()
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            package = self._deb(builder, root)
            pool = root / "pool"
            pool.mkdir()
            (pool / "dependency_'_injection.deb").write_bytes(b"unsafe-name")
            with self.assertRaisesRegex(builder.ReleaseError, "unsafe Debian package name"):
                builder.build_installer(
                    package,
                    pool,
                    root / "unsafe.run",
                    target="ubuntu-24.04-amd64",
                    version="0.8.0",
                    source_date_epoch=1,
                    offline_receipt=offline_receipt(root, "ubuntu-24.04-amd64"),
                    input_lock=offline_lock(root),
                )

    def test_offline_installer_refuses_dependency_outside_admitted_cache(self) -> None:
        builder = load_builder()
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            package = self._deb(builder, root)
            receipt = offline_receipt(root, "ubuntu-24.04-amd64")
            pool = root / "pool"
            pool.mkdir()
            self._dependency_deb(builder, pool / "dependency_1.0_amd64.deb")
            with self.assertRaisesRegex(builder.ReleaseError, "not admitted by the offline receipt"):
                builder.build_installer(
                    package,
                    pool,
                    root / "unadmitted.run",
                    target="ubuntu-24.04-amd64",
                    version="0.8.0",
                    source_date_epoch=1,
                    offline_receipt=receipt,
                    input_lock=offline_lock(root),
                )


class RaspberryPiImageTests(unittest.TestCase):
    def test_image_builder_delegates_only_validated_inputs_and_keeps_hardware_not_run(self) -> None:
        builder = load_builder()
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            base = root / "raspios.img.xz"
            base_image = bytearray(512)
            base_image[510:512] = b"\x55\xaa"
            base.write_bytes(lzma.compress(bytes(base_image)))
            package = root / "nilo_0.8.0_arm64.deb"
            payload = root / "payload.tar.xz"
            builder.build_payload(
                fixture_payload(root),
                payload,
                target="rpi-debian13-arm64",
                version="0.8.0",
                source_date_epoch=1_700_000_000,
                offline_receipt=offline_receipt(root, "rpi-debian13-arm64"),
                input_lock=offline_lock(root),
            )
            builder.build_deb(
                payload,
                package,
                target="rpi-debian13-arm64",
                version="0.8.0",
                source_date_epoch=1_700_000_000,
                offline_receipt=offline_receipt(root, "rpi-debian13-arm64"),
                input_lock=offline_lock(root),
            )
            pool = root / "pool"
            pool.mkdir()
            password_file = root / "first-admin.password"
            password_file.write_text("image-administrator-secret-1\n", encoding="utf-8")
            password_file.chmod(0o600)
            calls = []

            def fake_runner(command, *, env, check):
                calls.append((command, env, check))
                if str(command[1]).endswith("build-pi-image.sh"):
                    Path(command[-1]).write_bytes(lzma.compress(b"customized raw image"))

            output = root / "nilo-rpi-0.8.0.img.xz"
            receipt = builder.build_pi_image(
                base,
                package,
                pool,
                output,
                target="rpi-debian13-arm64",
                version="0.8.0",
                source_date_epoch=1_700_000_000,
                admin_password_file=password_file,
                offline_receipt=offline_receipt(
                    root, "rpi-debian13-arm64", admitted_paths=[base]
                ),
                input_lock=offline_lock(root),
                runner=fake_runner,
            )
            self.assertEqual(len(calls), 2)
            command, environment, check = calls[0]
            self.assertEqual(command[0], "sh")
            self.assertTrue(str(command[1]).endswith("build-pi-image.sh"))
            self.assertEqual(environment["SOURCE_DATE_EPOCH"], "1700000000")
            self.assertTrue(check)
            self.assertTrue(str(calls[1][0][1]).endswith("verify-image.sh"))
            self.assertEqual(receipt["hardware_acceptance"], "not_run")
            self.assertEqual(receipt["structural_verification"], "passed")
            self.assertEqual(receipt["sha256"], builder.sha256_file(output))
            self.assertEqual(lzma.decompress(output.read_bytes()), b"customized raw image")

            script = (PACKAGING_ROOT / "image" / "build-pi-image.sh").read_text(encoding="utf-8")
            for required in (
                "policy-rc.d",
                "machine-id",
                "ssh_host_",
                "NILO_GPIO_RECOVERY=disabled",
                "e2fsck",
                "trap cleanup",
                "--no-download",
            ):
                self.assertIn(required, script)
            self.assertIn("nilo-image-firstboot.service", script)
            self.assertIn("NetworkManager.service", script)
            self.assertIn("image-firstboot.pending", script)
            self.assertIn('output_dir=$(dirname -- "$output")', script)
            self.assertIn('sync "$temporary_output"', script)
            self.assertIn('sync "$output" "$output_dir"', script)
            firstboot = (PACKAGING_ROOT / "debian" / "nilo-image-firstboot").read_text(encoding="utf-8")
            self.assertIn("resizepart", firstboot)
            self.assertIn("resize2fs", firstboot)
            self.assertIn("systemd-machine-id-setup", firstboot)
            self.assertIn("image-firstboot.pending", firstboot)
            firstboot_unit = (
                PACKAGING_ROOT / "debian" / "units" / "nilo-image-firstboot.service"
            ).read_text(encoding="utf-8")
            self.assertIn("Before=nilo-firstboot.service nilo-nethelper.socket nilo.service", firstboot_unit)
            self.assertIn("ConditionPathExists=/var/lib/nilo/image-firstboot.pending", firstboot_unit)
            self.assertNotIn("curl ", script)
            self.assertNotIn("wget ", script)

    def test_image_builder_authenticates_package_before_privileged_execution(self) -> None:
        builder = load_builder()
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            base_header = bytearray(512)
            base_header[510:512] = b"\x55\xaa"
            base = root / "raspios.img.xz"
            base.write_bytes(lzma.compress(bytes(base_header)))
            payload = root / "payload.tar.xz"
            builder.build_payload(
                fixture_payload(root), payload, target="rpi-debian13-arm64",
                version="0.8.0", source_date_epoch=1_700_000_000,
                offline_receipt=offline_receipt(root, "rpi-debian13-arm64"),
                input_lock=offline_lock(root),
            )
            package = root / "nilo_0.8.0_arm64.deb"
            builder.build_deb(
                payload, package, target="rpi-debian13-arm64", version="0.8.0",
                source_date_epoch=1_700_000_000,
                offline_receipt=offline_receipt(root, "rpi-debian13-arm64"),
                input_lock=offline_lock(root),
            )
            members = read_ar(package)
            with tarfile.open(fileobj=io.BytesIO(members["control.tar.xz"]), mode="r:xz") as control:
                entries = [
                    (
                        member.name,
                        b"#!/bin/sh\nexit 0\n" if member.name == "./postinst" else control.extractfile(member).read(),
                        member.mode,
                    )
                    for member in control.getmembers()
                ]
            members["control.tar.xz"] = builder._archive_bytes(entries, 1_700_000_000)
            package.write_bytes(
                b"!<arch>\n"
                + builder._ar_member("debian-binary", members["debian-binary"], 1_700_000_000)
                + builder._ar_member("control.tar.xz", members["control.tar.xz"], 1_700_000_000)
                + builder._ar_member("data.tar.xz", members["data.tar.xz"], 1_700_000_000)
            )
            password = root / "password"
            password.write_text("a sufficiently long fixture password\n", encoding="utf-8")
            password.chmod(0o600)
            pool = root / "pool"
            pool.mkdir()
            calls = []
            with self.assertRaisesRegex(builder.ReleaseError, "maintainer metadata"):
                builder.build_pi_image(
                    base, package, pool, root / "output.img.xz",
                    target="rpi-debian13-arm64", version="0.8.0",
                    source_date_epoch=1_700_000_000, admin_password_file=password,
                    offline_receipt=offline_receipt(root, "rpi-debian13-arm64", admitted_paths=[base]),
                    input_lock=offline_lock(root),
                    runner=lambda *args, **kwargs: calls.append((args, kwargs)),
                )
            self.assertEqual(calls, [])

    def test_image_credential_remains_root_only_until_narrow_handoff(self) -> None:
        image_script = (PACKAGING_ROOT / "image" / "build-pi-image.sh").read_text(encoding="utf-8")
        firstboot_unit = (PACKAGING_ROOT / "debian" / "units" / "nilo-firstboot.service").read_text(encoding="utf-8")
        handoff = PACKAGING_ROOT / "debian" / "nilo-firstboot"
        self.assertTrue(handoff.is_file())
        handoff_text = handoff.read_text(encoding="utf-8")
        self.assertIn("600:root:root", image_script)
        self.assertLess(
            image_script.index("chown -R nilo:nilo /var/lib/nilo"),
            image_script.index('install -o 0 -g 0 -m 0600 "$admin_password"'),
        )
        self.assertNotIn("User=nilo", firstboot_unit)
        self.assertIn("/run/nilo/first-admin.password", handoff_text)
        self.assertIn("runuser -u nilo", handoff_text)
        self.assertIn("600:root:root", handoff_text)
        self.assertIn("/var/lib/nilo-installer/first-admin.password", image_script)
        self.assertIn("trap 'rm -f -- \"$handoff\"' EXIT HUP INT TERM", handoff_text)
        self.assertLess(handoff_text.index("trap 'rm -f"), handoff_text.index("runuser -u nilo"))
        self.assertIn("trap - EXIT HUP INT TERM", handoff_text)

    def test_privileged_recovery_state_is_outside_the_service_writable_data_root(self) -> None:
        builder = load_builder()
        driver = builder._installer_driver("ubuntu-24.04-amd64", "0.8.0", []).decode("utf-8")
        recovery = (PACKAGING_ROOT / "debian" / "nilo-installer-recover").read_text(encoding="utf-8")
        recovery_unit = (PACKAGING_ROOT / "debian" / "units" / "nilo-recovery.service").read_text(encoding="utf-8")
        self.assertIn('/var/lib/nilo-installer/transactions', driver)
        self.assertIn('install -d -o 0 -g 0 -m 0700 "$root/var/lib/nilo-installer"', driver)
        self.assertNotIn('/var/lib/nilo/transactions', driver)
        self.assertIn('state=$root/var/lib/nilo-installer', recovery)
        self.assertIn('case "$transaction" in\n    "$root"/var/lib/nilo-installer/transactions/install-*)', recovery)
        rollback = driver[driver.index("rollback() {"):driver.index("trap rollback")]
        self.assertLess(
            rollback.index('rm -rf -- "$root/var/lib/nilo"'),
            rollback.index('phase rolled_back'),
        )
        self.assertIn('/var/lib/nilo-installer/transactions', recovery_unit)
        self.assertNotIn('/var/lib/nilo/transactions', recovery_unit)

    def test_image_builder_refuses_non_pi_targets_and_symlinked_inputs(self) -> None:
        builder = load_builder()
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            base = root / "base.img.xz"
            base.write_bytes(lzma.compress(b"base"))
            package = root / "nilo.deb"
            package.write_bytes(b"package")
            pool = root / "pool"
            pool.mkdir()
            with self.assertRaisesRegex(builder.ReleaseError, "Raspberry Pi target"):
                builder.build_pi_image(
                    base,
                    package,
                    pool,
                    root / "wrong.img.xz",
                    target="ubuntu-24.04-amd64",
                    version="0.8.0",
                    source_date_epoch=1,
                    admin_password_file=package,
                    offline_receipt=offline_receipt(root, "ubuntu-24.04-amd64"),
                    input_lock=offline_lock(root),
                )


class ReleaseEvidenceTests(unittest.TestCase):
    @staticmethod
    def _signature_verifier(keyring: Path, inrelease: Path, fingerprints: list[str]) -> str:
        return fingerprints[0]

    def _lock(self, builder, root: Path) -> Path:
        artifact_types = {
            "base-image": "raspberry-pi-image-xz",
            "go-module-cache": "go-module-cache-tar",
            "gui-npm-cache": "npm-cache-tar",
            "os-package-pool": "debian-pool-tar",
            "sigrok-runtime": "sigrok-runtime-tar",
            "camera-build-input": "camera-build-input-tar",
            "source-offers": "source-offers-tar",
        }
        inputs = []
        fixture_root = root / "lock-fixtures"
        fixture_root.mkdir(parents=True, exist_ok=True)
        for scope, names in sorted(builder.REQUIRED_RELEASE_INPUTS.items()):
            for name in sorted(names):
                contents = f"{scope}/{name}".encode()
                apt = None
                if name == "base-image":
                    contents = lzma.compress(bytes(510) + b"\x55\xaa")
                elif name in {"go-module-cache", "gui-npm-cache"}:
                    contents = builder._archive_bytes(
                        [("fixture-cache-entry", contents, 0o644)], 1_700_000_000
                    )
                elif name == "source-offers":
                    contents = builder._archive_bytes(
                        [
                            ("LICENSES/MIT.txt", b"MIT fixture licence text\n", 0o644),
                            ("SOURCE_OFFER.txt", b"Corresponding source is included for redistributable inputs.\n", 0o644),
                            (
                                "source-index.json",
                                (json.dumps(
                                    {
                                        "entries": [
                                            {
                                                "license_file": "LICENSES/MIT.txt",
                                                "name": input_name,
                                                "scope": input_scope,
                                                "source_path": None,
                                            }
                                            for input_scope, input_names in sorted(builder.REQUIRED_RELEASE_INPUTS.items())
                                            for input_name in sorted(input_names)
                                        ],
                                        "schema": 1,
                                    },
                                    indent=2,
                                    sort_keys=True,
                                ) + "\n").encode("utf-8"),
                                0o644,
                            ),
                        ],
                        1_700_000_000,
                    )
                elif name == "sigrok-runtime":
                    runtime_manifest = {
                        "architecture": builder.TARGET_ARCHITECTURES[scope],
                        "components": [
                            {
                                "license": "GPL-3.0-or-later",
                                "name": "sigrok-runtime-fixture",
                                "source": "https://example.invalid/sigrok-source",
                                "version": "1.0",
                            }
                        ],
                        "files": [],
                        "health_checks": [
                            {"arguments": ["bin/python3", "--version"], "id": "python-version"},
                            {"arguments": ["bin/sigrok-cli", "--version"], "id": "sigrok-version"},
                            {"arguments": ["bin/sigrok-cli", "--scan"], "id": "sigrok-driver-scan"},
                        ],
                        "schema": 1,
                    }
                    contents = builder._archive_bytes(
                        [("runtime-manifest.json", json.dumps(runtime_manifest, sort_keys=True).encode() + b"\n", 0o644)],
                        1_700_000_000,
                    )
                elif name == "camera-build-input":
                    contents = builder._archive_bytes(
                        [
                            ("dependencies/nilo_integration/__init__.py", b"", 0o644),
                            ("dependencies/pydantic/__init__.py", b"", 0o644),
                            ("wheelhouse/pyrav4l2-1.0-py3-none-any.whl", b"wheel", 0o644),
                            ("wheelhouse/pillow-11.3.0-fixture.whl", b"wheel", 0o644),
                            ("THIRD_PARTY_NOTICES.txt", b"camera dependency notices\n", 0o644),
                        ],
                        1_700_000_000,
                    )
                if name == "os-package-pool":
                    architecture = builder.TARGET_ARCHITECTURES[scope]
                    package = dependency_deb(builder, architecture)
                    filename = f"pool/fixture-dependency_1.0_{architecture}.deb"
                    packages = (
                        f"Package: fixture-dependency\nVersion: 1.0\nArchitecture: {architecture}\n"
                        f"Filename: {filename}\nSize: {len(package)}\n"
                        f"SHA256: {hashlib.sha256(package).hexdigest()}\n\n"
                    ).encode()
                    suite = "trixie" if scope == "rpi-debian13-arm64" else "noble"
                    inrelease = (
                        "-----BEGIN PGP SIGNED MESSAGE-----\nHash: SHA256\n\n"
                        f"Suite: {suite}\nCodename: {suite}\nComponents: main\nArchitectures: {architecture}\n"
                        f"SHA256:\n {hashlib.sha256(packages).hexdigest()} {len(packages)} apt/Packages\n"
                        "-----BEGIN PGP SIGNATURE-----\nfixture\n-----END PGP SIGNATURE-----\n"
                    ).encode()
                    keyring = b"fixture release keyring"
                    contents = builder._archive_bytes(
                        [
                            ("apt/InRelease", inrelease, 0o644),
                            ("apt/Packages", packages, 0o644),
                            ("apt/archive-keyring.gpg", keyring, 0o644),
                            (filename, package, 0o644),
                        ],
                        1_700_000_000,
                    )
                    package_digest = hashlib.sha256(package).hexdigest()
                    apt = {
                        "base_packages": [],
                        "component": "main",
                        "keyring_sha256": hashlib.sha256(keyring).hexdigest(),
                        "packages": [
                            {
                                "architecture": builder.TARGET_ARCHITECTURES[scope],
                                "depends": "",
                                "filename": filename,
                                "package": "fixture-dependency",
                                "pre_depends": "",
                                "provides": "",
                                "sha256": package_digest,
                                "signed_by": "2" * 40,
                                "size": len(package),
                                "version": "1.0",
                            }
                        ],
                        "repository": f"https://repository.example.invalid/{scope}/snapshot",
                        "roots": ["fixture-dependency"],
                        "signing_fingerprints": ["2" * 40],
                        "suite": suite,
                    }
                (fixture_root / f"{scope}--{name}").write_bytes(contents)
                inputs.append(
                    {
                        "apt": apt,
                        "architecture": "all" if scope == "shared" else builder.TARGET_ARCHITECTURES[scope],
                        "artifact_type": artifact_types[name],
                        "license": "MIT",
                        "name": name,
                        "provenance": f"https://example.invalid/{scope}/{name}/provenance",
                        "redistribution": "permitted",
                        "scope": scope,
                        "sha256": hashlib.sha256(contents).hexdigest(),
                        "size": len(contents),
                        "url": f"https://example.invalid/{scope}/{name}",
                        "version": "1.0.0",
                    }
                )
        path = root / "inputs.lock.json"
        path.write_text(
            json.dumps({"complete": True, "incomplete_reasons": [], "inputs": inputs, "schema": 1}, indent=2, sort_keys=True) + "\n",
            encoding="utf-8",
        )
        return path

    def _acceptance(self, builder, root: Path) -> Path:
        targets = json.loads(
            (PACKAGING_ROOT / "config" / "targets.json").read_text(encoding="utf-8")
        )
        checks = [
            self._software_check(root, target, check)
            for target in sorted(builder.TARGET_ARCHITECTURES)
            for check in builder.SOFTWARE_ACCEPTANCE_CHECKS
        ]
        primary = root / "input-artifacts"
        prior_root = root / "prior-build"
        prior_root.mkdir()
        prior = self._artifacts(builder, prior_root, version="0.7.9", include_image=False)
        build_inputs = self._candidate_build_inputs(root)
        for check in checks:
            if check["check"] == "build-reproducibility":
                check["candidate_build_inputs"] = str(build_inputs.resolve())
            if check["check"] in builder.PRIOR_INSTALLER_CHECKS:
                check["prior_installer"] = str(
                    next(prior.glob(f"nilo-0.7.9-{check['target']}.run")).resolve()
                )
        checks.append(self._software_check(root, "rpi-debian13-arm64", "image-structure"))
        for target in targets["targets"]:
            checks.extend(
                {
                    "check": check,
                    "kind": "physical",
                    "reason": "physical target unavailable in this build environment",
                    "target": target["id"],
                }
                for check in target["hardware_acceptance"]
            )
        path = root / "acceptance-plan.json"
        path.write_text(
            json.dumps({"checks": checks, "schema": 2}, indent=2, sort_keys=True) + "\n",
            encoding="utf-8",
        )
        return path

    def _candidate_build_inputs(self, root: Path) -> Path:
        lock = root / "inputs.lock.json"
        if not lock.is_file():
            lock = self._lock(load_builder(), root)
        fixture_root = root / "lock-fixtures"
        base_image = fixture_root / "rpi-debian13-arm64--base-image"
        password = root / "first-admin.password"
        if not password.is_file():
            password.write_text("fixture-password-never-published\n", encoding="utf-8")
            password.chmod(0o600)
        document = {
            "admin_password": str(password.resolve()),
            "base_image": str(base_image.resolve()),
            "go_module_cache": str((fixture_root / "shared--go-module-cache").resolve()),
            "input_lock": str(lock.resolve()),
            "pi_pool": str((root / "rpi-debian13-arm64" / "pool").resolve()),
            "pi_receipt": str((root / "rpi-debian13-arm64" / "rpi-debian13-arm64-preparation.json").resolve()),
            "pi_sigrok_runtime": str((fixture_root / "rpi-debian13-arm64--sigrok-runtime").resolve()),
            "pi_camera_build_input": str((fixture_root / "rpi-debian13-arm64--camera-build-input").resolve()),
            "schema": 1,
            "source_root": str(REPOSITORY_ROOT.resolve()),
            "npm_cache": str((fixture_root / "shared--gui-npm-cache").resolve()),
            "ubuntu_pool": str((root / "ubuntu-24.04-amd64" / "pool").resolve()),
            "ubuntu_receipt": str((root / "ubuntu-24.04-amd64" / "ubuntu-24.04-amd64-preparation.json").resolve()),
            "ubuntu_sigrok_runtime": str((fixture_root / "ubuntu-24.04-amd64--sigrok-runtime").resolve()),
            "ubuntu_camera_build_input": str((fixture_root / "ubuntu-24.04-amd64--camera-build-input").resolve()),
        }
        path = root / "candidate-build-inputs.json"
        path.write_text(json.dumps(document, indent=2, sort_keys=True) + "\n", encoding="utf-8")
        return path

    def _build_set_receipt(
        self, builder, path: Path, artifacts: Path, invocation_id: str,
        lock_path: Path | None = None,
    ) -> Path:
        receipt_root = path.parent / f"{path.stem}-artifact-receipts"
        receipt_root.mkdir(exist_ok=True)
        lock = lock_path or path.parent / "inputs.lock.json"
        if not lock.is_file():
            lock = self._lock(builder, path.parent)
        artifact_receipts = []
        for artifact in sorted(artifacts.iterdir(), key=lambda item: item.name):
            target = (
                "rpi-debian13-arm64"
                if "arm64" in artifact.name or artifact.name.endswith(".img.xz")
                else "ubuntu-24.04-amd64"
            )
            receipt = receipt_root / f"{artifact.name}.json"
            command = "image" if artifact.name.endswith(".img.xz") else "installer" if artifact.name.endswith(".run") else "deb"
            package = next(
                (
                    candidate for candidate in artifacts.iterdir()
                    if candidate.name.endswith(
                        "_arm64.deb" if target == "rpi-debian13-arm64" else "_amd64.deb"
                    )
                ),
                lock,
            )
            input_paths = {
                "deb": {"input_lock": lock, "offline_receipt": lock, "payload": lock},
                "installer": {"input_lock": lock, "offline_receipt": lock, "package": package},
                "image": {
                    "base_image": lock,
                    "input_lock": lock,
                    "offline_receipt": lock,
                    "package": package,
                },
            }[command]
            if command in {"installer", "image"}:
                input_paths["pool/fixture-dependency.deb"] = package
            receipt.write_text(
                json.dumps(
                    {
                        "artifact": artifact.name,
                        "artifact_path": str(artifact.resolve()),
                        "build_invocation_id": invocation_id,
                        "builder_sha256": hashlib.sha256(Path(builder.__file__).read_bytes()).hexdigest(),
                        "command": command,
                        "input_lock_sha256": hashlib.sha256(lock.read_bytes()).hexdigest(),
                        "inputs": [
                            {
                                "name": name,
                                "path": str(input_path.resolve()),
                                "sha256": hashlib.sha256(input_path.read_bytes()).hexdigest(),
                                "size": input_path.stat().st_size,
                            }
                            for name, input_path in sorted(input_paths.items())
                        ],
                        "schema": 2,
                        "sha256": hashlib.sha256(artifact.read_bytes()).hexdigest(),
                        "size": artifact.stat().st_size,
                        "source_revision": "0123456789abcdef",
                        "target": target,
                        "version": "0.8.0" if "0.8.0" in artifact.name else "0.7.9",
                    },
                    sort_keys=True,
                ) + "\n",
                encoding="utf-8",
            )
            artifact_receipts.append(receipt)
        builder.create_build_set_receipt(
            artifacts,
            artifact_receipts,
            path,
            lock_path=lock,
            source_revision="0123456789abcdef",
        )
        return path

    def _software_check(self, root: Path, target: str, check: str) -> dict[str, object]:
        result = {
            "check": check,
            "kind": "software",
            "target": target,
            "timeout_seconds": 30,
        }
        if check in load_builder().LIFECYCLE_ACCEPTANCE_CHECKS:
            result["target_image"] = f"registry.example.invalid/{target}@sha256:" + "a" * 64
        return result

    @staticmethod
    def _offline_observer() -> dict[str, object]:
        return {
            "interfaces": ["lo"],
            "ipv4_default_route": False,
            "ipv6_default_route": False,
            "platform": "linux",
            "verified": True,
        }

    @staticmethod
    def _successful_acceptance(builder, command, options):
        target = command[command.index("--target") + 1]
        check = command[command.index("--check") + 1]
        if check == "build-reproducibility":
            artifacts = Path(command[command.index("--artifacts") + 1])
            bindings = [
                {
                    "name": path.name,
                    "sha256": hashlib.sha256(path.read_bytes()).hexdigest(),
                    "size": path.stat().st_size,
                }
                for path in sorted(artifacts.iterdir(), key=lambda item: item.name)
            ]
            stdout = json.dumps(
                {
                    "artifacts": bindings,
                    "builds": [
                        {"artifacts": bindings, "invocation_id": "acceptance-build-0001"},
                        {"artifacts": bindings, "invocation_id": "acceptance-build-0002"},
                    ],
                    "schema": 1,
                    "status": "pass",
                },
                sort_keys=True,
            ).encode("utf-8") + b"\n"
            return subprocess.CompletedProcess(command, 0, stdout, b"")
        architecture = builder.TARGET_ARCHITECTURES[target]
        version = "24.04" if target == "ubuntu-24.04-amd64" else "13"
        os_id = "ubuntu" if target == "ubuntu-24.04-amd64" else "debian"
        image_reference = options["env"].get("NILO_ACCEPTANCE_EXPECTED_IMAGE", "")
        stdout = (
            f"NILO_ACCEPTANCE_IMAGE_REFERENCE={image_reference}\n"
            f"NILO_ACCEPTANCE_IMAGE_ID=sha256:{'b' * 64}\n"
            "NILO_ACCEPTANCE_KERNEL=6.8.0-fixture\n"
            f"NILO_ACCEPTANCE_OS_ID={os_id}\n"
            f"NILO_ACCEPTANCE_OS_VERSION={version}\n"
            f"NILO_ACCEPTANCE_ARCHITECTURE={architecture}\nverified\n"
        ).encode("utf-8")
        return subprocess.CompletedProcess(command, 0, stdout, b"")

    def _source_offers(self, root: Path) -> Path:
        source = root / "source-offers"
        (source / "LICENSES").mkdir(parents=True)
        (source / "SOURCE_OFFER.txt").write_bytes(b"Corresponding source is included for redistributable inputs.\n")
        (source / "LICENSES" / "MIT.txt").write_bytes(b"MIT fixture licence text\n")
        (source / "source-index.json").write_bytes(
            (json.dumps(
                {
                    "entries": [
                        {
                            "license_file": "LICENSES/MIT.txt",
                            "name": input_name,
                            "scope": input_scope,
                            "source_path": None,
                        }
                        for input_scope, input_names in sorted(load_builder().REQUIRED_RELEASE_INPUTS.items())
                        for input_name in sorted(input_names)
                    ],
                    "schema": 1,
                },
                indent=2,
                sort_keys=True,
            ) + "\n").encode("utf-8"),
        )
        return source

    def _artifacts(
        self, builder, root: Path, *, version: str = "0.8.0", include_image: bool = True
    ) -> Path:
        artifacts = root / "input-artifacts"
        artifacts.mkdir()
        for target, architecture in (
            ("ubuntu-24.04-amd64", "amd64"),
            ("rpi-debian13-arm64", "arm64"),
        ):
            target_root = root / target
            target_root.mkdir()
            payload = target_root / "payload.tar.xz"
            builder.build_payload(
                fixture_payload(target_root),
                payload,
                target=target,
                version=version,
                source_date_epoch=1_700_000_000,
                offline_receipt=offline_receipt(target_root, target),
                input_lock=offline_lock(target_root),
            )
            package = artifacts / f"nilo_{version}_{architecture}.deb"
            builder.build_deb(
                payload,
                package,
                target=target,
                version=version,
                source_date_epoch=1_700_000_000,
                offline_receipt=offline_receipt(target_root, target),
                input_lock=offline_lock(target_root),
            )
            pool = target_root / "pool"
            pool.mkdir()
            dependency = pool / f"fixture-dependency_1.0_{architecture}.deb"
            dependency.write_bytes(dependency_deb(builder, architecture))
            installer = artifacts / f"nilo-{version}-{target}.run"
            builder.build_installer(
                package,
                pool,
                installer,
                target=target,
                version=version,
                source_date_epoch=1_700_000_000,
                offline_receipt=offline_receipt(
                    target_root, target, admitted_paths=[dependency]
                ),
                input_lock=offline_lock(target_root),
            )
        if include_image:
            (artifacts / f"nilo-pi4-{version}.img.xz").write_bytes(
                lzma.compress(bytes(510) + b"\x55\xaa")
            )
        return artifacts

    def _assemble(self, builder, root: Path) -> Path:
        artifacts = self._artifacts(builder, root)
        lock = self._lock(builder, root)
        release = root / "release"

        def execute_acceptance(command, **options):
            return self._successful_acceptance(builder, command, options)

        builder.assemble_release(
            artifacts,
            release,
            version="0.8.0",
            source_date_epoch=1_700_000_000,
            lock_path=lock,
            targets_path=PACKAGING_ROOT / "config" / "targets.json",
            source_revision="0123456789abcdef",
            acceptance_path=self._acceptance(builder, root),
            source_offers_path=self._source_offers(root),
            offline_receipts=[
                offline_receipt(root, target, lock)
                for target in sorted(builder.TARGET_ARCHITECTURES)
            ],
            network_observer=self._offline_observer,
            runner=execute_acceptance,
            apt_signature_verifier=self._signature_verifier,
        )
        return release

    def test_release_evidence_closes_every_artifact_and_keeps_physical_checks_not_run(self) -> None:
        builder = load_builder()
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            artifacts = self._artifacts(builder, root)
            lock = self._lock(builder, root)
            release = root / "release"
            executions = []

            def execute_acceptance(command, **options):
                executions.append((command, options))
                return self._successful_acceptance(builder, command, options)

            builder.assemble_release(
                artifacts,
                release,
                version="0.8.0",
                source_date_epoch=1_700_000_000,
                lock_path=lock,
                targets_path=PACKAGING_ROOT / "config" / "targets.json",
                source_revision="0123456789abcdef",
                acceptance_path=self._acceptance(builder, root),
                source_offers_path=self._source_offers(root),
                offline_receipts=[
                    offline_receipt(root, target, lock)
                    for target in sorted(builder.TARGET_ARCHITECTURES)
                ],
                network_observer=self._offline_observer,
                runner=execute_acceptance,
                apt_signature_verifier=self._signature_verifier,
            )
            result = builder.verify_release(release)
            self.assertEqual(result["status"], "pass")
            self.assertEqual(len(result["artifacts"]), 5)
            self.assertTrue(executions)
            lifecycle_commands = [
                command for command, _ in executions
                if command[command.index("--check") + 1] in builder.LIFECYCLE_ACCEPTANCE_CHECKS
            ]
            static_commands = [command for command, _ in executions if command not in lifecycle_commands]
            self.assertTrue(lifecycle_commands)
            self.assertTrue(all(command[:2] == ["sh", str(builder.TARGET_ACCEPTANCE_HARNESS.resolve())] for command in lifecycle_commands))
            self.assertTrue(all(command[:2] == ["sh", str(builder.ACCEPTANCE_RUNNER.resolve())] for command in static_commands))
            self.assertTrue(all("NILO_ACCEPTANCE_DISPOSABLE_TARGET" not in options["env"] for _, options in executions))
            for expected in (
                "manifest.json",
                "SHA256SUMS",
                "sbom.spdx.json",
                "THIRD_PARTY_NOTICES/inputs.txt",
                "OPERATOR_GUIDE.md",
                "inputs.lock.json",
                "provenance.json",
                "acceptance.json",
                "targets.json",
                "source-offer/SOURCE_OFFER.txt",
            ):
                self.assertTrue((release / expected).is_file(), expected)
            acceptance = json.loads((release / "acceptance.json").read_text(encoding="utf-8"))
            physical = [check for check in acceptance["checks"] if check["kind"] == "physical"]
            self.assertTrue(physical)
            self.assertTrue(all(check["status"] == "not_run" for check in physical))
            self.assertTrue(all(check["status"] == "passed" for check in acceptance["checks"] if check["kind"] != "physical"))

            sbom = json.loads((release / "sbom.spdx.json").read_text(encoding="utf-8"))
            dependency_packages = [item for item in sbom["packages"] if item["name"] == "fixture-dependency"]
            self.assertEqual(len(dependency_packages), 2)
            runtime_components = [item for item in sbom["packages"] if item["name"] == "sigrok-runtime-fixture"]
            self.assertEqual(len(runtime_components), 2)

            software = next(
                check
                for check in acceptance["checks"]
                if check["kind"] == "software" and check["check"] == "build-reproducibility"
            )
            result_path = release / software["evidence"]["path"]
            result = json.loads(result_path.read_text(encoding="utf-8"))
            self.assertEqual(result["exit_code"], 0)
            self.assertEqual(result["network_before"]["interfaces"], ["lo"])
            self.assertEqual(len(result["artifacts"]), 5)
            self.assertEqual(result["reproduced_artifacts"], result["artifacts"])
            self.assertTrue((release / result["stdout"]["path"]).is_file())
            self.assertTrue((release / result["stderr"]["path"]).is_file())

            artifact = next(release.rglob("*.deb"))
            artifact.write_bytes(artifact.read_bytes() + b"tampered")
            with self.assertRaisesRegex(builder.ReleaseError, "digest mismatch"):
                builder.verify_release(release)

    def test_release_evidence_refuses_an_incomplete_lock(self) -> None:
        builder = load_builder()
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            lock = root / "inputs.lock.json"
            lock.write_text(
                '{"schema":1,"complete":false,"incomplete_reasons":["missing"],"inputs":[]}\n',
                encoding="utf-8",
            )
            with self.assertRaisesRegex(builder.ReleaseError, "input lock is incomplete"):
                builder.assemble_release(
                    root,
                    root / "release",
                    version="0.8.0",
                    source_date_epoch=1,
                    lock_path=lock,
                    targets_path=PACKAGING_ROOT / "config" / "targets.json",
                    source_revision="revision",
                )

    def test_offline_receipt_must_match_the_selected_input_lock(self) -> None:
        builder = load_builder()
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            lock = self._lock(builder, root)
            receipt = offline_receipt(root, "ubuntu-24.04-amd64", lock)
            document = json.loads(receipt.read_text(encoding="utf-8"))
            document["inputs"][0]["version"] = "forged"
            receipt.write_text(json.dumps(document), encoding="utf-8")
            with self.assertRaisesRegex(builder.ReleaseError, "does not match the release input lock"):
                builder._validate_offline_receipt(
                    receipt, "ubuntu-24.04-amd64", lock_path=lock
                )

    def test_release_evidence_refuses_missing_executed_software_acceptance(self) -> None:
        builder = load_builder()
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            artifacts = self._artifacts(builder, root)
            lock = self._lock(builder, root)
            with self.assertRaisesRegex(builder.ReleaseError, "software acceptance evidence"):
                builder.assemble_release(
                    artifacts,
                    root / "release",
                    version="0.8.0",
                    source_date_epoch=1_700_000_000,
                    lock_path=lock,
                    targets_path=PACKAGING_ROOT / "config" / "targets.json",
                    source_revision="0123456789abcdef",
                    offline_receipts=[
                        offline_receipt(root, target, lock)
                        for target in sorted(builder.TARGET_ARCHITECTURES)
                    ],
                    apt_signature_verifier=self._signature_verifier,
                )

    def test_release_evidence_rejects_self_attested_passes_without_executing_checks(self) -> None:
        builder = load_builder()
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            artifacts = self._artifacts(builder, root)
            plan = self._acceptance(builder, root)
            document = json.loads(plan.read_text(encoding="utf-8"))
            software = next(check for check in document["checks"] if check["kind"] == "software")
            software["status"] = "passed"
            evidence = root / "claimed.json"
            evidence.write_text('{"claimed":"passed"}\n', encoding="utf-8")
            software["evidence"] = {
                "path": evidence.name,
                "sha256": hashlib.sha256(evidence.read_bytes()).hexdigest(),
            }
            plan.write_text(json.dumps(document), encoding="utf-8")

            with self.assertRaisesRegex(builder.ReleaseError, "acceptance command plan"):
                builder.assemble_release(
                    artifacts,
                    root / "release",
                    version="0.8.0",
                    source_date_epoch=1_700_000_000,
                    lock_path=self._lock(builder, root),
                    targets_path=PACKAGING_ROOT / "config" / "targets.json",
                    source_revision="0123456789abcdef",
                    acceptance_path=plan,
                    source_offers_path=self._source_offers(root),
                    offline_receipts=[
                        offline_receipt(root, target, self._lock(builder, root))
                        for target in sorted(builder.TARGET_ARCHITECTURES)
                    ],
                    network_observer=self._offline_observer,
                    apt_signature_verifier=self._signature_verifier,
                )

    def test_release_evidence_derives_failure_from_the_executed_command(self) -> None:
        builder = load_builder()
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            check = self._software_check(root, "ubuntu-24.04-amd64", "checksums")
            with self.assertRaisesRegex(builder.ReleaseError, "failed with exit 7"):
                builder._execute_acceptance_check(
                    check,
                    artifact_dir=root,
                    runner=lambda command, **options: subprocess.CompletedProcess(command, 7, b"", b"failed"),
                    network_observer=self._offline_observer,
                )

    def test_release_evidence_rejects_substitute_acceptance_commands(self) -> None:
        builder = load_builder()
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            check = self._software_check(root, "ubuntu-24.04-amd64", "checksums")
            check["command"] = {
                "argv": [str(Path(sys.executable).resolve()), "-c", "raise SystemExit(0)"],
                "cwd": str(root.resolve()),
                "timeout_seconds": 30,
            }
            with self.assertRaisesRegex(builder.ReleaseError, "repository-owned acceptance"):
                builder._execute_acceptance_check(
                    check,
                    artifact_dir=root,
                    runner=subprocess.run,
                    network_observer=self._offline_observer,
                )

    def test_release_evidence_rejects_non_reproducible_artifact_bytes(self) -> None:
        builder = load_builder()
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            artifacts = self._artifacts(builder, root)
            build_inputs = self._candidate_build_inputs(root)
            build_number = 0

            def execute_build(command, **options):
                nonlocal build_number
                build_number += 1
                output = Path(command[2])
                self.assertIn(str(REPOSITORY_ROOT.resolve()), command)
                self.assertFalse(any("payload.tar.xz" in argument for argument in command))
                shutil.copytree(artifacts, output)
                if build_number == 2:
                    package = next(output.glob("*.deb"))
                    package.write_bytes(package.read_bytes() + b"different")
                self._build_set_receipt(
                    builder,
                    Path(str(output) + ".build.json"),
                    output,
                    f"acceptance-build-000{build_number}",
                    lock_path=root / "inputs.lock.json",
                )
                return subprocess.CompletedProcess(command, 0, b"", b"")

            with self.assertRaisesRegex(builder.ReleaseError, "not byte-reproducible"):
                builder.verify_reproducible_candidate(
                    artifacts,
                    build_inputs,
                    source_revision="0123456789abcdef",
                    source_date_epoch=1_700_000_000,
                    runner=execute_build,
                )

    def test_reproducibility_runs_two_fixed_builds_in_fresh_roots(self) -> None:
        builder = load_builder()
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            artifacts = self._artifacts(builder, root)
            build_inputs = self._candidate_build_inputs(root)
            outputs = []

            def execute_build(command, **options):
                output = Path(command[2])
                self.assertFalse(output.exists())
                self.assertIn(str(REPOSITORY_ROOT.resolve()), command)
                self.assertFalse(any("payload.tar.xz" in argument for argument in command))
                outputs.append(output)
                shutil.copytree(artifacts, output)
                self._build_set_receipt(
                    builder,
                    Path(str(output) + ".build.json"),
                    output,
                    f"acceptance-build-000{len(outputs)}",
                    lock_path=root / "inputs.lock.json",
                )
                return subprocess.CompletedProcess(command, 0, b"", b"")

            result = builder.verify_reproducible_candidate(
                artifacts,
                build_inputs,
                source_revision="0123456789abcdef",
                source_date_epoch=1_700_000_000,
                runner=execute_build,
            )
            self.assertEqual(len(outputs), 2)
            self.assertEqual(result["status"], "pass")
            self.assertEqual(result["builds"][0]["artifacts"], result["artifacts"])
            self.assertNotEqual(
                result["builds"][0]["invocation_id"],
                result["builds"][1]["invocation_id"],
            )

    def test_build_set_receipts_are_emitted_from_artifact_builder_receipts(self) -> None:
        builder = load_builder()
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            artifacts = self._artifacts(builder, root)
            lock = self._lock(builder, root)
            output = root / "build-set.json"
            self._build_set_receipt(builder, output, artifacts, "fixture-build-0001")
            emitted = json.loads(output.read_text(encoding="utf-8"))
            self.assertEqual(emitted["schema"], 3)
            self.assertEqual(emitted["input_lock_sha256"], hashlib.sha256(lock.read_bytes()).hexdigest())
            self.assertEqual(len(emitted["artifact_receipts"]), len(list(artifacts.iterdir())))
            builder._validate_build_set_receipt(
                output,
                artifacts.resolve(),
                emitted["artifacts"],
                expected_lock_sha256=emitted["input_lock_sha256"],
                expected_source_revision="0123456789abcdef",
            )

    def test_build_set_receipts_reject_post_hoc_receipts_for_copied_artifacts(self) -> None:
        builder = load_builder()
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            artifacts = self._artifacts(builder, root)
            copied = root / "copied-artifacts"
            shutil.copytree(artifacts, copied)
            lock = self._lock(builder, root)
            primary = self._build_set_receipt(
                builder, root / "primary-build.json", artifacts, "primary-build-0001"
            )
            forged = self._build_set_receipt(
                builder, root / "forged-build.json", copied, "forged-build-0002"
            )
            check = self._software_check(root, "ubuntu-24.04-amd64", "build-reproducibility")
            check.update(
                {
                    "primary_build_receipt": str(primary.resolve()),
                    "reproduced_artifacts": str(copied.resolve()),
                    "reproduced_build_receipt": str(forged.resolve()),
                }
            )
            with self.assertRaisesRegex(builder.ReleaseError, "candidate build inputs"):
                builder._execute_acceptance_check(
                    check,
                    artifact_dir=artifacts,
                    runner=lambda command, **options: subprocess.CompletedProcess(
                        command, 0, b"", b""
                    ),
                    network_observer=self._offline_observer,
                    expected_lock_sha256=hashlib.sha256(lock.read_bytes()).hexdigest(),
                    expected_source_revision="0123456789abcdef",
                )

    def test_build_set_receipts_reject_malformed_embedded_builder_metadata(self) -> None:
        builder = load_builder()
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            artifacts = self._artifacts(builder, root)
            lock = self._lock(builder, root)
            receipt_path = self._build_set_receipt(
                builder, root / "build-set.json", artifacts, "fixture-build-0001"
            )
            receipt = json.loads(receipt_path.read_text(encoding="utf-8"))
            embedded = next(
                item for item in receipt["artifact_receipts"]
                if item["artifact"].endswith("_amd64.deb")
            )
            embedded["receipt"]["command"] = "installer"
            embedded["receipt_sha256"] = hashlib.sha256(
                builder.canonical_json(embedded["receipt"])
            ).hexdigest()
            receipt_path.write_bytes(builder.canonical_json(receipt))
            with self.assertRaisesRegex(builder.ReleaseError, "artifact-builder receipt"):
                builder._validate_build_set_receipt(
                    receipt_path,
                    artifacts.resolve(),
                    receipt["artifacts"],
                    expected_lock_sha256=hashlib.sha256(lock.read_bytes()).hexdigest(),
                    expected_source_revision="0123456789abcdef",
                )

    def test_acceptance_retains_exact_target_image_and_kernel_identity(self) -> None:
        builder = load_builder()
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            check = self._software_check(root, "ubuntu-24.04-amd64", "installer-clean-install")
            check["target_image"] = "registry.example.invalid/ubuntu@sha256:" + "a" * 64
            stdout = (
                "NILO_ACCEPTANCE_IMAGE_REFERENCE=registry.example.invalid/ubuntu@sha256:" + "a" * 64 + "\n"
                "NILO_ACCEPTANCE_IMAGE_ID=sha256:" + "b" * 64 + "\n"
                "NILO_ACCEPTANCE_KERNEL=6.8.0-fixture\n"
                "NILO_ACCEPTANCE_OS_ID=ubuntu\n"
                "NILO_ACCEPTANCE_OS_VERSION=24.04\n"
                "NILO_ACCEPTANCE_ARCHITECTURE=amd64\nverified\n"
            ).encode("utf-8")
            result, _ = builder._execute_acceptance_check(
                check,
                artifact_dir=root,
                runner=lambda command, **options: subprocess.CompletedProcess(command, 0, stdout, b""),
                network_observer=self._offline_observer,
            )
            self.assertEqual(result["target_environment"]["kernel"], "6.8.0-fixture")
            self.assertEqual(result["target_environment"]["image_reference"], check["target_image"])

    def test_release_sbom_requires_artifact_dependency_relationships(self) -> None:
        builder = load_builder()
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            release = self._assemble(builder, root)
            sbom_path = release / "sbom.spdx.json"
            sbom = json.loads(sbom_path.read_text(encoding="utf-8"))
            relationships = sbom.get("relationships", [])
            self.assertTrue(any(item.get("relationshipType") == "DESCRIBES" for item in relationships))
            self.assertTrue(any(item.get("relationshipType") == "CONTAINS" for item in relationships))
            self.assertTrue(any(item.get("relationshipType") == "DEPENDS_ON" for item in relationships))
            sbom["relationships"] = relationships[:-1]
            with self.assertRaisesRegex(builder.ReleaseError, "SBOM relationship"):
                builder._validate_sbom_relationships(sbom)

    def test_release_rejects_source_offer_bytes_not_bound_to_the_admitted_lock(self) -> None:
        builder = load_builder()
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            artifacts = self._artifacts(builder, root)
            lock = self._lock(builder, root)
            source_offers = self._source_offers(root)
            (source_offers / "SOURCE_OFFER.txt").write_text("unbound replacement\n", encoding="utf-8")
            with self.assertRaisesRegex(builder.ReleaseError, "source offer.*admitted"):
                builder.assemble_release(
                    artifacts,
                    root / "release",
                    version="0.8.0",
                    source_date_epoch=1_700_000_000,
                    lock_path=lock,
                    targets_path=PACKAGING_ROOT / "config" / "targets.json",
                    source_revision="0123456789abcdef",
                    acceptance_path=self._acceptance(builder, root),
                    source_offers_path=source_offers,
                    offline_receipts=[offline_receipt(root, target, lock) for target in sorted(builder.TARGET_ARCHITECTURES)],
                    network_observer=self._offline_observer,
                    runner=lambda command, **options: self._successful_acceptance(builder, command, options),
                    apt_signature_verifier=self._signature_verifier,
                )

    def test_release_evidence_rejects_malformed_artifacts_and_weakened_apt_provenance(self) -> None:
        builder = load_builder()
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            artifacts = self._artifacts(builder, root)
            (artifacts / "nilo-pi4-0.8.0.img.xz").write_bytes(lzma.compress(b"not an image"))
            lock = self._lock(builder, root)
            common = dict(
                version="0.8.0", source_date_epoch=1_700_000_000,
                targets_path=PACKAGING_ROOT / "config" / "targets.json",
                source_revision="0123456789abcdef",
                acceptance_path=self._acceptance(builder, root),
                source_offers_path=self._source_offers(root),
                offline_receipts=[offline_receipt(root, target, lock) for target in sorted(builder.TARGET_ARCHITECTURES)],
                network_observer=self._offline_observer,
                runner=lambda command, **options: self._successful_acceptance(builder, command, options),
                apt_signature_verifier=self._signature_verifier,
            )
            with self.assertRaisesRegex(builder.ReleaseError, "image structure"):
                builder.assemble_release(artifacts, root / "bad-image-release", lock_path=lock, **common)

            second = root / "second"
            second.mkdir()
            artifacts = self._artifacts(builder, second)
            lock = self._lock(builder, second)
            document = json.loads(lock.read_text(encoding="utf-8"))
            pool = next(item for item in document["inputs"] if item["name"] == "os-package-pool")
            del pool["apt"]["keyring_sha256"]
            lock.write_text(json.dumps(document), encoding="utf-8")
            with self.assertRaisesRegex(builder.ReleaseError, "dependency provenance"):
                builder.assemble_release(
                    artifacts, root / "bad-lock-release", lock_path=lock,
                    offline_receipts=[offline_receipt(root, target, lock) for target in sorted(builder.TARGET_ARCHITECTURES)],
                    **{key: value for key, value in common.items() if key != "offline_receipts"},
                )


class ProductionIntegrationTests(unittest.TestCase):
    def test_sigrok_bundle_embeds_its_runtime_without_package_resolution(self) -> None:
        builder = load_sigrok_builder()
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            runtime_root = root / "runtime-input"
            for relative, contents in {
                "bin/python3": elf_fixture("amd64"),
                "bin/sigrok-cli": elf_fixture("amd64"),
                "lib/libsigrok.so": elf_fixture("amd64"),
                "THIRD_PARTY_NOTICES.txt": b"runtime notices\n",
            }.items():
                path = runtime_root / relative
                path.parent.mkdir(parents=True, exist_ok=True)
                path.write_bytes(contents)
            write_runtime_manifest(runtime_root, "amd64")
            output = root / "sigrok"
            executions = []

            def run_health(arguments, **options):
                executions.append((arguments, options))
                return subprocess.CompletedProcess(arguments, 0, "healthy\n", "")

            builder.build(
                output,
                "amd64",
                runtime_root,
                runner=run_health,
                network_observer=lambda: {
                    "interfaces": ["lo"],
                    "ipv4_default_route": False,
                    "ipv6_default_route": False,
                    "platform": "linux",
                    "verified": True,
                },
            )
            self.assertEqual(
                [execution[0] for execution in executions],
                [
                    [str(runtime_root / "bin/python3"), "--version"],
                    [str(runtime_root / "bin/sigrok-cli"), "--version"],
                    [str(runtime_root / "bin/sigrok-cli"), "--scan"],
                ],
            )
            for _, options in executions:
                self.assertTrue(options["check"])
                self.assertTrue(options["capture_output"])
                self.assertTrue(options["text"])
                self.assertEqual(options["env"]["PATH"], str(runtime_root / "bin"))
                self.assertEqual(options["timeout"], 30)
            self.assertEqual((output / "runtime/bin/python3").read_bytes(), elf_fixture("amd64"))
            self.assertEqual((output / "runtime/bin/sigrok-cli").read_bytes(), elf_fixture("amd64"))
            runner = (output / "runner").read_text(encoding="utf-8")
            self.assertTrue(runner.startswith("#!/bin/sh\n"))
            self.assertIn("runtime/bin/python3", runner)
            metadata = json.loads((output / "runtime.json").read_text(encoding="utf-8"))
            self.assertEqual(metadata["backend"], "runtime/bin/sigrok-cli")
            self.assertTrue(metadata["self_contained"])
            self.assertGreaterEqual(metadata["maximum_bytes"], metadata["measured_bytes"])
            index = json.loads((output / "files.sha256.json").read_text(encoding="utf-8"))
            self.assertIn("runtime/bin/python3", index)
            self.assertIn("runtime/THIRD_PARTY_NOTICES.txt", index)
            health = json.loads((output / "runtime-health.json").read_text(encoding="utf-8"))
            self.assertEqual([item["id"] for item in health["checks"]], [
                "python-version",
                "sigrok-version",
                "sigrok-driver-scan",
            ])
            self.assertTrue(all(item["status"] == "passed" for item in health["checks"]))
            self.assertEqual(health["network_isolation"]["interfaces"], ["lo"])

            source = SIGROK_BUILDER_PATH.read_text(encoding="utf-8")
            self.assertNotIn('"uv"', source)
            self.assertNotIn('"pip"', source)

    def test_sigrok_bundle_refuses_incomplete_or_fixture_runtimes(self) -> None:
        builder = load_sigrok_builder()
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            runtime_root = root / "runtime-input"
            (runtime_root / "bin").mkdir(parents=True)
            (runtime_root / "bin/python3").write_bytes(elf_fixture("arm64"))
            (runtime_root / "THIRD_PARTY_NOTICES.txt").write_text("notices\n", encoding="utf-8")
            write_runtime_manifest(runtime_root, "arm64")
            with self.assertRaisesRegex(ValueError, "sigrok-cli"):
                builder.build(root / "missing", "arm64", runtime_root, runner=lambda *_args, **_kwargs: None)
            (runtime_root / "bin/sigrok-cli").write_bytes(elf_fixture("arm64"))
            (runtime_root / "fake_python").write_bytes(b"fixture")
            write_runtime_manifest(runtime_root, "arm64")
            with self.assertRaisesRegex(ValueError, "fixture"):
                builder.build(root / "fixture", "arm64", runtime_root, runner=lambda *_args, **_kwargs: None)

    def test_sigrok_bundle_rejects_failed_executed_health_check(self) -> None:
        builder = load_sigrok_builder()
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            runtime_root = root / "runtime-input"
            for relative, contents in {
                "bin/python3": elf_fixture("amd64"),
                "bin/sigrok-cli": elf_fixture("amd64"),
                "THIRD_PARTY_NOTICES.txt": b"runtime notices\n",
            }.items():
                path = runtime_root / relative
                path.parent.mkdir(parents=True, exist_ok=True)
                path.write_bytes(contents)
            write_runtime_manifest(runtime_root, "amd64")

            def fail_health(arguments, **_options):
                raise subprocess.CalledProcessError(2, arguments, stderr="unhealthy")

            output = root / "sigrok"
            with self.assertRaisesRegex(ValueError, "health check failed"):
                builder.build(
                    output,
                    "amd64",
                    runtime_root,
                    runner=fail_health,
                    network_observer=lambda: {
                        "interfaces": ["lo"],
                        "ipv4_default_route": False,
                        "ipv6_default_route": False,
                        "platform": "linux",
                        "verified": True,
                    },
                )
            self.assertFalse(output.exists())

    def test_sigrok_bundle_rejects_health_checks_in_a_routed_namespace(self) -> None:
        builder = load_sigrok_builder()
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            runtime_root = root / "runtime-input"
            for relative, contents in {
                "bin/python3": elf_fixture("amd64"),
                "bin/sigrok-cli": elf_fixture("amd64"),
                "THIRD_PARTY_NOTICES.txt": b"runtime notices\n",
            }.items():
                path = runtime_root / relative
                path.parent.mkdir(parents=True, exist_ok=True)
                path.write_bytes(contents)
            write_runtime_manifest(runtime_root, "amd64")
            executions = []
            with self.assertRaisesRegex(ValueError, "loopback-only"):
                builder.build(
                    root / "output",
                    "amd64",
                    runtime_root,
                    runner=lambda command, **options: executions.append((command, options)),
                    network_observer=lambda: {
                        "interfaces": ["eth0", "lo"],
                        "ipv4_default_route": True,
                        "ipv6_default_route": False,
                        "platform": "linux",
                        "verified": True,
                    },
                )
            self.assertEqual(executions, [])


class OperatorScriptTests(unittest.TestCase):
    def test_candidate_builder_uses_archived_revision_and_pinned_tools(self) -> None:
        source = (PACKAGING_ROOT / "build" / "build-candidate-set.sh").read_text(
            encoding="utf-8"
        )
        self.assertIn('version=$(tr -d \'\\r\\n\' <"$work/source/VERSION")', source)
        self.assertIn('. "$work/source/02_Tools/toolchain.versions"', source)
        self.assertIn('$($go_command version)', source)
        self.assertIn('$($node_command --version)', source)
        self.assertIn('$($npm_command --version)', source)
        self.assertNotIn('$source_root/02_Tools/env/with-go.sh', source)
        self.assertIn('build-camera-bundle.py', source)
        self.assertIn('--runtime-root "$ubuntu_sigrok_runtime"', source)
        self.assertIn('--runtime-root "$pi_sigrok_runtime"', source)

    @unittest.skipUnless(os.name == "posix", "target harness execution requires a POSIX host")
    def test_target_acceptance_harness_constructs_isolated_exact_target_command(self) -> None:
        harness = REPOSITORY_ROOT / "02_Tools" / "tests" / "packaging" / "run-target-acceptance.sh"
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            artifacts = root / "artifacts"
            artifacts.mkdir()
            engine = root / "fixture-engine"
            log = root / "engine.log"
            engine.write_text(
                "#!/bin/sh\n"
                "printf '%s\\n' \"$*\" >>\"$NILO_FAKE_ENGINE_LOG\"\n"
                "case \"$1\" in\n"
                "  image) printf '%s\\n' 'sha256:" + "b" * 64 + "';;\n"
                "  run|rm) :;;\n"
                "  exec)\n"
                "    case \"${3:-}\" in\n"
                "      systemctl) :;;\n"
                "      uname) printf '%s\\n' '6.8.0-fixture';;\n"
                "      sh) printf '%s\\n' 'ubuntu:24.04';;\n"
                "      dpkg) printf '%s\\n' 'amd64';;\n"
                "      *) :;;\n"
                "    esac;;\n"
                "  *) exit 2;;\n"
                "esac\n",
                encoding="utf-8",
            )
            engine.chmod(0o755)
            image = "fixture.invalid/ubuntu@sha256:" + "a" * 64
            environment = dict(os.environ)
            environment.update(
                {
                    "NILO_ACCEPTANCE_CONTAINER_ENGINE": str(engine),
                    "NILO_ACCEPTANCE_EXPECTED_IMAGE": image,
                    "NILO_ACCEPTANCE_IMAGE_UBUNTU_24_04_AMD64": image,
                    "NILO_FAKE_ENGINE_LOG": str(log),
                }
            )
            completed = subprocess.run(
                [
                    "sh", str(harness), "--target", "ubuntu-24.04-amd64",
                    "--check", "package-install", "--artifacts", str(artifacts),
                ],
                check=False,
                capture_output=True,
                env=environment,
                text=True,
            )
            self.assertEqual(completed.returncode, 0, completed.stderr)
            self.assertIn(f"NILO_ACCEPTANCE_IMAGE_REFERENCE={image}", completed.stdout)
            self.assertIn("NILO_ACCEPTANCE_KERNEL=6.8.0-fixture", completed.stdout)
            self.assertIn("NILO_ACCEPTANCE_OS_ID=ubuntu", completed.stdout)
            self.assertIn("NILO_ACCEPTANCE_OS_VERSION=24.04", completed.stdout)
            self.assertIn("NILO_ACCEPTANCE_ARCHITECTURE=amd64", completed.stdout)
            engine_log = log.read_text(encoding="utf-8")
            self.assertIn("--network none", engine_log)
            self.assertIn("--platform linux/amd64", engine_log)
            self.assertIn("--privileged", engine_log)
            self.assertIn("NILO_ACCEPTANCE_DISPOSABLE_TARGET=ubuntu-24.04-amd64", engine_log)

    def test_packaging_entrypoints_cover_each_delivery_artifact(self) -> None:
        expected = {
            "build/build-candidate-set.sh": "build-set-receipt",
            "build/build-payload.sh": "release_builder.py\" payload",
            "build/build-deb.sh": "release_builder.py\" deb",
            "build/build-offline-installer.sh": "release_builder.py\" installer",
            "build/build-pi-image.sh": "release_builder.py\" image",
            "build/build-release.sh": "release_builder.py\" release",
            "verify/verify-release.sh": "release_builder.py\" verify",
            "verify/verify-deb.sh": "dpkg-deb --info",
            "verify/verify-installer.sh": "--inspect",
            "verify/verify-image.sh": "xz -t",
        }
        for relative, marker in expected.items():
            with self.subTest(script=relative):
                path = PACKAGING_ROOT / relative
                self.assertTrue(path.is_file(), f"missing packaging entrypoint: {relative}")
                source = path.read_text(encoding="utf-8")
                self.assertIn("set -eu", source)
                self.assertIn(marker, source)
                self.assertNotIn("curl ", source)
                self.assertNotIn("wget ", source)

    def test_readme_describes_connected_acquisition_and_offline_build(self) -> None:
        readme = (PACKAGING_ROOT / "README.md").read_text(encoding="utf-8")
        self.assertNotIn("It does not yet build", readme)
        self.assertIn("Connected acquisition", readme)
        self.assertIn("Offline artifact build", readme)
        self.assertIn("not_run", readme)

    def test_fixed_acceptance_executes_distinct_package_and_integration_lifecycles(self) -> None:
        lifecycle = (
            REPOSITORY_ROOT / "02_Tools" / "tests" / "packaging" / "target-lifecycle-check.sh"
        ).read_text(encoding="utf-8")
        self.assertIn('dpkg -i "$deb"', lifecycle)
        self.assertIn("prior-installer", lifecycle)
        self.assertIn("/api/v1/plugins/sigrok/install", lifecycle)
        self.assertIn("/api/v1/plugins/sigrok/upgrade", lifecycle)
        for check in (
            "installer-upgrade",
            "installer-clean-rollback",
            "installer-rollback-dependencies",
            "installer-rollback-package",
            "installer-rollback-migrated",
            "installer-rollback-started",
            "installer-rollback-verified",
        ):
            self.assertIn(check, lifecycle)

        harness = (
            REPOSITORY_ROOT / "02_Tools" / "tests" / "packaging" / "run-target-acceptance.sh"
        ).read_text(encoding="utf-8")
        self.assertIn("--network none", harness)
        self.assertIn("--platform", harness)
        self.assertIn("--privileged", harness)
        self.assertIn("NILO_ACCEPTANCE_IMAGE_UBUNTU_24_04_AMD64", harness)
        self.assertIn("NILO_ACCEPTANCE_IMAGE_RPI_DEBIAN13_ARM64", harness)
        self.assertIn("NILO_ACCEPTANCE_DISPOSABLE_TARGET", harness)
        self.assertIn("NILO_ACCEPTANCE_IMAGE_REFERENCE", harness)
        self.assertIn("NILO_ACCEPTANCE_IMAGE_ID", harness)
        self.assertIn("NILO_ACCEPTANCE_KERNEL", harness)
        self.assertIn("NILO_ACCEPTANCE_FAIL_OPERATION", lifecycle)
        driver = load_builder()._installer_driver("ubuntu-24.04-amd64", "0.8.0", []).decode("utf-8")
        self.assertIn('dpkg --unpack "$fault_dependency"', driver)
        self.assertIn('dpkg --unpack "$nilo_package"', driver)
        self.assertIn("NILO_ACCEPTANCE_FORCE_RECOVERY_REQUIRED", driver)
        for phase in (
            "preflight", "quiesced", "snapshotted", "dependencies",
            "package", "migrated", "started", "verified", "committed",
            "rolling_back", "rolled_back", "recovery-required",
        ):
            self.assertIn(f"installer-interruption-{phase}", lifecycle)

    def test_artifact_builders_require_a_trusted_input_lock(self) -> None:
        builder = load_builder()
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            receipt = offline_receipt(root, "ubuntu-24.04-amd64")
            with self.assertRaisesRegex(builder.ReleaseError, "trusted input lock"):
                builder._validate_offline_receipt(
                    receipt, "ubuntu-24.04-amd64"
                )


if __name__ == "__main__":
    unittest.main()
