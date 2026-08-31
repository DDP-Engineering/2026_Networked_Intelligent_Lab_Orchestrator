$ErrorActionPreference = 'Stop'

$scriptRoot = Split-Path -Parent $PSCommandPath
$softwareRoot = (Resolve-Path (Join-Path $scriptRoot '..')).Path
$repositoryRoot = (Resolve-Path (Join-Path $softwareRoot '..')).Path
$withTools = Join-Path $repositoryRoot '02_Tools\env\with-tools.ps1'
$doctor = Join-Path $repositoryRoot '02_Tools\scripts\doctor.ps1'

function Invoke-Checked {
    param([string]$WorkingDirectory, [string[]]$Command)
    Push-Location $WorkingDirectory
    try {
        & $withTools @Command
        if ($LASTEXITCODE -ne 0) {
            throw "$($Command -join ' ') exited with code $LASTEXITCODE"
        }
    }
    finally {
        Pop-Location
    }
}

& $doctor
if ($LASTEXITCODE -ne 0) {
    throw "repository-local toolchain doctor exited with code $LASTEXITCODE"
}

$baseline = @(git -C $repositoryRoot status --porcelain=v1 --untracked-files=all)
if ($LASTEXITCODE -ne 0) {
    throw 'cannot record the pre-test Git state'
}

$sdkRoot = Join-Path $softwareRoot 'integration_sdk\python'
$fakeRoot = Join-Path $softwareRoot 'integrations\fake_python'
$goRoot = Join-Path $softwareRoot 'nilo_assistant'

Invoke-Checked $sdkRoot @('uv', 'sync', '--all-groups', '--locked')
Invoke-Checked $fakeRoot @('uv', 'sync', '--all-groups', '--locked')
Invoke-Checked $sdkRoot @('uv', 'run', 'pytest', '-q')
Invoke-Checked $sdkRoot @('uv', 'run', 'ruff', 'check', '.')
Invoke-Checked $sdkRoot @('uv', 'run', 'ruff', 'format', '--check', '.')
Invoke-Checked $fakeRoot @('uv', 'run', 'pytest', '-q')
Invoke-Checked $fakeRoot @('uv', 'run', 'ruff', 'check', '.')
Invoke-Checked $fakeRoot @('uv', 'run', 'ruff', 'format', '--check', '.')
Invoke-Checked $softwareRoot @('uv', 'run', '--project', 'integrations\fake_python', '--all-groups', 'pytest', '-q', 'tests\contract\test_integration_contract.py')

Invoke-Checked $goRoot @('go', 'test', './internal/operation', './internal/integration/supervisor', './internal/acceptance')
Invoke-Checked $goRoot @('go', 'test', './...')
Invoke-Checked $goRoot @('go', 'vet', './...')

$cgoEnabled = (& $withTools go env CGO_ENABLED) -join ''
& $withTools where.exe gcc *> $null
$gccAvailable = $LASTEXITCODE -eq 0
if ($cgoEnabled -eq '1' -and $gccAvailable) {
    Invoke-Checked $goRoot @('go', 'test', '-race', './...')
}
else {
    Write-Output "Go race tests: SKIPPED (CGO_ENABLED=$cgoEnabled, gcc_available=$gccAvailable)"
}

Invoke-Checked $softwareRoot @('powershell', '-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', (Join-Path $scriptRoot 'verify-portable-core.ps1'))

$after = @(git -C $repositoryRoot status --porcelain=v1 --untracked-files=all)
if ($LASTEXITCODE -ne 0) {
    throw 'cannot record the post-test Git state'
}
$delta = @(Compare-Object -ReferenceObject $baseline -DifferenceObject $after)
if ($delta.Count -ne 0) {
    $delta | Format-Table | Out-String | Write-Error
    throw 'Increment 1 verification left a repository artifact or changed a tracked file'
}

Write-Output 'NILO Increment 1 verification: PASS'
