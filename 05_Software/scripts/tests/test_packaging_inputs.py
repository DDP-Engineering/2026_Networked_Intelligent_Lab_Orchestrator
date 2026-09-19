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
@file test_packaging_inputs.py
@brief Verify the offline packaging-input boundary.
@details
    Exercises exact target selection, immutable acquisition, cache closure,
    offline isolation, deterministic receipts, and wrapper contracts.
"""

from __future__ import annotations

import hashlib
import importlib.util
import io
import json
import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from unittest import mock


REPOSITORY_ROOT = Path(__file__).resolve().parents[3]
PACKAGING_ROOT = REPOSITORY_ROOT / "02_Tools" / "packaging"
MODULE_PATH = PACKAGING_ROOT / "package_inputs.py"
TARGETS_PATH = PACKAGING_ROOT / "config" / "targets.json"
LOCK_PATH = PACKAGING_ROOT / "config" / "inputs.lock.json"


def load_packaging_module():
    """Load the packaging module from its repository path."""
    spec = importlib.util.spec_from_file_location("nilo_package_inputs", MODULE_PATH)
    if spec is None or spec.loader is None:
        raise AssertionError(f"cannot load packaging module from {MODULE_PATH}")
    module = importlib.util.module_from_spec(spec)
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


def write_json(path: Path, value: object) -> None:
    path.write_text(
        json.dumps(value, indent=2, sort_keys=True) + "\n",
        encoding="utf-8",
    )


def target_document() -> dict[str, object]:
    return {
        "schema": 1,
        "targets": [
            {
                "id": "rpi-debian13-arm64",
                "os": {
                    "id": "raspbian",
                    "family": "debian",
                    "version_id": "13",
                },
                "architecture": "arm64",
                "artifacts": ["deb", "img.xz", "run"],
                "hardware_acceptance": [
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
                ],
            },
            {
                "id": "ubuntu-24.04-amd64",
                "os": {
                    "id": "ubuntu",
                    "family": "debian",
                    "version_id": "24.04",
                },
                "architecture": "amd64",
                "artifacts": ["deb", "run"],
                "hardware_acceptance": [
                    "boot-reboot",
                    "integrations",
                    "network",
                    "services-gui",
                    "usb-device-access",
                ],
            },
        ],
    }


def lock_document(
    payloads: list[tuple[str, str, bytes]],
    *,
    complete: bool = True,
) -> dict[str, object]:
    return {
        "schema": 1,
        "complete": complete,
        "incomplete_reasons": []
        if complete
        else ["dependency closures are not locked"],
        "inputs": [
            {
                "license": "MIT",
                "name": name,
                "scope": scope,
                "url": f"https://packages.example.invalid/{name}",
                "version": "1.0.0",
                "sha256": hashlib.sha256(payload).hexdigest(),
                "size": len(payload),
            }
            for name, scope, payload in sorted(payloads, key=lambda item: item[:2])
        ],
    }


class FakeResponse(io.BytesIO):
    def __init__(self, payload: bytes, final_url: str) -> None:
        super().__init__(payload)
        self._final_url = final_url

    def geturl(self) -> str:
        return self._final_url

    def __enter__(self):
        return self

    def __exit__(self, exc_type, exc, traceback) -> None:
        self.close()


class InterruptingResponse(FakeResponse):
    """Return one partial chunk, then simulate a failed network read."""

    def __init__(self, payload: bytes, final_url: str) -> None:
        super().__init__(payload, final_url)
        self._read_count = 0

    def read(self, size: int = -1) -> bytes:
        self._read_count += 1
        if self._read_count > 1:
            raise OSError("connection interrupted during transfer")
        return super().read(max(1, len(self.getvalue()) // 2))


class ReplacedAfterParseLock:
    """Expose changed bytes if a caller reads a lock more than once."""

    def __init__(self, path: Path, original: bytes, replacement: bytes) -> None:
        self._path = path
        self._original = original
        self._replacement = replacement
        self._parsed = False

    def read_text(self, encoding: str) -> str:
        self._parsed = True
        return self._original.decode(encoding)

    def read_bytes(self) -> bytes:
        if self._parsed:
            return self._replacement
        self._parsed = True
        return self._original

    def __str__(self) -> str:
        return str(self._path)


class PackagingInputTests(unittest.TestCase):
    def test_shipped_target_registry_is_exact(self) -> None:
        self.assertTrue(TARGETS_PATH.is_file(), "the target registry is required")
        module = load_packaging_module()
        registry = module.load_targets(TARGETS_PATH)
        self.assertEqual(
            [target.id for target in registry.targets],
            ["rpi-debian13-arm64", "ubuntu-24.04-amd64"],
        )
        ubuntu = registry.require("ubuntu-24.04-amd64")
        self.assertEqual((ubuntu.os_id, ubuntu.version_id), ("ubuntu", "24.04"))
        self.assertEqual(ubuntu.architecture, "amd64")
        raspberry_pi = registry.require("rpi-debian13-arm64")
        self.assertEqual(
            (raspberry_pi.os_family, raspberry_pi.version_id),
            ("debian", "13"),
        )
        self.assertEqual(raspberry_pi.architecture, "arm64")
        self.assertEqual(
            raspberry_pi.hardware_acceptance,
            tuple(target_document()["targets"][0]["hardware_acceptance"]),
        )

    def test_target_registry_rejects_ambiguous_or_inconsistent_metadata(self) -> None:
        module = load_packaging_module()
        with tempfile.TemporaryDirectory() as temporary:
            path = Path(temporary) / "targets.json"
            cases = []

            duplicate = target_document()
            duplicate["targets"] = [
                duplicate["targets"][0],
                duplicate["targets"][0],
            ]
            cases.append(("duplicate", duplicate))

            unsupported_arch = target_document()
            unsupported_arch["targets"][0]["architecture"] = "armhf"
            cases.append(("architecture", unsupported_arch))

            wrong_artifact = target_document()
            wrong_artifact["targets"][1]["artifacts"] = ["deb", "img.xz", "run"]
            cases.append(("img.xz", wrong_artifact))

            empty_checks = target_document()
            empty_checks["targets"][0]["hardware_acceptance"] = []
            cases.append(("hardware_acceptance", empty_checks))

            wrong_ubuntu_architecture = target_document()
            wrong_ubuntu_architecture["targets"][1]["architecture"] = "arm64"
            cases.append(("exact definition", wrong_ubuntu_architecture))

            wrong_ubuntu_release = target_document()
            wrong_ubuntu_release["targets"][1]["os"]["version_id"] = "25.04"
            cases.append(("exact definition", wrong_ubuntu_release))

            wrong_pi_identity = target_document()
            wrong_pi_identity["targets"][0]["os"]["id"] = "debian"
            cases.append(("exact definition", wrong_pi_identity))

            incomplete_artifact_set = target_document()
            incomplete_artifact_set["targets"][0]["artifacts"] = ["deb", "run"]
            cases.append(("exact definition", incomplete_artifact_set))

            incomplete_hardware_checks = target_document()
            incomplete_hardware_checks["targets"][0]["hardware_acceptance"].remove(
                "administrator-credentials"
            )
            cases.append(("exact definition", incomplete_hardware_checks))

            unsupported_target = target_document()
            unsupported_target["targets"][1]["id"] = "ubuntu-25.04-amd64"
            cases.append(("unsupported target", unsupported_target))

            for expected, document in cases:
                with self.subTest(expected=expected):
                    write_json(path, document)
                    with self.assertRaisesRegex(module.PackagingError, expected):
                        module.load_targets(path)

    def test_fetch_caches_only_verified_https_bytes_atomically(self) -> None:
        module = load_packaging_module()
        ubuntu_payload = b"ubuntu-package"
        shared_payload = b"shared-runtime"
        pi_payload = b"pi-package"
        payloads = [
            ("pi-package", "rpi-debian13-arm64", pi_payload),
            ("shared-runtime", "shared", shared_payload),
            ("ubuntu-package", "ubuntu-24.04-amd64", ubuntu_payload),
        ]
        responses = {
            "https://packages.example.invalid/shared-runtime": shared_payload,
            "https://packages.example.invalid/ubuntu-package": ubuntu_payload,
        }
        calls: list[str] = []

        def opener(request, timeout=0):
            url = request.full_url
            calls.append(url)
            return FakeResponse(responses[url], url)

        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            targets = root / "targets.json"
            lock = root / "inputs.lock.json"
            cache = root / "cache"
            write_json(targets, target_document())
            write_json(lock, lock_document(payloads))

            inventory = module.fetch_inputs(
                targets, lock, cache, "ubuntu-24.04-amd64", opener=opener
            )
            self.assertEqual(
                [item.name for item in inventory],
                ["shared-runtime", "ubuntu-package"],
            )
            self.assertEqual(
                {(item.version, item.license) for item in inventory},
                {("1.0.0", "MIT")},
            )
            self.assertEqual(len(calls), 2)
            for item in inventory:
                object_path = cache / item.cache_object
                self.assertTrue(object_path.is_file())
                self.assertEqual(
                    hashlib.sha256(object_path.read_bytes()).hexdigest(),
                    item.sha256,
                )
            self.assertEqual(list((cache / ".staging").glob("*")), [])

            module.fetch_inputs(
                targets, lock, cache, "ubuntu-24.04-amd64", opener=opener
            )
            self.assertEqual(len(calls), 2, "verified objects must be reused")

    def test_fetch_rejects_redirect_size_digest_and_interruption_without_residue(
        self,
    ) -> None:
        module = load_packaging_module()
        payload = b"expected"
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            targets = root / "targets.json"
            lock = root / "inputs.lock.json"
            cache = root / "cache"
            write_json(targets, target_document())
            write_json(
                lock,
                lock_document(
                    [("ubuntu-package", "ubuntu-24.04-amd64", payload)]
                ),
            )

            cases = [
                (
                    "HTTPS",
                    lambda request, timeout=0: FakeResponse(
                        payload, "http://mirror.invalid/package"
                    ),
                ),
                (
                    "exceeded",
                    lambda request, timeout=0: FakeResponse(
                        payload + b"x", request.full_url
                    ),
                ),
                (
                    "size mismatch",
                    lambda request, timeout=0: FakeResponse(
                        payload[:-1], request.full_url
                    ),
                ),
                (
                    "SHA-256",
                    lambda request, timeout=0: FakeResponse(
                        b"mismatch", request.full_url
                    ),
                ),
                (
                    "download failed",
                    lambda request, timeout=0: InterruptingResponse(
                        payload, request.full_url
                    ),
                ),
                (
                    "download failed",
                    lambda request, timeout=0: (_ for _ in ()).throw(
                        OSError("connection interrupted")
                    ),
                ),
            ]
            for expected, opener in cases:
                with self.subTest(expected=expected):
                    with self.assertRaisesRegex(module.PackagingError, expected):
                        module.fetch_inputs(
                            targets,
                            lock,
                            cache,
                            "ubuntu-24.04-amd64",
                            opener=opener,
                        )
                    staging = cache / ".staging"
                    self.assertEqual(list(staging.glob("*")) if staging.exists() else [], [])
                    objects = cache / "objects"
                    self.assertEqual(
                        [path for path in objects.rglob("*") if path.is_file()]
                        if objects.exists()
                        else [],
                        [],
                    )

    def test_fetch_removes_a_corrupt_object_before_a_failed_retry(self) -> None:
        module = load_packaging_module()
        payload = b"expected"
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            targets = root / "targets.json"
            lock = root / "inputs.lock.json"
            cache = root / "cache"
            document = lock_document(
                [("ubuntu-package", "ubuntu-24.04-amd64", payload)]
            )
            write_json(targets, target_document())
            write_json(lock, document)
            object_path = module.cache_object_path(
                cache, document["inputs"][0]["sha256"]
            )
            object_path.parent.mkdir(parents=True)
            object_path.write_bytes(b"corrupt")

            def interrupted(request, timeout=0):
                raise OSError("offline")

            with self.assertRaisesRegex(module.PackagingError, "download failed"):
                module.fetch_inputs(
                    targets,
                    lock,
                    cache,
                    "ubuntu-24.04-amd64",
                    opener=interrupted,
                )
            self.assertFalse(object_path.exists())

    def test_lock_rejects_unsafe_sources_duplicates_and_incomplete_state(self) -> None:
        module = load_packaging_module()
        payload = b"payload"
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            targets_path = root / "targets.json"
            lock_path = root / "inputs.lock.json"
            write_json(targets_path, target_document())
            registry = module.load_targets(targets_path)

            cases = []
            insecure = lock_document(
                [("ubuntu-package", "ubuntu-24.04-amd64", payload)]
            )
            insecure["inputs"][0]["url"] = "http://example.invalid/package"
            cases.append(("HTTPS", insecure))

            credentials = lock_document(
                [("ubuntu-package", "ubuntu-24.04-amd64", payload)]
            )
            credentials["inputs"][0][
                "url"
            ] = "https://user:password@example.invalid/package"
            cases.append(("credentials", credentials))

            fragment = lock_document(
                [("ubuntu-package", "ubuntu-24.04-amd64", payload)]
            )
            fragment["inputs"][0]["url"] += "#fragment"
            cases.append(("fragment", fragment))

            malformed_url = lock_document(
                [("ubuntu-package", "ubuntu-24.04-amd64", payload)]
            )
            malformed_url["inputs"][0]["url"] = "https://[invalid"
            cases.append(("invalid URL", malformed_url))

            unsafe_name = lock_document(
                [("ubuntu-package", "ubuntu-24.04-amd64", payload)]
            )
            unsafe_name["inputs"][0]["name"] = "../escape"
            cases.append(("unsafe", unsafe_name))

            unknown_scope = lock_document(
                [("ubuntu-package", "ubuntu-24.04-amd64", payload)]
            )
            unknown_scope["inputs"][0]["scope"] = "ubuntu-25.04-amd64"
            cases.append(("unknown target", unknown_scope))

            missing_version = lock_document(
                [("ubuntu-package", "ubuntu-24.04-amd64", payload)]
            )
            missing_version["inputs"][0]["version"] = ""
            cases.append(("version", missing_version))

            missing_license = lock_document(
                [("ubuntu-package", "ubuntu-24.04-amd64", payload)]
            )
            missing_license["inputs"][0]["license"] = ""
            cases.append(("license", missing_license))

            duplicate = lock_document(
                [("ubuntu-package", "ubuntu-24.04-amd64", payload)]
            )
            duplicate["inputs"].append(dict(duplicate["inputs"][0]))
            cases.append(("duplicate", duplicate))

            incomplete = lock_document(
                [("ubuntu-package", "ubuntu-24.04-amd64", payload)],
                complete=False,
            )
            cases.append(("incomplete", incomplete))

            for expected, document in cases:
                with self.subTest(expected=expected):
                    write_json(lock_path, document)
                    with self.assertRaisesRegex(module.PackagingError, expected):
                        module.load_lock(lock_path, registry, require_complete=True)

    def test_verify_cache_reports_all_object_defects(self) -> None:
        module = load_packaging_module()
        payloads = {
            "pi-ignored": b"pi",
            "shared-long": b"long",
            "shared-missing": b"missing",
            "shared-short": b"expected-size",
            "ubuntu-digest": b"ubuntu",
        }
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            targets = root / "targets.json"
            lock = root / "inputs.lock.json"
            cache = root / "cache"
            document = lock_document(
                [
                    ("pi-ignored", "rpi-debian13-arm64", payloads["pi-ignored"]),
                    ("shared-long", "shared", payloads["shared-long"]),
                    ("shared-missing", "shared", payloads["shared-missing"]),
                    ("shared-short", "shared", payloads["shared-short"]),
                    (
                        "ubuntu-digest",
                        "ubuntu-24.04-amd64",
                        payloads["ubuntu-digest"],
                    ),
                ]
            )
            write_json(targets, target_document())
            write_json(lock, document)
            for entry in document["inputs"]:
                payload = payloads[entry["name"]]
                path = module.cache_object_path(cache, entry["sha256"])
                path.parent.mkdir(parents=True, exist_ok=True)
                path.write_bytes(payload)

            inventory = module.verify_cache(
                targets, lock, cache, "ubuntu-24.04-amd64"
            )
            self.assertEqual(
                [item.name for item in inventory],
                [
                    "shared-long",
                    "shared-missing",
                    "shared-short",
                    "ubuntu-digest",
                ],
            )

            entries = {entry["name"]: entry for entry in document["inputs"]}
            long = module.cache_object_path(
                cache, entries["shared-long"]["sha256"]
            )
            missing = module.cache_object_path(
                cache, entries["shared-missing"]["sha256"]
            )
            short = module.cache_object_path(
                cache, entries["shared-short"]["sha256"]
            )
            digest = module.cache_object_path(
                cache, entries["ubuntu-digest"]["sha256"]
            )
            long.write_bytes(payloads["shared-long"] + b"-too-long")
            missing.unlink()
            short.write_bytes(b"short")
            digest.write_bytes(b"UBUNTU")
            with self.assertRaises(module.PackagingError) as caught:
                module.verify_cache(
                    targets, lock, cache, "ubuntu-24.04-amd64"
                )
            message = str(caught.exception)
            self.assertIn("shared-long", message)
            self.assertIn("size", message)
            self.assertIn("shared-missing", message)
            self.assertIn("missing", message)
            self.assertIn("shared-short", message)
            self.assertIn("size", message)
            self.assertIn("ubuntu-digest", message)
            self.assertIn("SHA-256", message)

            receipt = root / "receipt.json"
            isolated = module.NetworkObservation("linux", ("lo",), False, False)
            with self.assertRaises(module.PackagingError):
                module.prepare_inputs(
                    targets,
                    lock,
                    cache,
                    "ubuntu-24.04-amd64",
                    receipt,
                    observation=isolated,
                )
            self.assertFalse(receipt.exists())

    @unittest.skipUnless(
        os.name == "posix" and hasattr(os, "mkfifo"),
        "FIFO cache-object checks require POSIX",
    )
    def test_verify_cache_rejects_fifo_without_blocking(self) -> None:
        module = load_packaging_module()
        payload = b"fifo-input"
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            targets = root / "targets.json"
            lock = root / "inputs.lock.json"
            cache = root / "cache"
            document = lock_document(
                [("fifo-input", "ubuntu-24.04-amd64", payload)]
            )
            write_json(targets, target_document())
            write_json(lock, document)
            object_path = module.cache_object_path(
                cache, document["inputs"][0]["sha256"]
            )
            object_path.parent.mkdir(parents=True)
            os.mkfifo(object_path)

            with self.assertRaisesRegex(module.PackagingError, "regular file"):
                module.verify_cache(
                    targets, lock, cache, "ubuntu-24.04-amd64"
                )

    @unittest.skipIf(os.name == "nt", "Windows test token cannot create symlinks")
    def test_verify_cache_rejects_symlinked_cache_objects(self) -> None:
        module = load_packaging_module()
        payload = b"outside-cache"
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            targets = root / "targets.json"
            lock = root / "inputs.lock.json"
            cache = root / "cache"
            outside = root / "outside.bin"
            document = lock_document(
                [
                    ("missing-input", "ubuntu-24.04-amd64", b"missing"),
                    ("symlink-input", "ubuntu-24.04-amd64", payload),
                ]
            )
            write_json(targets, target_document())
            write_json(lock, document)
            outside.write_bytes(payload)
            entries = {entry["name"]: entry for entry in document["inputs"]}
            object_path = module.cache_object_path(
                cache, entries["symlink-input"]["sha256"]
            )
            object_path.parent.mkdir(parents=True)
            object_path.symlink_to(outside)

            with self.assertRaises(module.PackagingError) as caught:
                module.verify_cache(
                    targets, lock, cache, "ubuntu-24.04-amd64"
                )
            message = str(caught.exception)
            self.assertIn("missing-input", message)
            self.assertIn("missing", message)
            self.assertIn("symlink-input", message)
            self.assertIn("regular file", message)

            parent_cache = root / "parent-cache"
            external_objects = root / "external-objects"
            external_path = (
                external_objects
                / "sha256"
                / entries["symlink-input"]["sha256"][:2]
                / entries["symlink-input"]["sha256"]
            )
            external_path.parent.mkdir(parents=True)
            external_path.write_bytes(payload)
            parent_cache.mkdir()
            (parent_cache / "objects").symlink_to(
                external_objects,
                target_is_directory=True,
            )
            one_input = lock_document(
                [("symlink-input", "ubuntu-24.04-amd64", payload)]
            )
            write_json(lock, one_input)
            with self.assertRaisesRegex(module.PackagingError, "regular file"):
                module.verify_cache(
                    targets, lock, parent_cache, "ubuntu-24.04-amd64"
                )

    def test_prepare_uses_one_configuration_snapshot(self) -> None:
        module = load_packaging_module()
        payload = b"ubuntu"
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            targets = root / "targets.json"
            lock = root / "inputs.lock.json"
            cache = root / "cache"
            output = root / "receipt.json"
            original_document = lock_document(
                [("ubuntu-package", "ubuntu-24.04-amd64", payload)]
            )
            original = (
                json.dumps(original_document, indent=2, sort_keys=True) + "\n"
            ).encode("utf-8")
            replacement_document = dict(original_document)
            replacement_document["schema"] = 99
            replacement = (
                json.dumps(replacement_document, indent=2, sort_keys=True) + "\n"
            ).encode("utf-8")
            write_json(targets, target_document())
            lock.write_bytes(original)
            object_path = module.cache_object_path(
                cache, original_document["inputs"][0]["sha256"]
            )
            object_path.parent.mkdir(parents=True)
            object_path.write_bytes(payload)
            changing_lock = ReplacedAfterParseLock(lock, original, replacement)
            isolated = module.NetworkObservation("linux", ("lo",), False, False)

            receipt = module.prepare_inputs(
                targets,
                changing_lock,
                cache,
                "ubuntu-24.04-amd64",
                output,
                observation=isolated,
            )

            self.assertEqual(
                receipt["lock_sha256"], hashlib.sha256(original).hexdigest()
            )

    def test_ipv6_reject_route_does_not_imply_connectivity(self) -> None:
        module = load_packaging_module()
        reject_route = (
            "0" * 32
            + " 00 "
            + "0" * 32
            + " 00 "
            + "0" * 32
            + " ffffffff 00000001 00000000 00200200 lo\n"
        )
        usable_route = (
            "0" * 32
            + " 00 "
            + "0" * 32
            + " 00 "
            + "0" * 32
            + " 00000064 00000000 00000000 00000003 eth0\n"
        )
        with tempfile.TemporaryDirectory() as temporary:
            route_path = Path(temporary) / "ipv6_route"
            route_path.write_text(reject_route, encoding="ascii")
            self.assertFalse(module._has_ipv6_default_route(route_path))
            route_path.write_text(reject_route + usable_route, encoding="ascii")
            self.assertTrue(module._has_ipv6_default_route(route_path))

    def test_prepare_requires_linux_loopback_only_and_preserves_prior_output(
        self,
    ) -> None:
        module = load_packaging_module()
        payload = b"ubuntu"
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            targets = root / "targets.json"
            lock = root / "inputs.lock.json"
            cache = root / "cache"
            output = root / "receipt.json"
            document = lock_document(
                [("ubuntu-package", "ubuntu-24.04-amd64", payload)]
            )
            write_json(targets, target_document())
            write_json(lock, document)
            object_path = module.cache_object_path(
                cache, document["inputs"][0]["sha256"]
            )
            object_path.parent.mkdir(parents=True)
            object_path.write_bytes(payload)

            isolated = module.NetworkObservation(
                platform="linux",
                interfaces=("lo",),
                ipv4_default_route=False,
                ipv6_default_route=False,
            )
            with mock.patch.object(
                module, "_download_input", side_effect=AssertionError("fetch called")
            ):
                receipt = module.prepare_inputs(
                    targets,
                    lock,
                    cache,
                    "ubuntu-24.04-amd64",
                    output,
                    observation=isolated,
                )
            self.assertEqual(receipt["offline"]["verified"], True)
            self.assertEqual(receipt["inputs"][0]["version"], "1.0.0")
            self.assertEqual(receipt["inputs"][0]["license"], "MIT")
            self.assertEqual(
                receipt["hardware_acceptance"]["status"], "not_run"
            )
            self.assertEqual(
                json.loads(output.read_text(encoding="utf-8")),
                receipt,
            )

            original = output.read_bytes()
            refused = [
                module.NetworkObservation(
                    "win32", ("lo",), False, False
                ),
                module.NetworkObservation(
                    "linux", ("eth0", "lo"), False, False
                ),
                module.NetworkObservation(
                    "linux", ("lo",), True, False
                ),
                module.NetworkObservation(
                    "linux", ("lo",), False, True
                ),
            ]
            for observation in refused:
                with self.subTest(observation=observation):
                    with self.assertRaises(module.PackagingError):
                        module.prepare_inputs(
                            targets,
                            lock,
                            cache,
                            "ubuntu-24.04-amd64",
                            output,
                            observation=observation,
                        )
                    self.assertEqual(output.read_bytes(), original)

    def test_prepare_receipts_never_infer_hardware_acceptance(self) -> None:
        module = load_packaging_module()
        payloads = {
            "rpi-debian13-arm64": b"pi",
            "ubuntu-24.04-amd64": b"ubuntu",
        }
        expected_checks = {
            target["id"]: target["hardware_acceptance"]
            for target in target_document()["targets"]
        }
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            targets = root / "targets.json"
            write_json(targets, target_document())
            isolated = module.NetworkObservation("linux", ("lo",), False, False)

            for target, payload in payloads.items():
                with self.subTest(target=target):
                    lock = root / f"{target}.lock.json"
                    cache = root / f"{target}.cache"
                    output = root / f"{target}.receipt.json"
                    document = lock_document(
                        [(f"{target}-input", target, payload)]
                    )
                    write_json(lock, document)
                    object_path = module.cache_object_path(
                        cache, document["inputs"][0]["sha256"]
                    )
                    object_path.parent.mkdir(parents=True)
                    object_path.write_bytes(payload)
                    receipt = module.prepare_inputs(
                        targets,
                        lock,
                        cache,
                        target,
                        output,
                        observation=isolated,
                    )
                    evidence = receipt["hardware_acceptance"]
                    self.assertEqual(evidence["status"], "not_run")
                    self.assertEqual(
                        evidence["required_checks"], expected_checks[target]
                    )
                    self.assertNotIn("passed", json.dumps(evidence).lower())

    def test_cli_and_wrappers_keep_fetch_and_offline_paths_separate(self) -> None:
        module = load_packaging_module()
        parser = module.build_parser()
        help_text = parser.format_help()
        self.assertIn("fetch", help_text)
        self.assertIn("prepare", help_text)
        self.assertNotIn("hardware-status", help_text)

        expected = [
            PACKAGING_ROOT / "fetch" / "fetch-all.ps1",
            PACKAGING_ROOT / "fetch" / "fetch-all.sh",
            PACKAGING_ROOT / "verify" / "verify-cache.ps1",
            PACKAGING_ROOT / "verify" / "verify-cache.sh",
            PACKAGING_ROOT / "build" / "prepare-offline.sh",
        ]
        for path in expected:
            self.assertTrue(path.is_file(), f"missing wrapper: {path}")

        shell_wrapper = (
            REPOSITORY_ROOT / "02_Tools" / "env" / "with-build-environment.sh"
        ).read_text(encoding="utf-8")
        power_shell_wrapper = (
            REPOSITORY_ROOT / "02_Tools" / "env" / "with-build-environment.ps1"
        ).read_text(encoding="utf-8")
        self.assertIn("package)", shell_wrapper)
        self.assertIn("--network none", shell_wrapper)
        self.assertIn("/packaging-cache:ro", shell_wrapper)
        self.assertIn(
            'command="sh ./02_Tools/packaging/build/prepare-offline.sh',
            shell_wrapper,
        )
        self.assertIn("'package'", power_shell_wrapper)

    def test_checked_in_lock_is_explicitly_incomplete(self) -> None:
        self.assertTrue(LOCK_PATH.is_file(), "the packaging lock is required")
        document = json.loads(LOCK_PATH.read_text(encoding="utf-8"))
        self.assertIs(document["complete"], False)
        self.assertIn("ubuntu-24.04-amd64", " ".join(document["incomplete_reasons"]))
        self.assertIn("rpi-debian13-arm64", " ".join(document["incomplete_reasons"]))
        module = load_packaging_module()
        registry = module.load_targets(TARGETS_PATH)
        with self.assertRaisesRegex(module.PackagingError, "incomplete"):
            module.load_lock(LOCK_PATH, registry, require_complete=True)

    def test_list_targets_cli_is_canonical_and_rejects_unknown_target(self) -> None:
        module = load_packaging_module()
        result = subprocess.run(
            [
                sys.executable,
                str(MODULE_PATH),
                "list-targets",
                "--targets",
                str(TARGETS_PATH),
                "--target",
                "ubuntu-24.04-amd64",
            ],
            cwd=REPOSITORY_ROOT,
            text=True,
            capture_output=True,
            check=False,
        )
        self.assertEqual(result.returncode, 0, result.stderr)
        parsed = json.loads(result.stdout)
        self.assertEqual(parsed["id"], "ubuntu-24.04-amd64")
        self.assertEqual(
            result.stdout,
            json.dumps(parsed, indent=2, sort_keys=True) + "\n",
        )

        self.assertEqual(
            module.main(
                [
                    "list-targets",
                    "--targets",
                    str(TARGETS_PATH),
                    "--target",
                    "ubuntu-25-amd64",
                ]
            ),
            2,
        )


if __name__ == "__main__":
    unittest.main()

# -----------------------------------------------------------------------------
# END OF FILE
# -----------------------------------------------------------------------------
