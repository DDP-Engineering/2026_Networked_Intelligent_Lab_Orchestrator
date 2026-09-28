#!/usr/bin/env python3
# -*- coding: utf-8 -*-
# -----------------------------------------------------------------------------
# Copyright (c) 2026 DDP Engineering
# Licensed under the PolyForm Noncommercial License 1.0.0.
# See the repository LICENSE.txt for full license details.
# SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
# -----------------------------------------------------------------------------
"""
@file locked_build_inputs.py
@brief Check source-lock coverage and prepare isolated offline Python projects.
@details The canonical target is CPython 3.12 on Linux amd64. Registry wheels
         retain uv.lock hashes; Git archives retain an exact revision and hash.
"""

from __future__ import annotations

import argparse
import json
import os
import re
import subprocess
import sys
import tarfile
import tempfile
import tomllib
from pathlib import Path
from urllib.parse import urlparse

from build_environment import (
    BuildEnvironmentError, _reconstruction_inputs, _require, _sha256,
)


# HELPER FUNCTIONS
def _name(value: str) -> str:
    return re.sub(r"[-_.]+", "-", value).lower()


def _compatible_wheel(filename: str) -> bool:
    """@brief Select universal or CPython 3.12 Linux amd64 locked wheels."""
    python, abi, platform = filename.removesuffix(".whl").rsplit("-", 3)[1:]
    python_tags = python.split(".")
    interpreter = bool({"py3", "cp312"} & set(python_tags))
    if abi == "abi3":
        interpreter = interpreter or any(
            tag.startswith("cp3") and tag[3:].isdigit() and int(tag[3:]) <= 12
            for tag in python_tags
        )
    return interpreter and (
        platform == "any" or any(
            tag.startswith("manylinux") and tag.endswith("_x86_64")
            for tag in platform.split(".")
        )
    )


def _go_object(module: str, version: str, suffix: str) -> str:
    escaped = "".join("!" + char.lower() if char.isupper() else char for char in module)
    escaped_version = "".join("!" + char.lower() if char.isupper() else char for char in version)
    return f"go-proxy/{escaped}/@v/{escaped_version}.{suffix}"


def _source_files(root: Path, filename: str) -> list[Path]:
    """@brief Discover source locks while excluding generated dependency trees."""
    found = []
    for directory, children, files in os.walk(root / "05_Software"):
        children[:] = sorted(child for child in children if not child.startswith(".venv")
                             and child not in {"node_modules", "dist", "__pycache__"})
        if filename in files:
            found.append(Path(directory) / filename)
    return sorted(found)


def _registry_input(package: dict, inputs: list[dict], lock: Path) -> dict:
    """@brief Require one compatible wheel with the exact source-lock identity."""
    for wheel in package.get("wheels", []):
        filename = Path(urlparse(wheel["url"]).path).name
        if not _compatible_wheel(filename):
            continue
        for item in inputs:
            if (item["kind"] == "python-packages"
                    and _name(item["name"]) == _name(package["name"])
                    and item["version"] == package["version"]
                    and item["sha256"] == wheel["hash"].removeprefix("sha256:")
                    and item["size"] == wheel["size"]
                    and item["source"] == wheel["url"]
                    and Path(item["cache_name"]).name == filename):
                return item
    raise BuildEnvironmentError(
        f"{lock}: missing locked Linux amd64 wheel: {package['name']}=={package['version']}"
    )


def python_plan(root: Path, inputs: list[dict]) -> list[tuple[Path, list[dict], list[Path]]]:
    """@brief Resolve every Python project before any environment is changed."""
    projects = _source_files(root, "pyproject.toml")
    result = []
    by_package = {(_name(item["name"]), item["version"]): item for item in inputs
                  if item["kind"] == "python-packages"}
    for project in projects:
        lock = project.with_name("uv.lock")
        _require(lock.is_file(), f"Python project has no lock: {project}")
        selected, local = [], []
        document = tomllib.loads(project.read_text(encoding="utf-8"))
        for requirement in document.get("build-system", {}).get("requires", []):
            name, separator, version = requirement.partition("==")
            _require(bool(separator), f"{project}: build dependency must be pinned: {requirement}")
            item = by_package.get((_name(name), version))
            _require(item is not None, f"{project}: missing locked build dependency: {requirement}")
            selected.append(item)
        for package in tomllib.loads(lock.read_text(encoding="utf-8"))["package"]:
            source = package["source"]
            if "registry" in source:
                selected.append(_registry_input(package, inputs, lock))
            elif "editable" in source:
                directory = (project.parent / source["editable"]).resolve()
                _require(directory.is_relative_to((root / "05_Software").resolve()),
                         f"{lock}: local dependency leaves software tree")
                _require((directory / "pyproject.toml").is_file(), f"{lock}: missing local dependency: {directory}")
                local.append(directory)
            elif "git" in source:
                url, _, revision = source["git"].partition("#")
                _require(bool(re.fullmatch(r"[0-9a-f]{40}", revision)), f"{lock}: Git revision must be exact")
                candidates = [item for item in inputs if item["kind"] == "python-sources"
                              and _name(item["name"]) == _name(package["name"])
                              and item["version"] == package["version"]
                              and item.get("git") == url.split("?", 1)[0]
                              and item.get("revision") == revision]
                _require(len(candidates) == 1, f"{lock}: missing locked Git source: {package['name']}@{revision}")
                item = candidates[0]
                _require(item.get("build_requires") == ["setuptools==80.9.0", "wheel==0.45.1"],
                         f"{lock}: Git source build tools must be pinned")
                for requirement in item["build_requires"]:
                    name, version = requirement.split("==", 1)
                    tool = by_package.get((_name(name), version))
                    _require(tool is not None, f"{lock}: missing Git build tool: {requirement}")
                    selected.append(tool)
                selected.append(item)
            else:
                raise BuildEnvironmentError(f"{lock}: unsupported locked source: {source}")
        result.append((project.parent, selected, local))
    return result


# PUBLIC API
def verify_coverage(root: Path, inputs: list[dict]) -> None:
    """@brief Refuse incomplete source-lock input inventories before build/export."""
    objects = {item["cache_name"] for item in inputs if item["kind"] == "go-modules"}
    for module in _source_files(root, "go.mod"):
        sums = module.with_name("go.sum")
        _require(sums.is_file(), f"Go module has no checksum lock: {module}")
        locked = {tuple(line.split()[:2]) for line in sums.read_text(encoding="utf-8").splitlines()}
        in_require = False
        for line in module.read_text(encoding="utf-8").splitlines():
            line = line.split("//", 1)[0].strip()
            if line == "require (":
                in_require = True
                continue
            if in_require and line == ")":
                in_require = False
                continue
            fields = line.removeprefix("require ").split() if in_require or line.startswith("require ") else []
            if len(fields) == 2:
                _require(tuple(fields) in locked, f"{module}: required module is absent from go.sum: {line}")
        for line in sums.read_text(encoding="utf-8").splitlines():
            name, version, checksum = line.split()
            _require(checksum.startswith("h1:"), f"{sums}: unsupported checksum")
            mod_only = version.endswith("/go.mod")
            version = version.removesuffix("/go.mod")
            for suffix in (("mod",) if mod_only else ("mod", "info", "zip")):
                path = _go_object(name, version, suffix)
                _require(path in objects, f"{sums}: missing locked Go input: {path}")
    python_plan(root, inputs)
    frontend = root / "05_Software/frontend/package-lock.json"
    if frontend.is_file():
        tarballs = {item["source"]: item for item in inputs if item["kind"] == "npm"}
        for package in json.loads(frontend.read_text(encoding="utf-8"))["packages"].values():
            if "resolved" not in package:
                continue
            item = tarballs.get(package["resolved"])
            _require(item is not None and item["version"] == package["version"],
                     f"{frontend}: missing locked npm input: {package['resolved']}")


def prepare_python(root: Path, inputs: list[dict], input_dir: Path, python: str, uv: str) -> None:
    """@brief Install only verified locked inputs, then check runtime closure."""
    plans = python_plan(root, inputs)
    selected = {item["cache_name"]: item for _, items, _ in plans for item in items}
    for item in selected.values():
        path = input_dir / item["cache_name"]
        _require(path.is_file() and not path.is_symlink(), f"missing locked Python input: {path}")
        _require(path.stat().st_size == item["size"] and _sha256(path) == item["sha256"],
                 f"locked Python input checksum mismatch: {path}")
    environment_name = os.environ.get("UV_PROJECT_ENVIRONMENT", ".venv-linux-x86_64")
    _require(environment_name.startswith(".venv") and Path(environment_name).name == environment_name,
             "UV_PROJECT_ENVIRONMENT must be a project-local .venv name")
    env = {**os.environ, "UV_OFFLINE": "1", "SOURCE_DATE_EPOCH": "1788566400"}
    for project, items, local in plans:
        environment = project / environment_name
        subprocess.run([uv, "venv", "--clear", "--python", python, str(environment)], env=env, check=True)
        interpreter = str(environment / "bin/python")
        install = [uv, "pip", "install", "--offline", "--no-index", "--no-deps",
                   "--no-build-isolation", "--python", interpreter]
        wheels = sorted({str(input_dir / item["cache_name"]) for item in items if item["kind"] == "python-packages"})
        subprocess.run([*install, *wheels], env=env, check=True)
        for item in items:
            if item["kind"] != "python-sources":
                continue
            with tempfile.TemporaryDirectory(prefix="nilo-locked-source-") as temporary:
                with tarfile.open(input_dir / item["cache_name"]) as archive:
                    archive.extractall(temporary, filter="data")
                directories = list(Path(temporary).iterdir())
                _require(len(directories) == 1 and directories[0].is_dir(), "Git archive must contain one source root")
                subprocess.run([*install, str(directories[0])], env=env, check=True)
        for directory in local:
            subprocess.run([*install, "-e", str(directory)], env=env, check=True)
        subprocess.run([uv, "pip", "check", "--python", interpreter], env=env, check=True)


def main() -> int:
    """@brief Dispatch source coverage or offline preparation with concise errors."""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", choices=("verify", "prepare-python"))
    parser.add_argument("--repository-root", type=Path, required=True)
    parser.add_argument("--lock", type=Path, required=True)
    parser.add_argument("--input-dir", type=Path, default=Path("/opt/nilo/inputs"))
    parser.add_argument("--python", default="/opt/nilo/python-bin/python3")
    parser.add_argument("--uv", default="uv")
    args = parser.parse_args()
    try:
        inputs = _reconstruction_inputs(args.lock, require_https=True)
        verify_coverage(args.repository_root.resolve(), inputs)
        if args.action == "prepare-python":
            prepare_python(args.repository_root.resolve(), inputs, args.input_dir, args.python, args.uv)
        print("source-lock input coverage verified")
        return 0
    except (BuildEnvironmentError, OSError, ValueError, subprocess.CalledProcessError) as error:
        print(f"error: {error}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())

# END OF FILE
