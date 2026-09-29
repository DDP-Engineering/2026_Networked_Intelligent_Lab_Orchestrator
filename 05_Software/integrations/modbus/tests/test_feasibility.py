#!/usr/bin/env python3
# -*- coding: utf-8 -*-
# -----------------------------------------------------------------------------
# Copyright (c) 2026 DDP Engineering
# This software is licensed under the PolyForm Noncommercial License 1.0.0.
# See LICENSE.txt in the repository root for full license details.
# SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
# -----------------------------------------------------------------------------
"""@file test_feasibility.py
@brief Prove the client on independent loopback and Linux PTY peers.
@details Feasibility only; production plugin registration is a later package.
"""

# IMPORTS
import importlib
import importlib.util
import sys

import pytest
from nilo_modbus.requests import Request
from peers import crc, peer


# HELPER FUNCTIONS
def client_type():
    """@brief Assert the simulation client exists before importing it."""
    assert importlib.util.find_spec("nilo_modbus.feasibility"), "three-mode client unavailable"
    return importlib.import_module("nilo_modbus.feasibility").SimulationClient


def make_client(p, timeout=0.2):
    """@brief Supply fixture-only connection settings and explicit simulation."""
    return client_type()(
        p.mode, host=p.host, port=p.port, path=p.path, timeout=timeout, simulation=True
    )


# PUBLIC API
def test_independent_crc_vector():
    assert crc(bytes.fromhex("01030000000a")).hex() == "c5cd"


@pytest.mark.parametrize("mode", ["tcp", "rtu_tcp", "rtu"])
@pytest.mark.parametrize("fc", [1, 2, 3, 4, 5, 6, 15, 16])
async def test_initial_functions(mode, fc):
    if mode == "rtu" and sys.platform != "linux":
        pytest.skip("real Linux PTY contract")
    count = 9 if fc in (1, 2, 15) else 2 if fc in (3, 4, 16) else 1
    values = (
        tuple(i % 2 == 0 for i in range(count))
        if fc in (5, 15)
        else tuple(100 + i for i in range(count))
        if fc in (6, 16)
        else ()
    )
    async with peer(mode, "split") as p:
        client = make_client(p, 0.5)
        try:
            assert await client.connect()
            result = await client.transact("request-1", Request(fc, 1, 0, count, values))
            assert result["outcome"] == "acknowledged"
            expected = (
                values
                if fc in (5, 6, 15, 16)
                else tuple(i % 2 == 0 for i in range(count))
                if fc in (1, 2)
                else tuple(100 + i for i in range(count))
            )
            assert result["values"] == expected
            assert len(p.requests) == 1
            if mode == "tcp" and fc == 3:
                assert p.requests[0].hex() == "000100000006010300000002"
        finally:
            client.close()


@pytest.mark.parametrize("mode", ["tcp", "rtu_tcp"])
async def test_coalesced_duplicate_response(mode):
    async with peer(mode, "duplicate") as p:
        client = make_client(p)
        try:
            assert await client.connect()
            result = await client.transact("coalesced", Request(3, 1, 0, 2))
            assert result["outcome"] == "acknowledged"
            assert result["values"] == (100, 101)
            assert len(p.requests) == 1
        finally:
            client.close()


@pytest.mark.parametrize(
    "mode,fault,fc",
    [
        ("tcp", "count", 3),
        ("tcp", "function", 3),
        ("tcp", "unit", 3),
        ("tcp", "pid", 3),
        ("tcp", "tid", 3),
        ("tcp", "length", 3),
        ("tcp", "echo", 6),
        ("rtu_tcp", "crc", 3),
        ("rtu_tcp", "unit", 3),
        ("rtu_tcp", "function", 3),
        ("rtu_tcp", "count", 3),
        ("rtu_tcp", "echo", 6),
        ("rtu", "crc", 3),
        ("rtu", "count", 3),
        ("rtu", "echo", 6),
    ],
)
async def test_inconsistent_reply(mode, fault, fc):
    if mode == "rtu" and sys.platform != "linux":
        pytest.skip("Linux PTY response validation")
    async with peer(mode, fault) as p:
        client = make_client(p, 0.1)
        try:
            assert await client.connect()
            request = Request(fc, 1, 0, 2) if fc == 3 else Request(fc, 1, 0, 1, (123,))
            result = await client.transact("invalid-reply", request)
            assert result["outcome"] == ("delivery_unknown" if fc == 6 else "protocol_error")
            assert result["values"] == ()
            assert client.fenced
            assert not client.client.connected
        finally:
            client.close()


@pytest.mark.parametrize("mode", ["tcp", "rtu_tcp", "rtu"])
async def test_device_exception(mode):
    if mode == "rtu" and sys.platform != "linux":
        pytest.skip("Linux PTY exception")
    async with peer(mode, "exception") as p:
        client = make_client(p)
        try:
            assert await client.connect()
            result = await client.transact("exception-reply", Request(3, 1, 0, 2))
            assert result == {"outcome": "exception", "values": (), "exception_code": 2}
        finally:
            client.close()


@pytest.mark.parametrize("mode", ["tcp", "rtu_tcp", "rtu"])
@pytest.mark.parametrize("fc", [5, 6, 15, 16])
async def test_lost_writes_never_replay(mode, fc):
    if mode == "rtu" and sys.platform != "linux":
        pytest.skip("Linux PTY write loss")
    count = 2 if fc in (15, 16) else 1
    values = tuple([True if fc in (5, 15) else 123] * count)
    async with peer(mode, "drop") as p:
        client = make_client(p, 0.05)
        try:
            assert await client.connect()
            assert client.client.ctx.retries == 0
            result = await client.transact("lost-write", Request(fc, 1, 0, count, values))
            assert result["outcome"] == "delivery_unknown"
            assert client.fenced
            assert not await client.connect()
            again = await client.transact("another-write", Request(fc, 1, 0, count, values))
            assert again["outcome"] == "disconnected"
            await __import__("asyncio").sleep(0.08)
            assert len(p.requests) == 1
        finally:
            client.close()


@pytest.mark.parametrize("fc", [3, 6])
async def test_cancellation_after_tx(fc):
    import asyncio

    async with peer("tcp", "drop") as p:
        client = make_client(p, 1)
        try:
            assert await client.connect()
            request = Request(fc, 1, 0, 1, (123,)) if fc == 6 else Request(fc, 1, 0, 2)
            task = asyncio.create_task(client.transact("cancelled-request", request))
            await asyncio.wait_for(p.received.wait(), 1)
            task.cancel()
            result = await asyncio.wait_for(task, 0.2)
            assert result["outcome"] == ("delivery_unknown" if fc == 6 else "cancelled")
            assert not client.client.connected
            assert len(p.requests) == 1
            assert any(
                e["direction"] == "tx" and e["request_id"] == "cancelled-request"
                for e in client.events
            )
        finally:
            client.close()


@pytest.mark.parametrize("mode", ["tcp", "rtu_tcp", "rtu"])
async def test_late_reply_cannot_be_fresh(mode):
    import asyncio

    if mode == "rtu" and sys.platform != "linux":
        pytest.skip("Linux PTY late response")
    async with peer(mode, "late") as p:
        client = make_client(p, 0.05)
        try:
            assert await client.connect()
            first = await client.transact("expired-read", Request(3, 1, 0, 2))
            assert first["outcome"] == "timeout"
            await asyncio.sleep(0.3)
            second = await client.transact("fresh-read", Request(3, 1, 0, 2))
            assert second["outcome"] == "disconnected"
            assert len(p.requests) == 1
        finally:
            client.close()


async def test_trace_is_bounded_and_correlated():
    async with peer("tcp") as p:
        client = make_client(p, 0.05)
        try:
            assert await client.connect()
            for index in range(140):
                result = await client.transact(f"bounded-{index}", Request(3, 1, 0, 2))
                if result["outcome"] != "acknowledged":
                    break
            assert client.fenced
            assert len(client.events) <= 256
            assert sum(len(e["data_hex"]) // 2 for e in client.events) <= 65536
            assert all(e["request_id"].startswith("bounded-") for e in client.events)
            assert [e["sequence"] for e in client.events] == list(range(1, len(client.events) + 1))
        finally:
            client.close()


@pytest.mark.skipif(sys.platform != "linux", reason="Linux descriptor exclusivity")
async def test_serial_exclusive_owner_and_release():
    import serial

    async with peer("rtu") as p:
        client = make_client(p)
        try:
            assert await client.connect()
            with pytest.raises(serial.SerialException):
                serial.Serial(p.path, exclusive=True)
        finally:
            client.close()
        contender = serial.Serial(p.path, exclusive=True)
        contender.close()


@pytest.mark.parametrize(
    "settings",
    [
        {"mode": "tcp", "host": "192.168.1.15", "port": 502, "simulation": True},
        {"mode": "tcp", "host": "127.0.0.1", "port": 502},
        {"mode": "rtu", "path": "/dev/ttyUSB0", "simulation": True},
        {"mode": "udp", "host": "127.0.0.1", "port": 502, "simulation": True},
    ],
)
def test_real_targets_are_refused(settings):
    with pytest.raises(ValueError):
        client_type()(**settings)


async def test_library_encoding_is_checked_before_transmission():
    async with peer("tcp") as p:
        client = make_client(p, 0.05)
        try:
            assert await client.connect()

            def corrupt(sending, pdu):
                """@brief Simulate a library encoder producing a different address."""
                if sending:
                    pdu.address = 1
                return pdu

            client.client.ctx.trace_pdu = corrupt
            result = await client.transact("canonical-write", Request(6, 1, 0, 1, (123,)))
            assert result["outcome"] == "rejected"
            assert not p.requests
            assert client.fenced
        finally:
            client.close()


# END OF FILE
