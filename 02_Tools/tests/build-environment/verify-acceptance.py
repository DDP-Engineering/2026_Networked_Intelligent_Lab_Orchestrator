#!/usr/bin/env python3
"""Delegate BE-01 through BE-10 evaluation to the repository core."""

from __future__ import annotations

import runpy
import sys
from pathlib import Path

CORE = Path(__file__).resolve().parents[2] / "scripts" / "build_environment.py"
sys.argv = [str(CORE), "verify-acceptance", *sys.argv[1:]]
runpy.run_path(str(CORE), run_name="__main__")
