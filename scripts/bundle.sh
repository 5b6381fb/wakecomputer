#!/usr/bin/env bash
# Build all binaries and assemble a deployment bundle for OpenWrt.
set -eu
cd "$(dirname "$0")/.."

VERSION="${VERSION:-0.1.0}"
bash scripts/build.sh

BUNDLE="build/bundle"
rm -rf "$BUNDLE"
mkdir -p "$BUNDLE/data/downloads"

cp deploy/docker-compose.yml "$BUNDLE/"
cp deploy/.env.example "$BUNDLE/"
cp deploy/Dockerfile.scratch "$BUNDLE/"
cp deploy/openwrt-install.sh "$BUNDLE/"
cp dist/wakecomputer-linux-amd64 "$BUNDLE/"

cp dist/wake-agent-linux-amd64 "$BUNDLE/data/downloads/"
cp dist/wake-agent-linux-arm64 "$BUNDLE/data/downloads/"
cp dist/wake-agent-windows-amd64.exe "$BUNDLE/data/downloads/"
cp dist/wake-agent-windows-386.exe "$BUNDLE/data/downloads/"

tar czf "dist/wakecomputer-deploy-${VERSION}.tar.gz" -C "$BUNDLE" .

echo "bundle: dist/wakecomputer-deploy-${VERSION}.tar.gz"
ls -lh "dist/wakecomputer-deploy-${VERSION}.tar.gz"
