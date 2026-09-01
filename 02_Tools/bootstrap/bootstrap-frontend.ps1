param(
    [switch]$Offline
)

$ErrorActionPreference = 'Stop'
$toolRoot = Split-Path -Parent $PSScriptRoot
$repositoryRoot = (Resolve-Path (Join-Path $toolRoot '..')).Path
$frontendRoot = Join-Path $repositoryRoot '05_Software\nilo_gui'
$lockFile = Join-Path $frontendRoot 'package-lock.json'
$toolWrapper = Join-Path $toolRoot 'env\with-tools.ps1'

if (-not (Test-Path -LiteralPath $lockFile -PathType Leaf)) {
    throw "Missing locked frontend dependencies: $lockFile"
}

$installArguments = @('npm.cmd', 'ci', '--no-audit', '--no-fund')
if ($Offline) {
    $installArguments += '--offline'
}

Push-Location $frontendRoot
try {
    & $toolWrapper @installArguments
    if ($LASTEXITCODE -ne 0) {
        throw "Frontend dependency restore failed with exit code $LASTEXITCODE"
    }
}
finally {
    Pop-Location
}

Write-Output 'NILO locked frontend dependencies: OK'
