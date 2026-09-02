<#
.SYNOPSIS
Install the pinned browser the integrated browser test drives, below the
repository-local runtime directory.

.DESCRIPTION
Asks the locked browser test package to install exactly one browser, Chromium,
into 02_Tools/runtime/windows-x86_64/playwright. The directory is ignored by
Git and is reproduced from the pinned package rather than committed.

Nothing outside the repository is written and no persistent host environment is
changed: the installation directory is set for the child process only, by
02_Tools/env/with-tools.ps1. An already-installed browser is left alone, so an
ordinary test run never downloads anything.

Supported platform: Windows x86_64, after bootstrap-frontend.ps1 has restored
the locked interface dependencies.

.EXAMPLE
.\02_Tools\bootstrap\bootstrap-browser.ps1
#>

param(
    [switch]$Offline
)

$ErrorActionPreference = 'Stop'
$toolRoot = Split-Path -Parent $PSScriptRoot
$repositoryRoot = (Resolve-Path (Join-Path $toolRoot '..')).Path
$frontendRoot = Join-Path $repositoryRoot '05_Software\nilo_gui'
$toolWrapper = Join-Path $toolRoot 'env\with-tools.ps1'
$browserRuntime = Join-Path $toolRoot 'runtime\windows-x86_64\playwright'
$driver = Join-Path $frontendRoot 'node_modules\@playwright\test'

if (-not (Test-Path -LiteralPath $driver -PathType Container)) {
    throw "Missing locked browser test package: $driver; run .\02_Tools\bootstrap\bootstrap-frontend.ps1"
}

# Chromium alone. The suite pins one browser engine on purpose: a second engine
# would double the download without testing another line of this repository.
if ($Offline) {
    if (-not (Test-Path -LiteralPath $browserRuntime -PathType Container)) {
        throw "No repository-local browser is present at $browserRuntime and -Offline forbids downloading one"
    }
    Write-Output "NILO repository-local browser: OK (offline, using $browserRuntime)"
    return
}

Push-Location $frontendRoot
try {
    & $toolWrapper npm.cmd exec --no -- playwright install chromium
    if ($LASTEXITCODE -ne 0) {
        throw "Browser installation failed with exit code $LASTEXITCODE"
    }
}
finally {
    Pop-Location
}

if (-not (Test-Path -LiteralPath $browserRuntime -PathType Container)) {
    throw "Browser installation reported success but wrote nothing to $browserRuntime"
}

Write-Output 'NILO repository-local browser: OK'
