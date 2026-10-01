[CmdletBinding()]
param(
    [string]$Distribution = 'Ubuntu-24.04',
    [ValidateSet('docker', 'podman')][string]$Engine = 'docker',
    [switch]$InstallEngine,
    [switch]$AllowEngineDowngrades,
    [string]$EngineArchivePath,
    [string]$ArchivePath,
    [switch]$Offline,
    [switch]$RebuildFromPinnedInputs,
    [string]$WslExecutable = 'wsl.exe'
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
if ($Offline -and $RebuildFromPinnedInputs) {
    throw '-Offline cannot be combined with -RebuildFromPinnedInputs.'
}
if ($AllowEngineDowngrades -and -not $InstallEngine) {
    throw '-AllowEngineDowngrades is valid only with -InstallEngine.'
}

$toolsRoot = Split-Path -Parent $PSScriptRoot
. (Join-Path $toolsRoot 'lib\Wsl.ps1')
$repositoryRoot = Split-Path -Parent $toolsRoot
$installed = @(Get-NiloWslDistributions -WslExecutable $WslExecutable)
if ($Distribution -notin $installed) {
    throw "WSL2 distribution '$Distribution' is unavailable. Prepare it explicitly with .\02_Tools\bootstrap\setup-wsl.ps1 -Distribution '$Distribution' -Install, then rerun."
}
$kernel = Get-NiloWslKernel -WslExecutable $WslExecutable -Distribution $Distribution
if (-not (Test-NiloWsl2Kernel -Kernel $kernel)) {
    throw "Distribution '$Distribution' is not running as WSL2 (kernel: $kernel)."
}
$identity = Invoke-NiloWsl -WslExecutable $WslExecutable -Arguments @('-d', $Distribution, '--exec', 'cat', '/etc/os-release') -AllowFailure
$architecture = Invoke-NiloWsl -WslExecutable $WslExecutable -Arguments @('-d', $Distribution, '--exec', 'uname', '-m') -AllowFailure
$systemd = Invoke-NiloWsl -WslExecutable $WslExecutable -Arguments @('-d', $Distribution, '--exec', 'systemctl', 'is-system-running') -AllowFailure
$osLines = @($identity.Output -split "`r?`n" | ForEach-Object { $_.Trim() })
$isUbuntu = 'ID=ubuntu' -in $osLines -and 'VERSION_ID="24.04"' -in $osLines
$isAmd64 = $architecture.ExitCode -eq 0 -and $architecture.Output.Trim() -eq 'x86_64'
$hasSystemd = $systemd.Output.Trim() -in @('running', 'degraded')
if ($identity.ExitCode -ne 0 -or -not $isUbuntu -or -not $isAmd64 -or -not $hasSystemd) {
    throw "Distribution '$Distribution' must be Ubuntu 24.04 x86_64 with systemd support; OS='$($identity.Output.Trim())', architecture='$($architecture.Output.Trim())', systemd='$($systemd.Output.Trim())'."
}
$wslRepository = ConvertTo-NiloWslPath -WslExecutable $WslExecutable -Distribution $Distribution -WindowsPath $repositoryRoot
$wslHostVolume = Get-NiloWslHostVolume -WslExecutable $WslExecutable -Distribution $Distribution
$wslHostVolumeArgument = "NILO_WSL_HOST_VOLUME=$wslHostVolume"
$wslHomeDirectory = (Invoke-NiloWsl -WslExecutable $WslExecutable -Arguments @('-d', $Distribution, '--exec', 'printenv', 'HOME')).Output.Trim()
if ($wslHomeDirectory.StartsWith('/mnt/')) { throw "WSL home and engine state must stay on the Linux filesystem; observed $wslHomeDirectory." }

$engineArguments = @('--engine', $Engine)
if ($InstallEngine) {
    if ([string]::IsNullOrWhiteSpace($EngineArchivePath)) { throw '-InstallEngine requires -EngineArchivePath containing verified local packages.' }
    $engineArchiveWsl = ConvertTo-NiloWslPath -WslExecutable $WslExecutable -Distribution $Distribution -WindowsPath $EngineArchivePath
    $engineArguments += @('--install', '--archive-path', $engineArchiveWsl)
    if ($AllowEngineDowngrades) { $engineArguments += '--allow-downgrades' }
    $engineScript = "$wslRepository/02_Tools/bootstrap/bootstrap-container-engine.sh"
    $engineCommandArguments = @('-d', $Distribution, '--exec', 'env', $wslHostVolumeArgument, 'sh', $engineScript) + $engineArguments
    Invoke-NiloWsl -WslExecutable $WslExecutable -Arguments $engineCommandArguments | Out-Null
}
$engineScript = "$wslRepository/02_Tools/bootstrap/bootstrap-container-engine.sh"

$setupArguments = @('--engine', $Engine, '--engine-verifier', $engineScript)
if ($ArchivePath) {
    $archiveWsl = ConvertTo-NiloWslPath -WslExecutable $WslExecutable -Distribution $Distribution -WindowsPath $ArchivePath
    $setupArguments += @('--archive-path', $archiveWsl)
}
if ($Offline) { $setupArguments += '--offline' }
if ($RebuildFromPinnedInputs) { $setupArguments += '--rebuild-from-pinned-inputs' }
$setupScript = "$wslRepository/02_Tools/bootstrap/setup-build-environment-image.sh"
$setupCommandArguments = @('-d', $Distribution, '--exec', 'env', $wslHostVolumeArgument, 'sh', $setupScript) + $setupArguments
$result = Invoke-NiloWsl -WslExecutable $WslExecutable -Arguments $setupCommandArguments -AllowFailure
if ($result.Output) { Write-Output $result.Output }
if ($result.ExitCode -ne 0) { exit $result.ExitCode }
Write-Output "NILO build environment is ready in $Distribution using $Engine."
