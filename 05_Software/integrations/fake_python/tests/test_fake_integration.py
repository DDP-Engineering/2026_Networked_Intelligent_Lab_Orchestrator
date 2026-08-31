import asyncio
import hashlib
import json
from pathlib import Path
from uuid import UUID

import pytest
from nilo_integration.models import ExecuteRequest, Progress

from nilo_fake_integration.plugin import FakeIntegration, FakeOperationError


class Events:
    def __init__(self) -> None:
        self.values: list[object] = []

    async def emit(self, event: object) -> None:
        self.values.append(event)


async def immediate_sleep(_: float) -> None:
    return None


@pytest.mark.asyncio
async def test_echo_emits_progress_and_returns_canonical_input() -> None:
    integration = FakeIntegration(sleeper=immediate_sleep)
    events = Events()
    parameters = {"value": ["hello", 2]}

    result = await integration.execute(
        ExecuteRequest(type="execute", operation="echo", parameters=parameters), events
    )

    assert [event.model_dump(mode="json") for event in events.values] == [
        Progress(fraction=0.0, message="echo started", stage="start").model_dump(mode="json"),
        Progress(fraction=1.0, message="echo completed", stage="complete").model_dump(mode="json"),
    ]
    assert result.status == "completed"
    assert result.data == {"canonical": json.loads(json.dumps(parameters, sort_keys=True))}


@pytest.mark.asyncio
async def test_sleep_reports_cancelled_after_cancel() -> None:
    started = asyncio.Event()
    release = asyncio.Event()

    async def blocking_sleep(_: float) -> None:
        started.set()
        await release.wait()

    integration = FakeIntegration(sleeper=blocking_sleep)
    events = Events()
    task = asyncio.create_task(
        integration.execute(
            ExecuteRequest(type="execute", operation="sleep", parameters={"duration_ms": 100}),
            events,
        )
    )
    await started.wait()
    await integration.cancel(UUID("00000000-0000-0000-0000-000000000001"))

    result = await task

    assert result.status == "cancelled"
    assert result.data == {"duration_ms": 100}
    assert [event.fraction for event in events.values] == [0.0, 1.0]


@pytest.mark.asyncio
async def test_fail_exposes_selected_typed_error_without_crashing() -> None:
    integration = FakeIntegration(sleeper=immediate_sleep)

    with pytest.raises(FakeOperationError) as raised:
        await integration.execute(
            ExecuteRequest(
                type="execute",
                operation="fail",
                parameters={"code": "fake_selected", "message": "planned failure"},
            ),
            Events(),
        )

    assert raised.value.code == "fake_selected"
    assert str(raised.value) == "planned failure"


@pytest.mark.asyncio
async def test_crash_emits_one_event_then_terminates() -> None:
    integration = FakeIntegration(sleeper=immediate_sleep)
    events = Events()

    with pytest.raises(SystemExit, match="70"):
        await integration.execute(
            ExecuteRequest(type="execute", operation="crash", parameters={}), events
        )

    assert [event.model_dump(mode="json") for event in events.values] == [
        Progress(fraction=0.5, message="crash imminent", stage="crash").model_dump(mode="json")
    ]


@pytest.mark.asyncio
async def test_artifact_writes_deterministic_bytes_inside_staging_directory(tmp_path: Path) -> None:
    integration = FakeIntegration(sleeper=immediate_sleep)
    events = Events()

    result = await integration.execute(
        ExecuteRequest(
            type="execute", operation="artifact", parameters={"staging_directory": str(tmp_path)}
        ),
        events,
    )

    expected = b"nilo fake artifact v1\n"
    artifact = tmp_path / "fake-artifact.bin"
    assert artifact.read_bytes() == expected
    assert list(tmp_path.iterdir()) == [artifact]
    event = events.values[-1]
    assert event.id == "fake-artifact"
    assert event.media_type == "application/octet-stream"
    assert event.size_bytes == len(expected)
    assert event.digest == f"sha256:{hashlib.sha256(expected).hexdigest()}"
    assert result.data == {"artifact_id": "fake-artifact"}
