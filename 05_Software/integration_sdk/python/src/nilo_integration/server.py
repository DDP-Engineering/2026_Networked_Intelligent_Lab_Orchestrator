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

from nilo_integration.api import EventSink, ExecuteFailure, Integration, _execute_request_id
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
class _Request:
    envelope: Envelope
    terminal: bool = False
    closing: bool = False
    failure: str | None = None
    lock: asyncio.Lock = field(default_factory=asyncio.Lock)
    task: asyncio.Task[None] | None = None


class _ServerEventSink(EventSink):
    def __init__(self, server: ProtocolServer, request: _Request) -> None:
        self._server = server
        self._request = request

    async def emit(self, event: EventPayload) -> None:
        if not hasattr(event, "model_dump"):
            raise ProtocolViolation("emitted event must be a typed protocol model")
        try:
            event = parse_event_payload(event.model_dump(mode="json"))
        except (ValidationError, ValueError, TypeError) as error:
            raise ProtocolViolation("emitted event is not a valid protocol event") from error
        async with self._request.lock:
            if self._request.closing or self._request.terminal:
                raise ProtocolViolation("cannot emit an event after terminal execute response")
            await self._server._send(
                "event",
                self._request.envelope.message_id,
                "execute",
                event.model_dump(mode="json"),
            )


class ProtocolServer:
    """Serve a strict protocol-v1 request stream for one integration process.

    The original stdout buffer is captured for protocol records. While the server
    and its tracked integration requests run, ordinary stdout is redirected to
    diagnostics.
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
        self._requests: dict[str, _Request] = {}
        self._hello_complete = False
        self._fatal_event = asyncio.Event()
        self._fatal_error: str | None = None

    async def run(self, reader: asyncio.StreamReader) -> int:
        """Handle requests until clean EOF, returning a process-style status code."""

        with contextlib.redirect_stdout(self._diagnostic_output):
            try:
                return await self._run(reader)
            finally:
                await self._abort_requests()

    async def _run(self, reader: asyncio.StreamReader) -> int:
        while True:
            try:
                line = await self._read_line_or_fatal(reader)
            except ProtocolViolation as error:
                self._diagnose(error)
                return 2
            if line is None:
                await asyncio.sleep(0)
                if self._active_requests():
                    self._diagnose("EOF during active integration request")
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
                request = self._start_request(envelope, payload)
                if envelope.method == "hello":
                    await request.task
                    if not request.terminal:
                        raise ProtocolViolation(
                            self._fatal_error or "hello response was not delivered", reply=False
                        )
                    self._hello_complete = True
            except (ValidationError, ProtocolViolation, ValueError, TypeError) as error:
                try:
                    replied = await self._reply_if_recoverable(raw, error)
                except Exception as delivery_error:  # noqa: BLE001 - protocol writer failure
                    self._record_fatal(
                        f"failed to deliver protocol error: {self._error_text(delivery_error)}"
                    )
                    self._diagnose(self._fatal_error)
                    return 2
                if replied:
                    continue
                self._diagnose(error)
                return 2
            except Exception as error:  # noqa: BLE001 - process boundary must not crash
                self._record_fatal(f"unexpected server error: {type(error).__name__}")
                self._diagnose(self._fatal_error)
                return 2

    async def _read_line_or_fatal(self, reader: asyncio.StreamReader) -> bytes | None:
        read_task = asyncio.create_task(self._read_line(reader))
        fatal_task = asyncio.create_task(self._fatal_event.wait())
        tasks = (read_task, fatal_task)
        try:
            done, _ = await asyncio.wait(tasks, return_when=asyncio.FIRST_COMPLETED)
            if fatal_task in done:
                raise ProtocolViolation(
                    self._fatal_error or "connection protocol output failed", reply=False
                )
            return read_task.result()
        finally:
            for task in tasks:
                if not task.done():
                    task.cancel()
            await asyncio.gather(*tasks, return_exceptions=True)

    def _accept_request(self, envelope: Envelope) -> None:
        if envelope.message_id in self._message_ids:
            raise ProtocolViolation(
                f"duplicate integration message ID {envelope.message_id!r}", reply=False
            )
        self._message_ids.add(envelope.message_id)

    def _start_request(self, envelope: Envelope, payload: RequestPayload) -> _Request:
        request = _Request(envelope=envelope)
        self._requests[envelope.message_id] = request
        request.task = asyncio.create_task(self._run_request(request, payload))
        return request

    def _active_requests(self) -> list[_Request]:
        return [request for request in self._requests.values() if not request.terminal]

    async def _abort_requests(self) -> None:
        requests = self._active_requests()
        for request in requests:
            async with request.lock:
                request.closing = True
            if request.task is not None and not request.task.done():
                request.task.cancel()
        tasks = [request.task for request in requests if request.task is not None]
        if tasks:
            await asyncio.gather(*tasks, return_exceptions=True)
        self._requests.clear()

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

    async def _run_request(self, request: _Request, payload: RequestPayload) -> None:
        try:
            result = await self._invoke(payload, request)
            await self._deliver_result(request, result)
        except asyncio.CancelledError:
            async with request.lock:
                request.closing = True
            raise
        except Exception as error:  # noqa: BLE001 - integration failure must become terminal evidence
            if request.envelope.method == "hello":
                self._record_request_failure(request, error)
                return
            if isinstance(error, ProtocolViolation) and not error.reply:
                self._record_request_failure(request, error)
                return
            try:
                await self._deliver_error(request, error)
            except Exception as delivery_error:  # noqa: BLE001 - writer failure is connection-fatal
                self._record_request_failure(request, delivery_error)
        finally:
            if request.terminal:
                self._requests.pop(request.envelope.message_id, None)

    async def _invoke(self, payload: RequestPayload, request: _Request) -> Any:
        if isinstance(payload, HelloRequest):
            name = getattr(self._integration, "name", type(self._integration).__name__)
            return HelloResponse(integration=name)
        if isinstance(payload, ControlRequest):
            if request.envelope.method == "describe":
                return await self._integration.describe()
            if request.envelope.method == "discover":
                return await self._integration.discover()
            if request.envelope.method == "health":
                return await self._integration.health()
            await self._integration.safe_shutdown()
            return SafeShutdownResult()
        if isinstance(payload, ConfigureRequest):
            return await self._integration.configure(payload)
        if isinstance(payload, CancelRequest):
            target = self._requests.get(payload.target_request_id)
            if target is None or target.envelope.method != "execute" or target.terminal:
                raise ProtocolViolation("cancel target is not an active execute request")
            try:
                target_request_id = UUID(payload.target_request_id)
            except ValueError as error:
                raise ProtocolViolation("cancel target_request_id must be a UUID") from error
            await self._integration.cancel(target_request_id)
            return CancelResult()
        if isinstance(payload, ExecuteRequest):
            try:
                request_id = UUID(request.envelope.message_id)
            except ValueError:
                request_id = None
            token = _execute_request_id.set(request_id)
            try:
                return await self._integration.execute(payload, _ServerEventSink(self, request))
            finally:
                _execute_request_id.reset(token)
        raise ProtocolViolation("unsupported request payload")

    async def _deliver_result(self, request: _Request, result: Any) -> None:
        async with request.lock:
            request.closing = True
            try:
                await self._respond(request.envelope, result)
            except Exception:
                request.closing = False
                raise
            request.terminal = True
            request.closing = False

    async def _deliver_error(self, request: _Request, error: Exception) -> None:
        async with request.lock:
            if request.terminal:
                return
            request.closing = True
            try:
                code = "execution_failed"
                retryable = False
                if isinstance(error, ExecuteFailure):
                    code = error.code
                    retryable = error.retryable
                await self._send_error(
                    request.envelope.message_id,
                    request.envelope.method,
                    code,
                    self._error_text(error),
                    retryable=retryable,
                )
            except Exception:
                request.closing = False
                raise
            request.terminal = True
            request.closing = False

    async def _respond(self, request: Envelope, result: Any) -> None:
        if not hasattr(result, "model_dump"):
            raise ProtocolViolation("integration returned an untyped result")
        try:
            result = _RESPONSE_MODELS[request.method].model_validate(result.model_dump(mode="json"))
        except ValidationError as error:
            raise ProtocolViolation("integration returned an invalid result") from error
        await self._send(
            "response",
            request.message_id,
            request.method,
            result.model_dump(mode="json", exclude_none=True),
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
        if method == "hello":
            return False
        await self._send_error(message_id, method, "invalid_request", self._error_text(error))
        return True

    async def _send_error(
        self, request_id: str, method: Method, code: str, message: str, *, retryable: bool = False
    ) -> None:
        if method == "execute":
            payload = ErrorResult(code=code, message=message, retryable=retryable).model_dump(
                mode="json"
            )
        else:
            payload = {"type": method, "status": "error"}
        await self._send("response", request_id, method, payload)

    async def _send(
        self, kind: Kind, request_id: str, method: Method, payload: dict[str, Any]
    ) -> None:
        async with self._write_lock:
            if self._fatal_event.is_set():
                raise ProtocolViolation(
                    self._fatal_error or "connection protocol output failed", reply=False
                )
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
            try:
                written = self._protocol_output.write(encoded)
                if written is not None and written != len(encoded):
                    raise OSError("short protocol output write")
                self._protocol_output.flush()
            except Exception as error:
                self._record_fatal(f"protocol output fault: {self._error_text(error)}")
                raise ProtocolViolation("protocol output write failed", reply=False) from error

    def _record_request_failure(self, request: _Request, error: Exception) -> None:
        request.closing = True
        request.failure = self._error_text(error)
        self._record_fatal(f"terminal response delivery failed: {request.failure}")

    def _record_fatal(self, message: str) -> None:
        if self._fatal_error is None:
            self._fatal_error = message
        self._fatal_event.set()

    @staticmethod
    def _error_text(error: Exception | str) -> str:
        message = str(error).strip() or type(error).__name__
        return message.encode("utf-8", "replace")[:MAX_DIAGNOSTIC_BYTES].decode("utf-8", "replace")

    def _new_message_id(self) -> str:
        message_id = str(uuid4())
        while message_id in self._message_ids:
            message_id = str(uuid4())
        return message_id

    def _diagnose(self, error: Exception | str | None) -> None:
        message = self._error_text(error or "integration protocol failure")
        self._diagnostic_output.write(f"nilo integration protocol error: {message}\n")
        self._diagnostic_output.flush()
