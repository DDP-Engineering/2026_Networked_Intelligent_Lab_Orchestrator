#!/usr/bin/env python3
# -*- coding: utf-8 -*-
# -----------------------------------------------------------------------------
# Copyright (c) 2026 DDP Engineering
# SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
# -----------------------------------------------------------------------------
"""@file    build-sigrok-bundle.py
@brief   Build a curated sigrok bundle with locked wheels for a target Linux Python.
@details Run through 02_Tools/env/with-tools.ps1 (Windows) or the pinned Linux environment.
The target requires Python at the declared minor version and installed sigrok-cli.
No target package installation or system environment modification is performed.
"""

# IMPORTS
from __future__ import annotations

import argparse
import hashlib
import json
import shutil
import subprocess
import tempfile
from pathlib import Path


def build(destination: Path, architecture: str, python_version: str) -> None:
    """@brief Assemble code and checksum-verified locked third-party wheels."""
    repository = Path(__file__).resolve().parents[2]
    project = repository / "05_Software/integrations/sigrok"
    destination = destination.resolve()
    if destination.exists():
        raise ValueError("Bundle destination must not already exist")
    destination.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(
        prefix="nilo-bundle-", dir=destination.parent
    ) as temporary:
        stage = Path(temporary) / "sigrok"
        stage.mkdir()
        requirements = Path(temporary) / "requirements.txt"
        subprocess.run(
            [
                "uv",
                "export",
                "--project",
                str(project),
                "--frozen",
                "--no-dev",
                "--no-emit-project",
                "--no-emit-package",
                "nilo-integration-sdk",
                "--output-file",
                str(requirements),
            ],
            check=True,
            capture_output=True,
        )
        platform = (
            "aarch64-manylinux_2_28"
            if architecture == "arm64"
            else "x86_64-manylinux_2_28"
        )
        subprocess.run(
            [
                "uv",
                "pip",
                "install",
                "--target",
                str(stage / "python"),
                "--python-platform",
                platform,
                "--python-version",
                python_version,
                "--only-binary",
                ":all:",
                "--require-hashes",
                "--no-deps",
                "--requirements",
                str(requirements),
            ],
            check=True,
        )
        shutil.copytree(
            project / "src/nilo_sigrok",
            stage / "python/nilo_sigrok",
            ignore=shutil.ignore_patterns("__pycache__", "*.pyc"),
        )
        sdk = repository / "05_Software/integration_sdk/python/src/nilo_integration"
        shutil.copytree(
            sdk,
            stage / "python/nilo_integration",
            ignore=shutil.ignore_patterns("__pycache__", "*.pyc"),
        )
        shutil.copytree(project / "schemas", stage / "schemas")
        (stage / "runner").write_bytes(
            (project / "runner").read_bytes().replace(b"\r\n", b"\n")
        )
        (stage / "runner").chmod(0o755)
        manifest = json.loads((project / "manifest.json").read_text(encoding="utf-8"))
        manifest["platforms"] = [{"os": "linux", "architecture": architecture}]
        # The manifest's integrity attests payload files excluding the manifest
        # itself. The outer index then covers the resulting manifest as well.
        payload = {
            p.relative_to(stage).as_posix(): hashlib.sha256(p.read_bytes()).hexdigest()
            for p in sorted(stage.rglob("*"))
            if p.is_file()
        }
        manifest["package"]["integrity"] = (
            "sha256:"
            + hashlib.sha256(json.dumps(payload, sort_keys=True).encode()).hexdigest()
        )
        (stage / "manifest.json").write_text(
            json.dumps(manifest, indent=2) + "\n", encoding="utf-8"
        )
        (stage / "runtime.json").write_text(
            json.dumps(
                {
                    "python": python_version,
                    "architecture": architecture,
                    "backend": "installed sigrok-cli",
                    "project_version": (repository / "VERSION").read_text().strip(),
                },
                indent=2,
            )
            + "\n",
            encoding="utf-8",
        )
        index = {
            p.relative_to(stage).as_posix(): hashlib.sha256(p.read_bytes()).hexdigest()
            for p in sorted(stage.rglob("*"))
            if p.is_file()
        }
        (stage / "files.sha256.json").write_text(
            json.dumps(index, sort_keys=True, indent=2) + "\n", encoding="utf-8"
        )
        stage.rename(destination)


def main() -> None:
    """@brief Parse the target runtime and write one immutable bundle directory."""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--out", type=Path, required=True)
    parser.add_argument("--architecture", choices=["arm64", "amd64"], required=True)
    parser.add_argument(
        "--python-version", choices=["3.11", "3.12", "3.13"], default="3.13"
    )
    args = parser.parse_args()
    build(args.out, args.architecture, args.python_version)
    print(args.out.resolve())


if __name__ == "__main__":
    main()

# END OF FILE
