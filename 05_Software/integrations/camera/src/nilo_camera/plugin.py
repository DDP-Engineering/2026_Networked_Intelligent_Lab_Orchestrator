#!/usr/bin/env python3
# -*- coding: utf-8 -*-
# -----------------------------------------------------------------------------
# Copyright (c) 2026 DDP Engineering
# SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
# -----------------------------------------------------------------------------
"""Run bounded operations against discovered physical V4L2 cameras."""

from __future__ import annotations

import hashlib
import io
import re
from datetime import UTC, datetime
from pathlib import Path
from uuid import UUID

from nilo_integration import EventSink, ExecuteFailure
from nilo_integration.models import (
    ArtifactDeclaration,
    ConfigureRequest,
    ConfigureResult,
    DescribeResult,
    DiscoverResult,
    ExecuteRequest,
    ExecuteResult,
    HealthResult,
)
from PIL import Image

from nilo_camera import v4l2
from nilo_camera.processing import render_image, validate_processing

CAMERA_ID = re.compile(r"cam-[1-9][0-9]*\Z")


def snapshot_parameters(value: object) -> dict:
    """Validate finite image parameters before opening a device or allocating a frame."""
    allowed = {
        "device_id",
        "endpoint_id",
        "hardware_identity",
        "staging_directory",
        "width",
        "height",
        "format",
        "frame_interval",
        "controls",
        "processing",
        "encoding",
        "jpeg_quality",
        "maximum_dimension",
    }
    if not isinstance(value, dict) or set(value) - allowed:
        raise ExecuteFailure("invalid_parameters", "Unsupported snapshot parameters")
    if not isinstance(value.get("device_id"), str) or not CAMERA_ID.fullmatch(value["device_id"]):
        raise ExecuteFailure("device_not_found", "Unknown camera")
    stage = Path(value.get("staging_directory", ""))
    if not stage.is_absolute() or not stage.is_dir() or stage.is_symlink():
        raise ExecuteFailure("invalid_parameters", "Core-owned staging directory is required")
    width, height = value.get("width", 640), value.get("height", 480)
    if (
        type(width) is not int
        or type(height) is not int
        or not 16 <= width <= 4096
        or not 16 <= height <= 4096
        or width * height > v4l2.MAX_PIXELS
    ):
        raise ExecuteFailure("image_too_large", "Snapshot dimensions exceed the limit")
    if value.get("format", "MJPG") not in v4l2.SUPPORTED_FORMATS:
        raise ExecuteFailure("unsupported_format", "Capture format is not available")
    frame_interval = value.get("frame_interval")
    if frame_interval is not None and (
        not isinstance(frame_interval, dict)
        or set(frame_interval) != {"numerator", "denominator"}
        or any(type(item) is not int or not 1 <= item <= 10000 for item in frame_interval.values())
    ):
        raise ExecuteFailure("invalid_parameters", "Invalid capture frame interval")
    encoding = value.get("encoding", "jpeg")
    if encoding not in ("jpeg", "png"):
        raise ExecuteFailure("invalid_parameters", "Unsupported image encoding")
    quality = value.get("jpeg_quality", 85)
    if type(quality) is not int or not 1 <= quality <= 95:
        raise ExecuteFailure("invalid_parameters", "JPEG quality is outside the limit")
    maximum_dimension = value.get("maximum_dimension")
    if maximum_dimension is not None and (
        type(maximum_dimension) is not int or not 16 <= maximum_dimension <= 4096
    ):
        raise ExecuteFailure("invalid_parameters", "Invalid output dimension limit")
    processing = validate_processing(value.get("processing", {}))
    controls = value.get("controls", {})
    if not isinstance(controls, dict) or len(controls) > 64:
        raise ExecuteFailure("invalid_parameters", "Invalid controls object")
    return {
        **value,
        "width": width,
        "height": height,
        "encoding": encoding,
        "jpeg_quality": quality,
        "processing": processing,
        "staging_directory": stage,
    }


class CameraIntegration:
    """Own a supervised physical camera runner process."""

    name = "camera"

    def __init__(self):
        self.plugin_settings = {
            "maximum_width": 4096,
            "maximum_height": 4096,
            "maximum_pixels": 2_000_000,
            "default_jpeg_quality": 85,
        }
        self.plugin_revision = 0
        self.baseline: dict[str, dict] = {}
        self.device_revisions: dict[str, int] = {}
        self.device_settings: dict[str, dict] = {}
        self.workspace = None

    @staticmethod
    def device_id(value: object, allowed: set[str]) -> tuple[str, dict]:
        if not isinstance(value, dict) or set(value) - allowed:
            raise ExecuteFailure("invalid_parameters", "Unsupported control parameters")
        device_id = value.get("device_id")
        if not isinstance(device_id, str) or not CAMERA_ID.fullmatch(device_id):
            raise ExecuteFailure("device_not_found", "Unknown camera")
        return device_id, value

    async def describe(self) -> DescribeResult:
        return DescribeResult(
            status="ok",
            integration=self.name,
            operations=[
                "discover_devices",
                "snapshot",
                "get_controls",
                "set_controls",
                "reset_controls",
                "workspace",
                "stream",
            ],
        )

    async def discover(self) -> DiscoverResult:
        return DiscoverResult(status="ok")

    async def health(self) -> HealthResult:
        return HealthResult(status="ok")

    async def configure(self, request: ConfigureRequest) -> ConfigureResult:
        configuration = request.configuration
        if (
            not isinstance(configuration, dict)
            or set(configuration) != {"schema_version", "plugin", "device"}
            or configuration["schema_version"] != 1
        ):
            raise ExecuteFailure("invalid_parameters", "Unsupported configuration envelope")
        plugin = configuration["plugin"]
        if (
            not isinstance(plugin, dict)
            or set(plugin) != {"revision", "settings"}
            or type(plugin["revision"]) is not int
            or plugin["revision"] < 0
            or not isinstance(plugin["settings"], dict)
            or set(plugin["settings"]) - set(self.plugin_settings)
        ):
            raise ExecuteFailure("invalid_parameters", "Unsupported plugin configuration")
        proposed = {**self.plugin_settings, **plugin["settings"]}
        for key in ("maximum_width", "maximum_height", "maximum_pixels"):
            if type(proposed[key]) is not int or not 16 <= proposed[key] <= 16_777_216:
                raise ExecuteFailure("invalid_parameters", "Invalid image limit")
        if (
            type(proposed["default_jpeg_quality"]) is not int
            or not 1 <= proposed["default_jpeg_quality"] <= 95
        ):
            raise ExecuteFailure("invalid_parameters", "Invalid JPEG quality")
        target = configuration["device"]
        if target is not None:
            allowed = {
                "baseline_controls",
                "mounting_rotation",
                "mounting_flip_horizontal",
                "mounting_flip_vertical",
                "default_width",
                "default_height",
                "default_format",
                "calibration_mm_per_pixel",
            }
            if (
                not isinstance(target, dict)
                or set(target) != {"id", "revision", "settings"}
                or not isinstance(target["id"], str)
                or not CAMERA_ID.fullmatch(target["id"])
                or type(target["revision"]) is not int
                or target["revision"] < 0
                or not isinstance(target["settings"], dict)
                or set(target["settings"]) - allowed
            ):
                raise ExecuteFailure("invalid_parameters", "Unsupported device configuration")
            baseline = target["settings"].get("baseline_controls", {})
            if not isinstance(baseline, dict) or len(baseline) > 64:
                raise ExecuteFailure("invalid_parameters", "Invalid control baseline")
            self.baseline[target["id"]] = baseline.copy()
            self.device_revisions[target["id"]] = target["revision"]
            self.device_settings[target["id"]] = target["settings"].copy()
        self.plugin_settings = proposed
        self.plugin_revision = plugin["revision"]
        return ConfigureResult(status="ok")

    async def cancel(self, request_id: UUID) -> None:
        del request_id
        if self.workspace:
            self.workspace.stop.set()

    async def safe_shutdown(self) -> None:
        if self.workspace:
            import asyncio

            await asyncio.to_thread(self.workspace.command, {"action": "stop"})

    async def execute(self, request: ExecuteRequest, events: EventSink) -> ExecuteResult:
        if request.operation == "stream":
            import asyncio

            from nilo_camera.workspace import Workspace

            device_id = request.parameters["device_id"]
            self.workspace = Workspace(
                request.parameters, self.device_settings.get(device_id, {}), self.plugin_settings
            )
            await asyncio.to_thread(self.workspace.run)
            if self.workspace.error == "capture_failed":
                raise ExecuteFailure("capture_failed", "Camera acquisition failed")
            return ExecuteResult(status="completed", data={"state": "stopped"})

        if request.operation == "discover_devices":
            if request.parameters != {}:
                raise ExecuteFailure("invalid_parameters", "Discovery takes no request parameters")
            return ExecuteResult(status="completed", data={"devices": v4l2.scan_usb_cameras()})

        if request.operation in ("get_controls", "set_controls", "reset_controls"):
            allowed = {"device_id", "endpoint_id", "hardware_identity"}
            if request.operation == "set_controls":
                allowed.add("controls")
            if request.operation == "reset_controls":
                allowed.update({"factory", "baseline_controls"})
            device_id, data = self.device_id(request.parameters, allowed)
            try:
                node, _ = v4l2.resolve_capture_node(
                    data.get("hardware_identity"), data.get("endpoint_id")
                )
                if request.operation == "set_controls":
                    v4l2.apply_controls(node, data.get("controls"))
                elif request.operation == "reset_controls":
                    descriptors, _ = v4l2.read_controls(node)
                    overrides = data.get("baseline_controls", self.baseline.get(device_id, {}))
                    if not isinstance(overrides, dict):
                        raise ExecuteFailure("invalid_parameters", "Invalid control baseline")
                    defaults = {
                        item["id"]: item["default"]
                        for item in descriptors
                        if not item["read_only"] and not item["inactive"]
                    }
                    if not data.get("factory"):
                        defaults.update(overrides)
                    if defaults:
                        v4l2.apply_controls(node, defaults)
                descriptors, values = v4l2.read_controls(node)
            except ValueError as error:
                raise ExecuteFailure("control_out_of_range", str(error)) from error
            except OSError as error:
                raise ExecuteFailure("device_busy", "Camera control operation failed") from error
            return ExecuteResult(
                status="completed",
                data={
                    "device_id": device_id,
                    "values": values,
                    "descriptors": descriptors,
                },
            )

        if request.operation != "snapshot":
            raise ExecuteFailure("unknown_operation", "Unsupported camera operation")
        data = snapshot_parameters(request.parameters)
        settings = self.device_settings.get(data["device_id"], {})
        if "width" not in request.parameters:
            data["width"] = settings.get("default_width", data["width"])
        if "height" not in request.parameters:
            data["height"] = settings.get("default_height", data["height"])
        if "format" not in request.parameters:
            data["format"] = settings.get("default_format", "MJPG")
        if "jpeg_quality" not in request.parameters:
            data["jpeg_quality"] = self.plugin_settings["default_jpeg_quality"]
        if (
            data["width"] > self.plugin_settings["maximum_width"]
            or data["height"] > self.plugin_settings["maximum_height"]
            or data["width"] * data["height"] > self.plugin_settings["maximum_pixels"]
        ):
            raise ExecuteFailure("image_too_large", "Configured image limit exceeded")
        try:
            node, endpoint = v4l2.resolve_capture_node(
                data.get("hardware_identity"), data.get("endpoint_id")
            )
            if data["format"] not in endpoint["formats"]:
                raise ExecuteFailure("unsupported_format", "Capture format is unavailable")
            if data.get("controls"):
                v4l2.apply_controls(node, data["controls"])
            if frame_interval := data.get("frame_interval"):
                image = v4l2.capture_image(
                    node, data["format"], data["width"], data["height"], frame_interval
                )
            else:
                image = v4l2.capture_image(node, data["format"], data["width"], data["height"])
            captured_at = datetime.now(UTC).isoformat().replace("+00:00", "Z")
            _, actual_controls = v4l2.read_controls(node)
        except (OSError, ValueError) as error:
            raise ExecuteFailure("capture_failed", "Camera frame could not be captured") from error
        image = render_image(image, data["processing"], settings, data["width"], data["height"])
        if data.get("maximum_dimension") is not None:
            image.thumbnail(
                (data["maximum_dimension"], data["maximum_dimension"]),
                resample=Image.Resampling.LANCZOS,
            )
        output_width, output_height = image.size
        output = io.BytesIO()
        if data["encoding"] == "jpeg":
            image.save(output, format="JPEG", quality=data["jpeg_quality"], optimize=False)
            filename, media_type = "image.jpg", "image/jpeg"
        else:
            image.save(output, format="PNG", optimize=False)
            filename, media_type = "image.png", "image/png"
        image.close()
        encoded = output.getvalue()
        if len(encoded) > 8 << 20:
            raise ExecuteFailure("image_too_large", "Encoded image exceeds the artifact limit")
        with (data["staging_directory"] / filename).open("xb") as stream:
            stream.write(encoded)
        await events.emit(
            ArtifactDeclaration(
                id="image",
                media_type=media_type,
                size_bytes=len(encoded),
                digest=f"sha256:{hashlib.sha256(encoded).hexdigest()}",
            )
        )
        return ExecuteResult(
            status="completed",
            data={
                "device_id": data["device_id"],
                "captured_at": captured_at,
                "selected_endpoint": endpoint["id"],
                "warmup": {"discarded_frames": 2},
                "artifact_size_bytes": len(encoded),
                "actual_controls": actual_controls,
                "requested_controls": data.get("controls", {}),
                "plugin_revision": self.plugin_revision,
                "device_revision": self.device_revisions.get(data["device_id"], 0),
                "calibration_revision": (
                    self.device_revisions.get(data["device_id"], 0)
                    if settings.get("calibration_mm_per_pixel") is not None
                    else None
                ),
                "effective": {
                    "width": data["width"],
                    "height": data["height"],
                    "format": data["format"],
                    "frame_interval": data.get("frame_interval"),
                    "encoding": data["encoding"],
                    "output_width": output_width,
                    "output_height": output_height,
                    "processing": data["processing"],
                    "mounting": {
                        key: settings.get(key)
                        for key in (
                            "mounting_rotation",
                            "mounting_flip_horizontal",
                            "mounting_flip_vertical",
                        )
                    },
                },
            },
        )
