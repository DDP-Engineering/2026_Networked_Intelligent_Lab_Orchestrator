param(
    [string]$Distribution = '',
    [switch]$Install,
    [string]$WslExecutable = 'wsl.exe'
)

$ErrorActionPreference = 'Stop'
$toolRoot = Split-Path -Parent $PSScriptRoot
. (Join-Path $toolRoot 'lib\Wsl.ps1')

Resolve-NiloWslExecutable -WslExecutable $WslExecutable | Out-Null
$distributions = @(Get-NiloWslDistributions -WslExecutable $WslExecutable -AllowUnavailable)
if ([string]::IsNullOrWhiteSpace($Distribution)) {
    if ($distributions.Count -eq 1) {
        $Distribution = $distributions[0]
    }
    elseif ($distributions.Count -gt 1) {
        throw "Multiple WSL distributions are installed: $($distributions -join ', '). Select one with -Distribution."
    }
    else {
        $Distribution = 'Ubuntu'
    }
}
$selected = $distributions | Where-Object { $_ -ieq $Distribution } | Select-Object -First 1

if ($null -eq $selected) {
    if (-not $Install) {
        throw "WSL distribution '$Distribution' is not installed. Rerun with -Install to request 'wsl.exe --install -d $Distribution'. A reboot or first-launch initialization may be required."
    }
    Write-Output "Requesting installation of WSL distribution '$Distribution'."
    Invoke-NiloWsl -WslExecutable $WslExecutable -Arguments @('--install', '-d', $Distribution) | Out-Null
    Write-Warning "WSL installation was requested. Complete any Windows restart or distribution initialization, then rerun this command."
    exit 10
}

$kernel = Get-NiloWslKernel -WslExecutable $WslExecutable -Distribution $selected
if (-not (Test-NiloWsl2Kernel -Kernel $kernel)) {
    if (-not $Install) {
        throw "WSL distribution '$selected' is not running as WSL2 (kernel: $kernel). Rerun with -Install to request conversion with 'wsl.exe --set-version $selected 2'."
    }
    Write-Output "Requesting conversion of '$selected' to WSL2."
    Invoke-NiloWsl -WslExecutable $WslExecutable -Arguments @('--set-version', $selected, '2') | Out-Null
    Write-Warning "WSL2 conversion was requested. Rerun this command after conversion completes."
    exit 10
}

Write-Output "WSL2 distribution: $selected"
Write-Output "WSL2 kernel: $kernel"
Write-Output 'NILO WSL2 host setup: READY'
