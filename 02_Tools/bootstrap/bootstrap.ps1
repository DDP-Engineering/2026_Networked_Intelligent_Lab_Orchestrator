param(
    [switch]$Offline
)

$ErrorActionPreference = 'Stop'
$toolRoot = Split-Path -Parent $PSScriptRoot
$platformKey = 'windows-x86_64'
$runtimeRoot = Join-Path $toolRoot "runtime\$platformKey"
$cacheRoot = Join-Path $toolRoot "cache\$platformKey"
$downloadRoot = Join-Path $toolRoot 'cache\downloads'
$versionFile = Join-Path $toolRoot 'toolchain.versions'

function Read-ToolVersions {
    $values = @{}
    foreach ($rawLine in Get-Content -LiteralPath $versionFile) {
        $line = $rawLine.Trim()
        if ($line.Length -eq 0 -or $line.StartsWith('#')) {
            continue
        }
        if ($line -notmatch '^([A-Z0-9_]+)=([0-9a-z.]+)$') {
            throw "Invalid toolchain manifest line: $rawLine"
        }
        if ($values.ContainsKey($Matches[1])) {
            throw "Duplicate toolchain manifest key: $($Matches[1])"
        }
        $values[$Matches[1]] = $Matches[2]
    }
    return $values
}

function Assert-ArchiveDigest {
    param([string]$Path, [string]$Expected)
    $actual = (Get-FileHash -Algorithm SHA256 -LiteralPath $Path).Hash
    if ($actual -ne $Expected) {
        throw "SHA-256 mismatch for $Path; expected $Expected, got $actual"
    }
}

function Get-VerifiedArchive {
    param([string]$Url, [string]$Path, [string]$Digest)
    if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) {
        if ($Offline) {
            throw "Offline bootstrap requires cached archive: $Path"
        }
        Write-Output "Downloading $Url"
        & curl.exe -fL --retry 3 --output $Path $Url
        if ($LASTEXITCODE -ne 0) {
            throw "Download failed with exit code ${LASTEXITCODE}: $Url"
        }
    }
    Assert-ArchiveDigest -Path $Path -Expected $Digest
}

if ($env:PROCESSOR_ARCHITECTURE -and $env:PROCESSOR_ARCHITECTURE -ne 'AMD64') {
    throw "bootstrap.ps1 supports Windows AMD64; detected $env:PROCESSOR_ARCHITECTURE"
}

$versions = Read-ToolVersions
New-Item -ItemType Directory -Force -Path $runtimeRoot, $downloadRoot | Out-Null

$goVersion = $versions['GO_VERSION']
$goArchiveName = "go$goVersion.windows-amd64.zip"
$goArchive = Join-Path $downloadRoot $goArchiveName
$goExecutable = Join-Path $runtimeRoot "go\$goVersion\go\bin\go.exe"
if (-not (Test-Path -LiteralPath $goExecutable -PathType Leaf)) {
    Get-VerifiedArchive -Url "https://go.dev/dl/$goArchiveName" -Path $goArchive -Digest $versions['GO_WINDOWS_AMD64_SHA256']
    $goVersionRoot = Join-Path $runtimeRoot "go\$goVersion"
    $stagingRoot = Join-Path $runtimeRoot "bootstrap-go-$PID"
    if (Test-Path -LiteralPath $stagingRoot) {
        throw "Refusing to replace existing bootstrap staging directory: $stagingRoot"
    }
    New-Item -ItemType Directory -Path $stagingRoot | Out-Null
    try {
        & tar.exe -xf $goArchive -C $stagingRoot
        if ($LASTEXITCODE -ne 0) {
            throw "Go extraction failed with exit code $LASTEXITCODE"
        }
        New-Item -ItemType Directory -Force -Path $goVersionRoot | Out-Null
        Move-Item -LiteralPath (Join-Path $stagingRoot 'go') -Destination $goVersionRoot
    }
    finally {
        if (Test-Path -LiteralPath $stagingRoot) {
            if ((Get-ChildItem -Force -LiteralPath $stagingRoot).Count -eq 0) {
                Remove-Item -LiteralPath $stagingRoot -Force
            }
            else {
                Write-Warning "Preserving non-empty failed bootstrap staging directory: $stagingRoot"
            }
        }
    }
}

$nodeVersion = $versions['NODE_VERSION']
$nodeArchiveName = "node-v$nodeVersion-win-x64.zip"
$nodeArchive = Join-Path $downloadRoot $nodeArchiveName
$nodeRoot = Join-Path $runtimeRoot "node\$nodeVersion"
$nodeExecutable = Join-Path $nodeRoot 'node.exe'
if (-not (Test-Path -LiteralPath $nodeExecutable -PathType Leaf)) {
    if (Test-Path -LiteralPath $nodeRoot) {
        throw "Refusing to replace incomplete Node.js runtime: $nodeRoot"
    }
    Get-VerifiedArchive -Url "https://nodejs.org/dist/v$nodeVersion/$nodeArchiveName" -Path $nodeArchive -Digest $versions['NODE_WINDOWS_AMD64_SHA256']
    $stagingRoot = Join-Path $runtimeRoot "bootstrap-node-$PID"
    if (Test-Path -LiteralPath $stagingRoot) {
        throw "Refusing to replace existing bootstrap staging directory: $stagingRoot"
    }
    New-Item -ItemType Directory -Path $stagingRoot | Out-Null
    try {
        & tar.exe -xf $nodeArchive -C $stagingRoot
        if ($LASTEXITCODE -ne 0) {
            throw "Node.js extraction failed with exit code $LASTEXITCODE"
        }
        New-Item -ItemType Directory -Force -Path (Split-Path -Parent $nodeRoot) | Out-Null
        Move-Item -LiteralPath (Join-Path $stagingRoot "node-v$nodeVersion-win-x64") -Destination $nodeRoot
    }
    finally {
        if (Test-Path -LiteralPath $stagingRoot) {
            if ((Get-ChildItem -Force -LiteralPath $stagingRoot).Count -eq 0) {
                Remove-Item -LiteralPath $stagingRoot -Force
            }
            else {
                Write-Warning "Preserving non-empty failed bootstrap staging directory: $stagingRoot"
            }
        }
    }
}

$uvVersion = $versions['UV_VERSION']
$uvArchiveName = 'uv-x86_64-pc-windows-msvc.zip'
$uvArchive = Join-Path $downloadRoot "uv-$uvVersion-windows-amd64.zip"
$uvRoot = Join-Path $runtimeRoot "uv\$uvVersion"
$uvExecutable = Join-Path $uvRoot 'uv.exe'
if (-not (Test-Path -LiteralPath $uvExecutable -PathType Leaf)) {
    Get-VerifiedArchive -Url "https://github.com/astral-sh/uv/releases/download/$uvVersion/$uvArchiveName" -Path $uvArchive -Digest $versions['UV_WINDOWS_AMD64_SHA256']
    New-Item -ItemType Directory -Force -Path $uvRoot | Out-Null
    & tar.exe -xf $uvArchive -C $uvRoot
    if ($LASTEXITCODE -ne 0) {
        throw "uv extraction failed with exit code $LASTEXITCODE"
    }
}

$env:UV_CACHE_DIR = Join-Path $cacheRoot 'uv'
$env:UV_PYTHON_INSTALL_DIR = Join-Path $runtimeRoot 'python'
$env:UV_PYTHON_BIN_DIR = Join-Path $runtimeRoot 'python-bin'
$env:UV_PYTHON_NO_REGISTRY = '1'
$env:UV_PYTHON_PREFERENCE = 'only-managed'
$env:UV_PROJECT_ENVIRONMENT = ".venv-$platformKey"
New-Item -ItemType Directory -Force -Path $env:UV_CACHE_DIR, $env:UV_PYTHON_INSTALL_DIR, $env:UV_PYTHON_BIN_DIR | Out-Null

$pythonArguments = @('python', 'install', $versions['PYTHON_VERSION'])
if ($Offline) {
    $pythonArguments = @('--offline') + $pythonArguments
}
& $uvExecutable $pythonArguments
if ($LASTEXITCODE -ne 0) {
    throw "Managed Python installation failed with exit code $LASTEXITCODE"
}

& (Join-Path $toolRoot 'scripts\doctor.ps1')
exit $LASTEXITCODE
