#!/usr/bin/env python3
# -*- coding: utf-8 -*-
# -----------------------------------------------------------------------------
# Copyright (c) 2026 DDP Engineering
# SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
# -----------------------------------------------------------------------------
"""@file workspace.py
@brief Supervise a physical camera and its processed media.
@details Core owns authorization, paths and limits; the Unix channel is private.
"""

# IMPORTS
from __future__ import annotations

import hashlib
import io
import json
import logging
import os
import select
import shutil
import socketserver
import subprocess
import threading
import time
from datetime import UTC, datetime
from http.server import BaseHTTPRequestHandler
from pathlib import Path
from uuid import UUID
from zoneinfo import ZoneInfo

from PIL import ImageDraw, ImageFont

from nilo_camera import v4l2
from nilo_camera.processing import FONT_FILE, render_image, validate_processing

LOGGER = logging.getLogger(__name__)


def media_tool(name: str) -> str:
    """Prefer indexed package tools; bench bundles use the provisioned OS tools."""
    packaged = Path(__file__).resolve().parents[2] / "runtime" / "bin" / name
    return str(packaged if packaged.is_file() else Path("/usr/bin") / name)


def stamp_timezone(stamp: dict):
    """UTC works without an OS timezone database; named zones use appliance tzdata."""
    name = stamp.get("timezone", "UTC")
    return UTC if name == "UTC" else ZoneInfo(name)


def validate_settings(raw: dict) -> dict:
    """@brief Validate the complete work configuration before replacing a revision."""
    defaults = {
        "width": 640,
        "height": 480,
        "format": "MJPG",
        "fps": 15,
        "processing": {},
        "stamps": [],
        "encoding": "jpeg",
        "quality": 85,
        "mode": "video",
        "interval": 5,
        "playback_fps": 15,
        "video_quality": 23,
    }
    if not isinstance(raw, dict) or set(raw) - set(defaults):
        raise ValueError("invalid_settings")
    settings = {**defaults, **raw}
    for key, minimum, maximum in [
        ("width", 16, 4096),
        ("height", 16, 4096),
        ("fps", 1, 30),
        ("quality", 1, 95),
        ("playback_fps", 1, 30),
        ("interval", 1, 3600),
        ("video_quality", 18, 35),
    ]:
        if type(settings[key]) is not int or not minimum <= settings[key] <= maximum:
            raise ValueError("invalid_settings")
    if settings["width"] * settings["height"] > 2_000_000:
        raise ValueError("image_too_large")
    if settings["width"] % 2 or settings["height"] % 2:
        raise ValueError("invalid_settings")
    if (
        settings["format"] not in {"MJPG", "YUYV"}
        or settings["encoding"] not in {"jpeg", "png"}
        or settings["mode"] not in {"video", "timelapse"}
    ):
        raise ValueError("invalid_settings")
    settings["processing"] = validate_processing(settings["processing"])
    if not isinstance(settings["stamps"], list) or len(settings["stamps"]) > 3:
        raise ValueError("invalid_stamps")
    for stamp in settings["stamps"]:
        if not isinstance(stamp, dict) or set(stamp) - {
            "kind",
            "text",
            "position",
            "background",
            "size",
            "timezone",
        }:
            raise ValueError("invalid_stamp")
        if (
            stamp.get("kind") not in {"time", "name", "text"}
            or stamp.get("position", "top-left")
            not in {
                "top-left",
                "top-right",
                "bottom-left",
                "bottom-right",
                "top-center",
                "bottom-center",
            }
            or stamp.get("background", "transparent") not in {"transparent", "black", "white"}
            or type(stamp.get("size", 18)) is not int
            or not 8 <= stamp.get("size", 18) <= 48
            or not isinstance(stamp.get("text", ""), str)
            or len(stamp.get("text", "")) > 120
        ):
            raise ValueError("invalid_stamp")
        stamp_timezone(stamp)
    return settings


def compose(frame, settings: dict, device: dict, name: str, captured: datetime):
    """@brief Apply image transforms first, then readable output-positioned text."""
    image = render_image(
        frame, settings["processing"], device, settings["width"], settings["height"]
    )
    draw = ImageDraw.Draw(image)
    offsets: dict[str, int] = {}
    for stamp in settings["stamps"]:
        text = name if stamp["kind"] == "name" else stamp.get("text", "")
        if stamp["kind"] == "time":
            text = captured.astimezone(stamp_timezone(stamp)).strftime("%Y-%m-%d %H:%M:%S %Z")
        font = ImageFont.truetype(str(FONT_FILE), stamp.get("size", 18))
        bounds = draw.textbbox((0, 0), text, font=font)
        width, height = bounds[2] - bounds[0], bounds[3] - bounds[1]
        position = stamp.get("position", "top-left")
        offset = offsets.get(position, 0)
        x = (
            8
            if position.endswith("left")
            else max(8, image.width - width - 8)
            if position.endswith("right")
            else max(8, (image.width - width) // 2)
        )
        y = 8 + offset if position.startswith("top") else image.height - height - 8 - offset
        background = stamp.get("background", "transparent")
        if background != "transparent":
            draw.rectangle((x - 3, y - 3, x + width + 3, y + height + 3), fill=background)
        draw.text(
            (x - bounds[0], y - bounds[1]),
            text,
            font=font,
            fill="black" if background == "white" else "white",
            stroke_width=1 if background == "transparent" else 0,
            stroke_fill="black",
        )
        offsets[position] = offset + height + 8
    return image


class Workspace:
    """@brief One physical stream, bounded latest frame, and at most one encoder."""

    def __init__(self, parameters: dict, device: dict, limits: dict | None = None):
        self.root = Path(parameters["workspace_directory"])
        self.root.mkdir(mode=0o700, parents=True, exist_ok=True)
        self.node, _ = v4l2.resolve_capture_node(
            parameters["hardware_identity"], parameters["endpoint_id"]
        )
        self.name = parameters["display_name"]
        self.device = device
        self.limits = limits or {}
        saved = parameters.get("settings", {})
        if not saved:
            from pyrav4l2 import Device

            camera = Device(self.node)
            modes = [
                (v4l2.format_name(fmt), int(size.width), int(size.height), fmt, size)
                for fmt, sizes in camera.available_formats.items()
                for size in sizes
                if v4l2.format_name(fmt) in {"MJPG", "YUYV"}
                and int(size.width) * int(size.height) <= 2_000_000
            ]
            preferred = (
                device.get("default_format", "MJPG"),
                device.get("default_width", 640),
                device.get("default_height", 480),
            )
            if not modes:
                raise ValueError("unsupported_format")
            chosen = next((mode for mode in modes if mode[:3] == preferred), modes[0])
            rates = [
                int(item.denominator) // int(item.numerator)
                for item in camera.get_available_frame_intervals(chosen[3], chosen[4])
                if int(item.numerator) > 0
                and int(item.denominator) % int(item.numerator) == 0
                and 1 <= int(item.denominator) // int(item.numerator) <= 30
            ]
            if not rates:
                raise ValueError("unsupported_format")
            saved = {
                "format": chosen[0],
                "width": chosen[1],
                "height": chosen[2],
                "fps": 15 if 15 in rates else rates[0],
            }
        self.settings = self.validate_mode(saved)
        self.lock = threading.RLock()
        self.stop = threading.Event()
        self.frame = None
        self.preview = b""
        self.captured = ""
        self.error = ""
        self.last_access = time.monotonic()
        self.expires = self.last_access + 55 * 60
        self.recording = None
        self.revision = 1
        self.frame_revision = 0
        _, self.controls = v4l2.read_controls(self.node)

    def validate_mode(self, raw):
        """Accept only advertised capture modes within administrator bounds."""
        from pyrav4l2 import Device

        candidate = validate_settings(raw)
        if (
            candidate["width"] > self.limits.get("maximum_width", 4096)
            or candidate["height"] > self.limits.get("maximum_height", 4096)
            or candidate["width"] * candidate["height"]
            > self.limits.get("maximum_pixels", 2_000_000)
        ):
            raise ValueError("image_too_large")
        camera = Device(self.node)
        choice = next(
            (
                (fmt, size)
                for fmt, sizes in camera.available_formats.items()
                for size in sizes
                if v4l2.format_name(fmt) == candidate["format"]
                and int(size.width) == candidate["width"]
                and int(size.height) == candidate["height"]
            ),
            None,
        )
        if choice is None or not any(
            int(item.denominator) == candidate["fps"] * int(item.numerator)
            for item in camera.get_available_frame_intervals(*choice)
        ):
            raise ValueError("unsupported_format")
        return candidate

    def status(self):
        """Return status without including server paths or raw driver errors."""
        with self.lock:
            recording = None
            if self.recording:
                record = self.recording
                recording = {
                    "id": record["id"],
                    "elapsed": round(time.monotonic() - record["started"], 1),
                    "bytes": record["path"].stat().st_size if record["path"].exists() else 0,
                    "mode": self.settings["mode"],
                    "frames": record["frames"],
                    "max_seconds": record["max_seconds"],
                    "max_bytes": record["max_bytes"],
                }
            return {
                "state": "failed"
                if self.error
                else "recording"
                if recording
                else "live"
                if self.preview
                else "starting",
                "error": self.error,
                "captured_at": self.captured,
                "recording": recording,
                "revision": self.revision,
                "settings": self.settings,
            }

    def _metadata(self, record, kind, reason="completed"):
        path = record["path"]
        duration = 0
        if kind == "video":
            probe = subprocess.run(
                [
                    media_tool("ffprobe"),
                    "-v",
                    "error",
                    "-show_entries",
                    "format=duration",
                    "-of",
                    "json",
                    str(path),
                ],
                capture_output=True,
                timeout=3,
                check=True,
            )
            duration = float(json.loads(probe.stdout)["format"]["duration"])
            if duration <= 0:
                raise ValueError("encoding_failed")
        digest = hashlib.sha256()
        with path.open("rb") as source:
            while chunk := source.read(1024 * 1024):
                digest.update(chunk)
            os.fsync(source.fileno())
        with (self.root / (record["id"] + ".thumb.jpg")).open("rb") as thumbnail:
            os.fsync(thumbnail.fileno())
        meta = {
            "id": record["id"],
            "kind": kind,
            "extension": path.suffix[1:],
            "bytes": path.stat().st_size,
            "sha256": digest.hexdigest(),
            "reason": reason,
            "duration": duration,
            "acquisition_seconds": max(
                0, time.monotonic() - record.get("started", time.monotonic())
            ),
            "width": self.settings["width"],
            "height": self.settings["height"],
            "captured_at": self.captured,
            "settings": self.settings,
            "hardware_controls": self.controls,
        }
        temporary = self.root / (record["id"] + ".json.tmp")
        temporary.write_text(json.dumps(meta), encoding="utf-8")
        with temporary.open("rb") as source:
            os.fsync(source.fileno())
        temporary.replace(self.root / (record["id"] + ".json"))
        if os.name == "posix":
            directory = os.open(self.root, os.O_RDONLY | os.O_DIRECTORY)
            try:
                os.fsync(directory)
            finally:
                os.close(directory)

    def _thumbnail(self, identity):
        thumbnail = self.frame.copy()
        thumbnail.thumbnail((320, 240))
        thumbnail.save(self.root / (identity + ".thumb.jpg"), "JPEG", quality=70)

    def finish_recording(self, reason="completed"):
        """@brief Bound encoder shutdown and publish only successfully finalized media."""
        record = self.recording
        if not record:
            return
        self.recording = None
        process = record["process"]
        try:
            process.stdin.close()
            code = process.wait(timeout=5)
            if (
                code != 0
                or not record["frames"]
                or record["path"].stat().st_size > record["max_bytes"]
            ):
                raise ValueError("encoding_failed")
            self._metadata(record, "video", reason)
        except (OSError, ValueError, KeyError, subprocess.SubprocessError):
            LOGGER.exception("Camera encoder finalization failed; frames=%s", record["frames"])
            process.kill()
            process.wait(timeout=3)
            self.error = "encoding_failed"

    def command(self, data):
        """@brief Handle a bounded Core-authorized command on the private socket."""
        with self.lock:
            self.last_access = time.monotonic()
            action = data.get("action")
            if action == "settings":
                if self.recording:
                    raise ValueError("recording_active")
                candidate = self.validate_mode(data["settings"])
                self.settings = candidate
                self.revision += 1
            elif action in {"controls", "read_controls", "reset_controls"}:
                if action != "read_controls" and self.recording:
                    raise ValueError("recording_active")
                if action == "controls":
                    v4l2.apply_controls(self.node, data.get("controls", {}))
                elif action == "reset_controls":
                    descriptors, _ = v4l2.read_controls(self.node)
                    values = {
                        item["id"]: item["default"]
                        for item in descriptors
                        if not item["read_only"] and not item["inactive"]
                    }
                    values.update(self.device.get("baseline_controls", {}))
                    v4l2.apply_controls(self.node, values)
                descriptors, values = v4l2.read_controls(self.node)
                self.controls = values
                if action != "read_controls":
                    self.revision += 1
                return {"descriptors": descriptors, "values": values}
            elif action == "snapshot":
                if (
                    self.frame is None
                    or self.frame_revision != self.revision
                    or time.time() - datetime.fromisoformat(self.captured).timestamp() > 3
                ):
                    raise ValueError("frame_unavailable")
                identity = str(UUID(data["id"]))
                encoding = self.settings["encoding"]
                path = self.root / (identity + (".jpg" if encoding == "jpeg" else ".png"))
                self.frame.save(
                    path,
                    "JPEG" if encoding == "jpeg" else "PNG",
                    **({"quality": self.settings["quality"]} if encoding == "jpeg" else {}),
                )
                if path.stat().st_size > 8 << 20:
                    path.unlink()
                    raise ValueError("image_too_large")
                self._thumbnail(identity)
                self._metadata({"id": identity, "path": path}, "image")
            elif action == "record_start":
                if self.recording:
                    raise ValueError("recording_active")
                if (
                    self.frame is None
                    or self.frame_revision != self.revision
                    or time.time() - datetime.fromisoformat(self.captured).timestamp() > 3
                ):
                    raise ValueError("frame_unavailable")
                identity = str(UUID(data["id"]))
                path = self.root / (identity + ".mp4")
                settings = self.settings
                fps = (
                    settings["playback_fps"] if settings["mode"] == "timelapse" else settings["fps"]
                )
                command = [
                    media_tool("ffmpeg"),
                    "-hide_banner",
                    "-loglevel",
                    "error",
                    "-nostdin",
                    "-f",
                    "rawvideo",
                    "-pixel_format",
                    "rgb24",
                    "-video_size",
                    f"{settings['width']}x{settings['height']}",
                    "-framerate",
                    str(fps),
                ]
                if settings["mode"] == "video":
                    command += ["-use_wallclock_as_timestamps", "1"]
                command += [
                    "-i",
                    "pipe:0",
                    "-an",
                    "-r",
                    str(fps),
                    "-fps_mode",
                    "cfr",
                    "-c:v",
                    "libx264",
                    "-preset",
                    "ultrafast",
                    "-threads",
                    "2",
                    "-crf",
                    str(settings["video_quality"]),
                    "-pix_fmt",
                    "yuv420p",
                    "-g",
                    str(fps),
                    "-movflags",
                    "+frag_keyframe+empty_moov+default_base_moof",
                    "-n",
                    str(path),
                ]
                process = subprocess.Popen(
                    command,
                    stdin=subprocess.PIPE,
                    stdout=subprocess.DEVNULL,
                    stderr=None,
                )
                os.set_blocking(process.stdin.fileno(), False)
                self.recording = {
                    "id": identity,
                    "path": path,
                    "process": process,
                    "fps": fps,
                    "frames": 0,
                    "started": time.monotonic(),
                    "next_frame": time.monotonic(),
                    "max_bytes": data["max_bytes"],
                    "max_seconds": data["max_seconds"],
                }
                self._thumbnail(identity)
            elif action == "record_stop":
                self.finish_recording()
            elif action == "stop":
                self.finish_recording("stopped")
                self.stop.set()
            else:
                raise ValueError("invalid_action")
            return self.status()

    def record_frame(self, image):
        record = self.recording
        if not record:
            return
        now = time.monotonic()
        size = record["path"].stat().st_size if record["path"].exists() else 0
        disk = shutil.disk_usage(self.root)
        # Leave room for one worst-case RGB frame and final container metadata.
        margin = max(2 << 20, image.width * image.height * 4)
        if (
            now - record["started"] >= record["max_seconds"]
            or size >= record["max_bytes"] - margin
            or disk.free < max(2 << 30, disk.total // 10) + margin
        ):
            self.finish_recording("limit_reached")
            return
        if self.settings["mode"] == "timelapse" and now < record["next_frame"]:
            return
        interval = (
            self.settings["interval"]
            if self.settings["mode"] == "timelapse"
            else 1 / self.settings["fps"]
        )
        payload = memoryview(image.tobytes())
        deadline = time.monotonic() + 3
        while payload:
            if time.monotonic() > deadline or record["process"].poll() is not None:
                raise ValueError("encoder_stalled")
            if select.select([], [record["process"].stdin], [], 0.1)[1]:
                count = os.write(record["process"].stdin.fileno(), payload)
                payload = payload[count:]
        record["frames"] += 1
        record["next_frame"] += interval

    def capture(self):
        """@brief Keep the device open; restart only when its capture mode changes."""
        from pyrav4l2 import Device, Stream

        try:
            while not self.stop.is_set():
                with self.lock:
                    settings = self.settings.copy()
                camera = Device(self.node)
                choice = next(
                    (fmt, size)
                    for fmt, sizes in camera.available_formats.items()
                    for size in sizes
                    if v4l2.format_name(fmt) == settings["format"]
                    and int(size.width) == settings["width"]
                    and int(size.height) == settings["height"]
                )
                camera.set_format(*choice)
                intervals = camera.get_available_frame_intervals(*choice)
                matching = [
                    item
                    for item in intervals
                    if int(item.denominator) == settings["fps"] * int(item.numerator)
                ]
                if matching:
                    camera.set_frame_interval(matching[0])
                stream = iter(Stream(camera))
                next_frame = time.monotonic()
                try:
                    for raw in stream:
                        if self.stop.is_set():
                            break
                        now = time.monotonic()
                        with self.lock:
                            current = self.settings.copy()
                            if any(
                                current[key] != settings[key]
                                for key in ("width", "height", "format", "fps")
                            ):
                                break
                            if not self.recording and now - self.last_access > 30:
                                self.stop.set()
                                break
                            if now < next_frame:
                                continue
                            next_frame = max(
                                next_frame + 1 / current["fps"], now - 1 / current["fps"]
                            )
                            captured = datetime.now(UTC)
                            frame = v4l2.frame_to_image(
                                bytes(raw), current["format"], current["width"], current["height"]
                            )
                            image = compose(frame, current, self.device, self.name, captured)
                            self.frame = image
                            self.frame_revision = self.revision
                            self.captured = captured.isoformat()
                            preview = image.copy()
                            preview.thumbnail((960, 720))
                            output = io.BytesIO()
                            preview.save(output, "JPEG", quality=75)
                            self.preview = output.getvalue()
                            self.record_frame(image)
                finally:
                    stream.close()
        except Exception:
            LOGGER.exception("Camera capture stopped")
            self.error = "capture_failed"
        finally:
            with self.lock:
                self.finish_recording("interrupted" if self.error else "stopped")
            self.stop.set()

    def run(self):
        workspace = self

        class Handler(BaseHTTPRequestHandler):
            def log_message(self, *_):
                return

            def do_GET(self):
                with workspace.lock:
                    workspace.last_access = time.monotonic()
                    body = (
                        workspace.preview
                        if self.path == "/frame"
                        else json.dumps(workspace.status()).encode()
                    )
                    if self.path == "/frame" and (
                        not workspace.captured
                        or time.time() - datetime.fromisoformat(workspace.captured).timestamp() > 3
                    ):
                        body = b""
                    media = "image/jpeg" if self.path == "/frame" else "application/json"
                self.send_response(200 if body else 503)
                self.send_header("Content-Type", media)
                self.send_header("Content-Length", str(len(body)))
                self.end_headers()
                self.wfile.write(body)

            def do_POST(self):
                try:
                    size = int(self.headers.get("Content-Length", "0"))
                    if not 1 <= size <= 16384:
                        raise ValueError("invalid_parameters")
                    result = workspace.command(json.loads(self.rfile.read(size)))
                    status = 200
                except Exception as error:
                    LOGGER.exception("Camera workspace command failed")
                    result = {
                        "error": str(error)
                        if isinstance(error, ValueError)
                        else "camera_command_failed"
                    }
                    status = 400
                body = json.dumps(result).encode()
                self.send_response(status)
                self.send_header("Content-Length", str(len(body)))
                self.end_headers()
                self.wfile.write(body)

        socket = self.root / "control.sock"
        socket.unlink(missing_ok=True)
        server = socketserver.UnixStreamServer(str(socket), Handler)
        server.timeout = 0.2
        thread = threading.Thread(target=self.capture, daemon=True)
        thread.start()
        try:
            while not self.stop.is_set():
                server.handle_request()
                with self.lock:
                    now = time.monotonic()
                    if (
                        self.recording
                        and now - self.recording["started"] >= self.recording["max_seconds"]
                    ):
                        self.finish_recording("limit_reached")
                    if now >= self.expires or (not self.recording and now - self.last_access > 30):
                        self.stop.set()
        finally:
            self.stop.set()
            with self.lock:
                self.finish_recording("stopped")
            thread.join(timeout=2)
            server.server_close()
            socket.unlink(missing_ok=True)


def run_workspace(parameters: dict, device: dict):
    """Run only from the supervised internal stream operation."""
    Workspace(parameters, device).run()
