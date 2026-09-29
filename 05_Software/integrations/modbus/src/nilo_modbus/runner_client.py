#!/usr/bin/env python3
# -*- coding: utf-8 -*-
# -----------------------------------------------------------------------------
# Copyright (c) 2026 DDP Engineering
# See LICENSE.txt in the repository root for full license details.
# SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
# -----------------------------------------------------------------------------
"""@file runner_client.py
@brief Open a pinned Modbus endpoint using the checked client boundary.
@details Reconnection creates a fresh transport only after the old one is fenced.
"""

# IMPORTS
import asyncio
import time
from collections.abc import Callable

import serial

from .feasibility import ValidatedClient


# DATA MODELS / TYPEDEFINITIONS
class RunnerClient(ValidatedClient):
    """@brief Own one production-configured transport without library retries."""

    def __init__(
        self,
        mode: str,
        *,
        host: str = "127.0.0.1",
        port: int = 0,
        path: str | None = None,
        settings: dict,
        serial_verifier: Callable[[], bool] | None = None,
    ):
        self.runner_mode = mode
        self.runner_host = host
        self.runner_port = port
        self.runner_path = path
        self.runner_settings = dict(settings)
        self.serial_verifier = serial_verifier
        self.connect_timeout = min(settings["connect_timeout_ms"] / 1000, 60)
        frame_gap = 3.5 * 11 / settings.get("baud_rate", 19200)
        self.quiet_seconds = max(0.004, frame_gap, settings["inter_request_ms"] / 1000)
        self._open_client()

    def _open_client(self) -> None:
        framing = self.runner_settings
        super().__init__(
            self.runner_mode,
            host=self.runner_host,
            port=self.runner_port,
            path=self.runner_path,
            timeout=framing["response_timeout_ms"] / 1000,
            baudrate=framing.get("baud_rate", 19200),
            parity={"none": "N", "even": "E", "odd": "O"}.get(framing.get("parity"), "N"),
            stopbits=framing.get("stop_bits", 1),
            local_echo=framing.get("local_echo", False),
        )

    async def connect(self):
        """@brief Enforce the pinned connection timeout before serving commands."""
        try:
            connected = await asyncio.wait_for(super().connect(), self.connect_timeout)
        except (TimeoutError, OSError):
            connected = False
        if not connected:
            self.close()
        return connected

    async def recover(self, deadline: float) -> bool:
        """@brief Discard serial leftovers or a closed socket before reopening."""
        self.close()
        if self.runner_mode == "rtu":
            if not self._verify_serial() or not await self._drain_serial(deadline):
                return False
            if not self._verify_serial():
                return False
        else:
            if deadline - time.monotonic() <= self.quiet_seconds:
                return False
            await asyncio.sleep(self.quiet_seconds)
        if deadline - time.monotonic() <= 0:
            return False
        self._open_client()
        return await self.connect()

    def _verify_serial(self) -> bool:
        """@brief Refuse a rebound tty before touching its serial descriptor."""
        if self.serial_verifier is None:
            return True
        try:
            return self.serial_verifier()
        except (ValueError, OSError):
            return False

    async def _drain_serial(self, deadline: float) -> bool:
        """@brief Read discarded bytes until a frame-gap silence fits the deadline."""
        framing = self.runner_settings
        try:
            handle = serial.Serial(
                self.runner_path,
                baudrate=framing["baud_rate"],
                bytesize=8,
                parity={"none": "N", "even": "E", "odd": "O"}[framing["parity"]],
                stopbits=framing["stop_bits"],
                timeout=0,
                exclusive=True,
            )
        except (OSError, serial.SerialException):
            return False
        try:
            silent_since = time.monotonic()
            while time.monotonic() < deadline:
                if handle.read(handle.in_waiting or 1):
                    silent_since = time.monotonic()
                elif time.monotonic() - silent_since >= self.quiet_seconds:
                    return True
                await asyncio.sleep(min(0.002, self.quiet_seconds / 4))
            return False
        except (OSError, serial.SerialException):
            return False
        finally:
            handle.close()


# END OF FILE
