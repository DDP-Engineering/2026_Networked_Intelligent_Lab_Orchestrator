#!/usr/bin/env python3
# -*- coding: utf-8 -*-  # noqa: UP009 - repository Python style
# -----------------------------------------------------------------------------
# Copyright (c) 2026 DDP Engineering
# SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
# -----------------------------------------------------------------------------
"""@file probe.py
@brief Collect bounded, reviewable V4L2 binding feasibility evidence.
@details Fixture evidence is explicitly ineligible for target acceptance.
"""

# IMPORTS
from __future__ import annotations

import argparse
import hashlib
import json
import os
import platform
import queue
import re
import signal
import stat
import subprocess
import sys
import tempfile
import threading
import time
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

# CONSTANTS
SCHEMA_VERSION = 2
MAX_INPUT_BYTES = 1024 * 1024
MAX_NODES = 32
MAX_CHILD_OUTPUT = 256 * 1024
NODE_NAME = re.compile(r"video[0-9]{1,3}\Z")
USB_ID = re.compile(r"[0-9a-fA-F]{4}\Z")
KINDS = ("webcam", "hdmi")
TARGETS = (
    ("pi-arm64", "webcam"),
    ("pi-arm64", "hdmi"),
    ("ubuntu-amd64", "webcam"),
    ("ubuntu-amd64", "hdmi"),
)
CANDIDATES = ("linuxpy", "pyrav4l2")
WORKER = Path(__file__).with_name("candidate_probe.py")


# HELPER FUNCTIONS
def _small_text(path: Path, limit: int = 256) -> str:
    """@brief Read a bounded sysfs field."""
    value = path.read_bytes()
    if len(value) > limit:
        raise ValueError(f"oversized sysfs field: {path.name}")
    return value.decode("utf-8", errors="replace").strip()


def _read_json(path: Path) -> dict[str, Any]:
    """@brief Read one bounded fixture document."""
    if path.stat().st_size > MAX_INPUT_BYTES:
        raise ValueError("fixture exceeds 1 MiB")
    value = json.loads(path.read_text(encoding="utf-8"))
    if not isinstance(value, dict):
        raise TypeError("fixture must be an object")
    return value


def _validate_nodes(raw: Any) -> list[dict[str, Any]]:
    """@brief Constrain node observations before grouping."""
    if not isinstance(raw, list) or len(raw) > MAX_NODES:
        raise ValueError("nodes must be an array of at most 32 entries")
    seen: set[str] = set()
    nodes: list[dict[str, Any]] = []
    for node in raw:
        if not isinstance(node, dict):
            raise TypeError("node must be an object")
        name = node.get("name")
        parent = node.get("usb_parent")
        vendor = node.get("vendor")
        product = node.get("product")
        serial = node.get("serial")
        interface = node.get("interface")
        capabilities = node.get("capabilities")
        formats = node.get("formats")
        if not isinstance(name, str) or not NODE_NAME.fullmatch(name) or name in seen:
            raise ValueError("invalid or duplicate video node")
        if not isinstance(parent, str) or not 1 <= len(parent) <= 64:
            raise ValueError("invalid USB parent")
        if not isinstance(vendor, str) or not USB_ID.fullmatch(vendor):
            raise ValueError("invalid USB vendor")
        if not isinstance(product, str) or not USB_ID.fullmatch(product):
            raise ValueError("invalid USB product")
        if not isinstance(serial, str) or len(serial) > 128:
            raise ValueError("invalid USB serial")
        if not isinstance(interface, str) or len(interface) > 16:
            raise ValueError("invalid interface")
        if (
            not isinstance(capabilities, list)
            or len(capabilities) > 32
            or any(
                not isinstance(value, str) or len(value) > 64 for value in capabilities
            )
        ):
            raise ValueError("invalid capabilities")
        if (
            not isinstance(formats, list)
            or len(formats) > 64
            or any(not isinstance(value, str) or len(value) > 16 for value in formats)
        ):
            raise ValueError("invalid formats")
        seen.add(name)
        nodes.append(
            {
                "name": name,
                "usb_parent": parent,
                "vendor": vendor.lower(),
                "product": product.lower(),
                "serial": serial,
                "interface": interface,
                "capabilities": sorted(set(capabilities)),
                "formats": sorted(set(formats)),
                "by_id": node.get("by_id", []),
            }
        )
    return sorted(nodes, key=lambda item: int(item["name"][5:]))


def _group_nodes(nodes: list[dict[str, Any]]) -> list[dict[str, Any]]:
    """@brief Keep physical and endpoint identities separate."""
    groups: dict[str, dict[str, Any]] = {}
    for node in nodes:
        key = node["usb_parent"]
        group = groups.setdefault(
            key,
            {
                "usb_parent": key,
                "vendor": node["vendor"],
                "product": node["product"],
                "serial": node["serial"],
                "endpoints": [],
                "selected_endpoint": None,
                "unusable_reason": None,
            },
        )
        if any(
            group[field] != node[field] for field in ("vendor", "product", "serial")
        ):
            raise ValueError("inconsistent USB identity within one physical device")
        group["endpoints"].append(
            {
                "name": node["name"],
                "interface": node["interface"],
                "capabilities": node["capabilities"],
                "formats": node["formats"],
                "by_id": node["by_id"],
            }
        )
    for group in groups.values():
        for endpoint in group["endpoints"]:
            if "VIDEO_CAPTURE" in endpoint["capabilities"] and any(
                value in endpoint["formats"] for value in ("MJPG", "YUYV")
            ):
                group["selected_endpoint"] = endpoint["name"]
                break
        if group["selected_endpoint"] is None:
            group["unusable_reason"] = "no validated MJPEG/YUYV capture endpoint"
    return [groups[key] for key in sorted(groups)]


def _target_profile() -> str | None:
    """@brief Recognise only the two planned appliance target profiles."""
    if platform.system() != "Linux":
        return None
    release = platform.release().casefold()
    if (
        "microsoft" in release
        or "wsl" in release
        or Path("/proc/sys/fs/binfmt_misc/WSLInterop").exists()
    ):
        return None
    machine = platform.machine().lower()
    try:
        os_release = Path("/etc/os-release").read_text(encoding="utf-8")
    except OSError:
        return None
    values = dict(line.split("=", 1) for line in os_release.splitlines() if "=" in line)
    os_id = values.get("ID", "").strip('"')
    version = values.get("VERSION_ID", "").strip('"')
    if (
        machine in ("aarch64", "arm64")
        and os_id in ("debian", "raspbian")
        and version == "13"
    ):
        try:
            model = (
                Path("/proc/device-tree/model")
                .read_bytes()[:128]
                .decode("utf-8", errors="replace")
            )
        except OSError:
            return None
        if "Raspberry Pi 4" in model:
            return "pi-arm64"
    if machine in ("x86_64", "amd64") and os_id == "ubuntu" and version == "24.04":
        return "ubuntu-amd64"
    return None


def _usb_identity(device_path: Path) -> tuple[Path | None, str]:
    """@brief Find the interface and physical USB parent from a video device."""
    interface = ""
    for ancestor in (device_path, *device_path.parents):
        if not interface and (ancestor / "bInterfaceNumber").exists():
            interface = _small_text(ancestor / "bInterfaceNumber")
        if (ancestor / "idVendor").exists() and (ancestor / "idProduct").exists():
            return ancestor, interface
    return None, interface


def _node_from_sysfs(name: str) -> dict[str, Any] | None:
    """@brief Resolve a character device to its USB parent and interface."""
    sys_node = Path("/sys/class/video4linux") / name
    dev_node = Path("/dev") / name
    if not NODE_NAME.fullmatch(name) or not sys_node.exists() or not dev_node.exists():
        return None
    mode = dev_node.stat()
    if not stat.S_ISCHR(mode.st_mode):
        return None
    if (
        _small_text(sys_node / "dev")
        != f"{os.major(mode.st_rdev)}:{os.minor(mode.st_rdev)}"
    ):
        return None
    usb_parent, interface = _usb_identity((sys_node / "device").resolve())
    if usb_parent is None:
        return None
    by_id: list[str] = []
    link_dir = Path("/dev/v4l/by-id")
    if link_dir.is_dir():
        for link in sorted(link_dir.iterdir()):
            if link.is_symlink() and link.resolve() == dev_node:
                by_id.append(link.name)
    return {
        "name": name,
        "usb_parent": usb_parent.name,
        "vendor": _small_text(usb_parent / "idVendor"),
        "product": _small_text(usb_parent / "idProduct"),
        "serial": _small_text(usb_parent / "serial")
        if (usb_parent / "serial").exists()
        else "",
        "interface": interface,
        "capabilities": [],
        "formats": [],
        "by_id": by_id,
    }


def _hardware_nodes(selected: str) -> list[dict[str, Any]]:
    """@brief Enumerate only the physical USB device containing a selected node."""
    chosen = _node_from_sysfs(Path(selected).name)
    if chosen is None:
        raise ValueError("selected node is not a verified USB video character device")
    nodes = []
    for path in sorted(Path("/sys/class/video4linux").glob("video*")):
        item = _node_from_sysfs(path.name)
        if item is not None and item["usb_parent"] == chosen["usb_parent"]:
            nodes.append(item)
    return _validate_nodes(nodes)


def _run_worker(
    candidate: str, action: str, node: str, pixel_format: str | None = None
) -> dict[str, Any]:
    """@brief Bound one candidate operation by an owned subprocess deadline."""
    command = [
        sys.executable,
        str(WORKER),
        "--candidate",
        candidate,
        "--action",
        action,
        "--node",
        str(Path("/dev") / node),
    ]
    if pixel_format is not None:
        command.extend(["--format", pixel_format])
    started = datetime.now(timezone.utc)
    process = subprocess.Popen(
        command, stdout=subprocess.PIPE, stderr=subprocess.PIPE, start_new_session=True
    )
    try:
        stdout, stderr = process.communicate(timeout=12 if action == "capture" else 8)
    except subprocess.TimeoutExpired:
        os.killpg(process.pid, signal.SIGKILL)
        stdout, stderr = process.communicate(timeout=3)
        return {
            "status": "timeout",
            "duration_ms": int(
                (datetime.now(timezone.utc) - started).total_seconds() * 1000
            ),
        }
    duration = int((datetime.now(timezone.utc) - started).total_seconds() * 1000)
    if len(stdout) > MAX_CHILD_OUTPUT or len(stderr) > MAX_CHILD_OUTPUT:
        return {
            "status": "failed",
            "reason": "worker output exceeded limit",
            "duration_ms": duration,
        }
    try:
        value = json.loads(stdout.decode("utf-8"))
    except (UnicodeDecodeError, json.JSONDecodeError):
        return {
            "status": "failed",
            "reason": "invalid worker result",
            "duration_ms": duration,
        }
    if not isinstance(value, dict):
        return {
            "status": "failed",
            "reason": "invalid worker result",
            "duration_ms": duration,
        }
    value["duration_ms"] = duration
    if process.returncode != 0:
        value["status"] = "failed"
    return value


def _stop_process(process: subprocess.Popen[bytes]) -> bool:
    """@brief Stop an owned child and its process group where supported."""
    if process.poll() is not None:
        return False
    if os.name == "nt":
        process.terminate()
    else:
        os.killpg(process.pid, signal.SIGTERM)
    return True


def _run_cancellation(candidate: str, node: str) -> dict[str, Any]:
    """@brief Measure termination after a worker declares its stream ready."""
    command = [
        sys.executable,
        str(WORKER),
        "--candidate",
        candidate,
        "--action",
        "cancel",
        "--node",
        str(Path("/dev") / node),
        "--format",
        "MJPG",
    ]
    process = subprocess.Popen(
        command, stdout=subprocess.PIPE, stderr=subprocess.PIPE, start_new_session=True
    )
    assert process.stdout is not None
    ready_line: queue.Queue[bytes] = queue.Queue(maxsize=1)
    reader = threading.Thread(
        target=lambda: ready_line.put(process.stdout.readline()), daemon=True
    )
    reader.start()
    try:
        line = ready_line.get(timeout=5)
        ready = json.loads(line.decode("utf-8"))
        if ready.get("status") != "ready":
            raise ValueError("worker did not enter capture")
    except (queue.Empty, UnicodeDecodeError, json.JSONDecodeError, ValueError):
        _stop_process(process)
        try:
            process.communicate(timeout=3)
        except subprocess.TimeoutExpired:
            process.kill()
            process.communicate(timeout=3)
        return {"status": "failed", "reason": "worker never reported a ready capture"}
    time.sleep(0.05)
    started = time.monotonic()
    signalled = _stop_process(process)
    try:
        stdout, stderr = process.communicate(timeout=3)
    except subprocess.TimeoutExpired:
        process.kill()
        process.communicate(timeout=3)
        return {
            "status": "timeout",
            "stop_ms": int((time.monotonic() - started) * 1000),
        }
    elapsed = int((time.monotonic() - started) * 1000)
    if len(stdout) > MAX_CHILD_OUTPUT or len(stderr) > MAX_CHILD_OUTPUT:
        return {
            "status": "failed",
            "reason": "worker output exceeded limit",
            "stop_ms": elapsed,
        }
    return {
        "status": "ok"
        if signalled and process.returncode != 0 and elapsed < 3000
        else "failed",
        "stop_ms": elapsed,
    }


def _wheel_closure(wheelhouse: Path | None, lock: Path | None) -> dict[str, Any]:
    """@brief Check an explicitly supplied finite offline wheel inventory."""
    if wheelhouse is None or lock is None:
        return {"status": "not_run", "wheels": []}
    expected = _read_json(lock).get("wheels")
    if not isinstance(expected, dict) or len(expected) > 128 or not wheelhouse.is_dir():
        raise ValueError("invalid wheel lock or wheelhouse")
    if any(
        not isinstance(name, str)
        or not name.endswith(".whl")
        or Path(name).name != name
        or not isinstance(digest, str)
        or re.fullmatch(r"[0-9a-f]{64}", digest) is None
        for name, digest in expected.items()
    ):
        raise ValueError("invalid wheel lock entry")
    actual: dict[str, str] = {}
    for path in sorted(wheelhouse.iterdir()):
        if (
            not path.is_file()
            or path.suffix != ".whl"
            or path.stat().st_size > 100 * 1024 * 1024
        ):
            raise ValueError("wheelhouse contains an invalid file")
        digest = hashlib.sha256()
        with path.open("rb") as stream:
            for chunk in iter(lambda: stream.read(1024 * 1024), b""):
                digest.update(chunk)
        actual[path.name] = digest.hexdigest()
    return {
        "status": "verified" if actual and actual == expected else "failed",
        "wheels": [{"file": key, "sha256": value} for key, value in actual.items()],
    }


def _candidate_ready(result: dict[str, Any]) -> bool:
    """@brief Admit only a complete observed binding run to manual review."""
    if result.get("status") != "observed":
        return False
    checks = result.get("checks", [])
    if not any(
        item.get("action") == "self-check" and item.get("status") == "available"
        for item in checks
    ):
        return False
    if not any(
        item.get("action") == "controls" and item.get("status") == "ok"
        for item in checks
    ):
        return False
    if not any(
        item.get("action") == "cancellation" and item.get("status") == "ok"
        for item in checks
    ):
        return False
    captures = [item for item in checks if item.get("action") == "capture"]
    return (
        bool(captures)
        and all(item.get("status") in ("ok", "unsupported") for item in captures)
        and any(item.get("status") == "ok" for item in captures)
    )


def _atomic_text(path: Path, value: str) -> None:
    """@brief Replace a report only after its complete content is ready."""
    path.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.NamedTemporaryFile(
        "w", encoding="utf-8", dir=path.parent, prefix=".camera-", delete=False
    ) as stream:
        temporary = Path(stream.name)
        stream.write(value)
        stream.flush()
        os.fsync(stream.fileno())
    temporary.replace(path)


def _evidence(args: argparse.Namespace) -> dict[str, Any]:
    """@brief Collect one fixture or selected-hardware observation."""
    fixture = args.fixture is not None
    if fixture:
        nodes = _validate_nodes(_read_json(args.fixture).get("nodes"))
    else:
        if platform.system() != "Linux" or args.device is None or args.kind is None:
            raise ValueError("hardware probe requires Linux, --device and --kind")
        nodes = _hardware_nodes(args.device)
    devices = _group_nodes(nodes)
    profile = _target_profile() if not fixture else None
    matrix = [
        {"target": target, "device_kind": kind, "status": "not_run"}
        for target, kind in TARGETS
    ]
    candidate_results: list[dict[str, Any]] = []
    if not fixture:
        for candidate in CANDIDATES:
            candidate_nodes = [dict(node) for node in nodes]
            checks: list[dict[str, Any]] = [
                {
                    "action": "self-check",
                    **_run_worker(candidate, "self-check", nodes[0]["name"]),
                }
            ]
            for node in candidate_nodes:
                inspection = _run_worker(candidate, "inspect", node["name"])
                checks.append({"action": "inspect", "node": node["name"], **inspection})
                if inspection.get("status") == "ok":
                    node["capabilities"] = inspection.get("capabilities", [])
                    node["formats"] = inspection.get("formats", [])
            try:
                candidate_devices = _group_nodes(_validate_nodes(candidate_nodes))
            except ValueError as error:
                candidate_results.append(
                    {
                        "candidate": candidate,
                        "status": "invalid_inspection",
                        "reason": str(error),
                        "checks": checks,
                    }
                )
                continue
            selected = next(
                (
                    device
                    for device in candidate_devices
                    if device["selected_endpoint"] is not None
                ),
                None,
            )
            if selected is None:
                candidate_results.append(
                    {
                        "candidate": candidate,
                        "status": "unsupported_endpoint",
                        "checks": checks,
                    }
                )
                continue
            if not any(device["selected_endpoint"] is not None for device in devices):
                devices = candidate_devices
            node = selected["selected_endpoint"]
            checks.append(
                {"action": "controls", **_run_worker(candidate, "controls", node)}
            )
            endpoint = next(
                item for item in selected["endpoints"] if item["name"] == node
            )
            for pixel_format in ("MJPG", "YUYV"):
                if pixel_format in endpoint["formats"]:
                    checks.append(
                        {
                            "action": "capture",
                            "format": pixel_format,
                            **_run_worker(candidate, "capture", node, pixel_format),
                        }
                    )
                else:
                    checks.append(
                        {
                            "action": "capture",
                            "format": pixel_format,
                            "status": "unsupported",
                        }
                    )
            if "MJPG" in endpoint["formats"]:
                checks.append(
                    {"action": "cancellation", **_run_cancellation(candidate, node)}
                )
            candidate_results.append(
                {
                    "candidate": candidate,
                    "selected_endpoint": node,
                    "checks": checks,
                    "status": "observed",
                }
            )
    closure = _wheel_closure(args.wheelhouse, args.lock)
    eligible = (
        not fixture
        and profile is not None
        and closure["status"] == "verified"
        and any(_candidate_ready(result) for result in candidate_results)
    )
    if not fixture and profile is not None:
        for cell in matrix:
            if cell["target"] == profile and cell["device_kind"] == args.kind:
                cell["status"] = "needs_review" if eligible else "incomplete"
    return {
        "schema_version": SCHEMA_VERSION,
        "probe_sources": {
            "probe_sha256": hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
            "worker_sha256": hashlib.sha256(WORKER.read_bytes()).hexdigest(),
        },
        "mode": "fixture" if fixture else "hardware",
        "target_eligible": eligible,
        "target_profile": profile,
        "device_kind": args.kind if not fixture else None,
        "observed_at": datetime.now(timezone.utc).isoformat(),
        "host": {
            "system": platform.system(),
            "machine": platform.machine(),
            "kernel": platform.release(),
            "python": platform.python_version(),
        },
        "devices": devices,
        "candidates": candidate_results,
        "wheel_closure": closure,
        "acceptance_matrix": matrix,
    }


# PUBLIC API
def main(argv: list[str] | None = None) -> int:
    """@brief Validate arguments and write one reviewable evidence record."""
    parser = argparse.ArgumentParser(description=__doc__)
    source = parser.add_mutually_exclusive_group(required=True)
    source.add_argument(
        "--fixture", type=Path, help="bounded JSON fixture; never hardware evidence"
    )
    source.add_argument("--device", help="selected /dev/videoN for a local target run")
    parser.add_argument(
        "--kind", choices=KINDS, help="physical device class verified by operator"
    )
    parser.add_argument(
        "--output", type=Path, required=True, help="evidence output directory"
    )
    parser.add_argument("--wheelhouse", type=Path, help="offline wheel directory")
    parser.add_argument(
        "--lock", type=Path, help="JSON object mapping wheel filenames to SHA-256"
    )
    args = parser.parse_args(argv)
    try:
        if args.device is not None and not re.fullmatch(
            r"/dev/video[0-9]{1,3}", args.device
        ):
            raise ValueError("--device must be /dev/videoN")
        if (args.wheelhouse is None) != (args.lock is None):
            raise ValueError("--wheelhouse and --lock must be supplied together")
        evidence = _evidence(args)
        _atomic_text(
            args.output / "evidence.json",
            json.dumps(evidence, indent=2, sort_keys=True) + "\n",
        )
        lines = [
            f"mode: {evidence['mode']}",
            f"target eligible: {evidence['target_eligible']}",
            f"physical devices: {len(evidence['devices'])}",
        ]
        lines.extend(
            f"{cell['target']} {cell['device_kind']}: {cell['status']}"
            for cell in evidence["acceptance_matrix"]
        )
        _atomic_text(args.output / "summary.txt", "\n".join(lines) + "\n")
        if evidence["mode"] == "hardware" and not evidence["target_eligible"]:
            return 3
        return 0
    except (OSError, TypeError, ValueError, json.JSONDecodeError) as error:
        print(f"camera probe: {error}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    raise SystemExit(main())

# END OF FILE
