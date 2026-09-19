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
import os
import re
import stat
import sys
import tempfile
import urllib.parse
import urllib.request
from dataclasses import dataclass
from pathlib import Path
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
    {"name", "scope", "url", "version", "license", "sha256", "size"}
)
DOWNLOAD_CHUNK_SIZE = 1024 * 1024
DOWNLOAD_TIMEOUT_SECONDS = 60
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
    url: str
    version: str
    license: str
    sha256: str
    size: int


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
    version: str
    license: str
    sha256: str
    size: int
    cache_object: str

    def to_dict(self) -> dict[str, Any]:
        """@brief Return the canonical receipt representation."""
        return {
            "cache_object": self.cache_object,
            "license": self.license,
            "name": self.name,
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
    version = _require_nonempty_string(record["version"], f"{context}.version")
    license_name = _require_nonempty_string(
        record["license"], f"{context}.license"
    )
    sha256 = _require_nonempty_string(record["sha256"], f"{context}.sha256")
    if SHA256_PATTERN.fullmatch(sha256) is None:
        raise PackagingError(f"{context}.sha256 must be 64 lowercase hex characters")
    size = record["size"]
    if type(size) is not int or size <= 0:
        raise PackagingError(f"{context}.size must be a positive integer")
    return LockedInput(
        name=name,
        scope=scope,
        url=url,
        version=version,
        license=license_name,
        sha256=sha256,
        size=size,
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
    return VerifiedInput(
        name=item.name,
        scope=item.scope,
        version=item.version,
        license=item.license,
        sha256=item.sha256,
        size=item.size,
        cache_object=path.relative_to(cache_root).as_posix(),
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
