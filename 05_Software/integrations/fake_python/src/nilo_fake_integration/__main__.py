"""Console entry point for the deterministic fake NILO integration."""

from __future__ import annotations

import asyncio
import sys
import threading

from nilo_integration import ProtocolServer

from nilo_fake_integration.plugin import FakeIntegration


async def run() -> int:
    reader = asyncio.StreamReader()
    queue: asyncio.Queue[bytes | None] = asyncio.Queue()
    _start_stdin_pump(asyncio.get_running_loop(), queue)
    pump = asyncio.create_task(_feed_reader(queue, reader))
    try:
        return await ProtocolServer(FakeIntegration()).run(reader)
    finally:
        pump.cancel()
        await asyncio.gather(pump, return_exceptions=True)


def _start_stdin_pump(loop: asyncio.AbstractEventLoop, queue: asyncio.Queue[bytes | None]) -> None:
    """Feed stdin from a daemon so fatal protocol exits never await an open pipe."""

    def pump() -> None:
        while line := sys.stdin.buffer.readline():
            loop.call_soon_threadsafe(queue.put_nowait, line)
        loop.call_soon_threadsafe(queue.put_nowait, None)

    threading.Thread(target=pump, name="nilo-fake-stdin", daemon=True).start()


async def _feed_reader(queue: asyncio.Queue[bytes | None], reader: asyncio.StreamReader) -> None:
    while (line := await queue.get()) is not None:
        reader.feed_data(line)
    reader.feed_eof()
    reader.feed_eof()


def main() -> None:
    raise SystemExit(asyncio.run(run()))


if __name__ == "__main__":
    main()
