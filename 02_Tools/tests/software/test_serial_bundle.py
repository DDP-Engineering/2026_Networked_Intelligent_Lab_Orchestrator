#!/usr/bin/env python3
# -*- coding: utf-8 -*-
# -----------------------------------------------------------------------------
# Copyright (c) 2026 DDP Engineering
# SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
# -----------------------------------------------------------------------------
"""@file test_serial_bundle.py
@brief Verify serial packaging input pins and target access policy.
@details Inspect controlled fixture inputs without installing appliance services.
"""

import importlib.util
from pathlib import Path
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[3]


def load(script):
    """@brief Load a repository-owned build helper for isolated input checks."""
    spec = importlib.util.spec_from_file_location(
        "serial_dependency", ROOT / "02_Tools/scripts" / script
    )
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


class SerialBundleTests(unittest.TestCase):
    """@brief Cover offline dependency refusal and non-root serial group access."""

    def test_offline_dependency_refuses_missing_or_changed_wheel(self):
        dependency = load("prepare-serial-dependency.py")
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            with self.assertRaisesRegex(ValueError, "missing"):
                dependency.prepare(root, offline=True)
            (root / dependency.NAME).write_bytes(b"changed")
            with self.assertRaisesRegex(ValueError, "checksum"):
                dependency.prepare(root, offline=True)

    def test_bundle_refuses_missing_inputs_and_unknown_target(self):
        builder = load("build-serial-bundle.py")
        project = ROOT / "05_Software/integrations/serial"
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            with self.assertRaisesRegex(ValueError, "Unsupported"):
                builder.build(project, root, root, root / "out", "x86")
            with self.assertRaisesRegex(ValueError, "missing"):
                builder.build(project, root, root, root / "out", "arm64")

    def test_nonroot_service_receives_usb_only_group_access(self):
        unit = (ROOT / "02_Tools/packaging/debian/units/nilo.service").read_text()
        self.assertIn("User=nilo", unit)
        self.assertIn("SupplementaryGroups=video dialout", unit)
        rules = (
            ROOT / "02_Tools/packaging/files/60-nilo-instruments.rules"
        ).read_text()
        self.assertIn(
            'SUBSYSTEM=="tty", SUBSYSTEMS=="usb", GROUP="dialout", MODE="0660"', rules
        )
