[CmdletBinding()]
param(
    [string]$Proposal = ""
)
$ErrorActionPreference = 'Stop'
$repository = (Resolve-Path (Join-Path $PSScriptRoot '..\..\..')).Path
$cache = if ($env:NILO_PACKAGING_CACHE) { $env:NILO_PACKAGING_CACHE } else { Join-Path $repository '02_Tools\cache\packaging' }
if (-not $Proposal) { $Proposal = Join-Path $repository '02_Tools\packaging\config\inputs.lock.proposal.json' }
& python (Join-Path $repository '02_Tools\packaging\package_inputs.py') resolve `
    --targets (Join-Path $repository '02_Tools\packaging\config\targets.json') `
    --declarations (Join-Path $repository '02_Tools\packaging\config\release-inputs.json') `
    --cache $cache `
    --proposal $Proposal
exit $LASTEXITCODE
