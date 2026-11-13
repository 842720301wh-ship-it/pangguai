param(
    [string]$Backend = 'http://127.0.0.1:18080',
    [string]$AccountID = '',
    [switch]$PreviewOnly
)
$ErrorActionPreference = 'Stop'
$uri = [Uri]$Backend
if ($uri.Scheme -ne 'http' -or -not $uri.IsLoopback -or $uri.UserInfo -or $uri.Query -or $uri.Fragment -or $uri.AbsolutePath -ne '/') {
    throw 'Backend must be a loopback HTTP origin.'
}
$Backend = $Backend.TrimEnd('/')
$headers = @{'X-Panguai-Client'='web'}
function Invoke-LocalApi([string]$Path, $Payload = $null) {
    $argsForRequest = @{ Uri="$Backend/api$Path"; Headers=$headers; TimeoutSec=30 }
    if ($null -ne $Payload) {
        $argsForRequest.Method = 'Post'
        $argsForRequest.ContentType = 'application/json'
        $argsForRequest.Body = [Text.Encoding]::UTF8.GetBytes(($Payload | ConvertTo-Json -Compress))
    }
    try { return Invoke-RestMethod @argsForRequest } catch {
        # Show only the backend's controlled message, never the raw HTTP body.
        $message = 'Local API failed. Check the official App before repeating a start request.'
        if ($_.ErrorDetails.Message) {
            try {
                $failure = $_.ErrorDetails.Message | ConvertFrom-Json
                if ($failure.message) { $message = [string]$failure.message }
            } catch { }
        }
        throw $message
    }
}
function Select-Index([int]$Count, [string]$Prompt) {
    [int]$index = 0
    $answer = Read-Host $Prompt
    if (-not [int]::TryParse($answer, [ref]$index) -or $index -lt 1 -or $index -gt $Count) { throw 'Selection cancelled.' }
    return $index - 1
}
$accounts = @((Invoke-LocalApi '/accounts').accounts)
if ($AccountID) { $accounts = @($accounts | Where-Object { $_.id -eq $AccountID }) }
if ($accounts.Count -ne 1) { throw 'Select one saved own account using -AccountID. This script never sends SMS.' }
$prefix = '/accounts/' + [Uri]::EscapeDataString($accounts[0].id)
$devices = @((Invoke-LocalApi "$prefix/devices").devices)
if (-not $devices.Count) { throw 'No recently used devices in the own account.' }
for ($i=0; $i -lt $devices.Count; $i++) {
    Write-Host ('{0}. {1} | {2} | workStatus={3}' -f ($i+1), $devices[$i].goodsName, $devices[$i].orgName, $devices[$i].workStatus)
}
$device = $devices[(Select-Index $devices.Count 'Select the physical device next to you')]
$path = "$prefix/devices/" + [Uri]::EscapeDataString([string]$device.goodsId) + '/usage'
$options = Invoke-LocalApi "$path/options"
Write-Host $options.billing
if (-not $options.start_supported) { throw 'This device mode is not supported by the first startup integration.' }
$services = @($options.device.services)
for ($i=0; $i -lt $services.Count; $i++) {
    $sku = $services[$i]
    Write-Host ('{0}. {1} | price={2}, amount={3}, unit={4}, discountPrice={5}, available={6}' -f ($i+1), $sku.name, $sku.price, $sku.amount, $sku.unit, $sku.discountPrice, $sku.scanAvailable)
    if ($sku.billing_unit) { Write-Host ('   Metered price(s): {0} {1}' -f (($sku.billing_levels | ForEach-Object { $_.price }) -join ', '), $sku.billing_unit) }
}
if ($PreviewOnly) { Write-Host 'Read-only preview complete. No startup request sent.'; return }
if (-not $services.Count) { throw 'No service options available.' }
$service = $services[(Select-Index $services.Count 'Select service')]
# Actual location stays in memory. No inferred/spoofed coordinates are supplied.
$culture = [Globalization.CultureInfo]::InvariantCulture
[double]$latitude = 0
[double]$longitude = 0
if (-not [double]::TryParse((Read-Host 'Actual latitude (decimal degrees)'), [Globalization.NumberStyles]::Float, $culture, [ref]$latitude)) { throw 'Invalid latitude.' }
if (-not [double]::TryParse((Read-Host 'Actual longitude (decimal degrees)'), [Globalization.NumberStyles]::Float, $culture, [ref]$longitude)) { throw 'Invalid longitude.' }
$prepared = Invoke-LocalApi "$path/prepare" @{sku_id=[string]$service.skuId; latitude=$latitude; longitude=$longitude}
Write-Host ('Device: {0}; service: {1}' -f $prepared.device.name, $prepared.service.name)
Write-Host ('Platform billing: price={0}, amount={1}, unit={2}, discountPrice={3}' -f $prepared.service.price, $prepared.service.amount, $prepared.service.unit, $prepared.service.discountPrice)
if ($prepared.service.billing_unit) { Write-Host ('Metered price(s): {0} {1}' -f (($prepared.service.billing_levels | ForEach-Object { $_.price }) -join ', '), $prepared.service.billing_unit) }
Write-Host 'The platform unit field is a raw value, not a currency/measurement label. Compare the metered rate with the official App.'
Write-Host $prepared.billing
if ((Read-Host 'Type START to accept metered billing and send ONE startup request; Enter cancels') -cne 'START') {
    Write-Host 'Cancelled. No startup request sent.'
    return
}
$result = Invoke-LocalApi "$path/start" @{confirmation_key=$prepared.confirmation_key; confirm=$true}
Write-Host ('Result: {0}. {1}' -f $result.status, $result.message)
Write-Host 'Verify the physical device and official App. This script does not automatically retry, end usage or pay an order.'
