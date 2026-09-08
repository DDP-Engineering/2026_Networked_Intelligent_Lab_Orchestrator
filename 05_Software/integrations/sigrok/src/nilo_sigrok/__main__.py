#!/usr/bin/env python3
# -*- coding: utf-8 -*-
# -----------------------------------------------------------------------------
# Copyright (c) 2026 DDP Engineering
# SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
# -----------------------------------------------------------------------------
"""@file __main__.py
@brief Start the isolated sigrok protocol server.
@details Runtime selection is supplied by the trusted package supervisor.
"""

import argparse
import asyncio
import sys
import threading

from nilo_integration import ProtocolServer

from nilo_sigrok.plugin import SigrokIntegration


async def run(integration: SigrokIntegration) -> int:
    """@brief Feed bounded stdin records without blocking fatal server shutdown."""
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
    """@brief Parse server-owned runtime arguments and start the runner."""
    parser = argparse.ArgumentParser()
    parser.add_argument("--stdio", action="store_true", required=True)
    parser.add_argument("--simulation", action="store_true")
    parser.add_argument("--backend", default="/usr/bin/sigrok-cli")
    options = parser.parse_args()
    raise SystemExit(asyncio.run(run(SigrokIntegration(options.backend, options.simulation))))


if __name__ == "__main__":
    main()

# END OF FILE
