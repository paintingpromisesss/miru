#!/bin/sh
# Install/update Miru. Never edits Nikki/Mihomo configuration.
set -eu
umask 077
REPO=paintingpromisesss/miru
BIN=/usr/bin/miru
CONF=/etc/miru/miru.env
ASSETS="${MIRU_ASSET_DIR:-}"
VERSION="${MIRU_VERSION:-latest}"
# Keep output readable when redirected or used by an automation.
if [ -t 1 ] && [ "${TERM:-dumb}" != dumb ]; then
    C=$(printf '\033'); B="$C[1m"; G="$C[32m"; Y="$C[33m"; R="$C[31m"; N="$C[0m"
else B= G= Y= R= N=; fi
section() { printf '\n%b%s%b\n%s\n' "$B" "$*" "$N" '--------------------------------------------'; }
info() { printf '  %s\n' "$*"; }
ok() { printf '  %b[OK]%b %s\n' "$G" "$N" "$*"; }
warn() { printf '  %b[!]%b %s\n' "$Y" "$N" "$*" >&2; }
fail() { printf '  %b[ERROR]%b %s\n' "$R" "$N" "$*" >&2; exit 1; }
YES=false
CONFIGURE=false
while [ "$#" -gt 0 ]; do
    case "$1" in
        --yes|-y) YES=true;;
        --configure) CONFIGURE=true;;
        --help|-h)
            printf '%s\n' 'Usage: sh install.sh [--yes] [--configure]' '  --yes        Non-interactive installation (environment/defaults)' '  --configure  Prompt again for settings, even on upgrades' '  MIRU_VERSION, MIRU_UPX, MIRU_ASSET_DIR and MIRU_* settings are supported.'
            exit 0;;
        *) fail "Unknown option: $1";;
    esac
    shift
done
INTERACTIVE=false
if [ "$YES" = false ] && ( : </dev/tty ) 2>/dev/null; then INTERACTIVE=true; fi
if [ "$CONFIGURE" = false ] && [ -n "${MIRU_BIND:-}${MIRU_PASSWORD:-}${MIRU_NO_AUTH:-}${MIRU_MODE:-}${MIRU_UPX:-}" ]; then INTERACTIVE=false; fi
prompt() {
    printf '  %s [%s]: ' "$1" "$2" >/dev/tty
    IFS= read -r ANSWER </dev/tty || fail "Input interrupted"
    ANSWER=${ANSWER:-$2}
}
choose() {
    question=$1; default=$2; shift 2
    printf '\n  %s\n' "$question" >/dev/tty
    for choice in "$@"; do printf '    %s\n' "$choice" >/dev/tty; done
    while :; do
        prompt "Choice" "$default"
        for choice in "$@"; do
            if [ "$ANSWER" = "${choice%%.*}" ]; then return 0; fi
        done
        warn "Choose one of the numbers shown."
    done
}
valid_port() {
    case "$1" in ''|*[!0-9]*|??????*) return 1;; esac
    [ "$1" -ge 1 ] && [ "$1" -le 65535 ]
}
valid_bind() {
    # Accept a literal IPv4/IPv6 address or local hostname; reject URL syntax.
    case "$1" in ''|*[!a-zA-Z0-9_.:-]*) return 1;; esac
}
prompt_password() {
    while :; do
        if command -v stty >/dev/null 2>&1 && old_stty=$(stty -g </dev/tty 2>/dev/null) && stty -echo </dev/tty; then
            trap 'stty "$old_stty" </dev/tty; exit 1' HUP INT TERM
            printf '  Password (hidden): ' >/dev/tty
            IFS= read -r ANSWER </dev/tty || ANSWER=
            stty "$old_stty" </dev/tty
            trap 'exit 1' HUP INT TERM
            printf '\n' >/dev/tty
        elif ( printf 'probe\n' | (IFS= read -r -s probe) ) 2>/dev/null; then
            printf '  Password (hidden): ' >/dev/tty
            IFS= read -r -s ANSWER </dev/tty || ANSWER=
            printf '\n' >/dev/tty
        else
            warn "Hidden input is unavailable."
            choose "Password entry" 2 "1. Enter visibly" "2. Return to authentication menu"
            [ "$ANSWER" = 1 ] || return 1
            printf '  Password (visible): ' >/dev/tty
            IFS= read -r ANSWER </dev/tty || ANSWER=
        fi
        [ -n "$ANSWER" ] && return 0
        warn "Password cannot be empty."
    done
}
section "Miru installer"

[ "$(id -u)" = 0 ] || fail "Run as root"
command -v sha256sum >/dev/null || fail "sha256sum required"
if [ -f /etc/openwrt_release ]; then
    INIT=procd; SERVICE=/etc/init.d/miru; SERVICE_ASSET=miru.init
elif command -v systemctl >/dev/null && [ -d /run/systemd/system ]; then
    INIT=systemd; SERVICE=/etc/systemd/system/miru.service; SERVICE_ASSET=miru.service
else fail "Supported init systems: procd and systemd"; fi
case "$(uname -m)" in
    aarch64|arm64) ARCH=arm64;; armv7l) ARCH=arm;;
    x86_64|amd64) ARCH=amd64;; mips) ARCH=mips;; mipsel|mipsle) ARCH=mipsle;;
    *) fail "Unsupported architecture: $(uname -m)";;
esac
STAGE=$(mktemp -d /tmp/miru-install.XXXXXX)
trap 'rm -rf "$STAGE"' EXIT
trap 'exit 1' HUP INT TERM
download() {
    info "Fetching $1 ..."
    if [ -n "$ASSETS" ]; then cp "$ASSETS/$1" "$2"
    elif command -v curl >/dev/null; then curl -fLsS --connect-timeout 15 --max-time 180 "$BASE/$1" -o "$2"
    else wget -O "$2" "$BASE/$1"; fi
}
if [ "$VERSION" = latest ]; then
    BASE=https://github.com/$REPO/releases/latest/download
    download VERSION "$STAGE/VERSION"
    VERSION=$(cat "$STAGE/VERSION")
fi
printf '%s\n' "$VERSION" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+$' || fail "Invalid release version"
BASE=https://github.com/$REPO/releases/download/$VERSION
section "1 / 4  Setup"
info "Version: $VERSION | Architecture: $ARCH | Service: $INIT"

REQUESTED_UPX="${MIRU_UPX:-}"
REQUESTED_MODE="${MIRU_MODE:-}"
LEGACY_MIGRATION=false
EXISTING=false
if [ -f "$CONF" ]; then
    EXISTING=true
    . "$CONF"
    info "Existing settings loaded."
fi
if [ -n "$REQUESTED_UPX" ]; then MIRU_UPX=$REQUESTED_UPX; fi
NIKKI=false
if [ -x /etc/init.d/nikki ] && [ -f /etc/nikki/mixin.yaml ]; then NIKKI=true; fi
if [ -z "${MIRU_MODE:-}" ]; then
    if [ "$NIKKI" = true ] && { [ -z "${MIRU_CONFIG:-}" ] || [ "$MIRU_CONFIG" = /etc/nikki/mixin.yaml ] || [ ! -f "$MIRU_CONFIG" ]; }; then
        MIRU_MODE=nikki
        if [ "$EXISTING" = true ] && [ "${MIRU_CONFIG:-}" != /etc/nikki/mixin.yaml ]; then
            LEGACY_MIGRATION=true
            warn "Migrating legacy settings to Nikki."
            unset MIRU_CONFIG MIRU_RULES_DIR MIRU_GITHUB_PROXY MIRU_MIHOMO_SECRET
            MIRU_PORT=8787
        fi
    else MIRU_MODE=standalone; fi
fi
LAN=
if [ "$INIT" = procd ]; then
    LAN=$(uci -q get network.lan.ipaddr 2>/dev/null || true)
    LAN=${LAN%% *}; LAN=${LAN%%/*}
fi
EDIT=$CONFIGURE
if [ "$INTERACTIVE" = true ]; then
    if [ "$EXISTING" = true ] && [ "$LEGACY_MIGRATION" = false ] && [ "$CONFIGURE" = false ]; then
        info "Current setup: $MIRU_MODE | ${MIRU_BIND:-127.0.0.1}:${MIRU_PORT:-8787}"
        choose "Installation action" 1 "1. Update (keep settings)" "2. Reconfigure" "0. Cancel"
        case "$ANSWER" in 1) EDIT=false;; 2) EDIT=true;; 0) exit 0;; esac
    else EDIT=true; fi
fi
# Retain the installed variant, unless a user explicitly selects another.
case "${MIRU_UPX:-0}" in 0|false) MIRU_UPX=0;; 1|true) MIRU_UPX=1;; *) fail "Invalid MIRU_UPX";; esac
if [ "$INTERACTIVE" = true ]; then
    variant_default=1; [ "$MIRU_UPX" = 0 ] || variant_default=2
    choose "Binary variant" "$variant_default" "1. Standard (recommended; faster startup)" "2. UPX compressed (less flash storage)" "0. Cancel"
    case "$ANSWER" in 1) MIRU_UPX=0;; 2) MIRU_UPX=1;; 0) exit 0;; esac
fi
NAME=miru-linux-$ARCH
VARIANT=Standard
if [ "$MIRU_UPX" = 1 ]; then NAME=$NAME-upx; VARIANT=UPX; fi

mode_defaults() {
    case "$MIRU_MODE" in
        nikki)
            : "${MIRU_CONFIG:=/etc/nikki/mixin.yaml}"
            : "${MIRU_RULES_DIR:=/etc/nikki/run/ruleset}"
            : "${MIRU_GITHUB_PROXY:=none}";;
        standalone)
            : "${MIRU_CONFIG:=/etc/mihomo/config.yaml}"
            : "${MIRU_RULES_DIR:=/etc/mihomo/rules}"
            : "${MIRU_GITHUB_PROXY:=auto}";;
        *) fail "Invalid MIRU_MODE";;
    esac
}
mode_defaults
: "${MIRU_PORT:=8787}"
: "${MIRU_BIND:=127.0.0.1}"
: "${MIRU_MIHOMO_API:=http://127.0.0.1:9090}"
: "${MIRU_MIHOMO_SECRET:=}"
: "${MIRU_GITHUB_TOKEN:=}"
: "${MIRU_PASSWORD:=}"
: "${MIRU_NO_AUTH:=false}"
case "$MIRU_NO_AUTH" in 1|true) MIRU_NO_AUTH=true;; 0|false) MIRU_NO_AUTH=false;; *) fail "Invalid MIRU_NO_AUTH";; esac

if [ "$INTERACTIVE" = true ] && [ "$EDIT" = true ]; then
    while :; do
        mode_default=2; [ "$MIRU_MODE" != nikki ] || mode_default=1
        choose "Operating mode" "$mode_default" "1. Nikki" "2. Standalone Mihomo" "0. Cancel"
        case "$ANSWER" in
            1)
                if [ "$NIKKI" != true ]; then warn "Nikki is not installed."; continue; fi
                if [ "$(uci -q get nikki.mixin.mixin_file_content || true)" != 1 ]; then warn "Enable mixin file content in Nikki first."; continue; fi
                selected=nikki;;
            2) selected=standalone;;
            0) exit 0;;
        esac
        if [ "$selected" != "$MIRU_MODE" ]; then
            unset MIRU_CONFIG MIRU_RULES_DIR MIRU_GITHUB_PROXY MIRU_MIHOMO_SECRET
            MIRU_MIHOMO_SECRET=
        fi
        MIRU_MODE=$selected; mode_defaults
        break
    done
    if [ "$MIRU_MODE" = standalone ]; then
        while :; do
            prompt "Mihomo configuration file" "$MIRU_CONFIG"
            if [ -f "$ANSWER" ]; then MIRU_CONFIG=$ANSWER; break; fi
            warn "File not found: $ANSWER"
        done
    fi
    while :; do
        prompt "Web UI port" "$MIRU_PORT"
        if valid_port "$ANSWER"; then MIRU_PORT=$ANSWER; break; fi
        warn "Enter a port from 1 to 65535."
    done
    bind_default=3
    if [ "$MIRU_BIND" = 127.0.0.1 ] || [ "$MIRU_BIND" = ::1 ]; then bind_default=2; fi
    if [ -n "$LAN" ]; then
        if [ "$MIRU_BIND" = "$LAN" ] || [ "$EXISTING" = false ] || [ "$LEGACY_MIGRATION" = true ]; then bind_default=1; fi
        choose "Web UI access" "$bind_default" "1. Local network ($LAN)" "2. This device only (127.0.0.1)" "3. Custom address"
    else
        [ "$bind_default" != 1 ] || bind_default=2
        choose "Web UI access" "$bind_default" "2. This device only (127.0.0.1)" "3. Custom address"
    fi
    case "$ANSWER" in
        1) MIRU_BIND=$LAN;;
        2) MIRU_BIND=127.0.0.1;;
        3) while :; do prompt "Listen address" "$MIRU_BIND"; if valid_bind "$ANSWER"; then MIRU_BIND=$ANSWER; break; fi; warn "Enter an address, not a URL."; done;;
    esac
    while :; do
        auth_default=1
        [ "$MIRU_NO_AUTH" != true ] || auth_default=2
        choose "Authentication" "$auth_default" "1. Password (recommended)" "2. No password (trusted network only)" "0. Cancel"
        case "$ANSWER" in
            1)
                if [ -n "$MIRU_PASSWORD" ]; then
                    choose "Password" 1 "1. Keep existing password" "2. Set a new password"
                    if [ "$ANSWER" = 2 ]; then prompt_password || continue; MIRU_PASSWORD=$ANSWER; fi
                else prompt_password || continue; MIRU_PASSWORD=$ANSWER; fi
                MIRU_NO_AUTH=false; break;;
            2)
                warn "Anyone with network access to Miru can modify routing."
                choose "Allow unauthenticated access?" 2 "1. Allow" "2. Back"
                if [ "$ANSWER" = 1 ]; then MIRU_NO_AUTH=true; MIRU_PASSWORD=; break; fi;;
            0) exit 0;;
        esac
    done
    choose "Configuration paths and controller" 1 "1. Use current settings (recommended)" "2. Advanced settings"
    if [ "$ANSWER" = 2 ]; then
        while :; do
            prompt "Editable YAML file" "$MIRU_CONFIG"
            if [ -f "$ANSWER" ]; then MIRU_CONFIG=$ANSWER; break; fi
            warn "File not found: $ANSWER"
        done
        prompt "Rules cache directory" "$MIRU_RULES_DIR"; MIRU_RULES_DIR=$ANSWER
        while :; do
            prompt "Mihomo controller URL" "$MIRU_MIHOMO_API"
            case "$ANSWER" in http://?*|https://?*) MIRU_MIHOMO_API=$ANSWER; break;; *) warn "Enter an http:// or https:// URL.";; esac
        done
        prompt "GitHub proxy (auto / none / URL)" "$MIRU_GITHUB_PROXY"; MIRU_GITHUB_PROXY=$ANSWER
    fi
fi
if [ "$MIRU_MODE" = nikki ]; then
    [ "$NIKKI" = true ] || fail "Nikki is not installed"
    [ "$(uci -q get nikki.mixin.mixin_file_content || true)" = 1 ] || fail "Enable Nikki mixin file content first"
fi
if [ "$MIRU_BIND" != 127.0.0.1 ] && [ "$MIRU_BIND" != ::1 ] && [ -z "$MIRU_PASSWORD" ] && [ "$MIRU_NO_AUTH" = false ]; then
    fail "Set MIRU_PASSWORD or explicitly MIRU_NO_AUTH=1 for LAN access"
fi
valid_port "$MIRU_PORT" || fail "Invalid port"
valid_bind "$MIRU_BIND" || fail "Invalid listen address"
[ -f "$MIRU_CONFIG" ] || fail "Missing config: $MIRU_CONFIG (run with --configure)"
printf '\n  %-19s %s\n' 'Mode' "$MIRU_MODE" 'Binary variant' "$VARIANT" 'Config' "$MIRU_CONFIG" 'Rules cache' "$MIRU_RULES_DIR" 'Listen' "$MIRU_BIND:$MIRU_PORT" 'Controller' "$MIRU_MIHOMO_API"
if [ "$MIRU_NO_AUTH" = true ]; then info "Authentication: disabled"
elif [ -n "$MIRU_PASSWORD" ]; then info "Authentication: password (user miru)"
else info "Authentication: local only"; fi
if [ "$INTERACTIVE" = true ]; then
    choose "Proceed with installation?" 1 "1. Install" "0. Cancel"
    [ "$ANSWER" = 1 ] || exit 0
fi

section "2 / 4  Download and verify"
download SHA256SUMS "$STAGE/SHA256SUMS"
for file in "$NAME" "$SERVICE_ASSET"; do
    awk -v f="$file" '$2 == f {found=1} END {exit !found}' "$STAGE/SHA256SUMS" || fail "Release asset unavailable: $file"
    download "$file" "$STAGE/$file"
    (cd "$STAGE"; awk -v f="$file" '$2 == f {print}' SHA256SUMS | sha256sum -c -) || fail "Checksum mismatch: $file"
done
chmod 755 "$STAGE/$NAME"
[ "$("$STAGE/$NAME" -version)" = "$VERSION" ] || fail "Downloaded binary version mismatch or unsupported executable"
ok "Release checksums and binary version verified"
export MIRU_MODE MIRU_CONFIG MIRU_RULES_DIR MIRU_GITHUB_PROXY MIRU_PORT MIRU_BIND MIRU_MIHOMO_API MIRU_MIHOMO_SECRET MIRU_GITHUB_TOKEN MIRU_PASSWORD MIRU_NO_AUTH
"$STAGE/$NAME" -check-config
section "3 / 4  Install and start service"
write_var() {
    printf "%s='" "$1"
    printf '%s' "$2" | sed "s/'/'\\\\''/g"
    printf "'\n"
}
for key in MIRU_MODE MIRU_CONFIG MIRU_RULES_DIR MIRU_GITHUB_PROXY MIRU_PORT MIRU_BIND MIRU_MIHOMO_API MIRU_MIHOMO_SECRET MIRU_GITHUB_TOKEN MIRU_PASSWORD MIRU_NO_AUTH MIRU_UPX; do
    eval 'value=${'"$key"'}'
    write_var "$key" "$value"
done > "$STAGE/miru.env"
mkdir -p /etc/miru
chmod 700 /etc/miru
for pair in "binary:$BIN" "service:$SERVICE" "config:$CONF"; do
    name=${pair%%:*}; path=${pair#*:}
    if [ -f "$path" ]; then cp -p "$path" "$STAGE/old-$name"; fi
done
was_running=false
if [ "$INIT" = procd ]; then
    if [ -x "$SERVICE" ] && "$SERVICE" running >/dev/null 2>&1; then was_running=true; fi
else
    if systemctl is-active --quiet miru; then was_running=true; fi
fi
stop() {
    if [ "$INIT" = procd ]; then [ ! -x "$SERVICE" ] || "$SERVICE" stop
    else systemctl stop miru; fi
}
start() {
    if [ "$INIT" = procd ]; then "$SERVICE" start
    else systemctl daemon-reload && systemctl start miru; fi
}
replace_file() { cp "$1" "$2.miru-new" && chmod "$3" "$2.miru-new" && mv -f "$2.miru-new" "$2"; }
restore_file() {
    if [ -f "$1" ]; then cp -p "$1" "$2.miru-new" && mv -f "$2.miru-new" "$2"
    else rm -f "$2"; fi
}
rollback() {
    trap - HUP INT TERM
    echo "Installation failed; restoring previous Miru files" >&2
    stop || true
    restore_file "$STAGE/old-binary" "$BIN" || echo "Binary restore failed" >&2
    restore_file "$STAGE/old-service" "$SERVICE" || echo "Service restore failed" >&2
    restore_file "$STAGE/old-config" "$CONF" || echo "Settings restore failed" >&2
    if [ "$INIT" = systemd ]; then systemctl daemon-reload || true; fi
    if [ "$was_running" = true ]; then start || echo "Previous service failed to restart" >&2; fi
    exit 1
}
if [ "$was_running" = true ]; then stop || fail "Could not stop old Miru service"; fi
trap rollback HUP INT TERM
replace_file "$STAGE/$NAME" "$BIN" 755 || rollback
replace_file "$STAGE/$SERVICE_ASSET" "$SERVICE" 755 || rollback
replace_file "$STAGE/miru.env" "$CONF" 600 || rollback
start || rollback
ok "Miru service started"
section "4 / 4  Health check and autostart"
HOST=$MIRU_BIND
case "$HOST" in 0.0.0.0) HOST=127.0.0.1;; ::) HOST='[::1]';; *:*) HOST="[$HOST]";; esac
healthy=false
for i in 1 2 3 4 5 6 7 8 9 10; do
    if command -v curl >/dev/null; then
        result=$(curl -fsS --max-time 2 "http://$HOST:$MIRU_PORT/healthz" 2>/dev/null || true)
    else result=$(wget -q -T 2 -O - "http://$HOST:$MIRU_PORT/healthz" 2>/dev/null || true); fi
    if [ "$result" = "miru $VERSION" ]; then
        if [ "$INIT" = procd ]; then "$SERVICE" running >/dev/null 2>&1 && healthy=true
        else systemctl is-active --quiet miru && healthy=true; fi
        [ "$healthy" = false ] || break
    fi
    sleep 1
done
[ "$healthy" = true ] || rollback
if [ "$INIT" = procd ]; then "$SERVICE" enable || rollback
else systemctl enable miru || rollback; fi
trap 'exit 1' HUP INT TERM
ok "Health check passed; autostart enabled"
section "Miru installed"
printf '  %-16s %s\n' 'Version' "$VERSION" 'Mode' "$MIRU_MODE" 'Variant' "$VARIANT" 'Web UI' "http://$HOST:$MIRU_PORT/" 'Settings' "$CONF" 'Binary' "$BIN"
if [ "$INIT" = procd ]; then
    printf '  %-16s %s\n' 'Restart' '/etc/init.d/miru restart' 'Logs' 'logread -e miru'
else
    printf '  %-16s %s\n' 'Restart' 'systemctl restart miru' 'Logs' 'journalctl -u miru -f'
fi
printf '\n'
if [ "$MIRU_NO_AUTH" = true ]; then echo 'WARNING: authentication disabled; trusted LAN only.'; fi
