[CmdletBinding()]
param(
    [Parameter(Position = 0, Mandatory = $true)]
    [ValidateSet('test', 'build', 'package', 'shell', 'run')][string]$Action,
    [Parameter(ValueFromRemainingArguments = $true)][string[]]$Command,
    [string]$Distribution = 'Ubuntu-24.04',
    [ValidateSet('docker', 'podman')][string]$Engine = 'docker',
    [string]$WslExecutable = 'wsl.exe'
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$toolsRoot = Split-Path -Parent $PSScriptRoot
. (Join-Path $toolsRoot 'lib\Wsl.ps1')
$repositoryRoot = Split-Path -Parent $toolsRoot
$wslRepository = ConvertTo-NiloWslPath -WslExecutable $WslExecutable -Distribution $Distribution -WindowsPath $repositoryRoot
$script = "$wslRepository/02_Tools/env/with-build-environment.sh"
$arguments = @('-d', $Distribution, '--exec', 'sh', $script, '--engine', $Engine, $Action) + $Command
$result = Invoke-NiloWsl -WslExecutable $WslExecutable -Arguments $arguments -AllowFailure
if ($result.Output) { Write-Output $result.Output }
exit $result.ExitCode
