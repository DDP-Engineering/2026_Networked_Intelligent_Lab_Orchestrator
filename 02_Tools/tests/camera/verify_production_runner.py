"""Exercise physical camera discovery and one bounded snapshot on the target."""

import argparse
import asyncio
import hashlib
import json
import sys
import tempfile
from pathlib import Path

sys.dont_write_bytecode = True

from nilo_camera.plugin import CameraIntegration
from nilo_integration.models import ExecuteRequest
from PIL import Image


class Events:
    def __init__(self):
        self.items = []

    async def emit(self, item):
        self.items.append(item)


async def main(maximum_dimension: int | None):
    runner = CameraIntegration()
    events = Events()
    discovery = await runner.execute(
        ExecuteRequest(type="execute", operation="discover_devices", parameters={}), events)
    devices = [item for item in discovery.data["devices"] if item["selected_endpoint"]]
    if not devices:
        raise SystemExit("no supported physical camera")
    device = devices[0]
    identity = {key: device[key] for key in ("vendor", "product", "serial", "port_path")}
    with tempfile.TemporaryDirectory(prefix="nilo-camera-snapshot-") as stage:
        result = await runner.execute(
            ExecuteRequest(type="execute", operation="snapshot", parameters={
                "device_id": "cam-1", "endpoint_id": device["selected_endpoint"],
                "hardware_identity": identity, "staging_directory": stage,
                "width": 640, "height": 480,
                **({"maximum_dimension": maximum_dimension} if maximum_dimension else {}),
            }), events)
        image = (Path(stage) / "image.jpg").read_bytes()
        artifact = events.items[-1]
        expected = f"sha256:{hashlib.sha256(image).hexdigest()}"
        if artifact.digest != expected or result.status != "completed":
            raise SystemExit("physical snapshot evidence mismatch")
        with Image.open(Path(stage) / "image.jpg") as decoded:
            size = decoded.size
        if maximum_dimension and max(size) > maximum_dimension:
            raise SystemExit("retained image exceeded the requested dimension")
        print(json.dumps({"device": device["selected_endpoint"],
                          "bytes": len(image), "digest": expected,
                          "controls": len(result.data["actual_controls"]),
                          "size": size}))


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--maximum-dimension", type=int)
    arguments = parser.parse_args()
    asyncio.run(main(arguments.maximum_dimension))
