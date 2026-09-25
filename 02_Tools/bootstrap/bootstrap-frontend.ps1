param(
    [switch]$Offline
)

$ErrorActionPreference = 'Stop'
$toolRoot = Split-Path -Parent $PSScriptRoot
$repositoryRoot = (Resolve-Path (Join-Path $toolRoot '..')).Path
$frontendRoot = Join-Path $repositoryRoot '05_Software\frontend'
$lockFile = Join-Path $frontendRoot 'package-lock.json'
$toolWrapper = Join-Path $toolRoot 'env\with-tools.ps1'

if (-not (Test-Path -LiteralPath $lockFile -PathType Leaf)) {
    throw "Missing locked frontend dependencies: $lockFile"
}

$installArguments = @('npm.cmd', 'ci', '--no-audit', '--no-fund')
if ($Offline) {
    $installArguments += '--offline'
}

# Browsers are acquired deliberately, by bootstrap-browser.ps1, so that a locked
# restore stays a locked restore. The pinned browser test package declares no
# install hook today, so this changes nothing now; it is set so that a future
# version reintroducing one cannot quietly populate a cache outside this
# repository during an ordinary dependency restore.
$previousSkip = $env:PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD
$env:PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD = '1'

Push-Location $frontendRoot
try {
    & $toolWrapper @installArguments
    if ($LASTEXITCODE -ne 0) {
        throw "Frontend dependency restore failed with exit code $LASTEXITCODE"
    }
}
finally {
    Pop-Location
    $env:PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD = $previousSkip
}

Write-Output 'NILO locked frontend dependencies: OK'
