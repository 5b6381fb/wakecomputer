#!/usr/bin/env bash
# Build the linux/amd64 binaries and package them into a Docker image tarball.
set -eu
cd "$(dirname "$0")/.."

VERSION="${VERSION:-0.1.0}"
bash scripts/build.sh

docker build -f deploy/Dockerfile.runtime -t "wakecomputer:${VERSION}" .
docker save "wakecomputer:${VERSION}" -o "dist/wakecomputer-${VERSION}.tar"

echo "image: dist/wakecomputer-${VERSION}.tar"
ls -lh "dist/wakecomputer-${VERSION}.tar"
