param(
    [string]$Backend = 'http://127.0.0.1:18080',
    [string]$AccountID = ''
)
$ErrorActionPreference = 'Stop'
$uri = [Uri]$Backend
if ($uri.Scheme -ne 'http' -or -not $uri.IsLoopback -or $uri.UserInfo -or $uri.Query -or $uri.Fragment -or $uri.AbsolutePath -ne '/') {
    throw 'Backend must be a loopback HTTP origin.'
}
$Backend = $Backend.TrimEnd('/')
$headers = @{'X-Panguai-Client'='web'}
$reply = Invoke-RestMethod -Uri "$Backend/api/accounts" -Headers $headers -TimeoutSec 30
$accounts = @($reply.accounts)
if ($AccountID) { $accounts = @($accounts | Where-Object { $_.id -eq $AccountID }) }
if ($accounts.Count -eq 0) { throw 'No saved account found. This check does not start authorization or send SMS.' }
foreach ($account in $accounts) {
    $id = [Uri]::EscapeDataString($account.id)
    try {
        $result = Invoke-RestMethod -Uri "$Backend/api/accounts/$id/check" -Method Post -Headers $headers -ContentType 'application/json' -Body '{}' -TimeoutSec 35
    } catch {
        # Do not print raw HTTP error bodies that could contain account details.
        throw 'Saved credential check failed. No authorization or SMS request was started.'
    }
    if ($result.status -ne 'valid') { throw 'The platform did not confirm a valid account.' }
    $label = if($result.account.phone){$result.account.phone}else{'WeChat account'}
    Write-Host "Saved account is valid: $label"
}
Write-Host 'Check complete. No SMS or new WeChat authorization was requested.'
