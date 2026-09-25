#!/usr/bin/env python3
# -*- coding: utf-8 -*-  # noqa: UP009 - repository Python style
# -----------------------------------------------------------------------------
# Copyright (c) 2026 DDP Engineering
# SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
# -----------------------------------------------------------------------------
"""@file candidate_probe.py
@brief Isolate one optional Python V4L2 binding operation in a child process.
@details No result from this worker alone certifies target hardware support.
"""

# IMPORTS
from __future__ import annotations

import argparse
import hashlib
import importlib.util
import io
import json
import platform
import time
from importlib import metadata
from typing import Any

# CONSTANTS
MAX_FRAME_BYTES = 8 * 1024 * 1024
MAX_PIXELS = 2 * 1024 * 1024
MAX_CONTROLS = 64
CANDIDATES = ("linuxpy", "pyrav4l2")


# HELPER FUNCTIONS
def _format_name(value: Any) -> str:
    """@brief Normalise only the two required V4L2 pixel formats."""
    fourcc = (
        value
        if isinstance(value, int)
        else getattr(value, "pixel_format", getattr(value, "pixelformat", None))
    )
    if isinstance(fourcc, int):
        decoded = (
            fourcc.to_bytes(4, "little", signed=False)
            .decode("ascii", errors="replace")
            .upper()
        )
        if decoded in ("MJPG", "YUYV"):
            return decoded
    name = str(getattr(value, "name", value)).upper()
    if "MJPG" in name or "MJPEG" in name or "MOTION-JPEG" in name:
        return "MJPG"
    if "YUYV" in name or "YUY2" in name:
        return "YUYV"
    return name[:32]


def _linuxpy_format_name(value: Any) -> str:
    """@brief Read the format field supplied by the pinned linuxpy version."""
    return _format_name(value)


def _linuxpy_capture_formats(values: Any) -> list[str]:
    """@brief Keep only pixel formats that the bounded capture probe supports."""
    return sorted(
        {name for item in values if (name := _linuxpy_format_name(item)) in ("MJPG", "YUYV")}
    )


def _capabilities(value: Any) -> list[str]:
    """@brief Convert a flag value to stable capability names."""
    text = str(getattr(value, "name", value)).upper()
    return [
        name
        for name in ("VIDEO_CAPTURE", "META_CAPTURE", "STREAMING", "READWRITE")
        if name in text
    ]


def _control_description(control: Any) -> dict[str, Any]:
    """@brief Keep a bounded set of typed control attributes."""
    result = {
        "id": str(getattr(control, "id", getattr(control, "ctrl_id", "")))[:32],
        "name": str(getattr(control, "name", ""))[:80],
        "type": type(control).__name__[:40],
        "flags": str(getattr(control, "flags", ""))[:80],
    }
    for key in ("min", "max", "step", "default"):
        source = {"min": "minimum", "max": "maximum"}.get(key, key)
        value = getattr(control, key, getattr(control, source, None))
        if isinstance(value, (int, float, bool)):
            result[key] = value
    return result


def _alternate(original: Any, control: Any) -> Any | None:
    """@brief Choose one reversible value in a reported Boolean/integer range."""
    if isinstance(original, bool):
        return not original
    minimum = getattr(control, "min", getattr(control, "minimum", None))
    maximum = getattr(control, "max", getattr(control, "maximum", None))
    step = getattr(control, "step", 1)
    if (
        isinstance(original, int)
        and all(isinstance(item, int) for item in (minimum, maximum, step))
        and step > 0
    ):
        if original + step <= maximum:
            return original + step
        if original - step >= minimum:
            return original - step
    return None


def _decode_image(
    frame: bytes, pixel_format: str, width: int, height: int
) -> dict[str, Any]:
    """@brief Exercise Pillow without retaining a frame in evidence."""
    if (
        len(frame) > MAX_FRAME_BYTES
        or width < 1
        or height < 1
        or width * height > MAX_PIXELS
    ):
        raise ValueError("frame exceeds probe limit")
    from PIL import Image

    if pixel_format == "MJPG":
        with Image.open(io.BytesIO(frame)) as image:
            decoded_width, decoded_height = image.size
            if (
                decoded_width < 1
                or decoded_height < 1
                or decoded_width * decoded_height > MAX_PIXELS
            ):
                raise ValueError("decoded image exceeds probe limit")
            image.load()
            decoded_size = list(image.size)
    else:
        if len(frame) < width * height * 2:
            raise ValueError("short YUYV frame")
        # YUYV pairs share chroma. A small bounded conversion checks Pillow use.
        rgb = bytearray(width * height * 3)
        source = memoryview(frame)
        for index in range(0, width * height, 2):
            base = index * 2
            y0, u, y1, v = source[base : base + 4]
            for offset, y in ((0, y0), (1, y1)):
                c, d, e = int(y) - 16, int(u) - 128, int(v) - 128
                target = (index + offset) * 3
                rgb[target] = max(0, min(255, (298 * c + 409 * e + 128) >> 8))
                rgb[target + 1] = max(
                    0, min(255, (298 * c - 100 * d - 208 * e + 128) >> 8)
                )
                rgb[target + 2] = max(0, min(255, (298 * c + 516 * d + 128) >> 8))
        image = Image.frombytes("RGB", (width, height), bytes(rgb))
        decoded_size = list(image.size)
        image.close()
    return {
        "frame_bytes": len(frame),
        "frame_sha256": hashlib.sha256(frame).hexdigest(),
        "decoded_size": decoded_size,
    }


def _linuxpy(action: str, node: str, pixel_format: str | None) -> dict[str, Any]:
    """@brief Exercise linuxpy using its documented Device/VideoCapture API."""
    from linuxpy.video.device import BufferType, Device, VideoCapture

    with Device(node) as camera:
        if action == "inspect":
            formats = _linuxpy_capture_formats(camera.info.formats)
            return {
                "status": "ok",
                "capabilities": _capabilities(camera.info.device_capabilities),
                "formats": formats,
            }
        if action == "controls":
            controls = list(camera.controls.values())[:MAX_CONTROLS]
            descriptions = [_control_description(control) for control in controls]
            writable = next(
                (
                    control
                    for control in controls
                    if _alternate(control.value, control) is not None
                    and "INACTIVE" not in str(getattr(control, "flags", "")).upper()
                ),
                None,
            )
            if writable is None:
                return {
                    "status": "unsupported",
                    "controls": descriptions,
                    "reason": "no reversible integer/Boolean control",
                }
            original = writable.value
            attempted = _alternate(original, writable)
            restored = False
            try:
                writable.value = attempted
                readback = writable.value
            finally:
                writable.value = original
                restored = writable.value == original
            return {
                "status": "ok" if readback == attempted and restored else "failed",
                "controls": descriptions,
                "control_test": {
                    "id": str(writable.id),
                    "original": original,
                    "attempted": attempted,
                    "readback": readback,
                    "restored": restored,
                },
            }
        if action == "capture":
            assert pixel_format is not None
            capture = VideoCapture(camera)
            capture.set_format(640, 480, pixel_format)
            actual = camera.get_format(BufferType.VIDEO_CAPTURE)
            width, height = int(actual.width), int(actual.height)
            if width * height > MAX_PIXELS:
                raise ValueError("configured capture exceeds pixel limit")
            started = time.monotonic()
            with capture:
                iterator = iter(capture)
                next(iterator)
                next(iterator)
                frame = bytes(next(iterator))
            result = _decode_image(frame, pixel_format, width, height)
            result.update(
                {
                    "status": "ok",
                    "width": width,
                    "height": height,
                    "warmup_frames": 2,
                    "capture_ms": int((time.monotonic() - started) * 1000),
                }
            )
            return result
        if action == "cancel":
            assert pixel_format is not None
            capture = VideoCapture(camera)
            capture.set_format(640, 480, pixel_format)
            with capture:
                print(json.dumps({"status": "ready"}), flush=True)
                for _ in capture:
                    pass
    raise ValueError("unknown worker action")


def _pyrav4l2(action: str, node: str, pixel_format: str | None) -> dict[str, Any]:
    """@brief Exercise pyrav4l2 using its documented Device/Stream API."""
    from pyrav4l2 import Device, Stream

    camera = Device(node)
    if action == "inspect":
        capable = camera.is_video_capture_capable
        formats = (
            sorted({_format_name(key) for key in camera.available_formats})
            if capable
            else []
        )
        capabilities = ["VIDEO_CAPTURE", "STREAMING"] if capable else []
        return {"status": "ok", "capabilities": capabilities, "formats": formats}
    if action == "controls":
        controls = list(camera.controls)[:MAX_CONTROLS]
        descriptions = [_control_description(control) for control in controls]
        if not hasattr(camera, "get_control_value") or not hasattr(
            camera, "set_control_value"
        ):
            return {
                "status": "unsupported",
                "controls": descriptions,
                "reason": "binding has no typed set/read-back API",
            }
        for control in controls:
            original = camera.get_control_value(control)
            attempted = _alternate(original, control)
            if attempted is None:
                continue
            restored = False
            try:
                camera.set_control_value(control, attempted)
                readback = camera.get_control_value(control)
            finally:
                camera.set_control_value(control, original)
                restored = camera.get_control_value(control) == original
            return {
                "status": "ok" if readback == attempted and restored else "failed",
                "controls": descriptions,
                "control_test": {
                    "id": str(getattr(control, "id", "")),
                    "original": original,
                    "attempted": attempted,
                    "readback": readback,
                    "restored": restored,
                },
            }
        return {
            "status": "unsupported",
            "controls": descriptions,
            "reason": "no reversible integer/Boolean control",
        }
    if action == "capture":
        assert pixel_format is not None
        options = [
            (key, size)
            for key, sizes in camera.available_formats.items()
            if _format_name(key) == pixel_format
            for size in sizes
        ]
        if not options:
            return {"status": "unsupported", "reason": "pixel format not advertised"}
        chosen_format, chosen_size = min(
            options, key=lambda item: int(item[1].width) * int(item[1].height)
        )
        camera.set_format(chosen_format, chosen_size)
        _, actual_size = camera.get_format()
        width = int(
            getattr(
                actual_size,
                "width",
                actual_size[0] if isinstance(actual_size, (tuple, list)) else 0,
            )
        )
        height = int(
            getattr(
                actual_size,
                "height",
                actual_size[1] if isinstance(actual_size, (tuple, list)) else 0,
            )
        )
        if width * height > MAX_PIXELS:
            raise ValueError("configured capture exceeds pixel limit")
        started = time.monotonic()
        iterator = iter(Stream(camera))
        try:
            next(iterator)
            next(iterator)
            frame = bytes(next(iterator))
        finally:
            close = getattr(iterator, "close", None)
            if close is not None:
                close()
        result = _decode_image(frame, pixel_format, width, height)
        result.update(
            {
                "status": "ok",
                "width": width,
                "height": height,
                "warmup_frames": 2,
                "capture_ms": int((time.monotonic() - started) * 1000),
            }
        )
        return result
    if action == "cancel":
        stream = iter(Stream(camera))
        try:
            next(stream)
            print(json.dumps({"status": "ready"}), flush=True)
            for _ in stream:
                pass
        finally:
            close = getattr(stream, "close", None)
            if close is not None:
                close()
    raise ValueError("unknown worker action")


# PUBLIC API
def main(argv: list[str] | None = None) -> int:
    """@brief Emit one bounded JSON result for the parent probe."""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--candidate", choices=CANDIDATES, required=True)
    parser.add_argument(
        "--action",
        choices=("self-check", "inspect", "controls", "capture", "cancel"),
        required=True,
    )
    parser.add_argument("--node")
    parser.add_argument("--format", choices=("MJPG", "YUYV"))
    args = parser.parse_args(argv)
    result: dict[str, Any] = {"candidate": args.candidate}
    try:
        available = importlib.util.find_spec(args.candidate) is not None
        if args.action == "self-check":
            result["status"] = "available" if available else "unavailable"
            if available:
                try:
                    result["version"] = metadata.version(args.candidate)
                    package = metadata.metadata(args.candidate)
                    result["license"] = (
                        package.get("License-Expression")
                        or package.get("License")
                        or "unknown"
                    )
                except metadata.PackageNotFoundError:
                    result["version"] = "unknown"
                    result["license"] = "unknown"
            try:
                result["pillow_version"] = metadata.version("Pillow")
            except metadata.PackageNotFoundError:
                result["pillow_version"] = "unavailable"
            print(json.dumps(result, sort_keys=True))
            return 0
        if platform.system() != "Linux":
            raise ValueError("V4L2 candidate requires Linux")
        if not available:
            raise ValueError("candidate dependency is unavailable")
        if args.node is None or not __import__("re").fullmatch(
            r"/dev/video[0-9]{1,3}", args.node
        ):
            raise ValueError("node must be /dev/videoN")
        if args.action in ("capture", "cancel") and args.format is None:
            raise ValueError("capture requires --format")
        probe = _linuxpy if args.candidate == "linuxpy" else _pyrav4l2
        result.update(probe(args.action, args.node, args.format))
        print(json.dumps(result, sort_keys=True))
        return 0 if result.get("status") in ("ok", "unsupported") else 1
    except Exception as error:  # noqa: BLE001 - isolate optional native binding faults
        result.update(
            {
                "status": "failed",
                "reason": f"{type(error).__name__}: {str(error)[:160]}",
            }
        )
        print(json.dumps(result, sort_keys=True))
        return 1


if __name__ == "__main__":
    raise SystemExit(main())

# END OF FILE
