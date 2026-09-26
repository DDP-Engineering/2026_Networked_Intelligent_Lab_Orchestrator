#!/usr/bin/env python3
# -*- coding: utf-8 -*-
# -----------------------------------------------------------------------------
# Copyright (c) 2026 DDP Engineering
# SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
# -----------------------------------------------------------------------------
"""@file test_workspace.py
@brief Verify bounded workspace options and image processing.
@details These unit tests do not establish physical camera throughput.
"""

from datetime import UTC, datetime

import pytest
from PIL import Image, ImageChops

from nilo_camera.workspace import compose, validate_settings


@pytest.mark.parametrize(
    "settings",
    [
        {"fps": 0},
        {"width": 4096, "height": 4096},
        {"width": 641},
        {"quality": True},
        {"encoding": "exe"},
        {"mode": "unbounded"},
        {"interval": 0},
        {"directory": "/tmp"},
        {"stamps": [{"kind": "shell"}]},
    ],
)
def test_workspace_rejects_unbounded_or_unknown_options(settings):
    with pytest.raises(ValueError):
        validate_settings(settings)


def test_dynamic_stamps_use_capture_time_and_processed_frame():
    frame = Image.new("RGB", (640, 480), "red")
    settings = validate_settings(
        {
            "processing": {"grayscale": True},
            "stamps": [
                {"kind": "time", "position": "bottom-right", "background": "black"},
                {"kind": "name", "position": "top-left", "background": "white"},
            ],
        }
    )
    first = compose(frame, settings, {}, "Camera 1", datetime(2026, 9, 26, tzinfo=UTC))
    second = compose(frame, settings, {}, "Camera 1", datetime(2026, 9, 27, tzinfo=UTC))
    assert first.size == (640, 480)
    assert len(set(first.getpixel((320, 240)))) == 1
    assert ImageChops.difference(first, second).getbbox() is not None
    assert first.getpixel((7, 7)) == (255, 255, 255)


def test_defaults_match_normal_video_and_jpeg_contract():
    settings = validate_settings({})
    assert (settings["width"], settings["height"], settings["fps"]) == (640, 480, 15)
    assert (settings["mode"], settings["encoding"], settings["quality"]) == ("video", "jpeg", 85)
