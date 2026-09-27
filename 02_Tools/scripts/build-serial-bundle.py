#!/usr/bin/env python3
# -*- coding: utf-8 -*-
# -----------------------------------------------------------------------------
# Copyright (c) 2026 DDP Engineering
# SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
# -----------------------------------------------------------------------------
"""@file build-serial-bundle.py
@brief Build an offline indexed USB serial integration for an explicit Linux target.
@details Verify wheel, native architecture and target runtime before publishing inventory.
"""

from __future__ import annotations

import argparse
import hashlib
import importlib.util
import json
from pathlib import Path
import shutil
import tempfile


def _load(name: str):
    """@brief Load shared repository-owned verification helpers by fixed filename."""
    spec = importlib.util.spec_from_file_location(
        name.replace("-", "_"), Path(__file__).with_name(name + ".py")
    )
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def build(
    project: Path,
    dependency_root: Path,
    wheelhouse: Path,
    destination: Path,
    architecture: str,
    runtime_root: Path | None = None,
) -> None:
    """@brief Verify universal wheel and native dependencies before publishing a bundle."""
    camera = _load("build-camera-bundle")
    dependency = _load("prepare-serial-dependency")
    if architecture not in ("arm64", "amd64"):
        raise ValueError("Unsupported target architecture")
    if destination.exists():
        raise ValueError("Bundle destination already exists")
    dependency.prepare(wheelhouse, offline=True)
    destination.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(
        prefix="nilo-serial-bundle-", dir=destination.parent
    ) as temp:
        stage = Path(temp) / "serial"
        python = stage / "python"
        python.mkdir(parents=True)
        for name in camera.DEPENDENCIES:
            camera.copy_tree(dependency_root / name, python / name)
        camera.copy_tree(project / "src/nilo_serial", python / "nilo_serial")
        camera.unpack_verified_wheel(
            wheelhouse, (dependency.NAME, dependency.SHA256), python
        )
        camera.assert_native_architecture(python, architecture)
        camera.copy_tree(project / "schemas", stage / "schemas")
        bootstrap = (
            "import sys\nfrom pathlib import Path\n"
            "sys.dont_write_bytecode = True\n"
            "sys.path.insert(0, str(Path(__file__).resolve().parent / 'python'))\n"
            "from nilo_serial.__main__ import main\nmain()\n"
        )
        if runtime_root is not None:
            notices = dependency_root.parent / "THIRD_PARTY_NOTICES.txt"
            if not notices.is_file() or not notices.read_text().strip():
                raise ValueError("Dependencies require third-party notices")
            camera.copy_tree(notices, stage / "THIRD_PARTY_NOTICES.txt")
            camera.copy_target_runtime(runtime_root, stage / "runtime", architecture)
            (stage / "bootstrap.py").write_text(bootstrap)
            runner = (
                "#!/bin/sh\nset -eu\n"
                'root=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)\n'
                'exec "$root/runtime/bin/python3" -B "$root/bootstrap.py" "$@"\n'
            )
        else:
            runner = "#!/usr/bin/env python3\n" + bootstrap
        (stage / "runner").write_text(runner, newline="\n")
        (stage / "runner").chmod(0o755)
        version = (project.parents[2] / "VERSION").read_text().strip()
        (stage / "runtime.json").write_text(
            json.dumps(
                {
                    "architecture": architecture,
                    "self_contained": runtime_root is not None,
                    "backend": "pyserial",
                    "project_version": version,
                },
                sort_keys=True,
                indent=2,
            )
            + "\n"
        )
        manifest = json.loads((project / "manifest.json").read_text())
        manifest["package"]["version"] = version
        manifest["platforms"] = [{"os": "linux", "architecture": architecture}]
        inventory = {
            path.relative_to(stage).as_posix(): hashlib.sha256(
                path.read_bytes()
            ).hexdigest()
            for path in sorted(stage.rglob("*"))
            if path.is_file()
        }
        manifest["package"]["integrity"] = (
            "sha256:"
            + hashlib.sha256(json.dumps(inventory, sort_keys=True).encode()).hexdigest()
        )
        (stage / "manifest.json").write_text(json.dumps(manifest, indent=2) + "\n")
        inventory["manifest.json"] = hashlib.sha256(
            (stage / "manifest.json").read_bytes()
        ).hexdigest()
        (stage / "files.sha256.json").write_text(
            json.dumps(inventory, sort_keys=True, indent=2) + "\n"
        )
        shutil.move(str(stage), destination)
        for path in destination.rglob("*"):
            if path.is_file():
                path.chmod(0o555 if path.stat().st_mode & 0o111 else 0o444)
        for path in sorted(
            (p for p in destination.rglob("*") if p.is_dir()),
            key=lambda item: len(item.parts),
            reverse=True,
        ):
            path.chmod(0o555)
        destination.chmod(0o555)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ("project", "dependency-root", "wheelhouse", "out"):
        parser.add_argument("--" + name, type=Path, required=True)
    parser.add_argument("--architecture", choices=("arm64", "amd64"), required=True)
    parser.add_argument("--runtime-root", type=Path)
    args = parser.parse_args()
    build(
        args.project.resolve(),
        args.dependency_root,
        args.wheelhouse,
        args.out,
        args.architecture,
        args.runtime_root,
    )

# END OF FILE
