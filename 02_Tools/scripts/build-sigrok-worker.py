#!/usr/bin/env python3
# -*- coding: utf-8 -*-
# -----------------------------------------------------------------------------
# Copyright (c) 2026 DDP Engineering
# SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
# -----------------------------------------------------------------------------
"""@file    build-sigrok-worker.py
@brief   Compile the standalone structured Sigrok feasibility worker.
@details Uses only the target C compiler and dynamic-loader API; native Sigrok
libraries and decoder data are resolved by the worker at execution time.
"""

# IMPORTS
from __future__ import annotations

import argparse
import subprocess
from pathlib import Path


# PUBLIC API
def main() -> None:
    """@brief Compile one worker binary without modifying host toolchains."""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--cc", default="cc")
    parser.add_argument("--source", type=Path, required=True)
    parser.add_argument("--out", type=Path, required=True)
    options = parser.parse_args()
    source = options.source.resolve(strict=True)
    destination = options.out.resolve()
    if destination.exists() or not destination.parent.is_dir():
        raise SystemExit("--out must name a new file in an existing directory")
    subprocess.run(
        [
            options.cc,
            "-std=c11",
            "-O2",
            "-Wall",
            "-Wextra",
            "-Werror",
            str(source),
            "-ldl",
            "-o",
            str(destination),
        ],
        check=True,
    )


if __name__ == "__main__":
    main()


# END OF FILE
