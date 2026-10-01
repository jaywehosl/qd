#!/usr/bin/env bash
set -Eeuo pipefail

die() { echo "release: $*" >&2; exit 1; }

V="${1:-}"
[[ "$V" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[a-z0-9.]+)?$ ]] || die "usage: QD_KEYSTORE_PASS=... QD_UPDATE_KEY=... $0 vX.Y.Z-alpha"
[ -n "${QD_KEYSTORE_PASS:-}" ] || die "QD_KEYSTORE_PASS is not set"
[ -n "${QD_UPDATE_KEY:-}" ] || die "QD_UPDATE_KEY is not set"

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT="$ROOT/dist/$V"
NAME="${V#v}"
NAME="${NAME%%-*}"
STAMP="-X github.com/jaywehosl/qd/internal/update.Version=$V"
GRADLE_FILE="$ROOT/android/app/build.gradle.kts"

cd "$ROOT"
export GOFLAGS=-buildvcs=false
export PATH="$PATH:$(go env GOPATH)/bin"
export ANDROID_HOME="${ANDROID_HOME:-${LOCALAPPDATA:-$HOME/AppData/Local}/Android/Sdk}"
[ -d "$ANDROID_HOME" ] || die "no Android SDK at $ANDROID_HOME, set ANDROID_HOME"
export ANDROID_NDK_HOME="${ANDROID_NDK_HOME:-$(ls -d "$ANDROID_HOME"/ndk/* | head -1)}"
GRADLE="${GRADLE:-$(ls -d "$HOME"/.gradle/wrapper/dists/gradle-*/*/gradle-*/bin/gradle 2>/dev/null | head -1)}"
[ -x "$GRADLE" ] || die "no gradle found, set GRADLE"
APKSIGNER="$(ls -d "$ANDROID_HOME"/build-tools/*/apksigner* | sort | tail -1)"

held="$(grep -oE 'versionName = "[^"]+"' "$GRADLE_FILE" | grep -oE '[0-9.]+')"
if [ "$held" != "$NAME" ]; then
    code="$(grep -oE 'versionCode = [0-9]+' "$GRADLE_FILE" | grep -oE '[0-9]+')"
    sed -i -e "s/versionCode = $code/versionCode = $((code + 1))/" -e "s/versionName = \"$held\"/versionName = \"$NAME\"/" "$GRADLE_FILE"
    echo "release: android $held ($code) -> $NAME ($((code + 1))), commit build.gradle.kts with the release"
fi

rm -rf "$OUT"
mkdir -p "$OUT"

(cd panel && npm run build >/dev/null)

GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w -X main.version=$V" -o "$OUT/qd-node-linux-amd64" ./cmd/qd-node
GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-H windowsgui -s -w $STAMP" -o "$OUT/qd-client-windows-amd64.exe" ./cmd/qd-tun
GOOS=windows GOARCH=amd64 go build -trimpath -tags core -ldflags "-s -w $STAMP" -o "$OUT/qd-core-windows-amd64.exe" ./cmd/qd-tun

gomobile bind -target=android/arm64 -androidapi 26 -ldflags "$STAMP" -o android/app/libs/qdmobile.aar ./mobile
(cd android && "$GRADLE" clean assembleRelease -q; "$GRADLE" --stop -q)
cp android/app/build/outputs/apk/release/app-release.apk "$OUT/qd-android-arm64.apk"

FILES=(qd-node-linux-amd64 qd-client-windows-amd64.exe qd-core-windows-amd64.exe qd-android-arm64.apk)
if [ -n "${QD_LINUX:-}" ]; then
    QD_VERSION="$V" bash packaging/linux/build.sh "$OUT"
    FILES+=(qd-client-linux-amd64.tar.gz)
fi

for f in qd-node-linux-amd64 qd-client-windows-amd64.exe qd-core-windows-amd64.exe; do
    grep -qa "$V" "$OUT/$f" || die "$f does not carry $V"
done
[ "$(unzip -p "$OUT/qd-android-arm64.apk" lib/arm64-v8a/libgojni.so | grep -ca "$V")" -gt 0 ] || die "the apk does not carry $V"
[ "$("$APKSIGNER" verify --print-certs "$OUT/qd-android-arm64.apk" | grep -c "CN=qd")" -gt 0 ] || die "the apk is not signed as CN=qd"

chunk="$(ls web/dist/assets | grep -m1 -oE 'NodesSection-[A-Za-z0-9_-]+\.js')"
grep -qa "$chunk" "$OUT/qd-client-windows-amd64.exe" || die "the windows client holds a stale panel"

(cd "$OUT" && sha256sum -b "${FILES[@]}" > checksums.txt && echo "$V *version" >> checksums.txt)
go run ./cmd/qd-sign "$OUT/checksums.txt" >/dev/null

echo
echo "release: $V is in $OUT"
ls -la "$OUT" | awk 'NR>3 {printf "  %10d  %s\n", $5, $9}'
cat "$OUT/checksums.txt" | sed 's/^/  /'
echo "release: next — commit, anonymised push, gh release create with these files, deploy the node"
