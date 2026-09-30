#!/usr/bin/env python3
# -*- coding: utf-8 -*-
# -----------------------------------------------------------------------------
# Copyright (c) 2026 DDP Engineering
# This software is licensed under the PolyForm Noncommercial License 1.0.0.
# See LICENSE.txt in the repository root for full license details.
# SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
# -----------------------------------------------------------------------------
"""@file test_contracts.py
@brief Exercise the strict versioned contract grammar.
@details Feasibility only; production plugin registration is a later package.
"""

# IMPORTS
import json
from copy import deepcopy
from pathlib import Path

import jsonschema
import pytest

# CONSTANTS
ROOT = Path(__file__).parents[1]
REF = {"id": "definition-1", "revision": 1}
POINT = {
    "id": "p1",
    "name": "Speed",
    "area": "holding",
    "address": 0,
    "type": "uint32",
    "width": 2,
    "order": "normal",
    "unit": "rpm",
    "scale": 1,
    "offset": 0,
    "access": "read",
    "write_min": None,
    "write_max": None,
    "labels": {},
}
SAMPLES = {
    "endpoint": {
        "id": "e1",
        "revision": 1,
        "name": "Peer",
        "transport": "tcp",
        "resource_id": "bus1",
        "enabled": True,
        "presence": "configured",
        "readiness": "offline",
        "network": {"host": "127.0.0.1", "port": 502, "unit_min": 0, "unit_max": 255},
    },
    "profile": {
        "id": "p1",
        "revision": 1,
        "owner_id": "admin1",
        "name": "Profile",
        "endpoint_id": "e1",
        "settings": {
            "response_timeout_ms": 1000,
            "connect_timeout_ms": 3000,
            "inter_request_ms": 0,
            "read_retries": 1,
        },
    },
    "map": {"id": "m1", "revision": 1, "owner_id": "u1", "name": "Map", "points": [POINT]},
    "workspace": {
        "id": "w1",
        "revision": 1,
        "owner_id": "u1",
        "name": "Workspace",
        "endpoint_id": "e1",
        "profile_ref": REF,
        "map_refs": [REF],
        "poll_groups": [],
        "layout": {
            "columns": ["address", "value"],
            "address_base": "zero",
            "series": ["p1"],
            "display_paused": False,
        },
    },
    "session": {
        "id": "s1",
        "endpoint_id": "e1",
        "owner_id": "u1",
        "credential_kind": "session",
        "credential_id": "credential-id",
        "state": "idle",
        "expires_at": "2026-09-29T12:00:00Z",
        "pinned": {"endpoint": REF, "profile": REF, "maps": [REF], "workspace": REF},
        "writes_enabled": True,
        "policy_revision": 1,
    },
    "command": {
        "id": "c1",
        "session_id": "s1",
        "idempotency_key": "request-key",
        "digest": "0" * 64,
        "unit_id": 1,
        "function": 16,
        "address": 0,
        "quantity": 2,
        "values": [123, 456],
        "state": "reserved",
    },
    "recording": {
        "id": "r1",
        "session_id": "s1",
        "owner_id": "u1",
        "name": "Recording",
        "started_at": "2026-09-29T12:00:00Z",
        "ended_at": None,
        "status": "active",
        "tags": [],
        "definition_snapshot": {"endpoint": {}, "profile": {}, "maps": [], "workspace": {}},
        "parts": [],
        "logging_mode": "both",
        "gaps": False,
    },
}

# HELPER FUNCTIONS
for value in SAMPLES.values():
    value["schema_version"] = 1
SAMPLES["recording"]["definition_snapshot"] = {
    "endpoint": deepcopy(SAMPLES["endpoint"]),
    "profile": deepcopy(SAMPLES["profile"]),
    "maps": [deepcopy(SAMPLES["map"])],
    "workspace": deepcopy(SAMPLES["workspace"]),
}


def schema_for(name):
    """@brief Load a named strict schema and retain local definition resolution."""
    path = ROOT / "schemas/contracts.schema.json"
    assert path.is_file(), "contract schemas are not declared"
    schema = json.loads(path.read_text())
    jsonschema.Draft202012Validator.check_schema(schema)
    return {"$schema": schema["$schema"], "$defs": schema["$defs"], "$ref": f"#/$defs/{name}"}


def validate(name, value):
    """@brief Check a sample with format validation enabled."""
    jsonschema.Draft202012Validator(
        schema_for(name), format_checker=jsonschema.FormatChecker()
    ).validate(value)


# PUBLIC API
@pytest.mark.parametrize("name", list(SAMPLES))
def test_contract_entities(name):
    value = deepcopy(SAMPLES[name])
    value["schema_version"] = 1
    validate(name, value)
    value["unrecognized"] = True
    with pytest.raises(jsonschema.ValidationError):
        validate(name, value)


def test_optional_map_point_display_fields():
    """@brief Keep the MB-05 optional point extensions in the strict contract."""
    value = deepcopy(SAMPLES["map"])
    value["points"][0].update(description="Register description", precision=2)
    validate("map", value)
    value["points"][0]["padding"] = "space"
    validate("map", value)
    value["points"][0]["padding"] = "invalid"
    with pytest.raises(jsonschema.ValidationError):
        validate("map", value)


@pytest.mark.parametrize(
    "name,field,value",
    [
        ("endpoint", "transport", "udp"),
        ("endpoint", "revision", True),
        ("session", "expires_at", "tomorrow"),
        ("session", "writes_enabled", "yes"),
        ("command", "function", 22),
        ("command", "address", 65536),
        ("command", "values", [65536]),
        ("profile", "settings", {"shell": "open"}),
    ],
)
def test_invalid_contract_fields(name, field, value):
    record = deepcopy(SAMPLES[name])
    record.update(schema_version=1)
    record[field] = value
    with pytest.raises(jsonschema.ValidationError):
        validate(name, record)


def test_endpoint_transport_identity_is_separate():
    record = deepcopy(SAMPLES["endpoint"])
    record["schema_version"] = 1
    record["transport"] = "rtu"
    with pytest.raises(jsonschema.ValidationError):
        validate("endpoint", record)
    record.pop("network")
    record["serial_identity"] = {
        "stable_id": "usb-stable",
        "vendor_id": "1234",
        "product_id": "abcd",
        "serial_number": "fixture",
        "bus_path": "usb-port-1",
    }
    validate("endpoint", record)


def test_exact_64_bit_and_nonfinite_values():
    for value in [
        {"type": "int64", "value": "9223372036854775807"},
        {"type": "uint64", "value": "18446744073709551615"},
        {"type": "nonfinite", "value": "nan", "quality": "decode_error"},
    ]:
        validate("decoded_value", value)
    for value in [
        {"type": "int64", "value": 9223372036854775807},
        {"type": "uint64", "value": "-1"},
        {"type": "nonfinite", "value": float("nan")},
    ]:
        with pytest.raises(jsonschema.ValidationError):
            validate("decoded_value", value)


def test_private_intent_is_strict_and_has_no_approval_assertion():
    payload = {
        "request_id": "private-1",
        "unit_id": 1,
        "function": 6,
        "address": 0,
        "quantity": 1,
        "values": [123],
        "deadline_ms": 1000,
    }
    message = {"version": 1, "request_id": "private-1", "operation": "transact", "payload": payload}
    validate("private_request", message)
    message["payload"]["approved"] = True
    with pytest.raises(jsonschema.ValidationError):
        validate("private_request", message)
    for operation in ("status", "close", "events"):
        message = {
            "version": 1,
            "request_id": "private-1",
            "operation": operation,
            "payload": {"unknown": True},
        }
        with pytest.raises(jsonschema.ValidationError):
            validate("private_request", message)


def test_schema_root_rejects_untyped_records():
    path = ROOT / "schemas/contracts.schema.json"
    validator = jsonschema.Draft202012Validator(json.loads(path.read_text()))
    assert not validator.is_valid({"untyped": True})
    error = {
        "code": "write_disabled",
        "message": "Plugin writes disabled",
        "request_id": "request-1",
        "retryable": False,
    }
    assert validator.is_valid(error)


@pytest.mark.parametrize(
    "operation,payload",
    [
        ("status", {"state": "idle", "active_request_id": None, "trace_cursor": 0}),
        (
            "transact",
            {
                "outcome": "acknowledged",
                "function": 3,
                "values": [123],
                "exception_code": None,
                "tx_sequence": 1,
                "rx_sequence": 2,
                "elapsed_ms": 1,
            },
        ),
        ("poll", {"state": "polling", "group_id": "group-1"}),
        ("cancel", {"command_id": "command-1", "outcome": "cancelled"}),
        ("events", {"items": [], "next_cursor": 0, "gap": False}),
        ("close", {"state": "closed"}),
    ],
)
def test_each_private_operation_has_a_typed_reply(operation, payload):
    message = {"version": 1, "request_id": "private-1", "operation": operation, "payload": payload}
    validate("private_reply", message)
    message["payload"]["unknown"] = True
    with pytest.raises(jsonschema.ValidationError):
        validate("private_reply", message)
    message.pop("payload")
    message["error"] = {
        "code": "disconnected",
        "message": "Unavailable",
        "request_id": "private-1",
        "retryable": False,
    }
    validate("private_reply", message)
    payload.pop("unknown")
    message["payload"] = payload
    with pytest.raises(jsonschema.ValidationError):
        validate("private_reply", message)


# END OF FILE
