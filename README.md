# Miru

<div align="center">

**Lightweight Mihomo Rule & MRS Manager with Embedded Web UI for OpenWrt Routers**

[![Go Version](https://img.shields.io/github/go-mod/go-version/paintingpromisesss/miru?style=flat-square)](https://go.dev/)
[![Release](https://img.shields.io/github/v/release/paintingpromisesss/miru?style=flat-square)](https://github.com/paintingpromisesss/miru/releases)
[![License](https://img.shields.io/github/license/paintingpromisesss/miru?style=flat-square)](LICENSE)

[Features](#features) • [Installation](#installation) • [CLI Flags](#configuration--cli-flags) • [API](#api-endpoints) • [Building](#building-from-source)

</div>

---

**Miru** is a self-contained, ultra-lightweight rule manager designed for the **Mihomo** (Clash.Meta) core running on resource-constrained embedded Linux systems and OpenWrt routers.

It provides a modern, fast Web UI embedded directly into a single static Go binary without third-party web frameworks or heavy runtime dependencies. Released binaries include local CSS and require no frontend CDN.

---

## Features

- **Two operating modes:** standalone Mihomo and native Nikki mixin integration.
- **Embedded Web UI:** vanilla JavaScript, locally bundled Tailwind CSS, a single Go executable.
- **YAML AST editing:** preserves comments and unrelated fields where possible; atomic writes, backup and attempted rollback when applying changes fails.
- **Ruleset catalogs:** Tachyon and MetaCubeX, with search, preview, installation and routing to any available target.
- **Custom routing:** bulk domains, custom URL lists (MRS, YAML, text), ordered raw rules, proxy groups and proxy providers.
- **Independent states:** installed, routed and cached are distinct; removing a route does not unregister its ruleset.
- **Service integration:** procd on OpenWrt and systemd on Linux, authentication by default for non-loopback access, explicit opt-out on trusted LANs.

## Operating modes

| | Standalone | Nikki |
|---|---|---|
| Editable file | `/etc/mihomo/config.yaml` | `/etc/nikki/mixin.yaml` |
| Rules | `rules` | `nikki-rules` |
| Proxy groups | `proxy-groups` | `nikki-proxy-groups` |
| Cache directory | `/etc/mihomo/rules` | `/etc/nikki/run/ruleset` |
| Apply | Mihomo controller reload | Nikki service restart |
| Default catalog | MetaCubeX | Tachyon |

Nikki requires mixin file content to be enabled. Miru reads its runtime configuration for inherited targets and never edits the generated runtime file or subscription. Applying changes may interrupt connections; keep an SSH session available.

---

## Supported Router Platforms

| Platform / SoC | Architecture | Typical Devices |
|---|---|---|
| MediaTek Filogic 820 / 830, Rockchip RK35xx | `linux/arm64` | GL.iNet MT3000, Netis NX31, NanoPi R5S, RPi 4/5 |
| MediaTek MT7621, MT7628, MT7620 | `linux/mipsle` (`GOMIPS=softfloat`) | Xiaomi R3G / 4A Gigabit, Keenetic, DIR-615 |
| Atheros AR7xxx, AR9xxx, QCA95xx | `linux/mips` (`GOMIPS=softfloat`) | TP-Link Archer C7, WR1043ND |
| Cortex-A7, Cortex-A9 | `linux/arm` (ARMv7) | Linksys WRT1900AC, ASUS RT-AC86U |
| x86-64 | `linux/amd64` | Intel N100 / J4125 Mini PCs, PC Engines APU, Proxmox |

> [!NOTE]
> All MIPS builds are compiled with `GOMIPS=softfloat` for compatibility on popular routers lacking hardware FPU units.

---

## Installation

### Automated installation (recommended)

Stop any manually started test instance before installing. Download and inspect the installer before running as root.

**OpenWrt:**

```sh
wget -O /tmp/install.sh https://raw.githubusercontent.com/paintingpromisesss/miru/main/scripts/install.sh
sh /tmp/install.sh
```

**Linux (systemd):**

```sh
curl -fsSL https://raw.githubusercontent.com/paintingpromisesss/miru/main/scripts/install.sh -o /tmp/install.sh
sudo sh /tmp/install.sh
```

The first-run wizard detects the mode and prompts for configuration path, cache directory, port, bind address, controller and authentication. On OpenWrt it suggests the LAN address if available. It verifies checksums, validates startup settings, installs the service, checks health and enables autostart. It attempts rollback if the new service fails to start. It does not modify the VPN configuration.

For unattended installation:

```sh
MIRU_MODE=nikki MIRU_BIND=192.168.0.1 MIRU_NO_AUTH=1 sh /tmp/install.sh --yes
```

Use `MIRU_PASSWORD` instead of `MIRU_NO_AUTH=1` to enable HTTP Basic authentication (username `miru`). No-auth is intended only for a trusted LAN. Never expose the plain HTTP interface publicly; prefer a trusted network or SSH tunnel.

### Build variants

| Variant | Asset | Trade-off |
|---|---|---|
| Standard | `miru-linux-<arch>` | Recommended; no runtime decompression |
| UPX | `miru-linux-<arch>-upx` | Smaller binary when compression succeeds; may increase startup time and RAM usage |

Actual sizes vary by architecture and version; see [release assets](https://github.com/paintingpromisesss/miru/releases/latest). Install the compressed variant with `MIRU_UPX=1 sh /tmp/install.sh`.

### Update or reconfigure

Run the installer again to upgrade, keeping existing settings. Use `sh /tmp/install.sh --configure` to revisit the wizard. For offline installation, copy `VERSION`, `SHA256SUMS`, `install.sh`, the appropriate service file (`miru.init` or `miru.service`) and your architecture's binary from the same release into a directory:

```sh
MIRU_ASSET_DIR=/tmp/miru-release sh /tmp/miru-release/install.sh
```

| Installer option | Purpose |
|---|---|
| `--yes` | Skip interactive prompts |
| `--configure` | Reconfigure an existing installation |
| `MIRU_VERSION` | Select release (default: latest) |
| `MIRU_ASSET_DIR` | Use local release files instead of downloads |
| `MIRU_UPX=1` | Select UPX binary |

### Service management

| Action | OpenWrt | systemd |
|---|---|---|
| Start | `/etc/init.d/miru start` | `systemctl start miru` |
| Stop | `/etc/init.d/miru stop` | `systemctl stop miru` |
| Restart | `/etc/init.d/miru restart` | `systemctl restart miru` |
| Enable autostart | `/etc/init.d/miru enable` | `systemctl enable miru` |
| Logs | `logread -e miru` | `journalctl -u miru -f` |

Settings are stored in `/etc/miru/miru.env` (mode 0600). Edit the file and restart the service to change settings. The installer displays the selected web address and port; the default CLI port is 8787.

---

## Configuration & CLI Flags

| Flag | Environment | Default / purpose |
|---|---|---|
| `-mode` | `MIRU_MODE` | standalone; nikki supported |
| `-bind` | `MIRU_BIND` | 127.0.0.1 |
| `-port` | `MIRU_PORT` | 8787 |
| `-config` | `MIRU_CONFIG` | Mode-specific YAML path |
| `-rules-dir` | `MIRU_RULES_DIR` | Mode-specific cache path |
| `-mihomo-api` | `MIRU_MIHOMO_API` | http://127.0.0.1:9090 |
| `-mihomo-secret` | `MIRU_MIHOMO_SECRET` | Auto-read if omitted |
| `-github-token` | `MIRU_GITHUB_TOKEN` | Optional GitHub token |
| `-github-proxy` | `MIRU_GITHUB_PROXY` | auto in standalone; none in Nikki |
| `-catalog-repo` | — | Mode-specific catalog repository |
| `-catalog-ref` | — | meta / generated |
| `-catalog-ttl` | — | 12h |
| `-no-auth` | `MIRU_NO_AUTH` | false; explicit opt-out |
| — | `MIRU_PASSWORD` | HTTP Basic password; required for non-loopback unless opted out |
| `-check-config` | — | Validate configuration and exit |
| `-version` | — | Print version and exit |

CLI flags override environment variables. Example manual startup:

```sh
MIRU_PASSWORD='choose-a-password' ./miru -mode nikki -bind 192.168.0.1 -port 8787
```

---

## API Endpoints

API responses are JSON; the health endpoint returns plain text. Authenticate with HTTP Basic when enabled.

| Method & Path | Description |
|---|---|
| `GET /api/local` | Current state: parsed rules, rule-providers, proxy groups, and local disk files |
| `GET /api/catalog` | Catalog items; source=default/tachyon/metacubex; action=refresh |
| `POST /api/rules` | Register rule-provider and append `RULE-SET` directive before `MATCH` |
| `DELETE /api/rules` | Delete rule and provider (`?name=<name>&delete_file=true` to purge disk file) |
| `PUT /api/rules` | Edit ruleset routing and provider settings |
| `GET /api/rules/preview` | Preview a local or remote list |
| `POST /api/domains` | Add a batch of domain routes |
| `POST/PUT/DELETE /api/raw-rules` | Edit individual routes |
| `POST /api/raw-rules/move` | Reorder routes |
| `POST/PUT/DELETE /api/proxy-groups` | Manage proxy groups |
| `POST/PUT/DELETE /api/proxy-providers` | Manage proxy sources |
| `POST /api/quic/global` | Toggle global QUIC blocking |
| `POST /api/reload` | Apply configuration using current mode |
| `GET /api/mihomo/status` | Core status |
| `GET /healthz` | Service health/version (no credentials required) |
| `GET /` | Embedded single-page Web UI |

### Example: Adding a Rule via cURL

```sh
curl -X POST http://127.0.0.1:8787/api/rules \
  -H "Content-Type: application/json" \
  -d '{
    "name": "youtube",
    "behavior": "domain",
    "format": "mrs",
    "url": "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/meta/geo/geosite/youtube.mrs",
    "proxy_group": "PROXY"
  }'
```

---

## Building from Source

Requires the Go version declared in go.mod (currently 1.26.2). Node.js is needed only at build time to bundle local CSS.

```sh
# Clone repository
git clone https://github.com/paintingpromisesss/miru.git
cd miru

# Bundle CSS for offline use (modifies web/index.html)
npx --yes --package=tailwindcss@3.4.17 tailwindcss -i web/tailwind.css -o /tmp/miru-tailwind.css --content web/index.html --minify
node web/bundle-css.cjs /tmp/miru-tailwind.css

# Native build
make build

# Cross-compile for OpenWrt targets
make build-linux-arm64    # ARM64 (MediaTek Filogic, RPi 4/5)
make build-linux-mipsle   # MIPSLE (MT7621 with softfloat)
make build-linux-mips     # MIPS Big-Endian (Atheros)
make build-linux-armv7    # ARMv7
make build-linux-amd64    # x86-64

# Run test suite
make test
```

---

## License

MIT License. See [LICENSE](LICENSE) for details.
