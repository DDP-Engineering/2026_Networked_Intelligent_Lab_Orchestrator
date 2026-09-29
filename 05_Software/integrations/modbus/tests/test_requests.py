#!/usr/bin/env python3
# -*- coding: utf-8 -*-
# -----------------------------------------------------------------------------
# Copyright (c) 2026 DDP Engineering
# This software is licensed under the PolyForm Noncommercial License 1.0.0.
# See LICENSE.txt in the repository root for full license details.
# SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
# -----------------------------------------------------------------------------
"""@file test_requests.py
@brief Verify initial function encoding and protocol bounds.
@details Feasibility only; production plugin registration is a later package.
"""

# IMPORTS
import importlib
import importlib.util

import pytest


# HELPER FUNCTIONS
def subject():
    """@brief Require the independently testable request encoder."""
    assert importlib.util.find_spec("nilo_modbus.requests"), "request validation is unavailable"
    return importlib.import_module("nilo_modbus.requests")


# PUBLIC API
@pytest.mark.parametrize(
    "fc,count,values,expected",
    [
        (1, 10, (), "010000000a"),
        (2, 10, (), "020000000a"),
        (3, 10, (), "030000000a"),
        (4, 10, (), "040000000a"),
        (5, 1, (True,), "050000ff00"),
        (6, 1, (0x1234,), "0600001234"),
        (15, 9, (True, False, True, True, False, False, False, True, True), "0f00000009028d01"),
        (16, 2, (0x1234, 0xABCD), "1000000002041234abcd"),
    ],
)
def test_encode_golden(fc, count, values, expected):
    m = subject()
    request = m.Request(fc, 1, 0, count, values)
    assert m.encode(request, "rtu").hex() == expected


@pytest.mark.parametrize(
    "fc,limit", [(1, 2000), (2, 2000), (3, 125), (4, 125), (15, 1968), (16, 123)]
)
def test_function_limits(fc, limit):
    m = subject()
    values = tuple([True if fc == 15 else 0] * limit) if fc in (15, 16) else ()
    m.encode(m.Request(fc, 1, 0, limit, values), "tcp")
    with pytest.raises(ValueError):
        m.encode(m.Request(fc, 1, 0, limit + 1, values), "tcp")


@pytest.mark.parametrize(
    "args,mode",
    [
        ((3, 1, 65535, 2, ()), "tcp"),
        ((3, 0, 0, 1, ()), "rtu"),
        ((3, 248, 0, 1, ()), "rtu_tcp"),
        ((3, 256, 0, 1, ()), "tcp"),
        ((22, 1, 0, 1, ()), "tcp"),
        ((3, 1, -1, 1, ()), "tcp"),
        ((3, 1, 0, 0, ()), "tcp"),
        ((3, True, 0, 1, ()), "tcp"),
        ((3, 1, 0, True, ()), "tcp"),
        ((3, 1, 0, 1, (5,)), "tcp"),
        ((5, 1, 0, 1, (1,)), "tcp"),
        ((6, 1, 0, 1, (True,)), "tcp"),
        ((6, 1, 0, 1, (65536,)), "tcp"),
        ((6, 1, 0, 2, (1, 2)), "tcp"),
        ((16, 1, 0, 2, (1,)), "tcp"),
        ((15, 1, 0, 1, ("on",)), "tcp"),
        ((3, 1, 0, 1, ()), "udp"),
    ],
)
def test_invalid_requests(args, mode):
    m = subject()
    with pytest.raises(ValueError):
        m.encode(m.Request(*args), mode)


def test_tcp_unit_zero_and_end_address():
    m = subject()
    assert m.encode(m.Request(3, 0, 65535), "tcp") == bytes.fromhex("03ffff0001")


def test_display_reference_conversion():
    m = subject()
    assert m.wire_address("holding", 40001, "reference") == 0
    assert m.wire_address("input", 30001, "reference") == 0
    assert m.wire_address("coil", 1, "reference") == 0
    assert m.wire_address("holding", 1, "one") == 0
    assert m.wire_address("holding", 0, "zero") == 0
    assert m.wire_address("holding", 105536, "reference") == 65535
    for area, value, base in [
        ("holding", 30001, "reference"),
        ("holding", 0, "one"),
        ("holding", 65536, "zero"),
        ("unknown", 0, "zero"),
    ]:
        with pytest.raises(ValueError):
            m.wire_address(area, value, base)


# END OF FILE
