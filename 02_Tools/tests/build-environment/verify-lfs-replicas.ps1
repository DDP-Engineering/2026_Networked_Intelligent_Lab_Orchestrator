[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][string]$ManifestPath,
    [Parameter(Mandatory = $true)][string]$GiteaReadbackPath,
    [Parameter(Mandatory = $true)][string]$BackupReadbackPath,
    [string]$GitHubReadbackPath
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$manifest = Get-Content -Raw -LiteralPath $ManifestPath | ConvertFrom-Json
function Test-Readback {
    param([string]$Name, [string]$Root)
    foreach ($part in $manifest.parts) {
        $path = Join-Path $Root $part.filename
        if (-not (Test-Path -LiteralPath $path -PathType Leaf)) { throw "$Name readback is missing $($part.filename)." }
        $hash = (Get-FileHash -Algorithm SHA256 -LiteralPath $path).Hash.ToLowerInvariant()
        if ($hash -ne $part.sha256) { throw "$Name readback hash mismatch for $($part.filename)." }
    }
    Write-Output "$Name readback verified: $($manifest.parts.Count) complete parts."
}
Test-Readback -Name 'Gitea clean-cache' -Root $GiteaReadbackPath
Test-Readback -Name 'Independent backup' -Root $BackupReadbackPath
if ($manifest.publication.github.status -eq 'complete') {
    if ([string]::IsNullOrWhiteSpace($GitHubReadbackPath)) { throw 'A complete GitHub replica declaration requires -GitHubReadbackPath.' }
    Test-Readback -Name 'GitHub clean-cache' -Root $GitHubReadbackPath
}
elseif ($manifest.publication.github.status -notin @('absent', 'pending')) {
    throw 'A partial GitHub archive is not a usable replica.'
}
