"""Bounded, stateful NDJSON server for one NILO integration runner."""

from __future__ import annotations

import asyncio
import contextlib
import json
import sys
from dataclasses import dataclass, field
from datetime import UTC, datetime
from typing import Any, BinaryIO, TextIO
from uuid import UUID, uuid4

from pydantic import ValidationError

from nilo_integration.api import EventSink, Integration
from nilo_integration.models import (
    MAX_LINE_BYTES,
    CancelRequest,
    CancelResult,
    ConfigureRequest,
    ConfigureResult,
    ControlRequest,
    DescribeResult,
    DiscoverResult,
    Envelope,
    ErrorResult,
    EventPayload,
    ExecuteRequest,
    ExecuteResult,
    HealthResult,
    HelloRequest,
    HelloResponse,
    Kind,
    Method,
    RequestPayload,
    SafeShutdownResult,
    parse_event_payload,
    parse_request_payload,
)

MAX_DIAGNOSTIC_BYTES = 4096

_RESPONSE_MODELS = {
    "hello": HelloResponse,
    "describe": DescribeResult,
    "discover": DiscoverResult,
    "health": HealthResult,
    "configure": ConfigureResult,
    "execute": ExecuteResult,
    "cancel": CancelResult,
    "safe_shutdown": SafeShutdownResult,
}


class ProtocolViolation(ValueError):
    """A protocol error, optionally safe to report using the supplied request ID."""

    def __init__(self, message: str, *, reply: bool = True) -> None:
        super().__init__(message)
        self.reply = reply


@dataclass
class _Execution:
    request: Envelope
    terminal: bool = False
    lock: asyncio.Lock = field(default_factory=asyncio.Lock)
    task: asyncio.Task[None] | None = None


class _ServerEventSink(EventSink):
    def __init__(self, server: ProtocolServer, execution: _Execution) -> None:
        self._server = server
        self._execution = execution

    async def emit(self, event: EventPayload) -> None:
        if not hasattr(event, "model_dump"):
            raise ProtocolViolation("emitted event must be a typed protocol model")
        try:
            event = parse_event_payload(event.model_dump(mode="json"))
        except (ValidationError, ValueError, TypeError) as error:
            raise ProtocolViolation("emitted event is not a valid protocol event") from error
        async with self._execution.lock:
            if self._execution.terminal:
                raise ProtocolViolation("cannot emit an event after terminal execute response")
            await self._server._send(
                "event",
                self._execution.request.message_id,
                "execute",
                event.model_dump(mode="json"),
            )


class ProtocolServer:
    """Serve a strict protocol-v1 request stream for one integration process.

    The original stdout buffer is captured for protocol records. While the server
    runs, regular and background integration diagnostics are redirected to stderr.
    """

    def __init__(
        self,
        integration: Integration,
        *,
        protocol_output: BinaryIO | None = None,
        diagnostic_output: TextIO | None = None,
    ) -> None:
        self._integration = integration
        self._protocol_output = protocol_output or getattr(sys.stdout, "buffer", sys.stdout)
        self._diagnostic_output = diagnostic_output or sys.stderr
        self._write_lock = asyncio.Lock()
        self._message_ids: set[str] = set()
        self._executions: dict[str, _Execution] = {}
        self._hello_complete = False

    async def run(self, reader: asyncio.StreamReader) -> int:
        """Handle requests until clean EOF, returning a process-style status code."""

        with contextlib.redirect_stdout(self._diagnostic_output):
            return await self._run(reader)

    async def _run(self, reader: asyncio.StreamReader) -> int:
        while True:
            try:
                line = await self._read_line(reader)
            except ProtocolViolation as error:
                await self._abort_executions()
                self._diagnose(error)
                return 2
            if line is None:
                await asyncio.sleep(0)
                if self._active_executions():
                    await self._abort_executions()
                    self._diagnose("EOF during active execute operation")
                    return 2
                return 0

            raw: dict[str, Any] | None = None
            try:
                raw = self._decode_object(line)
                envelope = Envelope.model_validate(raw)
                if envelope.kind != "request":
                    raise ProtocolViolation(
                        "runner input must contain request messages", reply=False
                    )
                self._accept_request(envelope)
                payload = parse_request_payload(envelope.method, envelope.payload)
                if not self._hello_complete and envelope.method != "hello":
                    raise ProtocolViolation("hello is required before operational requests")
                await self._handle(envelope, payload)
                if envelope.method == "hello":
                    self._hello_complete = True
            except (ValidationError, ProtocolViolation, ValueError, TypeError) as error:
                if await self._reply_if_recoverable(raw, error):
                    continue
                await self._abort_executions()
                self._diagnose(error)
                return 2
            except Exception as error:  # noqa: BLE001 - process boundary must not crash
                await self._abort_executions()
                self._diagnose(f"unexpected server error: {type(error).__name__}")
                return 2

    def _accept_request(self, envelope: Envelope) -> None:
        if envelope.message_id in self._message_ids:
            raise ProtocolViolation(
                f"duplicate integration message ID {envelope.message_id!r}", reply=False
            )
        self._message_ids.add(envelope.message_id)

    def _active_executions(self) -> list[_Execution]:
        return [
            execution
            for execution in self._executions.values()
            if execution.task and not execution.task.done()
        ]

    async def _abort_executions(self) -> None:
        executions = self._active_executions()
        for execution in executions:
            async with execution.lock:
                execution.terminal = True
            if execution.task is not None:
                execution.task.cancel()
        if executions:
            await asyncio.gather(
                *(execution.task for execution in executions if execution.task is not None),
                return_exceptions=True,
            )

    async def _read_line(self, reader: asyncio.StreamReader) -> bytes | None:
        """Read a line without retaining more than the full 1 MiB frame limit."""

        line = bytearray()
        for _ in range(MAX_LINE_BYTES - 1):
            byte = await reader.read(1)
            if not byte:
                if not line:
                    return None
                raise ProtocolViolation("message is missing newline delimiter")
            if byte == b"\n":
                return bytes(line)
            line.extend(byte)
        byte = await reader.read(1)
        if byte == b"\n":
            return bytes(line)
        if not byte:
            raise ProtocolViolation("message is missing newline delimiter")
        raise ProtocolViolation("message exceeds one MiB line limit")

    @staticmethod
    def _decode_object(line: bytes) -> dict[str, Any]:
        try:
            value = json.loads(
                line,
                object_pairs_hook=ProtocolServer._no_duplicate_keys,
                parse_constant=ProtocolServer._reject_non_standard_constant,
            )
        except (UnicodeDecodeError, json.JSONDecodeError) as error:
            raise ProtocolViolation(f"invalid JSON: {error.msg}") from error
        if not isinstance(value, dict):
            raise ProtocolViolation("message must be a JSON object")
        return value

    @staticmethod
    def _reject_non_standard_constant(value: str) -> None:
        raise ProtocolViolation(f"non-standard JSON constant {value!r}")

    @staticmethod
    def _no_duplicate_keys(pairs: list[tuple[str, Any]]) -> dict[str, Any]:
        value: dict[str, Any] = {}
        for key, item in pairs:
            if key in value:
                raise ProtocolViolation(f"duplicate JSON key {key!r}")
            value[key] = item
        return value

    async def _handle(self, envelope: Envelope, payload: RequestPayload) -> None:
        if isinstance(payload, HelloRequest):
            name = getattr(self._integration, "name", type(self._integration).__name__)
            await self._respond(envelope, HelloResponse(integration=name))
            return
        if isinstance(payload, ControlRequest):
            if envelope.method == "describe":
                await self._respond(envelope, await self._call(self._integration.describe()))
            elif envelope.method == "discover":
                await self._respond(envelope, await self._call(self._integration.discover()))
            elif envelope.method == "health":
                await self._respond(envelope, await self._call(self._integration.health()))
            else:
                await self._call(self._integration.safe_shutdown())
                await self._respond(envelope, SafeShutdownResult())
            return
        if isinstance(payload, ConfigureRequest):
            await self._respond(envelope, await self._call(self._integration.configure(payload)))
            return
        if isinstance(payload, CancelRequest):
            execution = self._executions.get(payload.target_request_id)
            if execution is None or execution.task is None or execution.task.done():
                raise ProtocolViolation("cancel target is not an active execute request")
            try:
                target_request_id = UUID(payload.target_request_id)
            except ValueError as error:
                raise ProtocolViolation("cancel target_request_id must be a UUID") from error
            await self._call(self._integration.cancel(target_request_id))
            await self._respond(envelope, CancelResult())
            return
        if isinstance(payload, ExecuteRequest):
            execution = _Execution(request=envelope)
            self._executions[envelope.message_id] = execution
            execution.task = asyncio.create_task(self._execute(payload, execution))
            return
        raise ProtocolViolation("unsupported request payload")

    async def _execute(self, request: ExecuteRequest, execution: _Execution) -> None:
        try:
            result = await self._call(
                self._integration.execute(request, _ServerEventSink(self, execution))
            )
            async with execution.lock:
                execution.terminal = True
                await self._respond(execution.request, result)
        except asyncio.CancelledError:
            async with execution.lock:
                execution.terminal = True
            raise
        except Exception as error:  # noqa: BLE001 - integration must not crash its runner
            async with execution.lock:
                if not execution.terminal:
                    execution.terminal = True
                    await self._send_error(
                        execution.request.message_id, "execute", "execution_failed", str(error)
                    )
        finally:
            self._executions.pop(execution.request.message_id, None)

    async def _call(self, awaitable: Any) -> Any:
        return await awaitable

    async def _respond(self, request: Envelope, result: Any) -> None:
        if not hasattr(result, "model_dump"):
            raise ProtocolViolation("integration returned an untyped result")
        try:
            result = _RESPONSE_MODELS[request.method].model_validate(result.model_dump(mode="json"))
        except ValidationError as error:
            raise ProtocolViolation("integration returned an invalid result") from error
        await self._send(
            "response", request.message_id, request.method, result.model_dump(mode="json")
        )

    async def _reply_if_recoverable(self, raw: dict[str, Any] | None, error: Exception) -> bool:
        if isinstance(error, ProtocolViolation) and not error.reply:
            return False
        if raw is None or raw.get("kind") != "request":
            return False
        message_id = raw.get("message_id")
        method = raw.get("method")
        if not isinstance(message_id, str) or not message_id.strip() or not isinstance(method, str):
            return False
        if method not in _RESPONSE_MODELS:
            return False
        await self._send_error(message_id, method, "invalid_request", str(error))
        return True

    async def _send_error(self, request_id: str, method: Method, code: str, message: str) -> None:
        if method == "execute":
            payload = ErrorResult(code=code, message=message).model_dump(mode="json")
        else:
            payload = {"type": method, "status": "error"}
        await self._send("response", request_id, method, payload)

    async def _send(
        self, kind: Kind, request_id: str, method: Method, payload: dict[str, Any]
    ) -> None:
        async with self._write_lock:
            try:
                json.dumps(payload, allow_nan=False)
            except (TypeError, ValueError) as error:
                raise ProtocolViolation("cannot encode protocol JSON output") from error
            message_id = self._new_message_id()
            envelope = Envelope(
                protocol="nilo.integration/1",
                kind=kind,
                message_id=message_id,
                request_id=request_id,
                method=method,
                timestamp=datetime.now(UTC),
                payload=payload,
            )
            try:
                encoded = json.dumps(
                    envelope.model_dump(mode="json"), separators=(",", ":"), allow_nan=False
                ).encode()
            except (TypeError, ValueError) as error:
                raise ProtocolViolation("cannot encode protocol JSON output") from error
            encoded += b"\n"
            if len(encoded) > MAX_LINE_BYTES:
                raise ProtocolViolation("generated message exceeds one MiB line limit")
            self._message_ids.add(message_id)
            written = self._protocol_output.write(encoded)
            if written is not None and written != len(encoded):
                raise ProtocolViolation("short protocol output write", reply=False)
            self._protocol_output.flush()

    def _new_message_id(self) -> str:
        message_id = str(uuid4())
        while message_id in self._message_ids:
            message_id = str(uuid4())
        return message_id

    def _diagnose(self, error: Exception | str) -> None:
        message = str(error).replace("\n", " ").replace("\r", " ")
        bounded = message.encode("utf-8", "replace")[:MAX_DIAGNOSTIC_BYTES].decode(
            "utf-8", "replace"
        )
        self._diagnostic_output.write(f"nilo integration protocol error: {bounded}\n")
        self._diagnostic_output.flush()
