<#
.SYNOPSIS
Public Windows command surface for NILO setup, development, checks and delivery.
.DESCRIPTION
Windows development uses the existing visible-console lifecycle. Canonical
build/test and packaging operations delegate to the same Linux command surface
inside the selected WSL distribution. Host changes remain explicit options.
#>
[CmdletBinding()]
param(
    [Parameter(Position = 0)][string]$Command = 'help',
    [Parameter(Position = 1)][string]$Mode = '',
    [Parameter(Position = 2, ValueFromRemainingArguments = $true)][string[]]$Paths,
    [string]$Distribution = '',
    [ValidateSet('docker', 'podman')][string]$Engine = 'docker',
    [switch]$Offline,
    [switch]$InstallWSL,
    [switch]$InstallEngine,
    [switch]$AllowEngineDowngrades,
    [switch]$RebuildFromPinnedInputs,
    [string]$ArchivePath,
    [string]$EngineArchivePath,
    [ValidateSet('healthy', 'unavailable')][string]$Scenario = 'healthy',
    [ValidateRange(1, 65535)][int]$BackendPort = 8090,
    [ValidateRange(1, 65535)][int]$FrontendPort = 5173,
    [switch]$OpenBrowser,
    [ValidateRange(1, 3600)][int]$ReadyTimeoutSeconds = 180,
    [string]$WslExecutable = 'wsl.exe'
)
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
function Show-ToolsHelp {
    @'
NILO repository tools (Windows x86-64 + Ubuntu 24.04 WSL2)
  setup [all|tools|container] [-Offline] [-Distribution NAME]
        [-InstallWSL] [-Engine docker|podman] [-ArchivePath DIR]
        [-RebuildFromPinnedInputs]
        [-InstallEngine -EngineArchivePath DIR [-AllowEngineDowngrades]]
  dev [start|status] [-Distribution NAME] [-Scenario healthy|unavailable]
        [-BackendPort PORT] [-FrontendPort PORT] [-OpenBrowser]
  doctor [all|tools|container] [-Distribution NAME] [-Engine docker|podman]
  test [offline|tools|network|appliance|packaging] [-Distribution NAME]
        [-Engine docker|podman]
  build [-Distribution NAME] [-Engine docker|podman]
  package TARGET [-Distribution NAME] [-Engine docker|podman]
  package release ARTIFACTS ACCEPTANCE SOURCE_OFFERS UBUNTU_RECEIPT PI_RECEIPT OUTPUT
  package verify RELEASE_DIRECTORY

setup prepares Windows/WSL tools and the canonical container. Host installation
and connected rebuilds require explicit flags. dev uses visible consoles; stop
each server with Ctrl+C in its console. Login: admin / admin_password (simulation).
test defaults to network-disabled validation; named acceptance fixtures are explicit.
build publishes binaries and GUI under 02_Tools/cache/containers/outputs.
package TARGET prepares inputs, not a finished release. Targets:
ubuntu-24.04-amd64, rpi-debian13-arm64. Release paths are translated into WSL.
Advanced archive/image operations: see 02_Tools/containers/build/README.md.
'@
}
function Stop-Usage([string]$Message) { Write-Output "error: $Message"; Show-ToolsHelp; exit 2 }
$Command = $Command.ToLowerInvariant()
$Mode = $Mode.ToLowerInvariant()
if ($Command -in @('help', '--help', '-h')) {
    if ($PSBoundParameters.Keys | Where-Object { $_ -ne 'Command' }) { Stop-Usage 'help takes no options' }
    Show-ToolsHelp; exit 0
}
if ($Command -notin @('setup', 'dev', 'doctor', 'test', 'build', 'package')) { Stop-Usage "Unknown command '$Command'" }
$allowed = @('Command', 'Mode', 'Distribution', 'WslExecutable')
switch ($Command) {
    setup { $allowed += @('Engine', 'Offline', 'InstallWSL', 'InstallEngine', 'AllowEngineDowngrades', 'RebuildFromPinnedInputs', 'ArchivePath', 'EngineArchivePath') }
    dev { $allowed += @('Scenario', 'BackendPort', 'FrontendPort', 'OpenBrowser', 'ReadyTimeoutSeconds') }
    package { $allowed += @('Paths', 'Engine') }
    default { $allowed += 'Engine' }
}
foreach ($key in $PSBoundParameters.Keys) { if ($key -notin $allowed) { Stop-Usage "-$key is not valid for $Command" } }
if ($Paths -and $Command -ne 'package') { Stop-Usage 'Unexpected positional arguments' }
switch ($Command) {
    setup {
        if (-not $Mode) { $Mode = 'all' }
        if ($Mode -notin @('all', 'tools', 'container')) { Stop-Usage 'setup mode must be all, tools or container' }
        if ($Offline -and $RebuildFromPinnedInputs) { Stop-Usage '-Offline cannot rebuild from network inputs' }
        if ($InstallEngine -and -not $EngineArchivePath) { Stop-Usage '-InstallEngine requires -EngineArchivePath' }
        if ($EngineArchivePath -and -not $InstallEngine) { Stop-Usage '-EngineArchivePath requires -InstallEngine' }
        if ($AllowEngineDowngrades -and -not $InstallEngine) { Stop-Usage '-AllowEngineDowngrades requires -InstallEngine' }
        if ($Mode -eq 'tools' -and ($InstallEngine -or $AllowEngineDowngrades -or $ArchivePath -or $EngineArchivePath -or $RebuildFromPinnedInputs)) { Stop-Usage 'Container options require setup all or container' }
        if ($Mode -eq 'container' -and $InstallWSL) { Stop-Usage '-InstallWSL requires setup all or tools' }
    }
    dev {
        if (-not $Mode) { $Mode = 'start' }
        if ($Mode -notin @('start', 'status')) { Stop-Usage 'dev mode must be start or status' }
        if ($Mode -eq 'status' -and ($OpenBrowser -or $PSBoundParameters.ContainsKey('Scenario'))) { Stop-Usage 'Scenario/OpenBrowser require dev start' }
    }
    doctor { if ($Mode -and $Mode -notin @('all', 'tools', 'container')) { Stop-Usage 'doctor mode must be all, tools or container' } }
    test {
        if ($Mode -and $Mode -notin @('offline', 'tools', 'network', 'appliance', 'packaging')) { Stop-Usage 'Unknown test mode' }
        if ($Engine -ne 'docker' -and $Mode -in @('network', 'packaging')) { Stop-Usage "$Mode fixture requires Docker" }
    }
    build { if ($Mode) { Stop-Usage 'build takes options only' } }
    package {
        $count = @($Paths | Where-Object { $null -ne $_ }).Count
        switch ($Mode) {
            release { if ($count -ne 6) { Stop-Usage 'package release requires six paths' } }
            verify { if ($count -ne 1) { Stop-Usage 'package verify requires one directory' } }
            { $_ -in @('ubuntu-24.04-amd64', 'rpi-debian13-arm64') } { if ($count) { Stop-Usage 'Target preparation takes no paths' } }
            default { Stop-Usage 'package requires an exact target, release or verify' }
        }
        if ($Mode -in @('release', 'verify') -and $PSBoundParameters.ContainsKey('Engine')) { Stop-Usage '-Engine is only valid for target preparation' }
    }
}

# Run child PowerShell scripts in their own process: their exit and environment
# changes must not terminate this dispatcher early or affect its caller.
function Invoke-ToolsScript([string]$RelativePath, [hashtable]$Options) {
    $arguments = @('-NoProfile', '-File', (Join-Path $PSScriptRoot $RelativePath))
    foreach ($key in $Options.Keys) {
        if ($Options[$key] -is [bool]) { if ($Options[$key]) { $arguments += "-$key" } }
        else { $arguments += @("-$key", [string]$Options[$key]) }
    }
    & (Get-Process -Id $PID).Path @arguments
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
}
if ($Command -eq 'dev') {
    Invoke-ToolsScript 'scripts/nilo-dev.ps1' @{
        Command=$Mode; Distribution=$Distribution; Scenario=$Scenario;
        BackendPort=$BackendPort; FrontendPort=$FrontendPort; OpenBrowser=[bool]$OpenBrowser;
        ReadyTimeoutSeconds=$ReadyTimeoutSeconds; WslExecutable=$WslExecutable
    }
    exit 0
}
if ($Command -eq 'setup' -and $Mode -ne 'container') {
    Invoke-ToolsScript 'bootstrap/bootstrap-development.ps1' @{
        Distribution=$Distribution; Offline=[bool]$Offline; InstallWSL=[bool]$InstallWSL; WslExecutable=$WslExecutable
    }
    if ($Mode -eq 'tools') { exit 0 }
}
if ($Command -eq 'doctor' -and $Mode -ne 'container') {
    Invoke-ToolsScript 'scripts/doctor.ps1' @{}
    Invoke-ToolsScript 'scripts/doctor-wsl.ps1' @{Distribution=$Distribution; WslExecutable=$WslExecutable}
    if ($Mode -eq 'tools') { exit 0 }
    $Mode = 'container'
}
. (Join-Path $PSScriptRoot 'lib/Wsl.ps1')
if (-not $Distribution) {
    $installed = @(Get-NiloWslDistributions -WslExecutable $WslExecutable)
    if ($installed.Count -eq 0) { Stop-Usage 'No development WSL distribution is installed; run setup tools -InstallWSL explicitly' }
    if ($installed.Count -ne 1) { Stop-Usage 'Select -Distribution when more than one development distribution is installed' }
    $Distribution = $installed[0]
}
if ($Command -eq 'setup') {
    Invoke-ToolsScript 'bootstrap/bootstrap-build-environment.ps1' @{
        Distribution=$Distribution; Engine=$Engine; Offline=[bool]$Offline;
        InstallEngine=[bool]$InstallEngine; AllowEngineDowngrades=[bool]$AllowEngineDowngrades;
        ArchivePath=$ArchivePath; EngineArchivePath=$EngineArchivePath;
        RebuildFromPinnedInputs=[bool]$RebuildFromPinnedInputs; WslExecutable=$WslExecutable
    }
    exit 0
}
$linuxRoot = ConvertTo-NiloWslPath -WslExecutable $WslExecutable -Distribution $Distribution -WindowsPath $PSScriptRoot
$linuxArguments = @($Command)
if ($Mode) { $linuxArguments += $Mode }
if ($Command -eq 'package' -and $Mode -in @('release', 'verify')) {
    foreach ($path in $Paths) {
        $absolute = $ExecutionContext.SessionState.Path.GetUnresolvedProviderPathFromPSPath($path)
        $linuxArguments += ConvertTo-NiloWslPath -WslExecutable $WslExecutable -Distribution $Distribution -WindowsPath $absolute
    }
} else { $linuxArguments += @('--engine', $Engine) }
$execution = @('-d', $Distribution, '--exec')
if ($Command -in @('test', 'build') -or ($Command -eq 'package' -and $Mode -notin @('release', 'verify'))) {
    $hostVolume = Get-NiloWslHostVolume -WslExecutable $WslExecutable -Distribution $Distribution
    $execution += @('env', "NILO_WSL_HOST_VOLUME=$hostVolume")
}
$result = Invoke-NiloWsl -WslExecutable $WslExecutable -Arguments ($execution + @('sh', "$linuxRoot/nilo-tools.sh") + $linuxArguments) -AllowFailure
if ($result.Output) { Write-Output $result.Output }
exit $result.ExitCode
