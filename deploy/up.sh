#!/bin/sh
# Pull the server image and start it. Run on the OpenWrt host:
#   WC_IMAGE=ghcr.io/owner/wakecomputer:0.1.0 sh up.sh
set -eu

say() { printf '%s\n' "$*"; }
fail() { printf 'ERROR: %s\n' "$*" >&2; exit 1; }

INSTALL_DIR="${INSTALL_DIR:-/opt/wakecomputer}"
SRC=$(CDPATH= cd -- "$(dirname "$0")" && pwd)

command -v docker >/dev/null 2>&1 || fail "docker is not installed"
docker compose version >/dev/null 2>&1 || docker-compose version >/dev/null 2>&1 || fail "docker compose is not installed"
if docker compose version >/dev/null 2>&1; then
  COMPOSE="docker compose"
else
  COMPOSE="docker-compose"
fi

mkdir -p "$INSTALL_DIR/data/downloads" "$INSTALL_DIR/data/backups"
cp "$SRC/docker-compose.yml" "$INSTALL_DIR/docker-compose.yml"
if [ ! -f "$INSTALL_DIR/.env" ]; then
  cp "$SRC/.env.example" "$INSTALL_DIR/.env"
  say "created $INSTALL_DIR/.env"
fi

if [ -n "${WC_IMAGE:-}" ]; then
  grep -v '^WC_IMAGE=' "$INSTALL_DIR/.env" > "$INSTALL_DIR/.env.tmp" || true
  printf 'WC_IMAGE=%s\n' "$WC_IMAGE" >> "$INSTALL_DIR/.env.tmp"
  mv "$INSTALL_DIR/.env.tmp" "$INSTALL_DIR/.env"
fi

IMAGE=$(sed -n 's/^WC_IMAGE=//p' "$INSTALL_DIR/.env" | head -1)
[ -n "$IMAGE" ] || fail "set WC_IMAGE in $INSTALL_DIR/.env, for example ghcr.io/owner/wakecomputer:0.1.0"
case "$IMAGE" in
  */*) ;;
  *) fail "WC_IMAGE must be a registry image, got: $IMAGE" ;;
esac

chown 1000:1000 "$INSTALL_DIR/data" 2>/dev/null || true
chmod 750 "$INSTALL_DIR/data"

cd "$INSTALL_DIR"
say "== pull $IMAGE =="
$COMPOSE pull
say "== start =="
$COMPOSE up -d --force-recreate

i=0
while [ "$i" -lt 25 ]; do
  if $COMPOSE exec -T wakecomputer /app/wakecomputer healthcheck >/dev/null 2>&1; then
    say "healthy"
    $COMPOSE ps
    say "If this is the first start, the admin password is in: $COMPOSE logs wakecomputer"
    exit 0
  fi
  i=$((i + 1))
  sleep 1
done

fail "container did not become healthy; see: $COMPOSE logs wakecomputer"
