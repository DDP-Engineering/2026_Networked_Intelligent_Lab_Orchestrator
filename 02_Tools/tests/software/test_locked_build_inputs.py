#!/usr/bin/env python3
# -*- coding: utf-8 -*-
# -----------------------------------------------------------------------------
# Copyright (c) 2026 DDP Engineering
# Licensed under the PolyForm Noncommercial License 1.0.0.
# See the repository LICENSE.txt for full license details.
# SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
# -----------------------------------------------------------------------------
"""
@file test_locked_build_inputs.py
@brief Test source-lock closure and preparation failure boundaries.
@details Missing or corrupt inputs must fail before an environment is cleared.
"""

import hashlib
import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from unittest import mock

ROOT = Path(__file__).resolve().parents[3]
sys.path.insert(0, str(ROOT / "02_Tools/scripts"))
import locked_build_inputs as subject


class LockedBuildInputTests(unittest.TestCase):
    """@brief Exercise locked identity, checksum and early-failure contracts."""

    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        self.project = self.root / "05_Software/project"
        self.project.mkdir(parents=True)
        (self.project / "pyproject.toml").write_text('[project]\nname="demo"\nversion="1.0"\n')
        self.payload = b"verified wheel fixture"
        self.digest = hashlib.sha256(self.payload).hexdigest()
        self.item = dict(name="demo", version="1.0", kind="python-packages",
                         cache_name="python-packages/demo-1.0-py3-none-any.whl",
                         sha256=self.digest, size=len(self.payload),
                         source="https://example.invalid/demo-1.0-py3-none-any.whl")
        (self.project / "uv.lock").write_text(
            '[[package]]\nname="demo"\nversion="1.0"\n'
            'source={registry="https://pypi.org/simple"}\n'
            f'wheels=[{{url="{self.item["source"]}", hash="sha256:{self.digest}", size={len(self.payload)}}}]\n'
        )

    def test_missing_go_object_or_checksum_refuses_preparation(self):
        (self.project / "go.mod").write_text("module demo\nrequire example.invalid/module v1.0.0\n")
        (self.project / "go.sum").write_text("example.invalid/module v1.0.0 h1:fixture\n")
        with self.assertRaisesRegex(subject.BuildEnvironmentError, "missing locked Go input"):
            subject.verify_coverage(self.root, [self.item])
        (self.project / "go.sum").write_text("")
        with self.assertRaisesRegex(subject.BuildEnvironmentError, "absent from go.sum"):
            subject.verify_coverage(self.root, [self.item])

    def test_registry_wheel_requires_lock_hash_version_and_source(self):
        subject.verify_coverage(self.root, [self.item])
        for field, value in [("sha256", "0" * 64), ("version", "2.0"), ("source", "https://example.invalid/other.whl")]:
            with self.subTest(field=field), self.assertRaisesRegex(subject.BuildEnvironmentError, "missing locked Linux"):
                subject.verify_coverage(self.root, [{**self.item, field: value}])

    def test_missing_or_corrupt_wheel_fails_before_environment_creation(self):
        inputs = self.root / "inputs"
        path = inputs / self.item["cache_name"]
        with mock.patch.object(subject.subprocess, "run") as command:
            with self.assertRaisesRegex(subject.BuildEnvironmentError, "missing locked Python input"):
                subject.prepare_python(self.root, [self.item], inputs, "python", "uv")
            path.parent.mkdir(parents=True)
            path.write_bytes(b"corrupt")
            with self.assertRaisesRegex(subject.BuildEnvironmentError, "checksum mismatch"):
                subject.prepare_python(self.root, [self.item], inputs, "python", "uv")
            command.assert_not_called()

    def test_git_source_requires_exact_revision_and_build_tool_closure(self):
        revision = "a" * 40
        (self.project / "uv.lock").write_text(
            '[[package]]\nname="binding"\nversion="1.0"\n'
            f'source={{git="https://example.invalid/binding.git?rev={revision}#{revision}"}}\n'
        )
        item = dict(name="binding", version="1.0", kind="python-sources",
                    git="https://example.invalid/binding.git", revision=revision,
                    build_requires=["setuptools==80.9.0", "wheel==0.45.1"])
        with self.assertRaisesRegex(subject.BuildEnvironmentError, "missing locked Git source"):
            subject.verify_coverage(self.root, [{**item, "revision": "b" * 40}])
        with self.assertRaisesRegex(subject.BuildEnvironmentError, "missing Git build tool"):
            subject.verify_coverage(self.root, [item])

    def test_declared_build_dependency_is_required_before_installation(self):
        (self.project / "pyproject.toml").write_text('[build-system]\nrequires=["uv_build==0.11.25"]\n')
        with self.assertRaisesRegex(subject.BuildEnvironmentError, "missing locked build dependency"):
            subject.verify_coverage(self.root, [self.item])

    def test_generated_dependency_trees_are_excluded(self):
        generated = self.project / ".venv-linux-x86_64/lib/package"
        generated.mkdir(parents=True)
        (generated / "pyproject.toml").write_text("not a source project")
        subject.verify_coverage(self.root, [self.item])

    def test_target_wheel_selection_accepts_abi3_and_rejects_other_platforms(self):
        for wheel in ["demo-1.0-py3-none-any.whl", "demo-1.0-cp39-abi3-manylinux_2_28_x86_64.whl"]:
            self.assertTrue(subject._compatible_wheel(wheel))
        for wheel in ["demo-1.0-cp313-cp313-manylinux_2_28_x86_64.whl", "demo-1.0-cp312-cp312-win_amd64.whl", "demo-1.0-cp312-cp312-manylinux_2_28_aarch64.whl"]:
            self.assertFalse(subject._compatible_wheel(wheel))

    def test_current_inventory_covers_every_source_lock(self):
        lock = ROOT / "02_Tools/containers/build/build-environment.lock.json"
        subject.verify_coverage(ROOT, subject._reconstruction_inputs(lock, require_https=True))

    def test_npm_lock_requires_the_exact_tarball_source_and_version(self):
        frontend = self.root / "05_Software/frontend"
        frontend.mkdir()
        source = "https://example.invalid/demo-1.0.tgz"
        (frontend / "package-lock.json").write_text(json.dumps({
            "packages": {"node_modules/demo": {"resolved": source, "version": "1.0"}}
        }))
        with self.assertRaisesRegex(subject.BuildEnvironmentError, "missing locked npm input"):
            subject.verify_coverage(self.root, [self.item])
        subject.verify_coverage(self.root, [self.item, {"kind": "npm", "source": source, "version": "1.0"}])

    def test_linux_entry_points_have_executable_index_modes(self):
        rows = subprocess.check_output(["git", "ls-files", "--stage", "*.sh", "*/runner"], cwd=ROOT, text=True)
        for row in rows.splitlines():
            metadata, path = row.split("\t", 1)
            if path.startswith(("02_Tools/", "05_Software/integrations/")):
                self.assertEqual(metadata.split()[0], "100755", path)


if __name__ == "__main__":
    unittest.main()

# END OF FILE
