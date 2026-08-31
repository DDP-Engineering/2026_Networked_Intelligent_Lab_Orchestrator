"""Process-level contract checks for an install-shaped NILO integration package."""

from __future__ import annotations

import json
import os
import platform
import shutil
import subprocess
import sys
import tempfile
from datetime import UTC, datetime
from pathlib import Path
from typing import Any

from jsonschema import Draft202012Validator

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


def manifest(package: Path = PACKAGE) -> dict[str, Any]:
    return json.loads((package / "manifest.json").read_text(encoding="utf-8"))


def validate_schema(relative_path: str, value: object) -> None:
    Draft202012Validator(json.loads((PACKAGE / relative_path).read_text(encoding="utf-8"))).validate(value)


def fake_command(package: Path = PACKAGE) -> tuple[str, list[str]]:
    """Resolve and launch the manifest's installed runner without substitution."""

    package_manifest = manifest(package)
    runner = select_host_runner(package_manifest)
    executable = package / runner["executable"]
    assert executable.is_file()
    arguments = runner["arguments"]
    assert isinstance(arguments, list) and arguments
    return str(executable), arguments


def select_host_runner(
    package_manifest: dict[str, Any], operating_system: str | None = None, architecture: str | None = None
) -> dict[str, Any]:
    architecture = architecture or {"AMD64": "amd64", "x86_64": "amd64", "aarch64": "arm64"}.get(
        platform.machine(), platform.machine().lower()
    )
    operating_system = operating_system or ("windows" if os.name == "nt" else "linux")
    for override in package_manifest["runner"].get("platform_overrides", []):
        if override["os"] == operating_system and override["architecture"] == architecture:
            return override
    return package_manifest["runner"]


def test_manifest_declares_the_expected_policies_and_separate_runner_fields() -> None:
    package_manifest = manifest()

    assert package_manifest["protocol"] == "nilo.integration/1"
    assert isinstance(package_manifest["runner"]["executable"], str)
    assert isinstance(package_manifest["runner"]["arguments"], list) and package_manifest["runner"]["arguments"]
    assert {operation["id"]: operation["policy"] for operation in package_manifest["operations"]} == {
        "echo": "auto_eligible",
        "artifact": "auto_eligible",
        "sleep": "full_only",
        "fail": "always_prompt",
        "crash": "always_prompt",
    }
    assert all(operation["resources"] == ["fake/device/stable-001"] for operation in package_manifest["operations"])
    assert {artifact["id"] for artifact in package_manifest["artifacts"]} == {"fake_artifact"}
    selected = select_host_runner(package_manifest)
    assert selected["arguments"] == ["--stdio"]
    assert selected["executable"] != ""
    assert (
        select_host_runner(package_manifest, "linux", "arm64")["executable"]
        == ".venv-linux-arm64/bin/nilo-fake-integration"
    )
    assert (
        select_host_runner(package_manifest, "linux", "amd64")["executable"]
        == ".venv-linux-x86_64/bin/nilo-fake-integration"
    )
    assert (
        select_host_runner(package_manifest, "windows", "amd64")["executable"]
        == ".venv-windows-x86_64/Scripts/nilo-fake-integration.exe"
    )
    assert select_host_runner({"runner": package_manifest["runner"]}, "plan9", "amd64") == package_manifest[
        "runner"
    ]


def test_declared_sleep_schema_is_strict() -> None:
    schema = json.loads((PACKAGE / "schemas" / "sleep.input.schema.json").read_text(encoding="utf-8"))

    assert schema["type"] == "object"
    assert schema["additionalProperties"] is False
    assert schema["required"] == ["duration_ms"]
    echo = json.loads((PACKAGE / "schemas" / "echo.input.schema.json").read_text(encoding="utf-8"))
    assert echo["additionalProperties"] is False


def test_manifest_operation_schemas_are_strict_and_validate_canonical_samples() -> None:
    operations = {operation["id"]: operation for operation in manifest()["operations"]}
    samples = {
        "echo": {"value": {"a": 1}},
        "artifact": {"staging_directory": "staging"},
        "sleep": {"duration_ms": 1},
        "fail": {"code": "fake_selected", "message": "planned failure"},
        "crash": {},
    }
    for operation_id, sample in samples.items():
        schema = json.loads((PACKAGE / operations[operation_id]["input_schema"]).read_text(encoding="utf-8"))
        assert schema["additionalProperties"] is False
        Draft202012Validator(schema).validate(sample)


def test_clean_package_install_launches_the_host_manifest_command() -> None:
    with tempfile.TemporaryDirectory() as directory:
        root = Path(directory)
        clean_package = root / "integrations" / "fake_python"
        clean_sdk = root / "integration_sdk" / "python"
        ignored_generated = shutil.ignore_patterns(".venv", ".venv-*", "__pycache__")
        shutil.copytree(PACKAGE, clean_package, ignore=ignored_generated)
        shutil.copytree(ROOT / "integration_sdk" / "python", clean_sdk, ignore=ignored_generated)
        installed = subprocess.run(
            ["uv", "sync", "--all-groups", "--locked"], cwd=clean_package, capture_output=True
        )
        assert installed.returncode == 0, installed.stderr.decode()
        executable, arguments = fake_command(clean_package)
        completed = subprocess.run(
            [executable, *arguments],
            input=(
                json.dumps(
                    request("hello-1", "hello", {"type": "hello", "client": "contract-kit"}),
                    separators=(",", ":"),
                ).encode()
                + b"\n"
            ),
            cwd=clean_package,
            capture_output=True,
            check=False,
        )
        assert completed.returncode == 0, completed.stderr.decode()
        assert decode_records(completed.stdout)[0]["method"] == "hello"


def test_runner_hello_describe_health_and_echo_have_strict_ordering() -> None:
    package_manifest = manifest()
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
    assert messages[0]["payload"]["integration"] == package_manifest["package"]["id"]
    assert messages[1]["payload"]["integration"] == package_manifest["package"]["id"]
    assert messages[1]["payload"]["operations"] == [
        operation["id"] for operation in package_manifest["operations"]
    ]
    assert messages[-1]["payload"] == {
        "type": "result",
        "status": "completed",
        "data": {"canonical": {"value": {"a": 1, "b": 2}}},
    }
    assert all(set(message) == {"protocol", "kind", "message_id", "request_id", "method", "timestamp", "payload"} for message in messages)
    validate_schema("schemas/echo.input.schema.json", {"value": {"b": 2, "a": 1}})
    validate_schema("schemas/echo.result.schema.json", messages[-1]["payload"]["data"])
    for message in messages[3:5]:
        validate_schema("schemas/progress.event.schema.json", message["payload"])


def test_runner_rejects_malformed_request_without_protocol_stdout() -> None:
    malformed = request("hello-1", "hello", {"type": "hello", "client": "contract-kit"})
    malformed["unexpected"] = True

    executable, arguments = fake_command()
    completed = launch(executable, arguments, [malformed])

    assert completed.returncode != 0
    assert completed.stdout == b""
    assert b"protocol error" in completed.stderr


def test_runner_preserves_selected_fake_failure_code() -> None:
    executable, arguments = fake_command()
    completed = launch(
        executable,
        arguments,
        [
            request("hello-1", "hello", {"type": "hello", "client": "contract-kit"}),
            request(
                "00000000-0000-0000-0000-000000000003",
                "execute",
                {
                    "type": "execute",
                    "operation": "fail",
                    "parameters": {"code": "fake_selected", "message": "planned failure"},
                },
            ),
        ],
    )

    assert completed.returncode == 0, completed.stderr.decode()
    assert decode_records(completed.stdout)[-1]["payload"] == {
        "type": "error",
        "code": "fake_selected",
        "message": "planned failure",
        "retryable": False,
    }


def test_runner_crash_and_malformed_request_exit_with_open_stdin() -> None:
    for record in (
        request(
            "hello-1",
            "hello",
            {"type": "hello", "client": "contract-kit", "unexpected": True},
        ),
        request("hello-1", "hello", {"type": "hello", "client": "contract-kit"}),
    ):
        executable, arguments = fake_command()
        process = subprocess.Popen(
            [executable, *arguments], cwd=PACKAGE, stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE
        )
        assert process.stdin is not None
        process.stdin.write(json.dumps(record, separators=(",", ":")).encode() + b"\n")
        if record["payload"].get("unexpected") is None:
            process.stdin.write(
                json.dumps(
                    request(
                        "00000000-0000-0000-0000-000000000004",
                        "execute",
                        {"type": "execute", "operation": "crash", "parameters": {}},
                    ),
                    separators=(",", ":"),
                ).encode()
                + b"\n"
            )
        process.stdin.flush()
        assert process.wait(timeout=3) != 0


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
    send(request("health-after-shutdown", "health", {"type": "health"}))
    assert receive()["payload"] == {"type": "health", "status": "stopped"}
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
        validate_schema("schemas/artifact.event.schema.json", messages[-2]["payload"])
        validate_schema("schemas/artifact.result.schema.json", messages[-1]["payload"]["data"])
        assert messages[-1]["payload"] == {
            "type": "result",
            "status": "completed",
            "data": {"artifact_id": "fake_artifact"},
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
