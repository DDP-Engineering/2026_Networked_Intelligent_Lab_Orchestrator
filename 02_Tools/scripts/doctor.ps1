$ErrorActionPreference = 'Stop'
$toolRoot = Split-Path -Parent $PSScriptRoot
$values = @{}
foreach ($rawLine in Get-Content -LiteralPath (Join-Path $toolRoot 'toolchain.versions')) {
    $line = $rawLine.Trim()
    if ($line.Length -eq 0 -or $line.StartsWith('#')) { continue }
    if ($line -notmatch '^([A-Z0-9_]+)=([0-9a-z.]+)$') { throw "Invalid toolchain manifest line: $rawLine" }
    $values[$Matches[1]] = $Matches[2]
}

$platformKey = 'windows-x86_64'
$runtimeRoot = Join-Path $toolRoot "runtime\$platformKey"
$goExecutable = Join-Path $runtimeRoot "go\$($values['GO_VERSION'])\go\bin\go.exe"
$uvExecutable = Join-Path $runtimeRoot "uv\$($values['UV_VERSION'])\uv.exe"
$nodeRoot = Join-Path $runtimeRoot "node\$($values['NODE_VERSION'])"
$nodeExecutable = Join-Path $nodeRoot 'node.exe'
$npmExecutable = Join-Path $nodeRoot 'npm.cmd'
foreach ($required in @($goExecutable, $uvExecutable, $nodeExecutable, $npmExecutable)) {
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
$nodeVersion = (& $nodeExecutable --version) -join ''
if ($LASTEXITCODE -ne 0 -or $nodeVersion -ne "v$($values['NODE_VERSION'])") {
    throw "Unexpected Node.js version: $nodeVersion"
}
$env:npm_config_cache = Join-Path $toolRoot "cache\$platformKey\npm"
New-Item -ItemType Directory -Force -Path $env:npm_config_cache | Out-Null
$npmVersion = (& $npmExecutable --version) -join ''
if ($LASTEXITCODE -ne 0 -or $npmVersion -ne $values['NPM_VERSION']) {
    throw "Unexpected npm version: $npmVersion"
}

$env:UV_CACHE_DIR = Join-Path $toolRoot "cache\$platformKey\uv"
$env:UV_PYTHON_INSTALL_DIR = Join-Path $runtimeRoot 'python'
$env:UV_PYTHON_BIN_DIR = Join-Path $runtimeRoot 'python-bin'
$env:UV_PYTHON_NO_REGISTRY = '1'
$env:UV_PYTHON_PREFERENCE = 'only-managed'
$env:UV_PROJECT_ENVIRONMENT = ".venv-$platformKey"
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
Write-Output "Node.js: $nodeVersion"
Write-Output "npm: $npmVersion"
Write-Output 'NILO repository-local toolchain: OK'
