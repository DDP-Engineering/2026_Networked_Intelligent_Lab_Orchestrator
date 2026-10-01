#!/usr/bin/env python3
# -*- coding: utf-8 -*-
# -----------------------------------------------------------------------------
# Copyright (c) 2026 DDP Engineering
# See LICENSE.txt in the repository root for full license details.
# SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
# -----------------------------------------------------------------------------
"""@file workspace.py
@brief Serve bounded Modbus commands through an owner-only Unix socket.
@details The SDK supervisor owns process lifetime; Core later owns public policy.
"""

# IMPORTS
import asyncio
import ipaddress
import json
import os
import re
import stat
from pathlib import Path

from .requests import WRITES, Request, encode
from .runner_client import RunnerClient
from .scheduler import EndpointScheduler
from .usb import TTY_NAME, resolve_tty

# CONSTANTS
SAFE_ID = re.compile(r"[A-Za-z0-9_.:-]{1,128}\Z")
SETTINGS = {
    "response_timeout_ms",
    "connect_timeout_ms",
    "inter_request_ms",
    "read_retries",
    "baud_rate",
    "data_bits",
    "parity",
    "stop_bits",
    "local_echo",
}
HOST_LABEL = re.compile(r"[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\Z")


# HELPER FUNCTIONS
def _strict_json(data: bytes) -> dict:
    def no_duplicates(pairs):
        value = {}
        for key, entry in pairs:
            if key in value:
                raise ValueError("invalid_parameters")
            value[key] = entry
        return value

    value = json.loads(
        data,
        object_pairs_hook=no_duplicates,
        parse_constant=lambda _: (_ for _ in ()).throw(ValueError("invalid_parameters")),
    )
    if not isinstance(value, dict):
        raise ValueError("invalid_parameters")  # noqa: TRY004 - stable private error
    return value


def _settings(raw: object, mode: str) -> dict:
    if not isinstance(raw, dict) or set(raw) - SETTINGS:
        raise ValueError("invalid_parameters")
    for key, low, high in (
        ("response_timeout_ms", 1, 2147483647),
        ("connect_timeout_ms", 1, 2147483647),
        ("inter_request_ms", 0, 2147483647),
        ("read_retries", 0, 2147483647),
    ):
        if type(raw.get(key)) is not int or not low <= raw[key] <= high:
            raise ValueError("invalid_parameters")
    if mode == "rtu":
        if (
            set(raw) != SETTINGS
            or type(raw["baud_rate"]) is not int
            or not 50 <= raw["baud_rate"] <= 4000000
            or type(raw["data_bits"]) is not int
            or raw["data_bits"] != 8
            or raw["parity"] not in {"none", "even", "odd"}
            or type(raw["stop_bits"]) is not int
            or raw["stop_bits"] not in {1, 2}
            or type(raw["local_echo"]) is not bool
        ):
            raise ValueError("invalid_parameters")
        if raw["parity"] != "none" and raw["stop_bits"] != 1:
            raise ValueError("invalid_parameters")
    elif set(raw) != SETTINGS - {"baud_rate", "data_bits", "parity", "stop_bits", "local_echo"}:
        raise ValueError("invalid_parameters")
    return raw


def _network(raw: object, mode: str) -> tuple[str, int]:
    if not isinstance(raw, dict) or set(raw) != {"host", "port", "unit_min", "unit_max"}:
        raise ValueError("invalid_parameters")
    host, port = raw["host"], raw["port"]
    if (
        not isinstance(host, str)
        or not 1 <= len(host) <= 253
        or type(port) is not int
        or not 1 <= port <= 65535
    ):
        raise ValueError("invalid_parameters")
    try:
        address = ipaddress.ip_address(host)
        if isinstance(address, ipaddress.IPv6Address) and address.scope_id is not None:
            raise ValueError("invalid_parameters")
    except ValueError:
        if not all(HOST_LABEL.fullmatch(label) for label in host.split(".")):
            raise ValueError("invalid_parameters") from None
    low, high = raw["unit_min"], raw["unit_max"]
    if (
        type(low) is not int
        or type(high) is not int
        or not 0 <= low <= high <= 255
        or mode == "rtu_tcp"
        and not 1 <= low <= high <= 247
    ):
        raise ValueError("invalid_parameters")
    return host, port


def _request(value: dict, endpoint: dict) -> tuple[str, Request, int, str | None]:
    required = {
        "version",
        "kind",
        "request_id",
        "function",
        "unit",
        "address",
        "count",
        "values",
        "deadline_ms",
    }
    optional = {22: {"and_mask", "or_mask"}, 23: {"read_address", "read_count"}}.get(value.get("function"), set())
    if (
        set(value) not in (required | optional, required | optional | {"poll_key"})
        or value.get("version") != 1
        or value.get("kind") != "transaction"
        or not isinstance(value.get("request_id"), str)
        or not SAFE_ID.fullmatch(value["request_id"])
    ):
        raise ValueError("invalid_parameters")
    if not isinstance(value["values"], list):
        raise ValueError("invalid_parameters")  # noqa: TRY004 - stable private error
    request = Request(
        value["function"], value["unit"], value["address"], value["count"], tuple(value["values"]),
        value.get("and_mask"), value.get("or_mask"), value.get("read_address"), value.get("read_count")
    )
    encode(request, endpoint["transport"])
    if (
        endpoint["transport"] != "rtu"
        and not endpoint["network"]["unit_min"] <= request.unit <= endpoint["network"]["unit_max"]
    ):
        raise ValueError("invalid_parameters")
    poll_key = value.get("poll_key")
    if poll_key is not None and (
        request.function in WRITES
        or not isinstance(poll_key, str)
        or not SAFE_ID.fullmatch(poll_key)
    ):
        raise ValueError("invalid_parameters")
    return value["request_id"], request, value["deadline_ms"], poll_key


# DATA MODELS / TYPEDEFINITIONS
class Workspace:
    """@brief Keep one private channel and one transport alive until supervisor stop."""

    def __init__(self, parameters: object):
        if not isinstance(parameters, dict) or set(parameters) not in (
            {"workspace_directory", "endpoint", "profile"},
            {"workspace_directory", "endpoint", "profile", "tty"},
        ):
            raise ValueError("invalid_parameters")
        root = parameters["workspace_directory"]
        if not isinstance(root, str):
            raise ValueError("invalid_workspace")  # noqa: TRY004 - stable private error
        self.root = Path(root)
        if not self.root.is_absolute() or self.root.is_symlink() or not self.root.is_dir():
            raise ValueError("invalid_workspace")
        metadata = self.root.stat()
        if metadata.st_uid != os.geteuid() or stat.S_IMODE(metadata.st_mode) != 0o700:
            raise ValueError("invalid_workspace")
        endpoint, profile = parameters["endpoint"], parameters["profile"]
        if not isinstance(endpoint, dict) or not isinstance(profile, dict):
            raise ValueError("invalid_parameters")  # noqa: TRY004 - stable private error
        if (
            endpoint.get("schema_version") != 1
            or profile.get("schema_version") != 1
            or endpoint.get("id") != profile.get("endpoint_id")
            or endpoint.get("transport") not in {"tcp", "rtu_tcp", "rtu"}
            or endpoint.get("enabled") is not True
        ):
            raise ValueError("invalid_parameters")
        if not all(
            type(value.get("revision")) is int and value["revision"] >= 1
            for value in (endpoint, profile)
        ):
            raise ValueError("invalid_parameters")
        mode = endpoint["transport"]
        if mode == "rtu":
            tty = parameters.get("tty")
            if (
                endpoint.get("presence") != "present"
                or endpoint.get("readiness") != "ready"
                or not isinstance(endpoint.get("serial_identity"), dict)
                or not isinstance(tty, str)
                or not TTY_NAME.fullmatch(tty)
            ):
                raise ValueError("invalid_parameters")
            self.path = "/dev/" + tty
            self.host, self.port = "127.0.0.1", 0
        else:
            network = endpoint.get("network")
            if parameters.get("tty") is not None:
                raise ValueError("invalid_parameters")
            self.host, self.port = _network(network, mode)
            self.path = None
        self.settings = _settings(profile.get("settings"), mode)
        self.endpoint = endpoint
        self.stop = asyncio.Event()
        self.socket = self.root / "control.sock"
        self.bound = False
        self.client = None
        self.scheduler = None

    async def _handle(self, reader: asyncio.StreamReader, writer: asyncio.StreamWriter) -> None:
        try:
            data = await asyncio.wait_for(reader.readline(), 10)
            if not data or len(data) > 8192 or not data.endswith(b"\n"):
                raise ValueError("invalid_parameters")
            value = _strict_json(data)
            if value == {"version": 1, "kind": "stop"}:
                response = {"outcome": "stopping"}
                self.stop.set()
            else:
                request_id, request, deadline_ms, poll_key = _request(value, self.endpoint)
                response = await self.scheduler.submit(
                    request_id, request, deadline_ms, poll_key=poll_key
                )
            writer.write(
                json.dumps(response, separators=(",", ":"), allow_nan=False).encode() + b"\n"
            )
            await writer.drain()
        except (TimeoutError, ValueError, TypeError, KeyError, asyncio.LimitOverrunError):
            writer.write(b'{"error":"invalid_parameters"}\n')
            await writer.drain()
        except (ConnectionError, OSError):
            pass
        finally:
            writer.close()
            await writer.wait_closed()

    async def run(self) -> None:
        """@brief Open, serve and release the complete private endpoint lifetime."""
        if self.socket.exists() or self.socket.is_symlink():
            raise ValueError("invalid_workspace")
        if self.endpoint["transport"] == "rtu":
            self.path = resolve_tty(
                self.endpoint["serial_identity"], self.path.removeprefix("/dev/")
            )
        serial_verifier = None
        if self.endpoint["transport"] == "rtu":

            def verify_serial() -> bool:
                return (
                    resolve_tty(self.endpoint["serial_identity"], self.path.removeprefix("/dev/"))
                    == self.path
                )

            serial_verifier = verify_serial
        self.client = RunnerClient(
            self.endpoint["transport"],
            host=self.host,
            port=self.port,
            path=self.path,
            settings=self.settings,
            serial_verifier=serial_verifier,
        )
        server = None
        try:
            if not await self.client.connect():
                raise ValueError("device_unavailable")
            if self.endpoint["transport"] == "rtu":
                resolve_tty(self.endpoint["serial_identity"], self.path.removeprefix("/dev/"))
            self.scheduler = EndpointScheduler(
                self.client, self.endpoint["transport"], self.settings
            )
            server = await asyncio.start_unix_server(
                self._handle, path=str(self.socket), limit=8192
            )
            self.bound = True
            self.socket.chmod(0o600)
            stop_task = asyncio.create_task(self.stop.wait())
            try:
                done, _ = await asyncio.wait(
                    {stop_task, self.scheduler.worker}, return_when=asyncio.FIRST_COMPLETED
                )
                if self.scheduler.worker in done:
                    if self.scheduler.worker.cancelled():
                        raise RuntimeError("scheduler_stopped")
                    await self.scheduler.worker
                    raise RuntimeError("scheduler_stopped")
            finally:
                stop_task.cancel()
                await asyncio.gather(stop_task, return_exceptions=True)
        finally:
            if server is not None:
                server.close()
                await server.wait_closed()
            if self.scheduler is not None:
                await self.scheduler.close()
            self.client.close()
            if self.bound:
                self.socket.unlink(missing_ok=True)


# END OF FILE
