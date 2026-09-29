#!/usr/bin/env python3
# -*- coding: utf-8 -*-
# -----------------------------------------------------------------------------
# Copyright (c) 2026 DDP Engineering
# Licensed under the PolyForm Noncommercial License 1.0.0.
# See the repository LICENSE.txt for full license details.
# SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
# -----------------------------------------------------------------------------
"""
@file test_build_environment.py
@brief Verify the reproducible OCI development environment contracts.
@details Exercises definition, archive, restore, isolation, and evidence behavior
         without requiring a real container engine or network connection.
"""

from __future__ import annotations

import hashlib
import importlib.util
import json
import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from unittest import mock


REPOSITORY_ROOT = Path(__file__).resolve().parents[3]
TOOLS_ROOT = REPOSITORY_ROOT / "02_Tools"
CORE = TOOLS_ROOT / "scripts" / "build_environment.py"
LOCK = TOOLS_ROOT / "containers" / "build" / "build-environment.lock.json"
IGNORED_WSL_DISTRIBUTIONS = {"docker-desktop", "docker-desktop-data"}


def windows_wsl_distribution() -> str:
    """Select the requested or sole installed development distribution."""
    requested = os.environ.get("NILO_WSL_DISTRIBUTION", "").strip()
    discovered = subprocess.run(
        ["wsl.exe", "--list", "--quiet"],
        capture_output=True,
        text=True,
        check=False,
    )
    if discovered.returncode != 0:
        raise RuntimeError("installed WSL distributions could not be listed")
    distributions = [
        line.strip()
        for line in discovered.stdout.replace("\x00", "").splitlines()
        if line.strip() and line.strip().casefold() not in IGNORED_WSL_DISTRIBUTIONS
    ]
    if requested:
        selected = next(
            (name for name in distributions if name.casefold() == requested.casefold()),
            None,
        )
        if selected is None:
            raise RuntimeError(
                f"requested WSL distribution {requested!r} is not installed"
            )
        return selected
    if len(distributions) != 1:
        raise RuntimeError(
            "set NILO_WSL_DISTRIBUTION when the host has zero or multiple "
            "development distributions"
        )
    return distributions[0]


class BuildEnvironmentTests(unittest.TestCase):
    """Prove source-level contracts and deterministic archive mechanics."""

    @staticmethod
    def _pending_manifest() -> dict[str, object]:
        """Return a self-contained pre-export manifest fixture."""
        return {
            "schema": 1,
            "generation": "fixture-generation",
            "archive": {},
            "image": {
                "digest": "sha256:" + ("0" * 64),
                "reference": "nilo/build-environment:fixture",
            },
            "parts": [],
            "verification": {"status": "pending-round-trip"},
            "publication": {
                "gitea": {"status": "pending"},
                "backup": {"status": "pending"},
                "github": {"status": "absent"},
            },
        }

    def _run(self, *arguments: str) -> subprocess.CompletedProcess[str]:
        return subprocess.run(
            [sys.executable, str(CORE), *arguments],
            cwd=REPOSITORY_ROOT,
            text=True,
            capture_output=True,
            check=False,
        )

    def _run_linux_bootstrap(self, *arguments: str) -> subprocess.CompletedProcess[str]:
        script = TOOLS_ROOT / "bootstrap" / "bootstrap-build-environment.sh"
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            manifest = self._pending_manifest()
            manifest["parts"] = []
            manifest["image"]["digest"] = "sha256:" + ("0" * 64)
            manifest_path = root / "current.json"
            manifest_path.write_text(json.dumps(manifest), encoding="utf-8")
            engine = root / "engine.sh"
            engine.write_text(
                "printf '%s\\n' \"error: OCI engine 'podman' is unavailable\" >&2\nexit 1\n",
                encoding="utf-8",
                newline="\n",
            )
            os_release = root / "os-release"
            os_release.write_text(
                'ID=ubuntu\nVERSION_ID="24.04"\n', encoding="utf-8", newline="\n"
            )
            architecture = root / "architecture.sh"
            architecture.write_text(
                "printf '%s\\n' x86_64\n", encoding="utf-8", newline="\n"
            )
            systemd = root / "systemd.sh"
            systemd.write_text(
                "printf '%s\\n' running\n", encoding="utf-8", newline="\n"
            )
            injected = {
                "NILO_TEST_BUILD_ENVIRONMENT_MANIFEST": self._linux_path(manifest_path),
                "NILO_TEST_ENGINE_BOOTSTRAP": self._linux_path(engine),
                "NILO_TEST_OS_RELEASE_FILE": self._linux_path(os_release),
                "NILO_TEST_ARCHITECTURE_PROBE": self._linux_path(architecture),
                "NILO_TEST_SYSTEMD_PROBE": self._linux_path(systemd),
            }
            if os.name == "nt":
                command = [
                    "wsl.exe",
                    "-d",
                    windows_wsl_distribution(),
                    "--",
                    "env",
                    *(f"{name}={value}" for name, value in injected.items()),
                    "sh",
                    self._linux_path(script),
                    *arguments,
                ]
                environment = None
            else:
                command = ["sh", str(script), *arguments]
                environment = os.environ.copy()
                environment.update(injected)
            return subprocess.run(
                command,
                cwd=REPOSITORY_ROOT,
                env=environment,
                text=True,
                capture_output=True,
                check=False,
            )

    def _linux_path(self, path: Path) -> str:
        if os.name != "nt":
            return str(path)
        drive = path.drive.rstrip(":").lower()
        return f"/mnt/{drive}/{path.as_posix().split(':/', 1)[1]}"

    def _run_native_bootstrap(
        self,
        arguments: tuple[str, ...],
        injected_paths: dict[str, Path],
    ) -> subprocess.CompletedProcess[str]:
        script = TOOLS_ROOT / "bootstrap" / "bootstrap-build-environment.sh"
        injected = {
            name: self._linux_path(path) for name, path in injected_paths.items()
        }
        if os.name == "nt":
            assignments = [f"{name}={value}" for name, value in injected.items()]
            command = [
                "wsl.exe",
                "-d",
                windows_wsl_distribution(),
                "--",
                "env",
                *assignments,
                "sh",
                self._linux_path(script),
                *arguments,
            ]
        else:
            environment = os.environ.copy()
            environment.update(injected)
            command = ["sh", str(script), *arguments]
            return subprocess.run(
                command,
                cwd=REPOSITORY_ROOT,
                env=environment,
                text=True,
                capture_output=True,
                check=False,
            )
        return subprocess.run(
            command,
            cwd=REPOSITORY_ROOT,
            text=True,
            capture_output=True,
            check=False,
        )

    def test_definition_and_lock_are_complete_and_digest_pinned(self) -> None:
        result = self._run("validate-lock", "--lock", str(LOCK))
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        lock = json.loads(LOCK.read_text(encoding="utf-8"))
        self.assertEqual(lock["schema"], 1)
        self.assertRegex(lock["oci"]["build_frontend"], r"@sha256:[0-9a-f]{64}$")
        self.assertRegex(lock["base_image"]["reference"], r"@sha256:[0-9a-f]{64}$")
        names = {package["name"] for package in lock["apt_roots"]}
        for name in (
            "gcc",
            "binutils",
            "gcc-aarch64-linux-gnu",
            "binutils-aarch64-linux-gnu",
            "libc6-dev-arm64-cross",
            "libsigrok-dev",
            "git-lfs",
            "file",
            "libnss3",
            "libgbm1",
        ):
            self.assertIn(name, names)
        for package in lock["apt_roots"]:
            self.assertRegex(package["sha256"], r"^[0-9a-f]{64}$")
            self.assertTrue(package["version"])
            self.assertTrue(package["source"].startswith("https://"))
            self.assertTrue(package["license"])
        downloads = {item["name"]: item for item in lock["downloads"]}
        for name in (
            "go",
            "uv",
            "node",
            "playwright-lock",
            "playwright-browser-catalog",
        ):
            self.assertRegex(downloads[name]["sha256"], r"^[0-9a-f]{64}$")

        containerfile = (TOOLS_ROOT / "containers/build/Containerfile").read_text(
            encoding="utf-8"
        )
        self.assertRegex(containerfile.splitlines()[0], r"@sha256:[0-9a-f]{64}$")
        self.assertIn("ARG BASE_IMAGE", containerfile)
        self.assertIn("FROM ${BASE_IMAGE}", containerfile)
        self.assertNotRegex(containerfile, r"(?m)^FROM\s+[^$].*:[^@\s]+\s*$")
        self.assertIn("USER 65532:65532", containerfile)
        self.assertIn("HOME=/work/home", containerfile)
        dockerignore = (
            TOOLS_ROOT / "containers/build/Containerfile.dockerignore"
        ).read_text(encoding="utf-8")
        self.assertIn("**", dockerignore)
        self.assertIn("!05_Software/", dockerignore)

    def test_reconstruction_lock_is_complete_and_consumed_offline(self) -> None:
        result = self._run("validate-reconstruction", "--lock", str(LOCK))
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        lock = json.loads(LOCK.read_text(encoding="utf-8"))
        reconstruction = lock["reconstruction"]
        self.assertEqual(reconstruction["status"], "complete")
        self.assertNotIn("missing", reconstruction)
        manifest_path = REPOSITORY_ROOT / reconstruction["manifest"]
        self.assertEqual(
            hashlib.sha256(manifest_path.read_bytes()).hexdigest(),
            reconstruction["manifest_sha256"],
        )
        inputs = json.loads(manifest_path.read_text(encoding="utf-8"))["inputs"]
        self.assertEqual(len(inputs), reconstruction["input_count"])
        self.assertGreater(len(inputs), len(lock["apt_roots"]))
        self.assertEqual(len({item["cache_name"] for item in inputs}), len(inputs))
        self.assertTrue(
            {
                "apt",
                "go",
                "uv",
                "node",
                "python",
                "npm",
                "go-modules",
                "python-packages",
                "chromium",
            }.issubset({item["kind"] for item in inputs})
        )
        for item in inputs:
            self.assertRegex(item["sha256"], r"^[0-9a-f]{64}$")
            self.assertGreater(item["size"], 0)
            self.assertTrue(item["source"])
            self.assertTrue(item["license"])
            self.assertTrue(item["consumer"])
        builder = (TOOLS_ROOT / "scripts/build-build-environment.sh").read_text(
            encoding="utf-8"
        )
        self.assertLess(
            builder.index("validate-reconstruction"),
            builder.index('command -v "$engine"'),
        )
        self.assertIn('--build-context "nilo_inputs=', builder)
        self.assertIn("--network=none", builder)
        containerfile = (TOOLS_ROOT / "containers/build/Containerfile").read_text(
            encoding="utf-8"
        )
        self.assertIn("COPY --from=nilo_inputs", containerfile)
        for forbidden in (
            "apt-get update",
            "curl --fail",
            "uv python install",
            "npx playwright install",
        ):
            self.assertNotIn(forbidden, containerfile)
        for offline_contract in (
            "dpkg --unpack /opt/nilo/inputs/apt/*.deb",
            "npm ci --offline",
            "GOPROXY=file:///opt/nilo/inputs/go-proxy",
            "GOSUMDB=off",
            "locked_build_inputs.py prepare-python",
        ):
            self.assertIn(offline_contract, containerfile)

    def test_materialized_input_directory_rejects_missing_and_tampered_payloads(
        self,
    ) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            inputs = root / "inputs"
            inputs.mkdir()
            payload = inputs / "payload.bin"
            payload.write_bytes(b"locked payload")
            manifest = root / "lock.json"
            manifest.write_text(
                json.dumps(
                    {
                        "reconstruction": {
                            "status": "complete",
                            "inputs": [
                                {
                                    "name": "fixture",
                                    "version": "1",
                                    "kind": "fixture",
                                    "cache_name": "payload.bin",
                                    "sha256": hashlib.sha256(
                                        b"locked payload"
                                    ).hexdigest(),
                                    "size": len(b"locked payload"),
                                    "source": "fixture://payload",
                                    "license": "test-only",
                                    "consumer": "fixture consumer",
                                }
                            ],
                        },
                    }
                ),
                encoding="utf-8",
            )
            valid = self._run(
                "verify-materialized-inputs",
                "--lock",
                str(manifest),
                "--input-dir",
                str(inputs),
            )
            self.assertEqual(valid.returncode, 0, valid.stdout + valid.stderr)
            payload.write_bytes(b"locked payloae")
            tampered = self._run(
                "verify-materialized-inputs",
                "--lock",
                str(manifest),
                "--input-dir",
                str(inputs),
            )
            self.assertNotEqual(tampered.returncode, 0)
            self.assertIn("SHA-256 mismatch", tampered.stderr)
            payload.unlink()
            missing = self._run(
                "verify-materialized-inputs",
                "--lock",
                str(manifest),
                "--input-dir",
                str(inputs),
            )
            self.assertNotEqual(missing.returncode, 0)
            self.assertIn("missing materialized input", missing.stderr)

    def test_inventory_only_verification_does_not_require_skopeo(self) -> None:
        verifier = (TOOLS_ROOT / "scripts/verify-build-environment.sh").read_text(
            encoding="utf-8"
        )
        inventory_exit = verifier.index('[ "$inventory_only" -eq 0 ] || exit 0')
        skopeo_gate = verifier.index("command -v skopeo")
        self.assertGreater(skopeo_gate, inventory_exit)

    def test_full_verification_seeds_the_offline_runtime_cache(self) -> None:
        verifier = (TOOLS_ROOT / "scripts/verify-build-environment.sh").read_text(
            encoding="utf-8"
        )
        seed_copy = verifier.index("cp -R /opt/nilo/cache-seed/. /work/runtime-cache/")
        offline_tests = verifier.index(
            "./02_Tools/tests/build-environment/verify-offline.sh"
        )
        self.assertLess(seed_copy, offline_tests)

    def test_full_verification_uses_the_canonical_offline_suite(self) -> None:
        verifier = (TOOLS_ROOT / "scripts/verify-build-environment.sh").read_text(
            encoding="utf-8"
        )
        offline_suite = (
            TOOLS_ROOT / "tests/build-environment/verify-offline.sh"
        ).read_text(encoding="utf-8")
        self.assertIn(
            "./02_Tools/tests/build-environment/verify-offline.sh",
            verifier,
        )
        self.assertIn("cd 05_Software", offline_suite)
        self.assertNotIn("python 02_Tools/scripts/aegis_test.py lint", offline_suite)
        self.assertIn("GOARCH=arm64 go test -exec=/bin/true", offline_suite)

    def test_offline_python_environments_install_only_locked_local_inputs(self) -> None:
        containerfile = (TOOLS_ROOT / "containers/build/Containerfile").read_text(
            encoding="utf-8"
        )
        preparer = (
            TOOLS_ROOT / "tests/build-environment/prepare-python-environments.sh"
        ).read_text(encoding="utf-8")
        build_all = (TOOLS_ROOT / "tests/build-environment/build-all.sh").read_text(
            encoding="utf-8"
        )
        self.assertIn("UV_PROJECT_ENVIRONMENT=.venv-linux-x86_64", containerfile)
        self.assertIn("prepare-python-environments.sh", build_all)
        self.assertIn("locked_build_inputs.py prepare-python", preparer)
        self.assertNotIn("prepare-serial-dependency.py", preparer)
        self.assertIn("locked_build_inputs.py verify", build_all)
        self.assertNotIn("uv sync", build_all)

    def test_archive_round_trip_and_corruption_fail_closed(self) -> None:
        exporter = (TOOLS_ROOT / "scripts/export-build-environment.sh").read_text(
            encoding="utf-8"
        )
        restorer = (TOOLS_ROOT / "scripts/restore-build-environment.sh").read_text(
            encoding="utf-8"
        )
        core_source = CORE.read_text(encoding="utf-8")
        self.assertIn("skopeo copy", exporter)
        self.assertIn("oci-archive:", exporter)
        self.assertIn("skopeo inspect --format '{{.Digest}}'", exporter + restorer)
        self.assertIn("skopeo copy --preserve-digests", exporter + restorer)
        self.assertNotIn('"$engine" save', exporter)
        self.assertNotIn("path.read_bytes()", core_source)
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            lock_generation = json.loads(LOCK.read_text(encoding="utf-8"))["generation"]
            source = root / "image.oci.tar"
            source.write_bytes(bytes(range(251)) * 19)
            output = root / "parts"
            manifest = output / "current.json"
            export = self._run(
                "split-archive",
                "--archive",
                str(source),
                "--output",
                str(output),
                "--generation",
                lock_generation,
                "--part-size",
                "1024",
                "--image-digest",
                "sha256:" + "1" * 64,
                "--lock",
                str(LOCK),
                "--source-base-commit",
                "a" * 40,
                "--uncompressed-size",
                "4096",
                "--source-tree-sha256",
                "b" * 64,
                "--source-status",
                "dirty",
            )
            self.assertEqual(export.returncode, 0, export.stdout + export.stderr)
            data = json.loads(manifest.read_text(encoding="utf-8"))
            self.assertEqual(data["verification"]["status"], "pending-round-trip")
            self.assertEqual(
                data["creation"]["supported_host"],
                "Ubuntu 24.04 linux/amd64 (native or WSL2)",
            )
            self.assertIsNone(data["verification"]["verified_at"])
            self.assertEqual(data["source"]["base_commit"], "a" * 40)
            self.assertEqual(data["source"]["tree_sha256"], "b" * 64)
            self.assertEqual(data["source"]["status"], "dirty")
            self.assertEqual(
                data["source"]["lock_manifest"],
                "02_Tools/containers/build/build-environment.lock.json",
            )
            self.assertEqual(
                data["artifacts"]["sbom"], f"metadata/{lock_generation}/sbom.spdx.json"
            )
            self.assertGreater(len(data["parts"]), 1)
            self.assertEqual(
                [part["ordinal"] for part in data["parts"]],
                list(range(len(data["parts"]))),
            )
            for part in data["parts"]:
                self.assertEqual(part["lfs_oid"], "sha256:" + part["sha256"])
                self.assertLessEqual(part["size"], 2_000_000_000)

            joined = root / "joined.oci.tar"
            restore = self._run(
                "join-archive",
                "--manifest",
                str(manifest),
                "--archive-path",
                str(output),
                "--output",
                str(joined),
            )
            self.assertEqual(restore.returncode, 0, restore.stdout + restore.stderr)
            self.assertEqual(joined.read_bytes(), source.read_bytes())

            evidence = output / "metadata" / lock_generation / "round-trip.log"
            evidence.parent.mkdir(parents=True)
            evidence.write_text(
                "restore and offline verification passed\n", encoding="utf-8"
            )
            promoted = self._run(
                "record-round-trip",
                "--manifest",
                str(manifest),
                "--evidence",
                str(evidence),
                "--command",
                "restore fixture",
                "--command",
                "offline verify fixture",
            )
            self.assertEqual(promoted.returncode, 0, promoted.stdout + promoted.stderr)
            promoted_data = json.loads(manifest.read_text(encoding="utf-8"))
            self.assertEqual(
                promoted_data["verification"]["status"], "local-round-trip-complete"
            )
            self.assertEqual(
                promoted_data["verification"]["evidence"],
                f"metadata/{lock_generation}/round-trip.log",
            )

            first = output / data["parts"][0]["filename"]
            payload = bytearray(first.read_bytes())
            payload[0] ^= 1
            first.write_bytes(payload)
            corrupt = self._run(
                "join-archive",
                "--manifest",
                str(manifest),
                "--archive-path",
                str(output),
                "--output",
                str(joined),
            )
            self.assertNotEqual(corrupt.returncode, 0)
            self.assertIn("SHA-256 mismatch", corrupt.stderr)

            oversized = self._run(
                "split-archive",
                "--archive",
                str(source),
                "--output",
                str(output),
                "--generation",
                "too-large",
                "--part-size",
                "2000000001",
                "--image-digest",
                "sha256:" + "1" * 64,
                "--lock",
                str(LOCK),
                "--source-base-commit",
                "a" * 40,
                "--uncompressed-size",
                "4096",
                "--source-tree-sha256",
                "b" * 64,
                "--source-status",
                "dirty",
            )
            self.assertNotEqual(oversized.returncode, 0)
            self.assertIn("2000000000", oversized.stderr)

            wrong_generation = self._run(
                "split-archive",
                "--archive",
                str(source),
                "--output",
                str(root / "wrong"),
                "--generation",
                "not-the-lock-generation",
                "--part-size",
                "1024",
                "--image-digest",
                "sha256:" + "1" * 64,
                "--lock",
                str(LOCK),
                "--source-base-commit",
                "a" * 40,
                "--uncompressed-size",
                "4096",
                "--source-tree-sha256",
                "b" * 64,
                "--source-status",
                "dirty",
            )
            self.assertNotEqual(wrong_generation.returncode, 0)
            self.assertIn("does not match lock generation", wrong_generation.stderr)

    def test_round_trip_status_requires_an_explicit_successful_transition(self) -> None:
        source = (TOOLS_ROOT / "scripts/export-build-environment.sh").read_text(
            encoding="utf-8"
        )
        verifier = TOOLS_ROOT / "tests/build-environment/verify-archive-round-trip.sh"
        self.assertNotIn("local-round-trip-complete", source)
        self.assertTrue(verifier.is_file())
        verifier_source = verifier.read_text(encoding="utf-8")
        self.assertIn("restore-build-environment.sh", verifier_source)
        self.assertIn("verify-build-environment.sh", verifier_source)
        self.assertIn("record-round-trip", verifier_source)

    def test_pointer_missing_extra_and_reordered_parts_are_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            parts = root / "parts"
            parts.mkdir()
            pointer = (
                b"version https://git-lfs.github.com/spec/v1\noid sha256:"
                + b"a" * 64
                + b"\nsize 10\n"
            )
            (parts / "x.part000").write_bytes(pointer)
            manifest = {
                "schema": 1,
                "generation": "test",
                "archive": {
                    "filename": "x",
                    "size": 10,
                    "sha256": "b" * 64,
                    "media_type": "application/vnd.oci.image.archive.v1+tar",
                    "compression": "gzip",
                },
                "image": {"digest": "sha256:" + "c" * 64, "platform": "linux/amd64"},
                "parts": [
                    {
                        "ordinal": 0,
                        "filename": "x.part000",
                        "size": len(pointer),
                        "sha256": hashlib.sha256(pointer).hexdigest(),
                        "lfs_oid": "sha256:" + hashlib.sha256(pointer).hexdigest(),
                    }
                ],
                "source": {
                    "base_commit": "a" * 40,
                    "tree_sha256": "b" * 64,
                    "status": "clean",
                },
                "verification": {
                    "status": "pending-round-trip",
                    "verified_at": None,
                    "commands": [],
                    "evidence": None,
                },
            }
            manifest_path = root / "current.json"
            manifest_path.write_text(json.dumps(manifest), encoding="utf-8")
            result = self._run(
                "inspect-parts",
                "--manifest",
                str(manifest_path),
                "--archive-path",
                str(parts),
            )
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("Git LFS pointer", result.stderr)
            self.assertIn("x.part000", result.stderr)
            self.assertIn("fetch the named LFS objects", result.stderr)
            self.assertIn("Recovery path 3", result.stderr)
            self.assertIn(
                "explicitly rebuild from pinned upstream inputs", result.stderr
            )

            (parts / "x.part000").unlink()
            missing = self._run(
                "inspect-parts",
                "--manifest",
                str(manifest_path),
                "--archive-path",
                str(parts),
            )
            self.assertNotEqual(missing.returncode, 0)
            self.assertIn("missing", missing.stderr.lower())

            (parts / "unexpected.part999").write_bytes(b"extra")
            extra = self._run(
                "inspect-parts",
                "--manifest",
                str(manifest_path),
                "--archive-path",
                str(parts),
            )
            self.assertNotEqual(extra.returncode, 0)
            self.assertIn("unexpected", extra.stderr.lower())

            manifest["parts"] = [
                {
                    "ordinal": 1,
                    "filename": "x.part001",
                    "size": 1,
                    "sha256": "d" * 64,
                    "lfs_oid": "sha256:" + "d" * 64,
                },
                {
                    "ordinal": 0,
                    "filename": "x.part000",
                    "size": 1,
                    "sha256": "e" * 64,
                    "lfs_oid": "sha256:" + "e" * 64,
                },
            ]
            manifest_path.write_text(json.dumps(manifest), encoding="utf-8")
            reordered = self._run("validate-manifest", "--manifest", str(manifest_path))
            self.assertNotEqual(reordered.returncode, 0)
            self.assertIn("ordinal", reordered.stderr.lower())

    def test_setup_has_no_forge_acquisition_or_persistent_environment_change(
        self,
    ) -> None:
        paths = [
            TOOLS_ROOT / "bootstrap/bootstrap-build-environment.ps1",
            TOOLS_ROOT / "bootstrap/bootstrap-build-environment.sh",
            TOOLS_ROOT / "bootstrap/bootstrap-container-engine.sh",
            TOOLS_ROOT / "bootstrap/setup-build-environment-image.sh",
            TOOLS_ROOT / "scripts/restore-build-environment.sh",
        ]
        source = "\n".join(path.read_text(encoding="utf-8") for path in paths).lower()
        forbidden = (
            "git fetch",
            "git pull",
            "git lfs fetch",
            "git lfs pull",
            "git remote",
            "setx",
            "setenvironmentvariable",
            "docker desktop",
        )
        for phrase in forbidden:
            self.assertNotIn(phrase, source)
        self.assertNotRegex(source, r"https?://(?:www\.)?(?:github|gitea)")
        self.assertIn("rebuild-from-pinned-inputs", source)
        self.assertIn("archive-path", source)
        host_lock = json.loads(
            (
                TOOLS_ROOT / "archives/wsl-host-bootstrap/host-bootstrap.lock.json"
            ).read_text(encoding="utf-8")
        )
        self.assertTrue(
            host_lock["ubuntu_snapshot"].startswith("https://snapshot.ubuntu.com/")
        )
        self.assertRegex(
            host_lock["docker_repository"]["key_sha256"], r"^[0-9a-f]{64}$"
        )
        pinned = {item["name"] for item in host_lock["root_package_pins"]}
        self.assertTrue({"docker-ce", "podman", "git-lfs", "skopeo"}.issubset(pinned))
        for item in host_lock["root_package_pins"]:
            self.assertRegex(item["sha256"], r"^[0-9a-f]{64}$")

        linux_setup = (
            TOOLS_ROOT / "bootstrap/setup-build-environment-image.sh"
        ).read_text(encoding="utf-8")
        inspect_position = linux_setup.index("inspect-parts")
        restore_position = linux_setup.index("restore-build-environment.sh")
        rebuild_position = linux_setup.index('if [ "$rebuild" -eq 1 ]')
        self.assertLess(inspect_position, restore_position)
        self.assertLess(restore_position, rebuild_position)
        self.assertIn(
            "continuing with explicitly selected pinned reconstruction", linux_setup
        )

    def test_host_bootstrap_preserves_an_existing_selected_engine(self) -> None:
        installer = (TOOLS_ROOT / "bootstrap/bootstrap-container-engine.sh").read_text(
            encoding="utf-8"
        )
        self.assertIn("engine_preinstalled=0", installer)
        self.assertIn("engine_preinstalled=1", installer)
        self.assertIn('if [ "$engine_preinstalled" -eq 1 ]', installer)
        self.assertIn(
            'd["common"] if preserve else d["common"]+d["engines"][selected]', installer
        )
        self.assertIn("preserving existing selected engine", installer)

    def test_host_archive_uses_apts_selected_dependency_closure(self) -> None:
        generator = (TOOLS_ROOT / "scripts/build-host-bootstrap.sh").read_text(
            encoding="utf-8"
        )
        self.assertNotIn("apt_cache depends", generator)
        self.assertNotIn("packages=$(apt_cache", generator)
        self.assertIn("Dir::State::status=$staging/apt/status", generator)
        self.assertIn("--download-only --no-install-recommends", generator)
        self.assertIn(
            "--download-only --no-install-recommends --reinstall -y", generator
        )
        self.assertIn("Dir::Cache::archives=$staging/packages/$group", generator)

    def test_host_archive_reads_raw_deb_control_field_values(self) -> None:
        generator = (TOOLS_ROOT / "scripts/build-host-bootstrap.sh").read_text(
            encoding="utf-8"
        )
        self.assertIn("def control_field(path, field):", generator)
        self.assertIn('["dpkg-deb","-f",str(path),field]', generator)
        self.assertNotIn('"Package","Version","Architecture"', generator)

    def test_native_ubuntu_entry_point_uses_shared_boundaries(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            os_release = root / "os-release"
            architecture = root / "architecture.sh"
            systemd = root / "systemd.sh"
            engine = root / "engine.sh"
            image = root / "image.sh"
            engine_log = root / "engine.log"
            image_log = root / "image.log"
            os_release.write_text('ID=ubuntu\nVERSION_ID="24.04"\n', encoding="utf-8")
            architecture.write_text(
                "printf '%s\\n' x86_64\n", encoding="utf-8", newline="\n"
            )
            systemd.write_text(
                "printf '%s\\n' degraded\n", encoding="utf-8", newline="\n"
            )
            engine.write_text(
                f"printf '%s\\n' \"$@\" > '{self._linux_path(engine_log)}'\n",
                encoding="utf-8",
                newline="\n",
            )
            image.write_text(
                f"printf '%s\\n' \"$@\" > '{self._linux_path(image_log)}'\n",
                encoding="utf-8",
                newline="\n",
            )
            result = self._run_native_bootstrap(
                (
                    "--engine",
                    "podman",
                    "--install-engine",
                    "--engine-archive-path",
                    "/verified-host-packages",
                    "--archive-path",
                    "/verified-image-parts",
                    "--offline",
                ),
                {
                    "NILO_TEST_OS_RELEASE_FILE": os_release,
                    "NILO_TEST_ARCHITECTURE_PROBE": architecture,
                    "NILO_TEST_SYSTEMD_PROBE": systemd,
                    "NILO_TEST_ENGINE_BOOTSTRAP": engine,
                    "NILO_TEST_IMAGE_SETUP": image,
                },
            )
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            self.assertEqual(
                engine_log.read_text(encoding="utf-8").splitlines(),
                [
                    "--engine",
                    "podman",
                    "--install",
                    "--archive-path",
                    "/verified-host-packages",
                ],
            )
            self.assertEqual(
                image_log.read_text(encoding="utf-8").splitlines(),
                [
                    "--engine",
                    "podman",
                    "--engine-verifier",
                    self._linux_path(engine),
                    "--archive-path",
                    "/verified-image-parts",
                    "--offline",
                ],
            )
            self.assertIn("Native Ubuntu build host verified", result.stdout)

    def test_native_ubuntu_rejects_unsupported_hosts_before_engine(self) -> None:
        cases = (
            ('ID=debian\nVERSION_ID="12"\n', "x86_64", "running"),
            ('ID=ubuntu\nVERSION_ID="24.04"\n', "aarch64", "running"),
            ('ID=ubuntu\nVERSION_ID="24.04"\n', "x86_64", "offline"),
        )
        for identity, architecture_value, systemd_value in cases:
            with self.subTest(
                identity=identity,
                architecture=architecture_value,
                systemd=systemd_value,
            ):
                with tempfile.TemporaryDirectory() as directory:
                    root = Path(directory)
                    paths = {
                        "NILO_TEST_OS_RELEASE_FILE": root / "os-release",
                        "NILO_TEST_ARCHITECTURE_PROBE": root / "architecture.sh",
                        "NILO_TEST_SYSTEMD_PROBE": root / "systemd.sh",
                        "NILO_TEST_ENGINE_BOOTSTRAP": root / "engine.sh",
                        "NILO_TEST_IMAGE_SETUP": root / "image.sh",
                    }
                    paths["NILO_TEST_OS_RELEASE_FILE"].write_text(
                        identity, encoding="utf-8"
                    )
                    paths["NILO_TEST_ARCHITECTURE_PROBE"].write_text(
                        f"printf '%s\\n' {architecture_value}\n",
                        encoding="utf-8",
                        newline="\n",
                    )
                    paths["NILO_TEST_SYSTEMD_PROBE"].write_text(
                        f"printf '%s\\n' {systemd_value}\n",
                        encoding="utf-8",
                        newline="\n",
                    )
                    paths["NILO_TEST_ENGINE_BOOTSTRAP"].write_text(
                        "exit 91\n", encoding="utf-8", newline="\n"
                    )
                    paths["NILO_TEST_IMAGE_SETUP"].write_text(
                        "exit 92\n", encoding="utf-8", newline="\n"
                    )
                    result = self._run_native_bootstrap(("--engine", "docker"), paths)
                    self.assertNotEqual(result.returncode, 0)
                    self.assertNotIn(result.returncode, (91, 92))
                    self.assertIn("Ubuntu 24.04 x86_64 with systemd", result.stderr)

    def test_windows_and_native_entry_points_share_linux_helpers(self) -> None:
        powershell = (
            TOOLS_ROOT / "bootstrap/bootstrap-build-environment.ps1"
        ).read_text(encoding="utf-8")
        native = (TOOLS_ROOT / "bootstrap/bootstrap-build-environment.sh").read_text(
            encoding="utf-8"
        )
        for helper in (
            "bootstrap-container-engine.sh",
            "setup-build-environment-image.sh",
        ):
            self.assertIn(helper, powershell)
            self.assertIn(helper, native)
        compatibility = (
            TOOLS_ROOT / "bootstrap/bootstrap-container-engine-wsl.sh"
        ).read_text(encoding="utf-8")
        self.assertIn("bootstrap-container-engine.sh", compatibility)
        generator = (TOOLS_ROOT / "scripts/build-host-bootstrap.sh").read_text(
            encoding="utf-8"
        )
        self.assertIn("apt_get", generator)
        self.assertIn("host-bootstrap.lock.json", generator)

    def test_pending_archive_reports_recovery_before_missing_tool_errors(self) -> None:
        result = self._run_linux_bootstrap("--engine", "podman", "--offline")
        output = result.stdout + result.stderr
        self.assertNotEqual(result.returncode, 0)
        for recovery_path in ("Recovery path 1", "Recovery path 2", "Recovery path 3"):
            self.assertIn(recovery_path, output)
        self.assertNotIn("OCI engine 'podman' is unavailable", output)
        self.assertNotIn("skopeo is unavailable", output)

    def test_explicit_rebuild_reports_selected_action_then_missing_tool(self) -> None:
        result = self._run_linux_bootstrap(
            "--engine", "podman", "--rebuild-from-pinned-inputs"
        )
        output = result.stdout + result.stderr
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("Explicit networked reconstruction selected", output)
        self.assertRegex(
            output,
            r"OCI engine 'podman' is unavailable|skopeo is unavailable",
        )
        self.assertNotIn("current generation is pending export", output)

    def test_normal_wrapper_is_offline_unprivileged_and_digest_checked(self) -> None:
        source = "\n".join(
            (TOOLS_ROOT / path).read_text(encoding="utf-8").lower()
            for path in (
                "env/with-build-environment.ps1",
                "env/with-build-environment.sh",
            )
        )
        for phrase in (
            "--network",
            "none",
            "--cap-drop",
            "all",
            "no-new-privileges",
            "/workspace",
            "current.json",
            "image digest",
            "uid",
            "gid",
            "type=volume",
            "/opt/nilo/cache",
            "/outputs",
        ):
            self.assertIn(phrase, source)
        self.assertNotIn("docker.sock", source)
        self.assertNotIn("--privileged", source)
        self.assertNotIn("--network host", source)
        self.assertNotIn("target=/opt/nilo/cache", source)
        self.assertIn("/opt/nilo/cache-seed", source)
        self.assertIn("target=/work/runtime-cache", source)
        self.assertIn(".nilo-seed-", source)
        topology = (REPOSITORY_ROOT / "05_Software/frontend/e2e/topology.ts").read_text(
            encoding="utf-8"
        )
        self.assertIn("requireApplianceStage(process.env", topology)
        self.assertNotIn("process.platform !== 'win32'", topology)

    def test_source_identity_accepts_dirty_content_and_ignores_outputs(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            relevant = root / "05_Software" / "example.txt"
            relevant.parent.mkdir(parents=True)
            relevant.write_text("clean\n", encoding="utf-8")
            (root / ".gitignore").write_text("/02_Tools/cache/\n", encoding="utf-8")
            subprocess.run(["git", "init", "-q"], cwd=root, check=True)
            subprocess.run(
                ["git", "add", ".gitignore", "05_Software/example.txt"],
                cwd=root,
                check=True,
            )
            subprocess.run(
                [
                    "git",
                    "-c",
                    "user.name=NILO Test",
                    "-c",
                    "user.email=nilo@example.invalid",
                    "commit",
                    "-qm",
                    "fixture",
                ],
                cwd=root,
                check=True,
            )
            clean = self._run("verify-source-state", "--repository-root", str(root))
            self.assertEqual(clean.returncode, 0, clean.stdout + clean.stderr)
            clean_identity = json.loads(clean.stdout)
            self.assertEqual(clean_identity["status"], "clean")
            self.assertRegex(clean_identity["base_commit"], r"^[0-9a-f]{40}$")
            self.assertRegex(clean_identity["tree_sha256"], r"^[0-9a-f]{64}$")

            relevant.write_text("dirty\n", encoding="utf-8")
            dirty = self._run("verify-source-state", "--repository-root", str(root))
            self.assertEqual(dirty.returncode, 0, dirty.stdout + dirty.stderr)
            dirty_identity = json.loads(dirty.stdout)
            self.assertEqual(dirty_identity["status"], "dirty")
            self.assertNotEqual(
                dirty_identity["tree_sha256"], clean_identity["tree_sha256"]
            )

            ignored = root / "02_Tools" / "cache" / "generated.bin"
            ignored.parent.mkdir(parents=True)
            ignored.write_bytes(b"ignored output")
            after_ignored = json.loads(
                self._run("verify-source-state", "--repository-root", str(root)).stdout
            )
            self.assertEqual(
                after_ignored["tree_sha256"], dirty_identity["tree_sha256"]
            )
            archive_output = root / "02_Tools" / "archives" / "generated.part000"
            archive_output.parent.mkdir(parents=True)
            archive_output.write_bytes(b"generated archive output")
            after_archive = json.loads(
                self._run("verify-source-state", "--repository-root", str(root)).stdout
            )
            self.assertEqual(
                after_archive["tree_sha256"], dirty_identity["tree_sha256"]
            )

            untracked = root / "02_Tools" / "new-input.txt"
            untracked.parent.mkdir(parents=True, exist_ok=True)
            untracked.write_text("candidate input\n", encoding="utf-8")
            after_untracked = json.loads(
                self._run("verify-source-state", "--repository-root", str(root)).stdout
            )
            self.assertNotEqual(
                after_untracked["tree_sha256"], dirty_identity["tree_sha256"]
            )

    def test_candidate_identity_can_be_confirmed_after_approval_commit(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            relevant = root / "05_Software" / "example.txt"
            relevant.parent.mkdir(parents=True)
            relevant.write_text("base\n", encoding="utf-8")
            subprocess.run(["git", "init", "-q"], cwd=root, check=True)
            subprocess.run(
                ["git", "add", "05_Software/example.txt"], cwd=root, check=True
            )
            commit = [
                "git",
                "-c",
                "user.name=NILO Test",
                "-c",
                "user.email=nilo@example.invalid",
                "commit",
                "-qm",
            ]
            subprocess.run([*commit, "base"], cwd=root, check=True)
            relevant.write_text("approved candidate\n", encoding="utf-8")
            candidate = json.loads(
                self._run("verify-source-state", "--repository-root", str(root)).stdout
            )
            rejected = self._run(
                "verify-committed-source",
                "--repository-root",
                str(root),
                "--expected-tree-sha256",
                candidate["tree_sha256"],
            )
            self.assertNotEqual(rejected.returncode, 0)
            self.assertIn("not clean", rejected.stderr)
            subprocess.run(
                ["git", "add", "05_Software/example.txt"], cwd=root, check=True
            )
            subprocess.run([*commit, "approved"], cwd=root, check=True)
            accepted = self._run(
                "verify-committed-source",
                "--repository-root",
                str(root),
                "--expected-tree-sha256",
                candidate["tree_sha256"],
            )
            self.assertEqual(accepted.returncode, 0, accepted.stdout + accepted.stderr)

    def test_untracked_shell_identity_matches_later_executable_index_mode(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            script = root / "02_Tools" / "bootstrap" / "entry.sh"
            script.parent.mkdir(parents=True)
            script.write_text(
                "#!/usr/bin/env sh\nexit 0\n", encoding="utf-8", newline="\n"
            )
            (root / ".gitignore").write_text("\n", encoding="utf-8")
            subprocess.run(["git", "init", "-q"], cwd=root, check=True)
            subprocess.run(
                ["git", "config", "core.filemode", "false"], cwd=root, check=True
            )
            subprocess.run(["git", "add", ".gitignore"], cwd=root, check=True)
            commit = [
                "git",
                "-c",
                "user.name=NILO Test",
                "-c",
                "user.email=nilo@example.invalid",
                "commit",
                "-qm",
            ]
            subprocess.run([*commit, "base"], cwd=root, check=True)

            candidate = json.loads(
                self._run("verify-source-state", "--repository-root", str(root)).stdout
            )
            subprocess.run(
                ["git", "add", "--chmod=+x", "--", "02_Tools/bootstrap/entry.sh"],
                cwd=root,
                check=True,
            )
            indexed = json.loads(
                self._run("verify-source-state", "--repository-root", str(root)).stdout
            )
            self.assertEqual(candidate["tree_sha256"], indexed["tree_sha256"])
            mode = subprocess.run(
                ["git", "ls-files", "--stage", "--", "02_Tools/bootstrap/entry.sh"],
                cwd=root,
                text=True,
                capture_output=True,
                check=True,
            ).stdout.split()[0]
            self.assertEqual(mode, "100755")
            subprocess.run([*commit, "approved shell"], cwd=root, check=True)
            accepted = self._run(
                "verify-committed-source",
                "--repository-root",
                str(root),
                "--expected-tree-sha256",
                candidate["tree_sha256"],
            )
            self.assertEqual(accepted.returncode, 0, accepted.stdout + accepted.stderr)

    def test_stale_image_source_identity_is_rejected(self) -> None:
        stale = self._run(
            "verify-image-source",
            "--current-tree-sha256",
            "a" * 64,
            "--image-tree-sha256",
            "b" * 64,
        )
        self.assertNotEqual(stale.returncode, 0)
        self.assertIn("stale image", stale.stderr)
        current = self._run(
            "verify-image-source",
            "--current-tree-sha256",
            "a" * 64,
            "--image-tree-sha256",
            "a" * 64,
        )
        self.assertEqual(current.returncode, 0, current.stdout + current.stderr)
        exporter = (TOOLS_ROOT / "scripts/export-build-environment.sh").read_text(
            encoding="utf-8"
        )
        self.assertLess(
            exporter.index("verify-image-source"),
            exporter.index("collect-build-environment-metadata.sh"),
        )

    def test_replica_preflight_rejects_partial_github_and_missing_gitea_evidence(
        self,
    ) -> None:
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "manifest.json"
            base = self._pending_manifest()
            base["publication"]["gitea"]["status"] = "pending"
            path.write_text(json.dumps(base), encoding="utf-8")
            pending = self._run("verify-replicas", "--manifest", str(path))
            self.assertNotEqual(pending.returncode, 0)
            self.assertIn("Gitea", pending.stderr)

            base["publication"]["github"]["status"] = "partial"
            path.write_text(json.dumps(base), encoding="utf-8")
            partial = self._run("validate-manifest", "--manifest", str(path))
            self.assertNotEqual(partial.returncode, 0)
            self.assertIn("GitHub", partial.stderr)

    def test_publication_preflight_delegates_to_local_part_integrity_check(
        self,
    ) -> None:
        source = (TOOLS_ROOT / "scripts/mirror-build-environment-lfs.ps1").read_text(
            encoding="utf-8"
        )
        self.assertIn("inspect-parts", source)
        self.assertLess(
            source.index("inspect-parts"), source.index("Preflight complete")
        )
        evidence = (
            TOOLS_ROOT / "tests/build-environment/evidence.example.json"
        ).read_text(encoding="utf-8")
        self.assertNotIn("-PreflightOnly", evidence)

    @unittest.skipUnless(
        os.name == "nt", "PowerShell publication wrapper runs on Windows"
    )
    def test_publication_preflight_rejects_bad_local_parts_and_accepts_complete_set(
        self,
    ) -> None:
        script = TOOLS_ROOT / "scripts/mirror-build-environment-lfs.ps1"

        def run_case(root: Path) -> subprocess.CompletedProcess[str]:
            return subprocess.run(
                [
                    "powershell.exe",
                    "-NoProfile",
                    "-File",
                    str(script),
                    "-ManifestPath",
                    str(root / "current.json"),
                    "-GiteaCapacityBytes",
                    "1000000",
                    "-BackupCapacityBytes",
                    "1000000",
                ],
                cwd=REPOSITORY_ROOT,
                text=True,
                capture_output=True,
                check=False,
            )

        pointer = (
            b"version https://git-lfs.github.com/spec/v1\n"
            b"oid sha256:" + (b"a" * 64) + b"\nsize 7\n"
        )
        for name, payload, materialized, extra, expected in (
            ("missing", b"payload", None, False, "missing archive part"),
            ("corrupt", b"payload", b"payloae", False, "SHA-256 mismatch"),
            ("pointer", pointer, pointer, False, "Git LFS pointer"),
            ("extra", b"payload", b"payload", True, "unexpected archive part"),
            ("complete", b"payload", b"payload", False, "Preflight complete"),
        ):
            with self.subTest(name=name), tempfile.TemporaryDirectory() as directory:
                root = Path(directory)
                filename = "fixture.oci.tar.gz.part000"
                digest = hashlib.sha256(payload).hexdigest()
                manifest = {
                    "schema": 1,
                    "generation": "fixture",
                    "archive": {
                        "filename": "fixture.oci.tar.gz",
                        "size": len(payload),
                        "sha256": digest,
                    },
                    "image": {
                        "digest": "sha256:" + ("b" * 64),
                        "platform": "linux/amd64",
                    },
                    "parts": [
                        {
                            "ordinal": 0,
                            "filename": filename,
                            "size": len(payload),
                            "sha256": digest,
                            "lfs_oid": "sha256:" + digest,
                        }
                    ],
                    "source": {
                        "base_commit": "a" * 40,
                        "tree_sha256": "c" * 64,
                        "status": "dirty",
                    },
                    "verification": {
                        "status": "pending-round-trip",
                        "verified_at": None,
                        "commands": [],
                        "evidence": None,
                    },
                    "publication": {
                        "gitea": {"status": "pending"},
                        "backup": {"status": "pending"},
                        "github": {"status": "absent"},
                    },
                }
                (root / "current.json").write_text(
                    json.dumps(manifest), encoding="utf-8"
                )
                if materialized is not None:
                    (root / filename).write_bytes(materialized)
                if extra:
                    (root / "unexpected.oci.tar.gz.part999").write_bytes(b"extra")
                result = run_case(root)
                output = result.stdout + result.stderr
                if name == "complete":
                    self.assertEqual(result.returncode, 0, output)
                    self.assertIn("No remote data was changed", output)
                else:
                    self.assertNotEqual(result.returncode, 0, output)
                self.assertIn(expected, output)

    def test_generated_builder_identity_is_host_neutral_and_consistent(self) -> None:
        collector = (
            TOOLS_ROOT / "scripts/collect-build-environment-metadata.sh"
        ).read_text(encoding="utf-8")
        exporter = (TOOLS_ROOT / "scripts/export-build-environment.sh").read_text(
            encoding="utf-8"
        )
        self.assertNotIn('"local WSL2 OCI engine"', collector)
        for source in (collector, exporter):
            self.assertIn("NILO_BUILDER_ID", source)
            self.assertIn("native or WSL2", source)

    def test_acceptance_reports_pending_external_gates_and_never_infers_pass(
        self,
    ) -> None:
        evidence = TOOLS_ROOT / "tests/build-environment/evidence.example.json"
        result = self._run("verify-acceptance", "--evidence", str(evidence))
        self.assertNotEqual(result.returncode, 0)
        report = json.loads(result.stdout)
        self.assertEqual(set(report), {f"BE-{number:02d}" for number in range(1, 11)})
        self.assertEqual(report["BE-01"]["status"], "passed")
        self.assertEqual(report["BE-02"]["status"], "pending")
        self.assertIn("verify-materialized-inputs", report["BE-02"]["command"])
        lock = json.loads(LOCK.read_text(encoding="utf-8"))
        self.assertIn(
            lock["reconstruction"]["manifest_sha256"],
            report["BE-02"]["detail"],
        )
        self.assertEqual(report["BE-04"]["status"], "pending")
        self.assertIn(lock["generation"], report["BE-04"]["detail"])
        for gate in (
            "BE-02",
            "BE-03",
            "BE-04",
            "BE-05",
            "BE-06",
            "BE-07",
            "BE-08",
            "BE-09",
        ):
            self.assertIn(report[gate]["status"], {"pending", "failed"})

    def test_reconstruction_acquires_base_and_frontend_images_by_digest(self) -> None:
        lock = json.loads(LOCK.read_text(encoding="utf-8"))
        images = lock["oci_images"]
        roles = {item["role"]: item for item in images}
        self.assertEqual(set(roles), {"base", "build-frontend"})
        for item in images:
            self.assertRegex(item["digest"], r"^sha256:[0-9a-f]{64}$")
            self.assertIn("@" + item["digest"], item["reference"])
            self.assertTrue(item["source"].startswith("docker://"))
            self.assertIn(item["digest"], item["source"])
            self.assertTrue(item["license"])
            self.assertEqual(item["platform"], "linux/amd64")
        self.assertEqual(roles["base"]["reference"], lock["base_image"]["reference"])
        self.assertEqual(
            roles["build-frontend"]["reference"], lock["oci"]["build_frontend"]
        )

        plan = self._run("plan-oci-images", "--lock", str(LOCK))
        self.assertEqual(plan.returncode, 0, plan.stdout + plan.stderr)
        planned = json.loads(plan.stdout)
        self.assertEqual([item["role"] for item in planned], ["base", "build-frontend"])

        builder = (TOOLS_ROOT / "scripts/build-build-environment.sh").read_text(
            encoding="utf-8"
        )
        self.assertLess(
            builder.index("acquire-oci-images"), builder.index('"$engine" build')
        )
        self.assertLess(
            builder.index("validate-reconstruction"),
            builder.index("acquire-oci-images"),
        )
        self.assertIn("networked materialization", builder)
        self.assertIn("offline image build", builder)
        self.assertIn("--pull=false", builder)
        self.assertIn("--network=none", builder)

    def test_oci_image_acquisition_verifies_digest_and_never_uses_a_mutable_tag(
        self,
    ) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            digest = "sha256:" + ("a" * 64)
            journal = root / "engine.log"

            def lock_file(reference: str) -> Path:
                path = root / "lock.json"
                path.write_text(
                    json.dumps(
                        {
                            "oci_images": [
                                {
                                    "role": "base",
                                    "reference": reference,
                                    "digest": digest,
                                    "platform": "linux/amd64",
                                    "source": "docker://" + reference,
                                    "license": "test-only",
                                }
                            ],
                        }
                    ),
                    encoding="utf-8",
                )
                return path

            stub = root / "engine_stub.py"
            stub.write_text(
                "import json, os, sys\n"
                "journal = os.environ['STUB_JOURNAL']\n"
                "open(journal, 'a', encoding='utf-8').write(' '.join(sys.argv[1:]) + chr(10))\n"
                "reported = os.environ.get('STUB_DIGEST', '')\n"
                "if sys.argv[1:2] == ['pull']:\n"
                "    open(journal + '.pulled', 'w', encoding='utf-8').write(sys.argv[-1])\n"
                "    sys.exit(0)\n"
                "present = os.path.exists(journal + '.pulled') or os.environ.get('STUB_PRESENT') == '1'\n"
                "if not present:\n"
                "    sys.stderr.write('no such image'); sys.exit(1)\n"
                "print(json.dumps([sys.argv[-1].split('@')[0] + '@' + reported] if reported else []))\n",
                encoding="utf-8",
            )
            environment = os.environ.copy()
            environment["NILO_ENGINE_COMMAND"] = f"{sys.executable} {stub}"
            environment["STUB_JOURNAL"] = str(journal)

            def acquire(
                reference: str, reported: str, present: str = "0"
            ) -> subprocess.CompletedProcess[str]:
                environment["STUB_DIGEST"] = reported
                environment["STUB_PRESENT"] = present
                return subprocess.run(
                    [
                        sys.executable,
                        str(CORE),
                        "acquire-oci-images",
                        "--lock",
                        str(lock_file(reference)),
                        "--engine",
                        "docker",
                    ],
                    cwd=REPOSITORY_ROOT,
                    env=environment,
                    text=True,
                    capture_output=True,
                    check=False,
                )

            mutable = acquire("ubuntu:24.04", digest)
            self.assertNotEqual(mutable.returncode, 0)
            self.assertIn("digest-pinned", mutable.stderr)
            self.assertFalse(
                journal.exists(), "a mutable tag must never reach the engine"
            )

            reference = "ubuntu@" + digest
            wrong = acquire(reference, "sha256:" + ("b" * 64))
            self.assertNotEqual(wrong.returncode, 0)
            self.assertIn("digest mismatch", wrong.stderr)
            self.assertIn(digest, wrong.stderr)

            journal.unlink(missing_ok=True)
            Path(str(journal) + ".pulled").unlink(missing_ok=True)
            acquired = acquire(reference, digest)
            self.assertEqual(acquired.returncode, 0, acquired.stdout + acquired.stderr)
            invocations = journal.read_text(encoding="utf-8")
            self.assertIn("pull " + reference, invocations)
            self.assertIn("acquired", acquired.stdout)

            journal.unlink(missing_ok=True)
            Path(str(journal) + ".pulled").unlink(missing_ok=True)
            cached = acquire(reference, digest, present="1")
            self.assertEqual(cached.returncode, 0, cached.stdout + cached.stderr)
            self.assertNotIn("pull ", journal.read_text(encoding="utf-8"))
            self.assertIn("verified local", cached.stdout)

    def test_container_execution_isolates_the_host_checkout(self) -> None:
        wrapper = (TOOLS_ROOT / "env/with-build-environment.sh").read_text(
            encoding="utf-8"
        )
        verifier = (TOOLS_ROOT / "scripts/verify-build-environment.sh").read_text(
            encoding="utf-8"
        )
        for source in (wrapper, verifier):
            self.assertIn('"$repo_root:/workspace:ro"', source)
            self.assertNotIn('"$repo_root:/workspace:rw"', source)
            self.assertIn("/work/source", source)
            self.assertIn("NILO_ISOLATED_SOURCE", source)
        # The isolated copy must exclude host dependency trees and the heavy
        # ignored payloads, and must still carry uncommitted working changes.
        for excluded in (
            "node_modules",
            ".venv",
            "02_Tools/cache",
            "02_Tools/archives",
            "02_Tools/runtime",
            "dist",
        ):
            self.assertIn(excluded, wrapper, f"isolated copy must exclude {excluded}")
        self.assertIn(".git:/work/git-dir:ro", wrapper)
        self.assertIn("GIT_OPTIONAL_LOCKS=0", wrapper)
        self.assertIn("--rm", wrapper)
        builder = (TOOLS_ROOT / "tests/build-environment/build-all.sh").read_text(
            encoding="utf-8"
        )
        self.assertIn("NILO_OUTPUT_DIR", builder)

    @unittest.skipIf(os.name == "nt", "requires POSIX tar")
    def test_source_copy_preserves_control_metadata_without_runtime_payloads(self) -> None:
        """Exercise local, verification and CI copies against a cache fixture."""
        retained = [
            "02_Tools/archives/wsl-host-bootstrap/host-bootstrap.lock.json",
            "02_Tools/archives/build-environment/current.json",
            "05_Software/frontend/package.json",
        ]
        excluded = [
            "02_Tools/archives/build-environment/nilo-build-env-test.oci.tar.gz.part000",
            "02_Tools/archives/build-environment/nilo-build-env-test.oci.tar.gz",
            "02_Tools/archives/wsl-host-bootstrap/nilo-wsl-host-bootstrap-test.tar.gz",
            "02_Tools/cache/runtime.bin",
            "02_Tools/tests/build-environment/evidence.local.json",
            "05_Software/integrations/fake_python/.venv/dependency.py",
            "05_Software/integrations/fake_python/.venv-windows-x86_64/dependency.py",
            "05_Software/integrations/fake_python/.pytest_cache/CACHEDIR.TAG",
            "05_Software/integrations/fake_python/.ruff_cache/CACHEDIR.TAG",
        ]
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            workspace = root / "workspace"
            for name in retained + excluded:
                path = workspace / name
                path.parent.mkdir(parents=True, exist_ok=True)
                path.write_text("fixture", encoding="utf-8")
            for index, name in enumerate([
                "env/with-build-environment.sh", "scripts/verify-build-environment.sh",
                "../.gitea/scripts/offline_ci.py",
            ]):
                with self.subTest(wrapper=name):
                    output = root / f"isolated-{index}"
                    output.mkdir()
                    wrapper = (TOOLS_ROOT / name).read_text(encoding="utf-8")
                    finish = 'find "$NILO_ISOLATED_SOURCE" -depth -type d -empty -delete'
                    if name.endswith("offline_ci.py"):
                        finish = 'find /work/source -depth -type d -empty -delete'
                    command = wrapper[wrapper.index("tar -C /workspace"):wrapper.index(finish) + len(finish)]
                    command = command.replace("/workspace", '"$NILO_TEST_WORKSPACE"')
                    command = command.replace("/work/source", '"$NILO_ISOLATED_SOURCE"')
                    result = subprocess.run(
                        ["sh", "-eu", "-c", command], text=True, capture_output=True,
                        env={**os.environ, "NILO_TEST_WORKSPACE": str(workspace),
                             "NILO_ISOLATED_SOURCE": str(output)},
                    )
                    self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
                    for path in retained:
                        self.assertEqual((output / path).read_text(), "fixture")
                    for path in excluded:
                        self.assertFalse((output / path).exists(), path)
                    self.assertFalse((output / "05_Software/integrations/fake_python").exists())

    def test_host_isolation_gate_proves_dependencies_are_untouched(self) -> None:
        gate = TOOLS_ROOT / "tests/build-environment/verify-host-isolation.sh"
        self.assertTrue(gate.is_file(), "a host-isolation gate script is required")
        source = gate.read_text(encoding="utf-8")
        self.assertIn("node_modules", source)
        self.assertIn(".venv-linux-x86_64", source)
        self.assertIn("with-build-environment.sh", source)
        # The gate must compare a recorded pre-state against the post-state and
        # fail closed, rather than merely printing what it found.
        self.assertIn("exit 1", source)
        verifier = (TOOLS_ROOT / "scripts/verify-build-environment.sh").read_text(
            encoding="utf-8"
        )
        self.assertIn("verify-host-isolation.sh", verifier)

    def test_restore_checks_space_and_avoids_duplicate_full_size_temporaries(
        self,
    ) -> None:
        restore = (TOOLS_ROOT / "scripts/restore-build-environment.sh").read_text(
            encoding="utf-8"
        )
        # A single streamed decompression, not a full .tar.gz temporary plus a
        # full .tar temporary.
        self.assertIn("check-space", restore)
        self.assertIn("--temporary-dir", restore)
        self.assertIn("join-archive", restore)
        self.assertIn("--output -", restore)
        self.assertNotIn('gzip -dc "$joined" > "$oci_archive"', restore)
        self.assertLess(restore.index("check-space"), restore.index("skopeo copy"))
        # Only task-owned temporaries are removed, and never the archive parts.
        self.assertIn("trap", restore)
        self.assertNotIn("$archive_path", restore.split("trap", 1)[1].split("\n", 1)[0])
        splitter = (TOOLS_ROOT / "scripts/build_environment.py").read_text(
            encoding="utf-8"
        )
        self.assertIn("uncompressed_size", splitter)

    def test_space_check_reports_required_and_available_and_fails_closed(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            ample = self._run(
                "check-space",
                "--path",
                str(root),
                "--required-bytes",
                "1",
                "--purpose",
                "join",
            )
            self.assertEqual(ample.returncode, 0, ample.stdout + ample.stderr)
            self.assertIn("available", ample.stdout)

            impossible = self._run(
                "check-space",
                "--path",
                str(root),
                "--required-bytes",
                str(1 << 60),
                "--purpose",
                "join",
            )
            self.assertNotEqual(impossible.returncode, 0)
            self.assertIn("required", impossible.stderr)
            self.assertIn("available", impossible.stderr)
            self.assertIn("join", impossible.stderr)

            unusable = self._run(
                "check-space",
                "--path",
                str(root / "absent" / "deeper"),
                "--required-bytes",
                "1",
                "--purpose",
                "join",
            )
            self.assertNotEqual(unusable.returncode, 0)
            self.assertIn("not a writable directory", unusable.stderr)

    def test_streamed_join_verifies_the_complete_archive_digest(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            payload = b"nilo streamed archive payload" * 8
            part = root / "fixture.tar.gz.part000"
            part.write_bytes(payload)
            manifest = {
                "schema": 1,
                "generation": "fixture",
                "archive": {
                    "filename": "fixture.tar.gz",
                    "size": len(payload),
                    "sha256": hashlib.sha256(payload).hexdigest(),
                },
                "image": {"digest": "sha256:" + ("0" * 64), "reference": "fixture:1"},
                "parts": [
                    {
                        "ordinal": 0,
                        "filename": part.name,
                        "size": len(payload),
                        "sha256": hashlib.sha256(payload).hexdigest(),
                        "lfs_oid": "sha256:" + hashlib.sha256(payload).hexdigest(),
                    }
                ],
                "source": {
                    "base_commit": "0" * 40,
                    "tree_sha256": "1" * 64,
                    "status": "dirty",
                },
                "verification": {"status": "pending-round-trip"},
                "publication": {
                    "gitea": {"status": "pending"},
                    "backup": {"status": "pending"},
                    "github": {"status": "absent"},
                },
            }
            manifest_path = root / "current.json"
            manifest_path.write_text(json.dumps(manifest), encoding="utf-8")

            streamed = subprocess.run(
                [
                    sys.executable,
                    str(CORE),
                    "join-archive",
                    "--manifest",
                    str(manifest_path),
                    "--archive-path",
                    str(root),
                    "--output",
                    "-",
                ],
                cwd=REPOSITORY_ROOT,
                capture_output=True,
                check=False,
            )
            self.assertEqual(streamed.returncode, 0, streamed.stderr.decode())
            self.assertEqual(streamed.stdout, payload)

            manifest["archive"]["sha256"] = hashlib.sha256(b"different").hexdigest()
            manifest_path.write_text(json.dumps(manifest), encoding="utf-8")
            corrupt = subprocess.run(
                [
                    sys.executable,
                    str(CORE),
                    "join-archive",
                    "--manifest",
                    str(manifest_path),
                    "--archive-path",
                    str(root),
                    "--output",
                    "-",
                ],
                cwd=REPOSITORY_ROOT,
                capture_output=True,
                check=False,
            )
            self.assertNotEqual(corrupt.returncode, 0)
            self.assertIn(b"archive SHA-256", corrupt.stderr)
            self.assertTrue(part.is_file(), "archive parts must survive a failed join")

    def _host_transaction(
        self,
        catalogue: dict[str, dict[str, str]],
        installed: dict[str, str],
        *extra: str,
        apt_exit: int = 0,
        apt_output: str = "No removals",
    ) -> subprocess.CompletedProcess[str]:
        """Plan a host package transaction against injected package-tool doubles."""
        directory = tempfile.mkdtemp()
        root = Path(directory)
        packages = root / "packages"
        packages.mkdir()
        for filename in catalogue:
            (packages / filename).write_bytes(b"deb")
        stub = root / "dpkg_stub.py"
        stub.write_text(
            "import json, os, sys\n"
            "catalogue = json.loads(os.environ['STUB_CATALOGUE'])\n"
            "installed = json.loads(os.environ['STUB_INSTALLED'])\n"
            "argv = sys.argv[1:]\n"
            "if argv[0] == '-f':\n"
            "    entry = catalogue[os.path.basename(argv[1])]\n"
            "    print(entry[argv[2]])\n"
            "elif argv[0] == '-W':\n"
            "    name = argv[-1]\n"
            "    if name not in installed:\n"
            "        sys.stderr.write('no packages found'); sys.exit(1)\n"
            "    print(installed[name])\n"
            "elif argv[0] == '--compare-versions':\n"
            "    left, relation, right = argv[1], argv[2], argv[3]\n"
            "    def key(value):\n"
            "        return [int(part) for part in value.replace('-', '.').split('.') if part.isdigit()]\n"
            "    outcome = {'lt': key(left) < key(right), 'gt': key(left) > key(right),\n"
            "               'eq': key(left) == key(right)}[relation]\n"
            "    sys.exit(0 if outcome else 1)\n"
            "else:\n"
            "    sys.exit(2)\n",
            encoding="utf-8",
        )
        apt_stub = root / "apt_stub.py"
        apt_stub.write_text(
            "import os, sys\n"
            "print(os.environ.get('STUB_APT_OUTPUT', ''))\n"
            "sys.exit(int(os.environ.get('STUB_APT_EXIT', '0')))\n",
            encoding="utf-8",
        )
        environment = os.environ.copy()
        environment["STUB_CATALOGUE"] = json.dumps(catalogue)
        environment["STUB_INSTALLED"] = json.dumps(installed)
        environment["STUB_APT_EXIT"] = str(apt_exit)
        environment["STUB_APT_OUTPUT"] = apt_output
        for name in (
            "NILO_DPKG_DEB_COMMAND",
            "NILO_DPKG_QUERY_COMMAND",
            "NILO_DPKG_COMMAND",
        ):
            environment[name] = f"{sys.executable} {stub}"
        environment["NILO_APT_GET_COMMAND"] = f"{sys.executable} {apt_stub}"
        return subprocess.run(
            [
                sys.executable,
                str(CORE),
                "plan-host-transaction",
                "--package-dir",
                str(packages),
                *extra,
            ],
            cwd=REPOSITORY_ROOT,
            env=environment,
            text=True,
            capture_output=True,
            check=False,
        )

    def test_host_installation_plans_the_transaction_before_changing_anything(
        self,
    ) -> None:
        catalogue = {
            "git-lfs.deb": {
                "Package": "git-lfs",
                "Version": "3.4.1",
                "Architecture": "amd64",
            },
            "jq.deb": {"Package": "jq", "Version": "1.7.1", "Architecture": "amd64"},
            "libkrb5.deb": {
                "Package": "libkrb5-3",
                "Version": "1.20.6",
                "Architecture": "amd64",
            },
        }
        installed = {"jq": "1.7.1\namd64", "libkrb5-3": "1.20.1\namd64"}
        planned = self._host_transaction(catalogue, installed)
        self.assertEqual(planned.returncode, 0, planned.stdout + planned.stderr)
        self.assertIn("install", planned.stdout)
        self.assertIn("git-lfs", planned.stdout)
        self.assertIn("reinstall", planned.stdout)
        self.assertIn("upgrade", planned.stdout)
        self.assertIn("1.20.1", planned.stdout)
        self.assertIn("1.20.6", planned.stdout)

    def test_host_installation_fails_closed_on_downgrades_and_architecture_changes(
        self,
    ) -> None:
        downgrade = {
            "libc6.deb": {
                "Package": "libc6",
                "Version": "2.39",
                "Architecture": "amd64",
            },
        }
        refused = self._host_transaction(downgrade, {"libc6": "2.40\namd64"})
        self.assertNotEqual(refused.returncode, 0)
        self.assertIn("downgrade", refused.stderr)
        self.assertIn("libc6", refused.stderr)
        self.assertIn("2.40", refused.stderr)
        self.assertIn("no host package was changed", refused.stderr.lower())

        overridden = self._host_transaction(
            downgrade,
            {"libc6": "2.40\namd64"},
            "--allow-downgrades",
        )
        self.assertEqual(
            overridden.returncode, 0, overridden.stdout + overridden.stderr
        )
        self.assertIn("downgrade", overridden.stdout)
        self.assertIn("explicitly authorized", overridden.stdout)

        replacement = {
            "libc6.deb": {
                "Package": "libc6",
                "Version": "2.41",
                "Architecture": "arm64",
            },
        }
        incompatible = self._host_transaction(replacement, {"libc6": "2.40\namd64"})
        self.assertNotEqual(incompatible.returncode, 0)
        self.assertIn("architecture", incompatible.stderr)
        self.assertIn("no host package was changed", incompatible.stderr.lower())

    def test_engine_installer_plans_before_it_mutates_the_host(self) -> None:
        installer = (TOOLS_ROOT / "bootstrap/bootstrap-container-engine.sh").read_text(
            encoding="utf-8"
        )
        self.assertIn("plan-host-transaction", installer)
        self.assertLess(
            installer.index("plan-host-transaction"), installer.index("sudo apt-get")
        )
        self.assertIn("--allow-downgrades", installer)
        self.assertIn("allow_downgrades", installer)

    def test_acceptance_evidence_distinguishes_maturity_states(self) -> None:
        for name in ("evidence.example.json", "evidence.local.json"):
            path = TOOLS_ROOT / "tests/build-environment" / name
            if not path.is_file():
                continue
            evidence = json.loads(path.read_text(encoding="utf-8"))
            maturity = evidence["maturity"]
            self.assertEqual(
                set(maturity),
                {
                    "implemented",
                    "locally_integrity_checked",
                    "round_trip_verified",
                    "clean_host_verified",
                    "published",
                    "independently_backed_up",
                    "fully_accepted",
                },
            )
            for value in maturity.values():
                self.assertIsInstance(value, bool)
        # A later state can never be claimed without every earlier one, and
        # full acceptance requires every gate to have passed.
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "evidence.json"
            evidence = json.loads(
                (
                    TOOLS_ROOT / "tests/build-environment/evidence.example.json"
                ).read_text(encoding="utf-8")
            )
            evidence["maturity"] = dict.fromkeys(evidence["maturity"], False)
            evidence["maturity"]["published"] = True
            path.write_text(json.dumps(evidence), encoding="utf-8")
            result = self._run("verify-acceptance", "--evidence", str(path))
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("maturity", result.stderr)
            self.assertIn("published", result.stderr)

            evidence["maturity"] = dict.fromkeys(evidence["maturity"], True)
            path.write_text(json.dumps(evidence), encoding="utf-8")
            claimed = self._run("verify-acceptance", "--evidence", str(path))
            self.assertNotEqual(claimed.returncode, 0)
            self.assertIn("fully_accepted", claimed.stderr)

    def test_evidence_detail_names_every_pending_gate(self) -> None:
        for name in ("evidence.example.json", "evidence.local.json"):
            path = TOOLS_ROOT / "tests/build-environment" / name
            if not path.is_file():
                continue
            evidence = json.loads(path.read_text(encoding="utf-8"))
            gates = evidence["gates"]
            pending = {
                gate for gate, record in gates.items() if record["status"] != "passed"
            }
            for gate, record in gates.items():
                if record["status"] != "passed":
                    continue
                # A passing gate must not describe a stale, narrower pending set.
                named = {
                    token.strip(".,;")
                    for token in record["detail"].replace(",", " ").split()
                    if token.startswith("BE-")
                }
                self.assertTrue(
                    named <= pending or not named,
                    f"{name}/{gate} names gates that are not pending: {sorted(named - pending)}",
                )
                if named:
                    self.assertEqual(
                        named,
                        pending,
                        f"{name}/{gate} omits pending gates {sorted(pending - named)}",
                    )

    def test_documentation_states_the_exact_supported_boundary(self) -> None:
        root_readme = (REPOSITORY_ROOT / "README.md").read_text(encoding="utf-8")
        # A new developer must be told which path serves which purpose.
        self.assertIn("Build environment", root_readme)
        self.assertIn("nilo-tools.ps1 setup container", root_readme)
        self.assertIn("nilo-tools.ps1 dev start", root_readme)
        self.assertIn("nilo-tools.ps1 test", root_readme)
        self.assertIn("02_Tools/README.md", root_readme)

        readme = (TOOLS_ROOT / "README.md").read_text(encoding="utf-8")
        self.assertIn("Ubuntu 24.04", readme)
        self.assertIn("systemd", readme)
        # Git and Git LFS acquisition is a human prerequisite, never a claim
        # that the repository bootstraps itself from an ordinary clone.
        self.assertIn("Git LFS", readme)
        self.assertNotIn("self-bootstrapping", readme.lower())
        for phrase in ("git lfs pull", "prerequisite"):
            self.assertIn(phrase, readme.lower())
        # Integrity is not authenticity.
        self.assertIn("integrity", readme.lower())
        self.assertNotIn("proves the publisher", readme.lower())
        self.assertNotIn("proof of authenticity", readme.lower())
        # Honest, unclaimed obligations.
        self.assertIn("not implemented", readme.lower())
        for topic in ("scan", "podman"):
            self.assertIn(topic, readme.lower())

    def test_space_check_accounts_for_the_wsl_host_volume(self) -> None:
        """A WSL ext4 virtual disk can only grow while its Windows volume has room.

        Free space reported inside the distribution is therefore not sufficient
        evidence that a multi-gigabyte staging file will fit.
        """
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            host_volume = root / "host"
            host_volume.mkdir()
            environment = os.environ.copy()
            environment["NILO_WSL_HOST_VOLUME"] = str(host_volume)
            environment["NILO_TEST_FORCE_WSL"] = "1"
            environment["NILO_TEST_HOST_VOLUME_FREE_BYTES"] = "1024"
            result = subprocess.run(
                [
                    sys.executable,
                    str(CORE),
                    "check-space",
                    "--path",
                    str(root),
                    "--required-bytes",
                    "2048",
                    "--purpose",
                    "joined OCI archive staging",
                ],
                cwd=REPOSITORY_ROOT,
                env=environment,
                text=True,
                capture_output=True,
                check=False,
            )
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("virtual disk", result.stderr.lower())
            self.assertIn("1024", result.stderr)
            self.assertIn("2048", result.stderr)

            environment["NILO_TEST_HOST_VOLUME_FREE_BYTES"] = str(1 << 40)
            ample = subprocess.run(
                [
                    sys.executable,
                    str(CORE),
                    "check-space",
                    "--path",
                    str(root),
                    "--required-bytes",
                    "2048",
                    "--purpose",
                    "joined OCI archive staging",
                ],
                cwd=REPOSITORY_ROOT,
                env=environment,
                text=True,
                capture_output=True,
                check=False,
            )
            self.assertEqual(ample.returncode, 0, ample.stdout + ample.stderr)
            self.assertIn("host volume", ample.stdout.lower())

    def test_engine_store_space_check_warns_rather_than_blocking_deduplicated_imports(
        self,
    ) -> None:
        """A vhdx with internal free space need not grow, so the engine-store
        check advises instead of blocking, while the staging file stays strict."""
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            environment = os.environ.copy()
            environment["NILO_TEST_FORCE_WSL"] = "1"
            environment["NILO_TEST_HOST_VOLUME_FREE_BYTES"] = "1024"
            environment["NILO_WSL_HOST_VOLUME"] = str(root)
            advisory = subprocess.run(
                [
                    sys.executable,
                    str(CORE),
                    "check-space",
                    "--path",
                    str(root),
                    "--required-bytes",
                    "2048",
                    "--purpose",
                    "docker image storage",
                    "--host-volume",
                    "advisory",
                ],
                cwd=REPOSITORY_ROOT,
                env=environment,
                text=True,
                capture_output=True,
                check=False,
            )
            self.assertEqual(advisory.returncode, 0, advisory.stdout + advisory.stderr)
            self.assertIn("warning", (advisory.stdout + advisory.stderr).lower())
            self.assertIn("1024", advisory.stdout + advisory.stderr)
        restore = (TOOLS_ROOT / "scripts/restore-build-environment.sh").read_text(
            encoding="utf-8"
        )
        self.assertIn("--host-volume advisory", restore)

    def test_engine_store_space_probe_does_not_require_user_write_access(self) -> None:
        spec = importlib.util.spec_from_file_location("nilo_build_environment", CORE)
        self.assertIsNotNone(spec)
        self.assertIsNotNone(spec.loader)
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        with tempfile.TemporaryDirectory() as directory:
            with mock.patch.object(module.os, "access", return_value=False):
                module.check_space(
                    Path(directory),
                    1,
                    "daemon-owned engine storage",
                    host_volume="advisory",
                    require_writable=False,
                )
        restore = (TOOLS_ROOT / "scripts/restore-build-environment.sh").read_text(
            encoding="utf-8"
        )
        self.assertIn("--allow-read-only-path", restore)

    def test_host_transaction_rejects_unresolved_dependencies_before_install(
        self,
    ) -> None:
        catalogue = {
            "engine.deb": {
                "Package": "engine",
                "Version": "1.0",
                "Architecture": "amd64",
            },
        }
        result = self._host_transaction(
            catalogue,
            {},
            apt_exit=100,
            apt_output="E: Unmet dependencies. Missing libengine1.",
        )
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("unmet dependencies", result.stderr.lower())
        self.assertIn("no host package was changed", result.stderr.lower())

        planner = (TOOLS_ROOT / "scripts/build_environment.py").read_text(
            encoding="utf-8"
        )
        installer = (TOOLS_ROOT / "bootstrap/bootstrap-container-engine.sh").read_text(
            encoding="utf-8"
        )
        self.assertIn('"--simulate"', planner)
        self.assertIn('"--no-remove"', planner)
        self.assertIn(
            "sudo apt-get --no-download --no-install-recommends --no-remove", installer
        )
        self.assertNotIn("sudo dpkg -i", installer)

    def test_downgrade_override_is_explicit_in_supported_entry_points(self) -> None:
        windows = (TOOLS_ROOT / "bootstrap/bootstrap-build-environment.ps1").read_text(
            encoding="utf-8"
        )
        native = (TOOLS_ROOT / "bootstrap/bootstrap-build-environment.sh").read_text(
            encoding="utf-8"
        )
        readme = (TOOLS_ROOT / "README.md").read_text(encoding="utf-8")
        self.assertIn("AllowEngineDowngrades", windows)
        self.assertIn("--allow-engine-downgrades", native)
        self.assertIn("--allow-downgrades", native)
        self.assertIn("AllowEngineDowngrades", readme)
        self.assertIn("--allow-engine-downgrades", readme)

    def test_wsl_space_check_requires_the_actual_backing_volume(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            environment = os.environ.copy()
            environment["NILO_TEST_FORCE_WSL"] = "1"
            environment.pop("NILO_WSL_HOST_VOLUME", None)
            result = subprocess.run(
                [
                    sys.executable,
                    str(CORE),
                    "check-space",
                    "--path",
                    directory,
                    "--required-bytes",
                    "1",
                    "--purpose",
                    "staging",
                ],
                cwd=REPOSITORY_ROOT,
                env=environment,
                text=True,
                capture_output=True,
                check=False,
            )
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("NILO_WSL_HOST_VOLUME", result.stderr)

        windows = (TOOLS_ROOT / "bootstrap/bootstrap-build-environment.ps1").read_text(
            encoding="utf-8"
        )
        shared = (TOOLS_ROOT / "lib/Wsl.ps1").read_text(encoding="utf-8")
        self.assertIn("Get-NiloWslHostVolume", windows)
        self.assertIn("DistributionName", shared)
        self.assertIn("NILO_WSL_HOST_VOLUME", windows)

    def test_host_isolation_sentinel_cleanup_is_interruption_safe(self) -> None:
        gate = (
            TOOLS_ROOT / "tests/build-environment/verify-host-isolation.sh"
        ).read_text(encoding="utf-8")
        self.assertIn("cleanup()", gate)
        self.assertIn("trap cleanup", gate)
        self.assertIn("sentinel_name=.nilo-host-isolation-sentinel-$$", gate)
        self.assertLess(
            gate.index('printf \'%s\\n\' "$path/$sentinel_name" >> "$work/sentinels"'),
            gate.index('printf \'%s\\n\' "$sentinel_token" > "$path/$sentinel_name"'),
        )

    def test_acceptance_evidence_commands_include_required_arguments(self) -> None:
        for name in ("evidence.example.json", "evidence.local.json"):
            path = TOOLS_ROOT / "tests/build-environment" / name
            if not path.is_file():
                continue
            gates = json.loads(path.read_text(encoding="utf-8"))["gates"]
            restore = gates["BE-05"]["command"]
            round_trip = gates["BE-07"]["command"]
            if "restore-build-environment.sh" in restore:
                self.assertIn("--engine docker", restore)
                self.assertIn("--manifest ", restore)
                self.assertIn("--archive-path ", restore)
                self.assertIn(
                    "--image nilo/build-environment:noble-20260905-01", restore
                )
            self.assertIn("verify-archive-round-trip.sh", round_trip)
            self.assertIn("--engine docker", round_trip)
            self.assertIn("--manifest ", round_trip)
            self.assertIn("--archive-path ", round_trip)

    def test_policy_lfs_and_documented_commands_match_the_plan(self) -> None:
        attributes = (REPOSITORY_ROOT / ".gitattributes").read_text(encoding="utf-8")
        self.assertIn("02_Tools/archives/build-environment/*.part", attributes)
        self.assertIn("filter=lfs", attributes)
        for path in (
            "02_Tools/bootstrap/bootstrap-build-environment.sh",
            "02_Tools/containers/build/Containerfile",
        ):
            result = subprocess.run(
                ["git", "check-attr", "eol", "--", path],
                cwd=REPOSITORY_ROOT,
                text=True,
                capture_output=True,
                check=True,
            )
            self.assertTrue(result.stdout.rstrip().endswith(": lf"), result.stdout)
        ignore = (REPOSITORY_ROOT / ".gitignore").read_text(encoding="utf-8")
        self.assertIn("/02_Tools/runtime/", ignore)
        readme = (TOOLS_ROOT / "README.md").read_text(encoding="utf-8")
        for command in (
            "bootstrap-build-environment.ps1",
            "bootstrap-build-environment.sh",
            "with-build-environment.ps1",
            "with-build-environment.sh",
            "build-host-bootstrap.sh",
            "export-build-environment.sh",
            "mirror-build-environment-lfs.ps1",
        ):
            self.assertIn(command, readme)
        self.assertNotIn(
            "checked-in archive manifests intentionally say `pending-generation`",
            readme.lower(),
        )
        self.assertIn("local-round-trip-complete", readme)
        definition_readme = (TOOLS_ROOT / "containers/build/README.md").read_text(
            encoding="utf-8"
        )
        self.assertIn("677-record", definition_readme)
        self.assertNotIn("deliberately incomplete", definition_readme)
        self.assertNotIn("not yet materialized", definition_readme)


if __name__ == "__main__":
    unittest.main()

# -----------------------------------------------------------------------------
# END OF FILE
# -----------------------------------------------------------------------------
