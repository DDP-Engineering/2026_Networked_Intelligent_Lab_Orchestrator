#!/usr/bin/env python3
# -*- coding: utf-8 -*-
# -----------------------------------------------------------------------------
# Copyright (c) 2026 DDP Engineering
# SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
# -----------------------------------------------------------------------------
"""@file test_scheduler.py
@brief Prove one-client fair scheduling before the runner is implemented.
@details Synthetic transport isolates queue ordering and cancellation.
"""

import asyncio

import pytest

from nilo_modbus.requests import Request
from nilo_modbus.scheduler import EndpointScheduler


class FakeClient:
    """@brief Block the first transaction so waiting-unit order is observable."""

    def __init__(self):
        self.started = asyncio.Event()
        self.release = asyncio.Event()
        self.calls = []
        self.active = 0
        self.maximum = 0
        self.fenced = False

    async def transact(self, request_id, request):
        self.active += 1
        self.maximum = max(self.maximum, self.active)
        self.calls.append((request_id, request.unit))
        if len(self.calls) == 1:
            self.started.set()
            await self.release.wait()
        self.active -= 1
        return {"outcome": "acknowledged", "values": (100,), "attempts": 1}

    def close(self):
        self.fenced = True


async def test_round_robin_units_and_single_in_flight():
    client = FakeClient()
    scheduler = EndpointScheduler(client, "tcp", {"read_retries": 0, "inter_request_ms": 0})
    first = asyncio.create_task(scheduler.submit("first", Request(3, 1, 0), 1000))
    await client.started.wait()
    more_one = asyncio.create_task(scheduler.submit("unit-one", Request(3, 1, 1), 1000))
    other = asyncio.create_task(scheduler.submit("unit-two", Request(3, 2, 0), 1000))
    await asyncio.sleep(0)
    client.release.set()
    assert (await first)["outcome"] == "acknowledged"
    assert (await other)["outcome"] == "acknowledged"
    assert (await more_one)["outcome"] == "acknowledged"
    assert [unit for _, unit in client.calls] == [1, 2, 1]
    assert client.maximum == 1
    await scheduler.close()


async def test_transaction_carries_exact_trace_frames():
    class TracedClient:
        fenced = False

        def __init__(self):
            self.events = []

        async def transact(self, request_id, _request):
            self.events = [
                {"request_id": request_id, "direction": "tx", "data_hex": "000102"},
                {"request_id": request_id, "direction": "rx", "data_hex": "010203"},
            ]
            return {"outcome": "acknowledged", "values": (100,)}

        def close(self):
            self.fenced = True

    scheduler = EndpointScheduler(TracedClient(), "tcp", {"read_retries": 0, "inter_request_ms": 0})
    result = await scheduler.submit("trace-one", Request(3, 1, 0), 1000)
    assert [frame["data_hex"] for frame in result["frames"]] == ["000102", "010203"]
    await scheduler.close()


async def test_pending_poll_coalesces_and_queue_is_bounded():
    client = FakeClient()
    scheduler = EndpointScheduler(client, "tcp", {"read_retries": 0, "inter_request_ms": 0})
    first = asyncio.create_task(scheduler.submit("first", Request(3, 1, 0), 1000))
    await client.started.wait()
    old = asyncio.create_task(scheduler.submit("old", Request(3, 2, 0), 1000, poll_key="grid"))
    await asyncio.sleep(0)
    new = asyncio.create_task(scheduler.submit("new", Request(3, 2, 1), 1000, poll_key="grid"))
    assert (await old)["outcome"] == "coalesced"
    assert scheduler.pending == 1
    client.release.set()
    assert (await first)["outcome"] == "acknowledged"
    assert (await new)["outcome"] == "acknowledged"
    assert [request_id for request_id, _ in client.calls] == ["first", "new"]
    await scheduler.close()


async def test_close_settles_active_and_queued_commands():
    client = FakeClient()
    scheduler = EndpointScheduler(client, "tcp", {"read_retries": 0, "inter_request_ms": 0})
    active = asyncio.create_task(scheduler.submit("active", Request(3, 1, 0), 1000))
    await client.started.wait()
    waiting = asyncio.create_task(scheduler.submit("waiting", Request(3, 2, 0), 1000))
    await asyncio.sleep(0)
    await scheduler.close()
    assert (await active)["outcome"] == "cancelled"
    assert (await waiting)["outcome"] == "disconnected"
    assert client.fenced


class RecoveryClient:
    """@brief Return one timeout before a fresh transport is established."""

    def __init__(self):
        self.calls = []
        self.fenced = False
        self.recoveries = 0

    async def transact(self, request_id, request):
        self.calls.append((request_id, request.unit))
        if len(self.calls) == 1:
            self.fenced = True
            return {"outcome": "timeout", "values": ()}
        return {"outcome": "acknowledged", "values": (100,)}

    async def recover(self, _deadline):
        self.recoveries += 1
        self.fenced = False
        return True

    def close(self):
        self.fenced = True


@pytest.mark.parametrize("mode", ["rtu", "rtu_tcp"])
async def test_rtu_timeout_quarantines_same_signature_but_other_unit_recovers(mode):
    client = RecoveryClient()
    scheduler = EndpointScheduler(client, mode, {"read_retries": 3, "inter_request_ms": 0})
    first = await scheduler.submit("late", Request(3, 1, 0), 1000)
    assert first["outcome"] == "timeout"
    same = await scheduler.submit("same", Request(3, 1, 0), 1000)
    assert same["outcome"] == "disconnected"
    same_function = await scheduler.submit("same-function", Request(3, 1, 1), 1000)
    assert same_function["outcome"] == "disconnected"
    other = await scheduler.submit("other", Request(3, 2, 0), 1000)
    assert other["outcome"] == "acknowledged"
    assert client.calls == [("late", 1), ("other", 2)]
    assert client.recoveries == 1
    await scheduler.close()


async def test_tcp_read_retry_is_bounded_and_write_is_not_replayed():
    read_client = RecoveryClient()
    scheduler = EndpointScheduler(read_client, "tcp", {"read_retries": 1, "inter_request_ms": 0})
    read = await scheduler.submit("read", Request(3, 1, 0), 1000)
    assert read["outcome"] == "acknowledged"
    assert read["attempts"] == 2
    assert read_client.recoveries == 1
    await scheduler.close()

    class LostWrite(RecoveryClient):
        async def transact(self, request_id, request):
            self.calls.append((request_id, request.unit))
            return {"outcome": "delivery_unknown", "values": ()}

    write_client = LostWrite()
    scheduler = EndpointScheduler(write_client, "tcp", {"read_retries": 3, "inter_request_ms": 0})
    write = await scheduler.submit("write", Request(6, 1, 0, 1, (123,)), 1000)
    assert write["outcome"] == "delivery_unknown" and write["attempts"] == 1
    assert len(write_client.calls) == 1
    await scheduler.close()


async def test_independent_endpoints_progress_and_pending_limit_is_fixed():
    slow, other = FakeClient(), FakeClient()
    sched_slow = EndpointScheduler(slow, "tcp", {"read_retries": 0, "inter_request_ms": 0})
    sched_other = EndpointScheduler(other, "tcp", {"read_retries": 0, "inter_request_ms": 0})
    first = asyncio.create_task(sched_slow.submit("slow", Request(3, 1, 0), 1000))
    second = asyncio.create_task(sched_other.submit("independent", Request(3, 1, 0), 1000))
    await asyncio.wait_for(asyncio.gather(slow.started.wait(), other.started.wait()), 1)
    pending = [
        asyncio.create_task(sched_slow.submit(f"q{i}", Request(3, 1, i + 1), 1000))
        for i in range(8)
    ]
    await asyncio.sleep(0)
    full = await sched_slow.submit("full", Request(3, 1, 20), 1000)
    assert full["outcome"] == "queue_full" and sched_slow.pending == 8
    other.release.set()
    assert (await second)["outcome"] == "acknowledged"
    assert not first.done()
    slow.release.set()
    await first
    await asyncio.gather(*pending)
    await sched_slow.close()
    await sched_other.close()


# END OF FILE
