[CmdletBinding()]
param(
    [Parameter(Position = 0, Mandatory = $true)]
    [ValidateSet('ubuntu-24.04-amd64', 'rpi-debian13-arm64')]
    [string]$Target,
    [string]$Cache
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$packagingRoot = Split-Path -Parent $PSScriptRoot
$toolsRoot = Split-Path -Parent $packagingRoot
if (-not $Cache) {
    $Cache = Join-Path $toolsRoot 'cache\packaging'
}

$arguments = @(
    (Join-Path $packagingRoot 'package_inputs.py'),
    'fetch',
    '--targets', (Join-Path $packagingRoot 'config\targets.json'),
    '--lock', (Join-Path $packagingRoot 'config\inputs.lock.json'),
    '--cache', $Cache,
    '--target', $Target
)
& (Join-Path $toolsRoot 'env\with-tools.ps1') python @arguments
exit $LASTEXITCODE
