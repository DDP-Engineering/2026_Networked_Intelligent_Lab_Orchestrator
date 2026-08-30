"""Tests for the portable AEGIS test router."""

from __future__ import annotations

import importlib.util
import sys
import tempfile
import unittest
from pathlib import Path


SOFTWARE_ROOT = Path(__file__).resolve().parents[2]
ROUTER_PATH = SOFTWARE_ROOT / "scripts" / "aegis_test.py"


class AegisTestRouterTests(unittest.TestCase):
    def test_discover_suites_finds_go_module_and_each_python_integration(self) -> None:
        """A future integration must not be silently omitted from AEGIS runs."""
        self.assertTrue(ROUTER_PATH.is_file(), f"missing router: {ROUTER_PATH}")

        with tempfile.TemporaryDirectory() as temporary_directory:
            root = Path(temporary_directory)
            go_module = root / "nilo_assistant"
            go_module.mkdir()
            (go_module / "go.mod").write_text("module example.test/nilo\n", encoding="utf-8")

            python_integrations = [
                root / "integrations" / "fixture_alpha",
                root / "integrations" / "fixture_beta",
            ]
            for integration in python_integrations:
                integration.mkdir(parents=True)
                (integration / "pyproject.toml").write_text(
                    "[project]\nname = \"fixture\"\nversion = \"0.0.0\"\n",
                    encoding="utf-8",
                )

            module = self._load_router()
            self.assertEqual(
                module.discover_suites(root),
                [("go", go_module), *( ("python", integration) for integration in python_integrations)],
            )

    def _load_router(self):
        specification = importlib.util.spec_from_file_location("aegis_test", ROUTER_PATH)
        self.assertIsNotNone(specification)
        self.assertIsNotNone(specification.loader)
        module = importlib.util.module_from_spec(specification)
        sys.modules[specification.name] = module
        specification.loader.exec_module(module)
        return module


if __name__ == "__main__":
    unittest.main()
