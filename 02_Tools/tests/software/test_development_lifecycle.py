"""Contract tests for the one-command Windows development lifecycle.

Every test drives the real PowerShell entry point with a stand-in WSL launcher
and injected boundaries, so no test opens a window, binds a port, or signals a
process. The command under test supervises nothing, so there is no
process-ownership surface to exercise here.

Two tests reach past the injected boundaries on purpose, because the defects
they guard lived in the real ones: the argument-boundary test launches a
short-lived hidden child that prints the arguments it received, and the socket
parsing test calls the parser directly. Neither starts a server.
"""

from __future__ import annotations

import json
import os
import re
import subprocess
import tempfile
import unittest
from pathlib import Path

REPOSITORY_ROOT = Path(__file__).resolve().parents[3]
TOOLS_ROOT = REPOSITORY_ROOT / "02_Tools"
ENTRY_POINT = TOOLS_ROOT / "scripts" / "nilo-dev.ps1"
LIBRARY = TOOLS_ROOT / "lib" / "DevLifecycle.ps1"

EXIT_OK = 0
EXIT_UNEXPECTED = 1
EXIT_USAGE = 2
EXIT_PORT_CONFLICT = 3
EXIT_STOPPED = 4
EXIT_PARTIAL = 5

BACKEND_STATUS_URL = "http://127.0.0.1:8090/api/v1/status"
FRONTEND_URL = "http://127.0.0.1:5173/"

# A stand-in for the real WSL launcher. It records every argument list it is
# given, then answers the read-only queries the command asks of a distribution.
FAKE_WSL = """param([Parameter(ValueFromRemainingArguments=$true)][string[]]$Arguments)
$line = $Arguments -join ' '
Add-Content -LiteralPath $env:NILO_FAKE_WSL_LOG -Value $line
if ($Arguments -contains '--list' -and $Arguments -contains '--quiet') {
    Write-Output 'Ubuntu-24.04'
    exit 0
}
if ($Arguments -contains 'uname') { Write-Output '5.15.0-microsoft-standard-WSL2'; exit 0 }
if ($Arguments -contains 'wslpath') { Write-Output '/mnt/z/elsewhere/nilo-checkout'; exit 0 }
if ($line -match 'NILO_HOME_PROBE') {
    if ($env:NILO_FAKE_WSL_NOISE) { Write-Output $env:NILO_FAKE_WSL_NOISE }
    if ($env:NILO_FAKE_WSL_HOME) { Write-Output $env:NILO_FAKE_WSL_HOME } else { Write-Output '/home/dev' }
    exit 0
}
exit 0
"""

# Injected boundaries. Each is self-contained: it re-reads its plan and appends
# its own recording, so nothing depends on state that would fall out of scope.
FAKE_BOUNDARIES = """
@{
    TestEndpoint = {
        param([string]$Url)
        $plan = Get-Content -LiteralPath $env:NILO_FAKE_PLAN -Raw | ConvertFrom-Json
        $planKeys = @($plan.PSObject.Properties | ForEach-Object { $_.Name })
        $counters = Join-Path (Split-Path -Parent $env:NILO_FAKE_BOUNDARY_LOG) 'counters.json'
        $seen = 0
        $state = @{}
        if (Test-Path -LiteralPath $counters) {
            $loaded = Get-Content -LiteralPath $counters -Raw | ConvertFrom-Json
            foreach ($property in $loaded.PSObject.Properties) { $state[$property.Name] = [int]$property.Value }
        }
        if ($state.ContainsKey($Url)) { $seen = $state[$Url] }
        $status = 0
        $endpoints = if ($planKeys -contains 'endpoints') { $plan.endpoints } else { $null }
        if ($null -ne $endpoints -and (@($endpoints.PSObject.Properties | ForEach-Object { $_.Name }) -contains $Url)) {
            $values = @($endpoints.$Url)
            $index = $seen
            if ($index -ge $values.Count) { $index = $values.Count - 1 }
            $status = [int]$values[$index]
        }
        $state[$Url] = $seen + 1
        Set-Content -LiteralPath $counters -Value ($state | ConvertTo-Json -Compress) -Encoding utf8
        Add-Content -LiteralPath $env:NILO_FAKE_BOUNDARY_LOG -Value (
            @{ call = 'TestEndpoint'; detail = @{ url = $Url; status = $status } } |
                ConvertTo-Json -Compress -Depth 6)
        return $status
    }
    TestPortOwner = {
        param([string]$Scope, [int]$Port)
        $plan = Get-Content -LiteralPath $env:NILO_FAKE_PLAN -Raw | ConvertFrom-Json
        $planKeys = @($plan.PSObject.Properties | ForEach-Object { $_.Name })
        Add-Content -LiteralPath $env:NILO_FAKE_BOUNDARY_LOG -Value (
            @{ call = 'TestPortOwner'; detail = @{ scope = $Scope; port = $Port } } |
                ConvertTo-Json -Compress -Depth 6)
        $ports = if ($planKeys -contains 'ports') { $plan.ports } else { $null }
        $key = "${Scope}:${Port}"
        if ($null -eq $ports -or (@($ports.PSObject.Properties | ForEach-Object { $_.Name }) -notcontains $key)) { return $null }
        $owner = $ports.$key
        return @{ Pid = [int]$owner.pid; Name = [string]$owner.name }
    }
    StartComponent = {
        param([string]$Component, $Specification)
        Add-Content -LiteralPath $env:NILO_FAKE_BOUNDARY_LOG -Value (
            @{
                call = 'StartComponent'
                detail = @{
                    component = $Component
                    filePath = [string]$Specification.FilePath
                    arguments = @($Specification.ArgumentList) -join ' | '
                }
            } | ConvertTo-Json -Compress -Depth 6)
    }
    OpenBrowser = {
        param([string]$Url)
        Add-Content -LiteralPath $env:NILO_FAKE_BOUNDARY_LOG -Value (
            @{ call = 'OpenBrowser'; detail = @{ url = $Url } } | ConvertTo-Json -Compress -Depth 6)
    }
}
"""


class LifecycleRun:
    """One completed invocation of the lifecycle entry point."""

    def __init__(self, completed, calls, wsl_log):
        self.returncode = completed.returncode
        self.output = completed.stdout + completed.stderr
        self.calls = calls
        self.wsl_log = wsl_log

    def calls_named(self, name: str) -> list[dict]:
        return [call["detail"] for call in self.calls if call["call"] == name]

    def started(self) -> set[str]:
        return {call["component"] for call in self.calls_named("StartComponent")}


@unittest.skipUnless(os.name == "nt", "the development lifecycle is a Windows entry point")
class DevelopmentLifecycleTests(unittest.TestCase):
    def test_entry_point_and_library_exist(self) -> None:
        self.assertTrue(ENTRY_POINT.is_file(), f"missing lifecycle entry point: {ENTRY_POINT}")
        self.assertTrue(LIBRARY.is_file(), f"missing lifecycle library: {LIBRARY}")

    # ---- command surface and options ----

    def test_entry_point_accepts_its_commands_and_rejects_bad_input(self) -> None:
        for command in ("start", "status"):
            # A short readiness bound: this test is about argument acceptance,
            # and the stopped plan never answers, so the default would make it
            # sit out the full timeout.
            run = self._run(command, "-ReadyTimeoutSeconds", "1", plan=self._plan_stopped())
            self.assertNotEqual(run.returncode, EXIT_USAGE, f"{command} rejected: {run.output}")

        for bad in (("restart",), ("stop",), ("logs",)):
            run = self._run(*bad, plan=self._plan_stopped())
            self.assertEqual(run.returncode, EXIT_USAGE, run.output)
            self.assertIn(bad[0], run.output)

        for bad_option in (("status", "-BackendPort", "99999"),
                           ("status", "-Scenario", "nonsense"),
                           ("status", "-BackendPort", "5173")):
            run = self._run(*bad_option, plan=self._plan_stopped())
            self.assertEqual(run.returncode, EXIT_USAGE, run.output)

    def test_options_default_to_the_documented_values(self) -> None:
        run = self._run("status", plan=self._plan_stopped())
        self.assertEqual(run.returncode, EXIT_STOPPED, run.output)
        self.assertIn("scenario: healthy", run.output)
        self.assertIn("backend port: 8090", run.output)
        self.assertIn("frontend port: 5173", run.output)
        self.assertIn("distribution: Ubuntu-24.04", run.output)

    # ---- status classification, from endpoints only ----

    def test_status_classifies_from_what_the_endpoints_answer(self) -> None:
        stopped = self._run("status", plan=self._plan_stopped())
        self.assertEqual(stopped.returncode, EXIT_STOPPED, stopped.output)
        self.assertIn("state: stopped", stopped.output)

        healthy = self._run("status", plan=self._plan_answering())
        self.assertEqual(healthy.returncode, EXIT_OK, healthy.output)
        self.assertIn("state: healthy", healthy.output)
        self.assertRegex(healthy.output, r"backend: answering")
        self.assertRegex(healthy.output, r"frontend: answering")

        plan = self._plan_answering()
        plan["endpoints"][FRONTEND_URL] = [0]
        partial = self._run("status", plan=plan)
        self.assertEqual(partial.returncode, EXIT_PARTIAL, partial.output)
        self.assertIn("state: partially running", partial.output)
        self.assertRegex(partial.output, r"frontend: not answering")

        conflicted = self._run("status", plan=self._plan_foreign_port())
        self.assertEqual(conflicted.returncode, EXIT_PORT_CONFLICT, conflicted.output)
        self.assertIn("state: port conflict", conflicted.output)

    def test_a_backend_that_answers_something_else_is_not_accepted(self) -> None:
        """Only 401 proves the port is Bench Core rather than another server."""
        plan = self._plan_answering()
        plan["endpoints"][BACKEND_STATUS_URL] = [200]
        run = self._run("status", plan=plan)
        self.assertNotEqual(run.returncode, EXIT_OK, run.output)
        self.assertRegex(run.output, r"backend: not answering")

    # ---- start ----

    def test_start_reports_success_only_after_both_endpoints_answer(self) -> None:
        plan = self._plan_stopped()
        plan["endpoints"] = {BACKEND_STATUS_URL: [0, 0, 401], FRONTEND_URL: [0, 200]}
        run = self._run("start", plan=plan)

        self.assertEqual(run.returncode, EXIT_OK, run.output)
        probes = [c for c in run.calls_named("TestEndpoint") if c["url"] == BACKEND_STATUS_URL]
        self.assertGreaterEqual(len(probes), 3)
        self.assertEqual(probes[-1]["status"], 401)

    def test_start_reports_a_timeout_naming_the_silent_component(self) -> None:
        plan = self._plan_stopped()
        plan["endpoints"] = {BACKEND_STATUS_URL: [401], FRONTEND_URL: [0]}
        run = self._run("start", "-ReadyTimeoutSeconds", "1", plan=plan)

        self.assertEqual(run.returncode, EXIT_PARTIAL, run.output)
        self.assertRegex(run.output.lower(), r"frontend.*timed out")
        self.assertIn("window", run.output)

    def test_start_leaves_an_already_answering_topology_alone(self) -> None:
        run = self._run("start", plan=self._plan_answering())

        self.assertEqual(run.returncode, EXIT_OK, run.output)
        self.assertEqual(run.started(), set(), "it started something that was already answering")
        self.assertRegex(run.output.lower(), r"already answering")

    def test_start_completes_a_half_up_topology_without_duplicating_it(self) -> None:
        plan = self._plan_stopped()
        plan["endpoints"] = {BACKEND_STATUS_URL: [401], FRONTEND_URL: [0, 200]}
        run = self._run("start", plan=plan)

        self.assertEqual(run.returncode, EXIT_OK, run.output)
        self.assertEqual(run.started(), {"frontend"}, "it restarted the answering backend")

    def test_port_conflict_stops_the_start_and_leaves_the_holder_alone(self) -> None:
        run = self._run("start", plan=self._plan_foreign_port())

        self.assertEqual(run.returncode, EXIT_PORT_CONFLICT, run.output)
        self.assertEqual(run.started(), set(), "it launched despite a conflict")
        self.assertIn("5173", run.output)
        self.assertIn("4242", run.output)
        self.assertRegex(run.output, r"left running|will not stop")

    # ---- what start prints ----

    def test_successful_start_prints_endpoints_selection_and_follow_ups(self) -> None:
        plan = self._plan_stopped()
        plan["endpoints"] = {
            "http://127.0.0.1:9099/api/v1/status": [0, 401],
            "http://127.0.0.1:5199/": [0, 200],
        }
        run = self._run(
            "start", "-BackendPort", "9099", "-FrontendPort", "5199",
            "-Scenario", "unavailable", plan=plan,
        )

        self.assertEqual(run.returncode, EXIT_OK, run.output)
        self.assertIn("http://127.0.0.1:9099", run.output)
        self.assertIn("http://127.0.0.1:5199", run.output)
        self.assertIn("Ubuntu-24.04", run.output)
        self.assertIn("unavailable", run.output)
        self.assertIn("issue-token", run.output)
        self.assertIn("nilo-dev.ps1' status", run.output)
        self.assertRegex(run.output, r"clos(e|ing) its window|Ctrl\+C")

    def test_printed_commands_are_portable_and_quoted(self) -> None:
        # Driven through a real start, not the already-answering short circuit,
        # so the token command is printed and can be checked for quoting too.
        run = self._run("start", "-Distribution", "Ubuntu Dev", plan=self._plan_becoming_ready())

        self.assertEqual(run.returncode, EXIT_OK, run.output)
        self.assertIn("-Distribution 'Ubuntu Dev'", run.output)
        self.assertNotRegex(run.output, r"-Distribution Ubuntu Dev(?!')")
        token = next(line for line in run.output.splitlines() if "issue-token" in line)
        self.assertIn("-d 'Ubuntu Dev'", token)

    def test_token_command_uses_the_translated_checkout_path(self) -> None:
        run = self._run("start", plan=self._plan_becoming_ready())

        self.assertIn("/mnt/z/elsewhere/nilo-checkout/05_Software/core", run.output)

    def test_browser_opens_only_when_requested(self) -> None:
        plan = self._plan_becoming_ready()

        quiet = self._run("start", plan=plan)
        self.assertEqual(quiet.calls_named("OpenBrowser"), [])

        asked = self._run("start", "-OpenBrowser", plan=plan)
        self.assertEqual(asked.calls_named("OpenBrowser"), [{"url": FRONTEND_URL}])

    def test_an_already_answering_start_still_prints_the_token_command(self) -> None:
        """Every successful start prints it, including the one that starts nothing."""
        run = self._run("start", plan=self._plan_answering())

        self.assertEqual(run.returncode, EXIT_OK, run.output)
        self.assertEqual(run.started(), set())
        self.assertIn("issue-token", run.output)
        self.assertIn("/home/dev/.local/state/nilo-development/", run.output)

    # ---- the real launcher and the real socket parser ----

    def test_the_launcher_preserves_argument_boundaries(self) -> None:
        """`Start-Process -ArgumentList` joins an array without quoting anything.

        Measured before the fix: three elements, one of them a multi-word shell
        script, arrived at the child as nine arguments, so `bash -lc` received
        only `mkdir` as its command and the backend never started.
        """
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            target = root / "print-arguments.ps1"
            target.write_text(
                'foreach ($argument in $args) { Write-Output "[$argument]" }\n',
                encoding="utf-8",
            )
            received = root / "received.txt"
            command_line = root / "command-line.txt"
            script = (
                ". $env:NILO_PROBE_LIBRARY\n"
                "$arguments = @('-NoProfile','-File',$env:NILO_PROBE_TARGET,"
                "'-d','Ubuntu Dev','--','bash','-lc','mkdir -p a && cd b')\n"
                "$line = ConvertTo-NiloWindowsCommandLine -Arguments $arguments\n"
                "Set-Content -LiteralPath $env:NILO_PROBE_COMMAND_LINE -Value $line -Encoding utf8\n"
                "Start-Process -FilePath 'powershell' -ArgumentList $line -NoNewWindow -Wait "
                "-RedirectStandardOutput $env:NILO_PROBE_OUTPUT\n"
            )
            completed = self._probe(root, script, {
                "NILO_PROBE_TARGET": str(target),
                "NILO_PROBE_OUTPUT": str(received),
                "NILO_PROBE_COMMAND_LINE": str(command_line),
            })
            self.assertEqual(completed.returncode, 0, completed.stdout + completed.stderr)

            # Compared line by line, never on whitespace: splitting on spaces
            # would make the split-apart failure look identical to the fix.
            self.assertEqual(
                [line for line in received.read_text(encoding="utf-8").splitlines() if line],
                ["[-d]", "[Ubuntu Dev]", "[--]", "[bash]", "[-lc]", "[mkdir -p a && cd b]"],
                "argument boundaries were lost between the command and its child",
            )
            self.assertIn('"mkdir -p a && cd b"', command_line.read_text(encoding="utf-8"))

        # The real launcher must use the quoter rather than hand over the array.
        library = LIBRARY.read_text(encoding="utf-8")
        self.assertRegex(library, r"\$commandLine = ConvertTo-NiloWindowsCommandLine")
        self.assertNotRegex(library, r"-ArgumentList \$Specification\.ArgumentList")

    def test_a_listening_socket_is_read_as_an_identifier_and_a_name(self) -> None:
        """`-match` overwrites $Matches, so the order of the two reads matters."""
        listening = (
            'LISTEN 0 4096 127.0.0.1:8090 0.0.0.0:* users:(("nilo",pid=716,fd=7))'
        )
        self.assertEqual(self._listener(listening), "716|nilo")
        self.assertEqual(self._listener("LISTEN 0 4096 127.0.0.1:8090 0.0.0.0:*"), "none")
        self.assertEqual(self._listener(""), "none")

    def test_browser_opens_when_requested_against_an_already_healthy_topology(self) -> None:
        """Showing the interface does not depend on having started it."""
        asked = self._run("start", "-OpenBrowser", plan=self._plan_answering())

        self.assertEqual(asked.returncode, EXIT_OK, asked.output)
        self.assertEqual(asked.started(), set())
        self.assertEqual(asked.calls_named("OpenBrowser"), [{"url": FRONTEND_URL}])

    # ---- WSL-native runtime state ----

    def test_backend_data_directory_is_linux_native(self) -> None:
        run = self._run("start", plan=self._plan_becoming_ready())

        backend = [c for c in run.calls_named("StartComponent") if c["component"] == "backend"]
        self.assertEqual(len(backend), 1, run.calls)
        arguments = backend[0]["arguments"]
        self.assertIn("--data-dir", arguments)
        # The checkout itself is legitimately on the Windows mount - that is
        # where the source lives. It is the runtime data directory that must
        # not be, so the assertion reads that argument rather than the whole
        # command line.
        data_dir = re.search(r"--data-dir '([^']*)'", arguments)
        self.assertIsNotNone(data_dir, arguments)
        self.assertTrue(
            data_dir.group(1).startswith("/home/dev/.local/state/nilo-development/"),
            data_dir.group(1),
        )
        self.assertNotIn("/mnt/", data_dir.group(1))

    def test_a_windows_mounted_home_is_refused(self) -> None:
        run = self._run("start", plan=self._plan_stopped(), wsl_home="/mnt/f/home")

        self.assertNotEqual(run.returncode, EXIT_OK)
        self.assertEqual(run.started(), set())
        self.assertIn("/mnt/", run.output)

    def test_a_diagnostic_line_from_wsl_does_not_corrupt_the_data_directory(self) -> None:
        """The WSL helper merges stderr into its output; only the path counts."""
        run = self._run(
            "start", plan=self._plan_becoming_ready(),
            wsl_noise="wsl: Detected localhost proxy configuration",
        )

        self.assertEqual(run.returncode, EXIT_OK, run.output)
        backend = [c for c in run.calls_named("StartComponent") if c["component"] == "backend"][0]
        self.assertIn("/home/dev/.local/state/nilo-development/", backend["arguments"])
        self.assertNotIn("localhost proxy", backend["arguments"])

    def test_shell_paths_are_quoted_against_apostrophes(self) -> None:
        library = LIBRARY.read_text(encoding="utf-8")
        entry = ENTRY_POINT.read_text(encoding="utf-8")
        self.assertIn("ConvertTo-NiloShellLiteral", library)
        # Every path interpolated into the WSL script goes through the quoter.
        self.assertNotRegex(entry, r'"[^"\n]*\$\(Get-CheckoutInWsl\)[^"\n]*"\s*\)?\s*,?\s*\n\s*"cd ')
        self.assertRegex(entry, r"\$assistant = ConvertTo-NiloShellLiteral")
        self.assertRegex(entry, r"\$binary = ConvertTo-NiloShellLiteral")
        self.assertRegex(entry, r"\$directory = ConvertTo-NiloShellLiteral")

    # ---- no supervision surface ----

    def test_the_lifecycle_never_signals_or_tracks_a_process(self) -> None:
        """The safety property is structural: there is no code that can kill."""
        sources = "\n".join(p.read_text(encoding="utf-8") for p in (ENTRY_POINT, LIBRARY))
        for forbidden in ("Stop" + "-Process", "task" + "kill", "p" + "kill",
                          "kill -TERM", "kill -KILL", "Get-CimInstance Win32_Process"):
            self.assertNotIn(forbidden, sources, f"the command can still signal a process: {forbidden}")
        self.assertNotIn("instance.json", sources, "the command still keeps ownership metadata")

    def test_the_lifecycle_writes_no_files_into_the_repository(self) -> None:
        before = self._git_status()
        for command in (("start",), ("status",)):
            self._run(*command, plan=self._plan_answering())
        self.assertEqual(before, self._git_status(), "the lifecycle changed what Git reports")
        self.assertFalse((TOOLS_ROOT / "state").exists(), "a development-state directory was created")

    def test_exit_codes_are_distinct_and_documented(self) -> None:
        observed = {
            "healthy": self._run("status", plan=self._plan_answering()).returncode,
            "usage": self._run("restart", plan=self._plan_stopped()).returncode,
            "conflict": self._run("start", plan=self._plan_foreign_port()).returncode,
            "stopped": self._run("status", plan=self._plan_stopped()).returncode,
        }
        self.assertEqual(observed["healthy"], EXIT_OK)
        self.assertEqual(observed["usage"], EXIT_USAGE)
        self.assertEqual(observed["conflict"], EXIT_PORT_CONFLICT)
        self.assertEqual(observed["stopped"], EXIT_STOPPED)
        self.assertEqual(len(set(observed.values())), len(observed), observed)

        readme = (TOOLS_ROOT / "README.md").read_text(encoding="utf-8")
        for code in (EXIT_OK, EXIT_UNEXPECTED, EXIT_USAGE, EXIT_PORT_CONFLICT,
                     EXIT_STOPPED, EXIT_PARTIAL):
            self.assertRegex(readme, rf"\|\s*`?{code}`?\s*\|", f"exit code {code} is undocumented")

    def test_boundaries_are_injectable_and_this_suite_uses_them(self) -> None:
        entry = ENTRY_POINT.read_text(encoding="utf-8")
        for option in ("$WslExecutable", "$BoundaryScript"):
            self.assertIn(option, entry, f"missing injection option {option}")

        suite = Path(__file__).read_text(encoding="utf-8")
        for forbidden in ("Stop" + "-Process", "task" + "kill", "os." + "kill",
                          "p" + "kill", "wsl" + ".exe"):
            self.assertNotIn(forbidden, suite, f"the suite reaches a real boundary: {forbidden}")

    # ---- existing behavior must survive ----

    def test_existing_bootstrap_entry_points_are_unchanged_in_shape(self) -> None:
        for relative in (
            "bootstrap/bootstrap.ps1", "bootstrap/bootstrap-development.ps1",
            "bootstrap/bootstrap-frontend.ps1", "bootstrap/setup-wsl.ps1",
            "scripts/doctor.ps1", "scripts/doctor-wsl.ps1",
            "env/with-tools.ps1", "lib/Wsl.ps1",
        ):
            self.assertTrue((TOOLS_ROOT / relative).is_file(), f"missing {relative}")

        combined = (TOOLS_ROOT / "bootstrap" / "bootstrap-development.ps1").read_text(encoding="utf-8")
        self.assertNotIn("nilo-dev.ps1", combined, "bootstrap must not start the topology")

    # ---------------- harness ----------------

    def _git_status(self) -> str:
        return subprocess.run(
            ["git", "status", "--porcelain"], cwd=REPOSITORY_ROOT,
            text=True, capture_output=True, check=False,
        ).stdout

    def _probe(self, root: Path, script: str, environment: dict):
        """Run a PowerShell fragment against the real library, strictly."""
        probe = root / "probe.ps1"
        probe.write_text(
            "Set-StrictMode -Version Latest\n$ErrorActionPreference = 'Stop'\n" + script,
            encoding="utf-8",
        )
        combined = os.environ.copy()
        combined["NILO_PROBE_LIBRARY"] = str(LIBRARY)
        combined.update(environment)
        return subprocess.run(
            ["powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", str(probe)],
            cwd=REPOSITORY_ROOT, env=combined, text=True, capture_output=True, check=False,
        )

    def _listener(self, socket_line: str) -> str:
        with tempfile.TemporaryDirectory() as temporary:
            completed = self._probe(
                Path(temporary),
                ". $env:NILO_PROBE_LIBRARY\n"
                # Windows reports an empty variable as unset, so it is restored
                # to a string before the parser sees it.
                "$line = $env:NILO_PROBE_LINE\n"
                "if ($null -eq $line) { $line = '' }\n"
                "$listener = Get-NiloDevListener -SocketLine $line\n"
                "if ($null -eq $listener) { Write-Output 'none' }\n"
                "else { Write-Output ('{0}|{1}' -f $listener.Pid, $listener.Name) }\n",
                {"NILO_PROBE_LINE": socket_line},
            )
            self.assertEqual(completed.returncode, 0, completed.stdout + completed.stderr)
            return completed.stdout.strip()

    def _run(self, command: str, *arguments: str, plan=None, wsl_home=None, wsl_noise=None):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            (root / "fake-wsl.ps1").write_text(FAKE_WSL, encoding="utf-8")
            (root / "fake-boundaries.ps1").write_text(FAKE_BOUNDARIES, encoding="utf-8")
            (root / "plan.json").write_text(json.dumps(plan or {}), encoding="utf-8")
            boundary_log = root / "boundary.jsonl"
            wsl_log = root / "wsl.log"
            for path in (boundary_log, wsl_log):
                path.write_text("", encoding="utf-8")

            # A readiness bound is always supplied unless the test sets its own,
            # so no test can ever sit out the production default.
            if "-ReadyTimeoutSeconds" not in arguments:
                arguments = (*arguments, "-ReadyTimeoutSeconds", "5")

            environment = os.environ.copy()
            environment["NILO_FAKE_PLAN"] = str(root / "plan.json")
            environment["NILO_FAKE_BOUNDARY_LOG"] = str(boundary_log)
            environment["NILO_FAKE_WSL_LOG"] = str(wsl_log)
            if wsl_home:
                environment["NILO_FAKE_WSL_HOME"] = wsl_home
            if wsl_noise:
                environment["NILO_FAKE_WSL_NOISE"] = wsl_noise

            completed = subprocess.run(
                [
                    "powershell", "-NoProfile", "-ExecutionPolicy", "Bypass",
                    "-File", str(ENTRY_POINT), command,
                    "-WslExecutable", str(root / "fake-wsl.ps1"),
                    "-BoundaryScript", str(root / "fake-boundaries.ps1"),
                    *arguments,
                ],
                cwd=REPOSITORY_ROOT, env=environment,
                text=True, capture_output=True, check=False,
            )
            calls = [
                json.loads(line)
                for line in boundary_log.read_text(encoding="utf-8").splitlines()
                if line.strip()
            ]
            return LifecycleRun(completed, calls, wsl_log.read_text(encoding="utf-8"))

    # ---------------- plans ----------------

    def _plan_stopped(self) -> dict:
        return {"ports": {}, "endpoints": {}}

    def _plan_answering(self) -> dict:
        return {"ports": {}, "endpoints": {BACKEND_STATUS_URL: [401], FRONTEND_URL: [200]}}

    def _plan_becoming_ready(self) -> dict:
        """Neither endpoint answers the opening probe, then both come up.

        A plan that answers immediately would make start take its
        already-answering short circuit and launch nothing, so any test about
        launching has to begin unavailable.
        """
        return {"ports": {}, "endpoints": {BACKEND_STATUS_URL: [0, 401], FRONTEND_URL: [0, 200]}}

    def _plan_foreign_port(self) -> dict:
        return {
            "ports": {"windows:5173": {"pid": 4242, "name": "someone-elses-server"}},
            "endpoints": {},
        }


if __name__ == "__main__":
    unittest.main()
