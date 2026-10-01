#!/usr/bin/env python3
# -*- coding: utf-8 -*-
# -----------------------------------------------------------------------------
# Copyright (c) 2026 DDP Engineering
# This software is licensed under the PolyForm Noncommercial License 1.0.0.
# See LICENSE.txt in the repository root for full license details.
# SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
# -----------------------------------------------------------------------------
"""@file requests.py
@brief Validate canonical requests and encode bounded Modbus PDUs.
@details Feasibility only; production plugin registration is a later package.
"""

# IMPORTS
import struct
from dataclasses import dataclass

# CONSTANTS
LIMITS = {1: 2000, 2: 2000, 3: 125, 4: 125, 5: 1, 6: 1, 11: 1, 15: 1968, 16: 123, 17: 1, 22: 1, 23: 121, 43: 1}
WRITES = frozenset((5, 6, 15, 16, 22, 23))


# DATA MODELS / TYPEDEFINITIONS
@dataclass(frozen=True)
class Request:
    """@brief Canonical bounded wire request, independent of display addressing."""

    function: int
    unit: int
    address: int
    count: int = 1
    values: tuple = ()
    and_mask: int | None = None
    or_mask: int | None = None
    read_address: int | None = None
    read_count: int | None = None


# PUBLIC API
def encode(request: Request, transport: str) -> bytes:
    """@brief Validate initial operations and encode their exact PDU.
    @param[in] request Canonical unit/address/count and raw values.
    @param[in] transport rtu, tcp or rtu_tcp.
    @return Function and data bytes, excluding transport headers.
    @exception ValueError Unsupported or inconsistent request.
    """
    if transport not in ("rtu", "tcp", "rtu_tcp"):
        raise ValueError("unsupported transport")
    fc, unit, address, count = request.function, request.unit, request.address, request.count
    if any(type(v) is not int for v in (fc, unit, address, count)):
        raise ValueError("integer fields required")
    if fc not in LIMITS or not 0 <= address <= 65535 or not 1 <= count <= LIMITS[fc]:
        raise ValueError("function/address/quantity")
    if address + count > 65536 or not (
        0 <= unit <= 255 if transport == "tcp" else 1 <= unit <= 247
    ):
        raise ValueError("address range/unit")
    values = request.values
    if not isinstance(values, tuple):
        raise ValueError("immutable values required")  # noqa: TRY004 - one validation error contract
    if fc in (11, 17, 43):
        if (
            address != 0
            or count != 1
            or values
            or unit == 0
            or fc in (11, 17) and transport == "tcp"
            or any(field is not None for field in (
                request.and_mask, request.or_mask, request.read_address, request.read_count
            ))
        ):
            raise ValueError("diagnostic fields/transport")
        return b"\x2b\x0e\x01\x00" if fc == 43 else bytes((fc,))
    if fc == 22:
        if (
            values
            or count != 1
            or any(type(mask) is not int or not 0 <= mask <= 65535 for mask in (request.and_mask, request.or_mask))
            or request.read_address is not None
            or request.read_count is not None
        ):
            raise ValueError("mask write fields")
        return struct.pack(">BHHH", fc, address, request.and_mask, request.or_mask)
    if fc == 23:
        if (
            type(request.read_address) is not int
            or type(request.read_count) is not int
            or not 0 <= request.read_address <= 65535
            or not 1 <= request.read_count <= 125
            or request.read_address + request.read_count > 65536
            or request.and_mask is not None
            or request.or_mask is not None
        ):
            raise ValueError("read/write fields")
    elif any(
        field is not None
        for field in (request.and_mask, request.or_mask, request.read_address, request.read_count)
    ):
        raise ValueError("irrelevant fields")
    if fc not in WRITES:
        if values:
            raise ValueError("read values")
        return struct.pack(">BHH", fc, address, count)
    if len(values) != count:
        raise ValueError("value count")
    if fc in (5, 15):
        if any(type(v) is not bool for v in values):
            raise ValueError("coil value must be Boolean")
        if fc == 5:
            return struct.pack(">BHH", fc, address, 0xFF00 if values[0] else 0)
        packed = bytearray((count + 7) // 8)
        for index, value in enumerate(values):
            packed[index // 8] |= int(value) << (index % 8)
        return struct.pack(">BHHB", fc, address, count, len(packed)) + packed
    if any(type(v) is not int or not 0 <= v <= 65535 for v in values):
        raise ValueError("register word must be uint16")
    if fc == 6:
        return struct.pack(">BHH", fc, address, values[0])
    payload = struct.pack(">" + "H" * count, *values)
    if fc == 23:
        return struct.pack(">BHHHHB", fc, request.read_address, request.read_count, address, count, len(payload)) + payload
    return struct.pack(">BHHB", fc, address, count, len(payload)) + payload


def wire_address(area: str, address: int, base: str) -> int:
    """@brief Convert explicitly selected presentation addressing to wire offset.
    @param[in] area coil, discrete, holding or input.
    @param[in] address Integer presentation reference.
    @param[in] base zero, one or reference.
    @return Wire address 0..65535.
    @exception ValueError Invalid area/base or outside selected reference range.
    """
    starts = {"coil": 1, "discrete": 10001, "input": 30001, "holding": 40001}
    if area not in starts or base not in ("zero", "one", "reference") or type(address) is not int:
        raise ValueError("address notation")
    offset = address - (0 if base == "zero" else 1 if base == "one" else starts[area])
    # Area is explicit, so extended references cover the full 16-bit wire range.
    if not 0 <= offset <= 65535:
        raise ValueError("display address range")
    return offset


# END OF FILE
