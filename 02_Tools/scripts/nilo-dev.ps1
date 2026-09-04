<#
.SYNOPSIS
Start the whole local NILO development topology from one command, and report
whether it is answering.

.DESCRIPTION
Launches the Go Bench Core under a WSL2 distribution on the simulation profile
and the browser interface development server on native Windows, each in its own
visible console window. Bench Core's SQLite database and artifacts stay on the
WSL Linux filesystem.

This command supervises nothing. It records no process identifiers, keeps no
state files, and never signals or terminates a process. Stop a component by
closing its window or pressing Ctrl+C in it. Its output is in its window, so
there is no separate log command.

It also never issues, captures, stores, or prints a bearer token. It prints the
exact command a developer runs to issue one.

Supported platform: Windows x86_64 with a WSL2 distribution prepared by
02_Tools/bootstrap/bootstrap-development.ps1.

Exit codes: 0 answering, 1 unexpected failure, 2 usage error, 3 port conflict,
4 stopped, 5 partially running.

.EXAMPLE
.\02_Tools\scripts\nilo-dev.ps1 start

.EXAMPLE
.\02_Tools\scripts\nilo-dev.ps1 start -Scenario unavailable -BackendPort 9090
#>

[CmdletBinding()]
param(
    [Parameter(Position = 0)]
    [string]$Command = 'status',

    # The WSL2 distribution that runs Bench Core. Defaults to the only
    # installed distribution when exactly one exists.
    [string]$Distribution,

    # The deterministic simulation scenario Bench Core serves.
    [string]$Scenario = 'healthy',

    [int]$BackendPort = 8090,
    [int]$FrontendPort = 5173,

    # Open the browser interface after a successful start. Off by default so a
    # command run from a terminal or a script never seizes the screen.
    [switch]$OpenBrowser,

    [int]$ReadyTimeoutSeconds = 180,

    # Injection points. The defaults are the real boundaries; the automated
    # tests replace them so no test launches a window or touches a real port.
    [string]$WslExecutable = 'wsl.exe',
    [string]$BoundaryScript
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$toolsRoot = Split-Path -Parent $PSScriptRoot
$repositoryRoot = Split-Path -Parent $toolsRoot
. (Join-Path $toolsRoot 'lib\Wsl.ps1')
. (Join-Path $toolsRoot 'lib\DevLifecycle.ps1')

function Write-UsageError {
    param([string]$Message)
    Write-Host $Message
    Write-Host ''
    Write-Host 'usage: nilo-dev.ps1 <start|status> [options]'
    Write-Host '  -Distribution <name>              WSL2 distribution running Bench Core'
    Write-Host '  -Scenario <healthy|unavailable>   deterministic simulation scenario'
    Write-Host '  -BackendPort <1-65535>            Bench Core loopback port (default 8090)'
    Write-Host '  -FrontendPort <1-65535>           interface port (default 5173)'
    Write-Host '  -OpenBrowser                      open the interface after start'
    Write-Host '  -ReadyTimeoutSeconds <n>          readiness wait, default 180'
    Write-Host ''
    Write-Host 'Stop a component by closing its window or pressing Ctrl+C in it.'
    exit (Get-NiloDevExitCode -Name 'Usage')
}

# ---- validate the command surface before touching anything ----

$knownCommands = @('start', 'status')
$requestedCommand = $Command.ToLowerInvariant()
if ($knownCommands -notcontains $requestedCommand) {
    Write-UsageError "Unknown command '$Command'."
}
foreach ($port in @(
        @{ Name = 'BackendPort'; Value = $BackendPort },
        @{ Name = 'FrontendPort'; Value = $FrontendPort })) {
    if ($port.Value -lt 1 -or $port.Value -gt 65535) {
        Write-UsageError "-$($port.Name) must be between 1 and 65535, not $($port.Value)."
    }
}
if ($BackendPort -eq $FrontendPort) {
    Write-UsageError "-BackendPort and -FrontendPort must differ; both were $BackendPort."
}
if (@('healthy', 'unavailable') -notcontains $Scenario.ToLowerInvariant()) {
    Write-UsageError "Unknown -Scenario '$Scenario'; expected healthy or unavailable."
}
if ($ReadyTimeoutSeconds -lt 1) {
    Write-UsageError "-ReadyTimeoutSeconds must be at least 1, not $ReadyTimeoutSeconds."
}

# ---- resolve the topology ----

function Resolve-Distribution {
    if ($Distribution) { return $Distribution }
    $available = @(Get-NiloWslDistributions -WslExecutable $WslExecutable -AllowUnavailable)
    if ($available.Count -eq 1) { return $available[0] }
    if ($available.Count -eq 0) {
        throw 'No WSL distribution was found. Run .\02_Tools\bootstrap\bootstrap-development.ps1 first.'
    }
    throw ("Several WSL distributions are installed ({0}); choose one with -Distribution." -f ($available -join ', '))
}

$settings = @{
    Distribution = $null
    Scenario     = $Scenario.ToLowerInvariant()
    BackendPort  = $BackendPort
    FrontendPort = $FrontendPort
}

try {
    $settings.Distribution = Resolve-Distribution
}
catch {
    Write-Host $_.Exception.Message
    exit (Get-NiloDevExitCode -Name 'Unexpected')
}

$instanceId = Get-NiloDevInstanceId -RepositoryRoot $repositoryRoot
$endpoints = Get-NiloDevEndpoints -Settings $settings
$script:checkoutInWsl = $null

$boundaries = New-NiloDevBoundaries -WslExecutable $WslExecutable -Distribution $settings.Distribution
if ($BoundaryScript) {
    if (-not (Test-Path -LiteralPath $BoundaryScript -PathType Leaf)) {
        Write-Host "The boundary script '$BoundaryScript' does not exist."
        exit (Get-NiloDevExitCode -Name 'Usage')
    }
    $boundaries = Merge-NiloDevBoundaries -Default $boundaries -Override (& $BoundaryScript)
}

function Get-CheckoutInWsl {
    # Translated through the distribution rather than assumed: this repository
    # is not always checked out at the same drive or path.
    if (-not $script:checkoutInWsl) {
        $script:checkoutInWsl = ConvertTo-NiloWslPath -WslExecutable $WslExecutable `
            -Distribution $settings.Distribution -WindowsPath $repositoryRoot
    }
    return $script:checkoutInWsl
}

function Get-StatusCommand {
    $script = ConvertTo-NiloPowerShellLiteral -Value '.\02_Tools\scripts\nilo-dev.ps1'
    $suffix = " -Distribution $(ConvertTo-NiloPowerShellLiteral -Value $settings.Distribution)"
    if ($settings.BackendPort -ne 8090) { $suffix += " -BackendPort $($settings.BackendPort)" }
    if ($settings.FrontendPort -ne 5173) { $suffix += " -FrontendPort $($settings.FrontendPort)" }
    return "& $script status$suffix"
}

function Get-TokenCommand {
    param([string]$DataDirectory)
    $assistant = "$(Get-CheckoutInWsl)/05_Software/nilo_assistant"
    return ("wsl.exe -d {0} --cd {1} -- ../../02_Tools/env/with-tools.sh go run ./cmd/nilo development issue-token --data-dir {2}" -f `
        (ConvertTo-NiloPowerShellLiteral -Value $settings.Distribution),
        (ConvertTo-NiloPowerShellLiteral -Value $assistant),
        (ConvertTo-NiloShellLiteral -Value $DataDirectory))
}

# ---- commands ----

function Invoke-Status {
    $result = Get-NiloDevState -Boundaries $boundaries -Settings $settings
    Write-NiloDevStateReport -Result $result -Settings $settings
    if ($result.State -ne 'healthy') {
        Write-Host ''
        Write-Host 'Start what is missing with:'
        Write-Host "  $((Get-StatusCommand) -replace ' status', ' start')"
    }
    return (Get-NiloDevExitCode -Name $result.State)
}

function Start-Backend {
    param([string]$DataDirectory)

    $assistant = ConvertTo-NiloShellLiteral -Value "$(Get-CheckoutInWsl)/05_Software/nilo_assistant"
    $binary = ConvertTo-NiloShellLiteral -Value "$DataDirectory/bin/nilo"
    $directory = ConvertTo-NiloShellLiteral -Value $DataDirectory

    # Every interpolated path is shell-quoted, so a checkout under a directory
    # containing an apostrophe or a space cannot break the generated script.
    # The trailing pause keeps the window open with the error visible when the
    # build or the server fails, which is the whole point of a visible window.
    # Only the bin directory is created. An earlier version also ran
    # `mkdir -p <binary>/..`, which creates the binary's own path as a
    # directory first, so `go build -o` then had a directory in its way.
    $script = @(
        "mkdir -p $directory/bin",
        "cd $assistant",
        "../../02_Tools/env/with-tools.sh go build -o $binary ./cmd/nilo",
        # --development-administrator creates a first administrator with a fixed,
        # published password so a developer can sign in immediately. Bench Core
        # refuses the flag on any profile but simulation, and it never touches an
        # administrator that already exists.
        ("$binary serve --profile simulation --scenario {0} --data-dir {1} --bind 127.0.0.1:{2} --development-administrator" -f `
                $settings.Scenario, $directory, $settings.BackendPort)
    ) -join ' && '
    $script += "`nprintf '\n[nilo-dev] Bench Core has stopped. Press Enter to close this window.\n'`nread _"

    Write-Host "backend: opening a window for Bench Core in $($settings.Distribution) (scenario $($settings.Scenario))"
    & $boundaries.StartComponent 'backend' @{
        FilePath         = $WslExecutable
        ArgumentList     = @('-d', $settings.Distribution, '--', 'bash', '-lc', $script)
        WorkingDirectory = $repositoryRoot
    }
}

function Start-Frontend {
    $wrapper = Join-Path $toolsRoot 'env\with-tools.ps1'
    $gui = Join-Path $repositoryRoot '05_Software\nilo_gui'

    # The interface only ever requests origin-relative paths, so the development
    # server's proxy target has to match the backend port chosen here.
    $env:VITE_NILO_API_BASE_URL = "http://127.0.0.1:$($settings.BackendPort)"

    Write-Host "frontend: opening a window for the interface on port $($settings.FrontendPort)"
    & $boundaries.StartComponent 'frontend' @{
        FilePath         = 'powershell'
        ArgumentList     = @(
            '-NoExit', '-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', $wrapper,
            'npm.cmd', '--prefix', $gui, 'run', 'dev', '--',
            '--port', "$($settings.FrontendPort)", '--strictPort'
        )
        WorkingDirectory = $repositoryRoot
    }
}

function Wait-ComponentReady {
    param([string]$Name, [string]$Url)

    $deadline = (Get-Date).AddSeconds($ReadyTimeoutSeconds)
    while ($true) {
        if (Test-NiloDevComponentAnswering -Boundaries $boundaries -Component $Name -Url $Url) {
            Write-Host "$Name`: answering at $Url"
            return $true
        }
        if ((Get-Date) -ge $deadline) {
            Write-Host "$Name`: did not answer at $Url within $ReadyTimeoutSeconds seconds; it timed out."
            Write-Host "$Name`: its window is still open - read the error there, then close it."
            return $false
        }
        Start-Sleep -Milliseconds 500
    }
}

function Resolve-DataDirectory {
    $linuxHome = (Invoke-NiloWsl -WslExecutable $WslExecutable -Arguments @(
            '-d', $settings.Distribution, '--exec', 'sh', '-c', 'printf %s "$HOME" # NILO_HOME_PROBE'
        )).Output
    return Resolve-NiloDevLinuxDataDirectory -LinuxHome $linuxHome -InstanceId $instanceId
}

function Invoke-Start {
    $before = Get-NiloDevState -Boundaries $boundaries -Settings $settings

    # Checked before the home probe: when a port is already blocked there is
    # nothing to start, so there is no reason to question the distribution. A
    # healthy topology never carries a conflict, so this cannot pre-empt it.
    if ($before.Conflicts.Count -gt 0) {
        Write-Host 'state: port conflict'
        foreach ($conflict in $before.Conflicts) {
            Write-Host ("{0} port {1} is already held by process {2} ({3})." -f `
                    $conflict.Scope, $conflict.Port, $conflict.Pid, $conflict.Name)
        }
        Write-Host 'Those processes were not started by this command, so they have been left running and nothing was started.'
        Write-Host 'Choose another port with -BackendPort or -FrontendPort, or stop them yourself.'
        return (Get-NiloDevExitCode -Name 'PortConflict')
    }

    # Resolved before the already-answering short circuit as well, because a
    # successful start prints the token-issuance command on every path and that
    # command needs the data directory Bench Core was given.
    $dataDirectory = $null
    try {
        $dataDirectory = Resolve-DataDirectory
    }
    catch {
        Write-Host $_.Exception.Message
        return (Get-NiloDevExitCode -Name 'Unexpected')
    }

    if ($before.State -eq 'healthy') {
        Write-Host 'state: healthy'
        Write-Host 'Both components are already answering; nothing was started.'
        Write-StartSummary -DataDirectory $dataDirectory
        # -OpenBrowser is about showing the interface, not about having started
        # it, so it is honoured on this path too.
        if ($OpenBrowser) { & $boundaries.OpenBrowser $endpoints.Frontend }
        return (Get-NiloDevExitCode -Name 'Ok')
    }

    # Only what is missing is started, so a half-up topology is completed
    # rather than duplicated.
    if (-not $before.Components['backend'].Answering) { Start-Backend -DataDirectory $dataDirectory }
    if (-not $before.Components['frontend'].Answering) { Start-Frontend }

    $ready = $true
    if (-not (Wait-ComponentReady -Name 'backend' -Url $endpoints.Backend)) { $ready = $false }
    if (-not (Wait-ComponentReady -Name 'frontend' -Url $endpoints.Frontend)) { $ready = $false }

    if (-not $ready) {
        Write-Host ''
        Write-Host 'The topology did not come up. Each component it opened has its own window;'
        Write-Host 'read the error there and close the window when you are done.'
        return (Get-NiloDevExitCode -Name 'Partial')
    }

    Write-Host 'state: healthy'
    Write-StartSummary -DataDirectory $dataDirectory
    if ($OpenBrowser) { & $boundaries.OpenBrowser $endpoints.Frontend }
    return (Get-NiloDevExitCode -Name 'Ok')
}

function Write-StartSummary {
    param([string]$DataDirectory)

    Write-Host ''
    Write-Host "distribution: $($settings.Distribution)"
    Write-Host "scenario: $($settings.Scenario)"
    Write-Host "backend port: $($settings.BackendPort)"
    Write-Host "frontend port: $($settings.FrontendPort)"
    Write-Host "backend endpoint: http://127.0.0.1:$($settings.BackendPort)"
    Write-Host "browser URL: $($endpoints.Frontend)"
    Write-Host ''
    Write-Host 'Sign in with the development administrator:'
    Write-Host '  username: admin'
    Write-Host '  password: admin_password'
    Write-Host 'It is created on first start, only on the simulation profile, and only'
    Write-Host 'when no administrator exists yet. It is never created on any other profile.'
    if ($DataDirectory) {
        Write-Host ''
        Write-Host "backend data directory (WSL Linux filesystem): $DataDirectory"
        Write-Host ''
        Write-Host 'A development bearer token, if you want one instead, reads the bench at'
        Write-Host 'the Viewer level and carries no management permission:'
        Write-Host "  $(Get-TokenCommand -DataDirectory $DataDirectory)"
        Write-Host 'This command never stores or prints a token itself.'
    }
    Write-Host ''
    Write-Host 'Check it again with:'
    Write-Host "  $(Get-StatusCommand)"
    Write-Host 'Stop a component by closing its window or pressing Ctrl+C in it.'
}

# ---- dispatch ----

try {
    switch ($requestedCommand) {
        'start' { exit (Invoke-Start) }
        'status' { exit (Invoke-Status) }
    }
}
catch {
    Write-Host "The development lifecycle command failed unexpectedly: $($_.Exception.Message)"
    exit (Get-NiloDevExitCode -Name 'Unexpected')
}
