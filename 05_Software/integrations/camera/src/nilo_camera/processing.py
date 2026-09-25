"""Bounded, reproducible rendering of a physical camera frame."""

from __future__ import annotations

import math
import re
from pathlib import Path

from nilo_integration import ExecuteFailure
from PIL import Image, ImageDraw, ImageFont, ImageOps

FONT_FILE = Path(__file__).with_name("fonts") / "DejaVuSans.ttf"

COLOR = re.compile(r"#[0-9a-fA-F]{6}\Z")
KINDS = {"crosshair", "grid", "line", "rectangle", "text", "scale_bar"}
OPTIONS = {
    "zoom",
    "center_x",
    "center_y",
    "rotation",
    "flip_horizontal",
    "flip_vertical",
    "grayscale",
    "invert",
    "overlays",
}


def _number(value: object, low: float, high: float) -> bool:
    return type(value) in (int, float) and math.isfinite(value) and low <= value <= high


def validate_processing(value: object) -> dict:
    """Reject unknown, unbounded or ill-typed transforms before capture."""
    if not isinstance(value, dict) or set(value) - OPTIONS:
        raise ExecuteFailure("invalid_parameters", "Unsupported processing option")
    result = {
        "zoom": 1,
        "center_x": 0.5,
        "center_y": 0.5,
        "rotation": 0,
        "flip_horizontal": False,
        "flip_vertical": False,
        "grayscale": False,
        "invert": False,
        "overlays": [],
        **value,
    }
    if (
        not _number(result["zoom"], 1, 8)
        or not _number(result["center_x"], 0, 1)
        or not _number(result["center_y"], 0, 1)
        or result["rotation"] not in (0, 90, 180, 270)
        or type(result["rotation"]) is not int
        or any(
            type(result[key]) is not bool
            for key in ("flip_horizontal", "flip_vertical", "grayscale", "invert")
        )
    ):
        raise ExecuteFailure("invalid_parameters", "Invalid image transform")
    overlays = result["overlays"]
    if not isinstance(overlays, list) or len(overlays) > 32:
        raise ExecuteFailure("invalid_parameters", "Too many overlays")
    for overlay in overlays:
        if not isinstance(overlay, dict) or overlay.get("kind") not in KINDS:
            raise ExecuteFailure("invalid_parameters", "Unknown overlay")
        if set(overlay) - {
            "kind",
            "x",
            "y",
            "x2",
            "y2",
            "color",
            "line_width",
            "text",
            "size",
            "spacing",
            "length_mm",
        }:
            raise ExecuteFailure("invalid_parameters", "Unknown overlay option")
        if (
            any(not _number(overlay[key], 0, 1) for key in ("x", "y", "x2", "y2") if key in overlay)
            or not isinstance(overlay.get("color", "#ffffff"), str)
            or not COLOR.fullmatch(overlay.get("color", "#ffffff"))
            or not _number(overlay.get("line_width", 2), 1, 16)
            or not _number(overlay.get("size", 16), 8, 48)
            or not _number(overlay.get("spacing", 0.1), 0.02, 0.5)
            or not _number(overlay.get("length_mm", 1), 0.01, 1000)
        ):
            raise ExecuteFailure("invalid_parameters", "Invalid overlay geometry")
        if overlay["kind"] == "text" and (
            not isinstance(overlay.get("text"), str)
            or len(overlay["text"]) > 120
            or "\x00" in overlay["text"]
        ):
            raise ExecuteFailure("invalid_parameters", "Invalid overlay text")
        if overlay["kind"] == "scale_bar" and ("x" not in overlay or "y" not in overlay):
            raise ExecuteFailure("invalid_parameters", "Scale bar needs a position")
    return result


def render_image(
    frame: Image.Image, options: dict, device: dict, output_width: int, output_height: int
) -> Image.Image:
    """Mount, crop, transform, resize, then project scene markers into output."""
    image = frame.convert("RGB")
    mounting_rotation = device.get("mounting_rotation", 0)
    if mounting_rotation:
        image = image.rotate(mounting_rotation, expand=True)
    if device.get("mounting_flip_horizontal"):
        image = ImageOps.mirror(image)
    if device.get("mounting_flip_vertical"):
        image = ImageOps.flip(image)
    scene_width, scene_height = image.size
    zoom = options["zoom"]
    crop_width, crop_height = scene_width / zoom, scene_height / zoom
    left = max(0, min(scene_width - crop_width, options["center_x"] * scene_width - crop_width / 2))
    top = max(
        0, min(scene_height - crop_height, options["center_y"] * scene_height - crop_height / 2)
    )
    image = image.crop(
        (round(left), round(top), round(left + crop_width), round(top + crop_height))
    )
    rotation = options["rotation"]
    if rotation:
        image = image.rotate(rotation, expand=True)
    if options["flip_horizontal"]:
        image = ImageOps.mirror(image)
    if options["flip_vertical"]:
        image = ImageOps.flip(image)
    if options["grayscale"]:
        image = ImageOps.grayscale(image).convert("RGB")
    if options["invert"]:
        image = ImageOps.invert(image)
    if image.size != (output_width, output_height):
        image = image.resize((output_width, output_height), Image.Resampling.LANCZOS)

    def project(x: float, y: float) -> tuple[int, int]:
        px = (x * scene_width - left) / crop_width
        py = (y * scene_height - top) / crop_height
        if rotation == 90:
            px, py = py, 1 - px
        elif rotation == 180:
            px, py = 1 - px, 1 - py
        elif rotation == 270:
            px, py = 1 - py, px
        if options["flip_horizontal"]:
            px = 1 - px
        if options["flip_vertical"]:
            py = 1 - py
        return round(px * output_width), round(py * output_height)

    draw = ImageDraw.Draw(image)
    for overlay in options["overlays"]:
        kind = overlay["kind"]
        color = overlay.get("color", "#ffffff")
        line_width = int(overlay.get("line_width", 2))
        x, y = project(overlay.get("x", 0.5), overlay.get("y", 0.5))
        if kind == "crosshair":
            arm = 12
            draw.line((x - arm, y, x + arm, y), fill=color, width=line_width)
            draw.line((x, y - arm, x, y + arm), fill=color, width=line_width)
        elif kind == "grid":
            count = min(32, max(2, round(1 / overlay.get("spacing", 0.1))))
            for part in range(1, count):
                px = round(part * output_width / count)
                py = round(part * output_height / count)
                draw.line((px, 0, px, output_height), fill=color, width=line_width)
                draw.line((0, py, output_width, py), fill=color, width=line_width)
        elif kind in ("line", "rectangle"):
            x2, y2 = project(overlay.get("x2", 0.5), overlay.get("y2", 0.5))
            if kind == "line":
                draw.line((x, y, x2, y2), fill=color, width=line_width)
            else:
                draw.rectangle(
                    (min(x, x2), min(y, y2), max(x, x2), max(y, y2)),
                    outline=color,
                    width=line_width,
                )
        elif kind == "text":
            font = ImageFont.truetype(str(FONT_FILE), int(overlay.get("size", 16)))
            draw.text((x, y), overlay["text"], fill=color, font=font, stroke_width=0)
        elif kind == "scale_bar":
            calibration = device.get("calibration_mm_per_pixel")
            if not _number(calibration, 0.000001, 1000):
                raise ExecuteFailure("invalid_parameters", "Scale bar requires calibration")
            length = round(overlay.get("length_mm", 1) / calibration * output_width / crop_width)
            if length < 1 or length > output_width:
                raise ExecuteFailure("invalid_parameters", "Scale bar exceeds frame")
            draw.line((x, y, x + length, y), fill=color, width=line_width)
            font = ImageFont.truetype(str(FONT_FILE), 16)
            draw.text((x, y + 4), f"{overlay.get('length_mm', 1):g} mm", fill=color, font=font)
    return image
