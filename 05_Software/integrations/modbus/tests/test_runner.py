#!/usr/bin/env python3
# -*- coding: utf-8 -*-
# -----------------------------------------------------------------------------
# Copyright (c) 2026 DDP Engineering
# SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
# -----------------------------------------------------------------------------
"""@file test_runner.py
@brief Exercise production-configured Modbus clients against independent peers.
@details Loopback and Linux PTY responses are handwritten outside PyModbus.
"""

import asyncio
import os
import sys
import time
from unittest.mock import Mock

import pytest
from peers import peer

from nilo_modbus.requests import Request
from nilo_modbus.runner_client import RunnerClient
from nilo_modbus.scheduler import EndpointScheduler


def settings(mode):
    value = {
        "response_timeout_ms": 500,
        "connect_timeout_ms": 1000,
        "inter_request_ms": 0,
        "read_retries": 0,
    }
    if mode == "rtu":
        value.update(baud_rate=19200, data_bits=8, parity="none", stop_bits=1, local_echo=False)
    return value


@pytest.mark.parametrize("timeout_ms", [1, 20000])
async def test_runner_accepts_core_canonical_response_timeout(timeout_ms):
    profile = settings("tcp")
    profile["response_timeout_ms"] = timeout_ms
    client = RunnerClient("tcp", port=502, settings=profile)
    try:
        assert client.timeout == timeout_ms / 1000
    finally:
        client.close()


async def test_runner_caps_extreme_connect_timeout_for_cancel():
    profile = settings("tcp")
    profile["connect_timeout_ms"] = 2147483647
    client = RunnerClient("tcp", port=502, settings=profile)
    try:
        assert client.connect_timeout <= 60
    finally:
        client.close()


async def test_serial_recovery_drains_until_quiet():
    if sys.platform != "linux":
        pytest.skip("Linux PTY contract")
    import pty

    master, slave = pty.openpty()
    client = RunnerClient("rtu", path=os.ttyname(slave), settings=settings("rtu"))
    client.quiet_seconds = 0.02
    try:

        async def late_bytes():
            await asyncio.sleep(0.01)
            os.write(master, b"late")

        task = asyncio.create_task(late_bytes())
        start = time.monotonic()
        assert await client._drain_serial(start + 0.2)
        assert time.monotonic() - start >= 0.025
        await task
    finally:
        client.close()
        os.close(master)
        os.close(slave)


async def test_serial_recovery_refuses_changed_pinned_identity():
    if sys.platform != "linux":
        pytest.skip("Linux PTY contract")
    import pty

    master, slave = pty.openpty()
    verifier = Mock(return_value=False)
    try:
        client = RunnerClient(
            "rtu",
            path=os.ttyname(slave),
            settings=settings("rtu"),
            serial_verifier=verifier,
        )
        try:
            assert not await client.recover(time.monotonic() + 0.2)
            verifier.assert_called_once()
        finally:
            client.close()
    finally:
        os.close(master)
        os.close(slave)


@pytest.mark.parametrize("mode", ["tcp", "rtu_tcp", "rtu"])
@pytest.mark.parametrize("function", [1, 2, 3, 4, 5, 6, 15, 16])
async def test_supervised_client_initial_functions(mode, function):
    if mode == "rtu" and sys.platform != "linux":
        pytest.skip("Linux PTY contract")
    count = 9 if function in (1, 2, 15) else 2 if function in (3, 4, 16) else 1
    values = (
        tuple(i % 2 == 0 for i in range(count))
        if function in (5, 15)
        else tuple(100 + i for i in range(count))
        if function in (6, 16)
        else ()
    )
    async with peer(mode, "split") as fixture:
        client = RunnerClient(
            mode, host=fixture.host, port=fixture.port, path=fixture.path, settings=settings(mode)
        )
        assert await client.connect()
        scheduler = EndpointScheduler(client, mode, settings(mode))
        try:
            result = await scheduler.submit("initial", Request(function, 1, 0, count, values), 2000)
            assert result["outcome"] == "acknowledged"
            assert result["attempts"] == 1
            assert len(fixture.requests) == 1
        finally:
            await scheduler.close()


@pytest.mark.parametrize(
    "mode,fault", [("tcp", "tid"), ("tcp", "length"), ("rtu_tcp", "crc"), ("rtu", "crc")]
)
async def test_runner_rejects_bad_reply_and_never_publishes_values(mode, fault):
    if mode == "rtu" and sys.platform != "linux":
        pytest.skip("Linux PTY contract")
    async with peer(mode, fault) as fixture:
        client = RunnerClient(
            mode, host=fixture.host, port=fixture.port, path=fixture.path, settings=settings(mode)
        )
        assert await client.connect()
        scheduler = EndpointScheduler(client, mode, settings(mode))
        try:
            result = await scheduler.submit("bad", Request(3, 1, 0, 2), 1000)
            assert result["outcome"] == "protocol_error"
            assert result["values"] == ()
            assert len(fixture.requests) == 1
        finally:
            await scheduler.close()


@pytest.mark.parametrize("mode", ["tcp", "rtu_tcp", "rtu"])
async def test_runner_never_retries_uncertain_write(mode):
    if mode == "rtu" and sys.platform != "linux":
        pytest.skip("Linux PTY contract")
    config = settings(mode)
    config["response_timeout_ms"] = 50
    config["read_retries"] = 3
    async with peer(mode, "drop") as fixture:
        client = RunnerClient(
            mode, host=fixture.host, port=fixture.port, path=fixture.path, settings=config
        )
        assert await client.connect()
        scheduler = EndpointScheduler(client, mode, config)
        try:
            result = await scheduler.submit("write", Request(6, 1, 0, 1, (123,)), 500)
            assert result["outcome"] == "delivery_unknown"
            assert result["attempts"] == 1
            assert len(fixture.requests) == 1
        finally:
            await scheduler.close()


# END OF FILE
