#!/usr/bin/env python3
# -*- coding: utf-8 -*-
# -----------------------------------------------------------------------------
# Copyright (c) 2026 DDP Engineering
# This software is licensed under the PolyForm Noncommercial License 1.0.0.
# See LICENSE.txt in the repository root for full license details.
# SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
# -----------------------------------------------------------------------------
"""@file feasibility.py
@brief Guard a candidate client inside isolated simulation.
@details Feasibility only; production plugin registration is a later package.
"""

# IMPORTS
import asyncio
import ipaddress
import time

from pymodbus.client import AsyncModbusSerialClient, AsyncModbusTcpClient
from pymodbus.exceptions import ModbusException
from pymodbus.framer import FramerType

from .requests import WRITES, Request, encode

# CONSTANTS
METHODS = {
    1: "read_coils",
    2: "read_discrete_inputs",
    3: "read_holding_registers",
    4: "read_input_registers",
    5: "write_coil",
    6: "write_register",
    15: "write_coils",
    16: "write_registers",
}


# HELPER FUNCTIONS
def crc(data):
    """@brief Compute low-first RTU CRC independently of the library framer."""
    value = 0xFFFF
    for byte in data:
        value ^= byte
        for _ in range(8):
            value = (value >> 1) ^ (0xA001 if value & 1 else 0)
    return value.to_bytes(2, "little")


def validate_reply(data, sent, request, mode):
    """@brief Check a complete ADU against its exact request.
    @return False while fragmented input is incomplete, True for a valid first frame.
    @exception ValueError Inconsistent framing, identity, quantity or echo.
    """
    if mode == "tcp":
        if len(data) < 6:
            return False
        size = int.from_bytes(data[4:6], "big")
        if data[2:4] != b"\x00\x00" or not 3 <= size <= 254:
            raise ValueError("MBAP protocol/length")
        if data[:2] != sent[:2]:
            raise ValueError("transaction")
        total = size + 6
        if len(data) < total:
            return False
        unit, pdu = data[6], data[7:total]
    else:
        if len(data) < 3:
            return False
        fc = data[1]
        total = 5 if fc & 0x80 else data[2] + 5 if fc in (1, 2, 3, 4) else 8
        if not 5 <= total <= 256:
            raise ValueError("RTU length")
        if len(data) < total:
            return False
        if crc(data[: total - 2]) != data[total - 2 : total]:
            raise ValueError("CRC")
        unit, pdu = data[0], data[1 : total - 2]
    if unit != request.unit or pdu[0] not in (request.function, request.function | 0x80):
        raise ValueError("unit/function")
    if pdu[0] & 0x80:
        if len(pdu) != 2 or pdu[1] == 0:
            raise ValueError("exception length/code")
        return True
    if request.function in WRITES:
        if pdu != encode(request, mode)[:5]:
            raise ValueError("write echo")
    else:
        expected = (request.count + 7) // 8 if request.function in (1, 2) else request.count * 2
        if len(pdu) != expected + 2 or pdu[1] != expected:
            raise ValueError("read count")
    return True


# DATA MODELS / TYPEDEFINITIONS
class SimulationClient:
    """@brief Own one explicit simulation connection with serialized operations.
    @details This prototype is never registered with Core or given real targets.
    """

    def __init__(self, mode, *, host="127.0.0.1", port=0, path=None, timeout=0.2, simulation=False):
        if not simulation or mode not in ("tcp", "rtu_tcp", "rtu"):
            raise ValueError("explicit supported simulation required")
        if mode == "rtu":
            if (
                not isinstance(path, str)
                or not path.startswith("/dev/pts/")
                or not path[9:].isdigit()
            ):
                raise ValueError("Linux PTY only")
        elif not ipaddress.ip_address(host).is_loopback or not 1 <= port <= 65535:
            raise ValueError("loopback only")
        if not 0.01 <= timeout <= 10:
            raise ValueError("bounded timeout")
        self.mode = mode
        self.timeout = timeout
        self.lock = asyncio.Lock()
        self.fenced = False
        self.request_id = None
        self.events = []
        self.trace_bytes = 0
        self.active = None
        self.sent = b""
        self.valid_reply = False
        self.protocol_fault = False
        self.started = time.monotonic()
        settings = {
            "timeout": timeout,
            "retries": 0,
            "reconnect_delay": 0,
            "trace_packet": self.trace,
        }
        if mode == "rtu":
            self.client = AsyncModbusSerialClient(
                path, baudrate=19200, parity="N", bytesize=8, stopbits=1, **settings
            )
        else:
            self.client = AsyncModbusTcpClient(
                host,
                port=port,
                framer=FramerType.SOCKET if mode == "tcp" else FramerType.RTU,
                **settings,
            )

    def trace(self, sending, data):
        """@brief Observe library buffers under the current request identity."""
        if self.request_id is None:
            return b""
        if len(self.events) >= 256 or self.trace_bytes + len(data) > 65536:
            self.protocol_fault = True
            return b""
        if sending:
            expected = encode(self.active, self.mode)
            valid = False
            if self.mode == "tcp" and len(data) >= 7:
                valid = (
                    data[2:4] == b"\x00\x00"
                    and int.from_bytes(data[4:6], "big") == len(data) - 6
                    and data[6] == self.active.unit
                    and data[7:] == expected
                )
            elif self.mode != "tcp" and len(data) >= 4:
                valid = (
                    data[0] == self.active.unit
                    and data[1:-2] == expected
                    and crc(data[:-2]) == data[-2:]
                )
            if not valid:
                self.protocol_fault = True
                return b""
        self.trace_bytes += len(data)
        self.events.append(
            {
                "request_id": self.request_id,
                "direction": "tx" if sending else "rx",
                "sequence": len(self.events) + 1,
                "kind": "library_buffer",
                "data_hex": data.hex(),
                "elapsed_ms": (time.monotonic() - self.started) * 1000,
            }
        )
        if sending:
            self.sent = data
        elif self.active and self.sent:
            try:
                self.valid_reply = validate_reply(data, self.sent, self.active, self.mode)
            except (ValueError, IndexError):
                self.protocol_fault = True
                return b""
        return data

    async def connect(self):
        """@brief Connect only by explicit caller action while the instance is fresh."""
        if self.fenced:
            return False
        return await self.client.connect()

    def close(self):
        """@brief Fence reuse and release the candidate transport."""
        self.fenced = True
        self.client.close()

    async def transact(self, request_id: str, request: Request):
        """@brief Run one validated initial request without automatic retries."""
        if not isinstance(request_id, str) or not 1 <= len(request_id) <= 128:
            raise ValueError("bounded correlation ID required")
        encode(request, self.mode)
        async with self.lock:
            if self.fenced or not self.client.connected:
                return {"outcome": "disconnected", "values": ()}
            self.request_id = request_id
            self.active = request
            self.sent = b""
            self.valid_reply = self.protocol_fault = False
            try:
                method = getattr(self.client, METHODS[request.function])
                if request.function in (1, 2, 3, 4):
                    result = await method(
                        request.address, count=request.count, device_id=request.unit
                    )
                else:
                    values = (
                        request.values[0] if request.function in (5, 6) else list(request.values)
                    )
                    result = await method(request.address, values, device_id=request.unit)
                if self.protocol_fault or not self.valid_reply:
                    raise ValueError("unvalidated reply")
                if result.isError():
                    return {
                        "outcome": "exception",
                        "values": (),
                        "exception_code": result.exception_code,
                    }
                values = (
                    request.values
                    if request.function in WRITES
                    else tuple(
                        result.bits[: request.count]
                        if request.function in (1, 2)
                        else result.registers
                    )
                )
                return {"outcome": "acknowledged", "values": values}
            except (ModbusException, OSError, ValueError, asyncio.CancelledError):
                cancelled = bool(asyncio.current_task().cancelling())
                self.close()
                return {
                    "outcome": "delivery_unknown"
                    if request.function in WRITES and self.sent
                    else "rejected"
                    if request.function in WRITES
                    else "cancelled"
                    if cancelled
                    else "protocol_error"
                    if self.protocol_fault
                    else "timeout",
                    "values": (),
                }
            except BaseException:
                self.close()
                raise
            finally:
                self.request_id = None
                self.active = None


# END OF FILE
