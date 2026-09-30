#!/usr/bin/env python3
# -*- coding: utf-8 -*-
# -----------------------------------------------------------------------------
# Copyright (c) 2026 DDP Engineering
# SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
# -----------------------------------------------------------------------------
"""@file test_package.py
@brief Verify the typed supervised Modbus runner package inventory.
@details The package is locally loadable without publishing a target bundle.
"""

import json
from pathlib import Path

from jsonschema import Draft202012Validator


def test_manifest_declares_finite_admission_and_private_runner():
    root = Path(__file__).resolve().parents[1]
    manifest = json.loads((root / "manifest.json").read_text(encoding="utf-8"))
    assert manifest["package"]["id"] == "modbus"
    assert manifest["protocol"] == "nilo.integration/1"
    assert manifest["runner"] == {"executable": "runner", "arguments": ["--stdio"]}
    assert [operation["id"] for operation in manifest["operations"]] == ["connect", "serve"]
    assert manifest["operations"][0]["cancellable"]
    assert manifest["artifacts"] == []
    for name in ("plugin", "device", "connect.input", "serve.input", "result", "event"):
        path = root / "schemas" / f"{name}.schema.json"
        Draft202012Validator.check_schema(json.loads(path.read_text(encoding="utf-8")))


# END OF FILE
