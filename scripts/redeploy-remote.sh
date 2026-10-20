#!/bin/sh
set -eu
cd /opt/wakecomputer

echo "== rebuild image =="
ROOTFS=/tmp/wc-rootfs-update
rm -rf "$ROOTFS"
mkdir -p "$ROOTFS/app"
cp wakecomputer-linux-amd64 "$ROOTFS/app/wakecomputer"
chmod 755 "$ROOTFS/app/wakecomputer"
tar -C "$ROOTFS" -cf /tmp/wc-rootfs-update.tar .
docker import \
  -c 'ENTRYPOINT ["/app/wakecomputer"]' \
  -c 'EXPOSE 8085' \
  /tmp/wc-rootfs-update.tar wakecomputer:0.1.0
rm -rf "$ROOTFS" /tmp/wc-rootfs-update.tar

echo "== recreate =="
docker compose up -d --force-recreate wakecomputer

echo "== wait =="
i=0
while [ $i -lt 20 ]; do
  if docker compose exec -T wakecomputer /app/wakecomputer healthcheck >/dev/null 2>&1; then
    echo healthy
    break
  fi
  i=$((i + 1))
  sleep 1
done

echo "== root =="
wget -qO- http://127.0.0.1:8085/ || curl -s http://127.0.0.1:8085/
echo
echo "== health =="
wget -qO- http://127.0.0.1:8085/healthz || curl -s http://127.0.0.1:8085/healthz
echo
docker ps --filter name=wakecomputer --format 'table {{.Names}}\t{{.Status}}'
