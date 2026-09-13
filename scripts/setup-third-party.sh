#!/bin/sh
set -e

VER="v0.60.0"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
MOD="$(go env GOMODCACHE)/github.com/quic-go/quic-go@${VER}"

if [ ! -d "$MOD" ]; then
	echo "качаю quic-go ${VER}..."
	(cd "$ROOT" && go mod download github.com/quic-go/quic-go)
fi
[ -d "$MOD" ] || { echo "нет модуля: $MOD" >&2; exit 1; }

rm -rf "$ROOT/third_party/quic-go"
mkdir -p "$ROOT/third_party"
cp -r "$MOD" "$ROOT/third_party/quic-go"
chmod -R u+w "$ROOT/third_party/quic-go"

patch -p1 -d "$ROOT/third_party/quic-go" < "$ROOT/patches/quic-go.patch"
echo "quic-go ${VER} пропатчен → third_party/quic-go (go.mod replace ссылается сюда)"

CIP="v0.1.0"
CIPMOD="$(go env GOMODCACHE)/github.com/quic-go/connect-ip-go@${CIP}"

if [ ! -d "$CIPMOD" ]; then
	echo "качаю connect-ip-go ${CIP}..."
	(cd "$ROOT" && go mod download github.com/quic-go/connect-ip-go)
fi
[ -d "$CIPMOD" ] || { echo "нет модуля: $CIPMOD" >&2; exit 1; }

rm -rf "$ROOT/third_party/connect-ip-go"
cp -r "$CIPMOD" "$ROOT/third_party/connect-ip-go"
chmod -R u+w "$ROOT/third_party/connect-ip-go"
rm -f "$ROOT/third_party/connect-ip-go/go.work" "$ROOT/third_party/connect-ip-go/go.work.sum"

patch -p1 -d "$ROOT/third_party/connect-ip-go" < "$ROOT/patches/connect-ip-go.patch"
echo "connect-ip-go ${CIP} пропатчен → third_party/connect-ip-go"
