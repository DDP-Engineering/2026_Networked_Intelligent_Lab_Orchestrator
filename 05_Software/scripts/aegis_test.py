"""Portable test and lint routing for the NILO Go core and Python integrations."""

from __future__ import annotations

import argparse
import subprocess
import sys
from pathlib import Path


IGNORED_PARTS = {".git", ".venv", "__pycache__"}


def discover_suites(root: Path) -> list[tuple[str, Path]]:
    """Return every Go module and Python project below the software root."""
    root = root.resolve()
    suites: list[tuple[str, Path]] = []
    for module_file in root.rglob("go.mod"):
        if not _is_ignored(module_file.relative_to(root)):
            suites.append(("go", module_file.parent))
    for project_file in root.rglob("pyproject.toml"):
        if not _is_ignored(project_file.relative_to(root)):
            suites.append(("python", project_file.parent))
    return sorted(suites, key=lambda suite: (suite[0] != "go", str(suite[1])))


def _is_ignored(path: Path) -> bool:
    return any(part in IGNORED_PARTS for part in path.parts)


def _run(command: list[str], cwd: Path) -> int:
    print(f"[{cwd}] {' '.join(command)}", flush=True)
    return subprocess.run(command, cwd=cwd, check=False).returncode


def _run_full(root: Path) -> int:
    script_tests = root / "scripts" / "tests"
    if script_tests.is_dir():
        result = _run(
            [sys.executable, "-m", "unittest", "discover", "-s", "scripts/tests", "-p", "test_*.py", "-v"],
            root,
        )
        if result:
            return result

    for kind, suite_root in discover_suites(root):
        if kind == "go":
            result = _run(["go", "test", "./..."], suite_root)
        else:
            tests = suite_root / "tests"
            if tests.is_dir():
                result = _run(
                    [sys.executable, "-m", "unittest", "discover", "-s", "tests", "-p", "test_*.py", "-v"],
                    suite_root,
                )
            else:
                print(f"[{suite_root}] no tests directory; compiling Python sources", flush=True)
                result = _run([sys.executable, "-m", "compileall", "-q", "."], suite_root)
        if result:
            return result
    return 0


def _run_lint(root: Path) -> int:
    for kind, suite_root in discover_suites(root):
        if kind == "go":
            result = _run(["go", "vet", "./..."], suite_root)
        else:
            result = _run([sys.executable, "-m", "compileall", "-q", "."], suite_root)
        if result:
            return result
    return _run([sys.executable, "-m", "compileall", "-q", "scripts"], root)


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("mode", choices=("full", "impacted", "lint"))
    parser.add_argument("selectors", nargs="*")
    arguments = parser.parse_args(argv)
    root = Path.cwd()

    if arguments.mode == "lint":
        return _run_lint(root)
    if arguments.mode == "impacted" and arguments.selectors:
        print("Impacted routing is conservative; running every discovered suite.", flush=True)
    return _run_full(root)


if __name__ == "__main__":
    raise SystemExit(main())
