"""Process-level contract checks for an install-shaped NILO integration package."""

from __future__ import annotations

import json
import os
import subprocess
import sys
import tempfile
from datetime import UTC, datetime
from pathlib import Path
from typing import Any


ROOT = Path(__file__).parents[2]
PACKAGE = ROOT / "integrations" / "fake_python"


def request(message_id: str, method: str, payload: dict[str, Any]) -> dict[str, Any]:
    return {
        "protocol": "nilo.integration/1",
        "kind": "request",
        "message_id": message_id,
        "request_id": "",
        "method": method,
        "timestamp": datetime.now(UTC).isoformat().replace("+00:00", "Z"),
        "payload": payload,
    }


def launch(
    executable: str, arguments: list[str], records: list[dict[str, Any]]
) -> subprocess.CompletedProcess[bytes]:
    """Launch the supplied executable plus argument array without a shell."""

    return subprocess.run(
        [executable, *arguments],
        input=b"".join(json.dumps(record, separators=(",", ":")).encode() + b"\n" for record in records),
        cwd=PACKAGE,
        capture_output=True,
        check=False,
        env={**os.environ, "PYTHONUNBUFFERED": "1"},
    )


def decode_records(stdout: bytes) -> list[dict[str, Any]]:
    return [json.loads(line) for line in stdout.splitlines()]


def fake_command() -> tuple[str, list[str]]:
    """Use the installed console entry as an executable with an argument array."""

    executable = Path(sys.executable).with_name(
        "nilo-fake-integration.exe" if os.name == "nt" else "nilo-fake-integration"
    )
    return str(executable), []


def test_manifest_declares_the_expected_policies_and_separate_runner_fields() -> None:
    manifest = json.loads((PACKAGE / "manifest.json").read_text(encoding="utf-8"))

    assert manifest["protocol"] == "nilo.integration/1"
    assert isinstance(manifest["runner"]["executable"], str)
    assert isinstance(manifest["runner"]["arguments"], list)
    assert {operation["id"]: operation["policy"] for operation in manifest["operations"]} == {
        "echo": "auto_eligible",
        "artifact": "auto_eligible",
        "sleep": "full_only",
        "fail": "always_prompt",
        "crash": "always_prompt",
    }
    assert all(operation["resources"] == ["fake/device/stable-001"] for operation in manifest["operations"])


def test_declared_sleep_schema_is_strict() -> None:
    schema = json.loads((PACKAGE / "schemas" / "sleep.input.schema.json").read_text(encoding="utf-8"))

    assert schema["type"] == "object"
    assert schema["additionalProperties"] is False
    assert schema["required"] == ["duration_ms"]
    echo = json.loads((PACKAGE / "schemas" / "echo.input.schema.json").read_text(encoding="utf-8"))
    assert echo["additionalProperties"] is False


def test_runner_hello_describe_health_and_echo_have_strict_ordering() -> None:
    executable, arguments = fake_command()
    completed = launch(
        executable,
        arguments,
        [
            request("hello-1", "hello", {"type": "hello", "client": "contract-kit"}),
            request("describe-1", "describe", {"type": "describe"}),
            request("health-1", "health", {"type": "health"}),
            request(
                "execute-1",
                "execute",
                {
                    "type": "execute",
                    "operation": "echo",
                    "parameters": {"value": {"b": 2, "a": 1}},
                },
            ),
        ],
    )

    messages = decode_records(completed.stdout)
    assert completed.returncode == 0, completed.stderr.decode()
    assert completed.stderr == b""
    assert [(message["kind"], message["method"]) for message in messages] == [
        ("response", "hello"),
        ("response", "describe"),
        ("response", "health"),
        ("event", "execute"),
        ("event", "execute"),
        ("response", "execute"),
    ]
    assert messages[1]["payload"]["status"] == "ok"
    assert messages[-1]["payload"] == {
        "type": "result",
        "status": "completed",
        "data": {"canonical": {"value": {"a": 1, "b": 2}}},
    }
    assert all(set(message) == {"protocol", "kind", "message_id", "request_id", "method", "timestamp", "payload"} for message in messages)


def test_runner_rejects_malformed_request_without_protocol_stdout() -> None:
    malformed = request("hello-1", "hello", {"type": "hello", "client": "contract-kit"})
    malformed["unexpected"] = True

    executable, arguments = fake_command()
    completed = launch(executable, arguments, [malformed])

    assert completed.returncode != 0
    assert completed.stdout == b""
    assert b"protocol error" in completed.stderr


def test_runner_cancel_and_safe_shutdown_have_one_terminal_execute_response() -> None:
    execute_id = "00000000-0000-0000-0000-000000000001"
    executable, arguments = fake_command()
    process = subprocess.Popen(
        [executable, *arguments],
        cwd=PACKAGE,
        stdin=subprocess.PIPE,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        env={**os.environ, "PYTHONUNBUFFERED": "1"},
    )
    assert process.stdin is not None
    assert process.stdout is not None
    assert process.stderr is not None

    def send(record: dict[str, Any]) -> None:
        process.stdin.write(json.dumps(record, separators=(",", ":")).encode() + b"\n")
        process.stdin.flush()

    def receive() -> dict[str, Any]:
        line = process.stdout.readline()
        assert line, process.stderr.read().decode()
        return json.loads(line)

    send(request("hello-1", "hello", {"type": "hello", "client": "contract-kit"}))
    assert receive()["method"] == "hello"
    send(
        request(
            execute_id,
            "execute",
            {"type": "execute", "operation": "sleep", "parameters": {"duration_ms": 60000}},
        )
    )
    assert receive()["payload"]["type"] == "progress"
    send(
        request(
            "cancel-1",
            "cancel",
            {"type": "cancel", "target_request_id": execute_id},
        )
    )

    responses = [receive(), receive(), receive()]
    assert any(message["method"] == "cancel" and message["kind"] == "response" for message in responses)
    execute_responses = [
        message
        for message in responses
        if message["method"] == "execute" and message["kind"] == "response"
    ]
    assert len(execute_responses) == 1
    assert execute_responses[0]["payload"] == {
        "type": "result",
        "status": "cancelled",
        "data": {"duration_ms": 60000},
    }

    send(request("shutdown-1", "safe_shutdown", {"type": "safe_shutdown"}))
    assert receive()["payload"] == {"type": "safe_shutdown", "status": "ok"}
    process.stdin.close()
    assert process.wait(timeout=5) == 0
    assert process.stdout.read() == b""
    assert process.stderr.read() == b""


def test_runner_artifact_stays_in_explicit_staging_and_crash_preserves_progress() -> None:
    with tempfile.TemporaryDirectory() as directory:
        executable, arguments = fake_command()
        completed = launch(
            executable,
            arguments,
            [
                request("hello-1", "hello", {"type": "hello", "client": "contract-kit"}),
                request(
                    "artifact-1",
                    "execute",
                    {"type": "execute", "operation": "artifact", "parameters": {"staging_directory": directory}},
                ),
            ],
        )
        messages = decode_records(completed.stdout)
        artifact = Path(directory) / "fake-artifact.bin"
        assert completed.returncode == 0, completed.stderr.decode()
        assert artifact.read_bytes() == b"nilo fake artifact v1\n"
        assert messages[-2]["payload"]["type"] == "artifact"
        assert messages[-1]["payload"] == {
            "type": "result",
            "status": "completed",
            "data": {"artifact_id": "fake-artifact"},
        }

    executable, arguments = fake_command()
    crash = launch(
        executable,
        arguments,
        [
            request("hello-1", "hello", {"type": "hello", "client": "contract-kit"}),
            request(
                "00000000-0000-0000-0000-000000000001",
                "execute",
                {"type": "execute", "operation": "crash", "parameters": {}},
            ),
        ],
    )
    messages = decode_records(crash.stdout)
    assert crash.returncode == 70
    assert [message["kind"] for message in messages] == ["response", "event"]
    assert messages[-1]["payload"]["message"] == "crash imminent"
