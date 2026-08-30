"""Bounded NDJSON server for a single integration-runner process."""

from __future__ import annotations

import asyncio
import contextlib
import json
import sys
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
    Method,
    RequestPayload,
    SafeShutdownResult,
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
    """A malformed runner request that must not reach the integration."""


class _ServerEventSink(EventSink):
    def __init__(self, server: ProtocolServer, request_id: str) -> None:
        self._server = server
        self._request_id = request_id

    async def emit(self, event: EventPayload) -> None:
        await self._server._send(
            "event", self._request_id, "execute", event.model_dump(mode="json")
        )


class ProtocolServer:
    """Serve strict protocol-v1 requests from stdin-style streams.

    Protocol writes use the stdout buffer captured at construction. Integration
    ``print`` calls are redirected to stderr while a handler is active.
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

    async def run(self, reader: asyncio.StreamReader) -> int:
        """Handle requests until clean EOF, returning a process-style status code."""

        while True:
            try:
                line = await self._read_line(reader)
            except ProtocolViolation as error:
                self._diagnose(error)
                return 2
            if line is None:
                return 0
            raw: dict[str, Any] | None = None
            try:
                raw = self._decode_object(line)
                envelope = Envelope.model_validate(raw)
                if envelope.kind != "request":
                    raise ProtocolViolation("runner input must contain request messages")
                payload = parse_request_payload(envelope.method, envelope.payload)
                await self._handle(envelope, payload)
            except (ValidationError, ProtocolViolation, ValueError, TypeError) as error:
                if await self._reply_if_recoverable(raw, error):
                    continue
                self._diagnose(error)
                return 2
            except Exception as error:  # noqa: BLE001 - defensive process boundary
                self._diagnose(f"unexpected server error: {type(error).__name__}")
                return 2

    async def _read_line(self, reader: asyncio.StreamReader) -> bytes | None:
        line = bytearray()
        for _ in range(MAX_LINE_BYTES):
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
            value = json.loads(line, object_pairs_hook=ProtocolServer._no_duplicate_keys)
        except (UnicodeDecodeError, json.JSONDecodeError) as error:
            raise ProtocolViolation(f"invalid JSON: {error.msg}") from error
        if not isinstance(value, dict):
            raise ProtocolViolation("message must be a JSON object")
        return value

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
            await self._call(self._integration.cancel(UUID(payload.target_request_id)))
            await self._respond(envelope, CancelResult())
            return
        if isinstance(payload, ExecuteRequest):
            try:
                result = await self._call(
                    self._integration.execute(payload, _ServerEventSink(self, envelope.message_id))
                )
                await self._respond(envelope, result)
            except Exception as error:  # noqa: BLE001 - integration must not crash its runner
                await self._send_error(
                    envelope.message_id, "execute", "execution_failed", str(error)
                )
            return
        raise ProtocolViolation("unsupported request payload")

    async def _call(self, awaitable: Any) -> Any:
        with contextlib.redirect_stdout(self._diagnostic_output):
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
        if raw is None or raw.get("kind") != "request":
            return False
        message_id = raw.get("message_id")
        method = raw.get("method")
        if not isinstance(message_id, str) or not message_id.strip() or not isinstance(method, str):
            return False
        known_methods = {
            "hello",
            "describe",
            "discover",
            "health",
            "configure",
            "execute",
            "cancel",
            "safe_shutdown",
        }
        if method not in known_methods:
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
        self, kind: str, request_id: str, method: Method, payload: dict[str, Any]
    ) -> None:
        envelope = Envelope(
            protocol="nilo.integration/1",
            kind=kind,  # type: ignore[arg-type]
            message_id=str(uuid4()),
            request_id=request_id,
            method=method,
            timestamp=datetime.now(UTC),
            payload=payload,
        )
        encoded = (
            json.dumps(envelope.model_dump(mode="json"), separators=(",", ":")).encode() + b"\n"
        )
        if len(encoded) > MAX_LINE_BYTES + 1:
            raise ProtocolViolation("generated message exceeds one MiB line limit")
        async with self._write_lock:
            self._protocol_output.write(encoded)
            self._protocol_output.flush()

    def _diagnose(self, error: Exception | str) -> None:
        message = str(error).replace("\n", " ").replace("\r", " ")
        bounded = message.encode("utf-8", "replace")[:MAX_DIAGNOSTIC_BYTES].decode(
            "utf-8", "replace"
        )
        self._diagnostic_output.write(f"nilo integration protocol error: {bounded}\n")
        self._diagnostic_output.flush()
