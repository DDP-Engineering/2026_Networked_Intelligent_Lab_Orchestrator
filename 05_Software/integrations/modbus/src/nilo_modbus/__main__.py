#!/usr/bin/env python3
# -*- coding: utf-8 -*-
# -----------------------------------------------------------------------------
# Copyright (c) 2026 DDP Engineering
# See LICENSE.txt in the repository root for full license details.
# SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
# -----------------------------------------------------------------------------
"""@file __main__.py
@brief Start the supervised Modbus SDK protocol process.
@details Bounded stdin pumping leaves cancellation responsive.
"""

# IMPORTS
import argparse
import asyncio
import sys
import threading

from nilo_integration import ProtocolServer

from .plugin import ModbusIntegration


# PUBLIC API
async def run(integration: ModbusIntegration) -> int:
    """@brief Feed bounded supervisor messages without blocking the event loop."""
    reader = asyncio.StreamReader()
    loop = asyncio.get_running_loop()
    queue: asyncio.Queue[bytes | None] = asyncio.Queue(maxsize=32)

    def read_stdin() -> None:
        while line := sys.stdin.buffer.readline((1 << 20) + 2):
            asyncio.run_coroutine_threadsafe(queue.put(line), loop).result()
        asyncio.run_coroutine_threadsafe(queue.put(None), loop).result()

    async def feed() -> None:
        while (line := await queue.get()) is not None:
            reader.feed_data(line)
        reader.feed_eof()

    threading.Thread(target=read_stdin, daemon=True).start()
    pump = asyncio.create_task(feed())
    try:
        return await ProtocolServer(integration).run(reader)
    finally:
        pump.cancel()
        await asyncio.gather(pump, return_exceptions=True)


def main() -> None:
    """@brief Accept only supervisor-owned startup options."""
    parser = argparse.ArgumentParser()
    parser.add_argument("--stdio", action="store_true", required=True)
    parser.parse_args()
    raise SystemExit(asyncio.run(run(ModbusIntegration())))


if __name__ == "__main__":
    main()


# END OF FILE
