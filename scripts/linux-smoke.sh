#!/usr/bin/env bash
set -euo pipefail

ROOT=/mnt/e/ai/opencode/wakecomputer
BIN="${1:-$ROOT/dist/wakecomputer-linux-amd64}"
PORT="${PORT:-18086}"
BASE="http://127.0.0.1:$PORT"
DATA=/tmp/wc-linux-data
LOG=/tmp/wc-linux-server.log

cp "$BIN" /tmp/wc-server
chmod +x /tmp/wc-server
rm -rf "$DATA"; mkdir -p "$DATA"

WC_DATA_DIR="$DATA" \
WC_HTTP_ADDR="127.0.0.1:$PORT" \
WC_ADMIN_PASSWORD="testpass123" \
WC_JWT_SECRET="0123456789abcdef0123456789abcdef0123456789abcdef" \
WC_WOL_DEFAULT_BROADCAST="127.0.0.1" \
WC_LOG_LEVEL="info" \
/tmp/wc-server serve >"$LOG" 2>&1 &
PID=$!
trap 'kill $PID 2>/dev/null || true' EXIT

for i in $(seq 1 20); do
  if curl -fsS "$BASE/healthz" >/dev/null 2>&1; then break; fi
  sleep 0.5
done

echo "--- health ---"
curl -s "$BASE/healthz"; echo

LOGIN=$(curl -s -X POST "$BASE/api/v1/auth/login" -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"testpass123"}')
TOKEN=$(printf '%s' "$LOGIN" | sed -n 's/.*"accessToken":"\([^"]*\)".*/\1/p')
[ -n "$TOKEN" ] || { echo "login failed: $LOGIN"; exit 1; }
echo "login ok (${#TOKEN} chars)"

echo "--- create host ---"
CREATED=$(curl -s -X POST "$BASE/api/v1/hosts" \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"name":"wsl-test","osType":"linux","mac":"AA:BB:CC:DD:EE:03","ip":"127.0.0.1","broadcastTargets":["127.0.0.1:9"],"probePort":135}')
HOST_ID=$(printf '%s' "$CREATED" | sed -n 's/.*"id":"\([^"]*\)".*/\1/p')
[ -n "$HOST_ID" ] || { echo "create failed: $CREATED"; exit 1; }
echo "host=$HOST_ID"

echo "--- wake ---"
curl -s -X POST "$BASE/api/v1/hosts/$HOST_ID/wake" -H "Authorization: Bearer $TOKEN"; echo

echo "--- batch ---"
BATCH=$(curl -s -X POST "$BASE/api/v1/batch" \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d "{\"hostIds\":[\"$HOST_ID\"],\"action\":\"wake\"}")
JOB_ID=$(printf '%s' "$BATCH" | sed -n 's/.*"jobId":"\([^"]*\)".*/\1/p')
echo "job=$JOB_ID"
for i in $(seq 1 20); do
  JOB=$(curl -s "$BASE/api/v1/jobs/$JOB_ID" -H "Authorization: Bearer $TOKEN")
  STATUS=$(printf '%s' "$JOB" | sed -n 's/.*"status":"\([^"]*\)".*/\1/p')
  [ "$STATUS" != "running" ] && break
  sleep 0.3
done
curl -s "$BASE/api/v1/jobs/$JOB_ID" -H "Authorization: Bearer $TOKEN"; echo

echo "--- audit ---"
curl -s "$BASE/api/v1/audit?limit=5" -H "Authorization: Bearer $TOKEN" | head -c 600; echo

echo "--- server log tail ---"
tail -5 "$LOG"
echo "OK"
