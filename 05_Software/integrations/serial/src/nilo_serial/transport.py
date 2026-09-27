#!/usr/bin/env python3
# -*- coding: utf-8 -*-
# -----------------------------------------------------------------------------
# Copyright (c) 2026 DDP Engineering
# SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
# -----------------------------------------------------------------------------
"""@file transport.py
@brief Own one exclusive Linux serial descriptor and bounded event ring.
@details Writes report only bytes accepted by the OS; they are never retried.
"""

from __future__ import annotations

import array
import base64
import errno
import fcntl
import json
import os
import select
import sys
import termios
import threading
import time
from collections import deque
from datetime import UTC, datetime

import serial

from nilo_serial.settings import validate_settings


class SerialError(ValueError):
    """@brief Carry a stable private-boundary error without host details."""


class EventRing:
    """@brief Bound reader lag by raw bytes and event count with explicit gaps."""

    def __init__(self):
        self.events = deque()
        self.byte_count = 0
        self.sequence = 0
        self.started = time.monotonic()
        self.condition = threading.Condition()

    def append(self, direction: str, data: bytes = b"", message: str = "") -> None:
        """@brief Append at most one 4096-byte event, evicting oldest entries."""
        if len(data) > 4096:
            raise ValueError("invalid_parameters")
        with self.condition:
            self.sequence += 1
            event = {
                "sequence": self.sequence,
                "timestamp": datetime.now(UTC).isoformat().replace("+00:00", "Z"),
                "elapsed_ms": round((time.monotonic() - self.started) * 1000, 3),
                "direction": direction,
                "data_base64": base64.b64encode(data).decode(),
                "byte_count": len(data),
            }
            if message:
                event["message"] = message[:256]
            self.events.append(event)
            self.byte_count += len(data)
            while self.byte_count > 1 << 20 or len(self.events) > 4096:
                self.byte_count -= self.events.popleft()["byte_count"]
            self.condition.notify_all()

    def read(self, after: int, wait_ms: int) -> dict:
        """@brief Return a bounded cursor page; slow readers see an explicit gap."""
        if type(after) is not int or after < 0 or not 0 <= wait_ms <= 10000:
            raise SerialError("invalid_parameters")
        with self.condition:
            if self.sequence <= after and wait_ms:
                self.condition.wait(timeout=wait_ms / 1000)
            gap = bool(self.events and after < self.events[0]["sequence"] - 1)
            result = {"events": [], "next_cursor": after, "gap": gap}
            size = 128
            for event in self.events:
                if event["sequence"] <= after:
                    continue
                encoded_size = len(json.dumps(event).encode()) + 2
                if size + encoded_size > 65000:
                    break
                result["events"].append(event)
                result["next_cursor"] = event["sequence"]
                size += encoded_size
            return result


class SerialTransport:
    """@brief Run a single read worker with bounded, serialized transmissions."""

    def __init__(self, endpoint: str, settings: dict):
        self.settings = validate_settings(settings)
        if self.settings["stop_bits"] == 1.5 and self.settings["data_bits"] != 5:
            raise SerialError("invalid_parameters")
        self.ring = EventRing()
        self.stop = threading.Event()
        self.write_lock = threading.Lock()
        self.signal_lock = threading.Lock()
        self.queue_lock = threading.Lock()
        self.close_lock = threading.Lock()
        self.queued = 0
        self.warning = ""
        self.port = serial.Serial(
            port=None,
            baudrate=self.settings["baud_rate"],
            bytesize=self.settings["data_bits"],
            parity={"none": "N", "odd": "O", "even": "E", "mark": "M", "space": "S"}[
                self.settings["parity"]
            ],
            stopbits=self.settings["stop_bits"],
            rtscts=self.settings["flow_control"] == "rtscts",
            xonxoff=self.settings["flow_control"] == "xonxoff",
            timeout=0.1,
            write_timeout=2,
            exclusive=True,
        )
        self.port.rts, self.port.dtr = self.settings["rts"], self.settings["dtr"]
        self.port.port = endpoint
        try:
            self.port.open()
            self._verify_settings()
            fcntl.ioctl(self.port.fileno(), termios.TIOCEXCL)
        except SerialError:
            self.port.close()
            raise
        except (OSError, serial.SerialException) as error:
            self.port.close()
            code = (
                "port_busy"
                if getattr(error, "errno", None) in (errno.EBUSY, errno.EAGAIN)
                else "device_unavailable"
            )
            raise SerialError(code) from error
        self.worker = threading.Thread(target=self._receive, name="serial-rx", daemon=True)
        self.worker.start()

    def _verify_settings(self) -> None:
        """Reject drivers that silently coerce approved framing or flow settings."""
        attrs = termios.tcgetattr(self.port.fileno())
        input_flags, control = attrs[0], attrs[2]
        bits = {5: termios.CS5, 6: termios.CS6, 7: termios.CS7, 8: termios.CS8}
        cmspar = getattr(serial.serialposix, "CMSPAR", 0)
        parity = {
            "none": 0,
            "even": termios.PARENB,
            "odd": termios.PARENB | termios.PARODD,
            "mark": termios.PARENB | termios.PARODD | cmspar,
            "space": termios.PARENB | cmspar,
        }[self.settings["parity"]]
        parity_mask = termios.PARENB | termios.PARODD | cmspar
        software = self.settings["flow_control"] == "xonxoff"
        hardware = self.settings["flow_control"] == "rtscts"
        if (
            control & termios.CSIZE != bits[self.settings["data_bits"]]
            or control & parity_mask != parity
            or bool(control & termios.CSTOPB) != (self.settings["stop_bits"] != 1)
            or bool(control & termios.CRTSCTS) != hardware
            or bool(input_flags & termios.IXON) != software
            or bool(input_flags & termios.IXOFF) != software
        ):
            raise SerialError("invalid_parameters")
        speed = getattr(termios, "B" + str(self.settings["baud_rate"]), None)
        if speed is not None and (attrs[4] != speed or attrs[5] != speed):
            raise SerialError("invalid_parameters")
        if speed is None:
            # The same Linux termios2 layout used by pySerial for custom baud rates.
            rates = array.array("i", [0] * 64)
            fcntl.ioctl(self.port.fileno(), serial.serialposix.TCGETS2, rates)
            if rates[9] != self.settings["baud_rate"] or rates[10] != self.settings["baud_rate"]:
                raise SerialError("invalid_parameters")

    def _receive(self) -> None:
        try:
            while not self.stop.is_set():
                data = self.port.read(4096)
                if data:
                    self.ring.append("rx", data)
        except (OSError, serial.SerialException, TypeError):
            if not self.stop.is_set():
                self.warning = "device_unavailable"
                self.ring.append("error", message=self.warning)
                self.stop.set()

    def read(self, after: int, wait_ms: int) -> dict:
        return self.ring.read(after, wait_ms)

    def send(self, data: bytes) -> dict:
        """@brief Write once with a two-second total deadline and exact partial count."""
        if not isinstance(data, bytes) or not 1 <= len(data) <= 4096:
            raise SerialError("invalid_parameters")
        with self.queue_lock:
            if self.queued + len(data) > 65536:
                raise SerialError("port_busy")
            self.queued += len(data)
        written = 0
        deadline = time.monotonic() + 2
        acquired = False
        error = ""
        try:
            acquired = self.write_lock.acquire(timeout=2)
            if not acquired:
                return {"status": "failed", "bytes_written": 0, "error": "port_busy"}
            while written < len(data):
                if self.stop.is_set() or not self.port.is_open:
                    error = "device_unavailable"
                    break
                remaining = deadline - time.monotonic()
                if remaining <= 0:
                    error = "write_timeout"
                    break
                _, ready, _ = select.select([], [self.port.fileno()], [], min(remaining, 0.1))
                if not ready:
                    continue
                try:
                    written += os.write(self.port.fileno(), data[written:])
                except BlockingIOError:
                    continue
            if written:
                self.ring.append("tx", data[:written])
            result = {
                "status": "sent" if written == len(data) else "partial" if written else "failed",
                "bytes_written": written,
            }
            if error:
                result["error"] = error
            return result
        except (OSError, serial.SerialException):
            if written:
                self.ring.append("tx", data[:written])
            return {
                "status": "partial" if written else "failed",
                "bytes_written": written,
                "error": "device_unavailable",
            }
        finally:
            if acquired:
                self.write_lock.release()
            with self.queue_lock:
                self.queued -= len(data)

    def status(self) -> dict:
        """@brief Report unsupported modem inputs and outputs as null."""
        signals = dict.fromkeys(("rts", "dtr", "cts", "dsr", "ri", "cd"))
        with self.signal_lock:
            if self.port.is_open:
                try:
                    # A real ioctl probes support; pySerial output properties only echo state.
                    flags = int.from_bytes(
                        fcntl.ioctl(self.port.fileno(), termios.TIOCMGET, b"\0" * 4),
                        sys.byteorder,
                    )
                    for name, bit in {
                        "rts": termios.TIOCM_RTS,
                        "dtr": termios.TIOCM_DTR,
                        "cts": termios.TIOCM_CTS,
                        "dsr": termios.TIOCM_DSR,
                        "ri": termios.TIOCM_RI,
                        "cd": termios.TIOCM_CD,
                    }.items():
                        signals[name] = bool(flags & bit)
                except (OSError, serial.SerialException):
                    pass
        return {
            "connected": not self.stop.is_set() and self.port.is_open,
            "signals": signals,
            "warning": self.warning,
            "last_sequence": self.ring.sequence,
        }

    def set_signals(self, value: dict) -> dict:
        """@brief Refuse unsupported/manual RTS during hardware flow control."""
        if (
            not value
            or set(value) - {"rts", "dtr"}
            or any(type(v) is not bool for v in value.values())
        ):
            raise SerialError("invalid_parameters")
        if "rts" in value and self.settings["flow_control"] == "rtscts":
            raise SerialError("unsupported_signals")
        with self.signal_lock:
            if not self.port.is_open or self.stop.is_set():
                raise SerialError("device_unavailable")
            try:
                fcntl.ioctl(self.port.fileno(), termios.TIOCMGET, b"\0" * 4)
                for name, state in value.items():
                    setattr(self.port, name, state)
            except (OSError, serial.SerialException) as error:
                raise SerialError("unsupported_signals") from error
        return self.status()

    def close(self) -> None:
        """@brief Stop workers before releasing exclusive ownership of the descriptor."""
        with self.close_lock:
            self.stop.set()
            if not self.port.is_open:
                return
            self.worker.join(timeout=0.3)
            with self.write_lock, self.signal_lock:
                try:
                    fcntl.ioctl(self.port.fileno(), termios.TIOCNXCL)
                except OSError:
                    pass
                self.port.close()


# END OF FILE
