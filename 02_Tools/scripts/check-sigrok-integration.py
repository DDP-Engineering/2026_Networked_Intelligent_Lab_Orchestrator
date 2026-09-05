#!/usr/bin/env python3
# -*- coding: utf-8 -*-
# -----------------------------------------------------------------------------
# Copyright (c) 2026 DDP Engineering
# SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
# -----------------------------------------------------------------------------
"""@file    check-sigrok-integration.py
@brief   Run an isolated appliance integration smoke without qualifying signal accuracy.
@details The supplied runtime root must be new. Credentials are generated in memory;
the fixture's TLS certificate is explicitly trusted. No installed service is stopped.
"""

# IMPORTS
from __future__ import annotations

import argparse
import http.cookiejar
import json
import secrets
import ssl
import subprocess
import time
import urllib.error
import urllib.request
from pathlib import Path


def main() -> None:
    """@brief Exercise package lifecycle, HTTP/CLI/MCP jobs and evidence."""
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ["root", "binary", "bundles", "gui", "cert", "key"]:
        parser.add_argument("--" + name, type=Path, required=True)
    parser.add_argument("--host", default="ddp-remote.local")
    parser.add_argument("--bind", default="192.168.1.103:8445")
    args = parser.parse_args()
    args.root.mkdir(parents=True, exist_ok=False)
    generated_password = secrets.token_urlsafe(32)
    subprocess.run(
        [
            str(args.binary),
            "admin",
            "bootstrap",
            "--data-dir",
            str(args.root),
            "--username",
            "integration-test",
        ],
        input=f"{generated_password}\n{generated_password}\n".encode(),
        stdout=subprocess.DEVNULL,
        check=True,
    )
    command = [
        str(args.binary),
        "serve",
        "--profile",
        "appliance",
        "--data-dir",
        str(args.root),
        "--bind",
        args.bind,
        "--gui-dir",
        str(args.gui),
        "--tls-cert",
        str(args.cert),
        "--tls-key",
        str(args.key),
        "--integration-bundles",
        str(args.bundles),
    ]
    base = f"https://{args.host}:{args.bind.rsplit(':', 1)[1]}"
    opener = urllib.request.build_opener(
        urllib.request.HTTPSHandler(
            context=ssl.create_default_context(cafile=str(args.cert))
        ),
        urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()),
    )
    csrf = ""

    def request(path: str, body: dict | None = None, token: str = "") -> dict:
        """@brief Make one authenticated, bounded request without printing secrets."""
        headers = {"Content-Type": "application/json", "X-NILO-CSRF": csrf}
        if token:
            headers["Authorization"] = "Bearer " + token
        data = None if body is None else json.dumps(body).encode()
        with opener.open(
            urllib.request.Request(base + path, data=data, headers=headers), timeout=15
        ) as response:
            return json.load(response)

    def wait(job: dict) -> dict:
        """@brief Poll the durable transaction without resubmitting the command."""
        deadline = time.monotonic() + 65
        while time.monotonic() < deadline:
            current = request("/api/v1/bench/jobs/" + job["transaction_id"])
            if current["state"] in ["completed", "failed", "cancelled", "incomplete"]:
                assert current["state"] == "completed", current
                return current
            time.sleep(0.2)
        raise TimeoutError("transaction did not finish")

    with (args.root / "server.log").open("wb") as log:
        server = subprocess.Popen(command, stdout=log, stderr=log)
        try:
            for _ in range(50):
                if server.poll() is not None:
                    raise RuntimeError("fixture exited; inspect server.log")
                try:
                    login = request(
                        "/api/v1/auth/login",
                        {"username": "integration-test", "password": generated_password},
                    )
                    break
                except urllib.error.URLError:
                    time.sleep(0.1)
            else:
                raise TimeoutError("fixture did not start")
            csrf = login["csrf_token"]
            request("/api/v1/plugins/sigrok/install", {})
            request("/api/v1/plugins/sigrok/enable", {"mode": "ask"})
            devices = request("/api/v1/plugins/sigrok/discover", {})["devices"]
            assert devices and devices[0]["channels"] == [f"D{i}" for i in range(16)]
            input_value = {
                "plugin_id": "sigrok",
                "operation": "capture",
                "device_id": devices[0]["id"],
                "parameters": {
                    "channels": ["D0", "D1"],
                    "samplerate": 1000000,
                    "samples": 1024,
                },
                "idempotency_key": "smoke-approval",
                "timeout_seconds": 60,
            }
            pending = request("/api/v1/bench/jobs", input_value)
            assert pending["state"] == "pending_approval"
            request(f"/api/v1/bench/jobs/{pending['transaction_id']}/approve", {})
            finished = wait(pending)
            repeated = request("/api/v1/bench/jobs", input_value)
            assert repeated["transaction_id"] == finished["transaction_id"]
            assert (
                finished["artifact_id"] and finished["result"]["actual_samples"] == 1024
            )
            assert all(
                reply["transaction_id"] == finished["transaction_id"]
                for reply in finished["replies"]
            )
            with opener.open(
                base + f"/api/v1/bench/jobs/{finished['transaction_id']}/artifact"
            ) as response:
                assert response.read(2) == b"PK"
            request("/api/v1/plugins/sigrok/disable", {})
            request("/api/v1/plugins/sigrok/enable", {"mode": "auto"})
            issued = request(
                "/api/v1/tokens",
                {
                    "owner": "integration-test",
                    "name": "integration-check",
                    "scopes": ["bench.read", "bench.execute", "plugins.manage"],
                    "expires_at": None,
                },
            )
            token_file = args.root / "client-token"
            token_file.write_text(issued["secret"])
            token_file.chmod(0o600)
            common = [
                "--server",
                base,
                "--token-file",
                str(token_file),
                "--ca-file",
                str(args.cert),
                "--json",
            ]
            cli_output = subprocess.check_output(
                [
                    str(args.binary),
                    "bench",
                    "capture",
                    "--device",
                    devices[0]["id"],
                    "--channels",
                    "D0,D1",
                    "--key",
                    "cli-smoke-capture",
                    *common,
                ]
            )
            cli_job = wait(json.loads(cli_output))
            subprocess.run(
                [
                    str(args.binary),
                    "bench",
                    "download",
                    "--id",
                    cli_job["transaction_id"],
                    "--out",
                    str(args.root / "cli-capture.sr"),
                    *common,
                ],
                check=True,
                stdout=subprocess.DEVNULL,
            )
            mcp_session = ""

            def rpc(method: str, params: dict, sequence: int | None) -> dict:
                """@brief Exchange a bounded MCP request with the current scoped token."""
                nonlocal mcp_session
                payload = {"jsonrpc": "2.0", "method": method, "params": params}
                if sequence is not None:
                    payload["id"] = sequence
                headers = {
                    "Authorization": "Bearer " + issued["secret"],
                    "Content-Type": "application/json",
                    "Accept": "application/json, text/event-stream",
                    "MCP-Protocol-Version": "2025-03-26",
                }
                if mcp_session:
                    headers["Mcp-Session-Id"] = mcp_session
                with opener.open(
                    urllib.request.Request(
                        base + "/mcp",
                        data=json.dumps(payload).encode(),
                        headers=headers,
                    ),
                    timeout=15,
                ) as response:
                    mcp_session = response.headers.get("Mcp-Session-Id", mcp_session)
                    raw = response.read(2 << 20).decode()
                if not raw:
                    return {}
                if raw.lstrip().startswith("{"):
                    return json.loads(raw)
                return json.loads(
                    next(
                        line[5:].strip()
                        for line in raw.splitlines()
                        if line.startswith("data:")
                    )
                )

            rpc(
                "initialize",
                {
                    "protocolVersion": "2025-03-26",
                    "capabilities": {},
                    "clientInfo": {"name": "nilo-integration-check", "version": "1"},
                },
                1,
            )
            rpc("notifications/initialized", {}, None)
            tools = rpc("tools/list", {}, 2)["result"]["tools"]
            assert any(tool["name"] == "bench_submit" for tool in tools)
            input_value["idempotency_key"] = "mcp-smoke-capture"
            response = rpc(
                "tools/call", {"name": "bench_submit", "arguments": input_value}, 3
            )["result"]
            assert not response.get("isError"), response
            mcp_job = wait(json.loads(response["content"][0]["text"]))
            reply = rpc(
                "tools/call",
                {
                    "name": "bench_get_job",
                    "arguments": {"transaction_id": mcp_job["transaction_id"]},
                },
                4,
            )["result"]
            assert json.loads(reply["content"][0]["text"])["state"] == "completed"
            token_file.unlink()
            request("/api/v1/plugins/sigrok/restart", {})
            request("/api/v1/plugins/sigrok/uninstall", {})
            assert request(f"/api/v1/bench/jobs/{finished['transaction_id']}")[
                "artifact_id"
            ]
            report = {
                "result": "passed",
                "device": devices[0]["id"],
                "transaction_id": finished["transaction_id"],
                "samples": finished["result"]["actual_samples"],
                "runtime": finished["result"].get("runtime"),
                "checks": [
                    "install",
                    "enable",
                    "digital discovery",
                    "human approval",
                    "finite capture",
                    "idempotent retry",
                    "correlated replies",
                    "artifact download",
                    "disable",
                    "restart",
                    "uninstall retains history",
                    "CLI capture and download",
                    "MCP submit and correlated result",
                ],
                "qualification": "integration smoke only; signal and decoder correctness deferred",
            }
            (args.root / "report.json").write_text(json.dumps(report, indent=2) + "\n")
            print(json.dumps(report, indent=2))
        finally:
            (args.root / "client-token").unlink(missing_ok=True)
            server.terminate()
            try:
                server.wait(timeout=15)
            except subprocess.TimeoutExpired:
                server.kill()
                server.wait()


if __name__ == "__main__":
    main()

# END OF FILE
