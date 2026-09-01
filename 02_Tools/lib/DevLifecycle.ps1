# Helpers for the NILO one-command development lifecycle.
#
# This component deliberately supervises nothing. It launches each part of the
# local topology into its own visible console window and answers questions
# about endpoints; it never records a process identifier, never keeps ownership
# metadata, and never signals or terminates a process. A developer stops a
# component by closing its window or pressing Ctrl+C.
#
# That is not a shortcut. Owning long-lived processes across Windows and a WSL2
# distribution means handling identifier reuse, process trees and two operating
# system namespaces, and it was the source of every defect this component has
# had. A command that terminates nothing cannot terminate the wrong thing.
#
# Supported platform: Windows x86_64 with a WSL2 distribution.
# External changes: starts development processes only through the injected
# launcher boundary. Writes no files.

Set-StrictMode -Version Latest

$script:NiloDevExit = @{
    Ok           = 0
    Unexpected   = 1
    Usage        = 2
    PortConflict = 3
    Stopped      = 4
    Partial      = 5
}

$script:NiloDevStateExit = @{
    'healthy'           = 0
    'stopped'           = 4
    'partially running' = 5
    'port conflict'     = 3
}

function Get-NiloDevExitCode {
    param([Parameter(Mandatory = $true)][string]$Name)

    if ($script:NiloDevStateExit.ContainsKey($Name)) { return $script:NiloDevStateExit[$Name] }
    if ($script:NiloDevExit.ContainsKey($Name)) { return $script:NiloDevExit[$Name] }
    return $script:NiloDevExit.Unexpected
}

function ConvertTo-NiloPowerShellLiteral {
    <#
    .SYNOPSIS
    Quote a value for PowerShell so a printed command runs exactly as printed.
    #>
    param([string]$Value)

    return "'" + ($Value -replace "'", "''") + "'"
}

function ConvertTo-NiloShellLiteral {
    <#
    .SYNOPSIS
    Quote a value for a POSIX shell.
    .DESCRIPTION
    Used for every path interpolated into the WSL command, so a checkout under
    a directory containing an apostrophe cannot break the generated script.
    #>
    param([string]$Value)

    return "'" + ($Value -replace "'", "'\''") + "'"
}

function ConvertTo-NiloWindowsArgument {
    <#
    .SYNOPSIS
    Quote one argument for a Windows command line.
    .DESCRIPTION
    `Start-Process -ArgumentList` takes an array but joins it with spaces
    without quoting anything, so an element containing a space arrives at the
    child as several arguments. Measured: passing three elements, one of them
    `mkdir -p a && cd b`, delivered nine arguments to the child. That silently
    truncated the backend's `bash -lc` script to `mkdir`.

    These are the rules CommandLineToArgvW reverses: a run of backslashes is
    literal unless a quote follows it, in which case the run is doubled and the
    quote escaped; a run before the closing quote is also doubled.
    #>
    param([Parameter(Mandatory = $true)][AllowEmptyString()][string]$Value)

    if ($Value.Length -gt 0 -and $Value -notmatch '[\s"]') { return $Value }

    $quoted = '"'
    $backslashes = 0
    foreach ($character in $Value.ToCharArray()) {
        if ($character -eq '\') { $backslashes++; continue }
        if ($character -eq '"') { $quoted += ('\' * (2 * $backslashes + 1)) + '"' }
        else { $quoted += ('\' * $backslashes) + $character }
        $backslashes = 0
    }
    return $quoted + ('\' * (2 * $backslashes)) + '"'
}

function ConvertTo-NiloWindowsCommandLine {
    <#
    .SYNOPSIS
    Join arguments into a command line that preserves their boundaries.
    #>
    param([Parameter(Mandatory = $true)][AllowEmptyCollection()][string[]]$Arguments)

    return (@($Arguments | ForEach-Object { ConvertTo-NiloWindowsArgument -Value $_ }) -join ' ')
}

function Get-NiloDevEndpoints {
    param([Parameter(Mandatory = $true)][hashtable]$Settings)

    return @{
        Backend  = "http://127.0.0.1:$($Settings.BackendPort)/api/v1/status"
        Frontend = "http://127.0.0.1:$($Settings.FrontendPort)/"
    }
}

function Test-NiloDevComponentAnswering {
    <#
    .SYNOPSIS
    Whether a component is answering on its endpoint.
    .DESCRIPTION
    Bench Core answers 401 to an unauthenticated status request. That proves it
    is listening and that it is Bench Core rather than some other server on the
    port, without this command ever obtaining or holding a bearer token.
    #>
    param(
        [Parameter(Mandatory = $true)][hashtable]$Boundaries,
        [Parameter(Mandatory = $true)][string]$Component,
        [Parameter(Mandatory = $true)][string]$Url
    )

    $status = [int](& $Boundaries.TestEndpoint $Url)
    if ($Component -eq 'backend') { return $status -eq 401 }
    return $status -eq 200
}

function Get-NiloDevListener {
    <#
    .SYNOPSIS
    The process identifier and name in one `ss -ltnHp` line, or $null.
    .DESCRIPTION
    A named function rather than inline matching so the WSL conflict path can be
    tested without a distribution. `-match` overwrites $Matches, so the
    identifier is read out before the name is matched; reading it afterwards
    converted the process name to an integer and turned a port conflict into an
    unexpected failure.
    #>
    param([Parameter(Mandatory = $true)][AllowEmptyString()][string]$SocketLine)

    if ($SocketLine -notmatch 'pid=(\d+)') { return $null }
    $identifier = [int]$Matches[1]
    $name = 'unknown'
    if ($SocketLine -match 'users:\(\("([^"]+)"') { $name = $Matches[1] }
    return @{ Pid = $identifier; Name = $name }
}

function Get-NiloDevState {
    <#
    .SYNOPSIS
    Classify the topology from what its endpoints answer.
    .DESCRIPTION
    Health is whether a component answers, not whether some process exists: a
    live process that is not serving is of no use to a developer, and a process
    identifier proves nothing about who owns it.
    #>
    param(
        [Parameter(Mandatory = $true)][hashtable]$Boundaries,
        [Parameter(Mandatory = $true)][hashtable]$Settings
    )

    $endpoints = Get-NiloDevEndpoints -Settings $Settings
    $components = [ordered]@{}
    $answering = 0

    foreach ($component in @('backend', 'frontend')) {
        $url = if ($component -eq 'backend') { $endpoints.Backend } else { $endpoints.Frontend }
        $isAnswering = Test-NiloDevComponentAnswering -Boundaries $Boundaries -Component $component -Url $url
        if ($isAnswering) { $answering++ }

        # A port that is held but not answering as expected belongs to
        # something else. It is reported, never touched.
        $foreign = $null
        if (-not $isAnswering) {
            $scope = if ($component -eq 'backend') { 'wsl' } else { 'windows' }
            $port = if ($component -eq 'backend') { $Settings.BackendPort } else { $Settings.FrontendPort }
            $owner = & $Boundaries.TestPortOwner $scope $port
            if ($null -ne $owner) {
                $foreign = @{ Scope = $scope; Port = $port; Pid = [int]$owner.Pid; Name = [string]$owner.Name }
            }
        }

        $components[$component] = @{ Answering = $isAnswering; Url = $url; Conflict = $foreign }
    }

    $conflicts = @($components.Values | Where-Object { $null -ne $_.Conflict } | ForEach-Object { $_.Conflict })
    $state = 'stopped'
    if ($answering -eq 2) { $state = 'healthy' }
    elseif ($answering -eq 1) { $state = 'partially running' }
    elseif ($conflicts.Count -gt 0) { $state = 'port conflict' }

    return @{ State = $state; Components = $components; Conflicts = $conflicts }
}

function Write-NiloDevStateReport {
    param(
        [Parameter(Mandatory = $true)][hashtable]$Result,
        [Parameter(Mandatory = $true)][hashtable]$Settings
    )

    Write-Host "state: $($Result.State)"
    Write-Host "distribution: $($Settings.Distribution)"
    Write-Host "scenario: $($Settings.Scenario)"
    Write-Host "backend port: $($Settings.BackendPort)"
    Write-Host "frontend port: $($Settings.FrontendPort)"

    foreach ($component in @('backend', 'frontend')) {
        $detail = $Result.Components[$component]
        $answering = if ($detail.Answering) { 'answering' } else { 'not answering' }
        Write-Host "${component}: ${answering} at $($detail.Url)"
    }

    foreach ($conflict in $Result.Conflicts) {
        Write-Host ("port conflict: {0} port {1} is held by process {2} ({3}), which this command did not start and will not stop" -f `
                $conflict.Scope, $conflict.Port, $conflict.Pid, $conflict.Name)
    }
}

function Resolve-NiloDevLinuxDataDirectory {
    <#
    .SYNOPSIS
    An absolute data directory on the WSL Linux filesystem.
    .DESCRIPTION
    A live SQLite database on a Windows drive mount has different locking and
    permission semantics from the Linux filesystem an appliance uses, so a home
    directory that resolves under a mount is refused rather than used.
    #>
    param(
        [Parameter(Mandatory = $true)][string]$LinuxHome,
        [Parameter(Mandatory = $true)][string]$InstanceId
    )

    # Only the last non-empty line is taken: the WSL helper merges standard
    # error into its output, so a first-contact diagnostic from wsl.exe would
    # otherwise be read as part of the path.
    $lines = @($LinuxHome -split "`r?`n" | ForEach-Object { $_.Trim() } | Where-Object { $_.Length -gt 0 })
    $resolved = if ($lines.Count -gt 0) { $lines[-1] } else { '' }

    if (-not $resolved.StartsWith('/')) {
        throw "The distribution reported a home directory that is not an absolute Linux path: '$resolved'."
    }
    if ($resolved -match '^/mnt/[a-zA-Z](/|$)') {
        throw "The distribution's home directory '$resolved' is on a Windows drive mount under /mnt/. Bench Core runtime state must stay on the WSL Linux filesystem, so nothing was started."
    }
    return "$resolved/.local/state/nilo-development/$InstanceId"
}

function Get-NiloDevFnv1a32 {
    <#
    .SYNOPSIS
    FNV-1a over a string, as an unsigned 32-bit value.
    .DESCRIPTION
    Arithmetic rather than a .NET digest so it also runs under constrained
    language mode, where the cryptography types are unavailable. The identity
    only has to be stable per checkout, not cryptographic.
    #>
    param([Parameter(Mandatory = $true)][string]$Text)

    # The mask is decimal because Windows PowerShell 5.1 parses the literal
    # 0xFFFFFFFF as -1, and the operands are [long] because multiplying a
    # [uint32] by an [int] promotes to [double] and loses the low bits.
    $mask = [long]4294967295
    $prime = [long]16777619
    $hash = [long]2166136261
    foreach ($character in $Text.ToCharArray()) {
        $hash = ($hash -bxor [long][int]$character) -band $mask
        $hash = ($hash * $prime) -band $mask
    }
    return [uint32]$hash
}

function Get-NiloDevInstanceId {
    <#
    .SYNOPSIS
    A stable short identity for one repository checkout.
    .DESCRIPTION
    Keeps two checkouts on the same machine from sharing a database, which is
    the only thing the identity is still used for.
    #>
    param([Parameter(Mandatory = $true)][string]$RepositoryRoot)

    $normalised = $RepositoryRoot.TrimEnd('\', '/').ToLowerInvariant()
    $first = Get-NiloDevFnv1a32 -Text $normalised
    $second = Get-NiloDevFnv1a32 -Text ($normalised + '|nilo')
    return ('{0:x8}{1:x8}' -f $first, $second).Substring(0, 12)
}

function New-NiloDevBoundaries {
    <#
    .SYNOPSIS
    The real endpoint, port, launcher and browser boundaries.
    #>
    param(
        [Parameter(Mandatory = $true)][string]$WslExecutable,
        [Parameter(Mandatory = $true)][string]$Distribution
    )

    # Held in script scope and read when a boundary runs: a PowerShell script
    # block is not a closure, so a local of this function would be gone by then.
    $script:NiloDevWsl = $WslExecutable
    $script:NiloDevDistribution = $Distribution

    return @{
        TestEndpoint = {
            param([string]$Url)

            try {
                $response = Invoke-WebRequest -Uri $Url -UseBasicParsing -TimeoutSec 3 `
                    -MaximumRedirection 0 -ErrorAction Stop
                return [int]$response.StatusCode
            }
            catch [System.Net.WebException] {
                $response = $_.Exception.Response
                if ($null -ne $response) { return [int]$response.StatusCode }
                return 0
            }
            catch {
                return 0
            }
        }

        TestPortOwner = {
            param([string]$Scope, [int]$Port)

            if ($Scope -eq 'windows') {
                $connection = Get-NetTCPConnection -LocalPort $Port -State Listen -ErrorAction SilentlyContinue |
                    Select-Object -First 1
                if ($null -eq $connection) { return $null }
                $owner = Get-Process -Id $connection.OwningProcess -ErrorAction SilentlyContinue
                return @{
                    Pid  = [int]$connection.OwningProcess
                    Name = if ($owner) { $owner.ProcessName } else { 'unknown' }
                }
            }

            $result = Invoke-NiloWsl -WslExecutable $script:NiloDevWsl -AllowFailure -Arguments @(
                '-d', $script:NiloDevDistribution, '--exec', 'sh', '-c',
                "ss -ltnHp 'sport = :$Port' 2>/dev/null | head -n 1"
            )
            if ($result.ExitCode -ne 0 -or [string]::IsNullOrWhiteSpace($result.Output)) { return $null }
            return (Get-NiloDevListener -SocketLine $result.Output)
        }

        StartComponent = {
            param([string]$Component, $Specification)

            # Its own visible window on purpose: the developer watches it, reads
            # its errors there, and stops it by closing it or pressing Ctrl+C.
            # Nothing about the process is recorded, so nothing can be signalled
            # by mistake later.
            #
            # One pre-quoted string, not the array: -ArgumentList joins an array
            # without quoting, which splits the backend's shell script and any
            # path containing a space.
            $commandLine = ConvertTo-NiloWindowsCommandLine -Arguments $Specification.ArgumentList
            Start-Process -FilePath $Specification.FilePath `
                -ArgumentList $commandLine `
                -WorkingDirectory $Specification.WorkingDirectory | Out-Null
        }

        OpenBrowser = {
            param([string]$Url)
            Start-Process $Url | Out-Null
        }
    }
}

function Merge-NiloDevBoundaries {
    <#
    .SYNOPSIS
    Overlay injected boundaries onto the real ones.
    #>
    param(
        [Parameter(Mandatory = $true)][hashtable]$Default,
        $Override
    )

    $merged = @{}
    foreach ($key in $Default.Keys) { $merged[$key] = $Default[$key] }
    if ($null -eq $Override) { return $merged }

    # Enumerated one at a time rather than through `.Properties.Name`, because
    # member enumeration over an empty collection throws under strict mode.
    $keys = if ($Override -is [hashtable]) {
        @($Override.Keys)
    }
    else {
        @($Override | ForEach-Object { $_.PSObject.Properties } | ForEach-Object { $_.Name })
    }
    foreach ($key in $keys) {
        if (-not $merged.ContainsKey($key)) {
            throw "The boundary script supplied an unknown boundary '$key'."
        }
        $merged[$key] = if ($Override -is [hashtable]) { $Override[$key] } else { $Override.$key }
    }
    return $merged
}
