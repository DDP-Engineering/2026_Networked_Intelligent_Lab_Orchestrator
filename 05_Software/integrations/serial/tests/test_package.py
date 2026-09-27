#!/usr/bin/env python3
# -*- coding: utf-8 -*-
# -----------------------------------------------------------------------------
# Copyright (c) 2026 DDP Engineering
# SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
# -----------------------------------------------------------------------------
"""@file test_package.py
@brief Verify the supervised serial package contract.
@details Check the finite declared operation inventory without loading platform I/O.
"""

import json
import unittest
from pathlib import Path


class PackageTests(unittest.TestCase):
    def test_serial_package_exposes_discovery_connect_and_private_stream(self):
        root = Path(__file__).resolve().parents[1]
        self.assertTrue((root / "manifest.json").is_file(), "Serial package is unavailable")
        manifest = json.loads((root / "manifest.json").read_text())
        self.assertEqual(manifest["package"]["id"], "serial")
        self.assertEqual(
            {op["id"] for op in manifest["operations"]}, {"discover_devices", "connect", "stream"}
        )
        self.assertEqual(manifest["artifacts"], [])
