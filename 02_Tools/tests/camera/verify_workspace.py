#!/usr/bin/env python3
# -*- coding: utf-8 -*-  # noqa: UP009 - repository Python style
# -----------------------------------------------------------------------------
# Copyright (c) 2026 DDP Engineering
# SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
# -----------------------------------------------------------------------------
"""@file verify_workspace.py
@brief Exercise a physical camera workspace.
@details Writes test captures only to the explicit probe directory.
"""

import argparse
import json
import threading
import time
from pathlib import Path
from uuid import uuid4

from nilo_camera import v4l2
from nilo_camera.workspace import Workspace


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("directory", type=Path)
    parser.add_argument("--seconds", type=int, default=20)
    parser.add_argument("--timelapse", action="store_true")
    args = parser.parse_args()
    observed = next(
        item for item in v4l2.scan_usb_cameras() if item["selected_endpoint"]
    )
    workspace = Workspace(
        {
            "workspace_directory": str(args.directory),
            "hardware_identity": {
                key: observed[key]
                for key in ("vendor", "product", "serial", "port_path")
            },
            "endpoint_id": observed["selected_endpoint"],
            "display_name": "Physical test camera",
            "settings": {
                "mode": "timelapse" if args.timelapse else "video",
                "interval": 2,
                "processing": {"grayscale": True},
                "stamps": [{"kind": "time", "background": "black"}],
            },
        },
        {},
    )
    worker = threading.Thread(target=workspace.run)
    worker.start()
    try:
        deadline = time.monotonic() + 10
        while (
            not workspace.preview and time.monotonic() < deadline and worker.is_alive()
        ):
            time.sleep(0.1)
        assert workspace.preview, workspace.status()
        image_id, video_id = str(uuid4()), str(uuid4())
        workspace.command({"action": "snapshot", "id": image_id})
        workspace.command(
            {
                "action": "record_start",
                "id": video_id,
                "max_bytes": 500 << 20,
                "max_seconds": args.seconds,
            }
        )
        time.sleep(args.seconds + 2)
        result = workspace.status()
        workspace.command({"action": "record_stop"})
        print(json.dumps({"status": result, "image": image_id, "video": video_id}))
        assert result["recording"] is None
        assert (args.directory / (video_id + ".json")).is_file()
    finally:
        workspace.command({"action": "stop"})
        worker.join(timeout=15)
        assert not worker.is_alive()


if __name__ == "__main__":
    main()
