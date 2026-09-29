#!/usr/bin/env python3
# -*- coding: utf-8 -*-
# -----------------------------------------------------------------------------
# Copyright (c) 2026 DDP Engineering
# SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
# -----------------------------------------------------------------------------
"""@file test_workspace.py
@brief Prove private Modbus workspace admission refuses unsafe paths.
@details No network or physical adapter is opened by these tests.
"""

import asyncio
import json
from pathlib import Path

import pytest
from peers import peer

from nilo_modbus.workspace import Workspace


def configuration(root: Path, port: int):
    return {
        "workspace_directory": str(root),
        "endpoint": {
            "schema_version": 1,
            "id": "peer",
            "revision": 1,
            "transport": "tcp",
            "enabled": True,
            "network": {"host": "127.0.0.1", "port": port, "unit_min": 0, "unit_max": 255},
        },
        "profile": {
            "schema_version": 1,
            "id": "profile",
            "revision": 1,
            "endpoint_id": "peer",
            "settings": {
                "response_timeout_ms": 500,
                "connect_timeout_ms": 1000,
                "inter_request_ms": 0,
                "read_retries": 0,
            },
        },
    }


def test_workspace_refuses_world_readable_directory(tmp_path: Path):
    root = tmp_path / "shared"
    root.mkdir(mode=0o755)
    root.chmod(0o755)
    with pytest.raises(ValueError, match="invalid_workspace"):
        Workspace({"workspace_directory": str(root), "endpoint": {}, "profile": {}})


async def test_private_socket_transacts_and_cleans_up(tmp_path: Path):
    root = tmp_path / "private"
    root.mkdir(mode=0o700)
    root.chmod(0o700)
    async with peer("tcp") as fixture:
        workspace = Workspace(configuration(root, fixture.port))
        task = asyncio.create_task(workspace.run())
        socket = root / "control.sock"
        for _ in range(200):
            if socket.exists():
                break
            await asyncio.sleep(0.01)
        assert socket.exists()
        assert socket.stat().st_mode & 0o777 == 0o600

        async def send(value):
            reader, writer = await asyncio.open_unix_connection(str(socket))
            writer.write(json.dumps(value).encode() + b"\n")
            await writer.drain()
            response = json.loads(await reader.readline())
            writer.close()
            await writer.wait_closed()
            return response

        try:
            invalid = {"version": 1, "kind": "transaction", "host": "example.org"}
            assert (await send(invalid))["error"] == "invalid_parameters"
            assert fixture.requests == []
            result = await send(
                {
                    "version": 1,
                    "kind": "transaction",
                    "request_id": "read-1",
                    "function": 3,
                    "unit": 1,
                    "address": 0,
                    "count": 2,
                    "values": [],
                    "deadline_ms": 1000,
                }
            )
            assert result["outcome"] == "acknowledged"
            assert result["values"] == [100, 101]
            assert (await send({"version": 1, "kind": "stop"}))["outcome"] == "stopping"
        finally:
            workspace.stop.set()
            await asyncio.wait_for(task, 3)
        assert not socket.exists()


async def test_scheduler_crash_stops_workspace_and_unlinks_socket(tmp_path: Path, monkeypatch):
    root = tmp_path / "private"
    root.mkdir(mode=0o700)
    root.chmod(0o700)

    class BrokenScheduler:
        def __init__(self, *_args):
            async def fail():
                await asyncio.sleep(0.01)
                raise RuntimeError("worker_failed")

            self.worker = asyncio.create_task(fail())

        async def close(self):
            await asyncio.gather(self.worker, return_exceptions=True)

    monkeypatch.setattr("nilo_modbus.workspace.EndpointScheduler", BrokenScheduler)
    async with peer("tcp") as fixture:
        workspace = Workspace(configuration(root, fixture.port))
        with pytest.raises(RuntimeError, match="worker_failed"):
            await asyncio.wait_for(workspace.run(), 1)
        assert not (root / "control.sock").exists()


# END OF FILE


# END OF FILE
