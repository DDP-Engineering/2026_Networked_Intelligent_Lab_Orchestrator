#!/usr/bin/env python3
# -*- coding: utf-8 -*-
# -----------------------------------------------------------------------------
# Copyright (c) 2026 DDP Engineering
# SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
# -----------------------------------------------------------------------------
"""@file plugin.py
@brief Implement the supervised SDK lifecycle for physical USB serial ports.
@details Core approves sessions; the private stream is supplied by Core only.
"""

import asyncio
import re

from nilo_integration import ExecuteFailure
from nilo_integration.models import (
    ConfigureResult,
    DescribeResult,
    DiscoverResult,
    ExecuteResult,
    HealthResult,
)

from nilo_serial.discovery import scan_ports
from nilo_serial.workspace import Workspace


class SerialIntegration:
    """@brief Own configuration and one private streaming workspace."""

    name = "serial"

    def __init__(self):
        self.workspace = None

    async def describe(self):
        return DescribeResult(
            integration=self.name, operations=["discover_devices", "connect", "stream"]
        )

    async def discover(self):
        return DiscoverResult()

    async def health(self):
        return HealthResult()

    async def configure(self, request):
        config = request.configuration
        if set(config) != {"schema_version", "plugin", "device"} or config["schema_version"] != 1:
            raise ExecuteFailure("invalid_parameters", "Invalid configuration")
        plugin, device = config["plugin"], config["device"]
        if (
            not isinstance(plugin, dict)
            or set(plugin) != {"revision", "settings"}
            or type(plugin["revision"]) is not int
            or plugin["revision"] < 0
            or plugin["settings"] != {}
        ):
            raise ExecuteFailure("invalid_parameters", "Invalid configuration")
        if device is not None and (
            not isinstance(device, dict)
            or set(device) != {"id", "revision", "settings"}
            or not isinstance(device["id"], str)
            or not re.fullmatch(r"serial-[1-9][0-9]*", device["id"])
            or type(device["revision"]) is not int
            or device["revision"] < 0
            or device["settings"] != {}
        ):
            raise ExecuteFailure("invalid_parameters", "Invalid device configuration")
        return ConfigureResult()

    async def cancel(self, _request_id):
        if self.workspace:
            self.workspace.stop.set()

    async def safe_shutdown(self):
        await self.cancel(None)

    async def execute(self, request, _events):
        if request.operation == "discover_devices" and request.parameters == {}:
            return ExecuteResult(status="completed", data={"devices": scan_ports()})
        if request.operation == "stream":
            try:
                self.workspace = Workspace(request.parameters)
                await asyncio.to_thread(self.workspace.run)
                return ExecuteResult(status="completed", data={"state": "stopped"})
            except (ValueError, OSError) as error:
                code = (
                    str(error)
                    if str(error) in {"invalid_parameters", "port_busy", "device_unavailable"}
                    else "device_unavailable"
                )
                raise ExecuteFailure(code, "Serial session could not run") from error
        raise ExecuteFailure("invalid_parameters", "Session connection is owned by Core")


# END OF FILE
