#!/usr/bin/env bash
# Build server and agent binaries for deployment (run inside WSL/Linux).
set -eu
cd "$(dirname "$0")/.."

VERSION="${VERSION:-0.1.0}"
COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo unknown)"
BUILD_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
LDFLAGS="-s -w \
  -X github.com/wakecomputer/wakecomputer/internal/version.Version=${VERSION} \
  -X github.com/wakecomputer/wakecomputer/internal/version.Commit=${COMMIT} \
  -X github.com/wakecomputer/wakecomputer/internal/version.BuildTime=${BUILD_TIME}"

mkdir -p dist
export CGO_ENABLED=0

echo "== server linux/amd64 =="
GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "$LDFLAGS" -o dist/wakecomputer-linux-amd64 ./cmd/server

echo "== agent linux/amd64 =="
GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "$LDFLAGS" -o dist/wake-agent-linux-amd64 ./cmd/agent

echo "== agent linux/arm64 =="
GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "$LDFLAGS" -o dist/wake-agent-linux-arm64 ./cmd/agent

echo "== agent windows/amd64 =="
GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "$LDFLAGS" -o dist/wake-agent-windows-amd64.exe ./cmd/agent

echo "== agent windows/386 =="
GOOS=windows GOARCH=386 go build -trimpath -ldflags "$LDFLAGS" -o dist/wake-agent-windows-386.exe ./cmd/agent

echo "done:"
ls -lh dist
