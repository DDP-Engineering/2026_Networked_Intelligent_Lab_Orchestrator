#!/usr/bin/env python3
# -*- coding: utf-8 -*-
# -----------------------------------------------------------------------------
# Copyright (c) 2026 DDP Engineering
# SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
# -----------------------------------------------------------------------------
"""@file    test_worker.py
@brief   Contract tests for the structured Sigrok feasibility worker.
@details Exercises process containment and validates retained native evidence;
these tests do not qualify signal or decoder electrical correctness.
"""

# IMPORTS
from __future__ import annotations

import json
import sys
from pathlib import Path

import pytest
from nilo_sigrok.structured_worker import (
    StructuredWorker,
    WorkerFailure,
    WorkerLimits,
    validate_records,
)

# CONSTANTS
ROOT = Path(__file__).parents[1]
EVIDENCE = ROOT / "evidence"
SCHEMA = "nilo.sigrok.worker/1"


# HELPER FUNCTIONS
def records(*values: dict) -> list[dict]:
    """@brief Add the protocol schema to concise test records."""
    return [{"schema": SCHEMA, **value} for value in values]


def write_fixture(tmp_path: Path, body: str) -> list[str]:
    """@brief Create a deterministic child process fixture.
    @return Python command prefix for the generated fixture.
    """
    path = tmp_path / "fixture.py"
    path.write_text(body, encoding="utf-8")
    return [sys.executable, str(path)]


# TESTS
def test_protocol_accepts_typed_mixed_records_and_explicit_event_availability():
    value = records(
        {"kind": "runtime", "libsigrok": "0.6.0", "libsigrokdecode": "0.6.0"},
        {"kind": "channel", "index": 0, "name": "D0", "type": "logic"},
        {"kind": "channel", "index": 8, "name": "A0", "type": "analog"},
        {
            "kind": "setting",
            "key": 50001,
            "id": "samplerate",
            "native_type": "t",
            "scope": "device",
            "get": True,
            "set": True,
            "list": True,
            "current": {"type": "uint64", "value": "200000"},
        },
        {
            "kind": "packet",
            "packet": "logic",
            "unit_size": 1,
            "byte_count": "4",
            "sample_count": "4",
            "data_hex": "0055aaff",
        },
        {
            "kind": "packet",
            "packet": "analog",
            "sample_count": "2",
            "channels": [8],
            "unit": "volt",
            "values": [0.0, 1.0],
        },
        {"kind": "event", "event": "frame_begin"},
        {"kind": "event", "event": "frame_end"},
        {
            "kind": "summary",
            "records": 7,
            "logic": True,
            "analog": True,
            "trigger": False,
            "frame_begin": True,
            "frame_end": True,
            "truncated": False,
        },
    )
    assert validate_records(value)[-1]["trigger"] is False


@pytest.mark.parametrize("token", ["PWM0", "pwm", "signal_generator", "output"])
def test_protocol_rejects_output_capability_tokens_recursively(token):
    value = records(
        {"kind": "channel", "index": 0, "name": token, "type": "logic"},
        {"kind": "summary", "records": 1},
    )
    with pytest.raises(WorkerFailure, match="output capability"):
        validate_records(value)


async def test_parent_accepts_one_bounded_valid_stream(tmp_path):
    payload = records(
        {"kind": "event", "event": "end"},
        {"kind": "summary", "records": 1, "trigger": False},
    )
    body = "import json\n" + "\n".join(f"print(json.dumps({record!r}))" for record in payload)
    worker = StructuredWorker(write_fixture(tmp_path, body))
    assert (await worker.run([]))[-1]["kind"] == "summary"


@pytest.mark.parametrize(
    ("body", "message"),
    [
        ("print('not-json')\n", "malformed"),
        ("raise SystemExit(7)\n", "exit"),
        ("import os\nos.kill(os.getpid(), 9)\n", "exit"),
    ],
)
async def test_parent_contains_malformed_exit_and_crash(tmp_path, body, message):
    worker = StructuredWorker(write_fixture(tmp_path, body))
    with pytest.raises(WorkerFailure, match=message):
        await worker.run([])


async def test_parent_kills_and_awaits_timeout(tmp_path):
    worker = StructuredWorker(
        write_fixture(tmp_path, "import time\ntime.sleep(60)\n"),
        WorkerLimits(timeout_seconds=0.05),
    )
    with pytest.raises(WorkerFailure, match="timeout"):
        await worker.run([])
    assert worker.process is None


async def test_parent_rejects_stream_budget(tmp_path):
    worker = StructuredWorker(
        write_fixture(tmp_path, "print('x' * 5000)\n"),
        WorkerLimits(max_line_bytes=256, max_output_bytes=512),
    )
    with pytest.raises(WorkerFailure, match="limit"):
        await worker.run([])


def test_native_worker_omits_setters_outputs_and_arbitrary_driver_grammar():
    source = (ROOT / "worker" / "nilo_sigrok_worker.c").read_text(encoding="utf-8")
    assert "#define MAX_RECORDS 200" in source
    assert "#define MAX_DECODER_RECORDS 200" in source
    assert "sr_config_set(" not in source
    assert "sr_output_" not in source
    assert "PWM" not in source
    assert '"demo-capture"' in source
    assert '"uart-fixture"' in source


def test_retained_demo_and_decoder_evidence_matches_coverage_register():
    demo = validate_records(
        [json.loads(line) for line in (EVIDENCE / "demo-capture.jsonl").read_text().splitlines()]
    )
    uart = validate_records(
        [json.loads(line) for line in (EVIDENCE / "uart-fixture.jsonl").read_text().splitlines()]
    )
    coverage = json.loads((EVIDENCE / "coverage.json").read_text(encoding="utf-8"))

    assert {record.get("type") for record in demo if record["kind"] == "channel"} >= {
        "logic",
        "analog",
    }
    assert {record.get("packet") for record in demo if record["kind"] == "packet"} >= {
        "logic",
        "analog",
    }
    assert any(record["kind"] == "setting" for record in demo)
    assert any(record["kind"] == "annotation" for record in uart)
    assert coverage["schema"] == "nilo.sigrok.coverage/1"
    assert coverage["worker_boundary"]["decision"] == "native_child_process"
    assert coverage["qualification"] == "integration_only_not_electrical"
    assert all(item["status"] for item in coverage["features"])
    assert all(item["evidence_level"] for item in coverage["features"])


def test_feasibility_document_records_cli_comparison_versions_and_dependencies():
    text = (
        ROOT.parents[2] / "06_SoftwareDocu" / "Sigrok_Structured_Worker_Feasibility.md"
    ).read_text(encoding="utf-8")
    for phrase in [
        "sigrok-cli bridge",
        "libsigrok 0.6.0",
        "libsigrokdecode 0.6.0",
        "native child process",
        "Python 3.10",
        "experimental",
        "electrical correctness",
        "PWM",
    ]:
        assert phrase in text


# END OF FILE
