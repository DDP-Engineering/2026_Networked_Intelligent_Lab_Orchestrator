"""Physical-camera runner contract tests with isolated V4L2 test doubles."""

import hashlib
import sys
from pathlib import Path

import pytest
from PIL import Image

sys.path.insert(0, str(Path(__file__).parents[1] / "src"))

IDENTITY = {"vendor": "0458", "product": "708f", "serial": "", "port_path": "1-1.3"}
BINDING = {"device_id": "cam-9", "endpoint_id": "video0", "hardware_identity": IDENTITY}


class Events:
    def __init__(self):
        self.items = []

    async def emit(self, event):
        self.items.append(event)


def install_camera_double(monkeypatch):
    from nilo_camera import v4l2

    monkeypatch.setattr(
        v4l2,
        "scan_usb_cameras",
        lambda: [
            {
                "id": "cam-1",
                **IDENTITY,
                "selected_endpoint": "video0",
                "endpoints": [{"id": "video0", "capture": True, "formats": ["MJPG"]}],
            }
        ],
    )
    monkeypatch.setattr(
        v4l2,
        "capture_image",
        lambda node, pixel_format, width, height: Image.new("RGB", (width, height), "blue"),
    )
    monkeypatch.setattr(v4l2, "read_controls", lambda node: ([], {}))


async def test_discovery_uses_physical_v4l2_scan(monkeypatch):
    from nilo_integration.models import ExecuteRequest

    from nilo_camera.plugin import CameraIntegration

    install_camera_double(monkeypatch)
    result = await CameraIntegration().execute(
        ExecuteRequest(type="execute", operation="discover_devices", parameters={}), Events()
    )
    assert result.data["devices"][0]["serial"] == ""
    assert result.data["devices"][0]["selected_endpoint"] == "video0"


@pytest.mark.parametrize("failure", ["", "capture_failed"])
async def test_stream_reports_capture_failure(monkeypatch, failure):
    from types import SimpleNamespace

    from nilo_integration import ExecuteFailure
    from nilo_integration.models import ExecuteRequest

    from nilo_camera import workspace
    from nilo_camera.plugin import CameraIntegration

    monkeypatch.setattr(
        workspace, "Workspace", lambda *args: SimpleNamespace(run=lambda: None, error=failure)
    )
    request = ExecuteRequest(type="execute", operation="stream", parameters={"device_id": "cam-1"})
    if failure:
        with pytest.raises(ExecuteFailure):
            await CameraIntegration().execute(request, Events())
    else:
        assert (await CameraIntegration().execute(request, Events())).status == "completed"


async def test_snapshot_revalidates_physical_binding_and_retains_image(tmp_path, monkeypatch):
    from nilo_integration import ExecuteFailure
    from nilo_integration.models import ExecuteRequest

    from nilo_camera.plugin import CameraIntegration

    install_camera_double(monkeypatch)
    events = Events()
    result = await CameraIntegration().execute(
        ExecuteRequest(
            type="execute",
            operation="snapshot",
            parameters={
                **BINDING,
                "staging_directory": str(tmp_path),
            },
        ),
        events,
    )
    assert result.status == "completed"
    image_bytes = (tmp_path / "image.jpg").read_bytes()
    artifact = events.items[0]
    assert artifact.id == "image"
    assert artifact.digest == f"sha256:{hashlib.sha256(image_bytes).hexdigest()}"
    assert result.data["captured_at"].endswith("Z")
    assert result.data["selected_endpoint"] == "video0"
    assert result.data["warmup"]["discarded_frames"] == 2
    assert result.data["artifact_size_bytes"] == len(image_bytes)
    with Image.open(tmp_path / "image.jpg") as image:
        assert image.size == (640, 480)
        assert not image.getexif()
    with pytest.raises(ExecuteFailure):
        await CameraIntegration().execute(
            ExecuteRequest(
                type="execute",
                operation="snapshot",
                parameters={
                    **BINDING,
                    "hardware_identity": {**IDENTITY, "port_path": "1-1.8"},
                    "staging_directory": str(tmp_path),
                },
            ),
            Events(),
        )


async def test_snapshot_requested_maximum_dimension_resizes_retained_image(tmp_path, monkeypatch):
    from nilo_integration.models import ExecuteRequest

    from nilo_camera.plugin import CameraIntegration

    install_camera_double(monkeypatch)
    result = await CameraIntegration().execute(
        ExecuteRequest(
            type="execute",
            operation="snapshot",
            parameters={**BINDING, "staging_directory": str(tmp_path), "maximum_dimension": 320},
        ),
        Events(),
    )
    with Image.open(tmp_path / "image.jpg") as image:
        assert image.size == (320, 240)
    assert result.data["effective"]["output_width"] == 320
    assert result.data["effective"]["output_height"] == 240


async def test_snapshot_frame_interval_reaches_physical_capture(tmp_path, monkeypatch):
    from nilo_integration import ExecuteFailure
    from nilo_integration.models import ExecuteRequest

    from nilo_camera import v4l2
    from nilo_camera.plugin import CameraIntegration

    install_camera_double(monkeypatch)
    selected = []

    def capture(node, pixel_format, width, height, interval):
        selected.append(interval)
        return Image.new("RGB", (width, height), "blue")

    monkeypatch.setattr(v4l2, "capture_image", capture)
    result = await CameraIntegration().execute(
        ExecuteRequest(
            type="execute",
            operation="snapshot",
            parameters={
                **BINDING,
                "staging_directory": str(tmp_path),
                "frame_interval": {"numerator": 1, "denominator": 30},
            },
        ),
        Events(),
    )
    assert selected == [{"numerator": 1, "denominator": 30}]
    assert result.data["effective"]["frame_interval"] == selected[0]
    with pytest.raises(ExecuteFailure) as error:
        await CameraIntegration().execute(
            ExecuteRequest(
                type="execute",
                operation="snapshot",
                parameters={
                    **BINDING,
                    "staging_directory": str(tmp_path),
                    "frame_interval": {"numerator": 0, "denominator": 30},
                },
            ),
            Events(),
        )
    assert error.value.code == "invalid_parameters"


async def test_configured_limit_applies_to_physical_snapshot(tmp_path, monkeypatch):
    from nilo_integration import ExecuteFailure
    from nilo_integration.models import ConfigureRequest, ExecuteRequest

    from nilo_camera.plugin import CameraIntegration

    install_camera_double(monkeypatch)
    runner = CameraIntegration()
    await runner.configure(
        ConfigureRequest(
            type="configure",
            configuration={
                "schema_version": 1,
                "plugin": {"revision": 2, "settings": {"maximum_width": 320}},
                "device": {
                    "id": "cam-9",
                    "revision": 3,
                    "settings": {"baseline_controls": {"brightness": 7}},
                },
            },
        )
    )
    with pytest.raises(ExecuteFailure) as error:
        await runner.execute(
            ExecuteRequest(
                type="execute",
                operation="snapshot",
                parameters={
                    **BINDING,
                    "staging_directory": str(tmp_path),
                    "width": 640,
                },
            ),
            Events(),
        )
    assert error.value.code == "image_too_large"
