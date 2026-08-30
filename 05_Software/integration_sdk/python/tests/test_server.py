import asyncio
import io
import json

import pytest

from nilo_integration.models import (
    ConfigureResult,
    DescribeResult,
    DiscoverResult,
    ExecuteResult,
    HealthResult,
    Progress,
    parse_event_payload,
)
from nilo_integration.server import MAX_LINE_BYTES, ProtocolServer, ProtocolViolation


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


class BlockingIntegration(PrintingIntegration):
    name = "blocking-test"

    def __init__(self) -> None:
        self.started = asyncio.Event()
        self.released = asyncio.Event()
        self.cancelled = asyncio.Event()
        self.execute_cancelled = asyncio.Event()

    async def execute(self, request, events):  # type: ignore[no-untyped-def]
        self.started.set()
        try:
            await self.released.wait()
        except asyncio.CancelledError:
            self.execute_cancelled.set()
            raise
        return ExecuteResult(status="completed", data={"operation": request.operation})

    async def cancel(self, request_id):  # type: ignore[no-untyped-def]
        self.cancelled.set()
        self.released.set()


class LateEventIntegration(PrintingIntegration):
    def __init__(self) -> None:
        self.completed = asyncio.Event()
        self.events = None

    async def execute(self, request, events):  # type: ignore[no-untyped-def]
        self.events = events
        self.completed.set()
        return ExecuteResult(status="completed", data={})


class BackgroundPrintingIntegration(PrintingIntegration):
    def __init__(self) -> None:
        self.completed = asyncio.Event()
        self.release_background_print = asyncio.Event()

    async def execute(self, request, events):  # type: ignore[no-untyped-def]
        asyncio.create_task(self._print_later())
        self.completed.set()
        return ExecuteResult(status="completed", data={})

    async def _print_later(self) -> None:
        await self.release_background_print.wait()
        print("background integration diagnostic")


class AllMethodsIntegration(PrintingIntegration):
    async def discover(self) -> DiscoverResult:
        return DiscoverResult(status="ok")

    async def health(self) -> HealthResult:
        return HealthResult(status="ok")

    async def configure(self, request) -> ConfigureResult:  # type: ignore[no-untyped-def]
        assert request.configuration == {"enabled": True}
        return ConfigureResult(status="ok")

    async def safe_shutdown(self) -> None:
        return None


class EmptyErrorIntegration(PrintingIntegration):
    async def execute(self, request, events):  # type: ignore[no-untyped-def]
        raise RuntimeError()


class OversizedResult:
    def model_dump(self, *, mode: str) -> dict[str, object]:
        assert mode == "json"
        return {"type": "result", "status": "completed", "data": {"padding": "x" * MAX_LINE_BYTES}}


class OversizedResultIntegration(PrintingIntegration):
    async def execute(self, request, events):  # type: ignore[no-untyped-def]
        return OversizedResult()


class FailSecondWrite(io.BytesIO):
    def __init__(self) -> None:
        super().__init__()
        self.writes = 0

    def write(self, data: bytes) -> int:
        self.writes += 1
        if self.writes >= 2:
            raise OSError("simulated protocol pipe loss")
        return super().write(data)


class TransientSecondWrite(io.BytesIO):
    def __init__(self) -> None:
        super().__init__()
        self.writes = 0

    def write(self, data: bytes) -> int:
        self.writes += 1
        if self.writes == 2:
            raise OSError("transient protocol pipe fault")
        return super().write(data)


class ShortSecondWrite(io.BytesIO):
    def __init__(self) -> None:
        super().__init__()
        self.writes = 0

    def write(self, data: bytes) -> int:
        self.writes += 1
        if self.writes == 2:
            partial = data[:-1]
            super().write(partial)
            return len(partial)
        return super().write(data)


class ResultIntegration(PrintingIntegration):
    async def execute(self, request, events):  # type: ignore[no-untyped-def]
        return ExecuteResult(status="completed", data={})


class CatchesEventWriteFailureIntegration(PrintingIntegration):
    async def execute(self, request, events):  # type: ignore[no-untyped-def]
        try:
            await events.emit(Progress(fraction=0.5, message="write this"))
        except ProtocolViolation:
            pass
        return ExecuteResult(status="completed", data={})


class BlockingDescribeIntegration(PrintingIntegration):
    def __init__(self) -> None:
        self.started = asyncio.Event()
        self.release = asyncio.Event()
        self.cancelled = asyncio.Event()

    async def describe(self) -> DescribeResult:
        self.started.set()
        try:
            await self.release.wait()
        except asyncio.CancelledError:
            self.cancelled.set()
            raise
        return DescribeResult(status="ok")


class InvalidHelloIntegration(PrintingIntegration):
    name = " "


class TrackingReadServer(ProtocolServer):
    def __init__(self) -> None:
        super().__init__(PrintingIntegration(), protocol_output=io.BytesIO())
        self.started = asyncio.Event()
        self.released = asyncio.Event()
        self.cancelled = asyncio.Event()

    async def _read_line(self, reader):  # type: ignore[no-untyped-def]
        self.started.set()
        try:
            await self.released.wait()
        except asyncio.CancelledError:
            self.cancelled.set()
            raise


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
    reader.feed_data(request("hello-1", "hello", {"type": "hello", "client": "nilo"}))
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
        ("response", "hello"),
        ("event", "progress"),
        ("response", "result"),
    ]
    assert (
        sum(
            message["kind"] == "response" and message["method"] == "execute" for message in messages
        )
        == 1
    )


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


@pytest.mark.asyncio
async def test_execute_does_not_block_correlated_cancel() -> None:
    reader = asyncio.StreamReader()
    integration = BlockingIntegration()
    execute_id = "00000000-0000-0000-0000-000000000001"
    reader.feed_data(request("hello-1", "hello", {"type": "hello", "client": "nilo"}))
    reader.feed_data(
        request(execute_id, "execute", {"type": "execute", "operation": "wait", "parameters": {}})
    )
    server_task = asyncio.create_task(
        ProtocolServer(integration, protocol_output=io.BytesIO()).run(reader)
    )
    await integration.started.wait()
    reader.feed_data(
        request(
            "cancel-1",
            "cancel",
            {"type": "cancel", "target_request_id": execute_id},
        )
    )

    try:
        await asyncio.wait_for(integration.cancelled.wait(), timeout=0.1)
    finally:
        integration.released.set()
        reader.feed_eof()
        await server_task


@pytest.mark.asyncio
async def test_eof_with_running_execute_is_not_success() -> None:
    reader = asyncio.StreamReader()
    integration = BlockingIntegration()
    reader.feed_data(request("hello-1", "hello", {"type": "hello", "client": "nilo"}))
    reader.feed_data(
        request(
            "00000000-0000-0000-0000-000000000001",
            "execute",
            {"type": "execute", "operation": "wait", "parameters": {}},
        )
    )
    server_task = asyncio.create_task(
        ProtocolServer(integration, protocol_output=io.BytesIO()).run(reader)
    )
    await integration.started.wait()
    reader.feed_eof()

    try:
        assert await asyncio.wait_for(server_task, timeout=0.1) != 0
    finally:
        integration.released.set()
        if not server_task.done():
            await server_task


@pytest.mark.asyncio
async def test_server_requires_hello_before_operational_request() -> None:
    reader = asyncio.StreamReader()
    reader.feed_data(request("describe-1", "describe", {"type": "describe"}))
    reader.feed_eof()
    protocol_output = io.BytesIO()

    exit_code = await ProtocolServer(PrintingIntegration(), protocol_output=protocol_output).run(
        reader
    )

    assert exit_code == 0
    messages = [json.loads(line) for line in protocol_output.getvalue().splitlines()]
    assert messages[0]["payload"]["status"] == "error"


@pytest.mark.asyncio
async def test_server_rejects_duplicate_connection_message_id() -> None:
    reader = asyncio.StreamReader()
    reader.feed_data(request("hello-1", "hello", {"type": "hello", "client": "nilo"}))
    reader.feed_data(request("hello-1", "hello", {"type": "hello", "client": "nilo"}))
    reader.feed_eof()

    exit_code = await ProtocolServer(PrintingIntegration(), protocol_output=io.BytesIO()).run(
        reader
    )

    assert exit_code != 0


@pytest.mark.asyncio
async def test_framing_counts_the_newline_in_the_one_mib_limit() -> None:
    server = ProtocolServer(PrintingIntegration(), protocol_output=io.BytesIO())
    exact_reader = asyncio.StreamReader()
    exact_reader.feed_data(b"x" * (MAX_LINE_BYTES - 1) + b"\n")
    assert len(await server._read_line(exact_reader)) == MAX_LINE_BYTES - 1

    oversized_reader = asyncio.StreamReader()
    oversized_reader.feed_data(b"x" * MAX_LINE_BYTES + b"\n")
    with pytest.raises(ProtocolViolation, match="one MiB"):
        await server._read_line(oversized_reader)


@pytest.mark.asyncio
async def test_server_rejects_non_standard_json_constants() -> None:
    with pytest.raises(ProtocolViolation, match="non-standard JSON constant"):
        ProtocolServer._decode_object(b'{"value":NaN}')


@pytest.mark.asyncio
async def test_server_rejects_non_standard_json_constants_on_output() -> None:
    server = ProtocolServer(PrintingIntegration(), protocol_output=io.BytesIO())

    with pytest.raises(ProtocolViolation, match="JSON"):
        await server._send("response", "request-1", "execute", {"value": float("nan")})


def test_event_payload_parser_rejects_fields_not_in_the_protocol() -> None:
    with pytest.raises(ValueError, match="Extra inputs are not permitted"):
        parse_event_payload(
            {"type": "progress", "fraction": 0.5, "message": "working", "unexpected": True}
        )


@pytest.mark.asyncio
async def test_event_sink_rejects_events_after_execute_terminal_response() -> None:
    reader = asyncio.StreamReader()
    integration = LateEventIntegration()
    reader.feed_data(request("hello-1", "hello", {"type": "hello", "client": "nilo"}))
    reader.feed_data(
        request("execute-1", "execute", {"type": "execute", "operation": "echo", "parameters": {}})
    )
    server_task = asyncio.create_task(
        ProtocolServer(integration, protocol_output=io.BytesIO()).run(reader)
    )
    await integration.completed.wait()

    try:
        assert integration.events is not None
        with pytest.raises(ProtocolViolation, match="after terminal"):
            await integration.events.emit(Progress(fraction=1, message="late"))
    finally:
        reader.feed_eof()
        await server_task


@pytest.mark.asyncio
async def test_background_diagnostics_stay_off_protocol_stdout() -> None:
    reader = asyncio.StreamReader()
    integration = BackgroundPrintingIntegration()
    diagnostics = io.StringIO()
    reader.feed_data(request("hello-1", "hello", {"type": "hello", "client": "nilo"}))
    reader.feed_data(
        request("execute-1", "execute", {"type": "execute", "operation": "echo", "parameters": {}})
    )
    server_task = asyncio.create_task(
        ProtocolServer(
            integration, protocol_output=io.BytesIO(), diagnostic_output=diagnostics
        ).run(reader)
    )
    await integration.completed.wait()
    integration.release_background_print.set()
    await asyncio.sleep(0)

    try:
        assert "background integration diagnostic" in diagnostics.getvalue()
    finally:
        reader.feed_eof()
        await server_task


@pytest.mark.asyncio
async def test_server_handles_each_non_execute_lifecycle_method_after_hello() -> None:
    reader = asyncio.StreamReader()
    reader.feed_data(request("hello-1", "hello", {"type": "hello", "client": "nilo"}))
    reader.feed_data(request("discover-1", "discover", {"type": "discover"}))
    reader.feed_data(request("health-1", "health", {"type": "health"}))
    reader.feed_data(
        request(
            "configure-1",
            "configure",
            {"type": "configure", "configuration": {"enabled": True}},
        )
    )
    reader.feed_data(request("shutdown-1", "safe_shutdown", {"type": "safe_shutdown"}))
    reader.feed_eof()
    protocol_output = io.BytesIO()

    exit_code = await ProtocolServer(AllMethodsIntegration(), protocol_output=protocol_output).run(
        reader
    )

    assert exit_code == 0
    assert [
        message["method"] for message in map(json.loads, protocol_output.getvalue().splitlines())
    ] == [
        "hello",
        "discover",
        "health",
        "configure",
        "safe_shutdown",
    ]


@pytest.mark.asyncio
async def test_output_framing_accepts_exact_one_mib_and_rejects_plus_one() -> None:
    base_output = io.BytesIO()
    await ProtocolServer(PrintingIntegration(), protocol_output=base_output)._send(
        "response", "request-1", "execute", {"padding": ""}
    )
    padding_length = MAX_LINE_BYTES - len(base_output.getvalue())
    exact_server = ProtocolServer(PrintingIntegration(), protocol_output=io.BytesIO())

    await exact_server._send("response", "request-1", "execute", {"padding": "x" * padding_length})
    assert len(exact_server._protocol_output.getvalue()) == MAX_LINE_BYTES

    with pytest.raises(ProtocolViolation, match="one MiB"):
        await ProtocolServer(PrintingIntegration(), protocol_output=io.BytesIO())._send(
            "response", "request-1", "execute", {"padding": "x" * (padding_length + 1)}
        )


@pytest.mark.asyncio
async def test_oversized_execute_result_gets_one_terminal_error() -> None:
    reader = asyncio.StreamReader()
    reader.feed_data(request("hello-1", "hello", {"type": "hello", "client": "nilo"}))
    reader.feed_data(
        request("execute-1", "execute", {"type": "execute", "operation": "large", "parameters": {}})
    )
    reader.feed_eof()
    output = io.BytesIO()

    exit_code = await ProtocolServer(OversizedResultIntegration(), protocol_output=output).run(
        reader
    )

    messages = [json.loads(line) for line in output.getvalue().splitlines()]
    assert exit_code == 0
    assert [(message["method"], message["payload"]["type"]) for message in messages] == [
        ("hello", "hello"),
        ("execute", "error"),
    ]


@pytest.mark.asyncio
async def test_empty_execute_exception_gets_one_terminal_error() -> None:
    reader = asyncio.StreamReader()
    reader.feed_data(request("hello-1", "hello", {"type": "hello", "client": "nilo"}))
    reader.feed_data(
        request("execute-1", "execute", {"type": "execute", "operation": "fail", "parameters": {}})
    )
    reader.feed_eof()
    output = io.BytesIO()

    exit_code = await ProtocolServer(EmptyErrorIntegration(), protocol_output=output).run(reader)

    messages = [json.loads(line) for line in output.getvalue().splitlines()]
    assert exit_code == 0
    assert messages[-1]["method"] == "execute"
    assert messages[-1]["payload"]["type"] == "error"
    assert messages[-1]["payload"]["message"] == "RuntimeError"


@pytest.mark.asyncio
async def test_terminal_write_loss_exits_nonzero_without_waiting_for_eof() -> None:
    reader = asyncio.StreamReader()
    reader.feed_data(request("hello-1", "hello", {"type": "hello", "client": "nilo"}))
    reader.feed_data(
        request("execute-1", "execute", {"type": "execute", "operation": "write", "parameters": {}})
    )
    output = FailSecondWrite()
    server_task = asyncio.create_task(
        ProtocolServer(ResultIntegration(), protocol_output=output).run(reader)
    )

    assert await asyncio.wait_for(server_task, timeout=0.2) != 0


@pytest.mark.asyncio
async def test_eof_with_running_non_execute_request_is_not_success() -> None:
    reader = asyncio.StreamReader()
    integration = BlockingDescribeIntegration()
    reader.feed_data(request("hello-1", "hello", {"type": "hello", "client": "nilo"}))
    reader.feed_data(request("describe-1", "describe", {"type": "describe"}))
    server_task = asyncio.create_task(
        ProtocolServer(integration, protocol_output=io.BytesIO()).run(reader)
    )
    await integration.started.wait()
    reader.feed_eof()

    assert await asyncio.wait_for(server_task, timeout=0.2) != 0
    assert integration.cancelled.is_set()


@pytest.mark.asyncio
async def test_cancelling_server_cancels_and_awaits_running_requests() -> None:
    reader = asyncio.StreamReader()
    integration = BlockingIntegration()
    reader.feed_data(request("hello-1", "hello", {"type": "hello", "client": "nilo"}))
    reader.feed_data(
        request(
            "00000000-0000-0000-0000-000000000001",
            "execute",
            {"type": "execute", "operation": "wait", "parameters": {}},
        )
    )
    server_task = asyncio.create_task(
        ProtocolServer(integration, protocol_output=io.BytesIO()).run(reader)
    )
    await integration.started.wait()
    server_task.cancel()

    with pytest.raises(asyncio.CancelledError):
        await server_task
    assert integration.execute_cancelled.is_set()


@pytest.mark.asyncio
@pytest.mark.parametrize("output_type", [TransientSecondWrite, ShortSecondWrite])
async def test_terminal_write_fault_is_connection_fatal(output_type) -> None:  # type: ignore[no-untyped-def]
    reader = asyncio.StreamReader()
    reader.feed_data(request("hello-1", "hello", {"type": "hello", "client": "nilo"}))
    reader.feed_data(
        request("execute-1", "execute", {"type": "execute", "operation": "write", "parameters": {}})
    )
    server_task = asyncio.create_task(
        ProtocolServer(ResultIntegration(), protocol_output=output_type()).run(reader)
    )

    assert await asyncio.wait_for(server_task, timeout=0.2) != 0


@pytest.mark.asyncio
async def test_caught_event_write_fault_still_terminates_connection() -> None:
    reader = asyncio.StreamReader()
    reader.feed_data(request("hello-1", "hello", {"type": "hello", "client": "nilo"}))
    reader.feed_data(
        request("execute-1", "execute", {"type": "execute", "operation": "event", "parameters": {}})
    )
    server_task = asyncio.create_task(
        ProtocolServer(
            CatchesEventWriteFailureIntegration(), protocol_output=TransientSecondWrite()
        ).run(reader)
    )

    assert await asyncio.wait_for(server_task, timeout=0.2) != 0


@pytest.mark.asyncio
async def test_invalid_hello_terminates_without_unlocking_operational_methods() -> None:
    reader = asyncio.StreamReader()
    reader.feed_data(request("hello-1", "hello", {"type": "hello", "client": "nilo"}))
    reader.feed_data(request("describe-1", "describe", {"type": "describe"}))
    reader.feed_eof()

    exit_code = await ProtocolServer(InvalidHelloIntegration(), protocol_output=io.BytesIO()).run(
        reader
    )

    assert exit_code != 0


@pytest.mark.asyncio
async def test_cancelling_run_cancels_and_awaits_internal_reader_tasks() -> None:
    server = TrackingReadServer()
    runner = asyncio.create_task(server.run(asyncio.StreamReader()))
    await server.started.wait()
    runner.cancel()

    try:
        with pytest.raises(asyncio.CancelledError):
            await runner
    finally:
        server.released.set()
        await asyncio.sleep(0)

    assert server.cancelled.is_set()


@pytest.mark.asyncio
async def test_malformed_hello_is_connection_fatal_not_a_control_error() -> None:
    reader = asyncio.StreamReader()
    reader.feed_data(
        request("hello-1", "hello", {"type": "hello", "client": "nilo", "unexpected": True})
    )
    reader.feed_eof()
    protocol_output = io.BytesIO()
    diagnostics = io.StringIO()

    exit_code = await ProtocolServer(
        PrintingIntegration(), protocol_output=protocol_output, diagnostic_output=diagnostics
    ).run(reader)

    assert exit_code != 0
    assert protocol_output.getvalue() == b""
    assert "protocol error" in diagnostics.getvalue()
