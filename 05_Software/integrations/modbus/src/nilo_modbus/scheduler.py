#!/usr/bin/env python3
# -*- coding: utf-8 -*-
# -----------------------------------------------------------------------------
# Copyright (c) 2026 DDP Engineering
# See LICENSE.txt in the repository root for full license details.
# SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
# -----------------------------------------------------------------------------
"""@file scheduler.py
@brief Serialize one Modbus endpoint while rotating waiting units fairly.
@details Bounded queues and monotonic deadlines prevent poll catch-up and overlap.
"""

# IMPORTS
import asyncio
import time
from collections import deque
from dataclasses import dataclass

from .requests import WRITES, Request, encode

# CONSTANTS
MAX_PENDING = 128
MAX_PER_UNIT = 8
MAX_READ_RETRIES = 3


# DATA MODELS / TYPEDEFINITIONS
@dataclass
class _Pending:
    request_id: str
    request: Request
    deadline: float
    poll_key: str | None
    result: asyncio.Future


class EndpointScheduler:
    """@brief Own the single caller of a transport client for one endpoint."""

    def __init__(self, client, mode: str, settings: dict):
        if mode not in {"rtu", "tcp", "rtu_tcp"}:
            raise ValueError("invalid_transport")
        self.client = client
        self.mode = mode
        self.settings = settings
        self.queues: dict[int, deque[_Pending]] = {}
        self.rotation: deque[int] = deque()
        self.ids: set[str] = set()
        self.pending = 0
        self.last_unit: int | None = None
        self.quarantined: set[tuple[int, int]] = set()
        self.active: _Pending | None = None
        self.closed = False
        self.wakeup = asyncio.Event()
        self.worker = asyncio.create_task(self._run())

    def _outcome(self, item: _Pending, outcome: str, attempts: int = 0) -> dict:
        return {
            "request_id": item.request_id,
            "outcome": outcome,
            "values": (),
            "attempts": attempts,
        }

    def _settle(self, item: _Pending, value: dict) -> None:
        self.ids.discard(item.request_id)
        if not item.result.done():
            item.result.set_result(value)

    async def submit(
        self, request_id: str, request: Request, deadline_ms: int, *, poll_key: str | None = None
    ) -> dict:
        """@brief Admit one bounded command and await its final outcome."""
        if (
            not isinstance(request_id, str)
            or not 1 <= len(request_id) <= 128
            or type(deadline_ms) is not int
            or not 1 <= deadline_ms <= 60000
            or poll_key is not None
            and (
                request.function in WRITES
                or not isinstance(poll_key, str)
                or not 1 <= len(poll_key) <= 128
            )
        ):
            raise ValueError("invalid_parameters")
        encode(request, self.mode)
        if self.closed:
            return {
                "request_id": request_id,
                "outcome": "disconnected",
                "values": (),
                "attempts": 0,
            }
        if request_id in self.ids:
            return {"request_id": request_id, "outcome": "rejected", "values": (), "attempts": 0}
        queue = self.queues.setdefault(request.unit, deque())
        if poll_key is not None:
            for old in tuple(queue):
                if old.poll_key == poll_key:
                    queue.remove(old)
                    self.pending -= 1
                    self._settle(old, self._outcome(old, "coalesced"))
        if self.pending >= MAX_PENDING or len(queue) >= MAX_PER_UNIT:
            return {"request_id": request_id, "outcome": "queue_full", "values": (), "attempts": 0}
        loop = asyncio.get_running_loop()
        item = _Pending(
            request_id,
            request,
            time.monotonic() + deadline_ms / 1000,
            poll_key,
            loop.create_future(),
        )
        if not queue and request.unit not in self.rotation:
            self.rotation.append(request.unit)
        queue.append(item)
        self.pending += 1
        self.ids.add(request_id)
        self.wakeup.set()
        try:
            return await item.result
        except asyncio.CancelledError:
            if item in queue:
                queue.remove(item)
                self.pending -= 1
                self.ids.discard(request_id)
            elif self.active is item:
                self.closed = True
                self.client.close()
                self.worker.cancel()
            raise

    def _next(self) -> _Pending | None:
        while self.rotation:
            if len(self.rotation) > 1 and self.rotation[0] == self.last_unit:
                self.rotation.rotate(-1)
            unit = self.rotation.popleft()
            queue = self.queues[unit]
            if not queue:
                continue
            item = queue.popleft()
            self.pending -= 1
            if queue:
                self.rotation.append(unit)
            self.last_unit = unit
            return item
        return None

    async def _execute(self, item: _Pending) -> dict:
        attempts = 0
        signature = (item.request.unit, item.request.function)
        if signature in self.quarantined:
            return self._outcome(item, "disconnected")
        if getattr(self.client, "fenced", False) and (
            not hasattr(self.client, "recover") or not await self.client.recover(item.deadline)
        ):
            return self._outcome(item, "disconnected")
        while not self.closed:
            remaining = item.deadline - time.monotonic()
            if remaining <= 0:
                return self._outcome(item, "timeout", attempts)
            attempts += 1
            try:
                result = await asyncio.wait_for(
                    self.client.transact(item.request_id, item.request), remaining
                )
            except TimeoutError:
                self.client.close()
                result = {
                    "outcome": "delivery_unknown" if item.request.function in WRITES else "timeout",
                    "values": (),
                }
            except asyncio.CancelledError:
                self.client.close()
                return self._outcome(
                    item,
                    "delivery_unknown" if item.request.function in WRITES else "cancelled",
                    attempts,
                )
            result = {**result, "request_id": item.request_id, "attempts": attempts}
            if self.mode != "tcp" and result["outcome"] in {"timeout", "protocol_error"}:
                self.quarantined.add(signature)
            if (
                item.request.function in WRITES
                or result["outcome"] != "timeout"
                or attempts > min(self.settings.get("read_retries", 0), MAX_READ_RETRIES)
                or self.mode != "tcp"
                or not hasattr(self.client, "recover")
            ):
                return result
            if not await self.client.recover(item.deadline):
                return result
        return self._outcome(item, "disconnected", attempts)

    async def _run(self) -> None:
        try:
            while not self.closed:
                item = self._next()
                if item is None:
                    self.wakeup.clear()
                    await self.wakeup.wait()
                    continue
                self.active = item
                if item.result.done() or time.monotonic() >= item.deadline:
                    self._settle(item, self._outcome(item, "timeout"))
                else:
                    value = await self._execute(item)
                    self._settle(item, value)
                self.active = None
                gap = self.settings.get("inter_request_ms", 0) / 1000
                if gap:
                    await asyncio.sleep(gap)
        finally:
            self.closed = True
            self.client.close()
            if self.active is not None:
                self._settle(self.active, self._outcome(self.active, "disconnected"))
            for queue in self.queues.values():
                while queue:
                    item = queue.popleft()
                    self._settle(item, self._outcome(item, "disconnected"))
            self.pending = 0

    async def close(self) -> None:
        """@brief Cancel the worker and settle every accepted command."""
        if self.closed:
            await asyncio.gather(self.worker, return_exceptions=True)
            return
        self.closed = True
        self.worker.cancel()
        await asyncio.gather(self.worker, return_exceptions=True)


# END OF FILE
