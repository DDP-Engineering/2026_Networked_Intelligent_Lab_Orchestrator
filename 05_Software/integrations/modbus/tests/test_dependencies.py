#!/usr/bin/env python3
# -*- coding: utf-8 -*-
# -----------------------------------------------------------------------------
# Copyright (c) 2026 DDP Engineering
# This software is licensed under the PolyForm Noncommercial License 1.0.0.
# See LICENSE.txt in the repository root for full license details.
# SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
# -----------------------------------------------------------------------------
"""@file test_dependencies.py
@brief Verify pinned inputs and offline preparation behavior.
@details Feasibility only; production plugin registration is a later package.
"""

# IMPORTS
import hashlib
import importlib.util
import json
import shutil
import tomllib
from pathlib import Path

import pytest

# CONSTANTS
ROOT = Path(__file__).parents[1]
REPO = ROOT.parents[2]


# HELPER FUNCTIONS
def manifest():
    """@brief Require dependency evidence before reading its exact pins."""
    path = ROOT / "evidence/dependencies.json"
    assert path.is_file(), "dependency closure evidence unavailable"
    return json.loads(path.read_text())


def preparer():
    """@brief Load the repository-local verified-input helper."""
    path = REPO / "02_Tools/scripts/prepare-modbus-dependencies.py"
    assert path.is_file(), "offline dependency preparation unavailable"
    spec = importlib.util.spec_from_file_location("prepare_modbus_dependencies", path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


# PUBLIC API
def test_runtime_closure_matches_lock():
    pins = manifest()
    lock = tomllib.loads((ROOT / "uv.lock").read_text())
    packages = {p["name"]: p for p in lock["package"]}
    assert pins["python_version"] == "3.12.10"
    assert pins["uv_version"] == "0.11.25"
    assert pins["architectures"] == ["linux-amd64", "linux-arm64"]
    assert {(p["name"], p["version"]) for p in pins["packages"]} == {
        ("pymodbus", "3.15.0"),
        ("pyserial", "3.5"),
    }
    for pin in pins["packages"]:
        package = packages[pin["name"]]
        assert package["version"] == pin["version"]
        lock_inputs = package["wheels"] + [package["sdist"]]
        for artifact in pin["artifacts"]:
            assert any(
                a["url"] == artifact["url"] and a["hash"] == "sha256:" + artifact["sha256"]
                for a in lock_inputs
            )
        assert len(pin["license_sha256"]) == 64
        assert pin["license_member"]
        assert "BSD" in pin["license"]


def test_offline_missing_and_corrupt_inputs_fail(tmp_path):
    module = preparer()
    with pytest.raises(ValueError, match="missing"):
        module.prepare(tmp_path, offline=True)
    artifact = manifest()["packages"][0]["artifacts"][0]
    target = tmp_path / artifact["filename"]
    target.write_bytes(b"invalid cache bytes")
    with pytest.raises(ValueError, match="checksum"):
        module.prepare(tmp_path, offline=True)


def test_verified_local_closure_and_notices(tmp_path):
    module = preparer()
    for package in manifest()["packages"]:
        for artifact in package["artifacts"]:
            filename = artifact["filename"]
            kind = "python-packages" if filename.endswith(".whl") else "license-sources"
            candidates = [
                REPO / "02_Tools/cache/modbus-feasibility" / filename,
                Path("/opt/nilo/inputs") / kind / filename,
            ]
            source = next((p for p in candidates if p.is_file()), None)
            if source is None:
                pytest.skip("verified acquisition command is a separate explicit connected step")
            shutil.copyfile(source, tmp_path / filename)
    paths = module.prepare(tmp_path, offline=True)
    assert len(paths) == 4
    for pin in manifest()["packages"]:
        notice = tmp_path / "licenses" / (pin["name"] + "-LICENSE.txt")
        assert hashlib.sha256(notice.read_bytes()).hexdigest() == pin["license_sha256"]


def test_preparer_rejects_unsafe_pin_url():
    module = preparer()
    with pytest.raises(ValueError, match="source"):
        module.validate_pin(
            {"filename": "x.whl", "url": "http://example.com/x.whl", "sha256": "0" * 64}
        )


def test_runtime_archives_are_in_offline_build_recipe():
    inputs = json.loads(
        (REPO / "02_Tools/containers/build/reconstruction-inputs.json").read_text()
    )["inputs"]
    for package in manifest()["packages"]:
        for artifact in package["artifacts"]:
            assert any(
                item["name"] == package["name"]
                and item["version"] == package["version"]
                and item["source"] == artifact["url"]
                and item["sha256"] == artifact["sha256"]
                for item in inputs
            ), f"offline recipe lacks {artifact['filename']}"


# END OF FILE
