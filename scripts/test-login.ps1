param(
    [ValidateSet('Full','Send','Confirm','Status')][string]$Step = 'Full',
    [string]$Phone = '',
    [string]$SessionID = '',
    [string]$BaseURL = 'http://127.0.0.1:18080'
)
$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
$statePath = Join-Path $projectRoot '.tools/login-test-session.json'
$baseUri = [Uri]$BaseURL
if (!$baseUri.IsAbsoluteUri -or $baseUri.Scheme -ne 'http' -or !$baseUri.IsLoopback -or $baseUri.UserInfo -or $baseUri.AbsolutePath -ne '/' -or $baseUri.Query -or $baseUri.Fragment) { throw 'BaseURL must be a loopback HTTP address with no path or credentials.' }
$BaseURL = $BaseURL.TrimEnd('/')
$headers = @{ 'X-Panguai-Client' = 'web' }

function Invoke-LocalAPI([string]$Method, [string]$Path, $Body = $null) {
    $parameters = @{ Uri = "$BaseURL$Path"; Method = $Method; Headers = $headers; TimeoutSec = 30; ErrorAction = 'Stop' }
    if ($null -ne $Body) {
        $parameters['ContentType'] = 'application/json; charset=utf-8'
        $parameters['Body'] = [Text.Encoding]::UTF8.GetBytes(($Body | ConvertTo-Json -Compress))
    }
    try { return Invoke-RestMethod @parameters }
    catch {
        $message = 'Local API request failed. No request is automatically retried.'
        try {
            $failure = $_.ErrorDetails.Message | ConvertFrom-Json
            if ($failure.message) { $message = $failure.message }
            if ($failure.code) { $message += " [$($failure.code)]" }
            if ($failure.upstream_path) { $message += " @ $($failure.upstream_path)" }
        } catch { }
        throw $message
    }
}

$status = Invoke-LocalAPI 'GET' '/api/status'
if (!$status.protocol_ready) { throw 'Backend platform configuration is not ready.' }
Write-Host 'Backend is ready. SMS is only sent in Full or Send mode.'

if ($Step -eq 'Full' -or $Step -eq 'Send') {
    if (!$Phone) { $Phone = Read-Host 'Phone number (your own account)' }
    $Phone = $Phone.Trim()
    if ($Phone -notmatch '^1[3-9][0-9]{9}$') { throw 'Invalid phone number.' }
    $maskedPhone = $Phone.Substring(0,3) + '****' + $Phone.Substring(7)
    $sent = Invoke-LocalAPI 'POST' '/api/login/sms/send' @{ phone = $Phone }
    $Phone = $null
    $SessionID = $sent.session_id
    if (!$SessionID -or $SessionID -notmatch '^[a-f0-9]{48}$') { throw 'API did not confirm a login session.' }
    New-Item -ItemType Directory -Force -Path (Split-Path -Parent $statePath) | Out-Null
    @{ base_url = $BaseURL; session_id = $SessionID; phone_masked = $maskedPhone } | ConvertTo-Json | Set-Content -LiteralPath $statePath -Encoding utf8
    Write-Host "SMS confirmed for $maskedPhone. Session is valid for $($sent.expires_in) seconds."
    if ($Step -eq 'Send') { Write-Host 'Next: .\scripts\test-login.ps1 -Step Confirm'; return }
}

if (!$SessionID -and (Test-Path -LiteralPath $statePath -PathType Leaf)) {
    $saved = Get-Content -LiteralPath $statePath -Raw | ConvertFrom-Json
    if ($saved.base_url -ne $BaseURL) { throw 'Saved session belongs to a different local API address.' }
    $SessionID = $saved.session_id
}
if (!$SessionID -or $SessionID -notmatch '^[a-f0-9]{48}$') { throw 'No valid local test session. Run -Step Send first.' }

if ($Step -eq 'Status') {
    Invoke-LocalAPI 'GET' "/api/login/sessions/$SessionID" | ConvertTo-Json -Depth 4
    return
}

$sessionState = Invoke-LocalAPI 'GET' "/api/login/sessions/$SessionID"
$secureCode = $null
$pointer = [IntPtr]::Zero
try {
    $confirmBody = @{ session_id = $SessionID }
    if ($sessionState.status -ne 'verifying' -and $sessionState.status -ne 'confirmed') {
        $secureCode = Read-Host 'SMS code (hidden; not saved to disk)' -AsSecureString
        $pointer = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($secureCode)
        $code = [Runtime.InteropServices.Marshal]::PtrToStringBSTR($pointer)
        if ($code -notmatch '^[0-9]{4,8}$') { throw 'Invalid SMS code format.' }
        $confirmBody['code'] = $code
    }
    $confirmed = Invoke-LocalAPI 'POST' '/api/login/sms/confirm' $confirmBody
} finally {
    if ($pointer -ne [IntPtr]::Zero) { [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($pointer) }
    $code = $null
    $confirmBody = $null
    if ($secureCode) { $secureCode.Dispose() }
}
if ($confirmed.status -ne 'confirmed' -or !$confirmed.account.id) { throw 'API did not confirm login.' }
$accountID = [Uri]::EscapeDataString($confirmed.account.id)
$checked = Invoke-LocalAPI 'POST' "/api/accounts/$accountID/check" @{}
if ($checked.status -ne 'valid') { throw 'Login completed but account state could not be checked.' }
Write-Host 'Login and saved-account check succeeded. No platform token is returned by the API.'
$checked.account | Format-List id,name,phone,updated_at
