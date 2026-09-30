#!/usr/bin/env python3
# -*- coding: utf-8 -*-
# -----------------------------------------------------------------------------
# Copyright (c) 2026 DDP Engineering
# See LICENSE.txt in the repository root for full license details.
# SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
# -----------------------------------------------------------------------------
"""@file plugin.py
@brief Run one pinned Modbus endpoint under the integration SDK supervisor.
@details Core owns policy, claims and public session admission.
"""

# IMPORTS
from nilo_integration import ExecuteFailure
from nilo_integration.models import (
    ConfigureResult,
    DescribeResult,
    DiscoverResult,
    ExecuteResult,
    HealthResult,
)

from .workspace import Workspace


# DATA MODELS / TYPEDEFINITIONS
class ModbusIntegration:
    """@brief Expose one long-lived private `serve` operation to the SDK."""

    name = "modbus"

    def __init__(self):
        self.workspace: Workspace | None = None

    async def describe(self):
        return DescribeResult(integration=self.name, operations=["connect", "serve"])

    async def discover(self):
        return DiscoverResult()

    async def health(self):
        return HealthResult()

    async def configure(self, request):
        config = request.configuration
        if (
            not isinstance(config, dict)
            or set(config) != {"schema_version", "plugin", "device"}
            or config["schema_version"] != 1
            or config["device"] is not None
            or not isinstance(config["plugin"], dict)
            or set(config["plugin"]) != {"revision", "settings"}
            or type(config["plugin"]["revision"]) is not int
            or config["plugin"]["revision"] < 0
            or config["plugin"]["settings"] != {}
        ):
            raise ExecuteFailure("invalid_parameters", "Invalid Modbus configuration")
        return ConfigureResult()

    async def cancel(self, _request_id):
        if self.workspace is not None:
            self.workspace.stop.set()

    async def safe_shutdown(self):
        await self.cancel(None)

    async def execute(self, request, _events):
        if request.operation != "serve":
            raise ExecuteFailure("invalid_parameters", "Unknown Modbus operation")
        try:
            self.workspace = Workspace(request.parameters)
            await self.workspace.run()
        except (ValueError, OSError) as error:
            code = (
                str(error)
                if str(error) in {"invalid_parameters", "invalid_workspace", "device_unavailable"}
                else "device_unavailable"
            )
            raise ExecuteFailure(code, "Modbus endpoint could not run") from error
        finally:
            self.workspace = None
        return ExecuteResult(status="completed", data={"state": "stopped"})


# END OF FILE
