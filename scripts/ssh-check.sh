#!/usr/bin/env bash
: "${OWRT_HOST:?export OWRT_HOST first}"
HOST="$OWRT_HOST"
: "${OWRT_PASS:?export OWRT_PASS first}"
PASS="$OWRT_PASS"

ping -c 2 -W 2 "$HOST" || true

echo "--- ssh ---"
timeout 15 sshpass -p "$PASS" ssh \
  -o StrictHostKeyChecking=no \
  -o UserKnownHostsFile=/dev/null \
  -o ConnectTimeout=5 \
  -o NumberOfPasswordPrompts=1 \
  -o PreferredAuthentications=password \
  -o PubkeyAuthentication=no \
  "root@$HOST" 'echo PING; uptime; echo "--- load ---"; cat /proc/loadavg' 2>&1
echo "exit=$?"
