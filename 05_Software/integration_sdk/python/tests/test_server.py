import asyncio
import io
import json

import pytest

from nilo_integration.models import DescribeResult, ExecuteResult, Progress
from nilo_integration.server import ProtocolServer


class PrintingIntegration:
    name = "printing-test"

    async def describe(self) -> DescribeResult:
        print("integration diagnostic")
        return DescribeResult(status="ok")

    async def discover(self):  # type: ignore[no-untyped-def]
        raise AssertionError("not used")

    async def health(self):  # type: ignore[no-untyped-def]
        raise AssertionError("not used")

    async def configure(self, request):  # type: ignore[no-untyped-def]
        raise AssertionError("not used")

    async def execute(self, request, events):  # type: ignore[no-untyped-def]
        raise AssertionError("not used")

    async def cancel(self, request_id):  # type: ignore[no-untyped-def]
        raise AssertionError("not used")

    async def safe_shutdown(self) -> None:
        raise AssertionError("not used")


class ExecutingIntegration(PrintingIntegration):
    async def execute(self, request, events):  # type: ignore[no-untyped-def]
        await events.emit(Progress(fraction=0.5, message="working"))
        return ExecuteResult(status="completed", data={"operation": request.operation})


def request(message_id: str, method: str, payload: dict[str, object]) -> bytes:
    return (
        json.dumps(
            {
                "protocol": "nilo.integration/1",
                "kind": "request",
                "message_id": message_id,
                "request_id": "",
                "method": method,
                "timestamp": "2026-08-30T10:00:00Z",
                "payload": payload,
            },
            separators=(",", ":"),
        ).encode()
        + b"\n"
    )


@pytest.mark.asyncio
async def test_server_answers_hello_and_describe_without_diagnostic_stdout() -> None:
    reader = asyncio.StreamReader()
    reader.feed_data(request("hello-1", "hello", {"type": "hello", "client": "nilo"}))
    reader.feed_data(request("describe-1", "describe", {"type": "describe"}))
    reader.feed_eof()
    protocol_output = io.BytesIO()
    diagnostics = io.StringIO()

    exit_code = await ProtocolServer(
        PrintingIntegration(), protocol_output=protocol_output, diagnostic_output=diagnostics
    ).run(reader)

    assert exit_code == 0
    messages = [json.loads(line) for line in protocol_output.getvalue().splitlines()]
    assert [(message["request_id"], message["method"]) for message in messages] == [
        ("hello-1", "hello"),
        ("describe-1", "describe"),
    ]
    assert "integration diagnostic" in diagnostics.getvalue()
    assert all(
        "integration diagnostic" not in line.decode()
        for line in protocol_output.getvalue().splitlines()
    )


@pytest.mark.asyncio
async def test_server_returns_one_terminal_error_for_invalid_execute() -> None:
    reader = asyncio.StreamReader()
    reader.feed_data(
        request(
            "execute-1",
            "execute",
            {"type": "execute", "operation": "", "parameters": {}},
        )
    )
    reader.feed_eof()
    protocol_output = io.BytesIO()

    exit_code = await ProtocolServer(PrintingIntegration(), protocol_output=protocol_output).run(
        reader
    )

    assert exit_code == 0
    messages = [json.loads(line) for line in protocol_output.getvalue().splitlines()]
    assert len(messages) == 1
    assert messages[0]["kind"] == "response"
    assert messages[0]["request_id"] == "execute-1"
    assert messages[0]["payload"]["type"] == "error"


@pytest.mark.asyncio
async def test_server_emits_events_then_one_execute_terminal_response() -> None:
    reader = asyncio.StreamReader()
    reader.feed_data(
        request(
            "execute-1",
            "execute",
            {"type": "execute", "operation": "echo", "parameters": {}},
        )
    )
    reader.feed_eof()
    protocol_output = io.BytesIO()

    exit_code = await ProtocolServer(ExecutingIntegration(), protocol_output=protocol_output).run(
        reader
    )

    assert exit_code == 0
    messages = [json.loads(line) for line in protocol_output.getvalue().splitlines()]
    assert [(message["kind"], message["payload"]["type"]) for message in messages] == [
        ("event", "progress"),
        ("response", "result"),
    ]
    assert sum(message["kind"] == "response" for message in messages) == 1


@pytest.mark.asyncio
async def test_server_rejects_line_larger_than_one_mib() -> None:
    reader = asyncio.StreamReader()
    reader.feed_data(b"x" * (1024 * 1024 + 1) + b"\n")
    reader.feed_eof()
    diagnostics = io.StringIO()

    exit_code = await ProtocolServer(
        PrintingIntegration(), protocol_output=io.BytesIO(), diagnostic_output=diagnostics
    ).run(reader)

    assert exit_code != 0
    assert "one MiB" in diagnostics.getvalue()
