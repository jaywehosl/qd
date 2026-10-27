#!/usr/bin/env bash
set -Eeuo pipefail

REPO="jaywehosl/qd"
UPDATE_KEY="1dq/OBvDoYanXq7Jdgh7QzF64xDoDP1lFj1Uo3mvfdw="
PREFIX="/opt/qd"
STATE="/var/lib/qd"
DB="$STATE/node.db"
CONF="/etc/qd/node.conf"
SERVICE="qd-node"
UNIT="/etc/systemd/system/$SERVICE.service"
KEEP="$PREFIX/previous"

MODE=""
WANT_VERSION=""
LOCAL_DIR=""
ASSUME_YES=0
PURGE=0
LE="/etc/letsencrypt"

PORT=443
DOMAIN=""
EMAIL=""
ADMIN_TAG=""
ADMIN_UUID="${QD_ADMIN_UUID:-}"
GROUP_TAG=""
ADDRESS=""
NETWORK_KEY="${QD_NETWORK_KEY:-}"
NODE_TAG=""
NODE_UUID="${QD_NODE_UUID:-}"
NODE_ID=""
NAT_SLOT=""
ROLE="ingress"
AUTHORITY=""
CERT=""
KEYFILE=""
WANT_CERT=1
CERT_ISSUED=0
DNS1="1.1.1.1"
DNS2="8.8.8.8"
DNS_CACHE=4096
DNS_MIN_TTL=60
DNS_MAX_TTL=3600
DNS_STALE=60

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
signed() {
    local pub sig ok=1
    pub="$(mktemp)"; sig="$(mktemp)"
    { printf '\060\052\060\005\006\003\053\145\160\003\041\000'; printf '%s' "$UPDATE_KEY" | base64 -d; } >"$pub"
    if base64 -d <"$1.sig" >"$sig" 2>/dev/null &&
        openssl pkeyutl -verify -pubin -keyform DER -inkey "$pub" -rawin -in "$1" -sigfile "$sig" >/dev/null 2>&1; then
        ok=0
    fi
    rm -f "$pub" "$sig"
    return $ok
}
check_signature() {
    if ! openssl pkeyutl -help 2>&1 | grep -q -- '-rawin'; then
        warn "$(openssl version | cut -d' ' -f1-2) cannot check Ed25519 signatures (OpenSSL 3.0 can), only the checksums are verified"
        return 0
    fi
    signed "$1" || die "$2 are not signed by the qd release key, refusing to install"
    good "release signature verified"
}
rule() { printf "  ${DIM}%s${OFF}\n" "--------------------------------------------------------" >&2; }

banner() {
    printf "\n  ${BOLD}qd${OFF} ${DIM}node installer${OFF}\n" >&2
    rule
}

usage() {
    cat <<'USAGE'
qd node installer.

Run it with no arguments to start a new network: it asks for what it needs.
Run it with the command the panel hands out to join an existing network: it
asks nothing and writes exactly what the panel handed it.

  --mode install|update|fresh|over|uninstall
                                     what to do when something is already here
  --uninstall                        short for --mode uninstall
  --purge                            uninstall without questions, and also delete the
                                     certificate of the node and certbot itself
  --version vX.Y.Z                   release to install, default: latest
  --from DIR                         install from files already on this machine
                                     instead of downloading them
  --domain NAME                      domain clients dial and the certificate is
                                     issued for; --address and --host mean the same
  --email ADDR                       e-mail Let's Encrypt registers against
  --no-cert                          skip certbot; clients refuse the node until it
                                     has a certificate they trust (--cert, --key-file)
  --port N                           udp port
  --dns1 A --dns2 B                  upstream resolvers
  --dns-cache N --dns-min-ttl N --dns-max-ttl N --dns-stale N
  --yes                              never ask, requires --mode on an existing install
  --help

Starting a network, asked when absent:
  --admin TAG --group TAG            the first administrator and the first group

Joining a network, every value comes from the panel:
  --key HEX                          the network key, or QD_NETWORK_KEY in the environment
  --authority HOST:PORT              what this node answers as, default: domain:port
                                     the certificate is issued by this script itself
  --tag NAME --role ingress|egress   what this node is
  --node-uuid UUID --node-id N       identity and number of this node, the uuid
                                     also as QD_NODE_UUID
  --admin-uuid UUID --admin TAG      the administrator that may reach it, the uuid
                                     also as QD_ADMIN_UUID

Modes when something is already installed:
  update    keep the database, swap the binary, roll back if it will not start
  fresh     back the database up, remove everything, build a new network
  over      overwrite the files, leave the database alone
  uninstall remove the node, its database and its unit; asks about the
            certificate and certbot
USAGE
}

while [ $# -gt 0 ]; do
    case "$1" in
        --no-cert|--uninstall|--purge|--yes|-y|--help|-h) ;;
        -*) [ $# -ge 2 ] || die "$1 needs a value" ;;
    esac
    case "$1" in
        --mode)        MODE="${2:-}"; shift 2 ;;
        --version)     WANT_VERSION="${2:-}"; shift 2 ;;
        --from)        LOCAL_DIR="${2:-}"; shift 2 ;;
        --port)        PORT="${2:-}"; shift 2 ;;
        --address|--host|--domain) ADDRESS="${2:-}"; DOMAIN="${2:-}"; shift 2 ;;
        --email)       EMAIL="${2:-}"; shift 2 ;;
        --no-cert)     WANT_CERT=0; shift ;;
        --admin)       ADMIN_TAG="${2:-}"; shift 2 ;;
        --admin-uuid)  ADMIN_UUID="${2:-}"; shift 2 ;;
        --group)       GROUP_TAG="${2:-}"; shift 2 ;;
        --key)         NETWORK_KEY="${2:-}"; shift 2 ;;
        --tag)         NODE_TAG="${2:-}"; shift 2 ;;
        --node-uuid)   NODE_UUID="${2:-}"; shift 2 ;;
        --node-id)     NODE_ID="${2:-}"; shift 2 ;;
        --nat-slot)    NAT_SLOT="${2:-}"; shift 2 ;;
        --role)        ROLE="${2:-}"; shift 2 ;;
        --authority)   AUTHORITY="${2:-}"; shift 2 ;;
        --cert)        CERT="${2:-}"; shift 2 ;;
        --key-file)    KEYFILE="${2:-}"; shift 2 ;;
        --dns1)        DNS1="${2:-}"; shift 2 ;;
        --dns2)        DNS2="${2:-}"; shift 2 ;;
        --dns-cache)   DNS_CACHE="${2:-}"; shift 2 ;;
        --dns-min-ttl) DNS_MIN_TTL="${2:-}"; shift 2 ;;
        --dns-max-ttl) DNS_MAX_TTL="${2:-}"; shift 2 ;;
        --dns-stale)   DNS_STALE="${2:-}"; shift 2 ;;
        --uninstall)   MODE="uninstall"; shift ;;
        --purge)       MODE="uninstall"; PURGE=1; shift ;;
        --yes|-y)      ASSUME_YES=1; shift ;;
        --help|-h)     usage; exit 0 ;;
        *)             die "unknown argument: $1" ;;
    esac
done

interactive() {
    if [ "$ASSUME_YES" -eq 1 ]; then return 1; fi
    [ -t 0 ] || [ -e /dev/tty ]
}

ask() {
    local prompt="$1" default="$2" answer=""
    if ! interactive; then printf "%s" "$default"; return 0; fi
    read -r -p "$prompt [$default]: " answer </dev/tty || answer=""
    printf "%s" "${answer:-$default}"
}

ask_tag() {
    local prompt="$1" default="$2" value
    while true; do
        value="$(ask "$prompt" "$default")"
        if [[ "$value" =~ ^[A-Za-z0-9_-]{1,32}$ ]]; then printf "%s" "$value"; return 0; fi
        warn "letters, digits, dash and underscore only, up to 32"
        if ! interactive; then die "invalid tag: $value"; fi
    done
}

ask_number() {
    local prompt="$1" default="$2" low="$3" high="$4" value
    while true; do
        value="$(ask "$prompt" "$default")"
        if [[ "$value" =~ ^[0-9]+$ ]] && [ "$value" -ge "$low" ] && [ "$value" -le "$high" ]; then
            printf "%s" "$value"; return 0
        fi
        warn "a whole number between $low and $high"
        if ! interactive; then die "invalid number: $value"; fi
    done
}

require_root() { [ "$(id -u)" -eq 0 ] || die "run this as root"; }

require_tools() {
    local missing=()
    command -v curl >/dev/null 2>&1 || missing+=(curl)
    command -v openssl >/dev/null 2>&1 || missing+=(openssl)
    if [ ${#missing[@]} -gt 0 ]; then
        if command -v apt-get >/dev/null 2>&1; then
            say "${missing[*]} missing, installing"
            apt-get update -qq && apt-get install -y -qq "${missing[@]}"
        else
            die "${missing[*]} missing and this is not an apt system, install it and run again"
        fi
    fi
    command -v systemctl >/dev/null 2>&1 || die "no systemd here, this installer needs it"
    [ "$(uname -m)" = "x86_64" ] || die "the release carries linux/amd64 only, this is $(uname -m)"
}

need_package() {
    local tool="$1" pkg="$2"
    command -v "$tool" >/dev/null 2>&1 && return 0
    command -v apt-get >/dev/null 2>&1 || die "$tool is missing and this is not an apt system"
    say "$tool is missing, installing $pkg"
    apt-get update -qq >/dev/null 2>&1 || true
    DEBIAN_FRONTEND=noninteractive apt-get install -y -qq "$pkg" >/dev/null 2>&1 \
        || die "could not install $pkg"
    command -v "$tool" >/dev/null 2>&1 || die "$pkg installed but $tool is still missing"
}

my_addresses() {
    ip -o addr show scope global 2>/dev/null \
        | sed -n 's/.*inet6\{0,1\} \([0-9a-f.:]*\)\/.*/\1/p'
}

has_ipv6() {
    ip -6 route get 2606:4700:4700::1111 >/dev/null 2>&1
}

resolve_records() {
    local name="$1" kind="$2"
    getent ahostsv4 "$name" 2>/dev/null | awk '{print $1}' | sort -u > /tmp/qd-a.$$
    getent ahostsv6 "$name" 2>/dev/null | awk '{print $1}' | sort -u > /tmp/qd-aaaa.$$
    case "$kind" in
        a) cat /tmp/qd-a.$$ ;;
        aaaa) cat /tmp/qd-aaaa.$$ ;;
    esac
    rm -f /tmp/qd-a.$$ /tmp/qd-aaaa.$$
}

check_domain() {
    local name="$1"
    step "checking $name"

    local mine v4 v6
    mine="$(my_addresses)"
    v4="$(resolve_records "$name" a)"
    if [ -z "$v4" ]; then
        die "$name has no A record; point it at this machine and run again"
    fi

    local matched=0 one
    for one in $v4; do
        if printf "%s\n" "$mine" | grep -qx "$one"; then matched=1; fi
    done
    if [ "$matched" -eq 1 ]; then
        good "A record points here"
    else
        warn "$name resolves to: $(echo $v4)"
        warn "this machine holds: $(echo $mine)"
        say  "behind NAT that is expected as long as the port is forwarded here"
        if interactive; then
            local answer
            read -r -p "  carry on anyway? [y/N]: " answer </dev/tty || answer=""
            case "$answer" in y|Y|yes) ;; *) die "stopped: $name does not point at this machine" ;; esac
        elif [ "$ASSUME_YES" -ne 1 ]; then
            die "$name does not point at this machine, pass --yes to carry on anyway"
        fi
    fi

    if has_ipv6; then
        v6="$(resolve_records "$name" aaaa)"
        if [ -z "$v6" ]; then
            warn "this machine has IPv6 but $name has no AAAA record"
            say  "clients on IPv6-only networks will not reach it"
        else
            good "AAAA record present"
        fi
    fi
}

port_taken() {
    local port="$1" proto="$2"
    command -v ss >/dev/null 2>&1 || return 1
    ss -ln"${proto}" 2>/dev/null | awk 'NR>1 {print $4}' | grep -qE "[:.]$port$"
}

check_ports() {
    step "checking ports"
    local busy=""
    port_taken "$PORT" u && busy="$busy $PORT/udp"
    port_taken "$PORT" t && busy="$busy $PORT/tcp"
    if [ "$WANT_CERT" -eq 1 ]; then
        port_taken 80 t && busy="$busy 80/tcp"
    fi

    if [ -z "$busy" ]; then
        good "free:$( [ "$WANT_CERT" -eq 1 ] && printf " 80/tcp") $PORT/tcp $PORT/udp"
        return 0
    fi

    warn "already in use:$busy"
    say  "who holds them:"
    ss -lntup 2>/dev/null | grep -E "[:.]($PORT|80)[[:space:]]" >&2 || true
    if is_running; then
        say "an older qd node is running here; choose 'fresh' or 'over' when asked"
        return 0
    fi
    die "free those ports and run again"
}

certbot_paths() {
    CERT="/etc/letsencrypt/live/$1/fullchain.pem"
    KEYFILE="/etc/letsencrypt/live/$1/privkey.pem"
}

have_certificate() {
    [ -f "/etc/letsencrypt/live/$1/fullchain.pem" ] && [ -f "/etc/letsencrypt/live/$1/privkey.pem" ]
}

issue_certificate() {
    local name="$1"
    if have_certificate "$name"; then
        certbot_paths "$name"
        good "certificate for $name is already here"
        return 0
    fi

    need_package certbot certbot

    local who=(--register-unsafely-without-email)
    [ -n "$EMAIL" ] && who=(-m "$EMAIL")

    step "asking Let's Encrypt for $name (dry run first)"
    if ! certbot certonly --standalone --non-interactive --agree-tos \
            "${who[@]}" -d "$name" --dry-run >/tmp/qd-certbot.log 2>&1; then
        tail -20 /tmp/qd-certbot.log >&2
        die "the dry run failed; the log above says why (usually port 80 or the DNS record)"
    fi
    good "dry run passed"

    step "issuing the certificate"
    if ! certbot certonly --standalone --non-interactive --agree-tos \
            "${who[@]}" -d "$name" >/tmp/qd-certbot.log 2>&1; then
        tail -20 /tmp/qd-certbot.log >&2
        die "certbot could not issue the certificate"
    fi
    CERT_ISSUED=1
    certbot_paths "$name"
    good "certificate in /etc/letsencrypt/live/$name"

    mkdir -p /etc/letsencrypt/renewal-hooks/deploy
    cat >/etc/letsencrypt/renewal-hooks/deploy/qd-node.sh <<'HOOK'
#!/bin/sh
systemctl restart qd-node 2>/dev/null || true
HOOK
    chmod 0755 /etc/letsencrypt/renewal-hooks/deploy/qd-node.sh
    good "renewal restarts the node"
}

resolve_version() {
    if [ -n "$LOCAL_DIR" ]; then printf "local"; return 0; fi
    if [ -n "$WANT_VERSION" ]; then printf "%s" "$WANT_VERSION"; return 0; fi

    local tag
    tag="$(curl -fsSLI -o /dev/null -w '%{url_effective}' \
        "https://github.com/$REPO/releases/latest" 2>/dev/null | sed 's#.*/tag/##')"

    if [ -z "$tag" ] || [ "${tag#v}" = "$tag" ]; then
        tag="$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" 2>/dev/null \
            | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -1)"
    fi

    if [ -z "$tag" ]; then
        tag="$(curl -fsSL "https://api.github.com/repos/$REPO/releases?per_page=1" 2>/dev/null \
            | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -1)"
        [ -n "$tag" ] && say "latest release is a pre-release: $tag"
    fi

    [ -n "$tag" ] || die "cannot work out the latest release of $REPO, pass --version vX.Y.Z"
    printf "%s" "$tag"
}

installed_version() {
    if [ -x "$PREFIX/qd-node" ]; then
        "$PREFIX/qd-node" -version 2>/dev/null || printf "unknown"
    else
        printf "none"
    fi
}

stage_from_dir() {
    local dir="$1"
    STAGE="$(mktemp -d)"
    step "taking the release from $dir"

    local binary=""
    for name in qd-node-linux-amd64 qd-node; do
        if [ -f "$dir/$name" ]; then binary="$dir/$name"; break; fi
    done
    [ -n "$binary" ] || die "no qd-node-linux-amd64 or qd-node in $dir"

    cp "$binary" "$STAGE/qd-node"

    if [ -f "$dir/checksums.txt" ]; then
        cp "$dir/checksums.txt" "$STAGE/checksums.txt"
        if [ -f "$dir/checksums.txt.sig" ]; then
            cp "$dir/checksums.txt.sig" "$STAGE/checksums.txt.sig"
            check_signature "$STAGE/checksums.txt" "the checksums in $dir"
        else
            warn "no checksums.txt.sig in $dir, the checksums are unsigned"
        fi
        if ( cd "$STAGE" && sed 's/qd-node-linux-amd64/qd-node/' checksums.txt | grep -E '[ *]qd-node$' | sha256sum -c --status - ); then
            good "checksums verified"
        else
            die "checksum mismatch in $dir, refusing to install"
        fi
    else
        warn "no checksums.txt in $dir, installing unverified"
    fi
    chmod 0755 "$STAGE/qd-node"
    good "version $("$STAGE/qd-node" -version 2>/dev/null || echo unknown)"
}

fetch_release() {
    if [ -n "$LOCAL_DIR" ]; then stage_from_dir "$LOCAL_DIR"; return 0; fi

    local tag="$1" base="https://github.com/$REPO/releases/download/$tag"
    STAGE="$(mktemp -d)"
    step "fetching $tag"
    curl -fsSL -o "$STAGE/qd-node" "$base/qd-node-linux-amd64" \
        || die "cannot fetch the binary of $tag; if the release host is unreachable, copy the files over and pass --from DIR"

    curl -fsSL -o "$STAGE/checksums.txt" "$base/checksums.txt" 2>/dev/null \
        && curl -fsSL -o "$STAGE/checksums.txt.sig" "$base/checksums.txt.sig" 2>/dev/null \
        || die "$tag carries no signed checksums, refusing to install"
    check_signature "$STAGE/checksums.txt" "the checksums of $tag"
    if ( cd "$STAGE" && sed 's/qd-node-linux-amd64/qd-node/' checksums.txt | grep -E '[ *]qd-node$' | sha256sum -c --status - ); then
        good "checksums verified"
    else
        die "checksum mismatch, refusing to install"
    fi
    chmod 0755 "$STAGE/qd-node"
}

has_unit()   { [ -f "$UNIT" ]; }
has_files()  { [ -e "$PREFIX/qd-node" ]; }
has_db()     { [ -f "$DB" ]; }
is_running() { systemctl is-active --quiet "$SERVICE" 2>/dev/null; }

anything_here() {
    if has_unit; then return 0; fi
    if has_files; then return 0; fi
    if has_db; then return 0; fi
    return 1
}

stamp() { date +%Y%m%d-%H%M%S; }

running_port() {
    local port=""
    if [ -r "$CONF" ]; then
        port="$(awk -F= '/^[[:space:]]*port[[:space:]]*=/ {gsub(/[[:space:]]/, "", $2); print $2; exit}' "$CONF")"
    fi
    if [ -z "$port" ] && command -v ss >/dev/null 2>&1; then
        port="$(ss -lunp 2>/dev/null | awk '/qd-node/ {n=split($4,a,":"); p=a[n]+0; if (!m || p<m) m=p} END {if (m) print m}' || true)"
    fi
    printf '%s' "$port"
}

backup_database() {
    has_db || return 0
    local dest="$STATE/backup-$(stamp)"
    mkdir -p "$dest"
    cp -a "$DB" "$dest/" 2>/dev/null || true
    cp -a "$DB-wal" "$dest/" 2>/dev/null || true
    cp -a "$DB-shm" "$dest/" 2>/dev/null || true
    say "database copied to $dest"
}

report_state() {
    local size="none"
    if has_db; then size="$DB ($(du -h "$DB" 2>/dev/null | cut -f1))"; fi
    step "what is already here"
    say "  unit       $(if has_unit; then echo "$UNIT"; else echo none; fi)"
    say "  service    $(if is_running; then echo running; else echo stopped; fi)"
    say "  files      $(if has_files; then echo "$PREFIX"; else echo none; fi)"
    say "  version    $(installed_version)"
    say "  database   $size"
}

choose_mode() {
    if [ -n "$MODE" ]; then return 0; fi
    if ! anything_here; then MODE="install"; return 0; fi

    report_state
    if ! interactive; then
        die "something is already installed; pass --mode update|fresh|over"
    fi

    cat >&2 <<'MENU'

This machine already carries a node. Pick one:

  1  update  keep the database and the network, swap the binary only.
             Rewrites the unit and rolls back by itself if the pair will not start.
  2  fresh   back the database up, remove unit and files, build a NEW network.
             The current network key and every client on it stop working here.
  3  over    overwrite the binary, leave the database alone.
  4  remove  take qd off this machine entirely: unit, files, database;
             asks whether the certificate and certbot go too.
  5  cancel

MENU
    local pick=""
    while true; do
        read -r -p "choice [1]: " pick </dev/tty || pick=""
        case "${pick:-1}" in
            1) MODE="update"; return 0 ;;
            2) MODE="fresh";  return 0 ;;
            3) MODE="over";      return 0 ;;
            4) MODE="uninstall"; return 0 ;;
            5) say "nothing done"; exit 0 ;;
            *) warn "1, 2, 3, 4 or 5" ;;
        esac
    done
}

confirm_fresh() {
    if ! interactive; then return 0; fi
    warn ""
    warn "fresh takes this node out of the current network."
    warn "Its clients keep the old key and will not reach this machine again."
    local answer=""
    read -r -p "type the word fresh to go ahead: " answer </dev/tty || answer=""
    if [ "$answer" != "fresh" ]; then say "nothing done"; exit 0; fi
}

confirm_uninstall() {
    if ! interactive; then return 0; fi
    warn ""
    warn "this removes the node, its unit and its database from this machine."
    warn "On the first node of a network that is the only copy of the network key,"
    warn "and every link handed out on it stops working."
    local answer=""
    read -r -p "  type the word remove to go ahead: " answer </dev/tty || answer=""
    if [ "$answer" != "remove" ]; then say "nothing done"; exit 0; fi
}

left() {
    if [ "$2" = "yes" ]; then
        printf "    ${RED}x${OFF}   %-24s still here\n" "$1" >&2
    else
        printf "    ${GRN}ok${OFF}  %-24s gone\n" "$1" >&2
    fi
}

yesno() { if "$@" >/dev/null 2>&1; then echo yes; else echo no; fi; }

write_unit() {
    cat >"$UNIT" <<UNITFILE
[Unit]
Description=qd node
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=$PREFIX/qd-node -config $CONF
Restart=on-failure
RestartSec=3
WorkingDirectory=$PREFIX
LimitMEMLOCK=infinity
LogRateLimitIntervalSec=0

[Install]
WantedBy=multi-user.target
UNITFILE
    systemctl daemon-reload
}

cap_journal() {
    mkdir -p /etc/systemd/journald.conf.d
    cat >/etc/systemd/journald.conf.d/qd.conf <<'JOURNALCONF'
[Journal]
SystemMaxUse=200M
SystemMaxFileSize=20M
JOURNALCONF

    systemctl restart systemd-journald >/dev/null 2>&1 || true
    journalctl --vacuum-size=200M >/dev/null 2>&1 || true
    say "journal capped at 200M"
}

tune_buffers() {
    local want=4194304
    local now pages low
    now=$(sysctl -n net.core.rmem_max 2>/dev/null || echo 0)
    pages=$(( $(awk '/^MemTotal:/ {print $2}' /proc/meminfo) * 1024 / $(getconf PAGESIZE) ))
    low=$(sysctl -n net.ipv4.tcp_mem 2>/dev/null | awk '{print $2}')
    if [ "$now" -ge "$want" ] && [ "${low:-0}" -ge $((pages / 4)) ]; then
        return 0
    fi

    cat >/etc/sysctl.d/99-qd.conf <<SYSCTLCONF
net.core.rmem_max = $want
net.core.wmem_max = $want
net.ipv4.tcp_mem = $((pages / 8)) $((pages / 4)) $((pages * 3 / 8))
SYSCTLCONF
    sysctl -p /etc/sysctl.d/99-qd.conf >/dev/null 2>&1 || true
    say "socket buffer ceiling raised from $now to $(sysctl -n net.core.rmem_max 2>/dev/null)"
}

install_files() {
    mkdir -p "$PREFIX" "$STATE"
    install -m 0755 "$STAGE/qd-node" "$PREFIX/qd-node"
}

keep_current() {
    has_files || return 0
    rm -rf "$KEEP"
    mkdir -p "$KEEP"
    cp -a "$PREFIX/qd-node" "$KEEP/" 2>/dev/null || true
}

restore_kept() {
    [ -f "$KEEP/qd-node" ] || return 1
    install -m 0755 "$KEEP/qd-node" "$PREFIX/qd-node"
    return 0
}

open_firewall() {
    command -v ufw >/dev/null 2>&1 || return 0
    ufw status 2>/dev/null | grep -q "^Status: active" || return 0
    if ufw allow "$1/udp" >/dev/null 2>&1; then say "ufw: opened $1/udp"; fi
    if ufw allow "$1/tcp" >/dev/null 2>&1; then say "ufw: opened $1/tcp"; fi
    return 0
}

verify_live() {
    local name="$1" port="$2" ok=1

    step "checking the node answers"
    if ss -lun 2>/dev/null | grep -qE "[:.]$port[[:space:]]"; then
        good "udp/$port is open"
    else
        warn "nothing listens on udp/$port"
        ok=0
    fi

    local code
    code="$(curl -s -o /dev/null -w '%{http_code}' --max-time 8 "https://$name:$port/" 2>/dev/null || true)"
    case "$code" in
        2*|3*|4*) good "the site answers on tcp/$port (http $code)" ;;
        *)
            warn "the site did not answer on tcp/$port"
            [ "$WANT_CERT" -eq 1 ] && say "if the certificate is fresh this is usually the name, not the port"
            ok=0
            ;;
    esac

    if [ "$ok" -eq 1 ]; then
        return 0
    fi
    warn "last lines of the log:"
    journalctl -u "$SERVICE" -n 20 --no-pager >&2 || true
    die "the node started but does not answer as it should"
}
wait_healthy() {
    local port="${1:-}" i
    for i in $(seq 1 20); do
        sleep 1
        if ! is_running; then continue; fi
        if [ -z "$port" ] || ! command -v ss >/dev/null 2>&1; then
            sleep 2
            if is_running; then return 0; fi
            continue
        fi
        if ss -lun 2>/dev/null | grep -q ":$port[[:space:]]"; then return 0; fi
    done
    return 1
}

joining() { [ -n "$NETWORK_KEY" ]; }

require_arg() {
    [ -n "$2" ] || die "joining a network needs $1"
}

take_join_arguments() {
    require_arg --tag "$NODE_TAG"
    require_arg --role "$ROLE"
    require_arg --admin "$ADMIN_TAG"
    require_arg --admin-uuid "$ADMIN_UUID"
    case "$ROLE" in
        ingress|egress) ;;
        *) die "--role is ingress or egress, not '$ROLE'" ;;
    esac
    if ! [[ "$NETWORK_KEY" =~ ^[0-9a-fA-F]{64}$ ]]; then
        die "--key must be 64 hex characters"
    fi
    [ -n "$ADDRESS" ] || die "joining a network needs --domain"
    DOMAIN="$ADDRESS"
    if [ -z "$EMAIL" ] && [ "$WANT_CERT" -eq 1 ] && interactive; then
        EMAIL="$(ask "e-mail for Let's Encrypt, empty to register without one" "")"
    fi
    step "joining as $NODE_TAG, $ROLE on $ADDRESS:$PORT"
}

ask_domain() {
    local value
    while true; do
        value="$(ask "domain clients dial" "$DOMAIN")"
        if [[ "$value" =~ ^[A-Za-z0-9]([A-Za-z0-9.-]*[A-Za-z0-9])?\.[A-Za-z]{2,}$ ]]; then
            printf "%s" "$value"; return 0
        fi
        warn "a domain name, like node.example.com — the certificate is issued for it"
        if ! interactive; then die "invalid domain: $value"; fi
    done
}

ask_install_questions() {
    if joining; then take_join_arguments; return 0; fi

    step "the network this node starts"
    DOMAIN="$(ask_domain)"
    ADDRESS="$DOMAIN"
    if [ "$WANT_CERT" -eq 1 ]; then
        EMAIL="$(ask "e-mail for Let's Encrypt, empty to register without one" "$EMAIL")"
    fi
    PORT="$(ask_number "udp port clients dial" "$PORT" 1 65535)"
    ADMIN_TAG="$(ask_tag "tag of the first admin" "${ADMIN_TAG:-admin}")"
    GROUP_TAG="$(ask_tag "tag of the default group" "${GROUP_TAG:-default}")"
}

initialise_database() {
    step "writing the database"
    mkdir -p "$STATE"
    local out
    if ! out="$("$STAGE/qd-node" -init -db "$DB" \
        -key "$NETWORK_KEY" -port "$PORT" -address "$ADDRESS" \
        -role "$ROLE" -tag "$NODE_TAG" -node-uuid "$NODE_UUID" -node-id "${NODE_ID:-0}" ${NAT_SLOT:+-nat-slot "$NAT_SLOT"} \
        -admin "$ADMIN_TAG" -admin-uuid "$ADMIN_UUID" -group "$GROUP_TAG" \
        -dns1 "$DNS1" -dns2 "$DNS2" -dns-cache "$DNS_CACHE" \
        -dns-min-ttl "$DNS_MIN_TTL" -dns-max-ttl "$DNS_MAX_TTL" -dns-stale "$DNS_STALE" \
        -config "$CONF" -authority "$AUTHORITY" -cert "$CERT" -key-file "$KEYFILE")"; then
        rm -f "$DB" "$DB-wal" "$DB-shm"
        die "the node refused to write the database"
    fi
    printf "%s\n" "$out" >"$STATE/first-node.txt"
    chmod 0600 "$STATE/first-node.txt"
    printf "%s" "$out"
}

field() { printf "%s\n" "$1" | sed -n "s/^$2=//p"; }

do_install() {
    ask_install_questions
    check_domain "$DOMAIN"
    check_ports

    local tag; tag="$(resolve_version)"
    fetch_release "$tag"

    if [ "$WANT_CERT" -eq 1 ]; then
        issue_certificate "$DOMAIN"
    else
        warn "no certificate asked for: clients refuse this node until it has one they trust"
    fi
    [ -n "$AUTHORITY" ] || AUTHORITY="$DOMAIN:$PORT"

    local out; out="$(initialise_database)"
    install_files
    write_unit
    cap_journal
    tune_buffers
    open_firewall "$PORT"

    step "starting"
    systemctl enable --now "$SERVICE" >/dev/null 2>&1 || true
    if ! wait_healthy "$PORT"; then
        warn "the node did not come up, last lines of its log:"
        journalctl -u "$SERVICE" -n 25 --no-pager >&2 || true
        die "install finished but the service is not healthy"
    fi
    verify_live "$DOMAIN" "$PORT"

    if joining; then
        cat <<JOINED

== the node is up

  node        $(field "$out" node-tag) on $DOMAIN:$PORT, $(field "$out" node-role)
  version     $(installed_version)
  certificate ${CERT:-self-signed}
  database    $DB

It carries the network key and knows $(field "$out" admin-tag), so the panel can
already reach it. Press "copy the network database onto this node" in the panel
to hand it the rest of the network.
JOINED
        return 0
    fi

    cat <<SUMMARY

== the network is up

  node        $(field "$out" node-tag) on $DOMAIN:$PORT, ingress
  version     $(installed_version)
  certificate ${CERT:-self-signed}
  database    $DB

Import this into the client. It carries the admin key, so the same link opens
the panel:

  $(field "$out" link)

Keep these two, the deploy script of every other node needs them:

  network key   $(field "$out" network-key)
  admin uuid    $(field "$out" admin-uuid)

Both are in $STATE/first-node.txt as well, readable by root only.
SUMMARY
}

do_update() {
    local tag current port
    tag="$(resolve_version)"
    current="$(installed_version)"
    say "installed $current, target $tag"
    if [ "$current" = "$tag" ] || [ "$current" = "${tag#v}" ]; then
        say "already on $tag, nothing to do"
        return 0
    fi

    port="$(running_port)"
    fetch_release "$tag"
    keep_current

    step "swapping"
    systemctl stop "$SERVICE" 2>/dev/null || true
    backup_database
    install_files
    write_unit
    cap_journal
    tune_buffers
    systemctl start "$SERVICE" 2>/dev/null || true

    if wait_healthy "$port"; then
        say "now on $(installed_version), the previous pair is kept in $KEEP"
        return 0
    fi

    warn "the new pair did not come up, rolling back"
    journalctl -u "$SERVICE" -n 25 --no-pager >&2 || true
    systemctl stop "$SERVICE" 2>/dev/null || true
    if restore_kept; then
        systemctl start "$SERVICE" 2>/dev/null || true
        if wait_healthy "$port"; then
            die "rolled back to $(installed_version), $tag does not start on this machine"
        fi
    fi
    die "the update failed and so did the rollback, the node is down"
}

do_fresh() {
    confirm_fresh
    backup_database
    step "removing the current install"
    systemctl disable --now "$SERVICE" >/dev/null 2>&1 || true
    rm -f "$UNIT"
    systemctl daemon-reload
    rm -rf "$PREFIX"
    rm -f "$DB" "$DB-wal" "$DB-shm"
    do_install
}

do_over() {
    local tag was_running=0
    tag="$(resolve_version)"
    if is_running; then was_running=1; fi
    fetch_release "$tag"
    keep_current

    step "overwriting the files"
    systemctl stop "$SERVICE" 2>/dev/null || true
    install_files
    if [ "$was_running" -eq 1 ]; then
        systemctl start "$SERVICE" 2>/dev/null || true
    fi
    say "the files are now $(installed_version), database and unit untouched"
}

ask_yes() {
    local answer="" hint="[y/N]"
    if ! interactive; then [ "$2" = "yes" ]; return; fi
    [ "$2" = "yes" ] && hint="[Y/n]"
    read -r -p "  $1 $hint: " answer </dev/tty || answer=""
    case "${answer:-$2}" in
        y|Y|yes|Yes|YES) return 0 ;;
    esac
    return 1
}

node_certificate() {
    [ -r "$CONF" ] || return 0
    awk -F= '/^[[:space:]]*cert[[:space:]]*=/ {gsub(/[[:space:]]/, "", $2); print $2; exit}' "$CONF" \
        | sed -n "s#^$LE/live/\([^/]*\)/.*#\1#p"
}

other_certificates() {
    local dir name
    for dir in "$LE"/live/*/; do
        name="$(basename "$dir")"
        if [ -d "$LE/live/$name" ] && [ "$name" != "$1" ]; then printf "%s " "$name"; fi
    done
    return 0
}

certbot_held() { command -v dpkg >/dev/null 2>&1 && dpkg -s certbot >/dev/null 2>&1; }

drop_certificate() {
    if command -v certbot >/dev/null 2>&1 &&
        certbot delete --cert-name "$1" --non-interactive >/dev/null 2>&1; then
        return 0
    fi
    rm -rf "$LE/live/$1" "$LE/archive/$1" "$LE/renewal/$1.conf"
}

drop_certbot() {
    DEBIAN_FRONTEND=noninteractive apt-get purge -y -qq certbot >/dev/null 2>&1 || true
    DEBIAN_FRONTEND=noninteractive apt-get autoremove -y -qq --purge >/dev/null 2>&1 || true
    rm -rf "$LE" /var/lib/letsencrypt /var/log/letsencrypt
}

do_uninstall() {
    local port cert others copy="" drop_cert=0 drop_bot=0 tuned=0 present=0
    if anything_here; then
        present=1
        report_state
        confirm_uninstall
    else
        step "no node here"
        say "no unit, no files and no database of qd; clearing what may be left around them"
    fi

    port="$(running_port)"
    cert="$(node_certificate)"
    if [ -f /etc/sysctl.d/99-qd.conf ]; then tuned=1; fi

    step "what else goes"
    if has_db && [ "$PURGE" -eq 0 ] && ask_yes "keep a copy of the database and the network key in /root" no; then
        copy="/root/qd-node-$(stamp)"
    fi
    if [ -n "$cert" ] && [ -d "$LE/live/$cert" ]; then
        if [ "$PURGE" -eq 1 ] || ask_yes "delete the certificate for $cert" no; then
            drop_cert=1
        fi
    fi
    if certbot_held; then
        if [ "$drop_cert" -eq 1 ]; then
            others="$(other_certificates "$cert")"
        else
            others="$(other_certificates "")"
        fi
        if [ -n "$others" ]; then
            say "certbot stays: it still holds ${others% }"
        elif [ "$PURGE" -eq 1 ] || ask_yes "remove certbot, the packages it brought and $LE" no; then
            drop_bot=1
        fi
    fi

    if [ "$present" -eq 1 ]; then
        step "stopping the node"
        systemctl disable --now "$SERVICE" >/dev/null 2>&1 || true
        sleep 2
    fi
    rm -f "$UNIT"
    rm -rf "/etc/systemd/system/$SERVICE.d"
    systemctl daemon-reload >/dev/null 2>&1 || true

    if [ -n "$copy" ]; then
        mkdir -p "$copy"
        chmod 0700 "$copy"
        cp -a "$DB" "$DB-wal" "$DB-shm" "$STATE/first-node.txt" "$CONF" "$copy/" 2>/dev/null || true
        good "database, config and network key copied to $copy"
    fi

    step "removing what the installer set up around it"
    rm -f /etc/systemd/journald.conf.d/qd.conf /etc/sysctl.d/99-qd.conf \
        "$LE/renewal-hooks/deploy/qd-node.sh" /tmp/qd-certbot.log
    systemctl restart systemd-journald >/dev/null 2>&1 || true
    sysctl --system >/dev/null 2>&1 || true
    if [ "$tuned" -eq 1 ]; then say "socket buffer limits go back to the system defaults after a reboot"; fi

    if [ -n "$port" ] && command -v ufw >/dev/null 2>&1; then
        for proto in udp tcp; do
            if ufw show added 2>/dev/null | grep -q "allow $port/$proto\$" &&
                ufw --force delete allow "$port/$proto" >/dev/null 2>&1; then
                say "ufw: closed $port/$proto"
            fi
        done
    fi

    if [ "$present" -eq 1 ]; then step "removing files"; fi
    rm -rf "$PREFIX" "$STATE" "$(dirname "$CONF")"

    if [ "$drop_cert" -eq 1 ]; then
        step "removing the certificate for $cert"
        drop_certificate "$cert"
    fi
    if [ "$drop_bot" -eq 1 ]; then
        step "removing certbot"
        drop_certbot
    fi

    step "what is left"
    left "service"  "$(yesno systemctl is-active --quiet "$SERVICE")"
    left "unit"     "$(yesno test -f "$UNIT")"
    left "$PREFIX"  "$(yesno test -d "$PREFIX")"
    left "$STATE"   "$(yesno test -d "$STATE")"
    left "$(dirname "$CONF")" "$(yesno test -d "$(dirname "$CONF")")"
    if [ "$drop_cert" -eq 1 ]; then left "certificate $cert" "$(yesno test -d "$LE/live/$cert")"; fi
    if [ "$drop_bot" -eq 1 ]; then left "certbot" "$(yesno certbot_held)"; fi
    if [ -n "$copy" ]; then say "kept: $copy"; fi
    printf "\n" >&2
}

banner
require_root
require_tools
choose_mode

case "$MODE" in
    install)
        if anything_here; then die "something is already here, pass --mode update|fresh|over"; fi
        do_install ;;
    update)    do_update ;;
    fresh)     do_fresh ;;
    over)      do_over ;;
    uninstall) do_uninstall ;;
    *)         die "unknown mode: $MODE" ;;
esac
