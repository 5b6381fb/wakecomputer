#!/bin/sh
# WakeComputer installer for OpenWrt / iStoreOS (x86_64).
# Run from the unpacked bundle directory:
#   sh openwrt-install.sh
# or point at a prebuilt image tarball:
#   sh openwrt-install.sh /tmp/wakecomputer-0.1.0.tar
set -eu

INSTALL_DIR="${INSTALL_DIR:-/opt/wakecomputer}"
IMAGE_TAR="${1:-}"

say() { printf '%s\n' "$*"; }
fail() { printf 'ERROR: %s\n' "$*" >&2; exit 1; }

say "== preflight =="
ARCH=$(uname -m)
say "arch: $ARCH"
[ "$ARCH" = "x86_64" ] || say "warning: expected x86_64, continuing anyway"

command -v docker >/dev/null 2>&1 || fail "docker is not installed"
docker info >/dev/null 2>&1 || fail "docker daemon is not running"

if docker compose version >/dev/null 2>&1; then
  COMPOSE="docker compose"
elif command -v docker-compose >/dev/null 2>&1; then
  COMPOSE="docker-compose"
else
  fail "docker compose plugin is not installed"
fi

say "docker: $(docker version --format '{{.Server.Version}}' 2>/dev/null || echo unknown)"
FREE_MB=$(df -m /overlay 2>/dev/null | awk 'NR==2{print $4}')
say "overlay free: ${FREE_MB:-unknown} MB"

say "== snapshot existing containers =="
docker ps --format '{{.Names}}\t{{.Image}}\t{{.Status}}' | tee /tmp/wakecomputer-docker-before.txt

say "== install directory =="
mkdir -p "$INSTALL_DIR/data/downloads"
chown 1000:1000 "$INSTALL_DIR/data" 2>/dev/null || true
chmod 750 "$INSTALL_DIR/data"

if [ "$(pwd)" != "$INSTALL_DIR" ]; then
  for f in docker-compose.yml .env.example Dockerfile.scratch wakecomputer-linux-amd64; do
    if [ -f "./$f" ]; then
      cp "./$f" "$INSTALL_DIR/$f"
    fi
  done
  if [ -d ./data/downloads ]; then
    cp -f ./data/downloads/* "$INSTALL_DIR/data/downloads/" 2>/dev/null || true
  fi
fi

[ -f "$INSTALL_DIR/docker-compose.yml" ] || fail "docker-compose.yml not found"
[ -f "$INSTALL_DIR/.env" ] || cp "$INSTALL_DIR/.env.example" "$INSTALL_DIR/.env"

IMAGE=$(sed -n 's/^WC_IMAGE=//p' "$INSTALL_DIR/.env" | head -1)
IMAGE=${WC_IMAGE:-$IMAGE}
case "$IMAGE" in
  */*)
    say "== pulling $IMAGE =="
    docker pull "$IMAGE"
    PULLED=1
    ;;
  *)
    PULLED=0
    ;;
esac

VERSION=$(sed -n 's/.*image: *wakecomputer:\([0-9A-Za-z._-]*\).*/\1/p' "$INSTALL_DIR/docker-compose.yml" | head -1)
VERSION=${VERSION:-0.1.0}

if [ "$PULLED" = 1 ]; then
  say "using registry image"
elif docker image inspect "wakecomputer:$VERSION" >/dev/null 2>&1; then
  say "image already present"
elif [ -n "$IMAGE_TAR" ] && [ -f "$IMAGE_TAR" ]; then
  say "== loading image =="
  docker load -i "$IMAGE_TAR"
elif [ -f "$INSTALL_DIR/wakecomputer-linux-amd64" ]; then
  say "== building image from static binary (docker import) =="
  ROOTFS="/tmp/wc-rootfs-$$"
  rm -rf "$ROOTFS"
  mkdir -p "$ROOTFS/app"
  cp "$INSTALL_DIR/wakecomputer-linux-amd64" "$ROOTFS/app/wakecomputer"
  chmod 755 "$ROOTFS/app/wakecomputer"
  tar -C "$ROOTFS" -cf /tmp/wc-rootfs.tar .
  docker import \
    -c 'ENTRYPOINT ["/app/wakecomputer"]' \
    -c 'EXPOSE 8085' \
    /tmp/wc-rootfs.tar "wakecomputer:$VERSION"
  rm -rf "$ROOTFS" /tmp/wc-rootfs.tar
else
  fail "no image, no tarball and no static binary found"
fi

say "== starting container =="
cd "$INSTALL_DIR"
$COMPOSE up -d

say "== waiting for health =="
i=0
while [ $i -lt 25 ]; do
  if $COMPOSE exec -T wakecomputer /app/wakecomputer healthcheck >/dev/null 2>&1; then
    say "healthy"
    break
  fi
  i=$((i + 1))
  sleep 1
done

$COMPOSE ps
say ""
say "Recent logs (the first-run admin password is printed here):"
$COMPOSE logs --tail=40 wakecomputer

say ""
say "Next steps:"
say "  1. Lucky: 在现有 443 Web 服务规则下新增 wake 子规则 -> http://192.168.1.3:8085"
say "  2. 在局域网安装 Agent（App 内生成接入码；WC_PUBLIC_URL 必须是内网地址）"
say "     改密码或重启服务端不会解除已有绑定"
say "  3. Android App 登录并添加主机"
