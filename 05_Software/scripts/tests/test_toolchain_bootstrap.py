"""Contract tests for the repository-local NILO development toolchain."""

from __future__ import annotations

import os
import subprocess
import tempfile
import unittest
from pathlib import Path

REPOSITORY_ROOT = Path(__file__).resolve().parents[3]
TOOLS_ROOT = REPOSITORY_ROOT / "02_Tools"
MANIFEST_PATH = TOOLS_ROOT / "toolchain.versions"


class ToolchainBootstrapTests(unittest.TestCase):
    def test_manifest_pins_supported_tool_archives(self) -> None:
        values = self._read_manifest()
        self.assertEqual(values["GO_VERSION"], "1.27.0")
        self.assertEqual(values["PYTHON_VERSION"], "3.12.10")
        self.assertEqual(values["UV_VERSION"], "0.11.25")

        expected_digests = {
            "GO_WINDOWS_AMD64_SHA256": "f0c0a0d33ba94f4d2c5dbc887334ce678b21813504ddb3aafcb06e60a5a667c4",
            "GO_LINUX_AMD64_SHA256": "675c26c449cbb18fc24b74650de1eabbae6e16f64326fd85a283fb3b58280685",
            "GO_LINUX_ARM64_SHA256": "51798d2c42d0e1c6ed7fd9f48728b4193abac9e8aad6dbac2fe96a81f5909bda",
            "UV_WINDOWS_AMD64_SHA256": "15bfd1423b7eaa7aae949922d4712ebaac2bb44a81af64ab59bbe007090cb0d0",
            "UV_LINUX_AMD64_SHA256": "1db18b5e76fa645a7f3865773139bdec8e2d46adbdbb35e7410b34fa8015ccd2",
            "UV_LINUX_ARM64_SHA256": "e0e9d73f74e06a7dcd53910d5962146ab48f0af9c92cc8df33a37baa0121014d",
        }
        for name, expected in expected_digests.items():
            self.assertEqual(values[name], expected)
            self.assertRegex(values[name], r"^[0-9a-f]{64}$")

        self.assertEqual(
            set(values),
            {"GO_VERSION", "PYTHON_VERSION", "UV_VERSION", *expected_digests},
        )

    def test_bootstrap_and_environment_scripts_are_checksum_gated_and_process_local(
        self,
    ) -> None:
        paths = {
            "bootstrap_ps1": TOOLS_ROOT / "bootstrap" / "bootstrap.ps1",
            "bootstrap_sh": TOOLS_ROOT / "bootstrap" / "bootstrap.sh",
            "environment_ps1": TOOLS_ROOT / "env" / "with-tools.ps1",
            "environment_sh": TOOLS_ROOT / "env" / "with-tools.sh",
            "doctor_ps1": TOOLS_ROOT / "scripts" / "doctor.ps1",
            "doctor_sh": TOOLS_ROOT / "scripts" / "doctor.sh",
        }
        for name, path in paths.items():
            self.assertTrue(path.is_file(), f"missing {name}: {path}")

        bootstrap_ps1 = paths["bootstrap_ps1"].read_text(encoding="utf-8").lower()
        bootstrap_sh = paths["bootstrap_sh"].read_text(encoding="utf-8").lower()
        environment = "\n".join(
            path.read_text(encoding="utf-8").lower()
            for path in (paths["environment_ps1"], paths["environment_sh"])
        )
        ignore = (REPOSITORY_ROOT / ".gitignore").read_text(encoding="utf-8").lower()

        self.assertIn("get-filehash", bootstrap_ps1)
        self.assertIn("sha256sum", bootstrap_sh)
        self.assertIn("https://go.dev/dl/", bootstrap_ps1 + bootstrap_sh)
        self.assertIn(
            "https://github.com/astral-sh/uv/releases/download/",
            bootstrap_ps1 + bootstrap_sh,
        )
        self.assertNotRegex(bootstrap_ps1, r"\bsetx(?:\.exe)?\b|setenvironmentvariable")
        self.assertNotRegex(
            bootstrap_sh, r"(?:^|\s)(?:>>|>)\s*[^\n]*(?:profile|bashrc|zshrc)"
        )

        for variable in ("uv_cache_dir", "uv_python_install_dir", "path"):
            self.assertIn(variable, environment)
        self.assertIn("/02_tools/runtime/", ignore)
        self.assertIn("/02_tools/cache/", ignore)
        self.assertIn("__pycache__/", ignore)

    def test_platform_dependent_tools_use_distinct_host_roots(self) -> None:
        windows_scripts = self._read_scripts(
            "bootstrap/bootstrap.ps1",
            "env/with-tools.ps1",
            "env/with-go.ps1",
            "scripts/doctor.ps1",
        )
        linux_scripts = self._read_scripts(
            "bootstrap/bootstrap.sh",
            "env/with-tools.sh",
            "env/with-go.sh",
            "scripts/doctor.sh",
        )

        self.assertIn("windows-x86_64", windows_scripts)
        self.assertIn("linux-x86_64", linux_scripts)
        self.assertIn("linux-arm64", linux_scripts)
        self.assertIn("uv_project_environment", windows_scripts)
        self.assertIn("uv_project_environment", linux_scripts)
        self.assertNotIn("runtime\\go\\1.27.0", windows_scripts)
        self.assertNotIn("runtime/go/1.27.0", linux_scripts)

    def test_wsl_setup_requires_opt_in_before_host_changes(self) -> None:
        setup = TOOLS_ROOT / "bootstrap" / "setup-wsl.ps1"
        helper = TOOLS_ROOT / "lib" / "Wsl.ps1"
        self.assertTrue(setup.is_file(), f"missing WSL setup: {setup}")
        self.assertTrue(helper.is_file(), f"missing WSL helper: {helper}")

        source = setup.read_text(encoding="utf-8").lower()
        self.assertIn("[switch]$install", source)
        self.assertIn("--install", source)
        self.assertIn("--set-version", source)
        self.assertIn("wsl2", source)
        self.assertNotIn("enable-windowsoptionalfeature", source)

        if os.name == "nt":
            missing = self._run_wsl_script(setup, "missing")
            self.assertNotEqual(missing.returncode, 0)
            self.assertIn("-Install", missing.stdout + missing.stderr)

            requested = self._run_wsl_script(setup, "missing", "-Install")
            self.assertNotEqual(requested.returncode, 0)
            self.assertIn("--install -d Ubuntu", requested.log)

            unavailable = self._run_wsl_script(setup, "unavailable", "-Install")
            self.assertNotEqual(unavailable.returncode, 0)
            self.assertIn("--install -d Ubuntu", unavailable.log)

    def test_combined_bootstrap_orchestrates_windows_and_wsl_tools(self) -> None:
        combined = TOOLS_ROOT / "bootstrap" / "bootstrap-development.ps1"
        self.assertTrue(combined.is_file(), f"missing combined bootstrap: {combined}")
        source = combined.read_text(encoding="utf-8").lower()

        expected_order = [
            "setup-wsl.ps1",
            "bootstrap.ps1",
            "bootstrap.sh",
            "doctor.ps1",
            "doctor-wsl.ps1",
        ]
        positions = [source.find(name) for name in expected_order]
        self.assertTrue(all(position >= 0 for position in positions), positions)
        self.assertEqual(positions, sorted(positions))
        self.assertIn("[switch]$offline", source)
        self.assertIn("[switch]$installwsl", source)

    def test_wsl_doctor_checks_kernel_checkout_and_linux_tools(self) -> None:
        doctor = TOOLS_ROOT / "scripts" / "doctor-wsl.ps1"
        helper = TOOLS_ROOT / "lib" / "Wsl.ps1"
        self.assertTrue(doctor.is_file(), f"missing WSL doctor: {doctor}")
        source = "\n".join(
            path.read_text(encoding="utf-8").lower() for path in (doctor, helper)
        )
        self.assertIn("uname", source)
        self.assertIn("wslpath", source)
        self.assertIn("doctor.sh", source)
        self.assertIn("wsl2", source)

        if os.name == "nt":
            ready = self._run_wsl_script(doctor, "ready")
            self.assertEqual(ready.returncode, 0, ready.stdout + ready.stderr)
            self.assertIn("development environment: ok", ready.stdout.lower())

            wsl1 = self._run_wsl_script(doctor, "wsl1")
            self.assertNotEqual(wsl1.returncode, 0)
            self.assertIn("wsl2", (wsl1.stdout + wsl1.stderr).lower())

    def _read_manifest(self) -> dict[str, str]:
        self.assertTrue(
            MANIFEST_PATH.is_file(), f"missing toolchain manifest: {MANIFEST_PATH}"
        )
        values: dict[str, str] = {}
        for line_number, raw_line in enumerate(
            MANIFEST_PATH.read_text(encoding="utf-8").splitlines(), 1
        ):
            line = raw_line.strip()
            if not line or line.startswith("#"):
                continue
            self.assertRegex(
                line, r"^[A-Z0-9_]+=[0-9a-z.]+$", f"invalid line {line_number}"
            )
            name, value = line.split("=", 1)
            self.assertNotIn(name, values, f"duplicate manifest key {name}")
            values[name] = value
        return values

    def _read_scripts(self, *relative_paths: str) -> str:
        return "\n".join(
            (TOOLS_ROOT / path).read_text(encoding="utf-8").lower()
            for path in relative_paths
        )

    def _run_wsl_script(
        self, script: Path, mode: str, *extra_arguments: str
    ) -> subprocess.CompletedProcess[str]:
        with tempfile.TemporaryDirectory() as temporary_directory:
            temporary_root = Path(temporary_directory)
            log_path = temporary_root / "wsl.log"
            fake_wsl = temporary_root / "fake-wsl.ps1"
            fake_wsl.write_text(
                """param([Parameter(ValueFromRemainingArguments=$true)][string[]]$Arguments)
$line = $Arguments -join ' '
Add-Content -LiteralPath $env:NILO_FAKE_WSL_LOG -Value $line
if ($Arguments -contains '--list' -and $Arguments -contains '--quiet') {
    if ($env:NILO_FAKE_WSL_MODE -eq 'unavailable') { Write-Error 'WSL is not enabled'; exit 1 }
    if ($env:NILO_FAKE_WSL_MODE -ne 'missing') { Write-Output 'Ubuntu' }
    exit 0
}
if ($Arguments -contains 'uname') {
    if ($env:NILO_FAKE_WSL_MODE -eq 'wsl1') { Write-Output '4.4.0-Microsoft' }
    else { Write-Output '5.15.0-microsoft-standard-WSL2' }
    exit 0
}
if ($Arguments -contains 'wslpath') { Write-Output '/mnt/f/nilo'; exit 0 }
if ($Arguments -contains '--install' -or $Arguments -contains '--set-version') { exit 0 }
if ($Arguments -contains 'doctor.sh') { exit 0 }
exit 0
""",
                encoding="utf-8",
            )
            environment = os.environ.copy()
            environment["NILO_FAKE_WSL_MODE"] = mode
            environment["NILO_FAKE_WSL_LOG"] = str(log_path)
            completed = subprocess.run(
                [
                    "powershell",
                    "-NoProfile",
                    "-ExecutionPolicy",
                    "Bypass",
                    "-File",
                    str(script),
                    "-Distribution",
                    "Ubuntu",
                    "-WslExecutable",
                    str(fake_wsl),
                    *extra_arguments,
                ],
                cwd=REPOSITORY_ROOT,
                env=environment,
                text=True,
                capture_output=True,
                check=False,
            )
            completed.log = (
                log_path.read_text(encoding="utf-8") if log_path.exists() else ""
            )  # type: ignore[attr-defined]
            return completed


if __name__ == "__main__":
    unittest.main()
