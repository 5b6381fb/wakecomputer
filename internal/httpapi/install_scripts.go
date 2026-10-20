package httpapi

const installSh = `#!/bin/sh
set -eu

SERVER=""
CODE=""
while [ $# -gt 0 ]; do
  case "$1" in
    --server) SERVER="$2"; shift 2 ;;
    --code)   CODE="$2";   shift 2 ;;
    *) echo "unknown argument: $1" >&2; exit 1 ;;
  esac
done
if [ -z "$SERVER" ] || [ -z "$CODE" ]; then
  echo "usage: install.sh --server https://wake.example.com --code ABCD1234" >&2
  exit 1
fi

case "$(uname -m)" in
  x86_64|amd64) ARCH=amd64 ;;
  aarch64|arm64) ARCH=arm64 ;;
  *) echo "unsupported architecture: $(uname -m)" >&2; exit 1 ;;
esac

BIN=/usr/local/bin/wake-agent
echo "downloading wake-agent for linux/$ARCH ..."
curl -fsSL "$SERVER/download/wake-agent-linux-$ARCH" -o "$BIN"
chmod 755 "$BIN"

"$BIN" enroll --server "$SERVER" --code "$CODE"

cat >/etc/systemd/system/wake-agent.service <<'UNIT'
[Unit]
Description=WakeComputer Agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/wake-agent run
Restart=always
RestartSec=5
User=root

[Install]
WantedBy=multi-user.target
UNIT

systemctl daemon-reload
systemctl enable --now wake-agent
sleep 1
systemctl --no-pager --lines=8 status wake-agent || true
echo "WakeComputer Agent installed and running."
`

const installPs1 = `param(
  [Parameter(Mandatory=$true)][string]$Server,
  [Parameter(Mandatory=$true)][string]$Code
)
$ErrorActionPreference = "Stop"

$arch = if ([Environment]::Is64BitOperatingSystem) { "amd64" } else { "386" }
$dir = Join-Path $env:ProgramFiles "WakeComputer"
New-Item -ItemType Directory -Force -Path $dir | Out-Null
$exe = Join-Path $dir "wake-agent.exe"

Write-Host "downloading wake-agent for windows/$arch ..."
Invoke-WebRequest -UseBasicParsing -Uri "$Server/download/wake-agent-windows-$arch.exe" -OutFile $exe

& $exe enroll --server $Server --code $Code
if ($LASTEXITCODE -ne 0) { throw "agent enrollment failed" }

& $exe install
if ($LASTEXITCODE -ne 0) { throw "service installation failed" }

Start-Service -Name WakeAgent -ErrorAction Stop

# Allow the agent port from the local subnet only.
New-NetFirewallRule -DisplayName "WakeComputer Agent" -Direction Inbound -Action Allow -Protocol TCP -LocalPort 45871 -RemoteAddress LocalSubnet -ErrorAction SilentlyContinue | Out-Null

Get-Service -Name WakeAgent | Format-Table -AutoSize
Write-Host "WakeComputer Agent installed and running."
`
