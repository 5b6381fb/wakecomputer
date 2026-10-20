param(
  [string]$Base = "http://127.0.0.1:18085",
  [string]$User = "admin",
  [string]$Pass = "testpass123"
)
$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
$agentExe = Join-Path $root "dist\wake-agent.exe"
$agentDir = Join-Path $env:TEMP "wc-agent-e2e"
$configPath = Join-Path $agentDir "config.json"

if (Test-Path $agentDir) { Remove-Item -Recurse -Force $agentDir }

$login = Invoke-RestMethod -Uri "$Base/api/v1/auth/login" -Method Post -ContentType "application/json" `
  -Body (@{ username = $User; password = $Pass } | ConvertTo-Json)
$headers = @{ Authorization = "Bearer $($login.accessToken)" }

# Remove leftovers from previous runs.
$existing = Invoke-RestMethod -Uri "$Base/api/v1/hosts" -Method Get -Headers $headers
foreach ($item in @($existing.items)) {
  if ($item -and $item.name -eq "e2e-agent") {
    Invoke-RestMethod -Uri "$Base/api/v1/hosts/$($item.id)" -Method Delete -Headers $headers | Out-Null
    Write-Host "removed stale host $($item.id)"
  }
}

$hostBody = @{
  name = "e2e-agent"; osType = "linux"; mac = "AA:BB:CC:DD:EE:02"; ip = "127.0.0.1"
  broadcastTargets = @("127.0.0.1:9"); probePort = 45871
} | ConvertTo-Json
$hostRec = Invoke-RestMethod -Uri "$Base/api/v1/hosts" -Method Post -Headers $headers -ContentType "application/json" -Body $hostBody
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
Start-Sleep -Seconds 3

$detail = Invoke-RestMethod -Uri "$Base/api/v1/hosts/$($hostRec.id)" -Method Get -Headers $headers
Write-Host "host status after heartbeat: $($detail.status) caps.shutdown=$($detail.capabilities.shutdown)"

$cfg = Get-Content $configPath -Raw | ConvertFrom-Json
$health = & curl.exe -s -k -H "Authorization: Bearer $($cfg.token)" "https://127.0.0.1:45871/v1/health"
Write-Host "agent health: $health"

$unauth = & curl.exe -s -k -o NUL -w "%{http_code}" "https://127.0.0.1:45871/v1/health"
Write-Host "agent unauth status: $unauth"

# Exercise the command channel with the harmless "cancel" action.
$power = Invoke-RestMethod -Uri "$Base/api/v1/hosts/$($hostRec.id)/power" -Method Post -Headers $headers `
  -ContentType "application/json" -Body (@{ action = "cancel" } | ConvertTo-Json)
Write-Host "power cancel accepted: $($power.accepted)"
Start-Sleep -Seconds 1

# Exercise the batch engine (wake only; nothing is powered off).
$batch = Invoke-RestMethod -Uri "$Base/api/v1/batch" -Method Post -Headers $headers `
  -ContentType "application/json" -Body (@{ hostIds = @($hostRec.id); action = "wake" } | ConvertTo-Json)
Write-Host "batch job: $($batch.jobId)"
$job = $null
for ($i = 0; $i -lt 10; $i++) {
  Start-Sleep -Milliseconds 500
  $job = Invoke-RestMethod -Uri "$Base/api/v1/jobs/$($batch.jobId)" -Method Get -Headers $headers
  if ($job.status -ne "running") { break }
}
Write-Host "batch status: $($job.status) ok=$($job.succeeded) failed=$($job.failed)"

Write-Host "agent stderr:"
if (Test-Path $errLog) { Get-Content $errLog -Tail 10 }

Invoke-RestMethod -Uri "$Base/api/v1/hosts/$($hostRec.id)" -Method Delete -Headers $headers | Out-Null
Stop-Process -Id $proc.Id -Force -ErrorAction SilentlyContinue
Write-Host "cleaned up"
