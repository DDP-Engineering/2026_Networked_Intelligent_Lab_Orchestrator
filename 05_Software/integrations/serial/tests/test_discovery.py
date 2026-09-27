#!/usr/bin/env python3
# -*- coding: utf-8 -*-
# -----------------------------------------------------------------------------
# Copyright (c) 2026 DDP Engineering
# SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
# -----------------------------------------------------------------------------
"""@file test_discovery.py
@brief Verify USB serial discovery metadata and endpoint binding.
@details Portable metadata fixtures model resolved sysfs ancestry without opening ports.
"""

import importlib.util
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch


class DiscoveryAvailability(unittest.TestCase):
    def test_usb_discovery_is_available(self):
        self.assertIsNotNone(
            importlib.util.find_spec("nilo_serial"), "USB-only serial discovery is unavailable"
        )

    def test_usb_metadata_separates_interfaces_without_open(self):
        self.test_usb_discovery_is_available()
        from nilo_serial.discovery import resolve_port, scan_ports

        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            usb = root / "devices/1-2"
            usb.mkdir(parents=True)
            for name, value in {
                "idVendor": "0403",
                "idProduct": "6010",
                "serial": "adapter",
                "product": "USB serial",
            }.items():
                (usb / name).write_text(value)
            tty = root / "tty"
            tty.mkdir()
            resolved = {}
            for index in range(2):
                interface = usb / f"interface-{index}"
                interface.mkdir()
                (interface / "bInterfaceNumber").write_text(f"{index:02x}")
                endpoint = interface / f"ttyUSB{index}"
                endpoint.mkdir()
                (endpoint / "port_number").write_text("0")
                entry = tty / f"ttyUSB{index}"
                entry.mkdir()
                resolved[entry] = endpoint
            (tty / "ttyS0").mkdir()
            # Supply resolved sysfs links without requiring Windows symlink privileges.
            with patch.object(Path, "resolve", lambda entry: resolved.get(entry, entry)):
                devices = scan_ports(tty)
                self.assertEqual(len(devices), 2)
                self.assertEqual(devices[0]["hardware_identity"]["serial"], "adapter/if00/port0")
                self.assertNotEqual(
                    devices[0]["hardware_identity"], devices[1]["hardware_identity"]
                )
                self.assertEqual(
                    resolve_port(devices[0]["hardware_identity"], "ttyUSB0", tty), "/dev/ttyUSB0"
                )
                with self.assertRaises(ValueError):
                    resolve_port(devices[0]["hardware_identity"], "ttyUSB1", tty)
                (usb / "serial").write_text("00000000")
                self.assertTrue(
                    all(item["hardware_identity"]["serial"] == "" for item in scan_ports(tty))
                )
