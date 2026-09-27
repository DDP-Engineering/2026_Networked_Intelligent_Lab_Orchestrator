#!/usr/bin/env python3
# -*- coding: utf-8 -*-
# -----------------------------------------------------------------------------
# Copyright (c) 2026 DDP Engineering
# SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
# -----------------------------------------------------------------------------
"""@file test_workspace.py
@brief Exercise private Unix HTTP control using a real pseudo-terminal.
@details Only discovery identity resolution is replaced by the PTY fixture.
"""

import base64
import http.client
import json
import os
import socket
import sys
import tempfile
import threading
import time
from pathlib import Path
from unittest.mock import patch

import pytest

if sys.platform == "linux":
    import pty

    from nilo_serial.workspace import Workspace
else:
    pytest.skip("Physical serial workspace requires Linux", allow_module_level=True)


class UnixHTTP(http.client.HTTPConnection):
    def __init__(self, path):
        super().__init__("localhost", timeout=3)
        self.path = str(path)

    def connect(self):
        self.sock = socket.socket(socket.AF_UNIX)
        self.sock.settimeout(self.timeout)
        self.sock.connect(self.path)


def request(path, method, route, data=None):
    connection = UnixHTTP(path)
    try:
        connection.request(
            method,
            route,
            json.dumps(data) if data is not None else None,
            {"Content-Type": "application/json"},
        )
        response = connection.getresponse()
        return response.status, json.loads(response.read())
    finally:
        connection.close()


def test_private_socket_exchange_permissions_validation_and_stop():
    master, slave = pty.openpty()
    try:
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "control.sock"
            workspace = Workspace(
                {
                    "device_id": "serial-1",
                    "endpoint_id": "ttyUSB0",
                    "hardware_identity": {},
                    "settings": {},
                    "workspace_directory": directory,
                }
            )
            with patch("nilo_serial.workspace.resolve_port", return_value=os.ttyname(slave)):
                worker = threading.Thread(target=workspace.run)
                worker.start()
                try:
                    until = time.monotonic() + 3
                    while not path.exists() and time.monotonic() < until:
                        time.sleep(0.01)
                    assert path.stat().st_mode & 0o777 == 0o600
                    assert request(path, "GET", "/status")[1]["connected"]
                    assert request(path, "POST", "/send", {"data_base64": "!"})[0] == 400
                    assert request(path, "GET", "/read?after=-1")[0] == 400
                    payload = base64.b64encode(b"A\x00\xff").decode()
                    status, sent = request(path, "POST", "/send", {"data_base64": payload})
                    assert status == 200 and sent["bytes_written"] == 3
                    assert os.read(master, 3) == b"A\x00\xff"
                    os.write(master, b"received")
                    page = request(path, "GET", "/read?after=1&wait_ms=1000")[1]
                    assert base64.b64decode(page["events"][0]["data_base64"]) == b"received"
                    assert request(path, "POST", "/stop", {}) == (200, {"ok": True})
                finally:
                    workspace.stop.set()
                    worker.join(3)
                assert not worker.is_alive()
                assert not path.exists()
    finally:
        os.close(master)
        os.close(slave)


def test_workspace_refuses_public_or_nonprivate_directory(tmp_path):
    tmp_path.chmod(0o755)
    with pytest.raises(ValueError, match="invalid_parameters"):
        Workspace(
            {
                "device_id": "serial-1",
                "endpoint_id": "ttyUSB0",
                "hardware_identity": {},
                "settings": {},
                "workspace_directory": str(tmp_path),
            }
        )
