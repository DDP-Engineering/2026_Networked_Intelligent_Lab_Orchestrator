#!/usr/bin/env python3
# -*- coding: utf-8 -*-
# -----------------------------------------------------------------------------
# Copyright (c) 2026 DDP Engineering
# This software is licensed under the PolyForm Noncommercial License 1.0.0.
# See LICENSE.txt in the repository root for full license details.
# SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
# -----------------------------------------------------------------------------
"""@file peers.py
@brief Handwritten simulation peers independent of the candidate library.
@details Feasibility only; production plugin registration is a later package.
"""

# IMPORTS
import asyncio
import os
import struct
from contextlib import asynccontextmanager


# HELPER FUNCTIONS
def crc(payload: bytes) -> bytes:
    """@brief Independent bitwise CRC oracle, low byte first."""
    value = 0xFFFF
    for byte in payload:
        value ^= byte
        for _ in range(8):
            value = (value >> 1) ^ (0xA001 if value & 1 else 0)
    return value.to_bytes(2, "little")


def reply(frame, mode, fault):
    """@brief Encode a reference response without library response classes."""
    unit, pdu = (frame[6], frame[7:]) if mode == "tcp" else (frame[0], frame[1:-2])
    fc, address, count = struct.unpack(">BHH", pdu[:5])
    if fc in (1, 2):
        values = bytearray((count + 7) // 8)
        for index in range(count):
            values[index // 8] |= (index % 2 == 0) << (index % 8)
        response = bytes((fc, len(values))) + values
    elif fc in (3, 4):
        response = bytes((fc, count * 2)) + struct.pack(
            ">" + "H" * count, *range(address + 100, address + 100 + count)
        )
    else:
        response = pdu[:5]
    if fault == "exception":
        response = bytes((fc | 0x80, 2))
    if fault == "function":
        response = bytes((4 if fc == 3 else 3,)) + response[1:]
    if fault == "count":
        response = bytes((fc, 2)) + b"\x00\x63"
    if fault == "echo":
        response = bytes((fc,)) + b"\x00\x01" + response[3:]
    if fault == "unit":
        unit += 1
    if mode == "tcp":
        tid = int.from_bytes(frame[:2], "big") + (1 if fault == "tid" else 0)
        packet = (
            struct.pack(">HHHB", tid, 1 if fault == "pid" else 0, len(response) + 1, unit)
            + response
        )
        if fault == "length":
            packet = packet[:4] + b"\xff\xff" + packet[6:]
        return packet
    packet = bytes((unit,)) + response
    check = crc(packet)
    if fault == "crc":
        check = bytes((check[0] ^ 1, check[1]))
    return packet + check


# DATA MODELS / TYPEDEFINITIONS
class Peer:
    """@brief Observe complete requests and emit explicit simulation responses."""

    def __init__(self, mode, fault=None):
        self.mode, self.fault = mode, fault
        self.requests = []
        self.received = asyncio.Event()
        self.tasks = set()
        self.host, self.port, self.path = "127.0.0.1", 0, None

    async def serve(self, reader, send):
        """@brief Process frames serially with bounded response fault cases."""
        try:
            while True:
                if self.mode == "tcp":
                    head = await reader.readexactly(7)
                    frame = head + await reader.readexactly(int.from_bytes(head[4:6], "big") - 1)
                else:
                    head = await reader.readexactly(6)
                    extra = 2
                    if head[1] in (15, 16):
                        byte_count = await reader.readexactly(1)
                        head += byte_count
                        extra += byte_count[0]
                    frame = head + await reader.readexactly(extra)
                    assert crc(frame[:-2]) == frame[-2:]
                self.requests.append(frame)
                self.received.set()
                if self.fault == "drop":
                    continue
                if self.fault == "late":
                    await asyncio.sleep(0.25)
                packet = reply(frame, self.mode, self.fault)
                if self.fault == "duplicate":
                    await send(packet + packet)
                elif self.fault == "split":
                    for byte in packet:
                        await send(bytes((byte,)))
                        await asyncio.sleep(0.001)
                else:
                    await send(packet)
        except (asyncio.IncompleteReadError, ConnectionError, OSError):
            return


# PUBLIC API
@asynccontextmanager
async def peer(mode, fault=None):
    """@brief Own loopback or Linux PTY lifecycle and clean up handler tasks."""
    p = Peer(mode, fault)
    descriptors = []
    loop = asyncio.get_running_loop()
    server = None
    try:
        if mode == "rtu":
            import pty

            master, slave = pty.openpty()
            descriptors = [master, slave]
            p.path = os.ttyname(slave)
            os.set_blocking(master, False)
            reader = asyncio.StreamReader()

            def ready():
                """@brief Feed PTY bytes; EOF remains visible to the peer task."""
                try:
                    data = os.read(master, 4096)
                    if data:
                        reader.feed_data(data)
                except OSError:
                    reader.feed_eof()

            loop.add_reader(master, ready)

            async def send(data):
                """@brief Write tiny fixture responses to the PTY master."""
                os.write(master, data)

            task = asyncio.create_task(p.serve(reader, send))
            p.tasks.add(task)
        else:

            async def connected(reader, writer):
                """@brief Track server tasks and close accepted sockets."""
                task = asyncio.current_task()
                p.tasks.add(task)

                async def send(data):
                    """@brief Emit one independent response chunk."""
                    writer.write(data)
                    await writer.drain()

                try:
                    await p.serve(reader, send)
                finally:
                    writer.close()
                    await writer.wait_closed()
                    p.tasks.discard(task)

            server = await asyncio.start_server(connected, p.host, 0)
            p.port = server.sockets[0].getsockname()[1]
        yield p
    finally:
        if server:
            server.close()
            await server.wait_closed()
        if descriptors:
            loop.remove_reader(descriptors[0])
        for task in list(p.tasks):
            task.cancel()
        await asyncio.gather(*list(p.tasks), return_exceptions=True)
        for descriptor in descriptors:
            os.close(descriptor)


# END OF FILE
