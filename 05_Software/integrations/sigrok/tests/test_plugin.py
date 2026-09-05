"""@file    test_plugin.py
@brief   Integration boundary checks; these do not qualify sigrok decoding correctness.
@details Defines the package import boundary.
"""

import asyncio
import sys
import zipfile

import pytest
from nilo_integration import ExecuteFailure
from nilo_integration.models import ExecuteRequest

from nilo_sigrok.plugin import SigrokIntegration, capture_parameters, parse_devices, read_capture


class Events:
    def __init__(self):
        self.items = []

    async def emit(self, event):
        self.items.append(event)


def capture(tmp_path):
    return {
        "conn": "1.3",
        "channels": ["D0", "D1"],
        "samplerate": 1_000_000,
        "samples": 1024,
        "staging_directory": str(tmp_path),
    }


def test_discovery_filters_pwm_and_preserves_distinct_connections():
    devices = parse_devices(
        "\n".join(
            f"atk-dl16:conn={conn} - ALIENTEK ATK DL16 with 18 channels: D0 PWM0 PWM1"
            for conn in ["1.3", "1.4"]
        )
    )
    assert [device["connection"] for device in devices] == ["1.3", "1.4"]
    assert devices[0]["channels"] == [f"D{i}" for i in range(16)]


@pytest.mark.parametrize(
    "change",
    [
        {"channels": ["PWM0"]},
        {"channels": ["D0", "D0"]},
        {"pwm": True},
        {"conn": "1.3:pwm=on"},
        {"samples": 0},
        {"samples": 1_000_001},
        {"samplerate": 500_000_000},
        {"samples": True},
        {"command": "anything"},
    ],
)
def test_capture_rejects_forbidden_or_unbounded_parameters(tmp_path, change):
    with pytest.raises(ExecuteFailure):
        capture_parameters({**capture(tmp_path), **change})


async def test_simulated_capture_has_correlated_artifact_and_bounded_preview(tmp_path):
    runner = SigrokIntegration(simulation=True)
    events = Events()
    result = await runner.execute(
        ExecuteRequest(type="execute", operation="capture", parameters=capture(tmp_path)), events
    )
    assert result.status == "completed"
    assert result.data["actual_samples"] == 1024
    assert len(result.data["preview"]) == 256
    assert result.data["simulation"] is True
    artifact = next(event for event in events.items if event.type == "artifact")
    assert artifact.id == "capture"
    assert artifact.digest.startswith("sha256:")
    decoded = await runner.decode_uart(
        {"input_path": str(tmp_path / "capture.sr"), "channel": "D0", "baudrate": 115200}
    )
    assert decoded.data["annotations"][0]["start_sample"] == "0"
    assert decoded.data["simulation"] is True


async def test_cancel_cleans_child_before_handler_finishes():
    runner = SigrokIntegration(backend=sys.executable)
    task = asyncio.create_task(runner.command(["-c", "import time; time.sleep(60)"]))
    while not runner.processes:
        await asyncio.sleep(0.01)
    child = next(iter(runner.processes))
    task.cancel()
    with pytest.raises(asyncio.CancelledError):
        await task
    assert child.returncode is not None
    assert not runner.processes


def test_capture_archive_rejects_decompression_over_budget(tmp_path):
    path = tmp_path / "bad.sr"
    with zipfile.ZipFile(path, "w", zipfile.ZIP_DEFLATED) as archive:
        archive.writestr("logic-1-1", bytes((8 << 20) + 1))
    with pytest.raises(ExecuteFailure, match="Unpacked"):
        read_capture(path)


# END OF FILE
