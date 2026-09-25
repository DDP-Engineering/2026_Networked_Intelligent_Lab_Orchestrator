"""Read, change, and restore one reversible control through the camera package."""

import json
import sys

sys.dont_write_bytecode = True

from nilo_camera import v4l2


def main():
    cameras = [item for item in v4l2.scan_usb_cameras() if item["selected_endpoint"]]
    if not cameras:
        raise SystemExit("no supported camera")
    node = "/dev/" + cameras[0]["selected_endpoint"]
    descriptors, values = v4l2.read_controls(node)
    brightness = next(item for item in descriptors if item["id"] == "brightness")
    original = values["brightness"]
    step = brightness["step"]
    alternate = original + step if original + step <= brightness["maximum"] else original - step
    if alternate < brightness["minimum"]:
        raise SystemExit("brightness has no reversible alternate")
    changed = None
    try:
        changed = v4l2.apply_controls(node, {"brightness": alternate})["brightness"]
    finally:
        restored = v4l2.apply_controls(node, {"brightness": original})["brightness"]
    if changed != alternate or restored != original:
        raise SystemExit("control readback failed")
    print(json.dumps({"node": node, "descriptors": len(descriptors), "changed": changed,
                      "restored": restored == original}))


if __name__ == "__main__":
    main()
