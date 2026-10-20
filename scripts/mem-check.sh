#!/usr/bin/env bash
set -euo pipefail
BIN="${1:-/tmp/wc-server}"
rm -rf /tmp/wc-mem; mkdir -p /tmp/wc-mem
WC_DATA_DIR=/tmp/wc-mem WC_HTTP_ADDR=127.0.0.1:18088 \
WC_ADMIN_PASSWORD=testpass123 \
WC_JWT_SECRET=0123456789abcdef0123456789abcdef0123456789abcdef \
"$BIN" serve >/tmp/wc-mem.log 2>&1 &
P=$!
trap 'kill $P 2>/dev/null || true' EXIT
for i in $(seq 1 20); do curl -fsS http://127.0.0.1:18088/healthz >/dev/null 2>&1 && break; sleep 0.3; done
echo -n "baseline:    "; grep VmRSS /proc/$P/status | awk '{print $2, $3}'
curl -s -X POST http://127.0.0.1:18088/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"testpass123"}' >/dev/null
sleep 1
echo -n "after-login: "; grep VmRSS /proc/$P/status | awk '{print $2, $3}'
echo "GOMEMLIMIT=${GOMEMLIMIT:-unset}"
