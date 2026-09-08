#!/usr/bin/env python3
# -*- coding: utf-8 -*-
# -----------------------------------------------------------------------------
# Copyright (c) 2026 DDP Engineering
# SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
# -----------------------------------------------------------------------------
"""@file plugin.py
@brief Adapt finite digital sigrok operations to NILO's runner protocol.
@details The core supplies confined paths; the adapter validates digital-only
arguments again and never forwards arbitrary CLI configuration.
"""

# IMPORTS
from __future__ import annotations

import asyncio
import configparser
import hashlib
import re
import struct
import zipfile
from pathlib import Path
from uuid import UUID

from nilo_integration import EventSink, ExecuteFailure
from nilo_integration.models import (
    ArtifactDeclaration,
    ConfigureRequest,
    ConfigureResult,
    DescribeResult,
    DiscoverResult,
    ExecuteRequest,
    ExecuteResult,
    HealthResult,
    Progress,
)

DIGITAL_CHANNELS = tuple(f"D{i}" for i in range(16))
SAMPLE_RATES = (1_000_000, 2_000_000, 4_000_000, 5_000_000, 10_000_000)
MAX_SAMPLES = 1_000_000
MAX_ARTIFACT = 8 << 20
CONNECTION = re.compile(r"[0-9]+(?:\.[0-9]+)+\Z")
DEVICE_LINE = re.compile(r"^atk-dl16:conn=([0-9.]+) - ALIENTEK ATK DL16 with")
ANNOTATION = re.compile(r"^(\d+)-(\d+) uart(?:-[^: ]+)?: (.*)$")


def invalid(message: str) -> ExecuteFailure:
    return ExecuteFailure("invalid_parameters", message)


def parameters(value: object, allowed: set[str]) -> dict:
    """@brief Reject unknown configuration before interpreting its values."""
    if not isinstance(value, dict) or set(value) - allowed:
        raise invalid("Unsupported parameters; only the declared digital operation is available")
    return value


def capture_parameters(value: object) -> dict:
    """@brief Validate finite digital acquisition, excluding all output controls."""
    data = parameters(value, {"conn", "channels", "samplerate", "samples", "staging_directory"})
    channels = data.get("channels")
    if (
        not isinstance(channels, list)
        or not channels
        or len(channels) > 16
        or any(channel not in DIGITAL_CHANNELS for channel in channels)
        or len(set(channels)) != len(channels)
    ):
        raise invalid("Select distinct digital channels D0 through D15")
    if type(data.get("samplerate")) is not int or data["samplerate"] not in SAMPLE_RATES:
        raise invalid("Unsupported sample rate")
    if type(data.get("samples")) is not int or not 1 <= data["samples"] <= MAX_SAMPLES:
        raise invalid("Sample count must be between 1 and 1000000")
    if not isinstance(data.get("conn"), str) or not CONNECTION.fullmatch(data["conn"]):
        raise invalid("Invalid analyzer connection")
    directory = Path(data.get("staging_directory", ""))
    if not directory.is_absolute() or not directory.is_dir() or directory.is_symlink():
        raise invalid("A core-owned staging directory is required")
    return data


def parse_devices(output: str) -> list[dict]:
    """@brief Normalize the approved driver output to digital-only capabilities."""
    devices = []
    for line in output.splitlines():
        match = DEVICE_LINE.match(line)
        if match and CONNECTION.fullmatch(match[1]):
            devices.append(
                {
                    "id": f"atk-dl16-{match[1]}",
                    "connection": match[1],
                    "display_name": "ALIENTEK ATK DL16",
                    "driver": "atk-dl16",
                    "channels": list(DIGITAL_CHANNELS),
                    "samplerates": list(SAMPLE_RATES),
                    "maximum_samples": MAX_SAMPLES,
                }
            )
    return devices


def read_capture(path: Path) -> dict:
    """@brief Inspect bounded srzip metadata and a small digital preview.
    @details Archive contents are read in place, never extracted to supplied paths.
    """
    if not path.is_file() or path.stat().st_size > MAX_ARTIFACT:
        raise ExecuteFailure("invalid_capture", "Capture is missing or exceeds its size limit")
    with zipfile.ZipFile(path) as archive:
        if (
            len(archive.infolist()) > 128
            or sum(i.file_size for i in archive.infolist()) > MAX_ARTIFACT
        ):
            raise ExecuteFailure("invalid_capture", "Unpacked capture exceeds its limit")
        config = configparser.ConfigParser(interpolation=None)
        config.read_string(archive.read("metadata").decode("utf-8"))
        device = config["device 1"]
        unitsize = int(device.get("unitsize", "2"))
        if unitsize not in (1, 2, 4):
            raise ExecuteFailure("invalid_capture", "Unsupported sample packing")
        members = sorted(i for i in archive.namelist() if i.startswith("logic-1-"))
        size = sum(archive.getinfo(i).file_size for i in members)
        if not members or size % unitsize:
            raise ExecuteFailure("invalid_capture", "Invalid digital sample payload")
        with archive.open(members[0]) as stream:
            preview = stream.read(256 * unitsize)
        return {
            "actual_samples": size // unitsize,
            "unitsize": unitsize,
            "sample_rate_text": device.get("samplerate", "unknown"),
            "preview": [
                int.from_bytes(preview[i : i + unitsize], "little")
                for i in range(0, len(preview), unitsize)
            ],
        }


class SigrokIntegration:
    """@brief Run acquisition and saved-capture analysis in an isolated process."""

    name = "sigrok"

    def __init__(self, backend: str = "/usr/bin/sigrok-cli", simulation: bool = False):
        self.backend = backend
        self.simulation = simulation
        self.processes: set[asyncio.subprocess.Process] = set()

    async def describe(self) -> DescribeResult:
        await self.health()
        return DescribeResult(
            status="ok",
            integration=self.name,
            operations=["discover_devices", "capture", "decode_uart"],
        )

    async def discover(self) -> DiscoverResult:
        return DiscoverResult(status="ok")

    async def health(self) -> HealthResult:
        if not self.simulation:
            await self.command(["--version"])
        return HealthResult(status="ok")

    async def configure(self, request: ConfigureRequest) -> ConfigureResult:
        parameters(request.configuration, set())
        return ConfigureResult(status="ok")

    async def cancel(self, request_id: UUID) -> None:
        del request_id  # SDK cancels the correlated handler task and its finally cleanup.

    async def safe_shutdown(self) -> None:
        for process in tuple(self.processes):
            if process.returncode is None:
                process.kill()
            await process.wait()

    async def command(self, arguments: list[str]) -> str:
        """@brief Bound child output and lifetime; await cleanup before replying."""
        if not Path(self.backend).is_absolute():
            raise ExecuteFailure("runtime_unavailable", "Runtime path must be absolute")
        process = await asyncio.create_subprocess_exec(
            self.backend,
            *arguments,
            stdout=asyncio.subprocess.PIPE,
            stderr=asyncio.subprocess.PIPE,
        )
        self.processes.add(process)

        async def read(stream: asyncio.StreamReader) -> bytes:
            data = bytearray()
            while chunk := await stream.read(8192):
                data.extend(chunk)
                if len(data) > 1 << 19:
                    raise ExecuteFailure("output_limit", "Instrument reply exceeded its limit")
            return bytes(data)

        stdout = asyncio.create_task(read(process.stdout))
        stderr = asyncio.create_task(read(process.stderr))
        try:
            async with asyncio.timeout(30):
                output, diagnostics = await asyncio.gather(stdout, stderr)
                code = await process.wait()
            if code:
                raise ExecuteFailure("backend_failed", diagnostics.decode(errors="replace")[-2000:])
            return output.decode("utf-8", errors="replace")
        finally:
            if process.returncode is None:
                process.kill()
            await process.wait()
            stdout.cancel()
            stderr.cancel()
            await asyncio.gather(stdout, stderr, return_exceptions=True)
            self.processes.discard(process)

    async def execute(self, request: ExecuteRequest, events: EventSink) -> ExecuteResult:
        if request.operation == "discover_devices":
            parameters(request.parameters, set())
            output = (
                "atk-dl16:conn=1.3 - ALIENTEK ATK DL16 with 18 channels: D0 PWM0 PWM1"
                if self.simulation
                else await self.command(["--driver", "atk-dl16", "--scan"])
            )
            return ExecuteResult(
                status="completed",
                data={"devices": parse_devices(output), "simulation": self.simulation},
            )
        if request.operation == "capture":
            return await self.capture(request.parameters, events)
        if request.operation == "decode_uart":
            return await self.decode_uart(request.parameters)
        raise ExecuteFailure("unknown_operation", "Unsupported sigrok operation")

    async def capture(self, value: object, events: EventSink) -> ExecuteResult:
        """@brief Capture finite digital samples into a newly created staged artifact."""
        data = capture_parameters(value)
        path = Path(data["staging_directory"]) / "capture.sr"
        if path.exists():
            raise ExecuteFailure("staging_conflict", "Capture destination already exists")
        await events.emit(
            Progress(fraction=0, message="Acquiring digital samples", stage="capture")
        )
        if self.simulation:
            await asyncio.sleep(0.15)
            metadata = (
                f"[global]\nsigrok version=0.6.0\n[device 1]\ncapturefile=logic-1\n"
                f"total probes=16\nsamplerate={data['samplerate']} Hz\nunitsize=2\n"
                + "".join(f"probe{i + 1}=D{i}\n" for i in range(16))
            )
            with zipfile.ZipFile(path, "w", zipfile.ZIP_DEFLATED) as archive:
                archive.writestr("version", "2")
                archive.writestr("metadata", metadata)
                archive.writestr(
                    "logic-1-1",
                    b"".join(struct.pack("<H", (i // 8) & 0xFFFF) for i in range(data["samples"])),
                )
            runtime = "NILO deterministic simulation"
        else:
            runtime = await self.command(["--version"])
            await self.command(
                [
                    "--driver",
                    f"atk-dl16:conn={data['conn']}",
                    "--config",
                    f"samplerate={data['samplerate']}",
                    "--channels",
                    ",".join(data["channels"]),
                    "--samples",
                    str(data["samples"]),
                    "--output-format",
                    "srzip",
                    "--output-file",
                    str(path),
                ]
            )
        metadata = read_capture(path)
        if metadata["actual_samples"] != data["samples"]:
            raise ExecuteFailure("incomplete_capture", "Actual sample count differs from request")
        digest = hashlib.sha256(path.read_bytes()).hexdigest()
        await events.emit(
            ArtifactDeclaration(
                id="capture",
                media_type="application/x-sigrok-session",
                size_bytes=path.stat().st_size,
                digest=f"sha256:{digest}",
            )
        )
        await events.emit(Progress(fraction=1, message="Capture saved", stage="complete"))
        return ExecuteResult(
            status="completed",
            data={
                **metadata,
                "channels": data["channels"],
                "samplerate": data["samplerate"],
                "connection": data["conn"],
                "runtime": runtime,
                "simulation": self.simulation,
            },
        )

    async def decode_uart(self, value: object) -> ExecuteResult:
        """@brief Return bounded original UART annotations from a saved capture."""
        data = parameters(value, {"input_path", "channel", "baudrate"})
        if data.get("channel") not in DIGITAL_CHANNELS:
            raise invalid("Select a digital UART channel")
        if type(data.get("baudrate")) is not int or not 1 <= data["baudrate"] <= 4_000_000:
            raise invalid("Invalid baud rate")
        path = Path(data.get("input_path", ""))
        if not path.is_absolute():
            raise invalid("A core-owned input artifact is required")
        read_capture(path)
        output = (
            "0-79 uart: data: simulated byte 0x55\n"
            if self.simulation
            else await self.command(
                [
                    "--input-file",
                    str(path),
                    "--protocol-decoders",
                    f"uart:rx={data['channel']}:baudrate={data['baudrate']}",
                    "--protocol-decoder-samplenum",
                    "--protocol-decoder-ann-class",
                ]
            )
        )
        annotations = []
        for line in output.splitlines():
            match = ANNOTATION.match(line)
            if not match:
                if line.strip():
                    raise ExecuteFailure("invalid_reply", "Unexpected decoder annotation format")
                continue
            annotations.append({"start_sample": match[1], "end_sample": match[2], "text": match[3]})
        return ExecuteResult(
            status="completed",
            data={
                "annotations": annotations[:1000],
                "truncated": len(annotations) > 1000,
                "simulation": self.simulation,
            },
        )


# END OF FILE
