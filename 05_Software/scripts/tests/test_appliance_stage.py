#!/usr/bin/env python3
# -*- coding: utf-8 -*-
# -----------------------------------------------------------------------------
# Copyright (c) 2026 DDP Engineering
# Licensed under the PolyForm Noncommercial License 1.0.0.
# See the repository LICENSE.txt for full license details.
# SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
# -----------------------------------------------------------------------------
"""
@file test_appliance_stage.py
@brief Verify the appliance verification stage's definition and refusals.
@details Exercises the image definition, the prerequisite refusals, and the
         host-isolation contract of 02_Tools/tests/appliance without starting a
         container, building an image, or touching a network.
"""

from __future__ import annotations

import json
import os
import re
import shutil
import subprocess
import tempfile
import unittest
from pathlib import Path

REPOSITORY_ROOT = Path(__file__).resolve().parents[3]
TOOLS_ROOT = REPOSITORY_ROOT / "02_Tools"
STAGE_ROOT = TOOLS_ROOT / "tests" / "appliance"
NETWORK_ROOT = TOOLS_ROOT / "tests" / "network"
NETWORK_CHECK = NETWORK_ROOT / "check.sh"

CHECK = STAGE_ROOT / "check.sh"
RUN = STAGE_ROOT / "run.sh"
PROBE = STAGE_ROOT / "probe.sh"
INVENTORY = STAGE_ROOT / "inventory.json"
DOCKERFILE = NETWORK_ROOT / "Dockerfile"
NETWORK_UP = NETWORK_ROOT / "network-up.sh"
VERSIONS = TOOLS_ROOT / "toolchain.versions"
GUI_MODULES = "/opt/nilo/gui-modules"

EXIT_PREREQUISITE = 2


def _versions() -> dict[str, str]:
    """@brief Read the pinned toolchain records as NAME=VALUE pairs."""
    pinned: dict[str, str] = {}
    for line in VERSIONS.read_text(encoding="utf-8").splitlines():
        line = line.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        name, _, value = line.partition("=")
        pinned[name.strip()] = value.strip()
    return pinned


class ApplianceImageDefinitionTests(unittest.TestCase):
    """REQ-PLAT-32b81548 - the image is pinned and carries what the mount cannot."""

    def test_base_image_is_named_by_digest_and_packages_are_explicit(self) -> None:
        text = DOCKERFILE.read_text(encoding="utf-8")
        bases = re.findall(r"^ARG\s+NILO_BASE=(\S+)", text, re.MULTILINE)
        self.assertTrue(bases, "the image definition declares no NILO_BASE")
        for base in bases:
            self.assertRegex(
                base,
                r"@sha256:[0-9a-f]{64}$",
                f"base image {base!r} must be pinned by content digest, not a tag",
            )
        self.assertIn(
            "--no-install-recommends",
            text,
            "the package list must be explicit rather than pulling recommendations",
        )
        for package in ("network-manager", "dbus", "nftables", "wireguard-tools"):
            self.assertIn(package, text, f"the image must carry {package}")

    def test_node_and_browser_engine_are_pinned_from_the_toolchain_manifest(self) -> None:
        text = DOCKERFILE.read_text(encoding="utf-8")
        pinned = _versions()
        for name in ("NODE_VERSION", "PLAYWRIGHT_VERSION", "NODE_LINUX_AMD64_SHA256"):
            self.assertIn(name, pinned, f"{name} is missing from toolchain.versions")
            self.assertIn(
                pinned[name],
                text,
                f"the image must pin {name} to the value in toolchain.versions",
            )

    def test_inventory_names_every_tool_the_stage_depends_on(self) -> None:
        inventory = json.loads(INVENTORY.read_text(encoding="utf-8"))
        names = {entry["name"] for entry in inventory["tools"]}
        self.assertEqual(
            names,
            {"NetworkManager", "dbus", "nftables", "WireGuard", "node", "chromium"},
            "the inventory must probe exactly the components the image carries",
        )
        for entry in inventory["tools"]:
            self.assertTrue(entry["command"], f"{entry['name']} declares no probe command")

    def test_every_inventory_probe_is_actually_run_inside_the_image(self) -> None:
        inventory = json.loads(INVENTORY.read_text(encoding="utf-8"))
        text = RUN.read_text(encoding="utf-8")
        self.assertIn(
            ". /repo/02_Tools/tests/appliance/probe.sh",
            text,
            "the stage must use the failure-preserving probe implementation",
        )
        for entry in inventory["tools"]:
            self.assertIn(
                f"probe {entry['name']} {entry['command']}",
                text,
                f"{entry['name']} is declared in the inventory but never probed in the image",
            )
        declared = {entry["name"] for entry in inventory["tools"]}
        probed = set(re.findall(r"^probe (\S+) ", text, re.MULTILINE))
        self.assertEqual(
            probed,
            declared,
            "the probes run inside the image must be exactly the declared inventory",
        )

    def test_inventory_probe_preserves_a_failed_commands_status(self) -> None:
        shell = shutil.which("sh")
        probe_path = PROBE.as_posix()
        command: list[str]
        if shell is not None:
            command = [shell, "-c"]
        else:
            wsl = shutil.which("wsl")
            if wsl is None:
                self.skipTest("no POSIX shell is available for the probe behavior test")
            translated = subprocess.run(
                [wsl, "--", "wslpath", "-a", str(PROBE).replace("\\", "/")],
                capture_output=True,
                text=True,
                timeout=120,
            )
            if translated.returncode != 0:
                self.skipTest("the WSL2 distribution could not resolve the probe path")
            probe_path = translated.stdout.strip()
            command = [wsl, "--", "sh", "-c"]
        result = subprocess.run(
            command
            + [f'. "{probe_path}"; probe broken sh -c \'echo failed-tool; exit 23\''],
            capture_output=True,
            text=True,
            timeout=30,
        )
        self.assertEqual(result.returncode, 23, result.stdout + result.stderr)
        self.assertIn("failed-tool", result.stderr)
        self.assertIn("MISSING", result.stderr)

    def test_shared_image_keeps_network_install_and_appliance_targets_distinct(
        self,
    ) -> None:
        dockerfile = DOCKERFILE.read_text(encoding="utf-8")
        marker = "FROM network-system AS appliance"
        self.assertIn(marker, dockerfile)
        network_stage, appliance_stage = dockerfile.split(marker, maxsplit=1)
        self.assertNotIn(
            "wireguard-tools",
            network_stage,
            "the package-install system test needs an image without WireGuard",
        )
        self.assertIn("wireguard-tools", appliance_stage)
        self.assertIn(
            "--target network-system", NETWORK_CHECK.read_text(encoding="utf-8")
        )
        self.assertIn("--target appliance", CHECK.read_text(encoding="utf-8"))


class ApplianceBrowserSuiteTests(unittest.TestCase):
    """REQ-TEST-68eedecb, REQ-TEST-b1177191 - the browser suite runs on the stage."""

    def test_image_carries_the_interface_projects_locked_dependencies(self) -> None:
        text = DOCKERFILE.read_text(encoding="utf-8")
        self.assertIn("npm ci", text, "the image must install the locked dependencies")
        self.assertIn("package-lock.json", text, "the install is driven by the lockfile")
        self.assertIn(GUI_MODULES, text, f"they must land at {GUI_MODULES}")

    def test_stage_builds_the_interface_and_runs_the_browser_suite(self) -> None:
        text = RUN.read_text(encoding="utf-8")
        self.assertIn("run build", text, "the stage must build the interface it serves")
        self.assertIn("test:e2e", text, "the stage must run the browser suite")
        self.assertIn(
            "NILO_GUI_DIR",
            text,
            "the stage must tell the harness which built interface to serve",
        )

    def test_stage_serves_the_built_interface_rather_than_a_development_server(self) -> None:
        self.assertNotIn(
            "vite",
            RUN.read_text(encoding="utf-8").lower(),
            "the stage must not start a development server",
        )

    def test_locked_container_no_longer_runs_the_browser_suite(self) -> None:
        offline = (
            TOOLS_ROOT / "tests" / "build-environment" / "verify-offline.sh"
        ).read_text(encoding="utf-8")
        self.assertNotIn(
            "run test:e2e",
            offline,
            "the locked container has no network and dropped capabilities, so it"
            " cannot run the appliance browser suite",
        )
        self.assertIn(
            "02_Tools/tests/appliance/check.sh",
            offline,
            "it must name where that suite runs instead",
        )

    def test_repository_gate_routes_the_browser_suite_to_the_stage(self) -> None:
        router = (
            REPOSITORY_ROOT / "05_Software" / "scripts" / "aegis_test.py"
        ).read_text(encoding="utf-8")
        self.assertIn("_run_appliance_stage", router, "the gate must reach the stage")
        self.assertNotIn(
            '"run", "test:e2e"',
            router,
            "the gate must not run the browser suite outside the stage",
        )

    def test_fixture_pins_route_metrics_for_repeatable_restore_verification(self) -> None:
        text = NETWORK_UP.read_text(encoding="utf-8")
        self.assertIn(
            "ipv4.route-metric 100",
            text,
            "the fixed IPv4 route must not be recalculated after a rollback",
        )
        self.assertIn(
            "ipv6.route-metric 100",
            text,
            "the fixed IPv6 route must not be recalculated after a rollback",
        )


class AppliancePrerequisiteTests(unittest.TestCase):
    """REQ-TEST-a276c88a - an absent prerequisite is named, never skipped.

    The engine refusals are only reachable on a Linux host, because the
    unsupported-host refusal correctly fires first anywhere else. On the
    supported Windows development host they are driven inside the WSL2
    distribution that host is required to have.
    """

    # The entry point needs `uname` before it reaches the engine checks, so the
    # doctored path carries that one tool and nothing else. Everything else it
    # uses first - printf, command, test - is a shell builtin.
    _DRIVER = """#!/bin/sh
bin=$(mktemp -d) || exit 90
shell=$(command -v sh) || exit 94
ln -s "$(command -v uname)" "$bin/uname" || exit 91
if [ "MODE" = daemon ]; then
  printf '#!/bin/sh\\necho "Cannot connect to the Docker daemon" >&2\\nexit 1\\n' > "$bin/docker" || exit 92
  chmod +x "$bin/docker" || exit 93
fi
PATH="$bin" NILO_APPLIANCE_PREREQUISITE_ONLY=1 "$shell" "CHECKPATH"
status=$?
rm -rf "$bin"
exit $status
"""

    @classmethod
    def _to_linux_path(cls, path: str) -> str:
        if cls.wsl is None:
            return path
        translated = subprocess.run(
            [cls.wsl, "--", "wslpath", "-a", path.replace("\\", "/")],
            capture_output=True,
            text=True,
            timeout=120,
        )
        if translated.returncode != 0:
            raise unittest.SkipTest("the WSL2 distribution could not resolve a path")
        return translated.stdout.strip()

    @classmethod
    def setUpClass(cls) -> None:
        import platform

        if platform.system() == "Linux":
            cls.wsl = None
            cls.shell = shutil.which("sh") or "/bin/sh"
        else:
            cls.wsl = shutil.which("wsl")
            if cls.wsl is None:
                raise unittest.SkipTest(
                    "no Linux shell: the engine refusals are reachable only on a Linux host"
                )
        cls.check_path = cls._to_linux_path(str(CHECK))

    def _run_check(self, mode: str) -> subprocess.CompletedProcess[str]:
        # The driver goes to a file rather than `sh -c`: variable expansion does
        # not survive the wsl.exe command-line boundary on the supported Windows
        # host, and a file has no quoting to survive at all.
        body = self._DRIVER.replace("MODE", mode).replace("CHECKPATH", self.check_path)
        handle = tempfile.NamedTemporaryFile(
            "w", suffix=".sh", delete=False, encoding="utf-8", newline="\n"
        )
        try:
            handle.write(body)
            handle.close()
            script = self._to_linux_path(handle.name)
            command = (
                [self.wsl, "--", "sh", script]
                if self.wsl is not None
                else [self.shell, script]
            )
            return subprocess.run(command, capture_output=True, text=True, timeout=180)
        finally:
            os.unlink(handle.name)

    def test_absent_container_engine_is_named_and_the_run_fails(self) -> None:
        result = self._run_check("absent")
        combined = (result.stdout + result.stderr).lower()
        self.assertEqual(result.returncode, EXIT_PREREQUISITE, combined)
        self.assertIn("container engine", combined)
        self.assertNotIn("skip", combined)

    def test_unreachable_engine_daemon_is_named_and_the_run_fails(self) -> None:
        result = self._run_check("daemon")
        combined = (result.stdout + result.stderr).lower()
        self.assertEqual(result.returncode, EXIT_PREREQUISITE, combined)
        self.assertIn("daemon", combined)
        self.assertNotIn("skip", combined)


class ApplianceUnsupportedHostTests(unittest.TestCase):
    """REQ-TEST-a276c88a - an unsupported host is named rather than skipped."""

    def test_entry_point_refuses_an_unsupported_host_by_name(self) -> None:
        text = CHECK.read_text(encoding="utf-8")
        self.assertIn(
            "uname",
            text,
            "the entry point must decide host support from the running system",
        )
        self.assertIn(
            "WSL2",
            text,
            "the refusal must name the supported host a Windows checkout uses",
        )

    def test_a_windows_host_is_refused_rather_than_passing(self) -> None:
        git = shutil.which("git")
        shell = (
            Path(git).resolve().parent.parent / "bin" / "sh.exe"
            if git is not None
            else None
        )
        if shell is None or not shell.exists() or os.name != "nt":
            self.skipTest("this case observes the refusal a Windows host receives")
        environment = dict(os.environ)
        environment["NILO_APPLIANCE_PREREQUISITE_ONLY"] = "1"
        shell_path = f"/{CHECK.drive[0].lower()}{CHECK.as_posix()[2:]}"
        result = subprocess.run(
            [shell, shell_path],
            capture_output=True,
            text=True,
            env=environment,
            timeout=180,
        )
        combined = (result.stdout + result.stderr).lower()
        self.assertEqual(result.returncode, EXIT_PREREQUISITE, combined)
        self.assertIn("wsl2", combined)
        self.assertNotIn("skip", combined)


class ApplianceHostIsolationTests(unittest.TestCase):
    """REQ-TEST-6072fc2d - the stage confines itself to what it created."""

    def test_cleanup_does_not_remove_a_container_that_create_never_created(
        self,
    ) -> None:
        wsl = shutil.which("wsl") if os.name == "nt" else None
        shell = shutil.which("sh")
        if wsl is None and shell is None:
            self.skipTest("no Linux shell is available for the cleanup behavior test")

        check_path = str(CHECK)
        if wsl is not None:
            translated = subprocess.run(
                [wsl, "--", "wslpath", "-a", check_path.replace("\\", "/")],
                capture_output=True,
                text=True,
                timeout=120,
            )
            if translated.returncode != 0:
                self.skipTest("the WSL2 distribution could not resolve the checkout")
            check_path = translated.stdout.strip()

        driver_body = f"""#!/bin/sh
fake=$(mktemp) || exit 90
log=$(mktemp) || exit 91
cat > "$fake" <<'FAKE'
#!/bin/sh
printf '%s\\n' "$*" >> "$NILO_FAKE_ENGINE_LOG"
[ "$1" = create ] && exit 17
exit 0
FAKE
chmod +x "$fake" || exit 92
NILO_APPLIANCE_ENGINE="$fake" NILO_FAKE_ENGINE_LOG="$log" sh '{check_path}'
status=$?
cat "$log"
rm -f "$fake" "$log"
exit $status
"""
        handle = tempfile.NamedTemporaryFile(
            "w", suffix=".sh", delete=False, encoding="utf-8", newline="\n"
        )
        try:
            handle.write(driver_body)
            handle.close()
            driver = handle.name
            if wsl is not None:
                translated = subprocess.run(
                    [wsl, "--", "wslpath", "-a", driver.replace("\\", "/")],
                    capture_output=True,
                    text=True,
                    timeout=120,
                )
                if translated.returncode != 0:
                    self.skipTest("the WSL2 distribution could not resolve a temp file")
                command = [wsl, "--", "sh", translated.stdout.strip()]
            else:
                command = [shell or "/bin/sh", driver]
            result = subprocess.run(command, capture_output=True, text=True, timeout=180)
        finally:
            os.unlink(handle.name)

        combined = result.stdout + result.stderr
        self.assertEqual(result.returncode, 17, combined)
        self.assertIn("create --name", result.stdout)
        self.assertNotRegex(
            result.stdout,
            r"(?m)^rm -f ",
            "cleanup must not remove a container whose create command failed",
        )

    def test_entry_point_creates_and_removes_its_own_engine_network(self) -> None:
        text = CHECK.read_text(encoding="utf-8")
        self.assertIn("network create", text, "the stage must create its own network")
        self.assertIn("network rm", text, "the stage must remove the network it created")
        self.assertIn("trap", text, "removal must run on every exit path")

    def test_entry_point_compares_host_interfaces_and_route_around_the_run(self) -> None:
        text = CHECK.read_text(encoding="utf-8")
        self.assertIn("host_state", text, "the stage must record the host's own state")
        occurrences = text.count("host_state")
        self.assertGreaterEqual(
            occurrences,
            3,
            "host state must be recorded before the run, after it, and compared",
        )

    def test_container_side_touches_no_host_interface(self) -> None:
        for script in (RUN, NETWORK_UP):
            text = script.read_text(encoding="utf-8")
            for forbidden in ("--network host", "--net=host", "/var/run/docker.sock"):
                self.assertNotIn(
                    forbidden,
                    text,
                    f"{script.name} must not reach the host network or engine",
                )

    def test_container_side_refuses_to_run_outside_its_container(self) -> None:
        text = RUN.read_text(encoding="utf-8")
        self.assertIn("/.dockerenv", text, "the stage body must refuse outside a container")
        self.assertIn(
            "NILO_APPLIANCE_STAGE",
            text,
            "the stage body must require an explicit opt-in",
        )


if __name__ == "__main__":
    unittest.main()
