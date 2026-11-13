param(
    [ValidateRange(1024,65535)][int]$Port = 18080,
    [switch]$SkipBuild
)
$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
$cacheDir = Join-Path $projectRoot '.cache'
$binaryPath = Join-Path $cacheDir 'panguai-api.exe'
New-Item -ItemType Directory -Force -Path $cacheDir | Out-Null
if (!$SkipBuild) {
    & go -C (Join-Path $projectRoot 'backend') build -buildvcs=false -o $binaryPath ./cmd/panguai
    if ($LASTEXITCODE -ne 0) { throw 'Go backend build failed.' }
}
if (!(Test-Path -LiteralPath $binaryPath -PathType Leaf)) { throw 'Backend executable not found; run without -SkipBuild.' }
& $binaryPath -addr "127.0.0.1:$Port" -data (Join-Path $projectRoot 'data')
exit $LASTEXITCODE
