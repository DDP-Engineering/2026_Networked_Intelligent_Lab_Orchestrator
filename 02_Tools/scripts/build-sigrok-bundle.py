#!/usr/bin/env python3
# -*- coding: utf-8 -*-
# -----------------------------------------------------------------------------
# Copyright (c) 2026 DDP Engineering
# SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
# -----------------------------------------------------------------------------
"""Build a curated Sigrok bundle around one pre-fetched target runtime."""

# IMPORTS
from __future__ import annotations

import argparse
import hashlib
import json
import shutil
import stat
import tempfile
from pathlib import Path


MAXIMUM_RUNTIME_BYTES = 1024 * 1024 * 1024
ELF_MACHINES = {"amd64": 62, "arm64": 183}


def _read_runtime_manifest(runtime_root: Path, architecture: str) -> dict:
    """Load the producer inventory that binds runtime bytes and licences."""
    manifest_path = runtime_root / "runtime-manifest.json"
    try:
        manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
    except (OSError, UnicodeDecodeError, json.JSONDecodeError) as error:
        raise ValueError("Runtime manifest is missing or invalid") from error
    if set(manifest) != {"architecture", "components", "files", "health_checks", "schema"}:
        raise ValueError("Runtime manifest fields are invalid")
    if manifest["schema"] != 1 or manifest["architecture"] != architecture:
        raise ValueError("Runtime manifest architecture is invalid")
    components = manifest["components"]
    if not isinstance(components, list) or not components:
        raise ValueError("Runtime manifest must inventory licensed components")
    for component in components:
        if not isinstance(component, dict) or set(component) != {"license", "name", "source", "version"}:
            raise ValueError("Runtime component inventory is invalid")
        if not all(isinstance(component[field], str) and component[field] for field in component):
            raise ValueError("Runtime component inventory contains empty metadata")
        if not component["source"].startswith("https://"):
            raise ValueError("Runtime component source must use HTTPS")
    health = manifest["health_checks"]
    required_health = {"python-version", "sigrok-version", "sigrok-driver-scan"}
    if not isinstance(health, list) or any(not isinstance(item, dict) for item in health):
        raise ValueError("Runtime health evidence is incomplete")
    if {item.get("id") for item in health} != required_health:
        raise ValueError("Runtime health evidence is incomplete")
    if any(set(item) != {"command", "id", "status"} or item["status"] != "passed" for item in health):
        raise ValueError("Runtime health evidence did not pass")
    return manifest


def _validate_elf(path: Path, architecture: str) -> None:
    header = path.read_bytes()[:20]
    expected_machine = ELF_MACHINES[architecture]
    if len(header) < 20 or header[:4] != b"\x7fELF" or header[4:6] != b"\x02\x01":
        raise ValueError(f"Runtime executable is not a 64-bit little-endian ELF: {path.name}")
    if int.from_bytes(header[18:20], "little") != expected_machine:
        raise ValueError(f"Runtime executable architecture mismatch: {path.name}")


def _validate_runtime(runtime_root: Path, architecture: str) -> int:
    """Validate a regular, licence-described runtime and return its byte size."""
    runtime_root = runtime_root.resolve()
    if not runtime_root.is_dir() or runtime_root.is_symlink():
        raise ValueError("Runtime root must be a regular directory")
    manifest = _read_runtime_manifest(runtime_root, architecture)
    required = (
        runtime_root / "bin/python3",
        runtime_root / "bin/sigrok-cli",
        runtime_root / "THIRD_PARTY_NOTICES.txt",
    )
    for path in required:
        if not path.is_file() or path.is_symlink():
            raise ValueError(f"Runtime is missing required regular file: {path.name}")
    if not (runtime_root / "THIRD_PARTY_NOTICES.txt").read_text(encoding="utf-8").strip():
        raise ValueError("Runtime third-party notices must not be empty")
    _validate_elf(runtime_root / "bin/python3", architecture)
    _validate_elf(runtime_root / "bin/sigrok-cli", architecture)
    total = 0
    observed = []
    for path in sorted(runtime_root.rglob("*")):
        details = path.lstat()
        relative = path.relative_to(runtime_root).as_posix()
        if "fake_python" in relative or "fixture" in relative.casefold():
            raise ValueError(f"Runtime contains a development fixture: {relative}")
        if stat.S_ISLNK(details.st_mode):
            raise ValueError(f"Runtime contains a symbolic link: {relative}")
        if path.is_dir():
            continue
        if not stat.S_ISREG(details.st_mode):
            raise ValueError(f"Runtime contains a special file: {relative}")
        total += details.st_size
        if total > MAXIMUM_RUNTIME_BYTES:
            raise ValueError("Runtime exceeds the reviewed one-GiB safety ceiling")
        if relative != "runtime-manifest.json":
            observed.append(
                {
                    "path": relative,
                    "sha256": hashlib.sha256(path.read_bytes()).hexdigest(),
                    "size": details.st_size,
                }
            )
    if manifest["files"] != sorted(observed, key=lambda item: item["path"]):
        raise ValueError("Runtime manifest file inventory does not match runtime bytes")
    return total


def build(destination: Path, architecture: str, runtime_root: Path) -> None:
    """Assemble code and one complete, already acquired target runtime."""
    repository = Path(__file__).resolve().parents[2]
    project = repository / "05_Software/integrations/sigrok"
    destination = destination.resolve()
    runtime_root = runtime_root.resolve()
    if architecture not in {"arm64", "amd64"}:
        raise ValueError(f"Unsupported architecture: {architecture}")
    measured_bytes = _validate_runtime(runtime_root, architecture)
    if destination.exists():
        raise ValueError("Bundle destination must not already exist")
    destination.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(
        prefix="nilo-bundle-", dir=destination.parent
    ) as temporary:
        stage = Path(temporary) / "sigrok"
        stage.mkdir()
        shutil.copytree(runtime_root, stage / "runtime")
        (stage / "runtime/bin/python3").chmod(0o755)
        (stage / "runtime/bin/sigrok-cli").chmod(0o755)
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
        (stage / "bootstrap.py").write_text(
            "import sys\n"
            "sys.dont_write_bytecode = True\n"
            "from pathlib import Path\n"
            "sys.path.insert(0, str(Path(__file__).resolve().parent / 'python'))\n"
            "from nilo_sigrok.__main__ import main\n"
            "main()\n",
            encoding="utf-8",
        )
        (stage / "runner").write_text(
            "#!/bin/sh\n"
            "set -eu\n"
            "root=$(CDPATH= cd -- \"$(dirname -- \"$0\")\" && pwd)\n"
            "exec \"$root/runtime/bin/python3\" -B \"$root/bootstrap.py\" \"$@\"\n",
            encoding="utf-8",
            newline="\n",
        )
        (stage / "runner").chmod(0o755)
        (stage / "runtime.json").write_text(
            json.dumps(
                {
                    "architecture": architecture,
                    "backend": "runtime/bin/sigrok-cli",
                    "maximum_bytes": (measured_bytes * 120 + 99) // 100,
                    "measured_bytes": measured_bytes,
                    "project_version": (repository / "VERSION").read_text().strip(),
                    "self_contained": True,
                    "source_manifest_sha256": hashlib.sha256(
                        (runtime_root / "runtime-manifest.json").read_bytes()
                    ).hexdigest(),
                },
                indent=2,
                sort_keys=True,
            )
            + "\n",
            encoding="utf-8",
        )
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
    parser.add_argument("--runtime-root", type=Path, required=True)
    args = parser.parse_args()
    build(args.out, args.architecture, args.runtime_root)
    print(args.out.resolve())


if __name__ == "__main__":
    main()

# END OF FILE
