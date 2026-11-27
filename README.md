# Miru

Lightweight Mihomo routing manager for OpenWrt and Linux. One static Go binary, embedded web UI, no runtime Node.js or external frontend CDN in release builds.

## Modes

- Standalone: edits Mihomo's config.yaml and applies via its controller.
- Nikki: edits `/etc/nikki/mixin.yaml`, reads generated runtime configuration to discover nodes, and restarts Nikki to apply. Enable mixin file content first. Restarts may briefly interrupt traffic.

Routes, installed rulesets, Tachyon / MetaCubeX catalogs, custom domains and URLs, proxy groups and advanced controls are managed separately. Removing a route does not unregister its ruleset. Config edits have backup and attempted rollback on failure.

## Install or upgrade

Stop any manually started test instance first. Download and inspect the installer before running as root:

```sh
wget -O /tmp/miru-install.sh https://raw.githubusercontent.com/paintingpromisesss/miru/main/scripts/install.sh
MIRU_BIND=192.168.0.1 MIRU_PASSWORD='choose-a-password' sh /tmp/miru-install.sh
```

The installer detects Nikki, otherwise standalone. Default bind is loopback, port 8787. Existing settings in `/etc/miru/miru.env` take precedence on upgrades. Downloads are checksum-verified and staged before stopping the old service; startup failure attempts to restore the old files. It never edits Nikki/Mihomo configuration. procd and systemd are supported. Rerun to upgrade.

For offline installation, download VERSION, SHA256SUMS, install.sh, miru.init (or miru.service), and the architecture-specific binary from the same release into a directory. Copy it to the device and set `MIRU_ASSET_DIR` to that directory.

Environment: MIRU_MODE, MIRU_BIND, MIRU_PORT, MIRU_PASSWORD, MIRU_NO_AUTH (explicit 1/true), MIRU_CONFIG, MIRU_RULES_DIR, MIRU_MIHOMO_API, MIRU_MIHOMO_SECRET, MIRU_GITHUB_TOKEN, MIRU_GITHUB_PROXY. MIRU_VERSION selects release; MIRU_UPX=1 selects compressed binary. Edit `/etc/miru/miru.env` and restart for changes after installation.

HTTP Basic user: miru. Do not expose this HTTP service publicly. For trusted LAN only, explicitly opt out with MIRU_NO_AUTH=1 instead of a password. Manual start:

```sh
./miru -mode nikki -bind 192.168.0.1 -port 8787 -no-auth
```

OpenWrt: `/etc/init.d/miru restart`, `logread -e miru`. Linux: `systemctl restart miru`, `journalctl -u miru`.

## Build

Linux arm64, armv7, amd64, mips and mipsle (softfloat). Use the Go version in go.mod. For offline UI assets:

```sh
npx --yes --package=tailwindcss@3.4.17 tailwindcss -i web/tailwind.css -o /tmp/miru-tailwind.css --content web/index.html --minify
node web/bundle-css.cjs /tmp/miru-tailwind.css
go test ./...
go build -o miru ./cmd/miru
```

The CSS bundler modifies the working copy. CI checks JavaScript, Go tests, vet and ARM64 build. Optional browser smoke test: `npm install --no-save --no-package-lock playwright@1.56.1`, `npx playwright install chromium`, `node web/browser-smoke.cjs` (after CSS bundling).

MIT License.
