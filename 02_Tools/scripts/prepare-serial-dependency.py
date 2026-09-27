#!/usr/bin/env python3
# -*- coding: utf-8 -*-
# -----------------------------------------------------------------------------
# Copyright (c) 2026 DDP Engineering
# SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
# -----------------------------------------------------------------------------
"""@file prepare-serial-dependency.py
@brief Prepare the pinned universal pySerial wheel for offline builds and tests.
@details Existing and downloaded inputs must match the published SHA-256 pin.
"""

import argparse
import hashlib
from pathlib import Path
import urllib.request

NAME = "pyserial-3.5-py2.py3-none-any.whl"
SHA256 = "c4451db6ba391ca6ca299fb3ec7bae67a5c55dde170964c7a14ceefec02f2cf0"
URL = (
    "https://files.pythonhosted.org/packages/07/bc/587a445451b253b285629263eb51c2d8e9bcea4fc97826266d186f96f558/"
    + NAME
)


def prepare(directory: Path, offline: bool = False) -> Path:
    """@brief Download the exact pinned artifact and verify before installation."""
    destination = directory / NAME
    if destination.is_file():
        if hashlib.sha256(destination.read_bytes()).hexdigest() != SHA256:
            raise ValueError("Pinned pySerial wheel checksum mismatch")
        return destination
    if offline:
        raise ValueError(
            "Pinned pySerial wheel missing; run prepare-serial-dependency.py first"
        )
    with urllib.request.urlopen(URL, timeout=30) as response:
        data = response.read(1 << 20)
    if hashlib.sha256(data).hexdigest() != SHA256:
        raise ValueError("Downloaded pySerial wheel checksum mismatch")
    directory.mkdir(parents=True, exist_ok=True)
    destination.write_bytes(data)
    return destination


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--offline", action="store_true")
    args = parser.parse_args()
    print(prepare(args.output, args.offline))

# END OF FILE
