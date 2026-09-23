#!/usr/bin/env bash
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
OUT="${1:-$ROOT/dist}"
STAGE="$(mktemp -d)"
trap 'rm -rf "$STAGE"' EXIT

cd "$ROOT"
export GOFLAGS=-buildvcs=false
mkdir -p "$STAGE/qd-client" "$OUT"

CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o "$STAGE/qd-client/qd-client" ./cmd/qd-tun
CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o "$STAGE/qd-client/qd-client-window" ./cmd/qd-window
cp packaging/linux/install.sh packaging/linux/qd-client.service packaging/linux/qd-client.desktop packaging/linux/qd-client.png "$STAGE/qd-client/"
chmod 0755 "$STAGE/qd-client/install.sh"

tar -C "$STAGE" -czf "$OUT/qd-client-linux-amd64.tar.gz" --owner=0 --group=0 qd-client
ls -la "$OUT/qd-client-linux-amd64.tar.gz"
