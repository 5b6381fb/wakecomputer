param([string]$Version = "0.1.0")

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

$env:CGO_ENABLED = "0"
$env:GOTOOLCHAIN = "local"
$buildTime = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
$commit = "unknown"
if (Test-Path .git) {
  $commit = (git rev-parse --short HEAD 2>$null)
  if (-not $commit) { $commit = "unknown" }
}
$ldflags = "-s -w -X github.com/wakecomputer/wakecomputer/internal/version.Version=$Version " +
           "-X github.com/wakecomputer/wakecomputer/internal/version.Commit=$commit " +
           "-X github.com/wakecomputer/wakecomputer/internal/version.BuildTime=$buildTime"

New-Item -ItemType Directory -Force -Path dist | Out-Null

$targets = @(
  @{ os = "linux";   arch = "amd64"; pkg = "./cmd/server"; out = "dist/wakecomputer-linux-amd64" },
  @{ os = "linux";   arch = "amd64"; pkg = "./cmd/agent";  out = "dist/wake-agent-linux-amd64" },
  @{ os = "linux";   arch = "arm64"; pkg = "./cmd/agent";  out = "dist/wake-agent-linux-arm64" },
  @{ os = "windows"; arch = "amd64"; pkg = "./cmd/agent";  out = "dist/wake-agent-windows-amd64.exe" },
  @{ os = "windows"; arch = "386";   pkg = "./cmd/agent";  out = "dist/wake-agent-windows-386.exe" }
)

foreach ($t in $targets) {
  Write-Host "== $($t.os)/$($t.arch) $($t.pkg) =="
  $env:GOOS = $t.os
  $env:GOARCH = $t.arch
  go build -trimpath -ldflags "$ldflags" -o $t.out $t.pkg
  if ($LASTEXITCODE -ne 0) { throw "build failed: $($t.pkg) $($t.os)/$($t.arch)" }
}

Get-ChildItem dist | Select-Object Name, Length
