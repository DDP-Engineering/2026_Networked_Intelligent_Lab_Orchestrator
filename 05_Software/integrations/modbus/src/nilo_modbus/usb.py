#!/usr/bin/env python3
# -*- coding: utf-8 -*-
# -----------------------------------------------------------------------------
# Copyright (c) 2026 DDP Engineering
# See LICENSE.txt in the repository root for full license details.
# SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
# -----------------------------------------------------------------------------
"""@file usb.py
@brief Resolve a trusted USB tty against Core's pinned physical identity.
@details A tty alias alone cannot select a different adapter after replug.
"""

# IMPORTS
import re
from pathlib import Path

# CONSTANTS
TTY_NAME = re.compile(r"tty(?:USB|ACM)[0-9]+\Z")
HEX_ID = re.compile(r"[0-9a-f]{4}\Z")


# HELPER FUNCTIONS
def _text(path: Path) -> str:
    try:
        value = path.read_text(encoding="utf-8").strip()
        return value if len(value) <= 200 and all(ord(char) >= 32 for char in value) else ""
    except (OSError, UnicodeError):
        return ""


# PUBLIC API
def resolve_tty(identity: object, tty: object, tty_root: Path = Path("/sys/class/tty")) -> str:
    """@brief Match USB ancestor and interface before opening the tty basename."""
    if not isinstance(identity, dict) or not isinstance(tty, str) or not TTY_NAME.fullmatch(tty):
        raise ValueError("device_unavailable")
    if set(identity) != {"stable_id", "vendor_id", "product_id", "serial_number", "bus_path"}:
        raise ValueError("device_unavailable")
    entry = tty_root / tty
    if not entry.exists():
        raise ValueError("device_unavailable")
    endpoint = entry.resolve()
    ancestry = (endpoint, *endpoint.parents)
    usb = next(
        (p for p in ancestry if (p / "idVendor").is_file() and (p / "idProduct").is_file()), None
    )
    interface = next((p for p in ancestry if (p / "bInterfaceNumber").is_file()), None)
    if usb is None or interface is None:
        raise ValueError("device_unavailable")
    vendor, product = _text(usb / "idVendor").lower(), _text(usb / "idProduct").lower()
    number = _text(interface / "bInterfaceNumber").lower()
    port = next((_text(p / "port_number") for p in ancestry if (p / "port_number").is_file()), "0")
    if (
        not HEX_ID.fullmatch(vendor)
        or not HEX_ID.fullmatch(product)
        or not re.fullmatch(r"[0-9a-f]{2}", number)
        or not port.isdecimal()
    ):
        raise ValueError("device_unavailable")
    suffix = f"/if{number}/port{port}"
    serial = _text(usb / "serial")
    if serial.lower() in {"0", "00000000", "0123456789", "123456789", "unknown", "none"}:
        serial = ""
    observed = {
        "vendor_id": vendor,
        "product_id": product,
        "serial_number": serial + suffix if serial else None,
        "bus_path": usb.name + suffix.replace("/", ":"),
    }
    if any(identity[key] != observed[key] for key in ("vendor_id", "product_id", "bus_path")) or (
        identity["serial_number"] is not None
        and identity["serial_number"] != observed["serial_number"]
    ):
        raise ValueError("device_unavailable")
    return "/dev/" + tty


# END OF FILE
