param(
  [string]$Base = "",
  [string]$User = "admin",
  [string]$Pass = ""
)
if (-not $Base) { $Base = $env:WC_BASE }
if (-not $Pass) { $Pass = $env:WC_ADMIN_PASSWORD }
if (-not $Base -or -not $Pass) {
  throw "usage: e2e-lan.ps1 -Base http://192.168.1.1:8085 -Pass <password>"
}
$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
$agentExe = Join-Path $root "dist\wake-agent-windows-amd64.exe"
$agentDir = Join-Path $env:TEMP "wc-lan-e2e"
$configPath = Join-Path $agentDir "config.json"

Write-Host "== local IPv4 =="
Get-NetIPAddress -AddressFamily IPv4 | Where-Object { $_.IPAddress -like "192.168.*" } |
  Select-Object IPAddress, InterfaceAlias | Format-Table -AutoSize

if (Test-Path $agentDir) { Remove-Item -Recurse -Force $agentDir }

$login = Invoke-RestMethod -Uri "$Base/api/v1/auth/login" -Method Post -ContentType "application/json" `
  -Body (@{ username = $User; password = $Pass } | ConvertTo-Json) -TimeoutSec 10
$headers = @{ Authorization = "Bearer $($login.accessToken)" }

$existing = Invoke-RestMethod -Uri "$Base/api/v1/hosts" -Method Get -Headers $headers
foreach ($item in @($existing.items)) {
  if ($item -and $item.name -eq "e2e-lan") {
    Invoke-RestMethod -Uri "$Base/api/v1/hosts/$($item.id)" -Method Delete -Headers $headers | Out-Null
  }
}

$hostBody = @{
  name = "e2e-lan"; osType = "windows"; mac = "AA:BB:CC:DD:EE:99"
  broadcastTargets = @("192.168.1.255:9")
} | ConvertTo-Json
$hostRec = Invoke-RestMethod -Uri "$Base/api/v1/hosts" -Method Post -Headers $headers `
  -ContentType "application/json" -Body $hostBody
Write-Host "created host $($hostRec.id)"

$ec = Invoke-RestMethod -Uri "$Base/api/v1/hosts/$($hostRec.id)/agent/enroll-code" -Method Post -Headers $headers
Write-Host "enroll code $($ec.code)"

& $agentExe enroll --server $Base --code $ec.code --config $configPath
if ($LASTEXITCODE -ne 0) { throw "enroll failed" }

$outLog = Join-Path $agentDir "agent.out.log"
$errLog = Join-Path $agentDir "agent.err.log"
$proc = Start-Process -FilePath $agentExe -ArgumentList @("run", "--config", $configPath) `
  -RedirectStandardOutput $outLog -RedirectStandardError $errLog -WindowStyle Hidden -PassThru
Write-Host "agent pid $($proc.Id)"
Start-Sleep -Seconds 4

$detail = Invoke-RestMethod -Uri "$Base/api/v1/hosts/$($hostRec.id)" -Method Get -Headers $headers
Write-Host "status=$($detail.status) ip=$($detail.ip) agent=$($detail.agentVersion) shutdown=$($detail.capabilities.shutdown)"

$power = $null
try {
  $power = Invoke-RestMethod -Uri "$Base/api/v1/hosts/$($hostRec.id)/power" -Method Post -Headers $headers `
    -ContentType "application/json" -Body (@{ action = "cancel" } | ConvertTo-Json) -TimeoutSec 20
  Write-Host "command accepted=$($power.accepted)"
} catch {
  Write-Host "command failed: $($_.Exception.Message)"
  if ($_.ErrorDetails) { Write-Host $_.ErrorDetails.Message }
}
Start-Sleep -Seconds 2

Write-Host "== agent stderr =="
if (Test-Path $errLog) { Get-Content $errLog -Tail 10 }

Invoke-RestMethod -Uri "$Base/api/v1/hosts/$($hostRec.id)" -Method Delete -Headers $headers | Out-Null
Stop-Process -Id $proc.Id -Force -ErrorAction SilentlyContinue
Write-Host "cleaned up"
