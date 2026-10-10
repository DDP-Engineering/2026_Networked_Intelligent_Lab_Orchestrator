#!/usr/bin/env python3
# -*- coding: utf-8 -*-
# -----------------------------------------------------------------------------
# Copyright (c) 2026 DDP Engineering
# SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
# -----------------------------------------------------------------------------
"""Build an indexed Modbus runner from verified offline Linux inputs."""

from __future__ import annotations

import argparse
import hashlib
import importlib.util
import json
import shutil
import stat
import tempfile
from pathlib import Path

ARCHITECTURES = {"amd64": 62, "arm64": 183}
SHARED_DEPENDENCIES = (
    "nilo_integration", "pydantic", "pydantic_core", "annotated_types",
    "typing_inspection", "typing_extensions.py",
)


def _load(name: str):
    spec = importlib.util.spec_from_file_location(
        name.replace("-", "_"), Path(__file__).with_name(name + ".py")
    )
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def _regular_tree(root: Path) -> None:
    """Refuse source links and special files before copying package inputs."""
    if not root.is_dir() or root.is_symlink():
        raise ValueError(f"Missing regular dependency directory: {root.name}")
    for item in root.rglob("*"):
        mode = item.lstat().st_mode
        if not (stat.S_ISREG(mode) or stat.S_ISDIR(mode)):
            raise ValueError(f"Linked or special dependency input: {item.name}")


def _copy_runtime(source: Path, target: Path, architecture: str) -> None:
    """Verify a manifest-bound target Python closure before copying it."""
    _regular_tree(source)
    manifest_path = source / "runtime-manifest.json"
    python = source / "bin/python3"
    notices = source / "THIRD_PARTY_NOTICES.txt"
    if not all(path.is_file() for path in (manifest_path, python, notices)):
        raise ValueError("Target runtime is missing Python, manifest or notices")
    if not notices.read_text(encoding="utf-8").strip():
        raise ValueError("Target runtime notices are empty")
    header = python.read_bytes()[:20]
    if (len(header) < 20 or header[:6] != b"\x7fELF\x02\x01"
            or int.from_bytes(header[18:20], "little") != ARCHITECTURES[architecture]):
        raise ValueError("Target Python runtime architecture is invalid")
    details = json.loads(manifest_path.read_text(encoding="utf-8"))
    observed = [
        {
            "path": path.relative_to(source).as_posix(),
            "sha256": hashlib.sha256(path.read_bytes()).hexdigest(),
            "size": path.stat().st_size,
        }
        for path in sorted(source.rglob("*"))
        if path.is_file() and path != manifest_path
    ]
    if details.get("architecture") != architecture or details.get("files") != observed:
        raise ValueError("Target runtime manifest does not match its files")
    shutil.copytree(source, target)


def build(
    project: Path,
    dependency_root: Path,
    wheelhouse: Path,
    destination: Path,
    architecture: str,
    runtime_root: Path | None = None,
) -> None:
    """Publish one immutable target bundle only after every input is verified."""
    if architecture not in ARCHITECTURES:
        raise ValueError("Unsupported target architecture")
    if destination.exists():
        raise ValueError("Bundle destination already exists")
    camera = _load("build-camera-bundle")
    pinned = _load("prepare-modbus-dependencies")
    _regular_tree(wheelhouse)
    pinned.prepare(wheelhouse, offline=True)
    _regular_tree(dependency_root)
    notices = dependency_root.parent / "THIRD_PARTY_NOTICES.txt"
    if not notices.is_file() or notices.is_symlink() or not notices.read_text(encoding="utf-8").strip():
        raise ValueError("Shared dependencies require third-party notices")
    destination.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix="nilo-modbus-bundle-", dir=destination.parent) as temporary:
        stage = Path(temporary) / "modbus"
        python = stage / "python"
        python.mkdir(parents=True)
        for name in SHARED_DEPENDENCIES:
            camera.copy_tree(dependency_root / name, python / name)
        camera.copy_tree(project / "src/nilo_modbus", python / "nilo_modbus")
        for package in json.loads(pinned.MANIFEST.read_text(encoding="utf-8"))["packages"]:
            wheel = next(item for item in package["artifacts"] if item["filename"].endswith(".whl"))
            camera.unpack_verified_wheel(
                wheelhouse, (wheel["filename"], wheel["sha256"]), python
            )
        camera.assert_native_architecture(python, architecture)
        camera.copy_tree(project / "schemas", stage / "schemas")
        camera.copy_tree(wheelhouse / "licenses", stage / "licenses")
        camera.copy_tree(notices, stage / "THIRD_PARTY_NOTICES.txt")
        bootstrap = (
            "import sys\nfrom pathlib import Path\n"
            "sys.dont_write_bytecode = True\n"
            "sys.path.insert(0, str(Path(__file__).resolve().parent / 'python'))\n"
            "from nilo_modbus.__main__ import main\nmain()\n"
        )
        if runtime_root is not None:
            _copy_runtime(runtime_root, stage / "runtime", architecture)
            (stage / "bootstrap.py").write_text(bootstrap, encoding="utf-8", newline="\n")
            runner = (
                "#!/bin/sh\nset -eu\n"
                'root=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)\n'
                'exec "$root/runtime/bin/python3" -B "$root/bootstrap.py" "$@"\n'
            )
        else:
            runner = "#!/usr/bin/env python3\n" + bootstrap
        (stage / "runner").write_text(runner, encoding="utf-8", newline="\n")
        (stage / "runner").chmod(0o755)
        version = (project.parents[2] / "VERSION").read_text(encoding="utf-8").strip()
        (stage / "runtime.json").write_text(
            json.dumps({
                "architecture": architecture,
                "self_contained": runtime_root is not None,
                "backend": "pymodbus",
                "project_version": version,
            }, sort_keys=True, indent=2) + "\n", encoding="utf-8", newline="\n",
        )
        descriptor = json.loads((project / "manifest.json").read_text(encoding="utf-8"))
        descriptor["package"]["version"] = version
        descriptor["platforms"] = [{"os": "linux", "architecture": architecture}]
        payload = {
            path.relative_to(stage).as_posix(): hashlib.sha256(path.read_bytes()).hexdigest()
            for path in sorted(stage.rglob("*")) if path.is_file()
        }
        descriptor["package"]["integrity"] = (
            "sha256:" + hashlib.sha256(json.dumps(payload, sort_keys=True).encode()).hexdigest()
        )
        (stage / "manifest.json").write_text(
            json.dumps(descriptor, indent=2) + "\n", encoding="utf-8", newline="\n"
        )
        payload["manifest.json"] = hashlib.sha256((stage / "manifest.json").read_bytes()).hexdigest()
        (stage / "files.sha256.json").write_text(
            json.dumps(payload, sort_keys=True, indent=2) + "\n", encoding="utf-8", newline="\n"
        )
        shutil.move(str(stage), destination)
        for path in destination.rglob("*"):
            if path.is_file():
                path.chmod(0o555 if path.stat().st_mode & 0o111 else 0o444)
        for path in sorted(
            (item for item in destination.rglob("*") if item.is_dir()),
            key=lambda item: len(item.parts), reverse=True,
        ):
            path.chmod(0o555)
        destination.chmod(0o555)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ("project", "dependency-root", "wheelhouse", "out"):
        parser.add_argument("--" + name, type=Path, required=True)
    parser.add_argument("--architecture", choices=ARCHITECTURES, required=True)
    parser.add_argument("--runtime-root", type=Path)
    args = parser.parse_args()
    build(args.project.resolve(), args.dependency_root, args.wheelhouse,
          args.out, args.architecture, args.runtime_root)
