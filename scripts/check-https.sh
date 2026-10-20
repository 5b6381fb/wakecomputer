#!/bin/sh
# Usage: check-https.sh <hostname> <ipv6> [port]
HOST="${1:?usage: check-https.sh wake.example.com 2001:db8::1 [8443]}"
IP6="${2:?usage: check-https.sh wake.example.com 2001:db8::1 [8443]}"
PORT="${3:-8443}"
echo "== br-lan ipv6 =="
ip -6 addr show br-lan 2>/dev/null | grep inet6
echo "== listen =="
netstat -lntp 2>/dev/null | grep "$PORT"
echo "== local public https =="
curl -g -6 -k -s --max-time 12 --resolve "$HOST:$PORT:[$IP6]" -w "\nHTTP=%{http_code}\n" "https://$HOST:$PORT/healthz" 2>&1
echo "== firewall refs =="
nft list ruleset 2>/dev/null | grep -i -E "$PORT|wan" | head -10
uci show firewall 2>/dev/null | grep -i -E "$PORT|wan" | head -10
echo "== ip6 forwarding =="
sysctl net.ipv6.conf.all.forwarding 2>/dev/null
