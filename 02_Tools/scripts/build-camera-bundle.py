#!/usr/bin/env python3
"""Assemble an indexed Camera package from verified target wheels.

Production builds supply a preverified target runtime root. Bench builds may
use the target's Python 3.13 interpreter. No package resolver runs at install.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import shutil
import stat
import tempfile
import zipfile
from pathlib import Path

PILLOW_WHEELS = {
    "arm64": (
        "pillow-11.3.0-cp313-cp313-manylinux_2_27_aarch64.manylinux_2_28_aarch64.whl",
        "c37d8ba9411d6003bba9e518db0db0c58a680ab9fe5179f040b0463644bc9805",
    ),
    "amd64": (
        "pillow-11.3.0-cp313-cp313-manylinux_2_27_x86_64.manylinux_2_28_x86_64.whl",
        "13f87d581e71d9189ab21fe0efb5a23e9f28552d5be6979e84001d3b8505abe8",
    ),
}
PYRAV4L2_WHEEL = (
    "pyrav4l2-1.0-py3-none-any.whl",
    "6b8f92e5bf535531aba1bec9137e5979bc35246ee83c25bd216880b22004d755",
)
DEPENDENCIES = (
    "nilo_integration", "pydantic", "pydantic_core", "annotated_types",
    "typing_inspection", "typing_extensions.py",
)


def copy_tree(source: Path, target: Path) -> None:
    if source.is_dir():
        shutil.copytree(source, target, ignore=shutil.ignore_patterns("__pycache__", "*.pyc"))
    elif source.is_file():
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(source, target)
    else:
        raise ValueError(f"Missing required dependency: {source.name}")


def unpack_verified_wheel(wheelhouse: Path, descriptor: tuple[str, str], target: Path) -> None:
    name, expected = descriptor
    wheel = wheelhouse / name
    if not wheel.is_file() or hashlib.sha256(wheel.read_bytes()).hexdigest() != expected:
        raise ValueError(f"Missing or changed pinned wheel: {name}")
    with zipfile.ZipFile(wheel) as archive:
        for member in archive.infolist():
            relative = Path(member.filename)
            if relative.is_absolute() or ".." in relative.parts:
                raise ValueError("Wheel contains an unsafe path")
            if member.is_dir():
                continue
            destination = target / relative
            destination.parent.mkdir(parents=True, exist_ok=True)
            with archive.open(member) as source, destination.open("xb") as output:
                shutil.copyfileobj(source, output)


def assert_native_architecture(root: Path, architecture: str) -> None:
    machine = {"arm64": 183, "amd64": 62}[architecture]
    native = list(root.rglob("*.so"))
    if not native:
        raise ValueError("Camera bundle has no native Pillow or Pydantic extension")
    for file in native:
        header = file.read_bytes()[:20]
        if len(header) < 20 or header[:4] != b"\x7fELF" or int.from_bytes(header[18:20], "little") != machine:
            raise ValueError(f"Wrong native architecture: {file.name}")


def copy_target_runtime(source: Path, target: Path, architecture: str) -> str:
    """Copy a complete, architecture-matched regular runtime input into the package."""
    if not source.is_dir() or source.is_symlink():
        raise ValueError("Target runtime must be a regular directory")
    manifest = source / "runtime-manifest.json"
    python = source / "bin/python3"
    media_tools = [source / "bin" / name for name in ("ffmpeg", "ffprobe")]
    notices = source / "THIRD_PARTY_NOTICES.txt"
    if any(not item.is_file() or item.is_symlink() for item in (manifest, python, notices, *media_tools)):
        raise ValueError("Target runtime is missing its manifest, Python, FFmpeg/FFprobe or notices")
    details = json.loads(manifest.read_text(encoding="utf-8"))
    if details.get("architecture") != architecture or not notices.read_text(encoding="utf-8").strip():
        raise ValueError("Target runtime architecture or notices are invalid")
    header = python.read_bytes()[:20]
    machine = {"arm64": 183, "amd64": 62}[architecture]
    if len(header) < 20 or header[:6] != b"\x7fELF\x02\x01" or int.from_bytes(header[18:20], "little") != machine:
        raise ValueError("Target interpreter architecture is invalid")
    for executable in media_tools:
        header = executable.read_bytes()[:20]
        if len(header) < 20 or header[:6] != b"\x7fELF\x02\x01" or int.from_bytes(header[18:20], "little") != machine:
            raise ValueError("Target media-tool architecture is invalid")
    total = 0
    observed = []
    for item in sorted(source.rglob("*")):
        mode = item.lstat().st_mode
        if stat.S_ISLNK(mode) or not (stat.S_ISDIR(mode) or stat.S_ISREG(mode)):
            raise ValueError("Target runtime contains a linked or special file")
        if stat.S_ISREG(mode):
            size = item.stat().st_size
            total += size
            if total > 1024 * 1024 * 1024:
                raise ValueError("Target runtime exceeds the one-GiB limit")
            if item != manifest:
                observed.append({"path": item.relative_to(source).as_posix(),
                                 "sha256": hashlib.sha256(item.read_bytes()).hexdigest(),
                                 "size": size})
    if details.get("files") != sorted(observed, key=lambda entry: entry["path"]):
        raise ValueError("Target runtime manifest does not match its files")
    shutil.copytree(source, target)
    return hashlib.sha256(manifest.read_bytes()).hexdigest()


def build(project: Path, dependency_root: Path, wheelhouse: Path, destination: Path,
          architecture: str, runtime_root: Path | None = None) -> None:
    if architecture not in PILLOW_WHEELS:
        raise ValueError("Unsupported target architecture")
    if destination.exists():
        raise ValueError("Bundle destination already exists")
    destination.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix="nilo-camera-bundle-", dir=destination.parent) as temp:
        stage = Path(temp) / "camera"
        python = stage / "python"
        python.mkdir(parents=True)
        for name in DEPENDENCIES:
            copy_tree(dependency_root / name, python / name)
        copy_tree(project / "src/nilo_camera", python / "nilo_camera")
        unpack_verified_wheel(wheelhouse, PILLOW_WHEELS[architecture], python)
        unpack_verified_wheel(wheelhouse, PYRAV4L2_WHEEL, python)
        assert_native_architecture(python, architecture)
        copy_tree(project / "schemas", stage / "schemas")
        bootstrap = (
            "import sys\n"
            "from pathlib import Path\n"
            "sys.dont_write_bytecode = True\n"
            "sys.path.insert(0, str(Path(__file__).resolve().parent / 'python'))\n"
            "from nilo_camera.__main__ import main\n"
            "main()\n"
        )
        if runtime_root is not None:
            dependency_notices = dependency_root.parent / "THIRD_PARTY_NOTICES.txt"
            if (not dependency_notices.is_file() or dependency_notices.is_symlink()
                    or not dependency_notices.read_text(encoding="utf-8").strip()):
                raise ValueError("Camera dependencies require nonempty third-party notices")
            copy_tree(dependency_notices, stage / "THIRD_PARTY_NOTICES.txt")
            copy_target_runtime(runtime_root, stage / "runtime", architecture)
            (stage / "bootstrap.py").write_text(bootstrap, encoding="utf-8", newline="\n")
            (stage / "runner").write_text(
                "#!/bin/sh\n"
                "set -eu\n"
                "root=$(CDPATH= cd -- \"$(dirname -- \"$0\")\" && pwd)\n"
                "exec \"$root/runtime/bin/python3\" -B \"$root/bootstrap.py\" \"$@\"\n",
                encoding="utf-8", newline="\n",
            )
        else:
            (stage / "runner").write_text("#!/usr/bin/env python3\n" + bootstrap,
                                          encoding="utf-8", newline="\n")
        (stage / "runner").chmod(0o755)
        runtime_bytes = sum(p.stat().st_size for p in (stage / "runtime").rglob("*") if p.is_file())
        (stage / "runtime.json").write_text(json.dumps({
            "architecture": architecture,
            "self_contained": runtime_root is not None,
            "backend": "v4l2-ffmpeg",
            "measured_bytes": runtime_bytes,
            "maximum_bytes": (runtime_bytes * 120 + 99) // 100,
            "project_version": (project.parents[2] / "VERSION").read_text().strip(),
        }, sort_keys=True, indent=2) + "\n", encoding="utf-8")
        descriptor = json.loads((project / "manifest.json").read_text(encoding="utf-8"))
        descriptor["platforms"] = [{"os": "linux", "architecture": architecture}]
        payload = {p.relative_to(stage).as_posix(): hashlib.sha256(p.read_bytes()).hexdigest()
                   for p in sorted(stage.rglob("*")) if p.is_file()}
        descriptor["package"]["integrity"] = "sha256:" + hashlib.sha256(
            json.dumps(payload, sort_keys=True).encode()).hexdigest()
        (stage / "manifest.json").write_text(
            json.dumps(descriptor, indent=2) + "\n", encoding="utf-8")
        index = {p.relative_to(stage).as_posix(): hashlib.sha256(p.read_bytes()).hexdigest()
                 for p in sorted(stage.rglob("*")) if p.is_file()}
        (stage / "files.sha256.json").write_text(
            json.dumps(index, indent=2, sort_keys=True) + "\n", encoding="utf-8")
        stage.rename(destination)
        for file in destination.rglob("*"):
            if file.is_file():
                file.chmod(0o555 if file.stat().st_mode & 0o111 else 0o444)
        for directory in sorted((p for p in destination.rglob("*") if p.is_dir()),
                                key=lambda p: len(p.parts), reverse=True):
            directory.chmod(0o555)
        destination.chmod(0o555)


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--project", type=Path, required=True)
    parser.add_argument("--dependency-root", type=Path)
    parser.add_argument("--wheelhouse", type=Path)
    parser.add_argument("--base-bundle", type=Path,
                        help="reuse a fully verified bench bundle's existing target dependencies")
    parser.add_argument("--out", type=Path, required=True)
    parser.add_argument("--architecture", choices=sorted(PILLOW_WHEELS), required=True)
    parser.add_argument("--runtime-root", type=Path,
                        help="preverified architecture-matched interpreter closure")
    arguments = parser.parse_args()
    if arguments.base_bundle:
        if arguments.runtime_root or arguments.dependency_root or arguments.wheelhouse:
            parser.error("--base-bundle is exclusive with fresh build inputs")
        refresh_bench_bundle(arguments.project, arguments.base_bundle, arguments.out, arguments.architecture)
    else:
        if not arguments.dependency_root or not arguments.wheelhouse:
            parser.error("fresh builds require --dependency-root and --wheelhouse")
        build(arguments.project, arguments.dependency_root, arguments.wheelhouse,
              arguments.out, arguments.architecture, arguments.runtime_root)
    print(arguments.out.resolve())


def refresh_bench_bundle(project: Path, base: Path, destination: Path, architecture: str) -> None:
    """Replace first-party code while retaining a verified bench dependency closure."""
    if destination.exists():
        raise ValueError("Bundle destination already exists")
    runtime = json.loads((base / "runtime.json").read_text(encoding="utf-8"))
    if runtime.get("self_contained") is not False or runtime.get("architecture") != architecture:
        raise ValueError("Base must be a matching bench bundle")
    index_bytes = (base / "files.sha256.json").read_bytes()
    index = json.loads(index_bytes)
    observed = {}
    for item in base.rglob("*"):
        if item.is_symlink() or not (item.is_file() or item.is_dir()):
            raise ValueError("Base contains a linked or special file")
        if item.is_file() and item.name != "files.sha256.json":
            observed[item.relative_to(base).as_posix()] = hashlib.sha256(item.read_bytes()).hexdigest()
    if observed != index:
        raise ValueError("Base inventory integrity mismatch")
    destination.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix="nilo-camera-refresh-", dir=destination.parent) as temporary:
        stage = Path(temporary) / "camera"
        shutil.copytree(base, stage)
        for item in [stage, *stage.rglob("*")]:
            item.chmod(0o755 if item.is_dir() or item.name == "runner" else 0o644)
        for relative in ("python/nilo_camera", "schemas"):
            target = (stage / relative).resolve()
            if not target.is_relative_to(stage.resolve()):
                raise ValueError("Unconfined refresh target")
            shutil.rmtree(target)
        copy_tree(project / "src/nilo_camera", stage / "python/nilo_camera")
        copy_tree(project / "schemas", stage / "schemas")
        descriptor = json.loads((project / "manifest.json").read_text(encoding="utf-8"))
        descriptor["platforms"] = [{"os": "linux", "architecture": architecture}]
        payload = {p.relative_to(stage).as_posix(): hashlib.sha256(p.read_bytes()).hexdigest()
                   for p in sorted(stage.rglob("*")) if p.is_file() and p.name not in ("manifest.json", "files.sha256.json")}
        descriptor["package"]["integrity"] = "sha256:" + hashlib.sha256(json.dumps(payload, sort_keys=True).encode()).hexdigest()
        (stage / "manifest.json").write_text(json.dumps(descriptor, indent=2) + "\n", encoding="utf-8")
        inventory = {p.relative_to(stage).as_posix(): hashlib.sha256(p.read_bytes()).hexdigest()
                     for p in sorted(stage.rglob("*")) if p.is_file() and p.name != "files.sha256.json"}
        (stage / "files.sha256.json").write_text(json.dumps(inventory, indent=2, sort_keys=True) + "\n", encoding="utf-8")
        assert_native_architecture(stage / "python", architecture)
        stage.rename(destination)


if __name__ == "__main__":
    main()
