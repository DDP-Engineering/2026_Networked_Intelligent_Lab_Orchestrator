#!/usr/bin/env python3
# -*- coding: utf-8 -*-
# -----------------------------------------------------------------------------
# Copyright (c) 2026 DDP Engineering
# SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
# -----------------------------------------------------------------------------
"""Verify the offline Modbus target-bundle boundary."""

import hashlib
import importlib.util
import json
import os
import shutil
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[3]
PROJECT = ROOT / "05_Software/integrations/modbus"
PIN_ROOT = ROOT / "02_Tools/cache/modbus-feasibility"


def pinned_input(filename):
    """Locate separately acquired offline inputs on host or canonical image."""
    kind = "python-packages" if filename.endswith(".whl") else "license-sources"
    for candidate in (PIN_ROOT / filename, Path('/opt/nilo/inputs') / kind / filename):
        if candidate.is_file():
            return candidate
    raise unittest.SkipTest("verified Modbus acquisition inputs are unavailable")


def load_builder():
    """Load the repository builder without changing sys.path."""
    path = ROOT / "02_Tools/scripts/build-modbus-bundle.py"
    spec = importlib.util.spec_from_file_location("modbus_bundle_builder", path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


class ModbusBundleTests(unittest.TestCase):
    """Exercise publication and corrupted-input refusal."""

    def test_rejects_missing_and_changed_offline_inputs(self):
        builder = load_builder()
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            with self.assertRaisesRegex(ValueError, "architecture"):
                builder.build(PROJECT, root, root, root / "out", "x86")
            with self.assertRaisesRegex(ValueError, "missing"):
                builder.build(PROJECT, root, root, root / "out", "arm64")
            self.assertFalse((root / "out").exists())
            wheel = root / "pymodbus-3.15.0-py3-none-any.whl"
            wheel.write_bytes(b"changed")
            with self.assertRaisesRegex(ValueError, "checksum"):
                builder.build(PROJECT, root, root, root / "out", "arm64")
            self.assertFalse((root / "out").exists())

    def test_appliance_build_has_explicit_modbus_bundle_switch(self):
        script = (ROOT / "02_Tools/scripts/build-appliance.sh").read_text()
        self.assertIn('NILO_BUNDLE_MODBUS', script)
        self.assertIn('build-modbus-bundle.py', script)

    @unittest.skipIf(os.name == "nt", "Windows symlink setup requires host privileges")
    def test_rejects_linked_wheelhouse_before_license_preparation(self):
        builder = load_builder()
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            wheelhouse = root / "wheels"
            wheelhouse.mkdir()
            (wheelhouse / "licenses").symlink_to(root, target_is_directory=True)
            with self.assertRaisesRegex(ValueError, "Linked or special"):
                builder.build(PROJECT, root, wheelhouse, root / "out", "arm64")
            self.assertFalse((root / "out").exists())

    def test_builds_indexed_readonly_bundle_from_pinned_inputs(self):
        builder = load_builder()
        pins = json.loads((PROJECT / "evidence/dependencies.json").read_text())
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            wheelhouse = root / "wheels"
            wheelhouse.mkdir()
            for package in pins["packages"]:
                for artifact in package["artifacts"]:
                    shutil.copy2(pinned_input(artifact["filename"]), wheelhouse / artifact["filename"])
            dependencies = root / "dependencies"
            for name in ("nilo_integration", "pydantic", "pydantic_core", "annotated_types", "typing_inspection"):
                (dependencies / name).mkdir(parents=True)
                (dependencies / name / "__init__.py").write_text("\n")
            (dependencies / "typing_extensions.py").write_text("\n")
            native = dependencies / "pydantic_core" / "_core.so"
            native.write_bytes(b"\x7fELF\x02\x01" + b"\0" * 12 + (183).to_bytes(2, "little"))
            (root / "THIRD_PARTY_NOTICES.txt").write_text("Pydantic dependency notices\n")
            output = root / "out"
            builder.build(PROJECT, dependencies, wheelhouse, output, "arm64")
            inventory = json.loads((output / "files.sha256.json").read_text())
            manifest = json.loads((output / "manifest.json").read_text())
            self.assertIn("python/pymodbus/__init__.py", inventory)
            self.assertIn("python/serial/__init__.py", inventory)
            self.assertIn("licenses/pyserial-LICENSE.txt", inventory)
            self.assertEqual(manifest["platforms"], [{"os": "linux", "architecture": "arm64"}])
            self.assertTrue((output / "runner").read_text().startswith("#!/usr/bin/env python3"))
            if os.name != "nt":
                self.assertTrue((output / "runner").stat().st_mode & 0o111)
            with self.assertRaisesRegex(ValueError, "already exists"):
                builder.build(PROJECT, dependencies, wheelhouse, output, "arm64")

    def test_runtime_manifest_rejects_changed_target_bytes(self):
        builder = load_builder()
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            runtime = root / "runtime"
            (runtime / "bin").mkdir(parents=True)
            binary = runtime / "bin/python3"
            binary.write_bytes(b"\x7fELF\x02\x01" + b"\0" * 12 + (183).to_bytes(2, "little"))
            notice = runtime / "THIRD_PARTY_NOTICES.txt"
            notice.write_text("runtime licenses\n")
            inventory = [
                {"path": path.relative_to(runtime).as_posix(), "sha256": hashlib.sha256(path.read_bytes()).hexdigest(), "size": path.stat().st_size}
                for path in sorted((binary, notice))
            ]
            (runtime / "runtime-manifest.json").write_text(json.dumps({"architecture": "arm64", "files": inventory}))
            binary.write_bytes(binary.read_bytes() + b"changed")
            with self.assertRaisesRegex(ValueError, "manifest"):
                builder._copy_runtime(runtime, root / "copy", "arm64")
            self.assertFalse((root / "copy").exists())


if __name__ == "__main__":
    unittest.main()
