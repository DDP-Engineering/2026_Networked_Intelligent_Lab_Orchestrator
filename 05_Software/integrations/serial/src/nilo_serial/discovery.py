#!/usr/bin/env python3
# -*- coding: utf-8 -*-
# -----------------------------------------------------------------------------
# Copyright (c) 2026 DDP Engineering
# SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
# -----------------------------------------------------------------------------
"""@file discovery.py
@brief Discover USB-ancestry tty ports without opening them.
@details Physical identity includes interface and channel to separate multiport devices.
"""

import re
from pathlib import Path

ENDPOINT = re.compile(r"tty(?:USB|ACM)[0-9]+\Z")
HEX_ID = re.compile(r"[0-9a-f]{4}\Z")


def _text(path: Path) -> str:
    try:
        value = path.read_text(encoding="utf-8").strip()
        return value if len(value) <= 200 and all(ord(char) >= 32 for char in value) else ""
    except (OSError, UnicodeError):
        return ""


def scan_ports(tty_root: Path = Path("/sys/class/tty")) -> list[dict]:
    """@brief Return only endpoints with a USB ancestor and stable identity."""
    result = []
    for entry in sorted(tty_root.glob("tty*")):
        if not ENDPOINT.fullmatch(entry.name):
            continue
        endpoint = entry.resolve()
        ancestry = (endpoint, *endpoint.parents)
        usb = next(
            (p for p in ancestry if (p / "idVendor").is_file() and (p / "idProduct").is_file()),
            None,
        )
        interface = next((p for p in ancestry if (p / "bInterfaceNumber").is_file()), None)
        if usb is None or interface is None:
            continue
        vendor, product = _text(usb / "idVendor").lower(), _text(usb / "idProduct").lower()
        number = _text(interface / "bInterfaceNumber").lower()
        port = next(
            (_text(p / "port_number") for p in ancestry if (p / "port_number").is_file()), "0"
        )
        if not HEX_ID.fullmatch(vendor) or not HEX_ID.fullmatch(product):
            continue
        if not re.fullmatch(r"[0-9a-f]{2}", number) or not port.isdecimal():
            continue
        serial = _text(usb / "serial")
        if serial.lower() in {"0", "00000000", "0123456789", "123456789", "unknown", "none"}:
            serial = ""
        suffix = f"/if{number}/port{port}"
        if len(serial + suffix) > 128:
            continue
        result.append(
            {
                "id": entry.name,
                "kind": "usb-serial",
                "display_name": _text(usb / "product") or "USB serial port",
                "hardware_identity": {
                    "vendor": vendor,
                    "product": product,
                    "serial": serial + suffix if serial else "",
                    "port_path": usb.name + suffix.replace("/", ":"),
                },
                "capabilities": {"interface": number},
            }
        )
    return result


def resolve_port(
    identity: object, endpoint_id: object, tty_root: Path = Path("/sys/class/tty")
) -> str:
    """@brief Match the approved full identity immediately before opening."""
    if not isinstance(identity, dict) or not isinstance(endpoint_id, str):
        raise ValueError("device_unavailable")  # noqa: TRY004 - stable runner failure boundary
    for port in scan_ports(tty_root):
        if port["id"] == endpoint_id and port["hardware_identity"] == identity:
            return "/dev/" + endpoint_id
    raise ValueError("device_unavailable")


# END OF FILE
