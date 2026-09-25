#!/usr/bin/env python3
# -*- coding: utf-8 -*-  # noqa: UP009 - repository Python style
# -----------------------------------------------------------------------------
# Copyright (c) 2026 DDP Engineering
# SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
# -----------------------------------------------------------------------------
"""@file test_probe.py
@brief Verify camera feasibility evidence without appliance hardware.
@details Fixture runs exercise grouping and reporting but cannot certify a target.
"""

# IMPORTS
from __future__ import annotations

import hashlib
import importlib.util
import json
import subprocess
import sys
import tempfile
import types
import unittest
from argparse import Namespace
from pathlib import Path
from unittest.mock import patch

# CONSTANTS
PROBE = Path(__file__).with_name("probe.py")
WORKER = Path(__file__).with_name("candidate_probe.py")


# HELPER FUNCTIONS
def _fixture() -> dict[str, object]:
    """@brief Return two physical cameras, including one metadata-only node."""
    return {
        "nodes": [
            {
                "name": "video0",
                "usb_parent": "1-1.2",
                "vendor": "046d",
                "product": "0825",
                "serial": "ABC",
                "interface": "00",
                "capabilities": ["VIDEO_CAPTURE", "STREAMING"],
                "formats": ["MJPG", "YUYV"],
            },
            {
                "name": "video1",
                "usb_parent": "1-1.2",
                "vendor": "046d",
                "product": "0825",
                "serial": "ABC",
                "interface": "01",
                "capabilities": ["META_CAPTURE", "STREAMING"],
                "formats": [],
            },
            {
                "name": "video2",
                "usb_parent": "1-1.3",
                "vendor": "534d",
                "product": "2109",
                "serial": "",
                "interface": "00",
                "capabilities": ["VIDEO_CAPTURE", "STREAMING"],
                "formats": ["MJPG"],
            },
        ]
    }


# PUBLIC API
class ProbeFixtureTests(unittest.TestCase):
    """@brief Check evidence safety and physical-node grouping."""

    def test_fixture_groups_nodes_without_claiming_target_acceptance(self) -> None:
        """@brief A fixture cannot fill the hardware acceptance matrix."""
        self.assertTrue(PROBE.is_file(), "camera feasibility probe is not implemented")
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            fixture = root / "fixture.json"
            fixture.write_text(json.dumps(_fixture()), encoding="utf-8")
            output = root / "evidence"
            process = subprocess.run(
                [
                    sys.executable,
                    str(PROBE),
                    "--fixture",
                    str(fixture),
                    "--output",
                    str(output),
                ],
                capture_output=True,
                text=True,
                check=False,
                timeout=15,
            )
            self.assertEqual(process.returncode, 0, process.stderr)
            evidence = json.loads(
                (output / "evidence.json").read_text(encoding="utf-8")
            )
            self.assertEqual(evidence["schema_version"], 2)
            self.assertEqual(evidence["mode"], "fixture")
            self.assertFalse(evidence["target_eligible"])
            self.assertEqual(len(evidence["devices"]), 2)
            webcam = next(
                device for device in evidence["devices"] if device["vendor"] == "046d"
            )
            self.assertEqual(len(webcam["endpoints"]), 2)
            self.assertEqual(webcam["selected_endpoint"], "video0")
            self.assertEqual(
                {cell["status"] for cell in evidence["acceptance_matrix"]}, {"not_run"}
            )
            self.assertIn(
                "not_run", (output / "summary.txt").read_text(encoding="utf-8")
            )

    def test_oversized_fixture_fails_without_evidence(self) -> None:
        """@brief Bounded input is rejected before evidence is written."""
        self.assertTrue(PROBE.is_file(), "camera feasibility probe is not implemented")
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            fixture = root / "fixture.json"
            fixture.write_bytes(b" " * (1024 * 1024 + 1))
            output = root / "evidence"
            process = subprocess.run(
                [
                    sys.executable,
                    str(PROBE),
                    "--fixture",
                    str(fixture),
                    "--output",
                    str(output),
                ],
                capture_output=True,
                text=True,
                check=False,
                timeout=15,
            )
            self.assertNotEqual(process.returncode, 0)
            self.assertFalse((output / "evidence.json").exists())

    def test_fixture_records_exact_probe_source_digests(self) -> None:
        """@brief Target records can prove they ran the same probe revision."""
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            fixture = root / "fixture.json"
            fixture.write_text(json.dumps(_fixture()), encoding="utf-8")
            output = root / "evidence"
            process = subprocess.run(
                [
                    sys.executable,
                    str(PROBE),
                    "--fixture",
                    str(fixture),
                    "--output",
                    str(output),
                ],
                capture_output=True,
                text=True,
                check=False,
                timeout=15,
            )
            self.assertEqual(process.returncode, 0, process.stderr)
            evidence = json.loads(
                (output / "evidence.json").read_text(encoding="utf-8")
            )
            self.assertEqual(
                evidence["probe_sources"],
                {
                    "probe_sha256": hashlib.sha256(PROBE.read_bytes()).hexdigest(),
                    "worker_sha256": hashlib.sha256(WORKER.read_bytes()).hexdigest(),
                },
            )

    def test_hardware_inspection_selects_capture_node_before_capture(self) -> None:
        """@brief Worker inspection must establish usable endpoints first."""
        spec = importlib.util.spec_from_file_location("camera_probe", PROBE)
        self.assertIsNotNone(spec)
        self.assertIsNotNone(spec.loader)
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        nodes = _fixture()["nodes"][:2]
        for node in nodes:
            node["capabilities"] = []
            node["formats"] = []
        nodes = module._validate_nodes(nodes)
        calls: list[tuple[str, str, str, str | None]] = []

        def worker(
            candidate: str, action: str, node: str, pixel_format: str | None = None
        ) -> dict:
            calls.append((candidate, action, node, pixel_format))
            if action == "inspect":
                if node == "video0":
                    return {
                        "status": "ok",
                        "capabilities": ["VIDEO_CAPTURE", "STREAMING"],
                        "formats": ["MJPG", "YUYV"],
                    }
                return {
                    "status": "ok",
                    "capabilities": ["META_CAPTURE", "STREAMING"],
                    "formats": [],
                }
            return {"status": "ok"}

        args = Namespace(
            fixture=None, device="video0", kind="webcam", wheelhouse=None, lock=None
        )
        with (
            patch.object(module.platform, "system", return_value="Linux"),
            patch.object(module, "_hardware_nodes", return_value=nodes),
            patch.object(module, "_target_profile", return_value="ubuntu-amd64"),
            patch.object(module, "_run_worker", side_effect=worker),
            patch.object(
                module,
                "_run_cancellation",
                return_value={"status": "ok", "stop_ms": 10},
            ),
        ):
            evidence = module._evidence(args)
        self.assertEqual(evidence["devices"][0]["selected_endpoint"], "video0")
        self.assertIn(("linuxpy", "inspect", "video0", None), calls)
        self.assertIn(("pyrav4l2", "capture", "video0", "MJPG"), calls)
        self.assertNotIn(("linuxpy", "capture", "video1", "MJPG"), calls)
        self.assertFalse(evidence["target_eligible"])
        observed = next(
            cell
            for cell in evidence["acceptance_matrix"]
            if cell["target"] == "ubuntu-amd64" and cell["device_kind"] == "webcam"
        )
        self.assertEqual(observed["status"], "incomplete")

    def test_candidate_worker_self_check_is_structured(self) -> None:
        """@brief Missing optional binding remains an explicit observation."""
        self.assertTrue(WORKER.is_file(), "candidate binding worker is not implemented")
        process = subprocess.run(
            [
                sys.executable,
                str(WORKER),
                "--candidate",
                "linuxpy",
                "--action",
                "self-check",
            ],
            capture_output=True,
            text=True,
            check=False,
            timeout=10,
        )
        self.assertEqual(process.returncode, 0, process.stderr)
        result = json.loads(process.stdout)
        self.assertEqual(result["candidate"], "linuxpy")
        self.assertIn(result["status"], ("available", "unavailable"))

    def test_cancellation_stops_a_ready_child(self) -> None:
        """@brief A blocked candidate has an owned, bounded stop path."""
        spec = importlib.util.spec_from_file_location("camera_probe", PROBE)
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        with tempfile.TemporaryDirectory() as directory:
            worker = Path(directory) / "worker.py"
            worker.write_text(
                'import time\nprint(\'{"status":"ready"}\', flush=True)\ntime.sleep(30)\n',
                encoding="utf-8",
            )
            with patch.object(module, "WORKER", worker):
                result = module._run_cancellation("linuxpy", "video0")
        self.assertEqual(result["status"], "ok")
        self.assertLess(result["stop_ms"], 3000)

    def test_cancellation_does_not_pass_after_child_exits_on_its_own(self) -> None:
        """@brief A stream that ends before cancellation proves no stop path."""
        spec = importlib.util.spec_from_file_location("camera_probe", PROBE)
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        with tempfile.TemporaryDirectory() as directory:
            worker = Path(directory) / "worker.py"
            worker.write_text(
                'print(\'{"status":"ready"}\', flush=True)\n', encoding="utf-8"
            )
            with patch.object(module, "WORKER", worker):
                result = module._run_cancellation("linuxpy", "video0")
        self.assertEqual(result["status"], "failed")

    def test_pyrav4l2_fourcc_is_normalized_from_numeric_format(self) -> None:
        """@brief A Motion-JPEG description maps to its actual V4L2 FourCC."""
        spec = importlib.util.spec_from_file_location("camera_candidate", WORKER)
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)

        class Format:
            description = "Motion-JPEG"
            pixelformat = int.from_bytes(b"MJPG", "little")

            def __str__(self) -> str:
                return self.description

        self.assertEqual(module._format_name(Format()), "MJPG")

    def test_linuxpy_image_format_uses_current_pixel_format_field(self) -> None:
        """@brief Inspect ImageFormat from the pinned linuxpy wheel."""
        spec = importlib.util.spec_from_file_location("camera_candidate", WORKER)
        assert spec is not None and spec.loader is not None
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)

        class ImageFormat:
            pixel_format = int.from_bytes(b"MJPG", "little")

        self.assertEqual(module._linuxpy_format_name(ImageFormat()), "MJPG")

    def test_linuxpy_node_capabilities_and_capture_formats(self) -> None:
        """@brief Exclude a sibling metadata node from the capture endpoint."""
        spec = importlib.util.spec_from_file_location("camera_candidate", WORKER)
        assert spec is not None and spec.loader is not None
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)

        class Flags:
            name = "META_CAPTURE|STREAMING"

        class Format:
            pixel_format = int.from_bytes(b"UVCH", "little")

        self.assertEqual(module._capabilities(Flags()), ["META_CAPTURE", "STREAMING"])
        self.assertEqual(module._linuxpy_capture_formats([Format()]), [])

    def test_generic_debian_arm64_is_not_claimed_as_raspberry_pi(self) -> None:
        """@brief The target profile needs observed Pi identity, not OS alone."""
        spec = importlib.util.spec_from_file_location("camera_probe", PROBE)
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        with (
            patch.object(module.platform, "system", return_value="Linux"),
            patch.object(module.platform, "machine", return_value="aarch64"),
            patch.object(
                Path, "read_text", return_value='ID=debian\nVERSION_ID="13"\n'
            ),
            patch.object(Path, "read_bytes", return_value=b"Generic ARM board"),
        ):
            self.assertIsNone(module._target_profile())

    def test_wsl_ubuntu_is_not_claimed_as_mini_pc(self) -> None:
        """@brief A WSL kernel cannot fill the Ubuntu appliance target cell."""
        spec = importlib.util.spec_from_file_location("camera_probe", PROBE)
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        with (
            patch.object(module.platform, "system", return_value="Linux"),
            patch.object(module.platform, "machine", return_value="x86_64"),
            patch.object(
                module.platform,
                "release",
                return_value="6.18.33.2-microsoft-standard-WSL2",
            ),
            patch.object(
                Path, "read_text", return_value='ID=ubuntu\nVERSION_ID="24.04"\n'
            ),
        ):
            self.assertIsNone(module._target_profile())

    def test_empty_wheel_lock_cannot_prove_offline_closure(self) -> None:
        """@brief An empty matching inventory is not dependency evidence."""
        spec = importlib.util.spec_from_file_location("camera_probe", PROBE)
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            wheelhouse = root / "wheels"
            wheelhouse.mkdir()
            lock = root / "lock.json"
            lock.write_text('{"wheels":{}}', encoding="utf-8")
            result = module._wheel_closure(wheelhouse, lock)
        self.assertEqual(result["status"], "failed")

    def test_usb_interface_is_read_from_resolved_device_directory(self) -> None:
        """@brief The interface directory itself carries its number."""
        spec = importlib.util.spec_from_file_location("camera_probe", PROBE)
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        with tempfile.TemporaryDirectory() as directory:
            physical = Path(directory) / "1-1"
            interface = physical / "interface-0"
            interface.mkdir(parents=True)
            (physical / "idVendor").write_text("046d", encoding="ascii")
            (physical / "idProduct").write_text("0825", encoding="ascii")
            (interface / "bInterfaceNumber").write_text("00", encoding="ascii")
            parent, number = module._usb_identity(interface)
        self.assertEqual(parent, physical)
        self.assertEqual(number, "00")

    def test_incomplete_hardware_run_returns_nonzero_with_evidence(self) -> None:
        """@brief An incomplete target cell is recorded without a success exit."""
        spec = importlib.util.spec_from_file_location("camera_probe", PROBE)
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        evidence = {
            "mode": "hardware",
            "target_eligible": False,
            "devices": [],
            "acceptance_matrix": [
                {"target": "pi-arm64", "device_kind": "webcam", "status": "incomplete"}
            ],
        }
        with tempfile.TemporaryDirectory() as directory:
            with patch.object(module, "_evidence", return_value=evidence):
                result = module.main(
                    [
                        "--device",
                        "/dev/video0",
                        "--kind",
                        "webcam",
                        "--output",
                        directory,
                    ]
                )
            self.assertEqual(result, 3)
            self.assertEqual(
                json.loads((Path(directory) / "evidence.json").read_text())["mode"],
                "hardware",
            )

    def test_oversized_decoded_image_is_rejected_before_load(self) -> None:
        """@brief Encoded byte bounds alone cannot bound Pillow allocation."""
        spec = importlib.util.spec_from_file_location("camera_candidate", WORKER)
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)

        class Image:
            size = (100_000, 100_000)

            def __enter__(self):
                return self

            def __exit__(self, *_):
                return None

            def load(self):
                raise AssertionError("decoder must not load an oversized image")

        fake = types.ModuleType("PIL")
        fake.Image = types.SimpleNamespace(open=lambda _: Image())
        with (
            patch.dict(sys.modules, {"PIL": fake}),
            self.assertRaises(ValueError),
        ):
            module._decode_image(b"small", "MJPG", 640, 480)


if __name__ == "__main__":
    unittest.main()

# END OF FILE
