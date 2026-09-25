"""Physical camera identity and endpoint selection tests."""

import io
import sys
import types
from pathlib import Path

import pytest
from PIL import Image

sys.path.insert(0, str(Path(__file__).parents[1] / "src"))


def test_grouping_excludes_metadata_endpoint_from_capture():
    from nilo_camera.v4l2 import group_observed_nodes

    devices = group_observed_nodes(
        [
            {
                "name": "video0",
                "usb_parent": "1-1.3",
                "vendor": "0458",
                "product": "708f",
                "serial": "",
                "interface": "00",
                "capture": True,
                "formats": ["MJPG", "YUYV"],
                "sizes": {"MJPG": [[640, 480]], "YUYV": [[640, 480]]},
            },
            {
                "name": "video1",
                "usb_parent": "1-1.3",
                "vendor": "0458",
                "product": "708f",
                "serial": "",
                "interface": "00",
                "capture": False,
                "formats": [],
            },
        ]
    )
    assert len(devices) == 1
    assert devices[0]["id"] == "cam-1"
    assert devices[0]["selected_endpoint"] == "video0"
    assert devices[0]["endpoints"][1]["capture"] is False
    assert devices[0]["serial"] == ""
    assert devices[0]["endpoints"][0]["sizes"]["MJPG"] == [[640, 480]]


def test_decode_frame_rejects_oversize_and_strips_source_metadata():
    from nilo_camera.v4l2 import frame_to_image

    source = Image.new("RGB", (2, 2), "red")
    encoded = io.BytesIO()
    source.save(encoded, format="JPEG", exif=b"Exif\x00\x00private")
    decoded = frame_to_image(encoded.getvalue(), "MJPG", 2, 2)
    assert decoded.size == (2, 2)
    assert not decoded.getexif()
    decoded.close()
    with pytest.raises(ValueError, match="limit"):
        frame_to_image(encoded.getvalue(), "MJPG", 4096, 4096)
    with pytest.raises(ValueError, match="short"):
        frame_to_image(b"\x00", "YUYV", 2, 2)


def test_capture_selects_advertised_format_and_discards_warmup(monkeypatch):
    from nilo_camera.v4l2 import capture_image

    encoded = io.BytesIO()
    Image.new("RGB", (2, 2), "red").save(encoded, format="JPEG")

    class Format:
        pixelformat = int.from_bytes(b"MJPG", "little")

    chosen = Format()
    size = types.SimpleNamespace(width=2, height=2)

    class Device:
        def __init__(self, path):
            assert path == "/dev/video0"
            self.available_formats = {chosen: [size]}

        def set_format(self, pixel_format, frame_size):
            assert pixel_format is chosen and frame_size is size

        def get_format(self):
            return chosen, size

    class Stream:
        def __init__(self, device):
            assert isinstance(device, Device)

        def __iter__(self):
            return iter([b"warmup-one", b"warmup-two", encoded.getvalue()])

    monkeypatch.setitem(
        sys.modules, "pyrav4l2", types.SimpleNamespace(Device=Device, Stream=Stream)
    )
    image = capture_image("/dev/video0", "MJPG", 2, 2)
    assert image.size == (2, 2)
    image.close()


def test_capture_sets_only_an_advertised_frame_interval(monkeypatch):
    from nilo_camera.v4l2 import capture_image

    encoded = io.BytesIO()
    Image.new("RGB", (2, 2), "red").save(encoded, format="JPEG")
    class Format:
        pixelformat = int.from_bytes(b"MJPG", "little")

    chosen = Format()
    size = types.SimpleNamespace(width=2, height=2)

    class Interval:
        def __init__(self, numerator, denominator):
            self.numerator = numerator
            self.denominator = denominator

    class Device:
        def __init__(self, path):
            self.available_formats = {chosen: [size]}
            self.interval = Interval(1, 30)

        def set_format(self, pixel_format, frame_size):
            assert pixel_format is chosen and frame_size is size

        def get_format(self):
            return chosen, size

        def get_available_frame_intervals(self, pixel_format, frame_size):
            return [Interval(1, 30), Interval(1, 15)]

        def set_frame_interval(self, interval):
            self.interval = interval

        def get_frame_interval(self):
            return self.interval

    class Stream:
        def __init__(self, device):
            self.device = device

        def __iter__(self):
            return iter([b"warmup-one", b"warmup-two", encoded.getvalue()])

    monkeypatch.setitem(sys.modules, "pyrav4l2", types.SimpleNamespace(Device=Device, Stream=Stream))
    image = capture_image("/dev/video0", "MJPG", 2, 2, {"numerator": 1, "denominator": 15})
    image.close()
    with pytest.raises(ValueError, match="interval"):
        capture_image("/dev/video0", "MJPG", 2, 2, {"numerator": 1, "denominator": 60})


def test_typed_v4l2_control_read_set_and_reset(monkeypatch):
    from nilo_camera.v4l2 import apply_controls, read_controls

    controls = [
        types.SimpleNamespace(
            id=9963776,
            name="Brightness",
            type=1,
            minimum=-255,
            maximum=255,
            step=1,
            default_value=0,
            flags=0,
            is_disabled=False,
            is_read_only=False,
            is_inactive=False,
            is_volatile=False,
        ),
        types.SimpleNamespace(
            id=9963788,
            name="White Balance, Automatic",
            type=2,
            minimum=0,
            maximum=1,
            step=1,
            default_value=1,
            flags=0,
            is_disabled=False,
            is_read_only=False,
            is_inactive=False,
            is_volatile=False,
        ),
    ]
    values = {9963776: 0, 9963788: 1}

    class Device:
        def __init__(self, path):
            assert path == "/dev/video0"
            self.controls = controls

        def get_control_value(self, control):
            return values[control.id]

        def set_control_value(self, control, value):
            values[control.id] = value

    monkeypatch.setitem(sys.modules, "pyrav4l2", types.SimpleNamespace(Device=Device))
    descriptors, current = read_controls("/dev/video0")
    assert current["brightness"] == 0
    assert {item["id"] for item in descriptors} == {"brightness", "auto_white_balance"}
    actual = apply_controls("/dev/video0", {"brightness": 12})
    assert actual["brightness"] == 12
    with pytest.raises(ValueError, match="range"):
        apply_controls("/dev/video0", {"brightness": 999})
    assert values[9963776] == 12
    assert apply_controls("/dev/video0", {"brightness": 0})["brightness"] == 0
