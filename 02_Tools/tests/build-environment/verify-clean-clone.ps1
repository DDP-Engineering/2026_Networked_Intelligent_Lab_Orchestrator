[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][ValidateSet('Gitea', 'GitHub')][string]$Source,
    [Parameter(Mandatory = $true)][string]$ClonePath,
    [string]$Distribution = 'Ubuntu-24.04'
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
if (-not (Test-Path -LiteralPath (Join-Path $ClonePath '.git') -PathType Container)) {
    throw "ClonePath must be an already-acquired clean clone; this verifier never clones or fetches: $ClonePath"
}
$bootstrap = Join-Path $ClonePath '02_Tools\bootstrap\bootstrap-build-environment.ps1'
& $bootstrap -Distribution $Distribution -Offline
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
Write-Output "$Source already-acquired clone restored the accepted environment with setup-local content only."
