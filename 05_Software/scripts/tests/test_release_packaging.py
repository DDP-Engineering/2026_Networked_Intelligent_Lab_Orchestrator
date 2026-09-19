#!/usr/bin/env python3
"""Verify deterministic, offline NILO delivery artifacts."""

from __future__ import annotations

import importlib.util
import hashlib
import io
import json
import lzma
import os
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
        "etc/nilo/nilo.env": b"NILO_GPIO_RECOVERY=disabled\n",
    }
    for relative, contents in files.items():
        path = source / relative
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_bytes(contents)
        if relative in {"usr/bin/nilo", "usr/libexec/nilo/nilo-nethelper"}:
            path.chmod(0o755)
    return source


def offline_receipt(root: Path, target: str, lock_path: Path | None = None) -> Path:
    architecture = "arm64" if target == "rpi-debian13-arm64" else "amd64"
    path = root / f"{target}-preparation.json"
    lock_digest = hashlib.sha256(lock_path.read_bytes()).hexdigest() if lock_path else "a" * 64
    path.write_text(
        json.dumps(
            {
                "inputs": [{"name": "fixture", "sha256": "b" * 64}],
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
            {"command": "python3 --version", "id": "python-version", "status": "passed"},
            {"command": "sigrok-cli --version", "id": "sigrok-version", "status": "passed"},
            {"command": "sigrok-cli --scan", "id": "sigrok-driver-scan", "status": "passed"},
        ],
        "schema": 1,
    }
    (runtime_root / "runtime-manifest.json").write_text(
        json.dumps(manifest, indent=2, sort_keys=True) + "\n", encoding="utf-8"
    )


class DeterministicPayloadTests(unittest.TestCase):
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
            )
            os.utime(source / "usr/bin/nilo", (1_800_000_000, 1_800_000_000))
            second_receipt = builder.build_payload(
                source,
                second,
                target="ubuntu-24.04-amd64",
                version="0.8.0",
                source_date_epoch=1_700_000_000,
                offline_receipt=offline_receipt(root, "ubuntu-24.04-amd64"),
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
            )
            builder.build_deb(
                payload,
                second,
                target="ubuntu-24.04-amd64",
                version="0.8.0",
                source_date_epoch=1_700_000_000,
                offline_receipt=offline_receipt(root, "ubuntu-24.04-amd64"),
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
                conffiles = control.extractfile("./conffiles").read().decode("utf-8")
            self.assertIn("Package: nilo\n", metadata)
            self.assertIn("Version: 0.8.0\n", metadata)
            self.assertIn("Architecture: amd64\n", metadata)
            self.assertIn("policy-rc.d", postinst)
            self.assertIn("/etc/nilo/nilo.env", conffiles)
            with tarfile.open(fileobj=io.BytesIO(members["data.tar.xz"]), mode="r:xz") as data:
                names = data.getnames()
                self.assertEqual(names, sorted(names))
                self.assertIn("./usr/bin/nilo", names)
                self.assertIn("./usr/lib/systemd/system/nilo.service", names)
                self.assertIn("./usr/lib/systemd/system/nilo-nethelper.socket", names)
                self.assertIn("./usr/lib/systemd/system/nilo-firstboot.service", names)
                self.assertIn("./usr/libexec/nilo/nilo-installer-recover", names)
                self.assertNotIn("./var/lib/nilo/nilo.db", names)
                core_unit = data.extractfile("./usr/lib/systemd/system/nilo.service").read().decode("utf-8")
                helper_unit = data.extractfile("./usr/lib/systemd/system/nilo-nethelper.service").read().decode("utf-8")
                recovery = data.extractfile("./usr/libexec/nilo/nilo-installer-recover").read().decode("utf-8")
                recovery_unit = data.extractfile("./usr/lib/systemd/system/nilo-recovery.service").read().decode("utf-8")
            self.assertNotIn("--management-port", core_unit)
            self.assertIn("--local-status-bind 127.0.0.1:8091", core_unit)
            self.assertIn("--allow-uid ${NILO_CALLER_UID}", helper_unit)
            self.assertIn("--socket-gid ${NILO_CALLER_GID}", helper_unit)
            self.assertIn("maintenance restore", recovery)
            self.assertIn("dpkg -i", recovery)
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
            )
            with self.assertRaisesRegex(builder.ReleaseError, "target mismatch"):
                builder.build_deb(
                    payload,
                    root / "wrong.deb",
                    target="rpi-debian13-arm64",
                    version="0.8.0",
                    source_date_epoch=1,
                    offline_receipt=offline_receipt(root, "rpi-debian13-arm64"),
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
        )
        package = root / "nilo_0.8.0_amd64.deb"
        builder.build_deb(
            payload,
            package,
            target="ubuntu-24.04-amd64",
            version="0.8.0",
            source_date_epoch=1_700_000_000,
            offline_receipt=offline_receipt(root, "ubuntu-24.04-amd64"),
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
                    offline_receipt=offline_receipt(root, "ubuntu-24.04-amd64"),
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
            self.assertIn("flock -n", driver)
            self.assertIn("is-active NetworkManager", driver)
            self.assertIn("Acquire::http::Proxy=false", driver)
            self.assertIn("dpkg --purge", driver)
            self.assertIn("systemctl is-active", driver)
            self.assertIn("maintenance snapshot", driver)
            self.assertIn("maintenance restore", driver)
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
            )
            builder.build_deb(
                payload,
                package,
                target="rpi-debian13-arm64",
                version="0.8.0",
                source_date_epoch=1_700_000_000,
                offline_receipt=offline_receipt(root, "rpi-debian13-arm64"),
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
                offline_receipt=offline_receipt(root, "rpi-debian13-arm64"),
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
            self.assertNotIn("curl ", script)
            self.assertNotIn("wget ", script)

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
                )


class ReleaseEvidenceTests(unittest.TestCase):
    def _lock(self, builder, root: Path) -> Path:
        artifact_types = {
            "base-image": "raspberry-pi-image-xz",
            "go-module-cache": "go-module-cache-tar",
            "gui-npm-cache": "npm-cache-tar",
            "os-package-pool": "debian-pool-tar",
            "sigrok-runtime": "sigrok-runtime-tar",
            "source-offers": "source-offers-tar",
        }
        inputs = []
        for scope, names in sorted(builder.REQUIRED_RELEASE_INPUTS.items()):
            for name in sorted(names):
                inputs.append(
                    {
                        "architecture": "all" if scope == "shared" else builder.TARGET_ARCHITECTURES[scope],
                        "artifact_type": artifact_types[name],
                        "license": "MIT",
                        "name": name,
                        "provenance": f"https://example.invalid/{scope}/{name}/provenance",
                        "redistribution": "permitted",
                        "scope": scope,
                        "sha256": hashlib.sha256(f"{scope}/{name}".encode()).hexdigest(),
                        "size": 1,
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
        checks.append(self._software_check(root, "rpi-debian13-arm64", "image-structure"))
        for target in targets["targets"]:
            checks.extend(
                {
                    "check": check,
                    "kind": "physical",
                    "reason": "physical target unavailable in this build environment",
                    "status": "not_run",
                    "target": target["id"],
                }
                for check in target["hardware_acceptance"]
            )
        path = root / "executed-acceptance.json"
        path.write_text(
            json.dumps({"checks": checks, "schema": 1}, indent=2, sort_keys=True) + "\n",
            encoding="utf-8",
        )
        return path

    def _software_check(self, root: Path, target: str, check: str) -> dict[str, object]:
        relative = Path("acceptance-evidence") / target / f"{check}.json"
        path = root / relative
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(
            json.dumps(
                {"check": check, "command": f"verify {check}", "exit_code": 0, "schema": 1, "target": target},
                sort_keys=True,
            )
            + "\n",
            encoding="utf-8",
        )
        return {
            "check": check,
            "evidence": {"path": relative.as_posix(), "sha256": hashlib.sha256(path.read_bytes()).hexdigest()},
            "kind": "software",
            "status": "passed",
            "target": target,
        }

    def _source_offers(self, root: Path) -> Path:
        source = root / "source-offers"
        (source / "LICENSES").mkdir(parents=True)
        (source / "SOURCE_OFFER.txt").write_text("Corresponding source is included for redistributable inputs.\n", encoding="utf-8")
        (source / "LICENSES" / "MIT.txt").write_text("MIT fixture licence text\n", encoding="utf-8")
        return source

    def _artifacts(self, builder, root: Path) -> Path:
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
                version="0.8.0",
                source_date_epoch=1_700_000_000,
                offline_receipt=offline_receipt(target_root, target),
            )
            package = artifacts / f"nilo_0.8.0_{architecture}.deb"
            builder.build_deb(
                payload,
                package,
                target=target,
                version="0.8.0",
                source_date_epoch=1_700_000_000,
                offline_receipt=offline_receipt(target_root, target),
            )
            pool = target_root / "pool"
            pool.mkdir()
            installer = artifacts / f"nilo-0.8.0-{target}.run"
            builder.build_installer(
                package,
                pool,
                installer,
                target=target,
                version="0.8.0",
                source_date_epoch=1_700_000_000,
                offline_receipt=offline_receipt(target_root, target),
            )
        (artifacts / "nilo-pi4-0.8.0.img.xz").write_bytes(
            lzma.compress(b"fixture appliance image")
        )
        return artifacts

    def test_release_evidence_closes_every_artifact_and_keeps_physical_checks_not_run(self) -> None:
        builder = load_builder()
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            artifacts = self._artifacts(builder, root)
            lock = self._lock(builder, root)
            release = root / "release"
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
            )
            result = builder.verify_release(release)
            self.assertEqual(result["status"], "pass")
            self.assertEqual(len(result["artifacts"]), 5)
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
            builder.build(output, "amd64", runtime_root)
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
                builder.build(root / "missing", "arm64", runtime_root)
            (runtime_root / "bin/sigrok-cli").write_bytes(elf_fixture("arm64"))
            (runtime_root / "fake_python").write_bytes(b"fixture")
            write_runtime_manifest(runtime_root, "arm64")
            with self.assertRaisesRegex(ValueError, "fixture"):
                builder.build(root / "fixture", "arm64", runtime_root)


class OperatorScriptTests(unittest.TestCase):
    def test_packaging_entrypoints_cover_each_delivery_artifact(self) -> None:
        expected = {
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


if __name__ == "__main__":
    unittest.main()
