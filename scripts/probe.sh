#!/usr/bin/env bash
HOST="${1:?usage: probe.sh <host> [ports...]}"
shift || true
PORTS="${*:-22 80 443 16601 8085 5432 6379}"
for p in $PORTS; do
  if timeout 2 bash -c "exec 3<>/dev/tcp/$HOST/$p" 2>/dev/null; then
    echo "port $p OPEN"
  else
    echo "port $p closed"
  fi
done
echo "--- ssh banner ---"
timeout 3 bash -c "exec 3<>/dev/tcp/$HOST/22; head -c 100 <&3" 2>/dev/null || true
echo
echo "--- http 80 ---"
curl -s -m 4 -o /dev/null -w "http80=%{http_code} server=%{scheme}\n" "http://$HOST/" || true
curl -s -m 4 "http://$HOST/" | head -c 300 || true
echo
echo "--- https 443 ---"
curl -sk -m 4 -o /dev/null -w "https443=%{http_code}\n" "https://$HOST/" || true
echo "--- lucky 16601 ---"
curl -s -m 4 -o /dev/null -w "lucky=%{http_code}\n" "http://$HOST:16601/" || true
