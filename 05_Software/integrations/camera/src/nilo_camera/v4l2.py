#!/usr/bin/env python3
# -*- coding: utf-8 -*-
# -----------------------------------------------------------------------------
# Copyright (c) 2026 DDP Engineering
# SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
# -----------------------------------------------------------------------------
"""@file v4l2.py
@brief Discover USB video nodes and isolate usable V4L2 capture endpoints.
"""

from __future__ import annotations

import io
import os
import re
import stat
from pathlib import Path

from PIL import Image

NODE = re.compile(r"video[0-9]{1,3}\Z")
SUPPORTED_FORMATS = ("MJPG", "YUYV")
MAX_FRAME_BYTES = 8 << 20
MAX_PIXELS = 2_000_000


def control_name(value: str) -> str:
    """@brief Give a typed V4L2 control a stable public machine name."""
    name = re.sub(r"[^a-z0-9]+", "_", value.lower()).strip("_")
    return {
        "white_balance_automatic": "auto_white_balance",
        "white_balance_temperature": "white_balance_temperature",
    }.get(name, name)


def _supported_control(control: object) -> bool:
    return int(getattr(control, "type", 0)) in (1, 2, 3, 9) and not bool(
        getattr(control, "is_disabled", False)
    )


def _control_value(camera: object, control: object) -> int | bool:
    value = camera.get_control_value(control)
    if hasattr(value, "index"):
        return int(value.index)
    if int(control.type) == 2:
        return bool(value)
    return int(value)


def read_controls(node: str) -> tuple[list[dict], dict[str, int | bool]]:
    """@brief Report finite typed controls and actual hardware read-back values."""
    from pyrav4l2 import Device

    camera = Device(node)
    descriptions: list[dict] = []
    values: dict[str, int | bool] = {}
    for control in list(camera.controls)[:64]:
        name = control_name(str(control.name))
        if not name or name in values:
            continue
        kind = int(getattr(control, "type", 0))
        supported = _supported_control(control)
        descriptor = {
            "id": name,
            "v4l2_id": int(control.id),
            "display_name": str(control.name)[:80],
            "type": {1: "integer", 2: "boolean", 3: "menu", 9: "menu"}.get(kind, "unsupported"),
            "minimum": int(getattr(control, "minimum", 0)),
            "maximum": int(getattr(control, "maximum", 0)),
            "step": int(getattr(control, "step", 0)),
            "default": (
                bool(getattr(control, "default_value", 0))
                if kind == 2
                else int(getattr(control, "default_value", 0))
            ),
            "read_only": bool(getattr(control, "is_read_only", False)) or not supported,
            "inactive": bool(getattr(control, "is_inactive", False)),
            "volatile": bool(getattr(control, "is_volatile", False)),
        }
        if kind in (3, 9):
            descriptor["choices"] = [
                {
                    "value": int(item.index),
                    "label": str(getattr(item, "name", getattr(item, "value", item.index)))[:80],
                }
                for item in getattr(control, "items", [])[:64]
            ]
        if supported:
            try:
                values[name] = _control_value(camera, control)
                descriptor["current"] = values[name]
            except (OSError, ValueError, StopIteration):
                descriptor["read_only"] = True
                descriptor["reason"] = "readback_unavailable"
        else:
            descriptor["reason"] = "unsupported_control_type"
        descriptions.append(descriptor)
    return descriptions, values


def apply_controls(node: str, updates: dict[str, int | bool]) -> dict[str, int | bool]:
    """@brief Validate typed writes before changing hardware and return read-back."""
    from pyrav4l2 import Device

    if not isinstance(updates, dict) or not updates or len(updates) > 64:
        raise ValueError("invalid control update")
    camera = Device(node)
    controls = {control_name(str(control.name)): control for control in camera.controls}
    prepared = []
    for name, value in updates.items():
        control = controls.get(name)
        if (
            control is None
            or not _supported_control(control)
            or bool(getattr(control, "is_read_only", False))
        ):
            raise ValueError("unsupported control")
        kind = int(control.type)
        if kind == 2:
            if type(value) is not bool:
                raise ValueError("control outside range")
            chosen = value
        elif kind in (3, 9):
            if type(value) is not int:
                raise ValueError("control outside range")
            chosen = next((item for item in control.items if int(item.index) == value), None)
            if chosen is None:
                raise ValueError("control outside range")
        else:
            minimum, maximum, step = int(control.minimum), int(control.maximum), int(control.step)
            if (
                type(value) is not int
                or step < 1
                or value < minimum
                or value > maximum
                or (value - minimum) % step
            ):
                raise ValueError("control outside range")
            chosen = value
        prepared.append((name, control, chosen))
    prepared.sort(key=lambda item: 0 if item[0].startswith("auto_") else 1)
    originals = [(control, camera.get_control_value(control)) for _, control, _ in prepared]
    try:
        for _, control, chosen in prepared:
            camera.set_control_value(control, chosen)
        actual = {name: _control_value(camera, control) for name, control, _ in prepared}
    except Exception:
        for control, original in reversed(originals):
            try:
                camera.set_control_value(control, original)
            except OSError:
                pass
        raise
    return actual


def frame_to_image(frame: bytes, pixel_format: str, width: int, height: int) -> Image.Image:
    """@brief Decode one bounded frame into a fresh metadata-free RGB image."""
    if width < 1 or height < 1 or width * height > MAX_PIXELS or len(frame) > MAX_FRAME_BYTES:
        raise ValueError("frame exceeds decode limit")
    if pixel_format == "MJPG":
        with Image.open(io.BytesIO(frame)) as source:
            if source.size != (width, height) or source.width * source.height > MAX_PIXELS:
                raise ValueError("decoded image exceeds decode limit")
            source.load()
            clean = Image.new("RGB", source.size)
            clean.paste(source.convert("RGB"))
            return clean
    if pixel_format != "YUYV":
        raise ValueError("unsupported pixel format")
    if width % 2 or len(frame) < width * height * 2:
        raise ValueError("short YUYV frame")
    rgb = bytearray(width * height * 3)
    source = memoryview(frame)
    for index in range(0, width * height, 2):
        base = index * 2
        y0, u, y1, v = source[base : base + 4]
        for offset, luminance in ((0, y0), (1, y1)):
            c, d, e = luminance - 16, u - 128, v - 128
            target = (index + offset) * 3
            rgb[target] = max(0, min(255, (298 * c + 409 * e + 128) >> 8))
            rgb[target + 1] = max(0, min(255, (298 * c - 100 * d - 208 * e + 128) >> 8))
            rgb[target + 2] = max(0, min(255, (298 * c + 516 * d + 128) >> 8))
    return Image.frombytes("RGB", (width, height), bytes(rgb))


def format_name(value: object) -> str:
    """@brief Normalise the two supported V4L2 pixel formats."""
    fourcc = getattr(value, "pixelformat", None)
    if isinstance(fourcc, int):
        code = fourcc.to_bytes(4, "little", signed=False).decode("ascii", errors="replace")
        if code in SUPPORTED_FORMATS:
            return code
    name = str(getattr(value, "name", value)).upper()
    if "MJPG" in name or "MJPEG" in name or "MOTION-JPEG" in name:
        return "MJPG"
    if "YUYV" in name or "YUY2" in name:
        return "YUYV"
    return ""


def group_observed_nodes(nodes: list[dict]) -> list[dict]:
    """@brief Return one camera per physical USB parent in stable port order."""
    groups: dict[str, dict] = {}
    for node in nodes:
        parent = node["usb_parent"]
        group = groups.setdefault(
            parent,
            {
                "vendor": node["vendor"],
                "product": node["product"],
                "serial": node["serial"],
                "port_path": parent,
                "endpoints": [],
                "selected_endpoint": None,
            },
        )
        if any(group[key] != node[key] for key in ("vendor", "product", "serial")):
            raise ValueError("inconsistent USB identity")
        group["endpoints"].append(
            {
                "id": node["name"],
                "interface": node["interface"],
                "capture": node["capture"],
                "formats": node["formats"],
                "sizes": node.get("sizes", {}),
                "frame_intervals": node.get("frame_intervals", []),
            }
        )
    devices = []
    for index, parent in enumerate(sorted(groups), 1):
        device = groups[parent]
        device["id"] = f"cam-{index}"
        device["endpoints"].sort(key=lambda item: int(item["id"][5:]))
        for endpoint in device["endpoints"]:
            if endpoint["capture"] and any(
                item in SUPPORTED_FORMATS for item in endpoint["formats"]
            ):
                device["selected_endpoint"] = endpoint["id"]
                break
        if device["selected_endpoint"] is None:
            device["unusable_reason"] = "no supported video capture endpoint"
        devices.append(device)
    return devices


def _small_text(path: Path) -> str:
    value = path.read_bytes()
    if len(value) > 128:
        raise ValueError("oversized USB identity field")
    return value.decode("utf-8", errors="replace").strip()


def _usb_parent(path: Path) -> tuple[Path | None, str]:
    interface = ""
    for parent in (path, *path.parents):
        if not interface and (parent / "bInterfaceNumber").exists():
            interface = _small_text(parent / "bInterfaceNumber")
        if (parent / "idVendor").exists() and (parent / "idProduct").exists():
            return parent, interface
    return None, interface


def scan_usb_cameras() -> list[dict]:
    """@brief Verify each device node through sysfs before V4L2 inspection."""
    from pyrav4l2 import Device

    nodes = []
    for sys_node in sorted(Path("/sys/class/video4linux").glob("video*")):
        name = sys_node.name
        dev_node = Path("/dev") / name
        if not NODE.fullmatch(name) or not dev_node.exists():
            continue
        info = dev_node.stat()
        if not stat.S_ISCHR(info.st_mode):
            continue
        if _small_text(sys_node / "dev") != f"{os.major(info.st_rdev)}:{os.minor(info.st_rdev)}":
            continue
        parent, interface = _usb_parent((sys_node / "device").resolve())
        if parent is None:
            continue
        try:
            camera = Device(str(dev_node))
            capture = bool(camera.is_video_capture_capable)
            formats = (
                sorted({format_name(item) for item in camera.available_formats}) if capture else []
            )
            sizes = (
                {
                    format_name(pixel_format): sorted(
                        {
                            (int(size.width), int(size.height))
                            for size in available
                            if 0 < int(size.width) * int(size.height) <= MAX_PIXELS
                        }
                    )[:64]
                    for pixel_format, available in camera.available_formats.items()
                    if capture and format_name(pixel_format) in SUPPORTED_FORMATS
                }
                if capture
                else {}
            )
            frame_intervals = []
            if capture:
                for pixel_format, available in camera.available_formats.items():
                    format_id = format_name(pixel_format)
                    if format_id not in SUPPORTED_FORMATS:
                        continue
                    for size in available[:64]:
                        width, height = int(size.width), int(size.height)
                        if width < 1 or height < 1 or width * height > MAX_PIXELS:
                            continue
                        for interval in camera.get_available_frame_intervals(pixel_format, size)[:64]:
                            numerator = int(interval.numerator)
                            denominator = int(interval.denominator)
                            if numerator > 0 and denominator > 0:
                                frame_intervals.append({
                                    "format": format_id, "width": width, "height": height,
                                    "numerator": numerator, "denominator": denominator,
                                })
        except (OSError, ValueError):
            capture, formats, sizes, frame_intervals = False, [], {}, []
        nodes.append(
            {
                "name": name,
                "usb_parent": parent.name,
                "vendor": _small_text(parent / "idVendor"),
                "product": _small_text(parent / "idProduct"),
                "serial": _small_text(parent / "serial") if (parent / "serial").exists() else "",
                "interface": interface,
                "capture": capture,
                "formats": [item for item in formats if item in SUPPORTED_FORMATS],
                "sizes": sizes,
                "frame_intervals": frame_intervals,
            }
        )
    devices = group_observed_nodes(nodes)
    for device in devices:
        if device["selected_endpoint"]:
            try:
                device["controls"], _ = read_controls("/dev/" + device["selected_endpoint"])
            except (OSError, ValueError):
                device["controls"] = []
    return devices


def resolve_capture_node(identity: dict, endpoint_id: str) -> tuple[str, dict]:
    """@brief Revalidate a core-selected physical binding immediately before I/O."""
    if (
        not isinstance(identity, dict)
        or set(identity) != {"vendor", "product", "serial", "port_path"}
        or any(not isinstance(value, str) for value in identity.values())
        or not isinstance(endpoint_id, str)
        or not NODE.fullmatch(endpoint_id)
    ):
        raise ValueError("invalid camera binding")
    for camera in scan_usb_cameras():
        if any(camera.get(key) != value for key, value in identity.items()):
            continue
        if camera.get("selected_endpoint") != endpoint_id:
            continue
        for endpoint in camera["endpoints"]:
            if endpoint["id"] == endpoint_id and endpoint["capture"]:
                return f"/dev/{endpoint_id}", endpoint
    raise ValueError("camera binding is absent")


def capture_image(
    node: str, pixel_format: str, width: int, height: int,
    frame_interval: dict[str, int] | None = None,
) -> Image.Image:
    """@brief Capture the third frame at an advertised size and format."""
    from pyrav4l2 import Device, Stream

    if (
        pixel_format not in SUPPORTED_FORMATS
        or width < 1
        or height < 1
        or width * height > MAX_PIXELS
    ):
        raise ValueError("capture request exceeds limit")
    camera = Device(node)
    choices = [
        (available_format, size)
        for available_format, sizes in camera.available_formats.items()
        if format_name(available_format) == pixel_format
        for size in sizes
        if int(size.width) == width and int(size.height) == height
    ]
    if not choices:
        raise ValueError("capture format or size is unavailable")
    camera.set_format(*choices[0])
    _, actual = camera.get_format()
    if int(actual.width) != width or int(actual.height) != height:
        raise ValueError("capture format changed unexpectedly")
    if frame_interval is not None:
        intervals = camera.get_available_frame_intervals(*choices[0])
        chosen = next((item for item in intervals if
                       int(item.numerator) == frame_interval["numerator"] and
                       int(item.denominator) == frame_interval["denominator"]), None)
        if chosen is None:
            raise ValueError("capture frame interval is unavailable")
        camera.set_frame_interval(chosen)
        actual_interval = camera.get_frame_interval()
        if (int(actual_interval.numerator), int(actual_interval.denominator)) != (
            frame_interval["numerator"], frame_interval["denominator"]
        ):
            raise ValueError("capture frame interval changed unexpectedly")
    stream = iter(Stream(camera))
    try:
        next(stream)
        next(stream)
        frame = bytes(next(stream))
    finally:
        close = getattr(stream, "close", None)
        if close is not None:
            close()
    return frame_to_image(frame, pixel_format, width, height)


# END OF FILE
