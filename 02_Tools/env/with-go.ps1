$toolRoot = Split-Path -Parent $PSScriptRoot
$goBin = Join-Path $toolRoot 'runtime\go\1.27.0\go\bin'
$goExecutable = Join-Path $goBin 'go.exe'

if ($args.Count -lt 1) {
    throw 'Pass a command to run, for example: .\02_Tools\env\with-go.ps1 go version'
}

if (-not (Test-Path -LiteralPath $goExecutable -PathType Leaf)) {
    throw "Repository-local Go 1.27.0 is missing at $goExecutable. Restore it with the documented bootstrap process."
}

$env:PATH = "$goBin;$env:PATH"
$executable = $args[0]
$commandArguments = if ($args.Count -gt 1) { @($args[1..($args.Count - 1)]) } else { @() }
& $executable $commandArguments
exit $LASTEXITCODE
