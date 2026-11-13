param([switch]$SkipBuild)
$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
if (!$SkipBuild) {
    Push-Location (Join-Path $projectRoot 'frontend')
    try {
        & npm.cmd run build
        if ($LASTEXITCODE -ne 0) { throw 'Frontend build failed.' }
    } finally { Pop-Location }
}
$frontendOutput = Join-Path $projectRoot 'dist/web'
if (!(Test-Path -LiteralPath (Join-Path $frontendOutput 'index.html'))) { throw 'Build the frontend first.' }
& go -C (Join-Path $projectRoot 'backend') run -buildvcs=false ./cmd/panguai -data (Join-Path $projectRoot 'data') -web $frontendOutput
exit $LASTEXITCODE
