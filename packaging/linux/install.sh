#!/usr/bin/env bash
set -Eeuo pipefail

PREFIX="/opt/qd-client"
UNIT="/etc/systemd/system/qd-client.service"
DESKTOP="/usr/share/applications/qd-client.desktop"
AUTOSTART="/etc/xdg/autostart/qd-client-tray.desktop"
ICON="/usr/share/icons/hicolor/256x256/apps/qd-client.png"
GROUP="qd-client"
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

if [ -t 2 ]; then
    BOLD=$'\033[1m'; RED=$'\033[31m'; GRN=$'\033[32m'; YEL=$'\033[33m'; CYA=$'\033[36m'; OFF=$'\033[0m'
else
    BOLD=""; RED=""; GRN=""; YEL=""; CYA=""; OFF=""
fi
step() { printf "\n  ${CYA}${BOLD}>${OFF} ${BOLD}%s${OFF}\n" "$*" >&2; }
good() { printf "    ${GRN}ok${OFF}  %s\n" "$*" >&2; }
warn() { printf "    ${YEL}!${OFF}   %s\n" "$*" >&2; }
die()  { printf "\n  ${RED}x${OFF}   %s\n\n" "$*" >&2; exit 1; }

[ "$(id -u)" -eq 0 ] || die "run this as root: sudo $0"
command -v systemctl >/dev/null 2>&1 || die "no systemd here, the client runs as a systemd service"
command -v ip >/dev/null 2>&1 || die "no ip command; install iproute2"

remove() {
    step "Removing the qd client"
    systemctl disable --now qd-client.service >/dev/null 2>&1 || true
    rm -f "$UNIT" "$DESKTOP" "$AUTOSTART" "$ICON"
    systemctl daemon-reload
    rm -rf "$PREFIX"
    command -v nft >/dev/null 2>&1 && nft delete table inet qd >/dev/null 2>&1 || true
    for g in qd-direct qd-tunnel; do
        [ -d "/sys/fs/cgroup/$g" ] || continue
        while read -r pid; do echo "$pid" > /sys/fs/cgroup/cgroup.procs; done < "/sys/fs/cgroup/$g/cgroup.procs" 2>/dev/null || true
        rmdir "/sys/fs/cgroup/$g" 2>/dev/null || true
    done
    command -v gtk-update-icon-cache >/dev/null 2>&1 && gtk-update-icon-cache -q /usr/share/icons/hicolor || true
    good "removed; the subscription is kept in /var/lib/qd-client, delete it by hand if you want it gone"
}

if [ "${1:-}" = "--remove" ]; then
    remove
    exit 0
fi

for f in qd-client qd-client-window qd-client.service qd-client.desktop qd-client.png; do
    [ -f "$HERE/$f" ] || die "$f is missing next to this installer"
done

step "Checking the window's libraries"
if ldconfig -p 2>/dev/null | grep -F "libwebkit2gtk-4.1.so.0" >/dev/null; then
    good "WebKitGTK 4.1 is present"
else
    warn "WebKitGTK 4.1 is missing, the service works but the window will not open. Install it with:"
    warn "  Debian/Ubuntu:  sudo apt install libwebkit2gtk-4.1-0"
    warn "  Arch:           sudo pacman -S webkit2gtk-4.1"
    warn "  Fedora:         sudo dnf install webkit2gtk4.1"
fi
if command -v nft >/dev/null 2>&1; then
    good "nftables is present"
else
    warn "nftables is missing, programs set to go direct will stay in the tunnel. Install it with:"
    warn "  Debian/Ubuntu:  sudo apt install nftables"
    warn "  Arch:           sudo pacman -S nftables"
    warn "  Fedora:         sudo dnf install nftables"
fi

step "Installing into $PREFIX"
systemctl stop qd-client.service >/dev/null 2>&1 || true
install -d -m 0755 "$PREFIX"
install -m 0755 "$HERE/qd-client" "$PREFIX/qd-client"
install -m 0755 "$HERE/qd-client-window" "$PREFIX/qd-client-window"
[ "$HERE" = "$PREFIX" ] || install -m 0755 "$HERE/install.sh" "$PREFIX/install.sh"
install -D -m 0644 "$HERE/qd-client.png" "$ICON"
install -D -m 0644 "$HERE/qd-client.desktop" "$DESKTOP"
install -m 0644 "$HERE/qd-client.service" "$UNIT"
command -v gtk-update-icon-cache >/dev/null 2>&1 && gtk-update-icon-cache -q /usr/share/icons/hicolor || true
command -v update-desktop-database >/dev/null 2>&1 && update-desktop-database -q /usr/share/applications || true
good "files in place"

step "Letting a desktop account open the window"
groupadd -f "$GROUP"
who="${SUDO_USER:-}"
if [ -n "$who" ] && [ "$who" != "root" ]; then
    if id -nG "$who" | tr ' ' '\n' | grep -Fx "$GROUP" >/dev/null; then
        good "$who is already in the $GROUP group"
    else
        usermod -aG "$GROUP" "$who"
        good "$who is in the $GROUP group; log out and back in once for it to take effect"
    fi
else
    warn "add your account to the $GROUP group: sudo usermod -aG $GROUP <name>"
fi

step "Starting the service"
systemctl daemon-reload
systemctl enable --now qd-client.service >/dev/null
sleep 2
if systemctl is-active --quiet qd-client.service; then
    good "qd-client is running; open qd from the applications menu"
else
    die "the service did not start, see: journalctl -u qd-client"
fi
