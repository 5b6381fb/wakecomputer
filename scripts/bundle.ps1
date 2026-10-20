param([string]$Version = "0.1.0")

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

& "$PSScriptRoot\build.ps1" -Version $Version
if ($LASTEXITCODE -ne 0) { throw "build failed" }

$bundle = Join-Path $root "build\bundle"
if (Test-Path $bundle) { Remove-Item -Recurse -Force $bundle }
New-Item -ItemType Directory -Force -Path "$bundle\data\downloads" | Out-Null

Copy-Item deploy\docker-compose.yml $bundle
Copy-Item deploy\.env.example $bundle
Copy-Item deploy\Dockerfile.scratch $bundle
Copy-Item deploy\openwrt-install.sh $bundle
Copy-Item dist\wakecomputer-linux-amd64 $bundle

Copy-Item dist\wake-agent-linux-amd64 "$bundle\data\downloads\"
Copy-Item dist\wake-agent-linux-arm64 "$bundle\data\downloads\"
Copy-Item dist\wake-agent-windows-amd64.exe "$bundle\data\downloads\"
Copy-Item dist\wake-agent-windows-386.exe "$bundle\data\downloads\"

$out = Join-Path $root "dist\wakecomputer-deploy-$Version.tar.gz"
if (Test-Path $out) { Remove-Item $out -Force }
tar -czf $out -C $bundle .
if ($LASTEXITCODE -ne 0) { throw "tar failed" }

Get-Item $out | Select-Object FullName, Length
