#!/usr/bin/env python3
# -*- coding: utf-8 -*-
# -----------------------------------------------------------------------------
# Copyright (c) 2026 DDP Engineering
# SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
# -----------------------------------------------------------------------------
"""@file workspace.py
@brief Serve bounded serial control over a Core-owned private Unix socket.
@details This endpoint is reachable only by the appliance service account.
"""

from __future__ import annotations

import base64
import binascii
import json
import os
import re
import threading
from http.server import BaseHTTPRequestHandler
from pathlib import Path
from socketserver import ThreadingMixIn, UnixStreamServer
from urllib.parse import parse_qs, urlsplit

from nilo_serial.discovery import resolve_port
from nilo_serial.settings import validate_settings
from nilo_serial.transport import SerialError, SerialTransport


class _Server(ThreadingMixIn, UnixStreamServer):
    daemon_threads = True
    request_queue_size = 16

    def __init__(self, path, handler, workspace):
        self.workspace = workspace
        self.slots = threading.BoundedSemaphore(24)
        super().__init__(path, handler)

    def process_request(self, request, client_address):
        if not self.slots.acquire(blocking=False):
            self.shutdown_request(request)
            return
        try:
            super().process_request(request, client_address)
        except Exception:
            self.slots.release()
            raise

    def process_request_thread(self, request, client_address):
        try:
            super().process_request_thread(request, client_address)
        finally:
            self.slots.release()


class _Handler(BaseHTTPRequestHandler):
    def setup(self):
        self.request.settimeout(12)
        super().setup()

    def log_message(self, *_args):
        """Keep host paths and transmitted bytes out of protocol stderr."""

    def _reply(self, status, value):
        data = json.dumps(value).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        try:
            self.wfile.write(data)
        except (BrokenPipeError, ConnectionResetError):
            pass

    def _request(self):
        workspace = self.server.workspace
        port = workspace.port
        route = urlsplit(self.path)
        if self.command == "GET":
            if route.path == "/status":
                return port.status()
            if route.path == "/read":
                query = parse_qs(route.query, strict_parsing=True)
                if set(query) - {"after", "wait_ms"} or any(len(v) != 1 for v in query.values()):
                    raise SerialError("invalid_parameters")
                return port.read(
                    int(query.get("after", ["0"])[0]), int(query.get("wait_ms", ["0"])[0])
                )
            raise SerialError("invalid_parameters")
        if self.headers.get("Transfer-Encoding"):
            raise SerialError("invalid_parameters")
        length = int(self.headers.get("Content-Length", "0"))
        if not 0 < length <= 8192:
            raise SerialError("invalid_parameters")
        raw = self.rfile.read(length)
        if len(raw) != length:
            raise SerialError("invalid_parameters")
        value = json.loads(raw)
        if not isinstance(value, dict):
            raise SerialError("invalid_parameters")
        if route.path == "/send":
            if set(value) != {"data_base64"} or not isinstance(value["data_base64"], str):
                raise SerialError("invalid_parameters")
            return port.send(base64.b64decode(value["data_base64"], validate=True))
        if route.path == "/signals":
            return port.set_signals(value)
        if route.path == "/stop" and not value:
            workspace.stop.set()
            return {"ok": True}
        raise SerialError("invalid_parameters")

    def do_GET(self):
        try:
            self._reply(200, self._request())
        except SerialError as error:
            code = str(error)
            self._reply(
                409 if code in ("port_busy", "device_unavailable") else 400, {"error": code}
            )
        except (ValueError, TypeError, binascii.Error, OSError):
            self._reply(400, {"error": "invalid_parameters"})

    do_POST = do_GET


class Workspace:
    """@brief Validate physical identity and serve one approved connection."""

    def __init__(self, parameters: object):
        fields = {
            "workspace_directory",
            "hardware_identity",
            "endpoint_id",
            "device_id",
            "settings",
        }
        if not isinstance(parameters, dict) or set(parameters) != fields:
            raise SerialError("invalid_parameters")
        if not isinstance(parameters["device_id"], str) or not re.fullmatch(
            r"serial-[1-9][0-9]*", parameters["device_id"]
        ):
            raise SerialError("invalid_parameters")
        if not isinstance(parameters["workspace_directory"], str):
            raise SerialError("invalid_parameters")
        self.directory = Path(parameters["workspace_directory"])
        if (
            not self.directory.is_absolute()
            or not self.directory.is_dir()
            or self.directory.is_symlink()
        ):
            raise SerialError("invalid_parameters")
        metadata = self.directory.stat()
        if metadata.st_uid != os.geteuid() or metadata.st_mode & 0o077:
            raise SerialError("invalid_parameters")
        self.settings = validate_settings(parameters["settings"])
        self.parameters = parameters
        self.stop = threading.Event()
        self.port = None

    def run(self):
        """@brief Open only a matching USB endpoint, serve until stopped, release it."""
        endpoint = resolve_port(
            self.parameters["hardware_identity"], self.parameters["endpoint_id"]
        )
        self.port = SerialTransport(endpoint, self.settings)
        socket = self.directory / "control.sock"
        server = None
        try:
            if (
                resolve_port(self.parameters["hardware_identity"], self.parameters["endpoint_id"])
                != endpoint
            ):
                raise SerialError("device_unavailable")
            # Bind refuses a preexisting path; never unlink somebody else's socket.
            server = _Server(str(socket), _Handler, self)
            socket.chmod(0o600)
            server.timeout = 0.1
            while not self.stop.is_set() and not self.port.stop.is_set():
                server.handle_request()
        finally:
            self.port.close()
            if server:
                server.server_close()
                socket.unlink(missing_ok=True)


# END OF FILE
