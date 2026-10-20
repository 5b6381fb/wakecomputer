#!/usr/bin/env bash
# Verify LuCI credentials without changing anything.
HOST="${1:-${OWRT_HOST:-}}"
if [ -z "$HOST" ]; then
  echo "usage: luci-auth.sh <host> [user] [password]" >&2
  exit 2
fi
USER="${2:-root}"
PASS="${3:-${OWRT_PASS:-}}"
if [ -z "$PASS" ]; then
  echo "usage: luci-auth.sh [host] [user] [password]" >&2
  echo "or export OWRT_PASS" >&2
  exit 2
fi
JAR=$(mktemp)
PAGE=$(mktemp)
BODY=$(mktemp)

curl -s -m 8 -c "$JAR" "http://$HOST/cgi-bin/luci/" -o "$PAGE"
echo "login_page_bytes=$(wc -c < "$PAGE")"
TOKEN=$(grep -oE 'name="csrf_token"[^>]*value="[^"]+"' "$PAGE" | sed -E 's/.*value="([^"]+)".*/\1/' | head -1)

CODE=$(curl -s -m 8 -b "$JAR" -c "$JAR" -o "$BODY" -w '%{http_code}' \
  -H "Referer: http://$HOST/cgi-bin/luci/" \
  -H "Origin: http://$HOST" \
  -H "User-Agent: Mozilla/5.0 (X11; Linux x86_64)" \
  --data-urlencode "luci_username=$USER" \
  --data-urlencode "luci_password=$PASS" \
  ${TOKEN:+--data-urlencode "csrf_token=$TOKEN"} \
  "http://$HOST/cgi-bin/luci/")

echo "post_status=$CODE"
echo "response_bytes=$(wc -c < "$BODY")"
if grep -qi 'sysauth' "$JAR"; then
  echo "RESULT: LOGIN_OK (sysauth cookie set)"
else
  echo "RESULT: NO_SESSION"
fi
echo "--- indicators ---"
grep -ioE 'invalid[^<"]{0,60}|incorrect[^<"]{0,60}|密码[^<"]{0,40}|错误[^<"]{0,40}|失败[^<"]{0,40}' "$BODY" | head -10 || true
echo "--- title ---"
grep -oE '<title>[^<]*' "$BODY" | head -2
rm -f "$JAR" "$PAGE" "$BODY"
