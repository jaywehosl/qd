#!/usr/bin/env bash
set -Eeuo pipefail

REPO="jaywehosl/qd"
PREFIX="/opt/qd-client"
ASSET="qd-client-linux-amd64.tar.gz"

WANT_VERSION=""
LOCAL_DIR=""
REMOVE=0

STAGE=""
trap 'rc=$?; [ -n "$STAGE" ] && rm -rf "$STAGE"; exit $rc' EXIT

if [ -t 2 ] && [ -z "${NO_COLOR:-}" ]; then
    BOLD=$'\033[1m'; DIM=$'\033[2m'; RED=$'\033[31m'; GRN=$'\033[32m'
    YEL=$'\033[33m'; CYA=$'\033[36m'; OFF=$'\033[0m'
else
    BOLD=""; DIM=""; RED=""; GRN=""; YEL=""; CYA=""; OFF=""
fi

say()  { printf "    %s\n" "$*" >&2; }
step() { printf "\n  ${CYA}${BOLD}>${OFF} ${BOLD}%s${OFF}\n" "$*" >&2; }
good() { printf "    ${GRN}ok${OFF}  %s\n" "$*" >&2; }
warn() { printf "    ${YEL}!${OFF}   %s\n" "$*" >&2; }
die()  { printf "\n  ${RED}x${OFF}   %s\n\n" "$*" >&2; exit 1; }

usage() {
    cat <<'USAGE'
qd client installer for Linux.

  curl -fsSL https://raw.githubusercontent.com/jaywehosl/qd/main/install-client.sh | sudo bash

  --version vX.Y.Z   release to install, default: latest
  --from DIR         install from qd-client-linux-amd64.tar.gz already in DIR
  --remove           remove the client, keep its subscription in /var/lib/qd-client
  --help

Pass arguments through the pipe with: ... | sudo bash -s -- --remove
USAGE
}

while [ $# -gt 0 ]; do
    case "$1" in
        --version) [ $# -ge 2 ] || die "$1 needs a value"; WANT_VERSION="$2"; shift 2 ;;
        --from)    [ $# -ge 2 ] || die "$1 needs a value"; LOCAL_DIR="$2"; shift 2 ;;
        --remove|--uninstall) REMOVE=1; shift ;;
        --help|-h) usage; exit 0 ;;
        *)         die "unknown argument: $1" ;;
    esac
done

printf "\n  ${BOLD}qd${OFF} ${DIM}client installer${OFF}\n" >&2

[ "$(id -u)" -eq 0 ] || die "run this as root: curl -fsSL ... | sudo bash"

if [ "$REMOVE" -eq 1 ]; then
    [ -x "$PREFIX/install.sh" ] || die "the qd client is not installed here"
    exec "$PREFIX/install.sh" --remove
fi

command -v systemctl >/dev/null 2>&1 || die "no systemd here, the client runs as a systemd service"
[ "$(uname -m)" = "x86_64" ] || die "the release carries linux/amd64 only, this is $(uname -m)"

has_webkit() { ldconfig -p 2>/dev/null | grep -F "libwebkit2gtk-4.1.so.0" >/dev/null; }

install_packages() {
    local want=() apt=() dnf=() pac=() zyp=()
    command -v curl >/dev/null 2>&1 || { apt+=(curl ca-certificates); dnf+=(curl); pac+=(curl); zyp+=(curl); }
    command -v ip >/dev/null 2>&1   || { apt+=(iproute2); dnf+=(iproute); pac+=(iproute2); zyp+=(iproute2); }
    command -v nft >/dev/null 2>&1  || { apt+=(nftables); dnf+=(nftables); pac+=(nftables); zyp+=(nftables); }
    has_webkit                      || { apt+=(libwebkit2gtk-4.1-0); dnf+=(webkit2gtk4.1); pac+=(webkit2gtk-4.1); zyp+=(libwebkit2gtk-4_1-0); }

    if command -v apt-get >/dev/null 2>&1; then
        want=("${apt[@]}")
    elif command -v dnf >/dev/null 2>&1; then
        want=("${dnf[@]}")
    elif command -v pacman >/dev/null 2>&1; then
        want=("${pac[@]}")
    elif command -v zypper >/dev/null 2>&1; then
        want=("${zyp[@]}")
    fi

    step "Checking what the client needs"
    if [ ${#apt[@]} -eq 0 ]; then
        good "iproute2, nftables and WebKitGTK 4.1 are in place"
        return 0
    fi
    [ ${#want[@]} -gt 0 ] || die "no apt, dnf, pacman or zypper here; install curl, iproute2, nftables and WebKitGTK 4.1 by hand"

    say "installing ${want[*]}"
    local log
    log="$(mktemp)"
    if command -v apt-get >/dev/null 2>&1; then
        apt-get update -qq >"$log" 2>&1 || true
        DEBIAN_FRONTEND=noninteractive apt-get install -y -qq "${want[@]}" >>"$log" 2>&1
    elif command -v dnf >/dev/null 2>&1; then
        dnf install -y -q "${want[@]}" >"$log" 2>&1
    elif command -v pacman >/dev/null 2>&1; then
        say "Arch takes no partial upgrades, so the system is brought up to date first (pacman -Syu)"
        pacman -Syu --needed --noconfirm "${want[@]}" >"$log" 2>&1
    else
        zypper --non-interactive install "${want[@]}" >"$log" 2>&1
    fi || { tail -n 6 "$log" | sed "s/^/      /" >&2; rm -f "$log"; die "could not install ${want[*]}"; }
    rm -f "$log"
    good "installed ${want[*]}"
}

resolve_version() {
    if [ -n "$WANT_VERSION" ]; then printf "%s" "$WANT_VERSION"; return 0; fi

    local tag
    tag="$(curl -fsSLI -o /dev/null -w '%{url_effective}' \
        "https://github.com/$REPO/releases/latest" 2>/dev/null | sed 's#.*/tag/##')"
    if [ -z "$tag" ] || [ "${tag#v}" = "$tag" ]; then
        tag="$(curl -fsSL "https://api.github.com/repos/$REPO/releases?per_page=1" 2>/dev/null \
            | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -1)"
        [ -n "$tag" ] && say "latest release is a pre-release: $tag"
    fi
    [ -n "$tag" ] || die "cannot work out the latest release of $REPO, pass --version vX.Y.Z"
    printf "%s" "$tag"
}

verify() {
    if [ ! -f "$STAGE/checksums.txt" ]; then
        warn "no checksums.txt next to the archive, installing unverified"
        return 0
    fi
    if ( cd "$STAGE" && grep -E "[ *]$ASSET\$" checksums.txt | sha256sum -c --status - ); then
        good "checksums verified"
    else
        die "checksum mismatch, refusing to install"
    fi
}

fetch() {
    STAGE="$(mktemp -d)"
    if [ -n "$LOCAL_DIR" ]; then
        step "Taking the release from $LOCAL_DIR"
        [ -f "$LOCAL_DIR/$ASSET" ] || die "no $ASSET in $LOCAL_DIR"
        cp "$LOCAL_DIR/$ASSET" "$STAGE/"
        [ -f "$LOCAL_DIR/checksums.txt" ] && cp "$LOCAL_DIR/checksums.txt" "$STAGE/"
    else
        local tag base
        tag="$(resolve_version)"
        base="https://github.com/$REPO/releases/download/$tag"
        step "Fetching $tag"
        curl -fsSL -o "$STAGE/$ASSET" "$base/$ASSET" \
            || die "cannot fetch $ASSET of $tag; if GitHub is unreachable, copy it over and pass --from DIR"
        curl -fsSL -o "$STAGE/checksums.txt" "$base/checksums.txt" 2>/dev/null || rm -f "$STAGE/checksums.txt"
    fi
    verify
    tar -xzf "$STAGE/$ASSET" -C "$STAGE" || die "the archive is damaged"
    [ -x "$STAGE/qd-client/install.sh" ] || die "the archive carries no installer"
}

tray_hint() {
    command -v gnome-shell >/dev/null 2>&1 || return 0
    ls -d /usr/share/gnome-shell/extensions/*appindicator* >/dev/null 2>&1 && return 0
    warn "GNOME shows no tray icons by itself; for the qd icon install the AppIndicator extension"
    warn "  (gnome-shell-extension-appindicator) and switch it on in Extensions"
}

install_packages
fetch
"$STAGE/qd-client/install.sh"
tray_hint
printf "\n" >&2
