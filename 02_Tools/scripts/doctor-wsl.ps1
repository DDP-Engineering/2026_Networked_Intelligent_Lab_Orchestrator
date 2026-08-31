param(
    [string]$Distribution = '',
    [string]$WslExecutable = 'wsl.exe'
)

$ErrorActionPreference = 'Stop'
$toolRoot = Split-Path -Parent $PSScriptRoot
$repositoryRoot = (Resolve-Path (Join-Path $toolRoot '..')).Path
. (Join-Path $toolRoot 'lib\Wsl.ps1')

$distributions = @(Get-NiloWslDistributions -WslExecutable $WslExecutable)
if ([string]::IsNullOrWhiteSpace($Distribution)) {
    if ($distributions.Count -ne 1) {
        throw "Expected one installed WSL distribution but found $($distributions.Count). Select one with -Distribution."
    }
    $Distribution = $distributions[0]
}
$selected = $distributions | Where-Object { $_ -ieq $Distribution } | Select-Object -First 1
if ($null -eq $selected) {
    throw "WSL distribution '$Distribution' is unavailable. Run .\02_Tools\bootstrap\setup-wsl.ps1 -Distribution '$Distribution' and add -Install only if host changes are intended."
}

$kernel = Get-NiloWslKernel -WslExecutable $WslExecutable -Distribution $selected
if (-not (Test-NiloWsl2Kernel -Kernel $kernel)) {
    throw "WSL distribution '$selected' is not WSL2 (kernel: $kernel). Run setup-wsl.ps1 -Distribution '$selected' -Install to request conversion."
}

$wslRepositoryRoot = ConvertTo-NiloWslPath -WslExecutable $WslExecutable -Distribution $selected -WindowsPath $repositoryRoot
Invoke-NiloWsl -WslExecutable $WslExecutable -Arguments @(
    '-d', $selected, '--exec', 'test', '-f', "$wslRepositoryRoot/02_Tools/bootstrap/bootstrap.sh"
) | Out-Null
$linuxDoctor = Invoke-NiloWsl -WslExecutable $WslExecutable -Arguments @(
    '-d', $selected, '--exec', 'sh', "$wslRepositoryRoot/02_Tools/scripts/doctor.sh"
)

Write-Output $linuxDoctor.Output
Write-Output "WSL2 distribution: $selected"
Write-Output "WSL2 kernel: $kernel"
Write-Output "WSL repository path: $wslRepositoryRoot"
Write-Output 'NILO Windows and WSL2 development environment: OK'
