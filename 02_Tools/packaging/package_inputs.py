#!/usr/bin/env python3
# -*- coding: utf-8 -*-
# -----------------------------------------------------------------------------
# Copyright (c) 2026 DDP Engineering
# This software is licensed under the PolyForm Noncommercial License 1.0.0.
# Commercial use requires a separate commercial license.
# See the LICENSE.txt file for full license details.
# SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
# -----------------------------------------------------------------------------
"""
@file package_inputs.py
@brief Validate NILO delivery targets and packaging inputs.
@details
    Provides the strict, dependency-free command boundary shared by connected
    input acquisition and offline package preparation.
"""

# -----------------------------------------------------------------------------
# IMPORTS
# -----------------------------------------------------------------------------

from __future__ import annotations

import argparse
import hashlib
import json
import io
import lzma
import os
import re
import shutil
import stat
import subprocess
import sys
import tempfile
import tarfile
import urllib.parse
import urllib.request
from dataclasses import dataclass
from pathlib import Path, PurePosixPath
from typing import Any, BinaryIO, Callable, Sequence


# -----------------------------------------------------------------------------
# CONSTANTS
# -----------------------------------------------------------------------------

TARGET_ID_PATTERN = re.compile(r"^[a-z0-9][a-z0-9.-]*$")
INPUT_NAME_PATTERN = re.compile(r"^[a-z0-9][a-z0-9._-]*$")
SHA256_PATTERN = re.compile(r"^[0-9a-f]{64}$")
ALLOWED_ARCHITECTURES = frozenset({"amd64", "arm64"})
ALLOWED_ARTIFACTS = frozenset({"deb", "img.xz", "run"})
EXACT_TARGET_DEFINITIONS = {
    "rpi-debian13-arm64": (
        "raspbian",
        "debian",
        "13",
        "arm64",
        ("deb", "img.xz", "run"),
        (
            "administrator-credentials",
            "boot-reboot",
            "filesystem-expansion",
            "gpio-recovery-disabled",
            "gpio-recovery-wired",
            "image-flash-boot",
            "integrations",
            "network",
            "power-cycle",
            "services-gui",
            "unique-identity",
        ),
    ),
    "ubuntu-24.04-amd64": (
        "ubuntu",
        "debian",
        "24.04",
        "amd64",
        ("deb", "run"),
        (
            "boot-reboot",
            "integrations",
            "network",
            "services-gui",
            "usb-device-access",
        ),
    ),
}
TARGET_FIELDS = frozenset(
    {
        "id",
        "os",
        "architecture",
        "artifacts",
        "hardware_acceptance",
    }
)
OS_FIELDS = frozenset({"id", "family", "version_id"})
LOCK_FIELDS = frozenset({"schema", "complete", "incomplete_reasons", "inputs"})
INPUT_FIELDS = frozenset(
    {
        "apt", "architecture", "artifact_type", "license", "name", "provenance",
        "redistribution", "scope", "sha256", "size", "url", "version",
    }
)
DECLARED_INPUT_FIELDS = frozenset(
    {
        "apt", "architecture", "artifact_type", "license", "name", "provenance",
        "redistribution", "scope", "url", "version",
    }
)
APT_POLICY_FIELDS = frozenset(
    {
        "base_packages", "component", "keyring_sha256", "repository", "roots",
        "signing_fingerprints", "suite",
    }
)
APT_LOCK_FIELDS = APT_POLICY_FIELDS | {"packages"}
APT_PACKAGE_FIELDS = frozenset(
    {
        "architecture", "depends", "filename", "package", "pre_depends", "provides",
        "sha256", "signed_by", "size", "version",
    }
)
DEBIAN_PACKAGE_ID_PATTERN = re.compile(r"^[a-z0-9][a-z0-9+.-]*$")
DEBIAN_FILENAME_PATTERN = re.compile(r"^pool/[A-Za-z0-9][A-Za-z0-9+._/~:-]*\.deb$")
SIGNING_FINGERPRINT_PATTERN = re.compile(r"^[0-9A-F]{40}(?:[0-9A-F]{24})?$")
INPUT_ARTIFACT_TYPES = {
    "base-image": "raspberry-pi-image-xz",
    "go-module-cache": "go-module-cache-tar",
    "gui-npm-cache": "npm-cache-tar",
    "os-package-pool": "debian-pool-tar",
    "sigrok-runtime": "sigrok-runtime-tar",
    "camera-build-input": "camera-build-input-tar",
    "source-offers": "source-offers-tar",
}
REQUIRED_RELEASE_INPUTS = {
    "shared": ("go-module-cache", "gui-npm-cache", "source-offers"),
    "rpi-debian13-arm64": ("base-image", "os-package-pool", "sigrok-runtime", "camera-build-input"),
    "ubuntu-24.04-amd64": ("os-package-pool", "sigrok-runtime", "camera-build-input"),
}
DOWNLOAD_CHUNK_SIZE = 1024 * 1024
DOWNLOAD_TIMEOUT_SECONDS = 60
MAXIMUM_RESOLVED_INPUT_BYTES = 16 * 1024 * 1024 * 1024
IPV6_ROUTE_REJECT_FLAG = 0x0200


# -----------------------------------------------------------------------------
# DATA MODELS / TYPEDEFINITIONS
# -----------------------------------------------------------------------------


class PackagingError(ValueError):
    """@brief Report one or more policy or input-validation failures."""

    def __init__(self, messages: str | Sequence[str]) -> None:
        if isinstance(messages, str):
            normalized = (messages,)
        else:
            normalized = tuple(str(message) for message in messages)
        self.messages = normalized
        super().__init__("\n".join(normalized))


class CacheObjectError(PackagingError):
    """@brief Classify a cache-object failure for safe fetch recovery."""

    def __init__(self, kind: str, messages: str | Sequence[str]) -> None:
        self.kind = kind
        super().__init__(messages)


@dataclass(frozen=True)
class Target:
    """@brief One exact supported delivery target."""

    id: str
    os_id: str
    os_family: str
    version_id: str
    architecture: str
    artifacts: tuple[str, ...]
    hardware_acceptance: tuple[str, ...]

    def to_dict(self) -> dict[str, Any]:
        """@brief Return the canonical JSON representation."""
        return {
            "architecture": self.architecture,
            "artifacts": list(self.artifacts),
            "hardware_acceptance": list(self.hardware_acceptance),
            "id": self.id,
            "os": {
                "family": self.os_family,
                "id": self.os_id,
                "version_id": self.version_id,
            },
        }


@dataclass(frozen=True)
class TargetRegistry:
    """@brief Validated delivery-target registry."""

    targets: tuple[Target, ...]

    def require(self, target_id: str) -> Target:
        """
        @brief Resolve a declared target.
        @param[in] target_id Stable target identifier.
        @return The matching target.
        @exception PackagingError The identifier is not declared.
        """
        for target in self.targets:
            if target.id == target_id:
                return target
        raise PackagingError(f"unknown delivery target: {target_id}")

    def to_dict(self) -> dict[str, Any]:
        """@brief Return the canonical registry representation."""
        return {
            "schema": 1,
            "targets": [target.to_dict() for target in self.targets],
        }


@dataclass(frozen=True)
class LockedInput:
    """@brief One immutable external input declared by the packaging lock."""

    name: str
    scope: str
    architecture: str
    artifact_type: str
    url: str
    version: str
    license: str
    provenance: str
    redistribution: str
    sha256: str
    size: int
    apt: dict[str, Any] | None


@dataclass(frozen=True)
class InputLock:
    """@brief Validated packaging-input lock."""

    complete: bool
    incomplete_reasons: tuple[str, ...]
    inputs: tuple[LockedInput, ...]

    def selected(self, target_id: str) -> tuple[LockedInput, ...]:
        """@brief Return stable shared and target-scoped inputs."""
        selected = tuple(
            item for item in self.inputs if item.scope in {"shared", target_id}
        )
        if not selected:
            raise PackagingError(f"lock has no inputs for target {target_id}")
        return selected


@dataclass(frozen=True)
class VerifiedInput:
    """@brief One cache object whose size and digest were observed."""

    name: str
    scope: str
    architecture: str
    artifact_type: str
    version: str
    license: str
    provenance: str
    redistribution: str
    sha256: str
    size: int
    cache_object: str
    apt: dict[str, Any] | None

    def to_dict(self) -> dict[str, Any]:
        """@brief Return the canonical receipt representation."""
        return {
            "apt": self.apt,
            "architecture": self.architecture,
            "artifact_type": self.artifact_type,
            "cache_object": self.cache_object,
            "license": self.license,
            "name": self.name,
            "provenance": self.provenance,
            "redistribution": self.redistribution,
            "scope": self.scope,
            "sha256": self.sha256,
            "size": self.size,
            "version": self.version,
        }


@dataclass(frozen=True)
class NetworkObservation:
    """@brief Linux namespace facts relevant to offline preparation."""

    platform: str
    interfaces: tuple[str, ...]
    ipv4_default_route: bool
    ipv6_default_route: bool

    def to_dict(self) -> dict[str, Any]:
        """@brief Return the canonical receipt representation."""
        return {
            "interfaces": list(self.interfaces),
            "ipv4_default_route": self.ipv4_default_route,
            "ipv6_default_route": self.ipv6_default_route,
            "platform": self.platform,
            "verified": True,
        }


# -----------------------------------------------------------------------------
# LOCAL HELPERS
# -----------------------------------------------------------------------------


def _read_bytes(path: Path) -> bytes:
    try:
        return path.read_bytes()
    except OSError as error:
        raise PackagingError(f"cannot read {path}: {error}") from error


def _decode_json(data: bytes, path: Path) -> Any:
    try:
        text = data.decode("utf-8")
    except UnicodeDecodeError as error:
        raise PackagingError(f"invalid UTF-8 in {path}: byte {error.start}") from error
    try:
        return json.loads(text)
    except json.JSONDecodeError as error:
        raise PackagingError(
            f"invalid JSON in {path}: line {error.lineno}, column {error.colno}"
        ) from error


def _read_json_snapshot(path: Path) -> tuple[bytes, Any]:
    data = _read_bytes(path)
    return data, _decode_json(data, path)


def _require_object(value: Any, context: str) -> dict[str, Any]:
    if not isinstance(value, dict):
        raise PackagingError(f"{context} must be an object")
    return value


def _check_fields(
    value: dict[str, Any],
    allowed: frozenset[str],
    context: str,
) -> None:
    unknown = sorted(set(value) - allowed)
    missing = sorted(allowed - set(value))
    messages = []
    if unknown:
        messages.append(f"{context} has unknown fields: {', '.join(unknown)}")
    if missing:
        messages.append(f"{context} is missing fields: {', '.join(missing)}")
    if messages:
        raise PackagingError(messages)


def _require_nonempty_string(value: Any, context: str) -> str:
    if not isinstance(value, str) or not value:
        raise PackagingError(f"{context} must be a non-empty string")
    return value


def _require_sorted_unique_strings(
    value: Any,
    context: str,
    *,
    allowed: frozenset[str] | None = None,
) -> tuple[str, ...]:
    if not isinstance(value, list) or not value:
        raise PackagingError(f"{context} must be a non-empty array")
    if any(not isinstance(item, str) or not item for item in value):
        raise PackagingError(f"{context} must contain non-empty strings")
    if len(value) != len(set(value)):
        raise PackagingError(f"{context} contains duplicate values")
    if value != sorted(value):
        raise PackagingError(f"{context} must be sorted")
    if allowed is not None:
        unsupported = sorted(set(value) - allowed)
        if unsupported:
            raise PackagingError(
                f"{context} contains unsupported values: {', '.join(unsupported)}"
            )
    return tuple(value)


def _parse_target(value: Any, index: int) -> Target:
    context = f"targets[{index}]"
    record = _require_object(value, context)
    _check_fields(record, TARGET_FIELDS, context)

    target_id = _require_nonempty_string(record["id"], f"{context}.id")
    if TARGET_ID_PATTERN.fullmatch(target_id) is None:
        raise PackagingError(f"{context}.id has an invalid target identifier")

    os_record = _require_object(record["os"], f"{context}.os")
    _check_fields(os_record, OS_FIELDS, f"{context}.os")
    os_id = _require_nonempty_string(os_record["id"], f"{context}.os.id")
    os_family = _require_nonempty_string(
        os_record["family"], f"{context}.os.family"
    )
    version_id = _require_nonempty_string(
        os_record["version_id"], f"{context}.os.version_id"
    )

    architecture = _require_nonempty_string(
        record["architecture"], f"{context}.architecture"
    )
    if architecture not in ALLOWED_ARCHITECTURES:
        raise PackagingError(
            f"{context}.architecture is unsupported: {architecture}"
        )

    artifacts = _require_sorted_unique_strings(
        record["artifacts"],
        f"{context}.artifacts",
        allowed=ALLOWED_ARTIFACTS,
    )
    if "img.xz" in artifacts and not (
        target_id == "rpi-debian13-arm64"
        and architecture == "arm64"
        and os_family == "debian"
        and version_id == "13"
    ):
        raise PackagingError(
            f"{context}.artifacts may include img.xz only for the Raspberry Pi target"
        )

    hardware_acceptance = _require_sorted_unique_strings(
        record["hardware_acceptance"],
        f"{context}.hardware_acceptance",
    )
    expected = EXACT_TARGET_DEFINITIONS.get(target_id)
    if expected is None:
        raise PackagingError(f"{context}.id names unsupported target: {target_id}")
    observed = (
        os_id,
        os_family,
        version_id,
        architecture,
        artifacts,
        hardware_acceptance,
    )
    if observed != expected:
        raise PackagingError(
            f"{context} does not match the exact definition for {target_id}"
        )
    return Target(
        id=target_id,
        os_id=os_id,
        os_family=os_family,
        version_id=version_id,
        architecture=architecture,
        artifacts=artifacts,
        hardware_acceptance=hardware_acceptance,
    )


def _canonical_json(value: Any) -> str:
    return json.dumps(value, indent=2, sort_keys=True) + "\n"


def _validate_https_url(value: Any, context: str) -> str:
    url = _require_nonempty_string(value, context)
    try:
        parsed = urllib.parse.urlsplit(url)
    except ValueError as error:
        raise PackagingError(f"{context} has an invalid URL: {error}") from error
    if parsed.scheme != "https" or not parsed.hostname:
        raise PackagingError(f"{context} must use HTTPS with a host")
    if parsed.username is not None or parsed.password is not None:
        raise PackagingError(f"{context} must not contain credentials")
    if parsed.fragment:
        raise PackagingError(f"{context} must not contain a fragment")
    return url


def _require_sorted_package_names(value: Any, context: str, *, allow_empty: bool) -> list[str]:
    if not isinstance(value, list) or (not value and not allow_empty):
        qualifier = "an array" if allow_empty else "a non-empty array"
        raise PackagingError(f"{context} must be {qualifier}")
    if any(not isinstance(item, str) or DEBIAN_PACKAGE_ID_PATTERN.fullmatch(item) is None for item in value):
        raise PackagingError(f"{context} contains an invalid Debian package name")
    if value != sorted(set(value)):
        raise PackagingError(f"{context} must be sorted and unique")
    return list(value)


def _parse_apt_policy(value: Any, context: str) -> dict[str, Any]:
    policy = _require_object(value, context)
    _check_fields(policy, APT_POLICY_FIELDS, context)
    component = _require_nonempty_string(policy["component"], f"{context}.component")
    suite = _require_nonempty_string(policy["suite"], f"{context}.suite")
    if INPUT_NAME_PATTERN.fullmatch(component) is None or INPUT_NAME_PATTERN.fullmatch(suite) is None:
        raise PackagingError(f"{context} suite/component metadata is unsafe")
    keyring_sha256 = _require_nonempty_string(policy["keyring_sha256"], f"{context}.keyring_sha256")
    if SHA256_PATTERN.fullmatch(keyring_sha256) is None:
        raise PackagingError(f"{context}.keyring_sha256 must be 64 lowercase hex characters")
    fingerprints = policy["signing_fingerprints"]
    if not isinstance(fingerprints, list) or not fingerprints:
        raise PackagingError(f"{context}.signing_fingerprints must be a non-empty array")
    normalized_fingerprints = [str(value).upper() for value in fingerprints]
    if (
        any(SIGNING_FINGERPRINT_PATTERN.fullmatch(value) is None for value in normalized_fingerprints)
        or normalized_fingerprints != sorted(set(normalized_fingerprints))
    ):
        raise PackagingError(f"{context}.signing_fingerprints must be sorted unique full fingerprints")
    return {
        "base_packages": _require_sorted_package_names(
            policy["base_packages"], f"{context}.base_packages", allow_empty=True,
        ),
        "component": component,
        "keyring_sha256": keyring_sha256,
        "repository": _validate_https_url(policy["repository"], f"{context}.repository"),
        "roots": _require_sorted_package_names(policy["roots"], f"{context}.roots", allow_empty=False),
        "signing_fingerprints": normalized_fingerprints,
        "suite": suite,
    }


def _parse_locked_apt(value: Any, context: str, architecture: str) -> dict[str, Any]:
    apt = _require_object(value, context)
    _check_fields(apt, APT_LOCK_FIELDS, context)
    policy = _parse_apt_policy(
        {key: apt[key] for key in APT_POLICY_FIELDS}, context,
    )
    packages_value = apt["packages"]
    if not isinstance(packages_value, list) or not packages_value:
        raise PackagingError(f"{context}.packages must be a non-empty array")
    packages: list[dict[str, Any]] = []
    for index, raw in enumerate(packages_value):
        package_context = f"{context}.packages[{index}]"
        package = _require_object(raw, package_context)
        _check_fields(package, APT_PACKAGE_FIELDS, package_context)
        name = _require_nonempty_string(package["package"], f"{package_context}.package")
        if DEBIAN_PACKAGE_ID_PATTERN.fullmatch(name) is None:
            raise PackagingError(f"{package_context}.package is invalid")
        package_architecture = _require_nonempty_string(
            package["architecture"], f"{package_context}.architecture",
        )
        if package_architecture not in {architecture, "all"}:
            raise PackagingError(f"{package_context}.architecture does not match the target")
        filename = _require_nonempty_string(package["filename"], f"{package_context}.filename")
        if DEBIAN_FILENAME_PATTERN.fullmatch(filename) is None or ".." in PurePosixPath(filename).parts:
            raise PackagingError(f"{package_context}.filename is unsafe")
        digest = _require_nonempty_string(package["sha256"], f"{package_context}.sha256")
        if SHA256_PATTERN.fullmatch(digest) is None:
            raise PackagingError(f"{package_context}.sha256 is invalid")
        size = package["size"]
        if type(size) is not int or size <= 0:
            raise PackagingError(f"{package_context}.size must be a positive integer")
        signed_by = str(package["signed_by"]).upper()
        if signed_by not in policy["signing_fingerprints"]:
            raise PackagingError(f"{package_context}.signed_by is not an admitted fingerprint")
        packages.append(
            {
                "architecture": package_architecture,
                "depends": str(package["depends"]),
                "filename": filename,
                "package": name,
                "pre_depends": str(package["pre_depends"]),
                "provides": str(package["provides"]),
                "sha256": digest,
                "signed_by": signed_by,
                "size": size,
                "version": _require_nonempty_string(package["version"], f"{package_context}.version"),
            }
        )
    identities = [(item["package"], item["version"], item["architecture"]) for item in packages]
    if identities != sorted(identities) or len(identities) != len(set(identities)):
        raise PackagingError(f"{context}.packages must be sorted and unique")
    return {**policy, "packages": packages}


def _parse_locked_input(
    value: Any,
    index: int,
    registry: TargetRegistry,
) -> LockedInput:
    context = f"inputs[{index}]"
    record = _require_object(value, context)
    _check_fields(record, INPUT_FIELDS, context)

    name = _require_nonempty_string(record["name"], f"{context}.name")
    if INPUT_NAME_PATTERN.fullmatch(name) is None:
        raise PackagingError(f"{context}.name has unsafe input metadata")
    scope = _require_nonempty_string(record["scope"], f"{context}.scope")
    known_scopes = {"shared", *(target.id for target in registry.targets)}
    if scope not in known_scopes:
        raise PackagingError(f"{context}.scope names unknown target: {scope}")
    url = _validate_https_url(record["url"], f"{context}.url")
    artifact_type = _require_nonempty_string(record["artifact_type"], f"{context}.artifact_type")
    expected_artifact_type = INPUT_ARTIFACT_TYPES.get(name)
    if expected_artifact_type is not None and artifact_type != expected_artifact_type:
        raise PackagingError(f"{context}.artifact_type does not match input class {name}")
    architecture = _require_nonempty_string(record["architecture"], f"{context}.architecture")
    expected_architecture = "all" if scope == "shared" else registry.require(scope).architecture
    if architecture != expected_architecture:
        raise PackagingError(f"{context}.architecture must be {expected_architecture}")
    version = _require_nonempty_string(record["version"], f"{context}.version")
    license_name = _require_nonempty_string(
        record["license"], f"{context}.license"
    )
    provenance = _validate_https_url(record["provenance"], f"{context}.provenance")
    redistribution = _require_nonempty_string(record["redistribution"], f"{context}.redistribution")
    if redistribution != "permitted":
        raise PackagingError(f"{context}.redistribution must be explicitly permitted")
    sha256 = _require_nonempty_string(record["sha256"], f"{context}.sha256")
    if SHA256_PATTERN.fullmatch(sha256) is None:
        raise PackagingError(f"{context}.sha256 must be 64 lowercase hex characters")
    size = record["size"]
    if type(size) is not int or size <= 0:
        raise PackagingError(f"{context}.size must be a positive integer")
    apt_value = record["apt"]
    if artifact_type == "debian-pool-tar":
        apt = _parse_locked_apt(apt_value, f"{context}.apt", architecture)
    elif apt_value is not None:
        raise PackagingError(f"{context}.apt is allowed only for a Debian pool")
    else:
        apt = None
    return LockedInput(
        name=name,
        scope=scope,
        architecture=architecture,
        artifact_type=artifact_type,
        url=url,
        version=version,
        license=license_name,
        provenance=provenance,
        redistribution=redistribution,
        sha256=sha256,
        size=size,
        apt=apt,
    )


def _cache_object_parts(sha256: str) -> tuple[str, ...]:
    return ("objects", "sha256", sha256[:2], sha256)


def _is_link_or_reparse(metadata: os.stat_result) -> bool:
    if stat.S_ISLNK(metadata.st_mode):
        return True
    attributes = getattr(metadata, "st_file_attributes", 0)
    reparse_flag = getattr(stat, "FILE_ATTRIBUTE_REPARSE_POINT", 0)
    return bool(attributes & reparse_flag)


def _open_cache_object_posix(
    cache_root: Path,
    item: LockedInput,
) -> BinaryIO:
    directory_fds: list[int] = []
    file_fd: int | None = None
    directory_flags = os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW
    file_flags = os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK
    try:
        current_fd = os.open(cache_root, directory_flags)
        directory_fds.append(current_fd)
        parts = _cache_object_parts(item.sha256)
        for component in parts[:-1]:
            current_fd = os.open(
                component,
                directory_flags,
                dir_fd=current_fd,
            )
            directory_fds.append(current_fd)
        file_fd = os.open(parts[-1], file_flags, dir_fd=current_fd)
        metadata = os.fstat(file_fd)
        if not stat.S_ISREG(metadata.st_mode):
            raise CacheObjectError(
                "nonregular",
                f"{item.name}: cache object must be a regular file inside cache",
            )
        stream = os.fdopen(file_fd, "rb")
        file_fd = None
        return stream
    except FileNotFoundError as error:
        raise CacheObjectError(
            "missing", f"{item.name}: cache object is missing"
        ) from error
    except CacheObjectError:
        raise
    except OSError as error:
        raise CacheObjectError(
            "nonregular",
            f"{item.name}: cache object must be a regular file inside cache: {error}",
        ) from error
    finally:
        if file_fd is not None:
            os.close(file_fd)
        for descriptor in reversed(directory_fds):
            os.close(descriptor)


def _open_cache_object_portable(
    cache_root: Path,
    item: LockedInput,
) -> BinaryIO:
    current = cache_root
    try:
        root_metadata = os.lstat(current)
        if _is_link_or_reparse(root_metadata) or not stat.S_ISDIR(
            root_metadata.st_mode
        ):
            raise CacheObjectError(
                "nonregular",
                f"{item.name}: cache object must be a regular file inside cache",
            )
        for component in _cache_object_parts(item.sha256)[:-1]:
            current /= component
            metadata = os.lstat(current)
            if _is_link_or_reparse(metadata) or not stat.S_ISDIR(metadata.st_mode):
                raise CacheObjectError(
                    "nonregular",
                    f"{item.name}: cache object must be a regular file inside cache",
                )
        path = current / item.sha256
        metadata = os.lstat(path)
        if _is_link_or_reparse(metadata) or not stat.S_ISREG(metadata.st_mode):
            raise CacheObjectError(
                "nonregular",
                f"{item.name}: cache object must be a regular file inside cache",
            )
        flags = os.O_RDONLY | getattr(os, "O_BINARY", 0)
        file_fd = os.open(path, flags)
        opened_metadata = os.fstat(file_fd)
        if not os.path.samestat(metadata, opened_metadata):
            os.close(file_fd)
            raise CacheObjectError(
                "nonregular",
                f"{item.name}: cache object changed while it was opened",
            )
        return os.fdopen(file_fd, "rb")
    except FileNotFoundError as error:
        raise CacheObjectError(
            "missing", f"{item.name}: cache object is missing"
        ) from error
    except CacheObjectError:
        raise
    except OSError as error:
        raise CacheObjectError(
            "nonregular",
            f"{item.name}: cache object must be a regular file inside cache: {error}",
        ) from error


def _open_cache_object(cache_root: Path, item: LockedInput) -> BinaryIO:
    if (
        os.name == "posix"
        and hasattr(os, "O_DIRECTORY")
        and hasattr(os, "O_NOFOLLOW")
        and os.open in os.supports_dir_fd
    ):
        return _open_cache_object_posix(cache_root, item)
    return _open_cache_object_portable(cache_root, item)


def _hash_cache_object(
    cache_root: Path,
    item: LockedInput,
) -> tuple[int, str]:
    digest = hashlib.sha256()
    size = 0
    try:
        with _open_cache_object(cache_root, item) as stream:
            while chunk := stream.read(DOWNLOAD_CHUNK_SIZE):
                size += len(chunk)
                digest.update(chunk)
    except CacheObjectError:
        raise
    except OSError as error:
        raise CacheObjectError(
            "nonregular", f"{item.name}: cannot read cache object: {error}"
        ) from error
    return size, digest.hexdigest()


def _remove_cache_object(cache_root: Path, item: LockedInput) -> None:
    path = cache_object_path(cache_root, item.sha256)
    if (
        os.name == "posix"
        and hasattr(os, "O_DIRECTORY")
        and hasattr(os, "O_NOFOLLOW")
        and os.open in os.supports_dir_fd
        and os.unlink in os.supports_dir_fd
    ):
        directory_fds: list[int] = []
        try:
            current_fd = os.open(
                cache_root,
                os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW,
            )
            directory_fds.append(current_fd)
            parts = _cache_object_parts(item.sha256)
            for component in parts[:-1]:
                current_fd = os.open(
                    component,
                    os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW,
                    dir_fd=current_fd,
                )
                directory_fds.append(current_fd)
            metadata = os.stat(
                parts[-1],
                dir_fd=current_fd,
                follow_symlinks=False,
            )
            if not stat.S_ISREG(metadata.st_mode):
                raise CacheObjectError(
                    "nonregular",
                    f"{item.name}: cache object must be a regular file inside cache",
                )
            os.unlink(parts[-1], dir_fd=current_fd)
            return
        except FileNotFoundError:
            return
        except CacheObjectError:
            raise
        except OSError as error:
            raise PackagingError(
                f"{item.name}: cannot remove corrupt cache object: {error}"
            ) from error
        finally:
            for descriptor in reversed(directory_fds):
                os.close(descriptor)

    current = cache_root
    try:
        root_metadata = os.lstat(current)
        if _is_link_or_reparse(root_metadata) or not stat.S_ISDIR(
            root_metadata.st_mode
        ):
            raise CacheObjectError(
                "nonregular",
                f"{item.name}: cache object must be a regular file inside cache",
            )
        for component in _cache_object_parts(item.sha256)[:-1]:
            current /= component
            directory_metadata = os.lstat(current)
            if _is_link_or_reparse(directory_metadata) or not stat.S_ISDIR(
                directory_metadata.st_mode
            ):
                raise CacheObjectError(
                    "nonregular",
                    f"{item.name}: cache object must be a regular file inside cache",
                )
        metadata = os.lstat(path)
        if _is_link_or_reparse(metadata) or not stat.S_ISREG(metadata.st_mode):
            raise CacheObjectError(
                "nonregular",
                f"{item.name}: cache object must be a regular file inside cache",
            )
        path.unlink()
    except FileNotFoundError:
        return
    except CacheObjectError:
        raise
    except OSError as error:
        raise PackagingError(
            f"{item.name}: cannot remove corrupt cache object: {error}"
        ) from error


def _verify_object(
    cache_root: Path,
    item: LockedInput,
) -> VerifiedInput:
    path = cache_object_path(cache_root, item.sha256)
    actual_size, actual_digest = _hash_cache_object(cache_root, item)
    messages = []
    if actual_size != item.size:
        messages.append(
            f"{item.name}: cache object size mismatch: "
            f"expected {item.size}, got {actual_size}"
        )
    if actual_digest != item.sha256:
        messages.append(
            f"{item.name}: cache object SHA-256 mismatch: "
            f"expected {item.sha256}, got {actual_digest}"
        )
    if messages:
        raise CacheObjectError("content", messages)
    if item.name in INPUT_ARTIFACT_TYPES:
        try:
            _validate_resolved_artifact(path, item)
        except PackagingError as error:
            raise CacheObjectError("content", error.messages) from error
    return VerifiedInput(
        name=item.name,
        scope=item.scope,
        architecture=item.architecture,
        artifact_type=item.artifact_type,
        version=item.version,
        license=item.license,
        provenance=item.provenance,
        redistribution=item.redistribution,
        sha256=item.sha256,
        size=item.size,
        cache_object=path.relative_to(cache_root).as_posix(),
        apt=item.apt,
    )


def _download_input(
    item: LockedInput,
    stream: BinaryIO,
    destination: Path,
) -> None:
    digest = hashlib.sha256()
    size = 0
    try:
        with destination.open("wb") as output:
            while chunk := stream.read(DOWNLOAD_CHUNK_SIZE):
                size += len(chunk)
                if size > item.size:
                    raise PackagingError(
                        f"{item.name}: download exceeded declared size {item.size}"
                    )
                digest.update(chunk)
                output.write(chunk)
            output.flush()
            os.fsync(output.fileno())
    except OSError as error:
        raise PackagingError(f"{item.name}: download failed: {error}") from error
    if size != item.size:
        raise PackagingError(
            f"{item.name}: download size mismatch: expected {item.size}, got {size}"
        )
    actual_digest = digest.hexdigest()
    if actual_digest != item.sha256:
        raise PackagingError(
            f"{item.name}: download SHA-256 mismatch: "
            f"expected {item.sha256}, got {actual_digest}"
        )


def _deb_control_from_bytes(contents: bytes, context: str) -> dict[str, str]:
    if not contents.startswith(b"!<arch>\n"):
        raise PackagingError(f"{context}: Debian package is not an ar archive")
    offset = 8
    members: dict[str, bytes] = {}
    while offset < len(contents):
        header = contents[offset : offset + 60]
        if len(header) != 60 or header[58:] != b"`\n":
            raise PackagingError(f"{context}: Debian package has an invalid ar header")
        try:
            name = header[:16].decode("ascii").rstrip().rstrip("/")
            size = int(header[48:58].decode("ascii").strip())
        except (UnicodeDecodeError, ValueError) as error:
            raise PackagingError(f"{context}: Debian package metadata is invalid") from error
        offset += 60
        if size < 0 or offset + size > len(contents) or name in members:
            raise PackagingError(f"{context}: Debian package member is invalid")
        members[name] = contents[offset : offset + size]
        offset += size + size % 2
    control_name = next((name for name in members if name.startswith("control.tar.")), "")
    if members.get("debian-binary") != b"2.0\n" or not control_name:
        raise PackagingError(f"{context}: Debian package control archive is missing")
    try:
        with tarfile.open(fileobj=io.BytesIO(members[control_name]), mode="r:*") as archive:
            control_member = next(
                (member for member in archive.getmembers() if member.name in {"control", "./control"}),
                None,
            )
            stream = archive.extractfile(control_member) if control_member is not None else None
            if stream is None:
                raise PackagingError(f"{context}: Debian control file is missing")
            text = stream.read().decode("utf-8")
    except (tarfile.TarError, UnicodeDecodeError) as error:
        raise PackagingError(f"{context}: Debian control archive is invalid") from error
    fields: dict[str, str] = {}
    for line in text.splitlines():
        if not line or line.startswith(" "):
            continue
        key, separator, value = line.partition(":")
        if not separator:
            raise PackagingError(f"{context}: Debian control file is invalid")
        fields[key] = value.strip()
    return fields


def _validated_tar_members(path: Path, context: str) -> dict[str, tarfile.TarInfo]:
    try:
        with tarfile.open(path, mode="r:*") as archive:
            members = archive.getmembers()
    except (OSError, tarfile.TarError) as error:
        raise PackagingError(f"{context}: expected a readable tar archive") from error
    if not members:
        raise PackagingError(f"{context}: archive must not be empty")
    observed: dict[str, tarfile.TarInfo] = {}
    for member in members:
        normalized = member.name.removeprefix("./")
        parts = tuple(part for part in normalized.split("/") if part)
        if not parts or normalized.startswith("/") or ".." in parts:
            raise PackagingError(f"{context}: archive contains an unsafe path")
        if not (member.isfile() or member.isdir()) or normalized in observed:
            raise PackagingError(f"{context}: archive contains a link, special file, or duplicate")
        observed[normalized] = member
    return observed


def _clear_signed_payload(contents: bytes, context: str) -> bytes:
    try:
        text = contents.decode("utf-8")
    except UnicodeDecodeError as error:
        raise PackagingError(f"{context}: InRelease is not UTF-8") from error
    prefix = "-----BEGIN PGP SIGNED MESSAGE-----\n"
    marker = "\n-----BEGIN PGP SIGNATURE-----\n"
    if not text.startswith(prefix) or marker not in text:
        raise PackagingError(f"{context}: InRelease is not a clear-signed document")
    headers_and_payload = text[len(prefix) :].split(marker, 1)[0]
    separator = headers_and_payload.find("\n\n")
    if separator < 0:
        raise PackagingError(f"{context}: InRelease clear-sign headers are malformed")
    payload = headers_and_payload[separator + 2 :]
    unescaped = "\n".join(line[2:] if line.startswith("- ") else line for line in payload.split("\n"))
    return unescaped.encode("utf-8")


def _parse_deb822_paragraphs(contents: bytes, context: str) -> list[dict[str, str]]:
    try:
        lines = contents.decode("utf-8").splitlines()
    except UnicodeDecodeError as error:
        raise PackagingError(f"{context}: metadata is not UTF-8") from error
    paragraphs: list[dict[str, str]] = []
    current: dict[str, str] = {}
    last_key = ""
    for line in [*lines, ""]:
        if not line:
            if current:
                paragraphs.append(current)
                current = {}
                last_key = ""
            continue
        if line[0].isspace():
            if not last_key:
                raise PackagingError(f"{context}: orphan continuation line")
            current[last_key] += "\n" + line.strip()
            continue
        key, separator, value = line.partition(":")
        if not separator or not key or key in current:
            raise PackagingError(f"{context}: malformed or duplicate field")
        current[key] = value.strip()
        last_key = key
    return paragraphs


def _default_apt_signature_verifier(
    keyring: Path, inrelease: Path, admitted_fingerprints: list[str],
) -> str:
    executable = shutil.which("gpgv")
    if not executable:
        raise PackagingError("Debian pool signature verification requires gpgv")
    try:
        result = subprocess.run(
            [executable, "--status-fd", "1", "--keyring", str(keyring), str(inrelease)],
            check=False,
            capture_output=True,
            timeout=60,
        )
    except (OSError, subprocess.TimeoutExpired) as error:
        raise PackagingError(f"Debian pool signature verification failed to execute: {error}") from error
    if result.returncode != 0:
        raise PackagingError("Debian pool InRelease signature is invalid")
    status = result.stdout.decode("utf-8", errors="replace")
    observed = [
        line.split()[2].upper()
        for line in status.splitlines()
        if line.startswith("[GNUPG:] VALIDSIG ") and len(line.split()) >= 3
    ]
    matched = next((value for value in observed if value in admitted_fingerprints), "")
    if not matched:
        raise PackagingError("Debian pool signature does not match an admitted full fingerprint")
    return matched


DEPENDENCY_ALTERNATIVE_PATTERN = re.compile(
    r"^([a-z0-9][a-z0-9+.-]*)(?::(any|native|[a-z0-9-]+))?"
    r"(?:\s*\((<<|<=|=|>=|>>)\s*([^)\s]+)\))?"
    r"(?:\s*\[([^\]]+)\])?\s*$"
)


def _debian_version_parts(version: str) -> tuple[int, str, str]:
    epoch_text, separator, remainder = version.partition(":")
    if separator:
        if not epoch_text.isdigit():
            raise PackagingError(f"Debian version has an invalid epoch: {version}")
        epoch = int(epoch_text)
    else:
        epoch, remainder = 0, epoch_text
    upstream, separator, revision = remainder.rpartition("-")
    if not separator:
        upstream, revision = remainder, "0"
    if not upstream:
        raise PackagingError(f"Debian version is invalid: {version}")
    return epoch, upstream, revision


def _debian_non_digit_order(character: str) -> int:
    if character == "~":
        return -1
    if character == "":
        return 0
    if character.isalpha():
        return ord(character)
    return ord(character) + 256


def _debian_part_compare(left: str, right: str) -> int:
    left_index = right_index = 0
    while left_index < len(left) or right_index < len(right):
        while (
            (left_index < len(left) and not left[left_index].isdigit())
            or (right_index < len(right) and not right[right_index].isdigit())
        ):
            left_character = left[left_index] if left_index < len(left) and not left[left_index].isdigit() else ""
            right_character = right[right_index] if right_index < len(right) and not right[right_index].isdigit() else ""
            difference = _debian_non_digit_order(left_character) - _debian_non_digit_order(right_character)
            if difference:
                return -1 if difference < 0 else 1
            if left_character:
                left_index += 1
            if right_character:
                right_index += 1
        left_zero = left_index
        while left_zero < len(left) and left[left_zero] == "0":
            left_zero += 1
        right_zero = right_index
        while right_zero < len(right) and right[right_zero] == "0":
            right_zero += 1
        left_end = left_zero
        while left_end < len(left) and left[left_end].isdigit():
            left_end += 1
        right_end = right_zero
        while right_end < len(right) and right[right_end].isdigit():
            right_end += 1
        left_digits, right_digits = left[left_zero:left_end], right[right_zero:right_end]
        if len(left_digits) != len(right_digits):
            return -1 if len(left_digits) < len(right_digits) else 1
        if left_digits != right_digits:
            return -1 if left_digits < right_digits else 1
        left_index, right_index = left_end, right_end
    return 0


def _debian_version_compare(left: str, right: str) -> int:
    left_epoch, left_upstream, left_revision = _debian_version_parts(left)
    right_epoch, right_upstream, right_revision = _debian_version_parts(right)
    if left_epoch != right_epoch:
        return -1 if left_epoch < right_epoch else 1
    upstream = _debian_part_compare(left_upstream, right_upstream)
    return upstream if upstream else _debian_part_compare(left_revision, right_revision)


def _version_satisfies(actual: str, relationship: str | None, required: str | None) -> bool:
    if relationship is None:
        return True
    assert required is not None
    compared = _debian_version_compare(actual, required)
    return {
        "<<": compared < 0,
        "<=": compared <= 0,
        "=": compared == 0,
        ">=": compared >= 0,
        ">>": compared > 0,
    }[relationship]


def _dependency_groups(expression: str, architecture: str) -> list[list[tuple[str, str | None, str | None]]]:
    groups: list[list[tuple[str, str | None, str | None]]] = []
    for clause in filter(None, (value.strip() for value in expression.split(","))):
        alternatives: list[tuple[str, str | None, str | None]] = []
        for raw in clause.split("|"):
            if re.search(r"\s<[^>]+>\s*$", raw):
                raise PackagingError("Debian build-profile qualifiers are not valid runtime closure input")
            matched = DEPENDENCY_ALTERNATIVE_PATTERN.fullmatch(raw.strip())
            if matched is None:
                raise PackagingError(f"Debian dependency expression is unsupported or unsafe: {raw.strip()}")
            name, qualifier, relationship, version, restrictions = matched.groups()
            if qualifier not in {None, "any", "native", architecture}:
                continue
            if restrictions:
                tokens = restrictions.split()
                positives = [token for token in tokens if not token.startswith("!")]
                negatives = [token[1:] for token in tokens if token.startswith("!")]
                if (positives and architecture not in positives) or architecture in negatives:
                    continue
            alternatives.append((name, relationship, version))
        if alternatives:
            groups.append(alternatives)
    return groups


def _provided_versions(record: dict[str, Any]) -> dict[str, str | None]:
    provided: dict[str, str | None] = {str(record["package"]): str(record["version"])}
    for raw in filter(None, (value.strip() for value in str(record.get("provides", "")).split(","))):
        matched = re.fullmatch(r"([a-z0-9][a-z0-9+.-]*)(?:\s*\(=\s*([^)\s]+)\))?", raw)
        if matched is None:
            raise PackagingError(f"Debian Provides expression is unsupported or unsafe: {raw}")
        provided[matched.group(1)] = matched.group(2)
    return provided


def _validate_apt_pool(
    path: Path,
    item: LockedInput,
    members: dict[str, tarfile.TarInfo],
    policy: dict[str, Any],
    signature_verifier: Callable[[Path, Path, list[str]], str],
) -> dict[str, Any]:
    required = {"apt/InRelease", "apt/Packages", "apt/archive-keyring.gpg"}
    if not required.issubset(members):
        raise PackagingError(f"{item.name}: Debian pool lacks signed repository metadata")
    with tarfile.open(path, mode="r:*") as archive:
        extracted: dict[str, bytes] = {}
        for name in members:
            if members[name].isfile():
                stream = archive.extractfile(members[name])
                if stream is None:
                    raise PackagingError(f"{item.name}: cannot read {name}")
                extracted[name] = stream.read()
    keyring_digest = hashlib.sha256(extracted["apt/archive-keyring.gpg"]).hexdigest()
    if keyring_digest != policy["keyring_sha256"]:
        raise PackagingError(f"{item.name}: archive keyring digest mismatch")
    with tempfile.TemporaryDirectory(prefix="nilo-apt-signature-") as temporary:
        temporary_root = Path(temporary)
        keyring = temporary_root / "archive-keyring.gpg"
        inrelease = temporary_root / "InRelease"
        keyring.write_bytes(extracted["apt/archive-keyring.gpg"])
        inrelease.write_bytes(extracted["apt/InRelease"])
        signed_by = str(
            signature_verifier(keyring, inrelease, list(policy["signing_fingerprints"]))
        ).upper()
    if signed_by not in policy["signing_fingerprints"]:
        raise PackagingError(f"{item.name}: signature verifier returned an untrusted fingerprint")

    release_paragraphs = _parse_deb822_paragraphs(
        _clear_signed_payload(extracted["apt/InRelease"], item.name), f"{item.name}/InRelease",
    )
    if len(release_paragraphs) != 1:
        raise PackagingError(f"{item.name}: InRelease must contain one metadata paragraph")
    release = release_paragraphs[0]
    if policy["suite"] not in {release.get("Suite"), release.get("Codename")}:
        raise PackagingError(f"{item.name}: signed suite/codename mismatch")
    if policy["component"] not in release.get("Components", "").split():
        raise PackagingError(f"{item.name}: signed component mismatch")
    if item.architecture not in release.get("Architectures", "").split():
        raise PackagingError(f"{item.name}: signed architecture mismatch")
    packages_digest = hashlib.sha256(extracted["apt/Packages"]).hexdigest()
    packages_size = len(extracted["apt/Packages"])
    checksum_lines = release.get("SHA256", "").splitlines()
    if not any(
        line.split() == [packages_digest, str(packages_size), "apt/Packages"]
        for line in checksum_lines
    ):
        raise PackagingError(f"{item.name}: signed InRelease does not bind apt/Packages")

    paragraphs = _parse_deb822_paragraphs(extracted["apt/Packages"], f"{item.name}/Packages")
    packages: list[dict[str, Any]] = []
    by_name: dict[str, dict[str, Any]] = {}
    filenames: set[str] = set()
    for index, paragraph in enumerate(paragraphs):
        context = f"{item.name}/Packages[{index}]"
        required_fields = {"Package", "Version", "Architecture", "Filename", "Size", "SHA256"}
        if not required_fields.issubset(paragraph):
            raise PackagingError(f"{context}: package provenance is incomplete")
        name = paragraph["Package"]
        filename = paragraph["Filename"]
        if DEBIAN_PACKAGE_ID_PATTERN.fullmatch(name) is None or DEBIAN_FILENAME_PATTERN.fullmatch(filename) is None:
            raise PackagingError(f"{context}: package identity or filename is unsafe")
        if filename in filenames or filename not in extracted or not filename.startswith("pool/"):
            raise PackagingError(f"{context}: package file is missing or duplicated")
        filenames.add(filename)
        contents = extracted[filename]
        try:
            expected_size = int(paragraph["Size"])
        except ValueError as error:
            raise PackagingError(f"{context}: package size is invalid") from error
        digest = hashlib.sha256(contents).hexdigest()
        if len(contents) != expected_size or digest != paragraph["SHA256"]:
            raise PackagingError(f"{context}: package bytes do not match signed metadata")
        control = _deb_control_from_bytes(contents, context)
        if any(
            control.get(key) != paragraph[field]
            for key, field in (("Package", "Package"), ("Version", "Version"), ("Architecture", "Architecture"))
        ):
            raise PackagingError(f"{context}: package control identity mismatches repository metadata")
        if any(
            control.get(field, "") != paragraph.get(field, "")
            for field in ("Depends", "Pre-Depends", "Provides")
        ):
            raise PackagingError(f"{context}: package control relationships mismatch repository metadata")
        if paragraph["Architecture"] not in {item.architecture, "all"}:
            raise PackagingError(f"{context}: architecture mismatch")
        record = {
            "architecture": paragraph["Architecture"],
            "depends": paragraph.get("Depends", ""),
            "filename": filename,
            "package": name,
            "pre_depends": paragraph.get("Pre-Depends", ""),
            "provides": paragraph.get("Provides", ""),
            "sha256": digest,
            "signed_by": signed_by,
            "size": len(contents),
            "version": paragraph["Version"],
        }
        if name in by_name:
            raise PackagingError(f"{context}: multiple versions of one package are not allowed")
        by_name[name] = record
        packages.append(record)
    actual_debs = {name for name in extracted if name.startswith("pool/") and name.endswith(".deb")}
    if actual_debs != filenames:
        raise PackagingError(f"{item.name}: pool contains a .deb absent from signed Packages metadata")

    base = set(policy["base_packages"])
    providers: dict[str, list[tuple[dict[str, Any], str | None]]] = {}
    for record in packages:
        for provided_name, provided_version in _provided_versions(record).items():
            providers.setdefault(provided_name, []).append((record, provided_version))
    for candidates in providers.values():
        candidates.sort(key=lambda value: (str(value[0]["package"]), str(value[0]["version"])))
    reachable: set[str] = set()
    pending = list(policy["roots"])
    while pending:
        name = pending.pop()
        if name in reachable or name in base:
            continue
        record = by_name.get(name)
        if record is None:
            raise PackagingError(f"{item.name}: dependency closure is missing {name}")
        reachable.add(name)
        for field in ("pre_depends", "depends"):
            for alternatives in _dependency_groups(str(record[field]), item.architecture):
                selected: dict[str, Any] | None = None
                for dependency_name, relationship, required_version in alternatives:
                    for candidate, provided_version in providers.get(dependency_name, []):
                        if relationship is None or (
                            provided_version is not None
                            and _version_satisfies(provided_version, relationship, required_version)
                        ):
                            selected = candidate
                            break
                    if selected is not None:
                        break
                    if dependency_name in base and relationship is None:
                        selected = {"package": dependency_name}
                        break
                if selected is None:
                    rendered = " | ".join(
                        name + (f" ({relationship} {version})" if relationship else "")
                        for name, relationship, version in alternatives
                    )
                    names_present = any(name in providers or name in base for name, _, _ in alternatives)
                    reason = (
                        "dependency version relationship is unsatisfied"
                        if names_present else "dependency closure is missing"
                    )
                    raise PackagingError(f"{item.name}: {reason}: {rendered}")
                if selected["package"] not in base:
                    pending.append(str(selected["package"]))
    if reachable != set(by_name):
        raise PackagingError(f"{item.name}: pool contains packages outside the declared recursive closure")
    packages.sort(key=lambda value: (value["package"], value["version"], value["architecture"]))
    completed = {**policy, "packages": packages}
    if "packages" in policy and policy["packages"] != packages:
        raise PackagingError(f"{item.name}: locked per-package provenance does not match the pool")
    return completed


def _validate_resolved_artifact(
    path: Path,
    item: LockedInput,
    *,
    apt_policy: dict[str, Any] | None = None,
    signature_verifier: Callable[[Path, Path, list[str]], str] = _default_apt_signature_verifier,
) -> dict[str, Any] | None:
    """Validate that a connected resolution produced its reviewed artifact class."""
    if item.artifact_type == "raspberry-pi-image-xz":
        try:
            with lzma.open(path, "rb") as image:
                header = image.read(512)
        except (OSError, lzma.LZMAError) as error:
            raise PackagingError(f"{item.name}: base image is not a valid xz stream") from error
        if len(header) != 512 or header[510:512] != b"\x55\xaa":
            raise PackagingError(f"{item.name}: base image lacks an MBR signature")
        return None

    members = _validated_tar_members(path, item.name)
    if item.artifact_type == "debian-pool-tar":
        effective_policy = apt_policy if apt_policy is not None else item.apt
        if effective_policy is None:
            raise PackagingError(f"{item.name}: Debian pool lacks a signed repository policy")
        return _validate_apt_pool(path, item, members, effective_policy, signature_verifier)
    elif item.artifact_type == "sigrok-runtime-tar":
        required = {"runtime-manifest.json", "THIRD_PARTY_NOTICES.txt", "bin/python3", "bin/sigrok-cli"}
        if not required.issubset(members):
            raise PackagingError(f"{item.name}: Sigrok runtime inventory is incomplete")
    elif item.artifact_type == "camera-build-input-tar":
        required = {
            "THIRD_PARTY_NOTICES.txt",
            "dependencies/nilo_integration/__init__.py",
            "dependencies/pydantic/__init__.py",
            "wheelhouse/pyrav4l2-1.0-py3-none-any.whl",
        }
        if not required.issubset(members) or not any(
            name.startswith("wheelhouse/pillow-11.3.0-") and name.endswith(".whl")
            for name in members
        ):
            raise PackagingError(f"{item.name}: Camera build input inventory is incomplete")
    elif item.artifact_type == "source-offers-tar":
        if not any(name.endswith(("SOURCE_OFFER.txt", ".tar.gz", ".tar.xz", ".zip")) for name in members):
            raise PackagingError(f"{item.name}: source-offer archive contains no source offer")
    return None


def _has_ipv4_default_route(path: Path) -> bool:
    try:
        lines = path.read_text(encoding="ascii").splitlines()
    except OSError as error:
        raise PackagingError(f"cannot inspect IPv4 routes at {path}: {error}") from error
    for line in lines[1:]:
        fields = line.split()
        if len(fields) >= 8 and fields[1] == "00000000" and fields[7] == "00000000":
            return True
    return False


def _has_ipv6_default_route(path: Path) -> bool:
    try:
        lines = path.read_text(encoding="ascii").splitlines()
    except OSError as error:
        raise PackagingError(f"cannot inspect IPv6 routes at {path}: {error}") from error
    for line in lines:
        fields = line.split()
        if len(fields) < 10 or fields[0] != "0" * 32 or fields[1] != "00":
            continue
        try:
            flags = int(fields[8], 16)
        except ValueError as error:
            raise PackagingError(
                f"invalid IPv6 route flags at {path}: {fields[8]}"
            ) from error
        if flags & IPV6_ROUTE_REJECT_FLAG == 0:
            return True
    return False


def _write_json_atomically(path: Path, value: Any) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    temporary_path: Path | None = None
    try:
        handle, temporary_name = tempfile.mkstemp(
            prefix=f".{path.name}.", suffix=".tmp", dir=path.parent
        )
        temporary_path = Path(temporary_name)
        with os.fdopen(handle, "w", encoding="utf-8", newline="\n") as output:
            output.write(_canonical_json(value))
            output.flush()
            os.fsync(output.fileno())
        os.replace(temporary_path, path)
        temporary_path = None
    except OSError as error:
        raise PackagingError(f"cannot write receipt {path}: {error}") from error
    finally:
        if temporary_path is not None:
            temporary_path.unlink(missing_ok=True)


def _parse_target_registry(document_value: Any) -> TargetRegistry:
    document = _require_object(document_value, "target registry")
    _check_fields(document, frozenset({"schema", "targets"}), "target registry")
    if type(document["schema"]) is not int or document["schema"] != 1:
        raise PackagingError("target registry schema must be integer 1")
    if not isinstance(document["targets"], list) or not document["targets"]:
        raise PackagingError("target registry targets must be a non-empty array")

    targets = tuple(
        _parse_target(value, index)
        for index, value in enumerate(document["targets"])
    )
    identifiers = [target.id for target in targets]
    if len(identifiers) != len(set(identifiers)):
        raise PackagingError("target registry contains duplicate target IDs")
    if identifiers != sorted(identifiers):
        raise PackagingError("target registry targets must be sorted by id")
    if identifiers != sorted(EXACT_TARGET_DEFINITIONS):
        raise PackagingError(
            "target registry must declare exactly the supported delivery targets"
        )
    return TargetRegistry(targets=targets)


def _load_targets_snapshot(path: Path) -> tuple[bytes, TargetRegistry]:
    data, document = _read_json_snapshot(path)
    return data, _parse_target_registry(document)


def _parse_input_lock(
    document_value: Any,
    registry: TargetRegistry,
    *,
    require_complete: bool,
) -> InputLock:
    document = _require_object(document_value, "input lock")
    _check_fields(document, LOCK_FIELDS, "input lock")
    if type(document["schema"]) is not int or document["schema"] != 1:
        raise PackagingError("input lock schema must be integer 1")
    if type(document["complete"]) is not bool:
        raise PackagingError("input lock complete must be a Boolean")
    complete = document["complete"]

    reasons_value = document["incomplete_reasons"]
    if not isinstance(reasons_value, list) or any(
        not isinstance(reason, str) or not reason for reason in reasons_value
    ):
        raise PackagingError(
            "input lock incomplete_reasons must contain non-empty strings"
        )
    if len(reasons_value) != len(set(reasons_value)):
        raise PackagingError("input lock incomplete_reasons contains duplicates")
    if reasons_value != sorted(reasons_value):
        raise PackagingError("input lock incomplete_reasons must be sorted")
    reasons = tuple(reasons_value)
    if complete and reasons:
        raise PackagingError("complete input lock must not have incomplete reasons")
    if not complete and not reasons:
        raise PackagingError("incomplete input lock must name incomplete reasons")
    if require_complete and not complete:
        raise PackagingError("input lock is incomplete: " + "; ".join(reasons))

    inputs_value = document["inputs"]
    if not isinstance(inputs_value, list):
        raise PackagingError("input lock inputs must be an array")
    inputs = tuple(
        _parse_locked_input(value, index, registry)
        for index, value in enumerate(inputs_value)
    )
    identities = [(item.scope, item.name) for item in inputs]
    if len(identities) != len(set(identities)):
        raise PackagingError("input lock contains duplicate scoped input names")
    if identities != sorted(identities):
        raise PackagingError("input lock inputs must be sorted by scope then name")
    if complete and not inputs:
        raise PackagingError("complete input lock must declare at least one input")
    return InputLock(
        complete=complete,
        incomplete_reasons=reasons,
        inputs=inputs,
    )


def _load_lock_snapshot(
    path: Path,
    registry: TargetRegistry,
    *,
    require_complete: bool,
) -> tuple[bytes, InputLock]:
    data, document = _read_json_snapshot(path)
    return data, _parse_input_lock(
        document,
        registry,
        require_complete=require_complete,
    )


def _verify_selected_cache(
    lock: InputLock,
    cache_root: Path,
    target_id: str,
) -> tuple[VerifiedInput, ...]:
    selected = lock.selected(target_id)
    inventory = []
    errors = []
    for item in selected:
        try:
            inventory.append(_verify_object(cache_root, item))
        except PackagingError as error:
            errors.extend(error.messages)
    if errors:
        raise PackagingError(errors)
    return tuple(inventory)


# -----------------------------------------------------------------------------
# EXPORTED FUNCTIONS / PUBLIC API
# -----------------------------------------------------------------------------


def load_targets(path: Path) -> TargetRegistry:
    """
    @brief Load and validate the exact delivery-target registry.
    @param[in] path Registry JSON path.
    @return Validated target registry.
    @exception PackagingError The document violates the registry contract.
    """
    _, registry = _load_targets_snapshot(path)
    return registry


def load_lock(
    path: Path,
    registry: TargetRegistry,
    *,
    require_complete: bool,
) -> InputLock:
    """
    @brief Load and validate a packaging-input lock.
    @param[in] path Lock JSON path.
    @param[in] registry Validated target registry.
    @param[in] require_complete Refuse an explicitly incomplete lock.
    @return Validated input lock.
    @exception PackagingError The document violates the lock contract.
    """
    _, lock = _load_lock_snapshot(
        path,
        registry,
        require_complete=require_complete,
    )
    return lock


def cache_object_path(cache_root: Path, sha256: str) -> Path:
    """
    @brief Derive an immutable cache path from a validated digest.
    @param[in] cache_root Packaging cache root.
    @param[in] sha256 Lowercase SHA-256 digest.
    @return Digest-derived object path.
    """
    if SHA256_PATTERN.fullmatch(sha256) is None:
        raise PackagingError("cache object digest must be 64 lowercase hex characters")
    return cache_root / "objects" / "sha256" / sha256[:2] / sha256


def resolve_inputs(
    targets_path: Path,
    declarations_path: Path,
    cache_root: Path,
    proposal_path: Path,
    *,
    opener: Callable[..., Any] = urllib.request.urlopen,
    signature_verifier: Callable[[Path, Path, list[str]], str] = _default_apt_signature_verifier,
) -> dict[str, Any]:
    """Resolve declared connected sources into an immutable lock proposal.

    A proposal becomes complete only when every reviewed input class for both
    exact targets is present. The proposal is intentionally separate from the
    adopted lock so a release engineer can review the observed digests first.
    """
    _, registry = _load_targets_snapshot(targets_path)
    document = _require_object(
        _decode_json(_read_bytes(declarations_path), declarations_path),
        "release input declarations",
    )
    _check_fields(
        document,
        frozenset({"schema", "sources"}),
        "release input declarations",
    )
    if type(document["schema"]) is not int or document["schema"] != 1:
        raise PackagingError("release input declarations schema must be integer 1")
    if not isinstance(document["sources"], list):
        raise PackagingError("release input declarations sources must be an array")

    known_scopes = {"shared", *(target.id for target in registry.targets)}
    parsed: list[dict[str, Any]] = []
    seen: set[tuple[str, str]] = set()
    for index, value in enumerate(document["sources"]):
        context = f"sources[{index}]"
        source = _require_object(value, context)
        _check_fields(source, DECLARED_INPUT_FIELDS, context)
        name = _require_nonempty_string(source["name"], f"{context}.name")
        if INPUT_NAME_PATTERN.fullmatch(name) is None:
            raise PackagingError(f"{context}.name has unsafe input metadata")
        scope = _require_nonempty_string(source["scope"], f"{context}.scope")
        if scope not in known_scopes:
            raise PackagingError(f"{context}.scope names unknown target: {scope}")
        if name not in REQUIRED_RELEASE_INPUTS[scope]:
            raise PackagingError(f"{context}.name is not a required input class for {scope}: {name}")
        key = (scope, name)
        if key in seen:
            raise PackagingError(f"release input declarations contain duplicate {scope}/{name}")
        seen.add(key)
        artifact_type = _require_nonempty_string(source["artifact_type"], f"{context}.artifact_type")
        if artifact_type != INPUT_ARTIFACT_TYPES[name]:
            raise PackagingError(f"{context}.artifact_type does not match input class {name}")
        architecture = _require_nonempty_string(source["architecture"], f"{context}.architecture")
        expected_architecture = "all" if scope == "shared" else registry.require(scope).architecture
        if architecture != expected_architecture:
            raise PackagingError(f"{context}.architecture must be {expected_architecture}")
        redistribution = _require_nonempty_string(source["redistribution"], f"{context}.redistribution")
        if redistribution != "permitted":
            raise PackagingError(f"{context}.redistribution must be explicitly permitted")
        if artifact_type == "debian-pool-tar":
            apt = _parse_apt_policy(source["apt"], f"{context}.apt")
        elif source["apt"] is not None:
            raise PackagingError(f"{context}.apt is allowed only for a Debian pool")
        else:
            apt = None
        parsed.append(
            {
                "apt": apt,
                "architecture": architecture,
                "artifact_type": artifact_type,
                "license": _require_nonempty_string(source["license"], f"{context}.license"),
                "name": name,
                "provenance": _validate_https_url(source["provenance"], f"{context}.provenance"),
                "redistribution": redistribution,
                "scope": scope,
                "url": _validate_https_url(source["url"], f"{context}.url"),
                "version": _require_nonempty_string(source["version"], f"{context}.version"),
            }
        )
    if parsed != sorted(parsed, key=lambda item: (item["scope"], item["name"])):
        raise PackagingError("release input declarations must be sorted by scope and name")

    staging_root = cache_root / ".staging"
    staging_root.mkdir(parents=True, exist_ok=True)
    locked: list[dict[str, Any]] = []
    for source in parsed:
        request = urllib.request.Request(
            source["url"], headers={"User-Agent": "NILO-packaging-resolver/1"}
        )
        try:
            response_context = opener(request, timeout=DOWNLOAD_TIMEOUT_SECONDS)
        except (OSError, ValueError) as error:
            raise PackagingError(f"{source['name']}: resolution download failed: {error}") from error
        temporary_path: Path | None = None
        try:
            handle, temporary_name = tempfile.mkstemp(
                prefix=f"resolve-{source['scope']}-{source['name']}.",
                suffix=".part",
                dir=staging_root,
            )
            temporary_path = Path(temporary_name)
            digest = hashlib.sha256()
            size = 0
            with os.fdopen(handle, "wb") as output, response_context as response:
                _validate_https_url(response.geturl(), f"{source['name']} final URL")
                while chunk := response.read(DOWNLOAD_CHUNK_SIZE):
                    size += len(chunk)
                    if size > MAXIMUM_RESOLVED_INPUT_BYTES:
                        raise PackagingError(f"{source['name']}: resolved input exceeds 16 GiB")
                    digest.update(chunk)
                    output.write(chunk)
                output.flush()
                os.fsync(output.fileno())
            if size == 0:
                raise PackagingError(f"{source['name']}: resolved input is empty")
            sha256 = digest.hexdigest()
            candidate = LockedInput(
                name=source["name"], scope=source["scope"],
                architecture=source["architecture"], artifact_type=source["artifact_type"],
                url=source["url"], version=source["version"], license=source["license"],
                provenance=source["provenance"], redistribution=source["redistribution"],
                sha256=sha256, size=size,
                apt=source["apt"],
            )
            apt = _validate_resolved_artifact(
                temporary_path,
                candidate,
                apt_policy=source["apt"],
                signature_verifier=signature_verifier,
            )
            final_path = cache_object_path(cache_root, sha256)
            final_path.parent.mkdir(parents=True, exist_ok=True)
            if final_path.exists():
                _verify_object(cache_root, candidate)
                temporary_path.unlink()
                temporary_path = None
            else:
                os.replace(temporary_path, final_path)
                temporary_path = None
            locked.append({**source, "apt": apt, "sha256": sha256, "size": size})
        finally:
            if temporary_path is not None:
                temporary_path.unlink(missing_ok=True)

    incomplete_reasons = sorted([
        f"{scope} is missing required input class {name}"
        for scope, names in sorted(REQUIRED_RELEASE_INPUTS.items())
        for name in names
        if (scope, name) not in seen
    ])
    proposal = {
        "complete": not incomplete_reasons,
        "incomplete_reasons": incomplete_reasons,
        "inputs": locked,
        "schema": 1,
    }
    _write_json_atomically(proposal_path, proposal)
    # Parse our own output before returning it so the proposal cannot diverge
    # from the strict lock format consumed by fetch and offline verification.
    _load_lock_snapshot(proposal_path, registry, require_complete=False)
    return proposal


def fetch_inputs(
    targets_path: Path,
    lock_path: Path,
    cache_root: Path,
    target_id: str,
    *,
    opener: Callable[..., Any] = urllib.request.urlopen,
) -> tuple[VerifiedInput, ...]:
    """
    @brief Fetch and atomically cache one target's immutable inputs.
    @param[in] targets_path Target registry JSON.
    @param[in] lock_path Complete input lock JSON.
    @param[in] cache_root Ignored cache directory.
    @param[in] target_id Selected exact target.
    @param[in] opener Injectable HTTPS opener used by tests.
    @return Sorted verified target inventory.
    @exception PackagingError Validation or acquisition fails.
    """
    _, registry = _load_targets_snapshot(targets_path)
    registry.require(target_id)
    _, lock = _load_lock_snapshot(
        lock_path,
        registry,
        require_complete=True,
    )
    selected = lock.selected(target_id)
    staging_root = cache_root / ".staging"
    staging_root.mkdir(parents=True, exist_ok=True)

    inventory = []
    for item in selected:
        final_path = cache_object_path(cache_root, item.sha256)
        try:
            inventory.append(_verify_object(cache_root, item))
            continue
        except CacheObjectError as error:
            if error.kind == "content":
                _remove_cache_object(cache_root, item)
            elif error.kind != "missing":
                raise

        temporary_path: Path | None = None
        try:
            handle, temporary_name = tempfile.mkstemp(
                prefix=f"{item.sha256}.", suffix=".part", dir=staging_root
            )
            os.close(handle)
            temporary_path = Path(temporary_name)
            try:
                request = urllib.request.Request(
                    item.url,
                    headers={"User-Agent": "NILO-packaging-inputs/1"},
                )
            except (TypeError, ValueError) as error:
                raise PackagingError(
                    f"{item.name}: invalid request URL: {error}"
                ) from error
            try:
                response_context = opener(request, timeout=DOWNLOAD_TIMEOUT_SECONDS)
            except (OSError, ValueError) as error:
                raise PackagingError(
                    f"{item.name}: download failed: {error}"
                ) from error
            with response_context as response:
                final_url = response.geturl()
                _validate_https_url(final_url, f"{item.name} final URL")
                _download_input(item, response, temporary_path)
            final_path.parent.mkdir(parents=True, exist_ok=True)
            os.replace(temporary_path, final_path)
            temporary_path = None
            inventory.append(_verify_object(cache_root, item))
        finally:
            if temporary_path is not None:
                temporary_path.unlink(missing_ok=True)
    return tuple(inventory)


def verify_cache(
    targets_path: Path,
    lock_path: Path,
    cache_root: Path,
    target_id: str,
) -> tuple[VerifiedInput, ...]:
    """
    @brief Verify the complete immutable cache closure for one target.
    @param[in] targets_path Target registry JSON.
    @param[in] lock_path Complete input lock JSON.
    @param[in] cache_root Ignored cache directory.
    @param[in] target_id Selected exact target.
    @return Sorted verified target inventory.
    @exception PackagingError Configuration or one or more objects fail.
    """
    _, registry = _load_targets_snapshot(targets_path)
    registry.require(target_id)
    _, lock = _load_lock_snapshot(
        lock_path,
        registry,
        require_complete=True,
    )
    return _verify_selected_cache(lock, cache_root, target_id)


def observe_network() -> NetworkObservation:
    """
    @brief Observe the current Linux namespace without opening a socket.
    @return Platform, interface, and default-route facts.
    @exception PackagingError Network state cannot be inspected.
    """
    platform = sys.platform
    if platform != "linux":
        return NetworkObservation(platform, tuple(), False, False)
    interface_root = Path("/sys/class/net")
    try:
        interfaces = tuple(sorted(path.name for path in interface_root.iterdir()))
    except OSError as error:
        raise PackagingError(
            f"cannot inspect network interfaces at {interface_root}: {error}"
        ) from error
    return NetworkObservation(
        platform=platform,
        interfaces=interfaces,
        ipv4_default_route=_has_ipv4_default_route(Path("/proc/net/route")),
        ipv6_default_route=_has_ipv6_default_route(Path("/proc/net/ipv6_route")),
    )


def prepare_inputs(
    targets_path: Path,
    lock_path: Path,
    cache_root: Path,
    target_id: str,
    output_path: Path,
    *,
    observation: NetworkObservation | None = None,
) -> dict[str, Any]:
    """
    @brief Prepare a deterministic receipt from verified offline inputs.
    @param[in] targets_path Target registry JSON.
    @param[in] lock_path Complete input lock JSON.
    @param[in] cache_root Read-only packaging cache.
    @param[in] target_id Selected exact target.
    @param[in] output_path Receipt destination.
    @param[in] observation Optional injected network observation for tests.
    @return Canonical receipt value.
    @exception PackagingError Cache or isolation validation fails.
    """
    _, registry = _load_targets_snapshot(targets_path)
    target = registry.require(target_id)
    lock_bytes, lock = _load_lock_snapshot(
        lock_path,
        registry,
        require_complete=True,
    )
    inventory = _verify_selected_cache(lock, cache_root, target_id)
    network = observation if observation is not None else observe_network()
    errors = []
    if network.platform != "linux":
        errors.append(
            f"offline preparation requires Linux, observed {network.platform}"
        )
    if network.interfaces != ("lo",):
        errors.append(
            "offline preparation requires loopback as the sole interface, "
            f"observed {', '.join(network.interfaces) or 'none'}"
        )
    if network.ipv4_default_route:
        errors.append("offline preparation detected an IPv4 default route")
    if network.ipv6_default_route:
        errors.append("offline preparation detected an IPv6 default route")
    if errors:
        raise PackagingError(errors)
    lock_digest = hashlib.sha256(lock_bytes).hexdigest()
    receipt = {
        "cache_root": str(cache_root.resolve()),
        "hardware_acceptance": {
            "required_checks": list(target.hardware_acceptance),
            "status": "not_run",
        },
        "inputs": [item.to_dict() for item in inventory],
        "lock_sha256": lock_digest,
        "offline": network.to_dict(),
        "schema": 1,
        "target": target.to_dict(),
    }
    _write_json_atomically(output_path, receipt)
    return receipt


def build_parser() -> argparse.ArgumentParser:
    """@brief Construct the command-line parser."""
    parser = argparse.ArgumentParser(
        description="Validate and prepare NILO packaging inputs."
    )
    subparsers = parser.add_subparsers(dest="command", required=True)
    list_parser = subparsers.add_parser(
        "list-targets", help="validate and list exact delivery targets"
    )
    list_parser.add_argument("--targets", required=True, type=Path)
    list_parser.add_argument("--target")
    fetch_parser = subparsers.add_parser(
        "fetch", help="download and verify locked packaging inputs"
    )
    fetch_parser.add_argument("--targets", required=True, type=Path)
    fetch_parser.add_argument("--lock", required=True, type=Path)
    fetch_parser.add_argument("--cache", required=True, type=Path)
    fetch_parser.add_argument("--target", required=True)
    resolve_parser = subparsers.add_parser(
        "resolve", help="download declared mutable sources and write a lock proposal"
    )
    resolve_parser.add_argument("--targets", required=True, type=Path)
    resolve_parser.add_argument("--declarations", required=True, type=Path)
    resolve_parser.add_argument("--cache", required=True, type=Path)
    resolve_parser.add_argument("--proposal", required=True, type=Path)
    verify_parser = subparsers.add_parser(
        "verify-cache", help="verify one target's complete packaging cache"
    )
    verify_parser.add_argument("--targets", required=True, type=Path)
    verify_parser.add_argument("--lock", required=True, type=Path)
    verify_parser.add_argument("--cache", required=True, type=Path)
    verify_parser.add_argument("--target", required=True)
    prepare_parser = subparsers.add_parser(
        "prepare", help="verify isolated offline inputs and write a receipt"
    )
    prepare_parser.add_argument("--targets", required=True, type=Path)
    prepare_parser.add_argument("--lock", required=True, type=Path)
    prepare_parser.add_argument("--cache", required=True, type=Path)
    prepare_parser.add_argument("--target", required=True)
    prepare_parser.add_argument("--output", required=True, type=Path)
    return parser


def main(argv: Sequence[str] | None = None) -> int:
    """
    @brief Run the packaging-input command line.
    @param[in] argv Optional arguments excluding the executable name.
    @return Process exit code.
    """
    parser = build_parser()
    arguments = parser.parse_args(argv)
    try:
        if arguments.command == "list-targets":
            registry = load_targets(arguments.targets)
            value: Any = registry.to_dict()
            if arguments.target:
                value = registry.require(arguments.target).to_dict()
            sys.stdout.write(_canonical_json(value))
            return 0
        if arguments.command == "fetch":
            inventory = fetch_inputs(
                arguments.targets,
                arguments.lock,
                arguments.cache,
                arguments.target,
            )
            sys.stdout.write(
                _canonical_json(
                    {
                        "inputs": [item.to_dict() for item in inventory],
                        "target": arguments.target,
                    }
                )
            )
            return 0
        if arguments.command == "resolve":
            proposal = resolve_inputs(
                arguments.targets,
                arguments.declarations,
                arguments.cache,
                arguments.proposal,
            )
            sys.stdout.write(_canonical_json(proposal))
            return 0
        if arguments.command == "prepare":
            receipt = prepare_inputs(
                arguments.targets,
                arguments.lock,
                arguments.cache,
                arguments.target,
                arguments.output,
            )
            sys.stdout.write(_canonical_json(receipt))
            return 0
        if arguments.command == "verify-cache":
            inventory = verify_cache(
                arguments.targets,
                arguments.lock,
                arguments.cache,
                arguments.target,
            )
            sys.stdout.write(
                _canonical_json(
                    {
                        "inputs": [item.to_dict() for item in inventory],
                        "target": arguments.target,
                    }
                )
            )
            return 0
    except PackagingError as error:
        for message in error.messages:
            print(f"error: {message}", file=sys.stderr)
        return 2
    parser.error(f"unsupported command: {arguments.command}")
    return 2


if __name__ == "__main__":
    raise SystemExit(main())

# -----------------------------------------------------------------------------
# END OF FILE
# -----------------------------------------------------------------------------
