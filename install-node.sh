#!/usr/bin/env bash
set -Eeuo pipefail

REPO="jaywehosl/qd"
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

PORT=443
DOMAIN=""
EMAIL=""
ADMIN_TAG=""
ADMIN_UUID=""
GROUP_TAG=""
ADDRESS=""
NETWORK_KEY=""
NODE_TAG=""
NODE_UUID=""
NODE_ID=""
ROLE="ingress"
AUTHORITY=""
CERT=""
KEYFILE=""
WANT_CERT=1
CERT_ISSUED=0
UNDO=()
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
rule() { printf "  ${DIM}%s${OFF}\n" "--------------------------------------------------------" >&2; }

banner() {
    printf "\n  ${BOLD}qd${OFF} ${DIM}node installer${OFF}\n" >&2
    rule
}

usage() {
    cat <<'USAGE'
qd node installer.

Run it with no arguments to start a new network: it asks for what it needs.
Run it with --key and the rest to join an existing one: it asks nothing and
writes exactly what the panel handed it.

  --mode install|update|fresh|over|uninstall
                                     what to do when something is already here
  --uninstall                        short for --mode uninstall
  --version vX.Y.Z                   release to install, default: latest
  --from DIR                         install from files already on this machine
                                     instead of downloading them
  --domain NAME                      domain clients dial and the certificate is
                                     issued for; --address and --host mean the same
  --email ADDR                       e-mail Let's Encrypt registers against
  --no-cert                          skip certbot, stand up self-signed
  --port N                           udp port
  --dns1 A --dns2 B                  upstream resolvers
  --dns-cache N --dns-min-ttl N --dns-max-ttl N --dns-stale N
  --yes                              never ask, requires --mode on an existing install
  --help

Starting a network, asked when absent:
  --admin TAG --group TAG            the first administrator and the first group

Joining a network, every value comes from the panel:
  --key HEX                          the network key
  --authority HOST:PORT              what this node answers as, default: domain:port
                                     the certificate is issued by this script itself
  --tag NAME --role ingress|egress   what this node is
  --node-uuid UUID --node-id N       identity and number of this node
  --admin-uuid UUID --admin TAG      the administrator that may reach it

Modes when something is already installed:
  update    keep the database, swap binary and object, roll back if it will not start
  fresh     back the database up, remove everything, build a new network
  over      overwrite the files, leave the database alone
  uninstall remove the node, its database and its unit
USAGE
}

while [ $# -gt 0 ]; do
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

# ---------------------------------------------------------------- environment

require_root() { [ "$(id -u)" -eq 0 ] || die "run this as root"; }

require_tools() {
    if ! command -v curl >/dev/null 2>&1; then
        if command -v apt-get >/dev/null 2>&1; then
            say "curl is missing, installing it"
            apt-get update -qq && apt-get install -y -qq curl
        else
            die "curl is missing and this is not an apt system, install curl and run again"
        fi
    fi
    command -v systemctl >/dev/null 2>&1 || die "no systemd here, this installer needs it"
    [ "$(uname -m)" = "x86_64" ] || die "the release carries linux/amd64 only, this is $(uname -m)"
}

# ---------------------------------------------------------------- the machine

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

# resolve_records печатает адреса, на которые указывает домен: A, а следом AAAA.
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

# check_domain держит одно правило: сертификат выпишут только на имя, которое
# указывает сюда. Проверяем это до certbot, иначе он упрётся сам и оставит за
# собой мусор в /etc/letsencrypt.
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

# check_ports смотрит на то, что нам нужно занять: udp — сам туннель, tcp —
# сайт-прикрытие, 80 — certbot на время выпуска. Занятый порт лучше назвать
# сейчас, чем узнать о нём из журнала после установки.
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

# ---------------------------------------------------------------- certificate

certbot_paths() {
    CERT="/etc/letsencrypt/live/$1/fullchain.pem"
    KEYFILE="/etc/letsencrypt/live/$1/privkey.pem"
}

have_certificate() {
    [ -f "/etc/letsencrypt/live/$1/fullchain.pem" ] && [ -f "/etc/letsencrypt/live/$1/privkey.pem" ]
}

# issue_certificate: сперва вхолостую, потом всерьёз. Холостой заход ловит
# упавший DNS и закрытый 80 до того, как Let's Encrypt посчитает попытки.
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

    # Обновление certbot делает сам таймером; узлу остаётся перечитать файлы,
    # а перечитывает он их только при старте.
    mkdir -p /etc/letsencrypt/renewal-hooks/deploy
    cat >/etc/letsencrypt/renewal-hooks/deploy/qd-node.sh <<'HOOK'
#!/bin/sh
systemctl restart qd-node 2>/dev/null || true
HOOK
    chmod 0755 /etc/letsencrypt/renewal-hooks/deploy/qd-node.sh
    good "renewal restarts the node"
}

# ---------------------------------------------------------------- release

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

    if curl -fsSL -o "$STAGE/checksums.txt" "$base/checksums.txt" 2>/dev/null; then
        if ( cd "$STAGE" && sed 's/qd-node-linux-amd64/qd-node/' checksums.txt | grep -E '[ *]qd-node$' | sha256sum -c --status - ); then
            good "checksums verified"
        else
            die "checksum mismatch, refusing to install"
        fi
    else
        warn "this release carries no checksums.txt, installing unverified"
    fi
    chmod 0755 "$STAGE/qd-node"
}

# ---------------------------------------------------------------- state

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
    command -v ss >/dev/null 2>&1 || return 0
    ss -lunp 2>/dev/null | awk '/qd-node/ {n=split($4,a,":"); print a[n]; exit}'
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

  1  update  keep the database and the network, swap binary and object only.
             Rewrites the unit and rolls back by itself if the pair will not start.
  2  fresh   back the database up, remove unit and files, build a NEW network.
             The current network key and every client on it stop working here.
  3  over    overwrite binary and object, leave the database alone.
  4  remove  take qd off this machine entirely: unit, files, database.
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

# ---------------------------------------------------------------- install

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

[Install]
WantedBy=multi-user.target
UNITFILE
    systemctl daemon-reload
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
    return 0
}


# verify_live проверяет не «процесс запустился», а три факта, по которым клиент
# и решает, живой ли узел: слушает udp, отдаёт сайт-прикрытие по tcp и отвечает
# на управляющий запрос. Первое без второго значит, что клиент упрётся в TLS.
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

# Всё, что не спрошено здесь, приезжает дефолтами базы и правится в панели. Тут
# остаётся только то, чего база знать не может: имя этой машины, кому писать о
# сертификате, и как зовут первого администратора.
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
        -role "$ROLE" -tag "$NODE_TAG" -node-uuid "$NODE_UUID" -node-id "${NODE_ID:-0}" \
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

# ---------------------------------------------------------------- flows

do_install() {
    ask_install_questions
    check_domain "$DOMAIN"
    check_ports

    local tag; tag="$(resolve_version)"
    fetch_release "$tag"

    if [ "$WANT_CERT" -eq 1 ]; then
        issue_certificate "$DOMAIN"
    else
        say "no certificate asked for, the node stands up self-signed"
    fi
    [ -n "$AUTHORITY" ] || AUTHORITY="$DOMAIN:$PORT"

    local out; out="$(initialise_database)"
    install_files
    write_unit
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

do_uninstall() {
    if ! anything_here; then
        step "nothing to remove"
        say "no unit, no files and no database of qd on this machine"
        return 0
    fi

    report_state
    confirm_uninstall

    local port
    port="$(running_port)"

    step "stopping the node"
    systemctl disable --now "$SERVICE" >/dev/null 2>&1 || true
    sleep 2
    rm -f "$UNIT"
    systemctl daemon-reload >/dev/null 2>&1 || true

    step "removing files"
    rm -rf "$PREFIX" "$STATE" "$(dirname "$CONF")"

    if [ -n "$port" ] && command -v ufw >/dev/null 2>&1; then
        if ufw --force delete allow "$port/udp" >/dev/null 2>&1; then
            say "ufw: closed $port/udp"
        fi
    fi

    step "what is left"
    left "service"  "$(yesno systemctl is-active --quiet "$SERVICE")"
    left "unit"     "$(yesno test -f "$UNIT")"
    left "$PREFIX"  "$(yesno test -d "$PREFIX")"
    left "$STATE"   "$(yesno test -d "$STATE")"
    printf "\n" >&2
}

# ---------------------------------------------------------------- main

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
