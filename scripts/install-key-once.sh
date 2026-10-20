#!/usr/bin/env bash
set -euo pipefail
PUB="${OWRT_PUBKEY_FILE:-$HOME/.ssh/wakecomputer_ed25519.pub}"
[ -f "$PUB" ] || { echo "missing public key: $PUB" >&2; exit 2; }
KEY=$(tr -d '\r' < "$PUB")
MARKER=$(printf '%s\n' "$KEY" | awk '{print $2}')
CMD="mkdir -p /etc/dropbear /root/.ssh && grep -q '$MARKER' /etc/dropbear/authorized_keys 2>/dev/null || printf '%s\n' '$KEY' >> /etc/dropbear/authorized_keys && grep -q '$MARKER' /root/.ssh/authorized_keys 2>/dev/null || printf '%s\n' '$KEY' >> /root/.ssh/authorized_keys && chmod 700 /root/.ssh && chmod 600 /etc/dropbear/authorized_keys /root/.ssh/authorized_keys && grep -c '$MARKER' /etc/dropbear/authorized_keys"

: "${OWRT_PASS:?export OWRT_PASS first}"
: "${OWRT_HOST:?export OWRT_HOST first}"
HOST="$OWRT_HOST"
sshpass -p "$OWRT_PASS" ssh \
  -o StrictHostKeyChecking=no \
  -o UserKnownHostsFile=/dev/null \
  -o ConnectTimeout=8 \
  -o NumberOfPasswordPrompts=1 \
  -o PreferredAuthentications=password \
  -o PubkeyAuthentication=no \
  "root@$HOST" "$CMD"
