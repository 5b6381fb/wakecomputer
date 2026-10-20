#!/usr/bin/env bash
set -euo pipefail

ROOT=/mnt/e/ai/opencode/wakecomputer
SERVER_BIN="$ROOT/dist/wakecomputer-linux-amd64"
AGENT_BIN="$ROOT/dist/wake-agent-linux-amd64"
PORT="${PORT:-18087}"
BASE="http://127.0.0.1:$PORT"
DATA=/tmp/wc-agent-data
CFG=/tmp/wc-agent-test/config.json

cp "$SERVER_BIN" /tmp/wc-server
cp "$AGENT_BIN" /tmp/wc-agent
chmod +x /tmp/wc-server /tmp/wc-agent
rm -rf "$DATA" /tmp/wc-agent-test; mkdir -p "$DATA" /tmp/wc-agent-test

WC_DATA_DIR="$DATA" WC_HTTP_ADDR="127.0.0.1:$PORT" \
WC_ADMIN_PASSWORD="testpass123" \
WC_JWT_SECRET="0123456789abcdef0123456789abcdef0123456789abcdef" \
WC_LOG_LEVEL="debug" \
/tmp/wc-server serve >/tmp/wc-agent-server.log 2>&1 &
SPID=$!
trap 'kill $SPID 2>/dev/null || true; kill ${APID:-0} 2>/dev/null || true' EXIT

for i in $(seq 1 20); do curl -fsS "$BASE/healthz" >/dev/null 2>&1 && break; sleep 0.5; done

TOKEN=$(curl -s -X POST "$BASE/api/v1/auth/login" -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"testpass123"}' | sed -n 's/.*"accessToken":"\([^"]*\)".*/\1/p')
HOST_ID=$(curl -s -X POST "$BASE/api/v1/hosts" \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"name":"wsl-agent","osType":"linux","mac":"AA:BB:CC:DD:EE:04","probePort":45871}' \
  | sed -n 's/.*"id":"\([^"]*\)".*/\1/p')
echo "host=$HOST_ID"

CODE=$(curl -s -X POST "$BASE/api/v1/hosts/$HOST_ID/agent/enroll-code" \
  -H "Authorization: Bearer $TOKEN" | sed -n 's/.*"code":"\([^"]*\)".*/\1/p')
echo "code=$CODE"

/tmp/wc-agent enroll --server "$BASE" --code "$CODE" --config "$CFG"

/tmp/wc-agent run --config "$CFG" >/tmp/wc-agent.log 2>&1 &
APID=$!
sleep 3

echo "--- host status ---"
curl -s "$BASE/api/v1/hosts/$HOST_ID" -H "Authorization: Bearer $TOKEN" | head -c 800; echo

AGENT_TOKEN=$(sed -n 's/.*"token": *"\([^"]*\)".*/\1/p' "$CFG")
echo "--- agent health ---"
curl -s -k -H "Authorization: Bearer $AGENT_TOKEN" https://127.0.0.1:45871/v1/health; echo

echo "--- cancel command (harmless) ---"
curl -s -X POST "$BASE/api/v1/hosts/$HOST_ID/power" \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"action":"cancel"}' ; echo
sleep 12

echo "--- memory (VmRSS) ---"
grep VmRSS /proc/$SPID/status /proc/$APID/status 2>/dev/null || true

echo "--- agent log ---"
cat /tmp/wc-agent.log

kill $APID 2>/dev/null || true
echo OK
