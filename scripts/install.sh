#!/bin/sh
# Miru installer for OpenWrt and Linux
# Run on your router or server:
#   wget -O /tmp/install.sh https://raw.githubusercontent.com/paintingpromisesss/miru/main/scripts/install.sh && sh /tmp/install.sh
# or:
#   curl -fsSL https://raw.githubusercontent.com/paintingpromisesss/miru/main/scripts/install.sh | sudo sh

set -e

REPO="paintingpromisesss/miru"
GITHUB_RAW="https://raw.githubusercontent.com/${REPO}/main"

if [ -n "$MIRU_VERSION" ] && [ "$MIRU_VERSION" != "latest" ]; then
    case "$MIRU_VERSION" in
        v*) ;;
        *)  MIRU_VERSION="v${MIRU_VERSION}" ;;
    esac
    GITHUB_RELEASES="https://github.com/${REPO}/releases/download/${MIRU_VERSION}"
    INSTALL_VERSION="$MIRU_VERSION"
else
    GITHUB_RELEASES="https://github.com/${REPO}/releases/latest/download"
    INSTALL_VERSION="latest"
fi

BIN_DST="/usr/bin/miru"
INIT_DST="/etc/init.d/miru"
SYSTEMD_SERVICE="/etc/systemd/system/miru.service"
CONFIG_DIR="/etc/miru"
CONFIG_FILE="${CONFIG_DIR}/miru.env"

ESC=$(printf '\033')
YEL="${ESC}[1;33m"
GRY="${ESC}[0;37m"
DIM="${ESC}[2;37m"
RED="${ESC}[0;31m"
GRN="${ESC}[0;32m"
NC="${ESC}[0m"

sep()   { printf "%b──────────────────────────────────────────────────%b\n" "$DIM" "$NC"; }
hdr()   { printf "\n%b  %s%b\n" "$YEL" "$1" "$NC"; sep; }
info()  { printf "%b    %s%b\n" "$GRY" "$1" "$NC"; }
ok()    { printf "%b  ✓ %s%b\n" "$GRN" "$1" "$NC"; }
warn()  { printf "%b  ! %s%b\n" "$YEL" "$1" "$NC"; }
error() { printf "%b  ✗ %s%b\n" "$RED" "$1" "$NC"; exit 1; }
ask()   { printf "%b    %s%b " "$YEL" "$1" "$NC"; }

read_input() {
    if [ -t 0 ]; then
        read -r "$1"
    elif [ -e /dev/tty ]; then
        read -r "$1" </dev/tty
    else
        read -r "$1" || true
    fi
}

check_root() {
    if [ "$(id -u 2>/dev/null || true)" != "0" ]; then
        error "This script must be run as root. Please run with sudo or as root."
    fi
}

detect_init() {
    if [ -f /etc/openwrt_release ] || [ -f /etc/init.d/rc.common ]; then
        INIT_SYSTEM="procd"
    elif command -v systemctl >/dev/null 2>&1 && [ -d /run/systemd/system ]; then
        INIT_SYSTEM="systemd"
    else
        INIT_SYSTEM="unknown"
    fi
}

detect_arch() {
    case "$(uname -m)" in
        aarch64|arm64)  echo "linux-arm64"  ;;
        armv7l|armv6l)  echo "linux-arm"    ;;
        x86_64|amd64)   echo "linux-amd64"  ;;
        mips)           echo "linux-mips"   ;;
        mipsel|mipsle)  echo "linux-mipsle" ;;
        *)
            error "Unsupported architecture: $(uname -m). Please build manually from source."
            ;;
    esac
}

check_deps() {
    if ! command -v wget >/dev/null 2>&1 && ! command -v curl >/dev/null 2>&1; then
        error "Neither wget nor curl is installed. Please install one of them."
    fi
    for cmd in chmod mkdir killall; do
        if ! command -v "$cmd" >/dev/null 2>&1; then
            [ "$cmd" = "killall" ] && continue
            error "Missing required utility: $cmd"
        fi
    done
}

download() {
    URL="$1"; DST="$2"
    info "Fetching $(basename "$DST") ..."
    if command -v wget >/dev/null 2>&1; then
        wget -q -O "$DST" "$URL" || error "Download failed: $URL"
    elif command -v curl >/dev/null 2>&1; then
        curl -fsSL -o "$DST" "$URL" || error "Download failed: $URL"
    fi
}

configure_settings() {
    mkdir -p "$CONFIG_DIR"

    DEF_CFG="/etc/mihomo/config.yaml"
    DEF_RULES="/etc/mihomo/rules"
    DEF_PORT="8080"
    DEF_API="http://127.0.0.1:9090"
    DEF_SECRET=""
    DEF_GITHUB_TOKEN=""

    if [ -f "$CONFIG_FILE" ]; then
        info "Found existing configuration in $CONFIG_FILE"
        . "$CONFIG_FILE"
        DEF_CFG="${MIRU_CONFIG:-$DEF_CFG}"
        DEF_RULES="${MIRU_RULES_DIR:-$DEF_RULES}"
        DEF_PORT="${MIRU_PORT:-$DEF_PORT}"
        DEF_API="${MIRU_MIHOMO_API:-$DEF_API}"
        DEF_SECRET="${MIRU_MIHOMO_SECRET:-$DEF_SECRET}"
        DEF_GITHUB_TOKEN="${MIRU_GITHUB_TOKEN:-$DEF_GITHUB_TOKEN}"
    fi

    if [ -t 0 ] || [ -e /dev/tty ]; then
        hdr "Configuration Setup"

        ask "Mihomo config.yaml path [${DEF_CFG}]:"
        read_input IN_CFG
        [ -n "$IN_CFG" ] && DEF_CFG="$IN_CFG"

        ask "Rules storage directory [${DEF_RULES}]:"
        read_input IN_RULES
        [ -n "$IN_RULES" ] && DEF_RULES="$IN_RULES"

        ask "Miru Web UI listen port [${DEF_PORT}]:"
        read_input IN_PORT
        [ -n "$IN_PORT" ] && DEF_PORT="$IN_PORT"

        ask "Mihomo API controller URL [${DEF_API}]:"
        read_input IN_API
        [ -n "$IN_API" ] && DEF_API="$IN_API"
    fi

    mkdir -p "$DEF_RULES"

    cat > "$CONFIG_FILE" << EOF
# Miru configuration environment
# Updated: $(date -Iseconds 2>/dev/null || date)

MIRU_CONFIG=${DEF_CFG}
MIRU_RULES_DIR=${DEF_RULES}
MIRU_PORT=${DEF_PORT}
MIRU_MIHOMO_API=${DEF_API}
MIRU_MIHOMO_SECRET=${DEF_SECRET}
MIRU_GITHUB_TOKEN=${DEF_GITHUB_TOKEN}
EOF

    chmod 600 "$CONFIG_FILE"
    ok "Configuration saved to $CONFIG_FILE"
    MIRU_ACTIVE_PORT="$DEF_PORT"
}

install_service() {
    SCRIPT_DIR="$(dirname "$0" 2>/dev/null || true)"

    if [ "$INIT_SYSTEM" = "procd" ]; then
        if [ -f "${SCRIPT_DIR}/miru.init" ]; then
            cp "${SCRIPT_DIR}/miru.init" "$INIT_DST"
        else
            download "${GITHUB_RAW}/scripts/miru.init" "$INIT_DST"
        fi
        chmod +x "$INIT_DST"
        "$INIT_DST" enable >/dev/null 2>&1 || true
        ok "Installed OpenWrt procd init script to $INIT_DST"
    elif [ "$INIT_SYSTEM" = "systemd" ]; then
        if [ -f "${SCRIPT_DIR}/miru.service" ]; then
            cp "${SCRIPT_DIR}/miru.service" "$SYSTEMD_SERVICE"
        else
            download "${GITHUB_RAW}/scripts/miru.service" "$SYSTEMD_SERVICE"
        fi
        systemctl daemon-reload
        systemctl enable miru >/dev/null 2>&1 || true
        ok "Installed systemd unit to $SYSTEMD_SERVICE"
    else
        warn "Unknown init system. Binary installed to $BIN_DST, please start manually."
    fi
}

start_service() {
    hdr "Starting Miru service"
    if [ "$INIT_SYSTEM" = "procd" ]; then
        "$INIT_DST" restart >/dev/null 2>&1 || "$INIT_DST" start >/dev/null 2>&1 || true
    elif [ "$INIT_SYSTEM" = "systemd" ]; then
        systemctl restart miru >/dev/null 2>&1 || systemctl start miru >/dev/null 2>&1 || true
    fi
    sleep 2

    PORT="${MIRU_ACTIVE_PORT:-8080}"
    HEALTH_OK=0
    if command -v curl >/dev/null 2>&1; then
        curl -fsS -o /dev/null "http://127.0.0.1:${PORT}/api/local" 2>/dev/null && HEALTH_OK=1 || true
    elif command -v wget >/dev/null 2>&1; then
        wget -q -O /dev/null "http://127.0.0.1:${PORT}/api/local" 2>/dev/null && HEALTH_OK=1 || true
    fi

    if [ "$HEALTH_OK" = "1" ]; then
        ok "Miru service is running and healthy!"
    else
        warn "Service started, checking health on http://127.0.0.1:${PORT}..."
    fi
    return 0
}

detect_ip() {
    ROUTER_IP=""
    if command -v uci >/dev/null 2>&1; then
        ROUTER_IP="$(uci -q get network.lan.ipaddr 2>/dev/null || true)"
    fi
    if [ -z "$ROUTER_IP" ] && command -v ip >/dev/null 2>&1; then
        ROUTER_IP="$(ip -4 addr show br-lan 2>/dev/null | awk '/inet /{print $2}' | head -n1 || true)"
        if [ -z "$ROUTER_IP" ]; then
            ROUTER_IP="$(ip -4 addr show lan 2>/dev/null | awk '/inet /{print $2}' | head -n1 || true)"
        fi
        if [ -z "$ROUTER_IP" ]; then
            ROUTER_IP="$(ip route get 1.1.1.1 2>/dev/null | awk '{for(i=1;i<=NF;i++) if($i=="src") print $(i+1)}' || true)"
        fi
    fi
    if [ -z "$ROUTER_IP" ] && command -v hostname >/dev/null 2>&1; then
        ROUTER_IP="$(hostname -I 2>/dev/null | awk '{print $1}' || true)"
    fi
    ROUTER_IP="${ROUTER_IP%% *}"
    ROUTER_IP="${ROUTER_IP%%/*}"
    ROUTER_IP="$(echo "$ROUTER_IP" | tr -d ' \r\n')"
    [ -z "$ROUTER_IP" ] && ROUTER_IP="127.0.0.1"
    return 0
}

main() {
    check_root
    check_deps
    detect_init

    printf "\n%b==================================================%b\n" "$YEL" "$NC"
    printf "%b  Miru — Mihomo Rule Manager Installer%b\n" "$YEL" "$NC"
    printf "%b==================================================%b\n" "$YEL" "$NC"

    ARCH=$(detect_arch)
    if [ "$MIRU_UPX" = "1" ] || [ "$MIRU_UPX" = "true" ]; then
        ARCH="${ARCH}-upx"
    fi

    info "Platform: $ARCH | Init: $INIT_SYSTEM | Version: $INSTALL_VERSION"

    if [ "$INIT_SYSTEM" = "procd" ] && [ -x "$INIT_DST" ]; then
        "$INIT_DST" stop >/dev/null 2>&1 || true
    elif [ "$INIT_SYSTEM" = "systemd" ] && systemctl is-active --quiet miru 2>/dev/null; then
        systemctl stop miru >/dev/null 2>&1 || true
    fi

    hdr "Downloading binary"
    BIN_NAME="miru-${ARCH}"
    BIN_URL="${GITHUB_RELEASES}/${BIN_NAME}"
    TMP_BIN="/tmp/miru.tmp.$$"

    download "$BIN_URL" "$TMP_BIN"
    chmod +x "$TMP_BIN"
    mv "$TMP_BIN" "$BIN_DST"
    ok "Binary installed to $BIN_DST"

    configure_settings
    install_service
    start_service
    detect_ip

    PORT="${MIRU_ACTIVE_PORT:-8080}"
    printf "\n%b==================================================%b\n" "$YEL" "$NC"
    printf "%b  Done! Miru Web UI is ready:%b\n" "$GRN" "$NC"
    printf "%b==================================================%b\n\n" "$YEL" "$NC"
    printf "  Web UI:       %bhttp://${ROUTER_IP}:${PORT}%b\n" "$YEL" "$NC"
    printf "  Localhost:    %bhttp://127.0.0.1:${PORT}%b\n" "$DIM" "$NC"
    printf "  Config file:  %b/etc/miru/miru.env%b\n\n" "$GRY" "$NC"

    if [ "$INIT_SYSTEM" = "procd" ]; then
        printf "  Service logs: %blogread -f -e miru%b\n" "$DIM" "$NC"
        printf "  Restart:      %b/etc/init.d/miru restart%b\n\n" "$DIM" "$NC"
    elif [ "$INIT_SYSTEM" = "systemd" ]; then
        printf "  Service logs: %bjournalctl -u miru -f%b\n" "$DIM" "$NC"
        printf "  Restart:      %bsystemctl restart miru%b\n\n" "$DIM" "$NC"
    fi
}

main "$@"
