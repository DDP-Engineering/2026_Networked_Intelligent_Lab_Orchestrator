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
$goBin = Join-Path $runtimeRoot "go\$($values['GO_VERSION'])\go\bin"
$uvBin = Join-Path $runtimeRoot "uv\$($values['UV_VERSION'])"
$pythonBin = Join-Path $runtimeRoot 'python-bin'
$nodeBin = Join-Path $runtimeRoot "node\$($values['NODE_VERSION'])"

if ($args.Count -lt 1) {
    throw 'Pass a command to run, for example: .\02_Tools\env\with-tools.ps1 go version'
}
# Go-only fixtures need no Python or frontend runtime. Keep this selection in
# the shared wrapper instead of maintaining another pair of Go wrappers.
if ($args[0] -in @('go', 'go.exe')) {
    $goExecutable = Join-Path $goBin 'go.exe'
    if (-not (Test-Path -LiteralPath $goExecutable -PathType Leaf)) {
        throw "Repository-local Go is missing: $goExecutable; run nilo-tools.ps1 setup tools"
    }
    $env:PATH = "$goBin;$env:PATH"
    & $goExecutable @($args | Select-Object -Skip 1)
    exit $LASTEXITCODE
}
foreach ($required in @(
    (Join-Path $goBin 'go.exe'),
    (Join-Path $uvBin 'uv.exe'),
    (Join-Path $nodeBin 'node.exe'),
    (Join-Path $nodeBin 'npm.cmd')
)) {
    if (-not (Test-Path -LiteralPath $required -PathType Leaf)) {
        throw "Repository-local tool is missing: $required; run .\02_Tools\bootstrap\bootstrap.ps1"
    }
}

$env:UV_CACHE_DIR = Join-Path $toolRoot "cache\$platformKey\uv"
$env:UV_PYTHON_INSTALL_DIR = Join-Path $runtimeRoot 'python'
$env:UV_PYTHON_BIN_DIR = $pythonBin
$env:UV_PYTHON_NO_REGISTRY = '1'
$env:UV_PYTHON_PREFERENCE = 'only-managed'
$env:UV_PROJECT_ENVIRONMENT = ".venv-$platformKey"
$env:npm_config_cache = Join-Path $toolRoot "cache\$platformKey\npm"
# Browser binaries for the integrated browser test are a repository-local
# runtime like every other pinned tool, so they are never written to the
# per-user cache Playwright would otherwise choose.
$env:PLAYWRIGHT_BROWSERS_PATH = Join-Path $runtimeRoot 'playwright'
New-Item -ItemType Directory -Force -Path $env:npm_config_cache | Out-Null
$managedPython = (& (Join-Path $uvBin 'uv.exe') python find $values['PYTHON_VERSION']) -join ''
if ($LASTEXITCODE -ne 0 -or -not (Test-Path -LiteralPath $managedPython -PathType Leaf)) {
    throw "Repository-local Python $($values['PYTHON_VERSION']) is missing; run .\02_Tools\bootstrap\bootstrap.ps1"
}
$managedPythonBin = Split-Path -Parent $managedPython
$env:PATH = "$goBin;$uvBin;$managedPythonBin;$pythonBin;$nodeBin;$env:PATH"

$executable = $args[0]
$commandArguments = if ($args.Count -gt 1) { @($args[1..($args.Count - 1)]) } else { @() }
& $executable $commandArguments
exit $LASTEXITCODE
