param(
  [string]$Base = "http://127.0.0.1:18085",
  [string]$User = "admin",
  [string]$Pass = "testpass123"
)
$ErrorActionPreference = "Stop"

function Show($label, $value) {
  Write-Host "--- $label ---"
  if ($value -is [string]) { Write-Host $value } else { $value | ConvertTo-Json -Depth 8 }
}

$health = Invoke-RestMethod -Uri "$Base/healthz" -Method Get
Show "health" $health

$loginBody = @{ username = $User; password = $Pass } | ConvertTo-Json
$login = Invoke-RestMethod -Uri "$Base/api/v1/auth/login" -Method Post -ContentType "application/json" -Body $loginBody
Show "login" @{ user = $login.user.username; expiresIn = $login.expiresIn; tokenPrefix = $login.accessToken.Substring(0, 16) }
$H = @{ Authorization = "Bearer $($login.accessToken)" }

$hosts = Invoke-RestMethod -Uri "$Base/api/v1/hosts" -Method Get -Headers $H
Show "hosts-before" $hosts

$newHost = @{
  name = "smoke-win"; osType = "windows"
  mac = "AA:BB:CC:DD:EE:FF"; ip = "127.0.0.1"
  broadcastTargets = @("127.0.0.1:9"); probePort = 135
  notes = "smoke test"
} | ConvertTo-Json
$created = Invoke-RestMethod -Uri "$Base/api/v1/hosts" -Method Post -Headers $H -ContentType "application/json" -Body $newHost
Show "host-created" @{ id = $created.id; name = $created.name; status = $created.status; wake = $created.capabilities.wake }

$wake = Invoke-RestMethod -Uri "$Base/api/v1/hosts/$($created.id)/wake" -Method Post -Headers $H -ContentType "application/json" -Body "{}"
Show "wake" $wake

$probe = Invoke-RestMethod -Uri "$Base/api/v1/hosts/$($created.id)/probe" -Method Post -Headers $H
Show "probe" $probe

$ec = Invoke-RestMethod -Uri "$Base/api/v1/hosts/$($created.id)/agent/enroll-code" -Method Post -Headers $H
Show "enroll-code" @{ code = $ec.code; linux = $ec.linuxCommand; windows = $ec.windowsCommand }

$groups = Invoke-RestMethod -Uri "$Base/api/v1/groups" -Method Get -Headers $H
Show "groups" $groups

$audit = Invoke-RestMethod -Uri "$Base/api/v1/audit?limit=10" -Method Get -Headers $H
Show "audit" ($audit.items | Select-Object action, result, hostName, createdAt)

$settings = Invoke-RestMethod -Uri "$Base/api/v1/settings" -Method Get -Headers $H
Show "settings" $settings

$bad = $null
try {
  Invoke-RestMethod -Uri "$Base/api/v1/hosts" -Method Get | Out-Null
} catch {
  $bad = $_.Exception.Response.StatusCode.value__
}
Show "unauthorized-status" $bad

Invoke-RestMethod -Uri "$Base/api/v1/hosts/$($created.id)" -Method Delete -Headers $H | Out-Null
Show "deleted" "ok"
