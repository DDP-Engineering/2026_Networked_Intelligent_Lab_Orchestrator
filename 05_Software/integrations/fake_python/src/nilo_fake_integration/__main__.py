"""Console entry point for the deterministic fake NILO integration."""

from __future__ import annotations

import asyncio
import sys

from nilo_integration import ProtocolServer

from nilo_fake_integration.plugin import FakeIntegration


async def run() -> int:
    reader = asyncio.StreamReader()
    pump = asyncio.create_task(_pump_stdin(reader))
    try:
        return await ProtocolServer(FakeIntegration()).run(reader)
    finally:
        if not pump.done():
            pump.cancel()
        await asyncio.gather(pump, return_exceptions=True)


async def _pump_stdin(reader: asyncio.StreamReader) -> None:
    """Bridge the blocking console pipe without platform-specific pipe transports."""

    while line := await asyncio.to_thread(sys.stdin.buffer.readline):
        reader.feed_data(line)
    reader.feed_eof()


def main() -> None:
    raise SystemExit(asyncio.run(run()))


if __name__ == "__main__":
    main()
