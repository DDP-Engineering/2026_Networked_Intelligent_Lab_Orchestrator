"""Tests for the portable AEGIS test router."""

from __future__ import annotations

import importlib.util
import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from unittest.mock import call, patch

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
                patch.object(module, "_run_appliance_stage", return_value=0) as stage,
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
            stage.assert_called_once_with(root)

    def test_a_failing_browser_end_to_end_run_fails_the_whole_command(self) -> None:
        """A skipped or failed browser run must never be reported as a pass."""
        with tempfile.TemporaryDirectory() as temporary_directory:
            root = Path(temporary_directory)
            frontend = root / "nilo_gui"
            frontend.mkdir()
            (frontend / "package.json").write_text(
                '{"name":"nilo-gui","private":true}', encoding="utf-8"
            )

            module = self._load_router()
            with (
                patch.object(module.sys, "platform", "win32"),
                patch.object(module, "_run", return_value=0),
                patch.object(module, "_run_appliance_stage", return_value=6) as stage,
            ):
                self.assertEqual(module._run_full(root), 6)

            stage.assert_called_once_with(root)

    def test_windows_appliance_stage_invokes_the_translated_script_without_shell_code(
        self,
    ) -> None:
        module = self._load_router()
        software_root = Path("C:/work/NILO/05_Software")
        translated = subprocess.CompletedProcess(
            args=[], returncode=0, stdout="/mnt/c/work/NILO\n", stderr=""
        )
        distributions = subprocess.CompletedProcess(
            args=[], returncode=0, stdout="Ubuntu Dev\nUbuntu-24.04\n", stderr=""
        )
        with (
            patch.object(module.sys, "platform", "win32"),
            patch.dict(os.environ, {"NILO_WSL_DISTRIBUTION": "Ubuntu Dev"}),
            patch.object(
                module.subprocess, "run", side_effect=[distributions, translated]
            ) as convert,
            patch.object(module, "_run", return_value=0) as run,
        ):
            self.assertEqual(module._run_appliance_stage(software_root), 0)

        self.assertEqual(
            convert.call_args_list,
            [
                call(
                    ["wsl.exe", "--list", "--quiet"],
                    capture_output=True,
                    text=True,
                    check=False,
                ),
                call(
                    [
                        "wsl.exe",
                        "-d",
                        "Ubuntu Dev",
                        "--exec",
                        "wslpath",
                        "-a",
                        "--",
                        "C:/work/NILO",
                    ],
                    capture_output=True,
                    text=True,
                    check=False,
                ),
            ],
        )
        run.assert_called_once_with(
            [
                "wsl.exe",
                "-d",
                "Ubuntu Dev",
                "--exec",
                "sh",
                "/mnt/c/work/NILO/02_Tools/tests/appliance/check.sh",
            ],
            Path("C:/work/NILO"),
        )

    def test_windows_appliance_stage_selects_the_only_installed_distribution(
        self,
    ) -> None:
        module = self._load_router()
        discovered = subprocess.CompletedProcess(
            args=[], returncode=0, stdout="Ubuntu-24.04\n", stderr=""
        )
        translated = subprocess.CompletedProcess(
            args=[], returncode=0, stdout="/mnt/c/work/NILO\n", stderr=""
        )
        with (
            patch.object(module.sys, "platform", "win32"),
            patch.dict(os.environ, {}, clear=False),
            patch.object(
                module.subprocess, "run", side_effect=[discovered, translated]
            ),
            patch.object(module, "_run", return_value=0) as run,
        ):
            os.environ.pop("NILO_WSL_DISTRIBUTION", None)
            self.assertEqual(
                module._run_appliance_stage(Path("C:/work/NILO/05_Software")), 0
            )

        self.assertEqual(
            run.call_args.args[0][0:4],
            ["wsl.exe", "-d", "Ubuntu-24.04", "--exec"],
        )

    def test_windows_appliance_stage_rejects_an_uninstalled_selected_distribution(
        self,
    ) -> None:
        module = self._load_router()
        discovered = subprocess.CompletedProcess(
            args=[], returncode=0, stdout="Ubuntu-24.04\n", stderr=""
        )
        with (
            patch.object(module.sys, "platform", "win32"),
            patch.dict(os.environ, {"NILO_WSL_DISTRIBUTION": "Missing Linux"}),
            patch.object(module.subprocess, "run", return_value=discovered),
            patch.object(module, "_run") as run,
        ):
            self.assertEqual(
                module._run_appliance_stage(Path("C:/work/NILO/05_Software")), 2
            )
        run.assert_not_called()

    def test_windows_appliance_stage_fails_when_no_distribution_is_installed(
        self,
    ) -> None:
        module = self._load_router()
        discovered = subprocess.CompletedProcess(
            args=[], returncode=0, stdout="", stderr=""
        )
        with (
            patch.object(module.sys, "platform", "win32"),
            patch.dict(os.environ, {}, clear=False),
            patch.object(module.subprocess, "run", return_value=discovered),
            patch.object(module, "_run") as run,
        ):
            os.environ.pop("NILO_WSL_DISTRIBUTION", None)
            self.assertEqual(
                module._run_appliance_stage(Path("C:/work/NILO/05_Software")), 2
            )
        run.assert_not_called()

    def test_windows_appliance_stage_requires_selection_for_multiple_distributions(
        self,
    ) -> None:
        module = self._load_router()
        discovered = subprocess.CompletedProcess(
            args=[], returncode=0, stdout="Ubuntu\nUbuntu-24.04\n", stderr=""
        )
        with (
            patch.object(module.sys, "platform", "win32"),
            patch.dict(os.environ, {}, clear=False),
            patch.object(module.subprocess, "run", return_value=discovered),
            patch.object(module, "_run") as run,
        ):
            os.environ.pop("NILO_WSL_DISTRIBUTION", None)
            self.assertEqual(
                module._run_appliance_stage(Path("C:/work/NILO/05_Software")), 2
            )
        run.assert_not_called()

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
