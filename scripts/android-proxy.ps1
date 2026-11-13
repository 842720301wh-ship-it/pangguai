param(
    [ValidateSet('Start','Restore')][string]$Action = 'Start',
    [string]$AdbPath = 'C:/Users/84272/AppData/Local/Android/Sdk/platform-tools/adb.exe',
    [string]$Serial = '',
    [ValidateRange(1024,65535)][int]$PhonePort = 19000,
    [ValidateRange(1024,65535)][int]$DesktopPort = 9000
)
# This helper manages USB forwarding only. It never writes Android settings,
# installs certificates, starts a VPN or changes the Windows system proxy.
$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
$statePath = Join-Path $projectRoot '.tools/adb-reverse-state.json'
if (!(Test-Path -LiteralPath $AdbPath -PathType Leaf)) { throw 'ADB executable not found.' }
if ($Action -eq 'Restore') {
    if (!(Test-Path -LiteralPath $statePath)) { Write-Output 'No owned USB mapping to remove.'; exit }
    $saved = Get-Content -LiteralPath $statePath -Raw | ConvertFrom-Json
    $mapping = (& $AdbPath -s $saved.serial reverse --list) -join "`n"
    if ($saved.created_by_this_script -and $mapping -match "tcp:$($saved.phone_port)\s+tcp:$($saved.desktop_port)") {
        & $AdbPath -s $saved.serial reverse --remove "tcp:$($saved.phone_port)" | Out-Null
    }
    Write-Output 'Owned USB mapping removed; system network settings untouched.'
    exit
}
if (!$Serial) {
    $deviceRows = @(& $AdbPath devices | Where-Object { $_ -match '^\S+\s+device$' })
    if ($deviceRows.Count -ne 1) { throw 'Specify -Serial when exactly one authorized device is not connected.' }
    $Serial = ($deviceRows[0] -split '\s+')[0]
}
$mapping = (& $AdbPath -s $Serial reverse --list) -join "`n"
$existing = [regex]::Match($mapping, "(?m)^\S+\s+tcp:$PhonePort\s+tcp:(\d+)\s*$")
if ($existing.Success -and [int]$existing.Groups[1].Value -ne $DesktopPort) { throw 'The requested phone port belongs to another forwarding rule.' }
$owned = !$existing.Success
New-Item -ItemType Directory -Force -Path (Split-Path -Parent $statePath) | Out-Null
@{ serial=$Serial; phone_port=$PhonePort; desktop_port=$DesktopPort; created_by_this_script=$owned } | ConvertTo-Json | Set-Content -LiteralPath $statePath -Encoding utf8
try {
    if ($owned) {
        & $AdbPath -s $Serial reverse "tcp:$PhonePort" "tcp:$DesktopPort"
        if ($LASTEXITCODE -ne 0) { throw 'USB forwarding failed.' }
    }
    Write-Output "USB forwarding active: phone $PhonePort -> desktop $DesktopPort. System proxy unchanged."
    while ($true) {
        Start-Sleep -Seconds 4
        $deviceState = (& $AdbPath -s $Serial get-state 2>$null) -join ''
        if ($deviceState -ne 'device') { Write-Output 'Device disconnected; ending forwarding session.'; break }
        $mapping = (& $AdbPath -s $Serial reverse --list) -join "`n"
        if ($owned -and $mapping -notmatch "tcp:$PhonePort\s+tcp:$DesktopPort") {
            & $AdbPath -s $Serial reverse "tcp:$PhonePort" "tcp:$DesktopPort" | Out-Null
            if ($LASTEXITCODE -ne 0) { throw 'USB forwarding lost.' }
        }
    }
} finally {
    $mapping = (& $AdbPath -s $Serial reverse --list 2>$null) -join "`n"
    if ($owned -and $mapping -match "tcp:$PhonePort\s+tcp:$DesktopPort") {
        & $AdbPath -s $Serial reverse --remove "tcp:$PhonePort" | Out-Null
    }
}
