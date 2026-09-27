#!/usr/bin/env python3
# -*- coding: utf-8 -*-
# -----------------------------------------------------------------------------
# Copyright (c) 2026 DDP Engineering
# SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
# -----------------------------------------------------------------------------
"""@file test_transport.py
@brief Exercise serial settings and Linux pseudo-terminal semantics.
@details Portable setting validation runs on each host; descriptor tests require Linux.
"""

import base64
import importlib.util
import os
import sys
import time
import unittest
from unittest.mock import patch


class TransportAvailability(unittest.TestCase):
    def test_transport_supports_bounded_serial_sessions(self):
        self.assertIsNotNone(
            importlib.util.find_spec("nilo_serial"), "Serial transport is unavailable"
        )

    @unittest.skipUnless(sys.platform == "linux", "Linux physical runner")
    def test_real_pty_exchange_exclusive_close_and_signals(self):
        self.test_transport_supports_bounded_serial_sessions()
        import pty

        from nilo_serial.transport import SerialError, SerialTransport

        master, slave = pty.openpty()
        path = os.ttyname(slave)
        port = SerialTransport(path, {})
        try:
            with self.assertRaises((SerialError, OSError)):
                SerialTransport(path, {})
            os.write(master, b"hello\x00\xff")
            result = port.read(0, 1000)
            self.assertEqual(base64.b64decode(result["events"][0]["data_base64"]), b"hello\x00\xff")
            sent = port.send(b"reply\x00")
            self.assertEqual(sent, {"status": "sent", "bytes_written": 6})
            self.assertEqual(os.read(master, 6), b"reply\x00")
            self.assertEqual(port.status()["last_sequence"], 2)
            self.assertIsNone(port.status()["signals"]["cts"])
            with patch("nilo_serial.transport.fcntl.ioctl", return_value=b"\0" * 4):
                self.assertFalse(port.status()["signals"]["rts"])
            with self.assertRaises(SerialError):
                port.set_signals({"rts": False})
            port.close()
            self.assertFalse(port.status()["connected"])
            # Linux PTYs silently coerce five data bits to eight; surface that mismatch.
            with self.assertRaises(SerialError):
                rejected = SerialTransport(path, {"data_bits": 5})
                rejected.close()
        finally:
            port.close()
            os.close(master)
            os.close(slave)

    @unittest.skipUnless(sys.platform == "linux", "Linux physical runner")
    def test_ring_bounds_and_gap(self):
        self.test_transport_supports_bounded_serial_sessions()
        from nilo_serial.transport import EventRing

        ring = EventRing()
        for _ in range(5000):
            ring.append("rx", b"x" * 4096)
        result = ring.read(0, 0)
        self.assertTrue(result["gap"])
        self.assertLessEqual(ring.byte_count, 1 << 20)
        self.assertLessEqual(len(ring.events), 4096)
        self.assertLessEqual(len(__import__("json").dumps(result).encode()), 65536)

    def test_settings_reject_invalid_values(self):
        from nilo_serial.settings import validate_settings

        self.assertEqual(validate_settings({})["baud_rate"], 115200)
        for settings in (
            {"baud_rate": True},
            {"stop_bits": 3},
            {"parity": "bad"},
            {"rts": "true"},
            {"extra": 1},
        ):
            with self.assertRaises(ValueError):
                validate_settings(settings)

    @unittest.skipUnless(sys.platform == "linux", "Linux physical runner")
    def test_backpressure_partial_counts_and_device_loss(self):
        import pty

        from nilo_serial.transport import SerialTransport

        master, slave = pty.openpty()
        port = SerialTransport(os.ttyname(slave), {})
        try:
            started = time.monotonic()
            total = 0
            for _ in range(32):
                result = port.send(b"x" * 4096)
                total += result["bytes_written"]
                if result["status"] != "sent":
                    break
            self.assertIn(result["status"], ("partial", "failed"))
            self.assertEqual(result["error"], "write_timeout")
            self.assertLess(time.monotonic() - started, 3)
            self.assertEqual(sum(event["byte_count"] for event in port.ring.events), total)
            os.close(master)
            master = -1
            until = time.monotonic() + 1
            while port.status()["connected"] and time.monotonic() < until:
                time.sleep(0.01)
            self.assertFalse(port.status()["connected"])
            self.assertEqual(port.status()["warning"], "device_unavailable")
        finally:
            port.close()
            if master >= 0:
                os.close(master)
            os.close(slave)
