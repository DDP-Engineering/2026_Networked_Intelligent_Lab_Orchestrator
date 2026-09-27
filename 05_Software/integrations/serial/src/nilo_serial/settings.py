#!/usr/bin/env python3
# -*- coding: utf-8 -*-
# -----------------------------------------------------------------------------
# Copyright (c) 2026 DDP Engineering
# SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
# -----------------------------------------------------------------------------
"""@file settings.py
@brief Validate immutable approved port settings.
@details Driver support is checked when opening the port.
"""

DEFAULTS = {
    "baud_rate": 115200,
    "data_bits": 8,
    "parity": "none",
    "stop_bits": 1,
    "flow_control": "none",
    "rts": True,
    "dtr": True,
}


def validate_settings(value: object) -> dict:
    """@brief Reject unknown fields and invalid or coercible values."""
    if not isinstance(value, dict) or set(value) - DEFAULTS.keys():
        raise ValueError("invalid_parameters")
    result = {**DEFAULTS, **value}
    if (
        type(result["baud_rate"]) is not int
        or not 50 <= result["baud_rate"] <= 4000000
        or type(result["data_bits"]) is not int
        or result["data_bits"] not in (5, 6, 7, 8)
        or result["parity"] not in ("none", "odd", "even", "mark", "space")
        or type(result["stop_bits"]) not in (int, float)
        or result["stop_bits"] not in (1, 1.5, 2)
        or result["flow_control"] not in ("none", "rtscts", "xonxoff")
        or type(result["rts"]) is not bool
        or type(result["dtr"]) is not bool
    ):
        raise ValueError("invalid_parameters")
    return result


# END OF FILE
