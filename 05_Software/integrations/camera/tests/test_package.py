"""Package contract for the camera integration."""

import json
import os
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).parents[1]


def test_camera_manifest_declares_phase_one_operations():
    manifest = json.loads((ROOT / "manifest.json").read_text(encoding="utf-8"))
    assert manifest["package"]["id"] == "camera"
    assert {item["id"] for item in manifest["operations"]} == {
        "discover_devices",
        "snapshot",
        "get_controls",
        "set_controls",
        "reset_controls",
        "workspace",
        "stream",
    }
    assert manifest["artifacts"] == [
        {
            "id": "image",
            "schema": "schemas/event.schema.json",
            "media_types": ["image/jpeg", "image/png"],
        }
    ]
    for operation in manifest["operations"]:
        schema = json.loads((ROOT / operation["input_schema"]).read_text(encoding="utf-8"))
        assert schema["additionalProperties"] is False


def test_camera_runner_answers_protocol_hello_and_describe():
    messages = [
        {
            "protocol": "nilo.integration/1",
            "kind": "request",
            "message_id": f"request-{method}",
            "request_id": "",
            "method": method,
            "timestamp": "2026-09-24T00:00:00Z",
            "payload": payload,
        }
        for method, payload in (
            ("hello", {"type": "hello", "client": "nilo"}),
            ("describe", {"type": "describe"}),
        )
    ]
    environment = os.environ.copy()
    environment["PYTHONPATH"] = str(ROOT / "src")
    process = subprocess.run(
        [sys.executable, "-m", "nilo_camera", "--stdio"],
        input="".join(json.dumps(item) + "\n" for item in messages),
        text=True,
        capture_output=True,
        env=environment,
        timeout=5,
        check=False,
    )
    assert process.returncode == 0, process.stderr
    replies = [json.loads(line) for line in process.stdout.splitlines()]
    assert [(item["kind"], item["method"]) for item in replies] == [
        ("response", "hello"),
        ("response", "describe"),
    ]
    assert replies[1]["payload"]["operations"] == [
        "discover_devices",
        "snapshot",
        "get_controls",
        "set_controls",
        "reset_controls",
        "workspace",
        "stream",
    ]
