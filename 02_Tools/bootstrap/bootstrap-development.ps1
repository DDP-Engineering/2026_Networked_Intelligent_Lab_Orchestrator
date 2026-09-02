param(
    [string]$Distribution = '',
    [switch]$InstallWSL,
    [switch]$Offline,
    [string]$WslExecutable = 'wsl.exe'
)

$ErrorActionPreference = 'Stop'
$toolRoot = Split-Path -Parent $PSScriptRoot
$repositoryRoot = (Resolve-Path (Join-Path $toolRoot '..')).Path
$setupWsl = Join-Path $PSScriptRoot 'setup-wsl.ps1'
$windowsBootstrap = Join-Path $PSScriptRoot 'bootstrap.ps1'
$frontendBootstrap = Join-Path $PSScriptRoot 'bootstrap-frontend.ps1'
$browserBootstrap = Join-Path $PSScriptRoot 'bootstrap-browser.ps1'
$linuxBootstrapName = 'bootstrap.sh'
$windowsDoctor = Join-Path $toolRoot 'scripts\doctor.ps1'
$wslDoctor = Join-Path $toolRoot 'scripts\doctor-wsl.ps1'
. (Join-Path $toolRoot 'lib\Wsl.ps1')

$setupArguments = @{
    Distribution = $Distribution
    WslExecutable = $WslExecutable
}
if ($InstallWSL) {
    $setupArguments['Install'] = $true
}
& $setupWsl @setupArguments
if ($LASTEXITCODE -ne 0) {
    exit $LASTEXITCODE
}

if ([string]::IsNullOrWhiteSpace($Distribution)) {
    $distributions = @(Get-NiloWslDistributions -WslExecutable $WslExecutable)
    if ($distributions.Count -ne 1) {
        throw "Expected one installed WSL distribution after setup but found $($distributions.Count). Select one with -Distribution."
    }
    $Distribution = $distributions[0]
}

$windowsArguments = @{}
if ($Offline) {
    $windowsArguments['Offline'] = $true
}
& $windowsBootstrap @windowsArguments
if ($LASTEXITCODE -ne 0) {
    throw "Windows tool bootstrap failed with exit code $LASTEXITCODE"
}

& $frontendBootstrap @windowsArguments
if ($LASTEXITCODE -ne 0) {
    throw "Windows frontend bootstrap failed with exit code $LASTEXITCODE"
}

& $browserBootstrap @windowsArguments
if ($LASTEXITCODE -ne 0) {
    throw "Windows browser bootstrap failed with exit code $LASTEXITCODE"
}

$wslRepositoryRoot = ConvertTo-NiloWslPath -WslExecutable $WslExecutable -Distribution $Distribution -WindowsPath $repositoryRoot
$linuxBootstrap = "$wslRepositoryRoot/02_Tools/bootstrap/$linuxBootstrapName"
$linuxArguments = @('-d', $Distribution, '--exec', 'sh', $linuxBootstrap)
if ($Offline) {
    $linuxArguments += '--offline'
}
Invoke-NiloWsl -WslExecutable $WslExecutable -Arguments $linuxArguments | Out-Null

& $windowsDoctor
if ($LASTEXITCODE -ne 0) {
    throw "Windows tool doctor failed with exit code $LASTEXITCODE"
}
& $wslDoctor -Distribution $Distribution -WslExecutable $WslExecutable
if ($LASTEXITCODE -ne 0) {
    throw "WSL tool doctor failed with exit code $LASTEXITCODE"
}

Write-Output 'NILO combined Windows and WSL2 development bootstrap: PASS'
