"""Contract tests for the repository-local NILO development toolchain."""

from __future__ import annotations

import re
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

        self.assertEqual(set(values), {"GO_VERSION", "PYTHON_VERSION", "UV_VERSION", *expected_digests})

    def test_bootstrap_and_environment_scripts_are_checksum_gated_and_process_local(self) -> None:
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
        self.assertIn("https://github.com/astral-sh/uv/releases/download/", bootstrap_ps1 + bootstrap_sh)
        self.assertNotRegex(bootstrap_ps1, r"\bsetx(?:\.exe)?\b|setenvironmentvariable")
        self.assertNotRegex(bootstrap_sh, r"(?:^|\s)(?:>>|>)\s*[^\n]*(?:profile|bashrc|zshrc)")

        for variable in ("uv_cache_dir", "uv_python_install_dir", "path"):
            self.assertIn(variable, environment)
        self.assertIn("/02_tools/runtime/", ignore)
        self.assertIn("/02_tools/cache/", ignore)
        self.assertIn("__pycache__/", ignore)

    def _read_manifest(self) -> dict[str, str]:
        self.assertTrue(MANIFEST_PATH.is_file(), f"missing toolchain manifest: {MANIFEST_PATH}")
        values: dict[str, str] = {}
        for line_number, raw_line in enumerate(MANIFEST_PATH.read_text(encoding="utf-8").splitlines(), 1):
            line = raw_line.strip()
            if not line or line.startswith("#"):
                continue
            self.assertRegex(line, r"^[A-Z0-9_]+=[0-9a-z.]+$", f"invalid line {line_number}")
            name, value = line.split("=", 1)
            self.assertNotIn(name, values, f"duplicate manifest key {name}")
            values[name] = value
        return values


if __name__ == "__main__":
    unittest.main()
