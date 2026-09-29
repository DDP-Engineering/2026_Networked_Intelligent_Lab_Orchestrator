Set-StrictMode -Version Latest

function Resolve-NiloWslExecutable {
    param([Parameter(Mandatory = $true)][string]$WslExecutable)

    if (Test-Path -LiteralPath $WslExecutable -PathType Leaf) {
        return (Resolve-Path -LiteralPath $WslExecutable).Path
    }
    $command = Get-Command -Name $WslExecutable -ErrorAction SilentlyContinue
    if ($null -eq $command) {
        throw "WSL command '$WslExecutable' is unavailable. Install a supported Windows WSL package, then rerun this command."
    }
    return $command.Source
}

function Invoke-NiloWsl {
    param(
        [Parameter(Mandatory = $true)][string]$WslExecutable,
        [Parameter(Mandatory = $true)][string[]]$Arguments,
        [switch]$AllowFailure
    )

    $resolvedExecutable = Resolve-NiloWslExecutable -WslExecutable $WslExecutable
    $previousErrorActionPreference = $ErrorActionPreference
    try {
        $ErrorActionPreference = 'Continue'
        $output = @(& $resolvedExecutable @Arguments 2>&1)
    }
    finally {
        $ErrorActionPreference = $previousErrorActionPreference
    }
    $exitCode = if ($null -eq $LASTEXITCODE) { 0 } else { $LASTEXITCODE }
    $text = ($output | ForEach-Object { "$_" }) -join [Environment]::NewLine
    if ($exitCode -ne 0 -and -not $AllowFailure) {
        throw "WSL command failed with exit code ${exitCode}: $($Arguments -join ' ')${([Environment]::NewLine)}$text"
    }
    return @{
        ExitCode = $exitCode
        Output = $text
    }
}

function Get-NiloWslDistributions {
    param(
        [Parameter(Mandatory = $true)][string]$WslExecutable,
        [switch]$AllowUnavailable
    )

    $result = Invoke-NiloWsl -WslExecutable $WslExecutable -Arguments @('--list', '--quiet') -AllowFailure
    if ($result.ExitCode -ne 0) {
        if ($AllowUnavailable) {
            return @()
        }
        throw "WSL distribution discovery failed with exit code $($result.ExitCode): $($result.Output)"
    }
    return @(
        $result.Output -replace "`0", '' -split "`r?`n" |
            ForEach-Object { $_.Trim().TrimStart('*').Trim() } |
            Where-Object { $_.Length -gt 0 -and $_ -notin @('docker-desktop', 'docker-desktop-data') }
    )
}

function Get-NiloWslKernel {
    param(
        [Parameter(Mandatory = $true)][string]$WslExecutable,
        [Parameter(Mandatory = $true)][string]$Distribution
    )

    $result = Invoke-NiloWsl -WslExecutable $WslExecutable -Arguments @(
        '-d', $Distribution, '--exec', 'uname', '-r'
    )
    return $result.Output.Trim()
}

function Test-NiloWsl2Kernel {
    param([Parameter(Mandatory = $true)][string]$Kernel)

    return $Kernel -match '(?i)(microsoft-standard|wsl2)'
}

function ConvertTo-NiloWslPath {
    param(
        [Parameter(Mandatory = $true)][string]$WslExecutable,
        [Parameter(Mandatory = $true)][string]$Distribution,
        [Parameter(Mandatory = $true)][string]$WindowsPath
    )

    $result = Invoke-NiloWsl -WslExecutable $WslExecutable -Arguments @(
        '-d', $Distribution, '--exec', 'wslpath', '-a', '--', $WindowsPath
    )
    $path = $result.Output.Trim()
    if (-not $path.StartsWith('/')) {
        throw "WSL could not translate the repository path '$WindowsPath': $path"
    }
    return $path
}

function Get-NiloWslHostVolume {
    param(
        [Parameter(Mandatory = $true)][string]$WslExecutable,
        [Parameter(Mandatory = $true)][string]$Distribution
    )

    # Use the registered virtual disk location, not the checkout's drive.
    $distributionRecord = @(
        Get-ChildItem -LiteralPath 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Lxss' |
            ForEach-Object { Get-ItemProperty -LiteralPath $_.PSPath } |
            Where-Object { $_.DistributionName -eq $Distribution }
    ) | Select-Object -First 1
    if ($null -eq $distributionRecord -or -not $distributionRecord.BasePath) {
        throw "Cannot locate the backing virtual disk for WSL distribution '$Distribution'."
    }
    $expandedBasePath = (Resolve-Path -LiteralPath $distributionRecord.BasePath).Path
    if ($expandedBasePath -notmatch '(?<Drive>[A-Za-z]:)\\') {
        throw "Cannot determine the Windows volume backing WSL distribution '$Distribution' from '$expandedBasePath'."
    }
    return ConvertTo-NiloWslPath -WslExecutable $WslExecutable -Distribution $Distribution -WindowsPath "$($Matches.Drive)\"
}
