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

if ($args.Count -lt 1) {
    throw 'Pass a command to run, for example: .\02_Tools\env\with-tools.ps1 go version'
}
foreach ($required in @((Join-Path $goBin 'go.exe'), (Join-Path $uvBin 'uv.exe'))) {
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
$managedPython = (& (Join-Path $uvBin 'uv.exe') python find $values['PYTHON_VERSION']) -join ''
if ($LASTEXITCODE -ne 0 -or -not (Test-Path -LiteralPath $managedPython -PathType Leaf)) {
    throw "Repository-local Python $($values['PYTHON_VERSION']) is missing; run .\02_Tools\bootstrap\bootstrap.ps1"
}
$managedPythonBin = Split-Path -Parent $managedPython
$env:PATH = "$goBin;$uvBin;$managedPythonBin;$pythonBin;$env:PATH"

$executable = $args[0]
$commandArguments = if ($args.Count -gt 1) { @($args[1..($args.Count - 1)]) } else { @() }
& $executable $commandArguments
exit $LASTEXITCODE
