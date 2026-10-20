#!/usr/bin/env bash
# Run a command on the OpenWrt host from WSL.
# Prefers the project SSH key; falls back to the password if no key is installed.
: "${OWRT_HOST:?export OWRT_HOST first}"
KEY="${OWRT_KEY:-$HOME/.ssh/wakecomputer_ed25519}"
OPTS=(-o StrictHostKeyChecking=accept-new -o UserKnownHostsFile=/dev/null -o ConnectTimeout=10)

if [ -f "$KEY" ]; then
  exec ssh -i "$KEY" -o IdentitiesOnly=yes "${OPTS[@]}" "root@$OWRT_HOST" "$@"
fi

: "${OWRT_PASS:?install the SSH key or export OWRT_PASS=<router password>}"
exec sshpass -p "$OWRT_PASS" ssh "${OPTS[@]}" \
  -o PreferredAuthentications=password,keyboard-interactive \
  -o PubkeyAuthentication=no \
  "root@$OWRT_HOST" "$@"
