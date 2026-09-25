"""Portable test and lint routing for the NILO Go core and Python integrations."""

from __future__ import annotations

import argparse
import os
import subprocess
import sys
from pathlib import Path

import tomllib

IGNORED_PARTS = {
    ".git",
    ".venv",
    "__pycache__",
    "coverage",
    "dist",
    "node_modules",
}

IGNORED_WSL_DISTRIBUTIONS = {
    "docker-desktop",
    "docker-desktop-data",
}


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
    if sys.platform == "win32":
        for package_file in root.rglob("package.json"):
            if not _is_ignored(package_file.relative_to(root)):
                suites.append(("frontend", package_file.parent))
    suite_order = {"go": 0, "python": 1, "frontend": 2}
    return sorted(suites, key=lambda suite: (suite_order[suite[0]], str(suite[1])))


def _is_ignored(path: Path) -> bool:
    return any(
        part in IGNORED_PARTS or part.startswith(".venv-") for part in path.parts
    )


def _run(command: list[str], cwd: Path) -> int:
    print(f"[{cwd}] {' '.join(command)}", flush=True)
    try:
        return subprocess.run(command, cwd=cwd, check=False).returncode
    except FileNotFoundError:
        print(
            f"error: required command not found: {command[0]}",
            file=sys.stderr,
            flush=True,
        )
        return 127


def _uses_uv_environment(suite_root: Path) -> bool:
    """Return whether a Python project's declared build environment is uv."""
    if (suite_root / "uv.lock").is_file():
        return True
    try:
        metadata = tomllib.loads(
            (suite_root / "pyproject.toml").read_text(encoding="utf-8")
        )
    except (OSError, tomllib.TOMLDecodeError):
        return False
    build_system = metadata.get("build-system", {})
    return (
        isinstance(build_system, dict)
        and build_system.get("build-backend") == "uv_build"
    )


def _python_test_command(suite_root: Path) -> list[str]:
    """Choose the test runner declared by a Python suite's project metadata."""
    if _uses_uv_environment(suite_root):
        return ["uv", "run", "pytest", "-q"]
    return [
        sys.executable,
        "-m",
        "unittest",
        "discover",
        "-s",
        "tests",
        "-p",
        "test_*.py",
        "-v",
    ]


def _run_full(root: Path) -> int:
    script_tests = root.parent / "02_Tools" / "tests" / "software"
    if script_tests.is_dir():
        result = _run(
            [
                sys.executable,
                "-m",
                "unittest",
                "discover",
                "-s",
                "../02_Tools/tests/software",
                "-p",
                "test_*.py",
                "-v",
            ],
            root,
        )
        if result:
            return result

    for kind, suite_root in discover_suites(root):
        if kind == "go":
            result = _run(["go", "test", "./..."], suite_root)
        elif kind == "python":
            tests = suite_root / "tests"
            if tests.is_dir():
                result = _run(_python_test_command(suite_root), suite_root)
            else:
                print(
                    f"[{suite_root}] no tests directory; compiling Python sources",
                    flush=True,
                )
                result = _run(
                    [sys.executable, "-m", "compileall", "-q", "."], suite_root
                )
        else:
            result = _run(["npm.cmd", "run", "test:ci"], suite_root)
            if not result:
                result = _run(["npm.cmd", "run", "build"], suite_root)
            if not result:
                # The browser-to-appliance proof. It fails the whole command
                # when a prerequisite is missing rather than skipping, so a
                # green gate never claims a browser reached Bench Core when none
                # did.
                result = _run_appliance_stage(root)
        if result:
            return result
    contract_suite = root / "tests" / "contract" / "test_integration_contract.py"
    if contract_suite.is_file():
        return _run(
            ["uv", "run", "--project", "integrations/sdk/python", "--all-groups", "--locked", "pytest", "-q", "tests/contract/test_integration_contract.py"],
            root,
        )
    return 0


def _run_appliance_stage(root: Path) -> int:
    """Run the browser suite where an appliance can exist.

    The suite drives Bench Core on the appliance profile, which needs the
    privileged helper and a real NetworkManager. Those live in the stage
    container, so this routes there rather than starting anything here. On a
    Windows checkout the stage is reached through the WSL2 distribution the
    supported development host already has.
    """
    stage = "02_Tools/tests/appliance/check.sh"
    repository_root = root.parent
    if sys.platform == "win32":
        distribution, selection_status = _select_wsl_distribution()
        if distribution is None:
            return selection_status
        translated = subprocess.run(
            [
                "wsl.exe",
                "-d",
                distribution,
                "--exec",
                "wslpath",
                "-a",
                "--",
                str(repository_root).replace("\\", "/"),
            ],
            capture_output=True,
            text=True,
            check=False,
        )
        if translated.returncode != 0:
            print(
                "error: the WSL2 distribution could not resolve this checkout, so the"
                " browser-to-appliance proof cannot run",
                file=sys.stderr,
                flush=True,
            )
            return translated.returncode
        linux_root = translated.stdout.strip().rstrip("/")
        return _run(
            [
                "wsl.exe",
                "-d",
                distribution,
                "--exec",
                "sh",
                f"{linux_root}/{stage}",
            ],
            repository_root,
        )
    return _run(["sh", stage], repository_root)


def _select_wsl_distribution() -> tuple[str | None, int]:
    """Select exactly one installed WSL distribution for the appliance stage."""
    try:
        discovered = subprocess.run(
            ["wsl.exe", "--list", "--quiet"],
            capture_output=True,
            text=True,
            check=False,
        )
    except FileNotFoundError:
        print("error: required command not found: wsl.exe", file=sys.stderr, flush=True)
        return None, 127
    if discovered.returncode != 0:
        print(
            "error: installed WSL distributions could not be listed",
            file=sys.stderr,
            flush=True,
        )
        return None, discovered.returncode

    distributions = [
        line.strip()
        for line in discovered.stdout.replace("\x00", "").splitlines()
        if line.strip() and line.strip().casefold() not in IGNORED_WSL_DISTRIBUTIONS
    ]
    requested = os.environ.get("NILO_WSL_DISTRIBUTION", "").strip()
    if requested:
        selected = next(
            (name for name in distributions if name.casefold() == requested.casefold()),
            None,
        )
        if selected is None:
            print(
                f"error: WSL distribution '{requested}' is not installed; "
                "set NILO_WSL_DISTRIBUTION to an installed distribution",
                file=sys.stderr,
                flush=True,
            )
            return None, 2
        return selected, 0
    if len(distributions) == 1:
        return distributions[0], 0
    if not distributions:
        print(
            "error: no WSL distribution is installed; prepare one with "
            "02_Tools/bootstrap/bootstrap-development.ps1",
            file=sys.stderr,
            flush=True,
        )
    else:
        print(
            "error: several WSL distributions are installed "
            f"({', '.join(distributions)}); set NILO_WSL_DISTRIBUTION to choose one",
            file=sys.stderr,
            flush=True,
        )
    return None, 2


def _run_lint(root: Path) -> int:
    for kind, suite_root in discover_suites(root):
        if kind == "go":
            result = _run(["go", "vet", "./..."], suite_root)
        elif kind == "python":
            result = _run([sys.executable, "-m", "compileall", "-q", "."], suite_root)
        else:
            result = _run(["npm.cmd", "run", "format:check"], suite_root)
            if not result:
                result = _run(["npm.cmd", "run", "lint"], suite_root)
        if result:
            return result
    return _run([sys.executable, "-m", "compileall", "-q", "../02_Tools/scripts"], root)


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("mode", choices=("full", "impacted", "lint"))
    parser.add_argument("selectors", nargs="*")
    arguments = parser.parse_args(argv)
    root = Path.cwd()

    if arguments.mode == "lint":
        return _run_lint(root)
    if arguments.mode == "impacted" and arguments.selectors:
        print(
            "Impacted routing is conservative; running every discovered suite.",
            flush=True,
        )
    return _run_full(root)


if __name__ == "__main__":
    raise SystemExit(main())
