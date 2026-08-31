$ErrorActionPreference = 'Stop'

function Invoke-GoCommand {
    param([string[]]$Arguments)

    & go @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "go $($Arguments -join ' ') exited with code $LASTEXITCODE"
    }
}

$scriptDirectory = Split-Path -Parent $PSCommandPath
$softwareRoot = Resolve-Path (Join-Path $scriptDirectory '..')
$moduleRoot = Join-Path $softwareRoot 'nilo_assistant'
$temporaryFile = New-TemporaryFile
Remove-Item -LiteralPath $temporaryFile
$temporaryDirectory = "$temporaryFile-nilo-portable-core"
New-Item -ItemType Directory -Path $temporaryDirectory | Out-Null
$temporaryDirectory = (Resolve-Path -LiteralPath $temporaryDirectory).Path

$previousCgo = $env:CGO_ENABLED
$previousGoos = $env:GOOS
$previousGoarch = $env:GOARCH

try {
    Push-Location $moduleRoot
    Invoke-GoCommand @('test', './...')
    Invoke-GoCommand @('build', '-o', (Join-Path $temporaryDirectory 'nilo-host.exe'), './cmd/nilo')

    foreach ($target in @(
        @{ OS = 'linux'; Arch = 'arm64'; Extension = '' },
        @{ OS = 'linux'; Arch = 'amd64'; Extension = '' },
        @{ OS = 'windows'; Arch = 'amd64'; Extension = '.exe' }
    )) {
        $env:CGO_ENABLED = '0'
        $env:GOOS = $target.OS
        $env:GOARCH = $target.Arch
        Invoke-GoCommand @('build', '-o', (Join-Path $temporaryDirectory ("nilo-$($target.OS)-$($target.Arch)$($target.Extension)")), './cmd/nilo')
    }
}
finally {
    Pop-Location
    $env:CGO_ENABLED = $previousCgo
    $env:GOOS = $previousGoos
    $env:GOARCH = $previousGoarch
    if (Test-Path -LiteralPath $temporaryDirectory) {
        Remove-Item -LiteralPath $temporaryDirectory -Recurse -Force
    }
}
