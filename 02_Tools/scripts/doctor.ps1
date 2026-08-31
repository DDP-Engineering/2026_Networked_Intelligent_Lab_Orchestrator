$ErrorActionPreference = 'Stop'
$toolRoot = Split-Path -Parent $PSScriptRoot
$values = @{}
foreach ($rawLine in Get-Content -LiteralPath (Join-Path $toolRoot 'toolchain.versions')) {
    $line = $rawLine.Trim()
    if ($line.Length -eq 0 -or $line.StartsWith('#')) { continue }
    if ($line -notmatch '^([A-Z0-9_]+)=([0-9a-z.]+)$') { throw "Invalid toolchain manifest line: $rawLine" }
    $values[$Matches[1]] = $Matches[2]
}

$runtimeRoot = Join-Path $toolRoot 'runtime'
$goExecutable = Join-Path $runtimeRoot "go\$($values['GO_VERSION'])\go\bin\go.exe"
$uvExecutable = Join-Path $runtimeRoot "uv\$($values['UV_VERSION'])\uv.exe"
foreach ($required in @($goExecutable, $uvExecutable)) {
    if (-not (Test-Path -LiteralPath $required -PathType Leaf)) {
        throw "Missing repository-local tool: $required"
    }
}

$goVersion = (& $goExecutable version) -join ''
if ($LASTEXITCODE -ne 0 -or $goVersion -notmatch "go$($values['GO_VERSION'])") {
    throw "Unexpected Go version: $goVersion"
}
$uvVersion = (& $uvExecutable --version) -join ''
if ($LASTEXITCODE -ne 0 -or $uvVersion -notmatch "^uv $($values['UV_VERSION'])( |$)") {
    throw "Unexpected uv version: $uvVersion"
}

$env:UV_CACHE_DIR = Join-Path $toolRoot 'cache\uv'
$env:UV_PYTHON_INSTALL_DIR = Join-Path $runtimeRoot 'python'
$env:UV_PYTHON_BIN_DIR = Join-Path $runtimeRoot 'python-bin'
$env:UV_PYTHON_NO_REGISTRY = '1'
$env:UV_PYTHON_PREFERENCE = 'only-managed'
$pythonExecutable = (& $uvExecutable python find $values['PYTHON_VERSION']) -join ''
if ($LASTEXITCODE -ne 0 -or -not (Test-Path -LiteralPath $pythonExecutable -PathType Leaf)) {
    throw "Missing repository-local Python $($values['PYTHON_VERSION'])"
}
$pythonVersion = (& $pythonExecutable --version) -join ''
if ($LASTEXITCODE -ne 0 -or $pythonVersion -ne "Python $($values['PYTHON_VERSION'])") {
    throw "Unexpected Python version: $pythonVersion"
}

Write-Output "Go: $goVersion"
Write-Output "uv: $uvVersion"
Write-Output "Python: $pythonVersion"
Write-Output 'NILO repository-local toolchain: OK'
