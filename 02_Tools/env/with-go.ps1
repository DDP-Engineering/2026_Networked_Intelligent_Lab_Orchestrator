$toolRoot = Split-Path -Parent $PSScriptRoot
$values = @{}
foreach ($rawLine in Get-Content -LiteralPath (Join-Path $toolRoot 'toolchain.versions')) {
    $line = $rawLine.Trim()
    if ($line.Length -eq 0 -or $line.StartsWith('#')) { continue }
    if ($line -notmatch '^([A-Z0-9_]+)=([0-9a-z.]+)$') { throw "Invalid toolchain manifest line: $rawLine" }
    $values[$Matches[1]] = $Matches[2]
}
$platformKey = 'windows-x86_64'
$goBin = Join-Path $toolRoot "runtime\$platformKey\go\$($values['GO_VERSION'])\go\bin"
$goExecutable = Join-Path $goBin 'go.exe'

if ($args.Count -lt 1) {
    throw 'Pass a command to run, for example: .\02_Tools\env\with-go.ps1 go version'
}

if (-not (Test-Path -LiteralPath $goExecutable -PathType Leaf)) {
    throw "Repository-local Go $($values['GO_VERSION']) is missing at $goExecutable. Run .\02_Tools\bootstrap\bootstrap.ps1."
}

$env:PATH = "$goBin;$env:PATH"
$executable = $args[0]
$commandArguments = if ($args.Count -gt 1) { @($args[1..($args.Count - 1)]) } else { @() }
& $executable $commandArguments
exit $LASTEXITCODE
