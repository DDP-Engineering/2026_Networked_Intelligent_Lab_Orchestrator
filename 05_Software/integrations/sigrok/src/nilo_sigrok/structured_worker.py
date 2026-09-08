#!/usr/bin/env python3
# -*- coding: utf-8 -*-
# -----------------------------------------------------------------------------
# Copyright (c) 2026 DDP Engineering
# SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
# -----------------------------------------------------------------------------
"""@file    structured_worker.py
@brief   Bound and validate the native Sigrok feasibility worker.
@details Keeps native library faults below the existing isolated Python runner
and admits only the versioned, output-free feasibility record contract.
"""

# IMPORTS
from __future__ import annotations

import asyncio
import json
import re
from dataclasses import dataclass
from typing import Any

# CONSTANTS
WORKER_SCHEMA = "nilo.sigrok.worker/1"
ALLOWED_KINDS = {
    "runtime",
    "driver",
    "device",
    "channel",
    "channel_group",
    "setting",
    "packet",
    "event",
    "decoder",
    "annotation",
    "binary",
    "summary",
    "error",
}
OUTPUT_TOKEN = re.compile(
    r"(?:^|[^a-z0-9])(pwm\d*|signal[_ -]?generator|output)(?:$|[^a-z0-9])", re.IGNORECASE
)


# DATA MODELS / TYPEDEFINITIONS
class WorkerFailure(RuntimeError):
    """@brief Report a rejected or failed native worker invocation."""


@dataclass(frozen=True)
class WorkerLimits:
    """@brief Resource limits for one worker invocation."""

    timeout_seconds: float = 10.0
    max_records: int = 256
    max_line_bytes: int = 4096
    max_output_bytes: int = 1 << 20
    max_diagnostics_bytes: int = 64 << 10


# HELPER FUNCTIONS
def _contains_output_capability(value: Any) -> bool:
    """@brief Search structured values for explicitly excluded output tokens."""
    if isinstance(value, str):
        return OUTPUT_TOKEN.search(value) is not None
    if isinstance(value, dict):
        return any(
            _contains_output_capability(key) or _contains_output_capability(item)
            for key, item in value.items()
        )
    if isinstance(value, list):
        return any(_contains_output_capability(item) for item in value)
    return False


def validate_records(records: list[dict], limits: WorkerLimits | None = None) -> list[dict]:
    """@brief Validate one complete feasibility response.
    @param[in] records Parsed JSON records in emitted order.
    @param[in] limits Optional resource limits.
    @return The same records after validation.
    @exception WorkerFailure The stream violates the worker contract.
    """
    limits = limits or WorkerLimits()
    if not records or len(records) > limits.max_records:
        raise WorkerFailure("worker record limit exceeded or stream is empty")
    encoded_size = 0
    summaries = 0
    for index, record in enumerate(records):
        if not isinstance(record, dict):
            raise WorkerFailure("worker record is not an object")
        encoded = json.dumps(record, separators=(",", ":"), ensure_ascii=False).encode("utf-8")
        encoded_size += len(encoded) + 1
        if len(encoded) > limits.max_line_bytes or encoded_size > limits.max_output_bytes:
            raise WorkerFailure("worker output limit exceeded")
        if record.get("schema") != WORKER_SCHEMA:
            raise WorkerFailure("worker schema is unsupported")
        kind = record.get("kind")
        if kind not in ALLOWED_KINDS:
            raise WorkerFailure("worker record kind is unsupported")
        if _contains_output_capability(record):
            raise WorkerFailure("worker exposed an output capability")
        if kind == "summary":
            summaries += 1
            if index != len(records) - 1:
                raise WorkerFailure("worker summary is not terminal")
    if summaries != 1:
        raise WorkerFailure("worker must emit one terminal summary")
    return records


async def _read_limited(stream: asyncio.StreamReader, limit: int) -> bytes:
    """@brief Read a child stream while enforcing a hard byte budget."""
    value = bytearray()
    while chunk := await stream.read(8192):
        value.extend(chunk)
        if len(value) > limit:
            raise WorkerFailure("worker output limit exceeded")
    return bytes(value)


# PUBLIC API
class StructuredWorker:
    """@brief Execute one native worker command in a contained child process."""

    def __init__(self, command: list[str], limits: WorkerLimits | None = None):
        if not command or not all(isinstance(item, str) and item for item in command):
            raise ValueError("A non-empty executable command is required")
        self.command = tuple(command)
        self.limits = limits or WorkerLimits()
        self.process: asyncio.subprocess.Process | None = None

    async def run(self, arguments: list[str]) -> list[dict]:
        """@brief Execute and validate one fixed worker operation.
        @param[in] arguments Allowlisted arguments selected by the integration.
        @return Validated protocol records.
        @exception WorkerFailure Timeout, resource, protocol, or process failure.
        """
        if self.process is not None:
            raise WorkerFailure("worker invocation is already active")
        if not all(isinstance(item, str) and item for item in arguments):
            raise WorkerFailure("worker arguments are invalid")
        self.process = await asyncio.create_subprocess_exec(
            *self.command,
            *arguments,
            stdout=asyncio.subprocess.PIPE,
            stderr=asyncio.subprocess.PIPE,
        )
        process = self.process
        stdout = asyncio.create_task(_read_limited(process.stdout, self.limits.max_output_bytes))
        stderr = asyncio.create_task(
            _read_limited(process.stderr, self.limits.max_diagnostics_bytes)
        )
        try:
            try:
                async with asyncio.timeout(self.limits.timeout_seconds):
                    output, diagnostics = await asyncio.gather(stdout, stderr)
                    code = await process.wait()
            except TimeoutError as error:
                raise WorkerFailure("worker timeout exceeded") from error
            if code != 0:
                message = diagnostics.decode("utf-8", errors="replace").strip()[-1000:]
                raise WorkerFailure(f"worker exit {code}: {message}".rstrip())
            parsed = []
            for raw_line in output.splitlines():
                if len(raw_line) > self.limits.max_line_bytes:
                    raise WorkerFailure("worker output line limit exceeded")
                try:
                    parsed.append(json.loads(raw_line))
                except (UnicodeDecodeError, json.JSONDecodeError) as error:
                    raise WorkerFailure("worker emitted malformed JSON") from error
            return validate_records(parsed, self.limits)
        finally:
            if process.returncode is None:
                process.kill()
            await process.wait()
            stdout.cancel()
            stderr.cancel()
            await asyncio.gather(stdout, stderr, return_exceptions=True)
            self.process = None


# END OF FILE
