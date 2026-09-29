#!/usr/bin/env python3
# -*- coding: utf-8 -*-
# -----------------------------------------------------------------------------
# Copyright (c) 2026 DDP Engineering
# SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
# -----------------------------------------------------------------------------
"""@file test_usb.py
@brief Verify pinned USB ancestry before selecting a physical tty.
@details Synthetic sysfs entries keep hardware access out of this test.
"""

from pathlib import Path
from unittest.mock import patch

import pytest

from nilo_modbus.usb import resolve_tty


def test_usb_identity_requires_exact_adapter_and_interface(tmp_path: Path):
    usb = tmp_path / "devices/1-2"
    interface = usb / "interface"
    endpoint = interface / "ttyUSB0"
    endpoint.mkdir(parents=True)
    (usb / "idVendor").write_text("0403", encoding="utf-8")
    (usb / "idProduct").write_text("6010", encoding="utf-8")
    (usb / "serial").write_text("adapter", encoding="utf-8")
    (interface / "bInterfaceNumber").write_text("00", encoding="utf-8")
    (endpoint / "port_number").write_text("0", encoding="utf-8")
    root = tmp_path / "tty"
    entry = root / "ttyUSB0"
    entry.mkdir(parents=True)
    identity = {
        "stable_id": "usb-valid",
        "vendor_id": "0403",
        "product_id": "6010",
        "serial_number": "adapter/if00/port0",
        "bus_path": "1-2:if00:port0",
    }
    with patch.object(Path, "resolve", lambda path: endpoint if path == entry else path):
        assert resolve_tty(identity, "ttyUSB0", root) == "/dev/ttyUSB0"
        with pytest.raises(ValueError, match="device_unavailable"):
            resolve_tty({**identity, "serial_number": "different/if00/port0"}, "ttyUSB0", root)
        with pytest.raises(ValueError, match="device_unavailable"):
            resolve_tty(identity, "../../ttyUSB0", root)
        assert resolve_tty({**identity, "serial_number": None}, "ttyUSB0", root) == "/dev/ttyUSB0"


# END OF FILE
