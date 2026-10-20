#!/usr/bin/env bash
# Deploy WakeComputer to the OpenWrt host.
# Requires either the project SSH key or OWRT_PASS.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
VERSION="${VERSION:-0.1.0}"
: "${OWRT_HOST:?export OWRT_HOST first}"
HOST="$OWRT_HOST"
USER="${OWRT_USER:-root}"
BUNDLE="$ROOT/dist/wakecomputer-deploy-$VERSION.tar.gz"

[ -f "$BUNDLE" ] || { echo "missing $BUNDLE (run scripts/bundle.ps1 first)"; exit 1; }

ssh_run() { bash "$ROOT/scripts/owrt.sh" "$@"; }

echo "== preflight =="
ssh_run 'uname -m; docker version --format "{{.Server.Version}}"; docker compose version'

echo "== snapshot =="
ssh_run 'docker ps --format "{{.Names}}\t{{.Image}}\t{{.Status}}"'

echo "== upload =="
cat "$BUNDLE" | ssh_run "cat > /tmp/wakecomputer-deploy-$VERSION.tar.gz"

echo "== install =="
ssh_run "set -e
  mkdir -p /opt/wakecomputer
  tar xzf /tmp/wakecomputer-deploy-$VERSION.tar.gz -C /opt/wakecomputer
  cd /opt/wakecomputer
  sh openwrt-install.sh
"

echo "== verify =="
ssh_run 'cd /opt/wakecomputer && docker compose ps && curl -fsS http://127.0.0.1:8085/healthz || true'
