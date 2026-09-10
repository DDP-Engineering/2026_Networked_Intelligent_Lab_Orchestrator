#!/usr/bin/env python3
# -*- coding: utf-8 -*-
# -----------------------------------------------------------------------------
# Copyright (c) 2026 DDP Engineering
# Licensed under the PolyForm Noncommercial License 1.0.0.
# See the repository LICENSE.txt for full license details.
# SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
# -----------------------------------------------------------------------------
"""
@file build_environment.py
@brief Validate and manipulate NILO build-environment metadata and archives.
@details Provides a portable, dependency-free core used by PowerShell and POSIX
         entry points. It never performs Git, forge, or credential operations.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import shlex
import shutil
import subprocess
import sys
import tempfile
import time
import urllib.request
from concurrent.futures import ThreadPoolExecutor
from datetime import UTC, datetime
from pathlib import Path
from typing import Any, Iterable


MAX_PART_SIZE = 2_000_000_000
DEFAULT_PART_SIZE = 1_073_741_824
LFS_POINTER_PREFIX = b"version https://git-lfs.github.com/spec/v1"
SHA256_HEX_LENGTH = 64
GATES = tuple(f"BE-{number:02d}" for number in range(1, 11))
MATURITY_STATES = (
    "implemented",
    "locally_integrity_checked",
    "round_trip_verified",
    "clean_host_verified",
    "published",
    "independently_backed_up",
    "fully_accepted",
)
RELEVANT_SOURCE_PATHS = (
    ".gitattributes",
    ".gitignore",
    "02_Tools",
    "05_Software",
    ":(exclude)02_Tools/archives",
    ":(exclude)02_Tools/cache",
    ":(exclude)02_Tools/runtime",
)


class BuildEnvironmentError(RuntimeError):
    """Represent a fail-closed validation or archive operation error."""


def _load_json(path: Path) -> dict[str, Any]:
    """Load one JSON object and preserve useful path context on failure."""
    try:
        value = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as error:
        raise BuildEnvironmentError(f"cannot read JSON object {path}: {error}") from error
    if not isinstance(value, dict):
        raise BuildEnvironmentError(f"expected a JSON object in {path}")
    return value


def _sha256(path: Path) -> str:
    """Calculate a file SHA-256 without loading large archives into memory."""
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def _require(condition: bool, message: str) -> None:
    """Raise one consistent validation exception when a contract is unmet."""
    if not condition:
        raise BuildEnvironmentError(message)


def _is_sha256(value: Any, *, prefixed: bool = False) -> bool:
    """Return whether a value is a lowercase SHA-256 string."""
    if not isinstance(value, str):
        return False
    if prefixed:
        if not value.startswith("sha256:"):
            return False
        value = value.removeprefix("sha256:")
    return len(value) == SHA256_HEX_LENGTH and all(character in "0123456789abcdef" for character in value)


def validate_lock(path: Path) -> dict[str, Any]:
    """Validate the reviewed environment lock before any networked build."""
    lock = _load_json(path)
    _require(lock.get("schema") == 1, "lock schema must be 1")
    _require(isinstance(lock.get("generation"), str) and lock["generation"], "lock generation is required")
    oci = lock.get("oci")
    _require(isinstance(oci, dict), "OCI format and build frontend lock is required")
    _require(_is_sha256(str(oci.get("build_frontend", "")).rsplit("@", 1)[-1], prefixed=True), "OCI build frontend must be digest-pinned")
    _require(oci.get("image_spec_version") == "1.1.0" and oci.get("layout_version") == "1.0.0", "reviewed OCI image/layout versions are required")
    _require(oci.get("compression") == "gzip", "reviewed OCI archive compression must be gzip")
    base = lock.get("base_image")
    _require(isinstance(base, dict), "base_image object is required")
    reference = base.get("reference", "")
    _require(isinstance(reference, str) and "@sha256:" in reference, "base image must be digest-pinned")
    _require(_is_sha256(reference.rsplit("@", 1)[-1], prefixed=True), "base image digest is invalid")
    acquired = {item["role"]: item for item in oci_images(path)}
    _require({"base", "build-frontend"} <= set(acquired), f"base and build-frontend acquisition records are required: {sorted(acquired)}")
    _require(acquired["base"]["reference"] == reference, "oci_images base record must match base_image.reference")
    _require(acquired["build-frontend"]["reference"] == oci.get("build_frontend"), "oci_images build-frontend record must match oci.build_frontend")
    _require(str(lock.get("ubuntu_snapshot", "")).startswith("https://snapshot.ubuntu.com/"), "immutable Ubuntu snapshot URL is required")
    packages = lock.get("apt_roots")
    _require(isinstance(packages, list) and packages, "apt_roots must be non-empty")
    seen: set[str] = set()
    for package in packages:
        _require(isinstance(package, dict), "every apt package must be an object")
        name = package.get("name")
        _require(isinstance(name, str) and name and name not in seen, f"invalid or duplicate apt package: {name}")
        seen.add(name)
        _require(isinstance(package.get("version"), str) and package["version"], f"{name}: exact version is required")
        _require(_is_sha256(package.get("sha256")), f"{name}: SHA-256 is invalid")
        _require(str(package.get("source", "")).startswith("https://"), f"{name}: HTTPS source is required")
        _require(isinstance(package.get("license"), str) and package["license"], f"{name}: licence metadata is required")
    tools = lock.get("tools")
    _require(isinstance(tools, list) and tools, "tools inventory is required")
    for tool in tools:
        _require(all(isinstance(tool.get(key), str) and tool[key] for key in ("name", "version", "command")), "tool entries require name, version, and command")
    downloads = lock.get("downloads")
    _require(isinstance(downloads, list) and downloads, "download inventory is required")
    for download in downloads:
        name = download.get("name", "unnamed download") if isinstance(download, dict) else "invalid download"
        _require(isinstance(download, dict), "every download must be an object")
        _require(all(isinstance(download.get(key), str) and download[key] for key in ("name", "version", "source", "license")), f"{name}: download identity, source, and licence are required")
        _require(_is_sha256(download.get("sha256")), f"{name}: download SHA-256 is invalid")
    repository_root = path.resolve().parents[3]
    manifest_path = repository_root / str(lock.get("toolchain_manifest", ""))
    _require(manifest_path.is_file(), "tracked toolchain manifest is required")
    values: dict[str, str] = {}
    for raw_line in manifest_path.read_text(encoding="utf-8").splitlines():
        line = raw_line.strip()
        if line and not line.startswith("#"):
            key, separator, value = line.partition("=")
            _require(bool(separator) and key not in values, f"invalid or duplicate toolchain manifest entry: {line}")
            values[key] = value
    expected_versions = {"go": "GO_VERSION", "python": "PYTHON_VERSION", "uv": "UV_VERSION", "node": "NODE_VERSION", "npm": "NPM_VERSION"}
    tool_versions = {item["name"]: item["version"] for item in tools}
    for name, key in expected_versions.items():
        _require(tool_versions.get(name) == values.get(key), f"lock and toolchain.versions disagree for {name}")
    download_hashes = {item["name"]: item["sha256"] for item in downloads}
    for name, key in {"go":"GO_LINUX_AMD64_SHA256", "uv":"UV_LINUX_AMD64_SHA256", "node":"NODE_LINUX_AMD64_SHA256", "playwright-lock":"PLAYWRIGHT_LOCK_SHA256", "playwright-browser-catalog":"PLAYWRIGHT_BROWSER_CATALOG_SHA256"}.items():
        _require(download_hashes.get(name) == values.get(key), f"lock and toolchain.versions disagree for {name} digest")
    package_lock = repository_root / "05_Software/nilo_gui/package-lock.json"
    _require(_sha256(package_lock) == download_hashes["playwright-lock"], "GUI package lock digest does not match the environment lock")
    return lock


def validate_reconstruction(path: Path) -> dict[str, Any]:
    """Refuse a network rebuild until every consumed payload is materialized."""
    lock = validate_lock(path)
    reconstruction = lock.get("reconstruction")
    _require(isinstance(reconstruction, dict), "reconstruction closure status is required")
    if reconstruction.get("status") != "complete":
        missing = reconstruction.get("missing", [])
        detail = "; ".join(str(item) for item in missing) or "missing items are not recorded"
        raise BuildEnvironmentError(
            "complete materialized input closure is not available; reconstruction "
            f"remains disabled: {detail}"
        )
    inputs = _reconstruction_inputs(path, require_https=True)
    required_kinds = {"apt", "go", "uv", "node", "python", "npm", "go-modules", "python-packages", "chromium"}
    kinds = {item["kind"] for item in inputs}
    _require(required_kinds <= kinds, f"reconstruction input kinds are incomplete: {sorted(required_kinds - kinds)}")
    indexed = {(item["name"], item["version"]): item for item in inputs}
    for root in lock["apt_roots"]:
        item = indexed.get((root["name"], root["version"]))
        _require(item is not None and item["kind"] == "apt", f"apt root is absent from the resolved closure: {root['name']}={root['version']}")
        _require(item["sha256"] == root["sha256"] and item["source"] == root["source"], f"apt root identity disagrees with closure: {root['name']}")
    by_name = {item["name"]: item for item in inputs}
    for download in lock["downloads"]:
        if download["name"] in {"go", "uv", "node"}:
            item = by_name.get(download["name"])
            _require(item is not None and item["sha256"] == download["sha256"] and item["source"] == download["source"], f"download identity is not consumed: {download['name']}")
    python_version = next(item["version"] for item in lock["tools"] if item["name"] == "python")
    _require(by_name.get("python", {}).get("version", "").startswith(python_version), "managed Python payload does not match tool inventory")
    return lock


def _reconstruction_inputs(lock_path: Path, *, require_https: bool) -> list[dict[str, Any]]:
    """Load and validate the canonical reconstruction input records."""
    lock = _load_json(lock_path)
    reconstruction = lock.get("reconstruction")
    _require(isinstance(reconstruction, dict) and reconstruction.get("status") == "complete", "complete reconstruction input records are required")
    if "manifest" in reconstruction:
        repository_root = lock_path.resolve().parents[3]
        manifest_path = repository_root / str(reconstruction["manifest"])
        _require(manifest_path.is_file(), f"reconstruction input manifest is missing: {manifest_path}")
        _require(_is_sha256(reconstruction.get("manifest_sha256")), "reconstruction manifest SHA-256 is invalid")
        _require(_sha256(manifest_path) == reconstruction["manifest_sha256"], "reconstruction input manifest SHA-256 mismatch")
        manifest = _load_json(manifest_path)
        _require(manifest.get("schema") == 1 and manifest.get("status") == "complete", "reconstruction input manifest must be complete schema 1")
        inputs = manifest.get("inputs")
        _require(reconstruction.get("input_count") == len(inputs) if isinstance(inputs, list) else False, "reconstruction input count mismatch")
    else:
        inputs = reconstruction.get("inputs")
    _require(isinstance(inputs, list) and inputs, "complete reconstruction requires consumed input records")
    cache_names: set[str] = set()
    for item in inputs:
        _require(isinstance(item, dict), "reconstruction input records must be objects")
        for key in ("name", "version", "kind", "cache_name", "source", "license", "consumer"):
            _require(isinstance(item.get(key), str) and item[key], f"every reconstruction input requires {key}")
        cache_name = item["cache_name"]
        cache_path = Path(cache_name)
        _require(not cache_path.is_absolute() and ".." not in cache_path.parts and cache_name not in cache_names, f"invalid or duplicate reconstruction cache name: {cache_name}")
        cache_names.add(cache_name)
        _require(_is_sha256(item.get("sha256")), f"{cache_name}: SHA-256 is invalid")
        _require(isinstance(item.get("size"), int) and item["size"] > 0, f"{cache_name}: positive payload size is required")
        if require_https:
            _require(item["source"].startswith("https://"), f"{cache_name}: materialized payload source must use HTTPS")
    return inputs


def verify_materialized_inputs(lock_path: Path, input_dir: Path) -> None:
    """Verify the exact materialized input tree and reject extra payloads."""
    inputs = _reconstruction_inputs(lock_path, require_https=False)
    root = input_dir.resolve()
    expected = {item["cache_name"] for item in inputs}
    actual = {
        path.relative_to(root).as_posix()
        for path in root.rglob("*")
        if path.is_file() or path.is_symlink()
    } if root.is_dir() else set()
    missing = sorted(expected - actual)
    extra = sorted(actual - expected)
    if missing:
        raise BuildEnvironmentError(f"missing materialized input: {missing[0]}")
    if extra:
        raise BuildEnvironmentError(f"unexpected materialized input: {extra[0]}")
    for item in inputs:
        path = root / item["cache_name"]
        _require(path.is_file() and not path.is_symlink(), f"materialized input is not a regular file: {item['cache_name']}")
        _require(path.stat().st_size == item["size"], f"size mismatch for materialized input {item['cache_name']}")
        _require(_sha256(path) == item["sha256"], f"SHA-256 mismatch for materialized input {item['cache_name']}")


def _run_git(root: Path, *arguments: str) -> bytes:
    """Run Git without line-oriented filename decoding."""
    try:
        return subprocess.run(
            ["git", "-C", str(root), *arguments],
            check=True,
            capture_output=True,
        ).stdout
    except (OSError, subprocess.CalledProcessError) as error:
        raise BuildEnvironmentError(f"cannot establish source-tree identity: {error}") from error


def verify_source_state(repository_root: Path) -> dict[str, str]:
    """Hash the exact relevant candidate content, including dirty inputs."""
    root = repository_root.resolve()
    commit = _run_git(root, "rev-parse", "HEAD").decode("ascii").strip()
    _require(len(commit) == 40 and all(character in "0123456789abcdef" for character in commit), "Git HEAD identity is invalid")
    path_output = _run_git(
        root, "ls-files", "-z", "--cached", "--others", "--exclude-standard",
        "--", *RELEVANT_SOURCE_PATHS,
    )
    paths = sorted(set(path_output.rstrip(b"\0").split(b"\0")) - {b""})
    stage_output = _run_git(root, "ls-files", "--stage", "-z", "--", *RELEVANT_SOURCE_PATHS)
    index_modes: dict[bytes, str] = {}
    for record in stage_output.rstrip(b"\0").split(b"\0"):
        if not record:
            continue
        metadata, separator, relative_path = record.partition(b"\t")
        _require(bool(separator), "unexpected Git index record while hashing source")
        mode = metadata.split(b" ", 1)[0].decode("ascii")
        index_modes[relative_path] = mode
    filemode = _run_git(root, "config", "--bool", "core.filemode").decode("ascii").strip() == "true"
    digest = hashlib.sha256(b"NILO-SOURCE-TREE-v1\0")
    for relative_bytes in paths:
        relative = os.fsdecode(relative_bytes)
        path = root / Path(relative)
        if path.is_symlink():
            kind = "symlink"
            mode = "120000"
            payload = os.fsencode(os.readlink(path))
        elif path.is_file():
            kind = "file"
            indexed_mode = index_modes.get(relative_bytes)
            if indexed_mode in {"100644", "100755"}:
                mode = indexed_mode
            elif relative.replace("\\", "/").endswith(".sh"):
                mode = "100755"
            elif not filemode:
                mode = "100644"
            else:
                mode = "100755" if path.stat().st_mode & 0o111 else "100644"
            payload_digest = hashlib.sha256()
            with path.open("rb") as stream:
                for chunk in iter(lambda: stream.read(1024 * 1024), b""):
                    payload_digest.update(chunk)
            payload = payload_digest.digest()
        elif not path.exists():
            kind = "missing"
            mode = index_modes.get(relative_bytes, "000000")
            payload = b""
        else:
            raise BuildEnvironmentError(f"unsupported source entry type: {relative}")
        record = json.dumps(
            {"kind": kind, "mode": mode, "path": relative.replace("\\", "/")},
            ensure_ascii=False,
            separators=(",", ":"),
            sort_keys=True,
        ).encode("utf-8")
        digest.update(len(record).to_bytes(8, "big"))
        digest.update(record)
        digest.update(len(payload).to_bytes(8, "big"))
        digest.update(payload)
    status = _run_git(
        root, "status", "--porcelain=v1", "-z", "--untracked-files=all",
        "--", *RELEVANT_SOURCE_PATHS,
    )
    return {
        "base_commit": commit,
        "tree_sha256": digest.hexdigest(),
        "status": "dirty" if status else "clean",
    }


def verify_committed_source(repository_root: Path, expected_tree_sha256: str) -> dict[str, str]:
    """Confirm a later clean commit contains the approved candidate content."""
    _require(_is_sha256(expected_tree_sha256), "expected source-tree SHA-256 is invalid")
    identity = verify_source_state(repository_root)
    _require(identity["status"] == "clean", "source tree is not clean after approval commit")
    _require(
        identity["tree_sha256"] == expected_tree_sha256,
        "committed source tree does not match the approved candidate digest",
    )
    return identity


def verify_image_source(current_tree_sha256: str, image_tree_sha256: str) -> None:
    """Reject an image whose embedded candidate identity is absent or stale."""
    _require(_is_sha256(current_tree_sha256), "current source-tree SHA-256 is invalid")
    _require(_is_sha256(image_tree_sha256), "image source-tree SHA-256 label is missing or invalid")
    _require(
        current_tree_sha256 == image_tree_sha256,
        "stale image: source-tree SHA-256 label does not match the current candidate",
    )


def materialize_inputs(lock_path: Path, cache_path: Path) -> None:
    """Download the complete lock-listed closure with bounded retries."""
    validate_reconstruction(lock_path)
    inputs = _reconstruction_inputs(lock_path, require_https=True)
    cache_path.mkdir(parents=True, exist_ok=True)

    def materialize(item: dict[str, Any]) -> str:
        source = item["source"]
        destination = cache_path / item["cache_name"]
        destination.parent.mkdir(parents=True, exist_ok=True)
        if destination.is_file() and destination.stat().st_size == item["size"] and _sha256(destination) == item["sha256"]:
            return f"verified cached input: {item['cache_name']}"
        temporary = destination.with_suffix(destination.suffix + ".download")
        temporary.unlink(missing_ok=True)
        last_error: Exception | None = None
        for attempt, delay in enumerate((0, 1, 2, 4), start=1):
            if delay:
                time.sleep(delay)
            try:
                request = urllib.request.Request(source, headers={"User-Agent": "NILO-build-environment/1"})
                with urllib.request.urlopen(request, timeout=120) as response, temporary.open("wb") as output:
                    shutil.copyfileobj(response, output, 1024 * 1024)
                _require(temporary.stat().st_size == item["size"], f"download size mismatch for {item['cache_name']} from {source}")
                _require(_sha256(temporary) == item["sha256"], f"download SHA-256 mismatch for {item['cache_name']} from {source}")
                os.replace(temporary, destination)
                return f"downloaded and verified input: {item['cache_name']}"
            except Exception as error:
                last_error = error
                temporary.unlink(missing_ok=True)
                if attempt == 4:
                    break
        if isinstance(last_error, BuildEnvironmentError):
            raise last_error
        raise BuildEnvironmentError(f"cannot materialize {item['cache_name']} from {source} after 4 attempts: {last_error}") from last_error

    with ThreadPoolExecutor(max_workers=8) as executor:
        for message in executor.map(materialize, inputs):
            print(message)
    verify_materialized_inputs(lock_path, cache_path)


def oci_images(lock_path: Path) -> list[dict[str, Any]]:
    """Load and validate the digest-pinned OCI images reconstruction acquires."""
    lock = _load_json(lock_path)
    images = lock.get("oci_images")
    _require(isinstance(images, list) and images, "oci_images acquisition records are required")
    roles: set[str] = set()
    for item in images:
        _require(isinstance(item, dict), "every OCI image record must be an object")
        role = item.get("role")
        _require(isinstance(role, str) and role and role not in roles, f"invalid or duplicate OCI image role: {role}")
        roles.add(role)
        digest = item.get("digest", "")
        _require(_is_sha256(digest, prefixed=True), f"{role}: OCI image digest is invalid")
        reference = item.get("reference", "")
        _require(
            isinstance(reference, str) and reference.endswith("@" + digest),
            f"{role}: OCI image reference must be digest-pinned and match its recorded digest",
        )
        source = item.get("source", "")
        _require(
            isinstance(source, str) and source.startswith("docker://") and source.endswith("@" + digest),
            f"{role}: immutable docker:// source pinned to the same digest is required",
        )
        _require(item.get("platform") == "linux/amd64", f"{role}: reviewed platform must be linux/amd64")
        _require(isinstance(item.get("license"), str) and item["license"], f"{role}: licence metadata is required")
    return images


def _engine_command(engine: str) -> list[str]:
    """Resolve the engine invocation, allowing an injected test double."""
    override = os.environ.get("NILO_ENGINE_COMMAND", "").strip()
    if not override:
        return [engine]
    # Windows paths use backslashes, which POSIX-mode shlex would treat as escapes.
    tokens = shlex.split(override, posix=os.name != "nt")
    return [token.strip('"') for token in tokens]


def _local_repository_digests(engine: str, reference: str) -> list[str]:
    """Return the local engine's recorded repository digests for a reference."""
    result = subprocess.run(
        [*_engine_command(engine), "image", "inspect", "--format", "{{json .RepoDigests}}", reference],
        text=True,
        capture_output=True,
        check=False,
    )
    if result.returncode != 0:
        return []
    try:
        digests = json.loads(result.stdout.strip() or "[]")
    except json.JSONDecodeError:
        return []
    return [str(item) for item in digests] if isinstance(digests, list) else []


def acquire_oci_images(lock_path: Path, engine: str) -> None:
    """Acquire every reviewed base/frontend image by immutable digest.

    This is the networked materialization phase. It never accepts a mutable
    tag and never returns successfully unless the local engine reports the
    exact reviewed digest, so the later offline build cannot silently depend
    on whatever the host happened to cache earlier.
    """
    for item in oci_images(lock_path):
        role, reference, digest = item["role"], item["reference"], item["digest"]
        present = _local_repository_digests(engine, reference)
        if any(candidate.endswith("@" + digest) for candidate in present):
            print(f"verified local OCI image: {role} {reference}")
            continue
        pull = subprocess.run([*_engine_command(engine), "pull", reference], text=True, capture_output=True, check=False)
        if pull.returncode != 0:
            raise BuildEnvironmentError(
                f"cannot acquire {role} image {reference}: {(pull.stderr or pull.stdout).strip()}"
            )
        acquired = _local_repository_digests(engine, reference)
        if not any(candidate.endswith("@" + digest) for candidate in acquired):
            raise BuildEnvironmentError(
                f"{role} image digest mismatch: expected {digest}, local engine reports "
                f"{', '.join(acquired) or 'no repository digest'}"
            )
        print(f"acquired and verified OCI image: {role} {reference}")


def _tool_command(variable: str, default: str) -> list[str]:
    """Resolve a dpkg helper invocation, allowing an injected test double."""
    override = os.environ.get(variable, "").strip()
    if not override:
        return [default]
    tokens = shlex.split(override, posix=os.name != "nt")
    return [token.strip('"') for token in tokens]


def _control_field(package: Path, field: str) -> str:
    """Read one raw control field value from a Debian package."""
    result = subprocess.run(
        [*_tool_command("NILO_DPKG_DEB_COMMAND", "dpkg-deb"), "-f", str(package), field],
        text=True, capture_output=True, check=False,
    )
    _require(result.returncode == 0, f"cannot read {field} from {package.name}: {result.stderr.strip()}")
    return result.stdout.strip()


def _installed_package(name: str) -> tuple[str, str] | None:
    """Return the installed version and architecture, or None when absent."""
    result = subprocess.run(
        [*_tool_command("NILO_DPKG_QUERY_COMMAND", "dpkg-query"), "-W", "-f=${Version}\n${Architecture}", name],
        text=True, capture_output=True, check=False,
    )
    if result.returncode != 0:
        return None
    fields = result.stdout.strip().splitlines()
    if len(fields) < 2 or not fields[0]:
        return None
    return fields[0].strip(), fields[1].strip()


def _version_relation(left: str, relation: str, right: str) -> bool:
    """Ask dpkg for the authoritative Debian version ordering."""
    result = subprocess.run(
        [*_tool_command("NILO_DPKG_COMMAND", "dpkg"), "--compare-versions", left, relation, right],
        text=True, capture_output=True, check=False,
    )
    return result.returncode == 0


def plan_host_transaction(package_dir: Path, allow_downgrades: bool) -> None:
    """Compute and display the exact host package transaction, fail-closed.

    Checksum verification proves package identity; it does not make host
    mutation safe. This plan is calculated and printed before any package is
    installed, and refuses downgrades and architecture replacements unless the
    operator has explicitly authorized them.
    """
    _require(package_dir.is_dir(), f"{package_dir} is not a directory of verified packages")
    packages = sorted(path for path in package_dir.rglob("*.deb"))
    _require(bool(packages), f"no Debian packages were found under {package_dir}")
    rows: list[str] = []
    refused: list[str] = []
    authorized: list[str] = []
    for package in packages:
        name = _control_field(package, "Package")
        version = _control_field(package, "Version")
        architecture = _control_field(package, "Architecture")
        current = _installed_package(name)
        if current is None:
            rows.append(f"  install    {name} {version} ({architecture})")
            continue
        installed_version, installed_architecture = current
        if architecture not in {installed_architecture, "all"} and installed_architecture != "all":
            refused.append(
                f"{name}: incompatible architecture replacement {installed_architecture} -> {architecture}"
            )
            continue
        if _version_relation(version, "eq", installed_version):
            rows.append(f"  reinstall  {name} {version} (unchanged)")
        elif _version_relation(version, "lt", installed_version):
            entry = f"{name}: downgrade {installed_version} -> {version}"
            if allow_downgrades:
                authorized.append(entry)
                rows.append(f"  downgrade  {name} {installed_version} -> {version} (explicitly authorized)")
            else:
                refused.append(entry)
        else:
            rows.append(f"  upgrade    {name} {installed_version} -> {version}")
    print(f"host package transaction ({len(packages)} verified packages):")
    for row in rows:
        print(row)
    if authorized:
        print("explicitly authorized downgrades:")
        for entry in authorized:
            print(f"  {entry}")
    if refused:
        detail = "; ".join(refused)
        raise BuildEnvironmentError(
            f"refusing the host transaction: {detail}. No host package was changed. "
            f"Re-run with --allow-downgrades only after reviewing each entry, or obtain a "
            f"host-bootstrap generation that matches this host."
        )

    command = [
        *_tool_command("NILO_APT_GET_COMMAND", "apt-get"),
        "--simulate", "--no-download", "--no-install-recommends", "--no-remove",
    ]
    if allow_downgrades:
        command.append("--allow-downgrades")
    command.extend(["install", *(str(package) for package in packages)])
    simulation = subprocess.run(command, text=True, capture_output=True, check=False)
    simulation_output = "\n".join(
        part.strip() for part in (simulation.stdout, simulation.stderr) if part.strip()
    )
    if simulation.returncode != 0:
        raise BuildEnvironmentError(
            "refusing the host transaction because dependency resolution failed before "
            f"installation: {simulation_output or 'apt-get simulation failed'}. "
            "No host package was changed."
        )
    print("resolved apt transaction (simulation only; no host package changed):")
    if simulation_output:
        print(simulation_output)


def validate_manifest(path: Path) -> dict[str, Any]:
    """Validate archive identity, ordering, limits, and publication semantics."""
    manifest = _load_json(path)
    _require(manifest.get("schema") == 1, "archive manifest schema must be 1")
    _require(isinstance(manifest.get("generation"), str) and manifest["generation"], "generation is required")
    archive = manifest.get("archive")
    image = manifest.get("image")
    parts = manifest.get("parts")
    _require(isinstance(archive, dict), "archive object is required")
    _require(isinstance(image, dict) and _is_sha256(image.get("digest"), prefixed=True), "image digest is invalid")
    _require(isinstance(parts, list), "parts must be an array")
    filenames: set[str] = set()
    for expected_ordinal, part in enumerate(parts):
        _require(isinstance(part, dict), "every part must be an object")
        _require(part.get("ordinal") == expected_ordinal, "part ordinal/order mismatch")
        filename = part.get("filename")
        _require(isinstance(filename, str) and filename and filename not in filenames, "part filename is missing or duplicated")
        filenames.add(filename)
        size = part.get("size")
        _require(isinstance(size, int) and 0 < size <= MAX_PART_SIZE, f"{filename}: part exceeds 2 GB or has invalid size")
        _require(_is_sha256(part.get("sha256")), f"{filename}: SHA-256 is invalid")
        _require(part.get("lfs_oid") == "sha256:" + part["sha256"], f"{filename}: LFS OID must match content SHA-256")
    if parts:
        _require(isinstance(archive.get("size"), int) and archive["size"] > 0, "joined archive size is required")
        _require(_is_sha256(archive.get("sha256")), "joined archive SHA-256 is invalid")
        source = manifest.get("source")
        _require(isinstance(source, dict), "source identity is required")
        base_commit = source.get("base_commit")
        _require(isinstance(base_commit, str) and len(base_commit) == 40 and all(character in "0123456789abcdef" for character in base_commit), "source base commit is invalid")
        _require(_is_sha256(source.get("tree_sha256")), "source-tree SHA-256 is invalid")
        _require(source.get("status") in {"clean", "dirty"}, "source-tree status is invalid")
    verification = manifest.get("verification")
    _require(isinstance(verification, dict), "verification object is required")
    _require(verification.get("status") in {"pending-round-trip", "local-round-trip-complete"}, "verification status is invalid")
    if verification["status"] == "local-round-trip-complete":
        _require(bool(verification.get("verified_at")), "completed round trip requires a verification time")
        _require(bool(verification.get("commands")), "completed round trip requires executed commands")
        _require(bool(verification.get("evidence")), "completed round trip requires evidence")
        _require(not Path(str(verification["evidence"])).is_absolute(), "round-trip evidence path must be manifest-relative")
    publication = manifest.get("publication", {})
    if publication:
        github_status = publication.get("github", {}).get("status")
        _require(github_status in {"absent", "complete", "pending"}, "GitHub archive status must be absent, pending, or complete; partial replicas are unusable")
        _require(publication.get("gitea", {}).get("status") in {"pending", "complete"}, "Gitea archive status must be pending or complete")
        _require(publication.get("backup", {}).get("status") in {"pending", "complete"}, "backup status must be pending or complete")
    return manifest


def _part_diagnostics(manifest_path: Path, archive_path: Path) -> tuple[dict[str, Any], list[str]]:
    """Return manifest and every local part defect before joining anything."""
    manifest = validate_manifest(manifest_path)
    defects: list[str] = []
    expected = {part["filename"] for part in manifest["parts"]}
    actual = {path.name for path in archive_path.glob("*.part[0-9][0-9][0-9]") if path.is_file()}
    for filename in sorted(actual - expected):
        defects.append(f"unexpected archive part: {filename}")
    for part in manifest["parts"]:
        path = archive_path / part["filename"]
        if not path.is_file():
            defects.append(f"missing archive part: {part['filename']} ({part['lfs_oid']})")
            continue
        with path.open("rb") as stream:
            prefix = stream.read(len(LFS_POINTER_PREFIX))
        if prefix == LFS_POINTER_PREFIX:
            defects.append(f"Git LFS pointer is not materialized: {part['filename']} ({part['lfs_oid']})")
            continue
        size = path.stat().st_size
        if size != part["size"]:
            defects.append(f"size mismatch for {part['filename']}: expected {part['size']}, got {size}")
            continue
        actual_hash = _sha256(path)
        if actual_hash != part["sha256"]:
            defects.append(f"SHA-256 mismatch for {part['filename']}: expected {part['sha256']}, got {actual_hash}")
    return manifest, defects


def inspect_parts(manifest_path: Path, archive_path: Path) -> dict[str, Any]:
    """Validate all local parts and emit actionable local-only recovery text."""
    manifest, defects = _part_diagnostics(manifest_path, archive_path)
    if defects:
        detail = "\n".join(f"- {defect}" for defect in defects)
        raise BuildEnvironmentError(
            f"local build-environment archive is unavailable or invalid:\n{detail}\n"
            "Recovery path 1: fetch the named LFS objects for the current clone yourself, then rerun setup.\n"
            "Recovery path 2: obtain the same parts from another repository origin and supply their directory with --archive-path.\n"
            "Recovery path 3: explicitly rebuild from pinned upstream inputs with --rebuild-from-pinned-inputs."
        )
    return manifest


def split_archive(arguments: argparse.Namespace) -> None:
    """Split a complete archive and create its exact portable manifest."""
    archive = Path(arguments.archive).resolve()
    output = Path(arguments.output).resolve()
    lock_path = Path(arguments.lock).resolve()
    _require(archive.is_file(), f"archive does not exist: {archive}")
    _require(0 < arguments.part_size <= MAX_PART_SIZE, "part size must be from 1 through 2000000000 bytes")
    lock = validate_lock(lock_path)
    _require(arguments.generation == lock["generation"], f"requested generation {arguments.generation} does not match lock generation {lock['generation']}")
    _require(len(arguments.source_base_commit) == 40 and all(character in "0123456789abcdef" for character in arguments.source_base_commit), "source base commit must be an exact lowercase Git object ID")
    _require(_is_sha256(arguments.source_tree_sha256), "source-tree SHA-256 is invalid")
    _require(arguments.source_status in {"clean", "dirty"}, "source-tree status must be clean or dirty")
    repository_root = lock_path.parents[3]
    try:
        relative_lock = lock_path.relative_to(repository_root).as_posix()
    except ValueError as error:
        raise BuildEnvironmentError("lock manifest must be inside the repository") from error
    output.mkdir(parents=True, exist_ok=True)
    stem = f"nilo-build-env-{arguments.generation}.oci.tar.gz"
    existing_parts = sorted(output.glob(f"{stem}.part*"))
    if existing_parts:
        raise BuildEnvironmentError(
            "refusing to overwrite existing generation parts; remove or archive "
            f"them explicitly first: {existing_parts[0]}"
        )
    parts: list[dict[str, Any]] = []
    with archive.open("rb") as source:
        ordinal = 0
        while True:
            filename = f"{stem}.part{ordinal:03d}"
            part_path = output / filename
            temporary_part = output / f".{filename}.tmp"
            digest = hashlib.sha256()
            part_bytes = 0
            try:
                with temporary_part.open("wb") as destination:
                    while part_bytes < arguments.part_size:
                        chunk = source.read(min(1024 * 1024, arguments.part_size - part_bytes))
                        if not chunk:
                            break
                        destination.write(chunk)
                        digest.update(chunk)
                        part_bytes += len(chunk)
                if part_bytes == 0:
                    temporary_part.unlink(missing_ok=True)
                    break
                os.replace(temporary_part, part_path)
            finally:
                temporary_part.unlink(missing_ok=True)
            digest_hex = digest.hexdigest()
            parts.append({"ordinal": ordinal, "filename": filename, "size": part_bytes, "sha256": digest_hex, "lfs_oid": f"sha256:{digest_hex}"})
            ordinal += 1
    _require(parts, "cannot split an empty archive")
    manifest = {
        "schema": 1,
        "generation": arguments.generation,
        "archive": {"filename": stem, "media_type": lock["oci"]["archive_media_type"], "compression": lock["oci"]["compression"], "size": archive.stat().st_size, "sha256": _sha256(archive), "uncompressed_size": arguments.uncompressed_size},
        "image": {"digest": arguments.image_digest, "reference": lock["image"]["reference"], "platform": "linux/amd64"},
        "parts": parts,
        "source": {"base_commit": arguments.source_base_commit, "tree_sha256": arguments.source_tree_sha256, "status": arguments.source_status, "lock_manifest": relative_lock, "lock_sha256": _sha256(lock_path)},
        "creation": {"command": "export-build-environment.sh", "builder": os.environ.get("NILO_BUILDER_ID", "not-recorded"), "created_at": datetime.now(UTC).replace(microsecond=0).isoformat(), "supported_host": "Ubuntu 24.04 linux/amd64 (native or WSL2)"},
        "artifacts": {"sbom": f"metadata/{arguments.generation}/sbom.spdx.json", "licenses": f"metadata/{arguments.generation}/licenses.tar.gz", "provenance": f"metadata/{arguments.generation}/provenance.json"},
        "verification": {"status": "pending-round-trip", "verified_at": None, "commands": [], "evidence": None},
        "publication": {"gitea": {"status": "pending", "readback": "pending", "capacity_bytes": None}, "backup": {"status": "pending", "verified_at": None, "capacity_bytes": None}, "github": {"status": "absent", "readback": "not-required", "allowance_bytes": None}},
    }
    manifest_path = output / "current.json"
    temporary = output / ".current.json.tmp"
    temporary.write_text(json.dumps(manifest, indent=2) + "\n", encoding="utf-8")
    validate_manifest(temporary)
    temporary.replace(manifest_path)
    print(manifest_path)


def free_space(path: Path, require_writable: bool = True) -> int:
    """Return usable bytes without requiring daemon-owned storage to be writable."""
    if not path.is_dir():
        if require_writable:
            raise BuildEnvironmentError(f"{path} is not a writable directory for build-environment temporaries")
        raise BuildEnvironmentError(f"{path} is not a directory that can be checked for free space")
    if require_writable and not os.access(path, os.W_OK):
        raise BuildEnvironmentError(f"{path} is not a writable directory for build-environment temporaries")
    try:
        return shutil.disk_usage(path).free
    except OSError as error:
        raise BuildEnvironmentError(f"cannot check free space for {path}: {error}") from error


def _running_under_wsl() -> bool:
    """Detect a WSL distribution, whose root filesystem is a virtual disk."""
    if os.environ.get("NILO_TEST_FORCE_WSL") == "1":
        return True
    if os.environ.get("WSL_DISTRO_NAME"):
        return True
    try:
        return "microsoft" in Path("/proc/version").read_text(encoding="utf-8", errors="ignore").lower()
    except OSError:
        return False


def _wsl_host_volume_free(path: Path) -> tuple[Path, int] | None:
    """Return the Windows volume backing a WSL ext4 disk and its free bytes.

    Space reported inside the distribution is not evidence that a large file
    will fit: the ext4 virtual disk is sparse and can only grow while the
    Windows volume holding it still has room. Paths already under /mnt are
    Windows volumes themselves and need no second check.
    """
    if not _running_under_wsl():
        return None
    resolved = path.resolve()
    if resolved.parts[:2] == (os.sep, "mnt"):
        return None
    configured_host_volume = os.environ.get("NILO_WSL_HOST_VOLUME", "").strip()
    _require(
        bool(configured_host_volume),
        "NILO_WSL_HOST_VOLUME must name the actual Windows volume backing this WSL "
        "distribution (for example /mnt/c); use the Windows bootstrap to set it automatically",
    )
    host_volume = Path(configured_host_volume)
    _require(host_volume.is_dir(), f"configured WSL host volume is unavailable: {host_volume}")
    forced = os.environ.get("NILO_TEST_HOST_VOLUME_FREE_BYTES")
    if forced is not None:
        return host_volume, int(forced)
    try:
        return host_volume, shutil.disk_usage(host_volume).free
    except OSError as error:
        raise BuildEnvironmentError(f"cannot check WSL host-volume free space for {host_volume}: {error}") from error


def check_space(
    path: Path,
    required_bytes: int,
    purpose: str,
    host_volume: str = "strict",
    require_writable: bool = True,
) -> None:
    """Fail before any long operation that cannot fit on the chosen filesystem.

    `host_volume` selects how a short Windows volume behind a WSL virtual disk
    is treated. Use "strict" where the operation certainly writes that many new
    bytes, such as a staging file. Use "advisory" where it may not, such as an
    image import whose layers can already exist, or where the sparse disk can
    reuse space it has already claimed from the host volume.
    """
    available = free_space(path, require_writable=require_writable)
    if available < required_bytes:
        raise BuildEnvironmentError(
            f"insufficient free space for {purpose} in {path}: required {required_bytes} bytes, "
            f"available {available} bytes; free space or select another directory with "
            f"--temporary-dir (or NILO_TMPDIR)"
        )
    host = _wsl_host_volume_free(path)
    if host is not None:
        host_mount, host_available = host
        if host_available < required_bytes:
            message = (
                f"{path} reports {available} bytes, but this distribution's ext4 virtual disk can "
                f"only grow while {host_mount} has room, and {host_mount} has {host_available} bytes "
                f"free against a required {required_bytes} bytes"
            )
            if host_volume == "strict":
                raise BuildEnvironmentError(
                    f"insufficient free space for {purpose}: {message}. Free space on the Windows "
                    f"volume, or pass --temporary-dir pointing at a Windows volume with room "
                    f"(for example /mnt/d/Temp)."
                )
            print(
                f"warning: {purpose} may not fit: {message}. The import can still succeed when the "
                f"virtual disk reuses space it already holds, or when existing layers are reused.",
                file=sys.stderr,
            )
            return
        print(
            f"space check passed for {purpose} in {path}: required {required_bytes}, "
            f"available {available}, backing host volume {host_mount} free {host_available}"
        )
        return
    print(f"space check passed for {purpose} in {path}: required {required_bytes}, available {available}")


def stream_archive(manifest_path: Path, archive_path: Path, output: Path | None) -> None:
    """Stream the verified joined archive without a second full-size temporary.

    Parts are hashed individually by `inspect_parts` first; the complete archive
    digest is then verified while the bytes are streamed, so a corrupt archive
    fails before any import and every part is left untouched on disk.
    """
    manifest = inspect_parts(manifest_path, archive_path)
    expected = manifest["archive"]["sha256"]
    digest = hashlib.sha256()
    total = 0
    destination = sys.stdout.buffer if output is None else output.open("wb")
    try:
        for part in manifest["parts"]:
            with (archive_path / part["filename"]).open("rb") as stream:
                while chunk := stream.read(1024 * 1024):
                    digest.update(chunk)
                    total += len(chunk)
                    destination.write(chunk)
        destination.flush()
    finally:
        if output is not None:
            destination.close()
    _require(total == manifest["archive"]["size"], f"joined archive size mismatch: expected {manifest['archive']['size']}, streamed {total}")
    _require(digest.hexdigest() == expected, f"joined archive SHA-256 mismatch: expected {expected}, streamed {digest.hexdigest()}")


def join_archive(manifest_path: Path, archive_path: Path, output: Path) -> None:
    """Verify then join parts through a temporary file and atomic replacement."""
    manifest = inspect_parts(manifest_path, archive_path)
    output.parent.mkdir(parents=True, exist_ok=True)
    descriptor, temporary_name = tempfile.mkstemp(prefix="nilo-build-env-", suffix=".join", dir=output.parent)
    os.close(descriptor)
    temporary = Path(temporary_name)
    try:
        with temporary.open("wb") as joined:
            for part in manifest["parts"]:
                with (archive_path / part["filename"]).open("rb") as stream:
                    shutil.copyfileobj(stream, joined, 1024 * 1024)
        _require(temporary.stat().st_size == manifest["archive"]["size"], "joined archive size mismatch")
        _require(_sha256(temporary) == manifest["archive"]["sha256"], "joined archive SHA-256 mismatch")
        os.replace(temporary, output)
    finally:
        temporary.unlink(missing_ok=True)


def record_round_trip(manifest_path: Path, evidence_path: Path, commands: list[str]) -> None:
    """Promote archive verification only after a non-empty real evidence log."""
    manifest = validate_manifest(manifest_path)
    _require(bool(manifest["parts"]), "cannot verify a manifest without archive parts")
    _require(evidence_path.is_file() and evidence_path.stat().st_size > 0, "round-trip evidence log is missing or empty")
    _require(commands, "round-trip verification commands are required")
    try:
        evidence = evidence_path.resolve().relative_to(manifest_path.resolve().parent).as_posix()
    except ValueError as error:
        raise BuildEnvironmentError("round-trip evidence must be stored below the manifest directory") from error
    manifest["verification"] = {
        "status": "local-round-trip-complete",
        "verified_at": datetime.now(UTC).replace(microsecond=0).isoformat(),
        "commands": commands,
        "evidence": evidence,
    }
    temporary = manifest_path.with_name(f".{manifest_path.name}.tmp")
    temporary.write_text(json.dumps(manifest, indent=2) + "\n", encoding="utf-8")
    validate_manifest(temporary)
    temporary.replace(manifest_path)


def verify_replicas(manifest_path: Path) -> None:
    """Require complete Gitea/readback/backup and all-or-nothing GitHub state."""
    manifest = validate_manifest(manifest_path)
    publication = manifest.get("publication", {})
    gitea = publication.get("gitea", {})
    backup = publication.get("backup", {})
    github = publication.get("github", {})
    _require(gitea.get("status") == "complete" and gitea.get("readback") == "complete", "Gitea complete archive and clean readback evidence are required")
    _require(isinstance(gitea.get("capacity_bytes"), int) and gitea["capacity_bytes"] >= manifest["archive"].get("size", 0), "Gitea capacity/headroom evidence is insufficient")
    _require(backup.get("status") == "complete" and backup.get("verified_at"), "independent Gitea Git/LFS backup verification is required")
    _require(isinstance(backup.get("capacity_bytes"), int) and backup["capacity_bytes"] >= manifest["archive"].get("size", 0), "backup capacity/headroom evidence is insufficient")
    if github.get("status") == "complete":
        _require(github.get("readback") == "complete", "a declared complete GitHub replica requires clean readback")


def verify_acceptance(evidence_path: Path) -> int:
    """Print normalized BE-01..BE-10 status and return nonzero until all pass."""
    evidence = _load_json(evidence_path)
    records = evidence.get("gates")
    _require(isinstance(records, dict), "evidence gates object is required")
    report: dict[str, dict[str, str]] = {}
    all_passed = True
    for gate in GATES:
        record = records.get(gate, {})
        status = record.get("status", "pending") if isinstance(record, dict) else "pending"
        if status not in {"passed", "failed", "pending"}:
            status = "failed"
        command = record.get("command", "") if isinstance(record, dict) else ""
        artifact = record.get("evidence", "") if isinstance(record, dict) else ""
        if status == "passed" and (not command or not artifact):
            status = "failed"
        detail = record.get("detail", "not recorded") if isinstance(record, dict) else "not recorded"
        report[gate] = {"status": status, "detail": detail, "command": command, "evidence": artifact}
        all_passed = all_passed and status == "passed"
    print(json.dumps(report, indent=2))
    verify_maturity(evidence, all_passed)
    return 0 if all_passed else 1


def verify_maturity(evidence: dict[str, Any], all_gates_passed: bool) -> None:
    """Keep the seven maturity states honest and strictly ordered.

    Implemented, locally integrity-checked, round-trip verified, clean-host
    verified, published, independently backed up and fully accepted are
    different things. A later state cannot be claimed without every earlier
    one, and full acceptance requires every gate to have passed.
    """
    maturity = evidence.get("maturity")
    _require(isinstance(maturity, dict), "evidence maturity object is required")
    unknown = set(maturity) - set(MATURITY_STATES)
    _require(not unknown, f"unknown maturity states: {sorted(unknown)}")
    missing = [state for state in MATURITY_STATES if state not in maturity]
    _require(not missing, f"maturity states are missing: {missing}")
    for state in MATURITY_STATES:
        _require(isinstance(maturity[state], bool), f"maturity state {state} must be a boolean")
    for index, state in enumerate(MATURITY_STATES):
        if not maturity[state]:
            continue
        unmet = [earlier for earlier in MATURITY_STATES[:index] if not maturity[earlier]]
        if unmet:
            raise BuildEnvironmentError(
                f"maturity state {state} cannot be claimed while {', '.join(unmet)} "
                f"{'is' if len(unmet) == 1 else 'are'} not claimed"
            )
    if maturity["fully_accepted"] and not all_gates_passed:
        raise BuildEnvironmentError(
            "fully_accepted cannot be claimed while any BE gate is pending or failed"
        )


def build_parser() -> argparse.ArgumentParser:
    """Create the command-line contract shared by repository wrappers."""
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="command", required=True)
    lock_parser = commands.add_parser("validate-lock")
    lock_parser.add_argument("--lock", required=True)
    manifest_parser = commands.add_parser("validate-manifest")
    manifest_parser.add_argument("--manifest", required=True)
    inspect_parser = commands.add_parser("inspect-parts")
    inspect_parser.add_argument("--manifest", required=True)
    inspect_parser.add_argument("--archive-path", required=True)
    join_parser = commands.add_parser("join-archive")
    join_parser.add_argument("--manifest", required=True)
    join_parser.add_argument("--archive-path", required=True)
    join_parser.add_argument("--output", required=True)
    split_parser = commands.add_parser("split-archive")
    split_parser.add_argument("--archive", required=True)
    split_parser.add_argument("--output", required=True)
    split_parser.add_argument("--generation", required=True)
    split_parser.add_argument("--part-size", type=int, default=DEFAULT_PART_SIZE)
    split_parser.add_argument("--uncompressed-size", type=int, required=True)
    split_parser.add_argument("--image-digest", required=True)
    split_parser.add_argument("--lock", required=True)
    split_parser.add_argument("--source-base-commit", required=True)
    split_parser.add_argument("--source-tree-sha256", required=True)
    split_parser.add_argument("--source-status", choices=("clean", "dirty"), required=True)
    replica_parser = commands.add_parser("verify-replicas")
    replica_parser.add_argument("--manifest", required=True)
    acceptance_parser = commands.add_parser("verify-acceptance")
    acceptance_parser.add_argument("--evidence", required=True)
    materialize_parser = commands.add_parser("materialize-inputs")
    materialize_parser.add_argument("--lock", required=True)
    materialize_parser.add_argument("--cache", required=True)
    reconstruction_parser = commands.add_parser("validate-reconstruction")
    reconstruction_parser.add_argument("--lock", required=True)
    materialized_parser = commands.add_parser("verify-materialized-inputs")
    materialized_parser.add_argument("--lock", required=True)
    materialized_parser.add_argument("--input-dir", required=True)
    source_parser = commands.add_parser("verify-source-state")
    source_parser.add_argument("--repository-root", required=True)
    committed_source_parser = commands.add_parser("verify-committed-source")
    committed_source_parser.add_argument("--repository-root", required=True)
    committed_source_parser.add_argument("--expected-tree-sha256", required=True)
    image_source_parser = commands.add_parser("verify-image-source")
    image_source_parser.add_argument("--current-tree-sha256", required=True)
    image_source_parser.add_argument("--image-tree-sha256", required=True)
    host_parser = commands.add_parser("plan-host-transaction")
    host_parser.add_argument("--package-dir", required=True)
    host_parser.add_argument("--allow-downgrades", action="store_true")
    space_parser = commands.add_parser("check-space")
    space_parser.add_argument("--path", required=True)
    space_parser.add_argument("--required-bytes", type=int, required=True)
    space_parser.add_argument("--purpose", required=True)
    space_parser.add_argument("--host-volume", choices=("strict", "advisory"), default="strict")
    space_parser.add_argument("--allow-read-only-path", action="store_true")
    plan_images_parser = commands.add_parser("plan-oci-images")
    plan_images_parser.add_argument("--lock", required=True)
    acquire_images_parser = commands.add_parser("acquire-oci-images")
    acquire_images_parser.add_argument("--lock", required=True)
    acquire_images_parser.add_argument("--engine", choices=("docker", "podman"), required=True)
    round_trip_parser = commands.add_parser("record-round-trip")
    round_trip_parser.add_argument("--manifest", required=True)
    round_trip_parser.add_argument("--evidence", required=True)
    round_trip_parser.add_argument("--command", dest="executed_commands", action="append", required=True)
    return parser


def main(argv: Iterable[str] | None = None) -> int:
    """Dispatch one validation or archive operation with concise errors."""
    arguments = build_parser().parse_args(argv)
    try:
        if arguments.command == "validate-lock":
            validate_lock(Path(arguments.lock))
        elif arguments.command == "validate-manifest":
            validate_manifest(Path(arguments.manifest))
        elif arguments.command == "inspect-parts":
            inspect_parts(Path(arguments.manifest), Path(arguments.archive_path))
        elif arguments.command == "join-archive":
            if arguments.output == "-":
                stream_archive(Path(arguments.manifest), Path(arguments.archive_path), None)
            else:
                join_archive(Path(arguments.manifest), Path(arguments.archive_path), Path(arguments.output))
        elif arguments.command == "plan-host-transaction":
            plan_host_transaction(Path(arguments.package_dir), arguments.allow_downgrades)
        elif arguments.command == "check-space":
            check_space(
                Path(arguments.path), arguments.required_bytes, arguments.purpose,
                arguments.host_volume, require_writable=not arguments.allow_read_only_path,
            )
        elif arguments.command == "split-archive":
            split_archive(arguments)
        elif arguments.command == "verify-replicas":
            verify_replicas(Path(arguments.manifest))
        elif arguments.command == "verify-acceptance":
            return verify_acceptance(Path(arguments.evidence))
        elif arguments.command == "materialize-inputs":
            materialize_inputs(Path(arguments.lock), Path(arguments.cache))
        elif arguments.command == "validate-reconstruction":
            validate_reconstruction(Path(arguments.lock))
        elif arguments.command == "verify-materialized-inputs":
            verify_materialized_inputs(Path(arguments.lock), Path(arguments.input_dir))
        elif arguments.command == "verify-source-state":
            print(json.dumps(verify_source_state(Path(arguments.repository_root)), sort_keys=True))
        elif arguments.command == "verify-committed-source":
            print(json.dumps(verify_committed_source(Path(arguments.repository_root), arguments.expected_tree_sha256), sort_keys=True))
        elif arguments.command == "verify-image-source":
            verify_image_source(arguments.current_tree_sha256, arguments.image_tree_sha256)
        elif arguments.command == "plan-oci-images":
            print(json.dumps(oci_images(Path(arguments.lock)), indent=2, sort_keys=True))
        elif arguments.command == "acquire-oci-images":
            acquire_oci_images(Path(arguments.lock), arguments.engine)
        elif arguments.command == "record-round-trip":
            record_round_trip(Path(arguments.manifest), Path(arguments.evidence), arguments.executed_commands)
        return 0
    except BuildEnvironmentError as error:
        print(f"error: {error}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())

# -----------------------------------------------------------------------------
# END OF FILE
# -----------------------------------------------------------------------------
