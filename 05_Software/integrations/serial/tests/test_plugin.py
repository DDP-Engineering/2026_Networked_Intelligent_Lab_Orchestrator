#!/usr/bin/env python3
# -*- coding: utf-8 -*-
# -----------------------------------------------------------------------------
# Copyright (c) 2026 DDP Engineering
# SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
# -----------------------------------------------------------------------------
"""@file test_plugin.py
@brief Verify serial SDK configuration, discovery and protocol handshake.
@details Configuration never opens a serial descriptor.
"""

import json
import subprocess
import sys
from datetime import UTC, datetime
from unittest.mock import patch

import pytest
from nilo_integration import ExecuteFailure
from nilo_integration.models import ConfigureRequest, ExecuteRequest

if sys.platform == "linux":
    from nilo_serial.plugin import SerialIntegration
else:
    pytest.skip("Supervised physical serial runner requires Linux", allow_module_level=True)


async def test_configure_and_discovery_do_not_open_ports():
    plugin = SerialIntegration()
    config = {
        "schema_version": 1,
        "plugin": {"revision": 1, "settings": {}},
        "device": {"id": "serial-1", "revision": 1, "settings": {}},
    }
    with patch("serial.Serial", side_effect=AssertionError("must not open")):
        assert (
            await plugin.configure(ConfigureRequest(type="configure", configuration=config))
        ).status == "ok"
        result = await plugin.execute(
            ExecuteRequest(type="execute", operation="discover_devices", parameters={}), None
        )
        assert result.status == "completed"
        assert isinstance(result.data["devices"], list)
        with pytest.raises(ExecuteFailure):
            await plugin.execute(
                ExecuteRequest(type="execute", operation="connect", parameters={}), None
            )


def test_supervised_process_hello_and_describe():
    process = subprocess.Popen(
        [sys.executable, "-m", "nilo_serial", "--stdio"],
        stdin=subprocess.PIPE,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
    )
    try:

        def call(method, payload):
            envelope = {
                "protocol": "nilo.integration/1",
                "kind": "request",
                "message_id": method,
                "request_id": "",
                "method": method,
                "timestamp": datetime.now(UTC).isoformat(),
                "payload": payload,
            }
            process.stdin.write(json.dumps(envelope) + "\n")
            process.stdin.flush()
            return json.loads(process.stdout.readline())

        assert (
            call("hello", {"type": "hello", "client": "test"})["payload"]["integration"] == "serial"
        )
        assert call("describe", {"type": "describe"})["payload"]["operations"] == [
            "discover_devices",
            "connect",
            "stream",
        ]
        call("safe_shutdown", {"type": "safe_shutdown"})
        process.stdin.close()
        assert process.wait(timeout=3) == 0
    finally:
        if process.poll() is None:
            process.kill()
            process.wait()
