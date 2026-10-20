#!/bin/sh
# Remote bootstrap: read-only reconnaissance + install the deploy public key.
set -eu

KEY="${OWRT_PUBKEY:-${1:-}}"
if [ -z "$KEY" ]; then
  echo "usage: OWRT_PUBKEY='ssh-ed25519 AAAA... comment' sh bootstrap-remote.sh" >&2
  exit 2
fi
MARKER=$(printf '%s\n' "$KEY" | awk '{print $2}')

echo "== system =="
uname -a
[ -f /etc/openwrt_release ] && . /etc/openwrt_release && echo "$DISTRIB_ID $DISTRIB_RELEASE $DISTRIB_ARCH"

echo "== docker =="
docker version --format 'server={{.Server.Version}}' 2>&1 || true
docker compose version 2>&1 || docker-compose version 2>&1 || true

echo "== existing containers (snapshot) =="
docker ps --format '{{.Names}}\t{{.Image}}\t{{.Status}}' 2>&1 || true

echo "== disk =="
df -h 2>/dev/null | grep -E 'Filesystem|/overlay|/mnt|/$' || df -h 2>/dev/null | head -10

echo "== memory =="
free -m 2>/dev/null || true

echo "== listening ports =="
(netstat -lntp 2>/dev/null || ss -lntp 2>/dev/null) | grep -E ':(80|443|8085|16601|45871)\b' || true

echo "== port 8085 check =="
if (netstat -lnt 2>/dev/null || ss -lnt 2>/dev/null) | grep -q ':8085 '; then
  echo "8085 already in use"
else
  echo "8085 free"
fi

echo "== reverse proxy =="
ps w 2>/dev/null | grep -iE 'lucky|nginx|caddy' | grep -v grep | head -5 || true
docker ps --format '{{.Names}}\t{{.Image}}' 2>/dev/null | grep -iE 'lucky|nginx|caddy' || true

echo "== dropbear config =="
uci show dropbear 2>/dev/null || true
ls -la /etc/dropbear 2>/dev/null || true
ls -la /root/.ssh 2>/dev/null || true

echo "== install deploy key =="
mkdir -p /etc/dropbear /root/.ssh
touch /etc/dropbear/authorized_keys /root/.ssh/authorized_keys
if ! grep -q "$MARKER" /etc/dropbear/authorized_keys 2>/dev/null; then
  echo "$KEY" >> /etc/dropbear/authorized_keys
fi
if ! grep -q "$MARKER" /root/.ssh/authorized_keys 2>/dev/null; then
  echo "$KEY" >> /root/.ssh/authorized_keys
fi
chmod 700 /root/.ssh
chmod 600 /root/.ssh/authorized_keys /etc/dropbear/authorized_keys
echo "key installed (dropbear reads authorized_keys per login; no restart needed)"

echo "== target dir =="
ls -la /opt 2>/dev/null | head -10 || true
