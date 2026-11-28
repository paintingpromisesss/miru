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
if [ "$YES" = false ] && { [ "$CONFIGURE" = true ] || [ ! -f "$CONF" ]; }; then
    if ( : </dev/tty ) 2>/dev/null; then INTERACTIVE=true; fi
fi
# Explicit settings from automation should not unexpectedly prompt.
if [ "$CONFIGURE" = false ] && [ -n "${MIRU_BIND:-}${MIRU_PASSWORD:-}${MIRU_NO_AUTH:-}${MIRU_MODE:-}" ]; then INTERACTIVE=false; fi
prompt() {
    printf '  %s [%s]: ' "$1" "$2" >/dev/tty
    IFS= read -r ANSWER </dev/tty || fail "Input interrupted"
    ANSWER=${ANSWER:-$2}
}
prompt_password() {
    # BusyBox ash supports silent read on builds that omit the stty applet.
    if command -v stty >/dev/null 2>&1 && old_stty=$(stty -g </dev/tty 2>/dev/null); then
        printf '  Password (input hidden): ' >/dev/tty
        trap 'stty "$old_stty" </dev/tty; exit 1' HUP INT TERM
        if stty -echo </dev/tty; then
            IFS= read -r ANSWER </dev/tty || ANSWER=
            stty "$old_stty" </dev/tty
        else
            fail "Cannot disable terminal echo; set MIRU_PASSWORD before running the installer"
        fi
        trap 'exit 1' HUP INT TERM
        printf '\n' >/dev/tty
    elif ( printf 'probe\n' | (IFS= read -r -s probe) ) 2>/dev/null; then
        printf '  Password (input hidden): ' >/dev/tty
        IFS= read -r -s ANSWER </dev/tty || ANSWER=
        printf '\n' >/dev/tty
    else
        warn "Hidden input is unavailable on this device."
        prompt "Enter password visibly? (yes / no)" no
        case "$ANSWER" in yes|y) ;; *) fail "Set MIRU_PASSWORD before running the installer, or select no authentication";; esac
        printf '  Password (visible): ' >/dev/tty
        IFS= read -r ANSWER </dev/tty || ANSWER=
    fi
    [ -n "$ANSWER" ] || fail "Password cannot be empty"
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
NAME=miru-linux-$ARCH
case "${MIRU_UPX:-0}" in 1|true) NAME=$NAME-upx;; esac
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
section "1 / 4  Download and verify"
info "Version: $VERSION | Architecture: $ARCH | Service: $INIT"
for file in "$NAME" "$SERVICE_ASSET" SHA256SUMS; do download "$file" "$STAGE/$file"; done
for file in "$NAME" "$SERVICE_ASSET"; do
    (cd "$STAGE"; awk -v f="$file" '$2 == f {print}' SHA256SUMS | sha256sum -c -) || fail "Checksum mismatch: $file"
done
chmod 755 "$STAGE/$NAME"
[ "$("$STAGE/$NAME" -version)" = "$VERSION" ] || fail "Downloaded binary version mismatch"
ok "Release checksums and binary version verified"
section "2 / 4  Configuration"
# Existing settings take precedence on upgrades. Legacy installs did not
# record a mode or bind address and may refer to a removed standalone core.
REQUESTED_MODE="${MIRU_MODE:-}"
LEGACY_MIGRATION=false
if [ -f "$CONF" ]; then . "$CONF"; info "Existing settings loaded: $CONF"; fi
if [ -z "${MIRU_MODE:-}" ]; then
    if [ -x /etc/init.d/nikki ] && [ -f /etc/nikki/mixin.yaml ] &&
        { [ -z "${MIRU_CONFIG:-}" ] || [ "$MIRU_CONFIG" = /etc/nikki/mixin.yaml ] || [ ! -f "$MIRU_CONFIG" ]; }; then
        MIRU_MODE=nikki
        if [ -f "$CONF" ] && [ "${MIRU_CONFIG:-}" != /etc/nikki/mixin.yaml ]; then
            LEGACY_MIGRATION=true
            warn "Legacy standalone configuration is unavailable; migrating Miru settings to Nikki."
            unset MIRU_CONFIG MIRU_RULES_DIR MIRU_GITHUB_PROXY MIRU_MIHOMO_SECRET
            if [ "${MIRU_PORT:-}" = 8080 ]; then MIRU_PORT=8787; fi
        fi
    else MIRU_MODE=standalone; fi
fi
if [ "$LEGACY_MIGRATION" = true ] && [ -z "$REQUESTED_MODE" ] && [ "$YES" = false ]; then
    if ( : </dev/tty ) 2>/dev/null; then INTERACTIVE=true; fi
fi
if [ "$INTERACTIVE" = true ]; then
    info "Detected mode: $MIRU_MODE"
    prompt "Mode (nikki / standalone)" "$MIRU_MODE"
    if [ "$ANSWER" != "$MIRU_MODE" ]; then
        unset MIRU_CONFIG MIRU_RULES_DIR MIRU_GITHUB_PROXY MIRU_MIHOMO_SECRET
    fi
    MIRU_MODE=$ANSWER
fi
case "$MIRU_MODE" in
    nikki)
        [ -x /etc/init.d/nikki ] || fail "Nikki is not installed"
        [ "$(uci -q get nikki.mixin.mixin_file_content || true)" = 1 ] || fail "Enable Nikki mixin file content first"
        : "${MIRU_CONFIG:=/etc/nikki/mixin.yaml}"
        : "${MIRU_RULES_DIR:=/etc/nikki/run/ruleset}"
        : "${MIRU_GITHUB_PROXY:=none}";;
    standalone)
        : "${MIRU_CONFIG:=/etc/mihomo/config.yaml}"
        : "${MIRU_RULES_DIR:=/etc/mihomo/rules}"
        : "${MIRU_GITHUB_PROXY:=auto}";;
    *) fail "Invalid MIRU_MODE";;
esac
: "${MIRU_PORT:=8787}"
: "${MIRU_BIND:=127.0.0.1}"
: "${MIRU_MIHOMO_API:=http://127.0.0.1:9090}"
: "${MIRU_MIHOMO_SECRET:=}"
: "${MIRU_GITHUB_TOKEN:=}"
: "${MIRU_PASSWORD:=}"
: "${MIRU_NO_AUTH:=false}"
if [ "$INTERACTIVE" = true ]; then
    prompt "Editable configuration" "$MIRU_CONFIG"; MIRU_CONFIG=$ANSWER
    prompt "Rules cache directory" "$MIRU_RULES_DIR"; MIRU_RULES_DIR=$ANSWER
    prompt "Web UI port" "$MIRU_PORT"; MIRU_PORT=$ANSWER
    LAN=
    if [ "$INIT" = procd ]; then
        LAN=$(uci -q get network.lan.ipaddr 2>/dev/null || true)
        LAN=${LAN%% *}; LAN=${LAN%%/*}
    fi
    if [ -n "$LAN" ] && { [ ! -f "$CONF" ] || [ "$LEGACY_MIGRATION" = true ]; } && [ "$MIRU_BIND" = 127.0.0.1 ]; then MIRU_BIND=$LAN; fi
    prompt "Listen address (127.0.0.1 for local only)" "$MIRU_BIND"; MIRU_BIND=$ANSWER
    prompt "Mihomo controller URL" "$MIRU_MIHOMO_API"; MIRU_MIHOMO_API=$ANSWER
    auth=password
    case "$MIRU_NO_AUTH" in true|1) auth=none;; esac
    prompt "Authentication (password / none)" "$auth"
    case "$ANSWER" in
        password)
            MIRU_NO_AUTH=false
            if [ -n "$MIRU_PASSWORD" ]; then
                prompt "Keep existing password? (yes / no)" yes
                case "$ANSWER" in yes|y) ;; no|n) prompt_password; MIRU_PASSWORD=$ANSWER;; *) fail "Invalid choice";; esac
            else prompt_password; MIRU_PASSWORD=$ANSWER; fi;;
        none)
            warn "Unauthenticated users can edit routing and restart the VPN."
            prompt "Disable authentication on this trusted network? (yes / no)" no
            case "$ANSWER" in yes|y) MIRU_NO_AUTH=true; MIRU_PASSWORD=;; *) fail "Authentication not disabled";; esac;;
        *) fail "Choose password or none";;
    esac
fi
case "$MIRU_NO_AUTH" in 1|true) MIRU_NO_AUTH=true;; 0|false) MIRU_NO_AUTH=false;; *) fail "Invalid MIRU_NO_AUTH";; esac
if [ "$MIRU_BIND" != 127.0.0.1 ] && [ "$MIRU_BIND" != ::1 ] && [ -z "$MIRU_PASSWORD" ] && [ "$MIRU_NO_AUTH" = false ]; then
    fail "Set MIRU_PASSWORD or explicitly MIRU_NO_AUTH=1 for LAN access"
fi
case "$MIRU_PORT" in ''|*[!0-9]*) fail "Invalid port";; esac
[ "$MIRU_PORT" -ge 1 ] && [ "$MIRU_PORT" -le 65535 ] || fail "Invalid port"
[ -f "$MIRU_CONFIG" ] || fail "Missing config: $MIRU_CONFIG"
export MIRU_MODE MIRU_CONFIG MIRU_RULES_DIR MIRU_GITHUB_PROXY MIRU_PORT MIRU_BIND MIRU_MIHOMO_API MIRU_MIHOMO_SECRET MIRU_GITHUB_TOKEN MIRU_PASSWORD MIRU_NO_AUTH
"$STAGE/$NAME" -check-config
printf '\n  %-19s %s\n' 'Mode' "$MIRU_MODE" 'Config' "$MIRU_CONFIG" 'Rules cache' "$MIRU_RULES_DIR" 'Listen' "$MIRU_BIND:$MIRU_PORT" 'Controller' "$MIRU_MIHOMO_API" 'Init system' "$INIT"
if [ "$MIRU_NO_AUTH" = true ]; then warn "Authentication disabled"
elif [ -n "$MIRU_PASSWORD" ]; then info "Authentication: enabled (user miru)"
else info "Authentication: local only"; fi
if [ "$INTERACTIVE" = true ]; then
    prompt "Install with these settings? (yes / no)" yes
    case "$ANSWER" in yes|y) ;; *) fail "Installation cancelled";; esac
fi
section "3 / 4  Install and start service"
write_var() {
    printf "%s='" "$1"
    printf '%s' "$2" | sed "s/'/'\\\\''/g"
    printf "'\n"
}
for key in MIRU_MODE MIRU_CONFIG MIRU_RULES_DIR MIRU_GITHUB_PROXY MIRU_PORT MIRU_BIND MIRU_MIHOMO_API MIRU_MIHOMO_SECRET MIRU_GITHUB_TOKEN MIRU_PASSWORD MIRU_NO_AUTH; do
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
printf '  %-16s %s\n' 'Version' "$VERSION" 'Mode' "$MIRU_MODE" 'Web UI' "http://$HOST:$MIRU_PORT/" 'Settings' "$CONF" 'Binary' "$BIN"
if [ "$INIT" = procd ]; then
    printf '  %-16s %s\n' 'Restart' '/etc/init.d/miru restart' 'Logs' 'logread -e miru'
else
    printf '  %-16s %s\n' 'Restart' 'systemctl restart miru' 'Logs' 'journalctl -u miru -f'
fi
printf '\n'
if [ "$MIRU_NO_AUTH" = true ]; then echo 'WARNING: authentication disabled; trusted LAN only.'; fi
