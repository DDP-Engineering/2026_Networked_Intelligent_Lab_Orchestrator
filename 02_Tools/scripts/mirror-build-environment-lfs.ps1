[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][string]$ManifestPath,
    [Parameter(Mandatory = $true)][long]$GiteaCapacityBytes,
    [Parameter(Mandatory = $true)][long]$BackupCapacityBytes,
    [string]$GiteaRemote = 'gitea',
    [string]$GitHubRemote,
    [long]$GitHubAllowanceBytes = 0,
    [switch]$ExecutePublication
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$manifestFile = (Resolve-Path -LiteralPath $ManifestPath).Path
$archivePath = Split-Path -Parent $manifestFile
$core = Join-Path $PSScriptRoot 'build_environment.py'
& python $core inspect-parts --manifest $manifestFile --archive-path $archivePath
if ($LASTEXITCODE -ne 0) { throw 'Publication preflight rejected the local archive parts.' }
$manifest = Get-Content -Raw -LiteralPath $manifestFile | ConvertFrom-Json
if ($manifest.schema -ne 1 -or $manifest.parts.Count -eq 0) { throw 'A generated schema-1 manifest with at least one part is required.' }
$total = [long]$manifest.archive.size
if ($GiteaCapacityBytes -lt $total) { throw "Gitea capacity $GiteaCapacityBytes is smaller than the complete archive $total." }
if ($BackupCapacityBytes -lt $total) { throw "Backup capacity $BackupCapacityBytes is smaller than the complete archive $total." }
$githubComplete = -not [string]::IsNullOrWhiteSpace($GitHubRemote)
if ($githubComplete -and $GitHubAllowanceBytes -lt $total) {
    throw 'GitHub publication is all-or-nothing; the measured allowance cannot hold the complete generation. Omit GitHubRemote and record the replica as absent.'
}
foreach ($part in $manifest.parts) {
    if ([long]$part.size -gt 2000000000) { throw "Part $($part.filename) exceeds 2 GB." }
    if ($part.lfs_oid -ne "sha256:$($part.sha256)") { throw "Part $($part.filename) has inconsistent LFS identity." }
}
Write-Output "Preflight complete: archive=$total bytes; Gitea=$GiteaCapacityBytes; backup=$BackupCapacityBytes; GitHub complete replica=$githubComplete"
if (-not $ExecutePublication) {
    Write-Output 'No remote data was changed. Human operator: rerun with -ExecutePublication only after reviewing capacity, backup, remote names, and the complete OID set.'
    exit 0
}

foreach ($part in $manifest.parts) {
    & git lfs push --object-id $GiteaRemote $part.sha256
    if ($LASTEXITCODE -ne 0) { throw "Gitea LFS publication failed for $($part.lfs_oid)." }
}
if ($githubComplete) {
    foreach ($part in $manifest.parts) {
        & git lfs push --object-id $GitHubRemote $part.sha256
        if ($LASTEXITCODE -ne 0) { throw "GitHub LFS publication failed for $($part.lfs_oid); do not mark this generation as replicated." }
    }
}
Write-Output 'Publication commands completed. Clean-cache readback and independent backup verification are still required before updating current.json to complete.'
