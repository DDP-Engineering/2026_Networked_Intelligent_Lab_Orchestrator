#!/usr/bin/env python3
# -*- coding: utf-8 -*-
# -----------------------------------------------------------------------------
# Copyright (c) 2026 DDP Engineering
# SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
# -----------------------------------------------------------------------------
"""@file test_supervised.py
@brief Exercise the real SDK child process and private Modbus control channel.
@details Loopback peers are handwritten; no physical or public Core operation runs.
"""

import asyncio
import json
import sys
from datetime import UTC, datetime

import pytest
from peers import peer
from test_workspace import configuration


async def message(process, method, message_id, payload):
    """@brief Send one SDK envelope to the child without blocking fixture peers."""
    envelope = {
        "protocol": "nilo.integration/1",
        "kind": "request",
        "message_id": message_id,
        "request_id": "",
        "method": method,
        "timestamp": datetime.now(UTC).isoformat(),
        "payload": payload,
    }
    process.stdin.write(json.dumps(envelope).encode() + b"\n")
    await process.stdin.drain()


async def reply(process):
    return json.loads(await asyncio.wait_for(process.stdout.readline(), 12))


@pytest.mark.skipif(sys.platform != "linux", reason="private Unix runner")
@pytest.mark.parametrize("ending", ["socket", "cancel"])
async def test_sdk_supervised_runner_private_transaction_and_stop(tmp_path, ending):
    root = tmp_path / "workspace"
    root.mkdir(mode=0o700)
    root.chmod(0o700)
    async with peer("tcp") as fixture:
        process = await asyncio.create_subprocess_exec(
            sys.executable,
            "-m",
            "nilo_modbus",
            "--stdio",
            stdin=asyncio.subprocess.PIPE,
            stdout=asyncio.subprocess.PIPE,
            stderr=asyncio.subprocess.PIPE,
        )
        try:
            await message(process, "hello", "hello", {"type": "hello", "client": "test"})
            assert (await reply(process))["payload"]["integration"] == "modbus"
            await message(process, "describe", "describe", {"type": "describe"})
            assert (await reply(process))["payload"]["operations"] == ["serve"]
            execute_id = "00000000-0000-0000-0000-000000000001"
            await message(
                process,
                "execute",
                execute_id,
                {
                    "type": "execute",
                    "operation": "serve",
                    "parameters": configuration(root, fixture.port),
                },
            )
            socket = root / "control.sock"
            for _ in range(300):
                if socket.exists():
                    break
                if process.returncode is not None:
                    break
                await asyncio.sleep(0.01)
            assert socket.exists()
            assert socket.stat().st_mode & 0o777 == 0o600
            reader, writer = await asyncio.open_unix_connection(str(socket))
            writer.write(
                json.dumps(
                    {
                        "version": 1,
                        "kind": "transaction",
                        "request_id": "r1",
                        "function": 3,
                        "unit": 1,
                        "address": 0,
                        "count": 2,
                        "values": [],
                        "deadline_ms": 1000,
                    }
                ).encode()
                + b"\n"
            )
            await writer.drain()
            result = json.loads(await asyncio.wait_for(reader.readline(), 2))
            assert result["outcome"] == "acknowledged"
            writer.close()
            await writer.wait_closed()
            if ending == "socket":
                reader, writer = await asyncio.open_unix_connection(str(socket))
                writer.write(b'{"version":1,"kind":"stop"}\n')
                await writer.drain()
                assert json.loads(await reader.readline())["outcome"] == "stopping"
                writer.close()
                await writer.wait_closed()
                assert (await reply(process))["payload"]["status"] == "completed"
            else:
                await message(
                    process,
                    "cancel",
                    "cancel",
                    {"type": "cancel", "target_request_id": execute_id},
                )
                replies = [await reply(process), await reply(process)]
                assert {item["method"] for item in replies} == {"cancel", "execute"}
                assert (
                    next(item for item in replies if item["method"] == "execute")["payload"][
                        "status"
                    ]
                    == "completed"
                )
            assert not socket.exists()
            await message(process, "safe_shutdown", "shutdown", {"type": "safe_shutdown"})
            await reply(process)
            process.stdin.close()
            assert await asyncio.wait_for(process.wait(), 3) == 0
        finally:
            if process.returncode is None:
                process.kill()
                await process.wait()


@pytest.mark.skipif(sys.platform != "linux", reason="private Unix runner")
async def test_killed_runner_leaves_no_connectable_private_socket(tmp_path):
    root = tmp_path / "workspace"
    root.mkdir(mode=0o700)
    root.chmod(0o700)
    async with peer("tcp") as fixture:
        process = await asyncio.create_subprocess_exec(
            sys.executable,
            "-m",
            "nilo_modbus",
            "--stdio",
            stdin=asyncio.subprocess.PIPE,
            stdout=asyncio.subprocess.PIPE,
            stderr=asyncio.subprocess.PIPE,
        )
        try:
            await message(process, "hello", "hello", {"type": "hello", "client": "test"})
            await reply(process)
            await message(
                process,
                "execute",
                "00000000-0000-0000-0000-000000000001",
                {
                    "type": "execute",
                    "operation": "serve",
                    "parameters": configuration(root, fixture.port),
                },
            )
            socket = root / "control.sock"
            for _ in range(300):
                if socket.exists():
                    break
                await asyncio.sleep(0.01)
            assert socket.exists()
            process.kill()
            assert await asyncio.wait_for(process.wait(), 3) != 0
            with pytest.raises((ConnectionRefusedError, FileNotFoundError)):
                await asyncio.open_unix_connection(str(socket))
        finally:
            if process.returncode is None:
                process.kill()
                await process.wait()


# END OF FILE
