"""A deterministic protocol integration with no hardware or process dependencies."""

from __future__ import annotations

import asyncio
import hashlib
import json
import os
import stat
from collections.abc import Awaitable, Callable
from datetime import UTC, datetime
from pathlib import Path
from typing import NoReturn
from uuid import UUID, uuid4

from nilo_integration import EventSink, ExecuteFailure, current_execute_request_id
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
UUIDFactory = Callable[[], UUID]
Crash = Callable[[int], NoReturn]


class FakeOperationError(ExecuteFailure):
    """A selected, typed fake operation failure for protocol-boundary tests."""

    def __init__(self, code: str, message: str, *, retryable: bool = False) -> None:
        super().__init__(code, message, retryable=retryable)


class FakeIntegration:
    """A small deterministic Integration implementation for runner contract tests."""

    name = "fake-python"

    def __init__(
        self,
        *,
        sleeper: Sleeper = asyncio.sleep,
        clock: Clock | None = None,
        uuid_factory: UUIDFactory = uuid4,
        crash: Crash = os._exit,
    ) -> None:
        self._sleeper = sleeper
        self._clock = clock or (lambda: datetime.now(UTC))
        self._uuid_factory = uuid_factory
        self._crash = crash
        self._cancellations: dict[UUID, asyncio.Event] = {}
        self._pending_cancellations: set[UUID] = set()
        self._shutdown_observed = False
        self._last_observed_at: datetime | None = None

    @property
    def shutdown_observed(self) -> bool:
        return self._shutdown_observed

    @property
    def last_observed_at(self) -> datetime | None:
        return self._last_observed_at

    async def describe(self) -> DescribeResult:
        return DescribeResult(
            status="ok",
            integration=self.name,
            operations=["echo", "artifact", "sleep", "fail", "crash"],
        )

    async def discover(self) -> DiscoverResult:
        return DiscoverResult(status="ok")

    async def health(self) -> HealthResult:
        self._observe()
        return HealthResult(status="stopped" if self._shutdown_observed else "ok")

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
            self._crash(70)
        if request.operation == "artifact":
            return await self._artifact(request.parameters, events)
        raise FakeOperationError(
            "unknown_operation", f"unsupported fake operation: {request.operation}"
        )

    async def cancel(self, request_id: UUID) -> None:
        event = self._cancellations.get(request_id)
        if event is None:
            self._pending_cancellations.add(request_id)
            return
        event.set()

    async def safe_shutdown(self) -> None:
        self._shutdown_observed = True
        for event in self._cancellations.values():
            event.set()

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
        request_id, cancellation = self._register_active_request()
        await events.emit(Progress(fraction=0.0, message="sleep started", stage="start"))
        sleeping = asyncio.create_task(self._sleeper(duration_ms / 1000))
        cancelled = asyncio.create_task(cancellation.wait())
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
            self._cancellations.pop(request_id, None)
            self._pending_cancellations.discard(request_id)
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
        self._write_artifact(staging)
        digest = hashlib.sha256(_ARTIFACT_BYTES).hexdigest()
        await events.emit(
            ArtifactDeclaration(
                id="fake_artifact",
                media_type="application/octet-stream",
                size_bytes=len(_ARTIFACT_BYTES),
                digest=f"sha256:{digest}",
            )
        )
        return ExecuteResult(status="completed", data={"artifact_id": "fake_artifact"})

    def _register_active_request(self) -> tuple[UUID, asyncio.Event]:
        request_id = current_execute_request_id() or self._uuid_factory()
        cancellation = asyncio.Event()
        self._cancellations[request_id] = cancellation
        if request_id in self._pending_cancellations:
            cancellation.set()
        return request_id, cancellation

    def _observe(self) -> datetime:
        self._last_observed_at = self._clock()
        return self._last_observed_at

    @staticmethod
    def _write_artifact(staging: Path) -> Path:
        try:
            directory = os.lstat(staging)
        except OSError as error:
            raise FakeOperationError(
                "invalid_staging_directory", "cannot inspect staging_directory"
            ) from error
        if stat.S_ISLNK(directory.st_mode) or not stat.S_ISDIR(directory.st_mode):
            raise FakeOperationError(
                "invalid_staging_directory", "staging_directory must be a real directory"
            )
        destination = staging / _ARTIFACT_NAME
        flags = os.O_WRONLY | os.O_CREAT | os.O_EXCL
        if hasattr(os, "O_BINARY"):
            flags |= os.O_BINARY
        if hasattr(os, "O_NOFOLLOW"):
            flags |= os.O_NOFOLLOW
        try:
            descriptor = os.open(destination, flags, 0o600)
        except FileExistsError as error:
            raise FakeOperationError(
                "artifact_exists", "artifact destination already exists"
            ) from error
        except OSError as error:
            raise FakeOperationError(
                "artifact_write_failed", "cannot create artifact destination"
            ) from error
        try:
            if not stat.S_ISREG(os.fstat(descriptor).st_mode):
                raise FakeOperationError(
                    "artifact_write_failed", "artifact destination is not regular"
                )
            offset = 0
            while offset < len(_ARTIFACT_BYTES):
                offset += os.write(descriptor, _ARTIFACT_BYTES[offset:])
            os.fsync(descriptor)
        finally:
            os.close(descriptor)
        return destination
