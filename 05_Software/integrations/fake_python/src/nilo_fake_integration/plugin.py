"""A deterministic protocol integration with no hardware or process dependencies."""

from __future__ import annotations

import asyncio
import hashlib
import json
from collections.abc import Awaitable, Callable
from datetime import UTC, datetime
from pathlib import Path
from uuid import UUID

from nilo_integration import EventSink
from nilo_integration.models import (
    ArtifactDeclaration,
    ConfigureRequest,
    ConfigureResult,
    DescribeResult,
    DiscoverResult,
    ExecuteRequest,
    ExecuteResult,
    HealthResult,
    Progress,
)

_ARTIFACT_BYTES = b"nilo fake artifact v1\n"
_ARTIFACT_NAME = "fake-artifact.bin"
_STABLE_DEVICE = "fake/device/stable-001"

Sleeper = Callable[[float], Awaitable[None]]
Clock = Callable[[], datetime]


class FakeOperationError(RuntimeError):
    """A selected, typed fake operation failure for protocol-boundary tests."""

    def __init__(self, code: str, message: str) -> None:
        super().__init__(message)
        self.code = code


class FakeIntegration:
    """A small deterministic Integration implementation for runner contract tests."""

    name = "nilo-fake-python"

    def __init__(
        self,
        *,
        sleeper: Sleeper = asyncio.sleep,
        clock: Clock | None = None,
    ) -> None:
        self._sleeper = sleeper
        self._clock = clock or (lambda: datetime.now(UTC))
        self._cancel_requested = asyncio.Event()
        self._shutdown = False

    async def describe(self) -> DescribeResult:
        return DescribeResult(status="ok")

    async def discover(self) -> DiscoverResult:
        return DiscoverResult(status="ok")

    async def health(self) -> HealthResult:
        # Keep a clock seam even though the v1 health payload intentionally has no timestamp.
        self._clock()
        return HealthResult(status="ok")

    async def configure(self, request: ConfigureRequest) -> ConfigureResult:
        del request
        return ConfigureResult(status="ok")

    async def execute(self, request: ExecuteRequest, events: EventSink) -> ExecuteResult:
        if request.operation == "echo":
            return await self._echo(request.parameters, events)
        if request.operation == "sleep":
            return await self._sleep(request.parameters, events)
        if request.operation == "fail":
            return self._fail(request.parameters)
        if request.operation == "crash":
            await events.emit(Progress(fraction=0.5, message="crash imminent", stage="crash"))
            raise SystemExit(70)
        if request.operation == "artifact":
            return await self._artifact(request.parameters, events)
        raise FakeOperationError(
            "unknown_operation", f"unsupported fake operation: {request.operation}"
        )

    async def cancel(self, request_id: UUID) -> None:
        del request_id
        self._cancel_requested.set()

    async def safe_shutdown(self) -> None:
        self._shutdown = True
        self._cancel_requested.set()

    async def _echo(self, parameters: object, events: EventSink) -> ExecuteResult:
        await events.emit(Progress(fraction=0.0, message="echo started", stage="start"))
        canonical = json.loads(json.dumps(parameters, sort_keys=True, separators=(",", ":")))
        await events.emit(Progress(fraction=1.0, message="echo completed", stage="complete"))
        return ExecuteResult(status="completed", data={"canonical": canonical})

    async def _sleep(self, parameters: object, events: EventSink) -> ExecuteResult:
        if not isinstance(parameters, dict) or not isinstance(parameters.get("duration_ms"), int):
            raise FakeOperationError("invalid_parameters", "sleep requires integer duration_ms")
        duration_ms = parameters["duration_ms"]
        if duration_ms < 0:
            raise FakeOperationError("invalid_parameters", "duration_ms must be non-negative")
        self._cancel_requested.clear()
        await events.emit(Progress(fraction=0.0, message="sleep started", stage="start"))
        sleeping = asyncio.create_task(self._sleeper(duration_ms / 1000))
        cancelled = asyncio.create_task(self._cancel_requested.wait())
        try:
            done, _ = await asyncio.wait((sleeping, cancelled), return_when=asyncio.FIRST_COMPLETED)
            if cancelled in done:
                if not sleeping.done():
                    sleeping.cancel()
                    await asyncio.gather(sleeping, return_exceptions=True)
                status = "cancelled"
            else:
                await sleeping
                status = "completed"
        finally:
            if not cancelled.done():
                cancelled.cancel()
            await asyncio.gather(cancelled, return_exceptions=True)
        await events.emit(Progress(fraction=1.0, message=f"sleep {status}", stage="complete"))
        return ExecuteResult(status=status, data={"duration_ms": duration_ms})

    @staticmethod
    def _fail(parameters: object) -> ExecuteResult:
        if not isinstance(parameters, dict):
            raise FakeOperationError("invalid_parameters", "fail requires an object")
        code = parameters.get("code", "fake_failure")
        message = parameters.get("message", "deterministic fake failure")
        if not isinstance(code, str) or not code or not isinstance(message, str) or not message:
            raise FakeOperationError(
                "invalid_parameters", "fail requires non-empty code and message"
            )
        raise FakeOperationError(code, message)

    async def _artifact(self, parameters: object, events: EventSink) -> ExecuteResult:
        if not isinstance(parameters, dict) or not isinstance(
            parameters.get("staging_directory"), str
        ):
            raise FakeOperationError("invalid_parameters", "artifact requires staging_directory")
        staging = Path(parameters["staging_directory"])
        if not staging.is_dir():
            raise FakeOperationError(
                "invalid_staging_directory", "staging_directory must be an existing directory"
            )
        destination = staging / _ARTIFACT_NAME
        destination.write_bytes(_ARTIFACT_BYTES)
        digest = hashlib.sha256(_ARTIFACT_BYTES).hexdigest()
        await events.emit(
            ArtifactDeclaration(
                id="fake-artifact",
                media_type="application/octet-stream",
                size_bytes=len(_ARTIFACT_BYTES),
                digest=f"sha256:{digest}",
            )
        )
        return ExecuteResult(status="completed", data={"artifact_id": "fake-artifact"})
