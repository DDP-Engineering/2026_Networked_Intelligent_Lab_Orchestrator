"""Tests for the portable AEGIS test router."""

from __future__ import annotations

import importlib.util
import sys
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

SOFTWARE_ROOT = Path(__file__).resolve().parents[2]
ROUTER_PATH = SOFTWARE_ROOT / "scripts" / "aegis_test.py"


class AegisTestRouterTests(unittest.TestCase):
    def test_discover_suites_finds_windows_frontend_and_ignores_generated_packages(
        self,
    ) -> None:
        with tempfile.TemporaryDirectory() as temporary_directory:
            root = Path(temporary_directory)
            frontend = root / "nilo_gui"
            frontend.mkdir()
            (frontend / "package.json").write_text(
                '{"name":"nilo-gui","private":true}', encoding="utf-8"
            )
            generated = frontend / "node_modules" / "fixture"
            generated.mkdir(parents=True)
            (generated / "package.json").write_text(
                '{"name":"generated"}', encoding="utf-8"
            )

            module = self._load_router()
            with patch.object(module.sys, "platform", "win32"):
                suites = module.discover_suites(root)

            self.assertEqual(suites, [("frontend", frontend)])

    def test_windows_frontend_runs_test_build_format_and_lint_commands(self) -> None:
        with tempfile.TemporaryDirectory() as temporary_directory:
            root = Path(temporary_directory)
            frontend = root / "nilo_gui"
            frontend.mkdir()
            (frontend / "package.json").write_text(
                '{"name":"nilo-gui","private":true}', encoding="utf-8"
            )

            module = self._load_router()
            commands: list[tuple[list[str], Path]] = []
            with (
                patch.object(module.sys, "platform", "win32"),
                patch.object(
                    module,
                    "_run",
                    side_effect=lambda command, cwd: commands.append((command, cwd))
                    or 0,
                ),
            ):
                self.assertEqual(module._run_full(root), 0)
                self.assertEqual(module._run_lint(root), 0)

            self.assertEqual(
                commands,
                [
                    (["npm.cmd", "run", "test:ci"], frontend),
                    (["npm.cmd", "run", "build"], frontend),
                    (["npm.cmd", "run", "format:check"], frontend),
                    (["npm.cmd", "run", "lint"], frontend),
                    ([sys.executable, "-m", "compileall", "-q", "scripts"], root),
                ],
            )

    def test_discover_suites_finds_go_module_and_each_python_integration(self) -> None:
        """A future integration must not be silently omitted from AEGIS runs."""
        self.assertTrue(ROUTER_PATH.is_file(), f"missing router: {ROUTER_PATH}")

        with tempfile.TemporaryDirectory() as temporary_directory:
            root = Path(temporary_directory)
            go_module = root / "nilo_assistant"
            go_module.mkdir()
            (go_module / "go.mod").write_text(
                "module example.test/nilo\n", encoding="utf-8"
            )

            python_integrations = [
                root / "integrations" / "fixture_alpha",
                root / "integrations" / "fixture_beta",
            ]
            for integration in python_integrations:
                integration.mkdir(parents=True)
                (integration / "pyproject.toml").write_text(
                    '[project]\nname = "fixture"\nversion = "0.0.0"\n',
                    encoding="utf-8",
                )

            generated_environment = root / "integration" / ".venv-windows-x86_64"
            generated_environment.mkdir(parents=True)
            (generated_environment / "pyproject.toml").write_text(
                '[project]\nname = "generated-environment"\nversion = "0.0.0"\n',
                encoding="utf-8",
            )

            module = self._load_router()
            self.assertEqual(
                module.discover_suites(root),
                [
                    ("go", go_module),
                    *(("python", integration) for integration in python_integrations),
                ],
            )

    def test_run_full_uses_uv_pytest_for_uv_build_project(self) -> None:
        """uv-managed projects must not inherit the host Python environment."""
        with tempfile.TemporaryDirectory() as temporary_directory:
            root = Path(temporary_directory)
            project = root / "integration"
            (project / "tests").mkdir(parents=True)
            (project / "pyproject.toml").write_text(
                "[build-system]\n"
                'requires = ["uv_build>=0.9"]\n'
                'build-backend = "uv_build"\n',
                encoding="utf-8",
            )

            module = self._load_router()
            commands: list[tuple[list[str], Path]] = []
            with patch.object(
                module,
                "_run",
                side_effect=lambda command, cwd: commands.append((command, cwd)) or 0,
            ):
                self.assertEqual(module._run_full(root), 0)

            self.assertEqual(commands, [(["uv", "run", "pytest", "-q"], project)])

    def test_missing_project_runner_returns_shell_not_found_status(self) -> None:
        """A missing declared runner must fail cleanly instead of crashing AEGIS."""
        module = self._load_router()
        with patch.object(module.subprocess, "run", side_effect=FileNotFoundError):
            try:
                status = module._run(["uv", "run", "pytest", "-q"], Path.cwd())
            except FileNotFoundError:
                status = -1
        self.assertEqual(status, 127)

    def _load_router(self):
        specification = importlib.util.spec_from_file_location(
            "aegis_test", ROUTER_PATH
        )
        self.assertIsNotNone(specification)
        self.assertIsNotNone(specification.loader)
        module = importlib.util.module_from_spec(specification)
        sys.modules[specification.name] = module
        specification.loader.exec_module(module)
        return module


if __name__ == "__main__":
    unittest.main()
