#!/usr/bin/env python3
# -*- coding: utf-8 -*-
# -----------------------------------------------------------------------------
# Copyright (c) 2026 DDP Engineering
# SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
# -----------------------------------------------------------------------------
"""
@file    test_tools_commands.py
@brief   Exercise public tool routing without setup, containers or host changes.
@details Staged script boundaries prove refusals precede mutation, child exits
         propagate and shell/WSL arguments retain their original boundaries.
"""

from __future__ import annotations

# IMPORTS
import json
import os
import shutil
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

# CONSTANTS
TOOLS = Path(__file__).resolve().parents[2]
STUB = """import json, os, sys
from pathlib import Path
with Path(os.environ['NILO_TOOL_TEST_LOG']).open('a') as log:
    log.write(json.dumps([sys.argv[1], *sys.argv[2:]]) + '\\n')
raise SystemExit(int(os.environ.get('NILO_TOOL_TEST_EXIT', '0')))
"""


# TEST FIXTURES
class CommandFixture(unittest.TestCase):
    """@brief Own an isolated repository and recording-only command boundaries."""

    def setUp(self) -> None:
        self.temporary = tempfile.TemporaryDirectory(prefix="nilo tools ")
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        self.tools = self.root / "02_Tools"
        self.tools.mkdir()
        self.log = self.root / "calls.jsonl"
        self.stub = self.root / "record.py"
        self.stub.write_text(STUB, encoding="utf-8")
        self.env = {
            **os.environ,
            "NILO_TOOL_TEST_LOG": str(self.log),
            "NILO_TOOL_TEST_STUB": str(self.stub),
            "NILO_TOOL_TEST_PYTHON": sys.executable,
        }

    def calls(self) -> list[list[str]]:
        return (
            [json.loads(line) for line in self.log.read_text().splitlines()]
            if self.log.exists()
            else []
        )

    def shell_stub(self, relative: str) -> None:
        """@brief Record one subprocess's argument vector and configured exit."""
        path = self.tools / relative
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(
            '#!/bin/sh\nexec "$NILO_TOOL_TEST_PYTHON" "$NILO_TOOL_TEST_STUB" "$0" "$@"\n',
            encoding="utf-8",
        )
        path.chmod(0o755)


@unittest.skipIf(os.name == "nt", "POSIX routing runs inside the Linux offline gate")
class LinuxCommands(CommandFixture):
    def setUp(self) -> None:
        super().setUp()
        shutil.copyfile(TOOLS / "nilo-tools.sh", self.tools / "nilo-tools.sh")
        for path in (
            "bootstrap/bootstrap.sh",
            "bootstrap/bootstrap-build-environment.sh",
            "env/with-build-environment.sh",
            "env/with-tools.sh",
            "scripts/doctor.sh",
            "tests/network/check.sh",
            "tests/appliance/check.sh",
            "tests/packaging/check.sh",
        ):
            self.shell_stub(path)

    def run_tools(self, *args: str) -> subprocess.CompletedProcess[str]:
        return subprocess.run(
            ["sh", str(self.tools / "nilo-tools.sh"), *args],
            env=self.env,
            text=True,
            capture_output=True,
            check=False,
        )

    def test_help_requires_no_runtime_or_engine(self) -> None:
        result = self.run_tools()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("package TARGET", result.stdout)
        self.assertEqual(self.calls(), [])

    def test_invalid_requests_have_no_side_effects(self) -> None:
        for arguments in (
            ("erase",),
            ("setup", "--offline", "--rebuild-from-pinned-inputs"),
            ("setup", "--install-engine"),
            ("setup", "--archive-path", "--offline"),
            ("build", "--install-engine"),
            ("build", "extra"),
            ("test", "unknown"),
            ("package", "unknown"),
            ("package", "release", "one"),
            ("test", "--in-container"),
            ("build", "--in-container"),
            ("test", "network", "--engine", "podman"),
            ("dev", "backend", "--data-dir", "/mnt/c/state"),
            ("dev", "backend", "--data-dir", "/home/../../mnt/c/state"),
        ):
            with self.subTest(arguments=arguments):
                result = self.run_tools(*arguments)
                self.assertEqual(result.returncode, 2, result.stderr)
                self.assertEqual(self.calls(), [])

    def test_setup_all_preserves_explicit_offline_and_archive_paths(self) -> None:
        result = self.run_tools(
            "setup", "--offline", "--archive-path", "/local archive/parts"
        )
        self.assertEqual(result.returncode, 0, result.stderr)
        calls = self.calls()
        self.assertEqual(calls[0][1:], ["--offline"])
        self.assertEqual(
            calls[1][1:],
            [
                "--engine",
                "docker",
                "--offline",
                "--archive-path",
                "/local archive/parts",
            ],
        )

    def test_linux_development_keeps_resolved_state_on_linux(self) -> None:
        (self.root / "05_Software/core").mkdir(parents=True)
        link = self.root / "windows state"
        link.symlink_to("/mnt/c/state")
        refused = self.run_tools("dev", "backend", "--data-dir", str(link))
        self.assertEqual(refused.returncode, 2, refused.stderr)
        self.assertEqual(self.calls(), [])
        state = self.root / "linux state"
        result = self.run_tools(
            "dev",
            "backend",
            "--data-dir",
            str(state),
            "--backend-port",
            "8190",
            "--scenario",
            "unavailable",
        )
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(
            self.calls()[0][1:],
            [
                "go",
                "run",
                "./cmd/nilo",
                "serve",
                "--profile",
                "simulation",
                "--data-dir",
                str(state),
                "--bind",
                "127.0.0.1:8190",
                "--scenario",
                "unavailable",
                "--development-administrator",
            ],
        )

    def test_failed_host_bootstrap_stops_setup_before_container(self) -> None:
        self.env["NILO_TOOL_TEST_EXIT"] = "17"
        result = self.run_tools("setup")
        self.assertEqual(result.returncode, 17)
        self.assertEqual(len(self.calls()), 1)

    def test_canonical_routes_and_child_exit_codes(self) -> None:
        self.env["NILO_TOOL_TEST_EXIT"] = "23"
        for arguments, tail in (
            (("test",), ["--engine", "docker", "test"]),
            (("build", "--engine", "podman"), ["--engine", "podman", "build"]),
            (
                ("package", "rpi-debian13-arm64"),
                ["--engine", "docker", "package", "rpi-debian13-arm64"],
            ),
        ):
            with self.subTest(arguments=arguments):
                self.assertEqual(self.run_tools(*arguments).returncode, 23)
                self.assertEqual(self.calls()[-1][1:], tail)

    def test_focused_tools_check_does_not_expand_test_glob(self) -> None:
        result = self.run_tools("test", "tools")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(
            self.calls()[0][1:],
            [
                "--engine",
                "docker",
                "run",
                "python",
                "-m",
                "unittest",
                "discover",
                "-s",
                "02_Tools/tests/software",
                "-p",
                "test_*.py",
                "-v",
            ],
        )

    def test_release_paths_remain_individual_arguments(self) -> None:
        paths = [
            "/input one",
            "/plan.json",
            "/source offers",
            "/ubuntu receipt",
            "/pi receipt",
            "/output's release",
        ]
        result = self.run_tools("package", "release", *paths)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.calls()[0][-6:], paths)

    def test_go_only_wrapper_needs_no_python_or_node_installation(self) -> None:
        shutil.copyfile(TOOLS / "env/with-tools.sh", self.tools / "env/with-tools.sh")
        (self.tools / "toolchain.versions").write_text(
            "GO_VERSION=1.27.0\nUV_VERSION=0.11.25\n", encoding="utf-8"
        )
        self.shell_stub("runtime/linux-x86_64/go/1.27.0/go/bin/go")
        result = subprocess.run(
            ["sh", str(self.tools / "env/with-tools.sh"), "go", "test", "a path"],
            env=self.env,
            text=True,
            capture_output=True,
            check=False,
        )
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.calls()[0][-2:], ["test", "a path"])

    def test_container_run_preserves_literal_shell_characters(self) -> None:
        """@brief User arguments survive container dispatch without shell evaluation."""
        shutil.copyfile(
            TOOLS / "env/with-build-environment.sh",
            self.tools / "env/with-build-environment.sh",
        )
        archive = self.tools / "archives/build-environment"
        archive.mkdir(parents=True)
        (archive / "current.json").write_text(
            json.dumps(
                {
                    "generation": "fixture",
                    "image": {
                        "reference": "fixture:local",
                        "digest": "sha256:" + "a" * 64,
                    },
                }
            ),
            encoding="utf-8",
        )
        (self.root / ".git").mkdir()
        binary = self.root / "bin"
        binary.mkdir()
        skopeo = binary / "skopeo"
        skopeo.write_text(
            '#!/bin/sh\nprintf "%s\\n" "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"\n',
            encoding="utf-8",
        )
        skopeo.chmod(0o755)
        engine = binary / "docker"
        engine.write_text(
            f'#!{sys.executable}\nimport subprocess,sys\nraise SystemExit(subprocess.call(["sh", "-c", sys.argv[-2]]))\n',
            encoding="utf-8",
        )
        engine.chmod(0o755)
        self.env["PATH"] = str(binary) + os.pathsep + self.env["PATH"]
        marker = self.root / "unexpected"
        values = [
            "hello world",
            "a'b",
            f"$(touch '{marker}')",
            f"; touch '{marker}'",
            "line\nbreak",
        ]
        result = subprocess.run(
            [
                "sh",
                str(self.tools / "env/with-build-environment.sh"),
                "run",
                sys.executable,
                "-c",
                "import json,sys; print(json.dumps(sys.argv[1:]))",
                *values,
            ],
            env=self.env,
            text=True,
            capture_output=True,
            check=False,
        )
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(json.loads(result.stdout.splitlines()[-1]), values)
        self.assertFalse(marker.exists())


@unittest.skipUnless(
    os.name == "nt", "Windows command routing needs PowerShell on Windows"
)
class WindowsCommands(CommandFixture):
    def setUp(self) -> None:
        super().setUp()
        self.pwsh = shutil.which("pwsh")
        if not self.pwsh:
            self.skipTest("PowerShell is unavailable")
        shutil.copyfile(TOOLS / "nilo-tools.ps1", self.tools / "nilo-tools.ps1")
        (self.tools / "lib").mkdir()
        shutil.copyfile(TOOLS / "lib/Wsl.ps1", self.tools / "lib/Wsl.ps1")
        # Registry lookup is a platform boundary; route using a distinct backing
        # volume so the regression cannot accidentally reuse the checkout path.
        with (self.tools / "lib/Wsl.ps1").open("a", encoding="utf-8") as library:
            library.write(
                "\nfunction Get-NiloWslHostVolume { param($WslExecutable, $Distribution); '/mnt/backing-volume' }\n"
            )
        self.wsl = self.root / "fake-wsl.ps1"
        self.wsl.write_text(
            """
Add-Content -LiteralPath $env:NILO_TOOL_TEST_LOG -Value (ConvertTo-Json -Compress -InputObject @('wsl', $args))
if ($args[0] -eq '--list') { Write-Output @('Ubuntu', 'docker-desktop', 'docker-desktop-data'); exit 0 }
if ($args -contains 'wslpath') { Write-Output '/linux tools'; exit 0 }
exit ([int]$env:NILO_TOOL_TEST_EXIT)
""",
            encoding="utf-8",
        )
        self.env["NILO_TOOL_TEST_EXIT"] = "0"
        for relative in (
            "bootstrap/bootstrap-development.ps1",
            "bootstrap/bootstrap-build-environment.ps1",
            "scripts/nilo-dev.ps1",
            "scripts/doctor.ps1",
            "scripts/doctor-wsl.ps1",
        ):
            path = self.tools / relative
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text(
                "Add-Content -LiteralPath $env:NILO_TOOL_TEST_LOG -Value (ConvertTo-Json -Compress -InputObject @($PSCommandPath, $args))\nexit ([int]$env:NILO_TOOL_TEST_EXIT)\n",
                encoding="utf-8",
            )

    def run_tools(self, *args: str) -> subprocess.CompletedProcess[str]:
        return subprocess.run(
            [
                self.pwsh,
                "-NoProfile",
                "-File",
                str(self.tools / "nilo-tools.ps1"),
                *args,
            ],
            env=self.env,
            text=True,
            capture_output=True,
            check=False,
        )

    def test_help_and_invalid_requests_do_not_launch_children(self) -> None:
        self.assertEqual(self.run_tools().returncode, 0)
        for args in (
            ("build", "-InstallEngine"),
            ("setup", "-Offline", "-RebuildFromPinnedInputs"),
            ("setup", "-InstallEngine"),
            ("test", "unknown"),
            ("dev", "stop"),
            ("package", "unknown"),
            ("package", "release", "one"),
        ):
            with self.subTest(args=args):
                result = self.run_tools(*args)
                self.assertEqual(result.returncode, 2, result.stdout + result.stderr)
                self.assertEqual(self.calls(), [])

    def test_default_test_translates_to_shared_linux_command(self) -> None:
        self.env["NILO_TOOL_TEST_EXIT"] = "19"
        result = self.run_tools("test", "-WslExecutable", str(self.wsl))
        self.assertEqual(result.returncode, 19, result.stdout + result.stderr)
        args = self.calls()[-1][1]
        self.assertEqual(
            args,
            [
                "-d",
                "Ubuntu",
                "--exec",
                "env",
                "NILO_WSL_HOST_VOLUME=/mnt/backing-volume",
                "sh",
                "/linux tools/nilo-tools.sh",
                "test",
                "--engine",
                "docker",
            ],
        )

    def test_setup_does_not_continue_after_child_failure(self) -> None:
        self.env["NILO_TOOL_TEST_EXIT"] = "17"
        result = self.run_tools("setup", "-Offline")
        self.assertEqual(result.returncode, 17, result.stdout + result.stderr)
        self.assertEqual(len(self.calls()), 1)

    def test_package_preparation_takes_no_release_paths(self) -> None:
        result = self.run_tools(
            "package",
            "ubuntu-24.04-amd64",
            "-Distribution",
            "Ubuntu",
            "-WslExecutable",
            str(self.wsl),
        )
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(
            self.calls()[-1][1][-4:],
            ["package", "ubuntu-24.04-amd64", "--engine", "docker"],
        )

    def test_release_translates_each_windows_path_once(self) -> None:
        paths = [
            str(self.root / name)
            for name in (
                "artifacts",
                "acceptance.json",
                "source offers",
                "ubuntu receipt.json",
                "pi receipt.json",
                "release output",
            )
        ]
        result = self.run_tools(
            "package",
            "release",
            *paths,
            "-Distribution",
            "Ubuntu",
            "-WslExecutable",
            str(self.wsl),
        )
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(
            self.calls()[-1][1][-8:], ["package", "release", *["/linux tools"] * 6]
        )
        translated = [call[1][-1] for call in self.calls() if "wslpath" in call[1]]
        self.assertEqual(translated[1:], paths)

    def test_dev_preserves_original_lifecycle_and_options(self) -> None:
        result = self.run_tools(
            "dev", "status", "-Distribution", "Ubuntu", "-BackendPort", "9090"
        )
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        path, args = self.calls()[0]
        self.assertTrue(path.endswith("nilo-dev.ps1"))
        self.assertEqual(args[args.index("-Command") + 1], "status")
        self.assertEqual(args[args.index("-BackendPort") + 1], "9090")


# END OF FILE
if __name__ == "__main__":
    unittest.main()
