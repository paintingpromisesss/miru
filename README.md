# Miru

<div align="center">

**Lightweight Mihomo Rule & MRS Manager with Embedded Web UI for OpenWrt Routers**

[![Go Version](https://img.shields.io/github/go-mod/go-version/paintingpromisesss/miru?style=flat-square)](https://go.dev/)
[![Release](https://img.shields.io/github/v/release/paintingpromisesss/miru?style=flat-square)](https://github.com/paintingpromisesss/miru/releases)
[![License](https://img.shields.io/github/license/paintingpromisesss/miru?style=flat-square)](LICENSE)

[Features](#features) • [Installation](#installation) • [Architecture](#architecture) • [CLI Flags](#configuration--cli-flags) • [API](#api-endpoints) • [Building](#building-from-source)

</div>

---

**Miru** is a self-contained, ultra-lightweight rule manager designed for the **Mihomo** (Clash.Meta) core running on resource-constrained embedded Linux systems and OpenWrt routers.

It provides a modern, fast Web UI embedded directly into a single static Go binary without third-party web frameworks, heavy runtimes, or external build steps.

```
       Browser (Web UI)
              │
              │  HTTP (REST API & Embedded Vanilla UI)
              ▼
        ┌───────────┐
        │   Miru    │ ◄─── GitHub Trees API (Cached MRS Catalog)
        └─────┬─────┘
              │
      Safe AST Editing (yaml.Node)
              │
              ▼
   /etc/mihomo/config.yaml  ───► HTTP PUT /configs?force=true ───► Mihomo Core
   (Comments Preserved)                                             (Hot Reload)
```

---

## Features

- **Zero-Dependency Core**: Built purely on Go's standard library (`net/http`, `embed`) and `gopkg.in/yaml.v3`. No Gin, Fiber, Chi, or Node.js required.
- **Embedded Web UI**: Dark modern UI built with pure Vanilla JS and Tailwind CDN compiled directly into the binary via `//go:embed`.
- **Lossless YAML AST Manipulation**: Uses `yaml.Node` to read and edit `/etc/mihomo/config.yaml`. Formatting, custom indentation, and inline comments are strictly preserved.
- **Remote MRS Catalog**: Fetches and memory-caches available `.mrs` rulesets (MetaCubeX/meta-rules-dat) using the GitHub Trees API to stay well clear of rate limits.
- **Strict Rule Placement**: Automatically registers new `rule-providers` entries and places `RULE-SET,<name>,<group>` directives strictly before the final `MATCH` fallback rule.
- **Custom Rule Support**: Add custom `.mrs` rulesets from any URL with configurable behavior (`domain`, `ipcidr`, `classical`).
- **Disk Synchronization**: Scans the local rules directory (`/etc/mihomo/rules/`) to show downloaded file status, and optionally deletes `.mrs` binaries upon rule removal.
- **Seamless Hot-Reload**: Triggers `PUT /configs?force=true` on the Mihomo controller upon changes, updating rules without dropping active user connections.

---

## Supported Router Platforms

| Platform / SoC | Architecture | Typical Devices |
|---|---|---|
| MediaTek Filogic 820 / 830, Rockchip RK35xx | `linux/arm64` | GL.iNet MT3000, Netis NX31, NanoPi R5S, RPi 4/5 |
| MediaTek MT7621, MT7628, MT7620 | `linux/mipsle` (`GOMIPS=softfloat`) | Xiaomi R3G / 4A Gigabit, Keenetic, DIR-615 |
| Atheros AR7xxx, AR9xxx, QCA95xx | `linux/mips` (`GOMIPS=softfloat`) | TP-Link Archer C7, WR1043ND |
| Cortex-A7, Cortex-A9 | `linux/arm` (ARMv7) | Linksys WRT1900AC, ASUS RT-AC86U |
| x86 / x86-64 | `linux/amd64` | Intel N100 / J4125 Mini PCs, PC Engines APU, Proxmox |

> [!NOTE]
> All MIPS builds are compiled with `GOMIPS=softfloat` to prevent `Illegal instruction` crashes on popular routers lacking hardware FPU units.

---

## Installation

### Automated Installation (Recommended)

Run directly on your router or server:

**OpenWrt (Routers):**
```sh
wget -O /tmp/install.sh https://raw.githubusercontent.com/paintingpromisesss/miru/main/scripts/install.sh && sh /tmp/install.sh
```

**Linux (Debian, Ubuntu, Arch, CentOS):**
```sh
curl -fsSL https://raw.githubusercontent.com/paintingpromisesss/miru/main/scripts/install.sh | sudo sh
```

### UPX Compressed Variant (Low Storage)

For devices with limited flash memory (16MB/32MB routers), specify `MIRU_UPX=1`:

**OpenWrt:**
```sh
wget -O /tmp/install.sh https://raw.githubusercontent.com/paintingpromisesss/miru/main/scripts/install.sh && MIRU_UPX=1 sh /tmp/install.sh
```

**Linux:**
```sh
curl -fsSL https://raw.githubusercontent.com/paintingpromisesss/miru/main/scripts/install.sh | sudo MIRU_UPX=1 sh
```

### Build Variants Comparison

| Variant | Binary Size | RAM Usage | Notes |
|---|---|---|---|
| **Standard** | ~6.5 MB | ~10–14 MB | Recommended default. Fast startup, minimal CPU overhead. |
| **UPX-compressed** (`*-upx`) | ~2.1 MB | ~16–22 MB | ~68% smaller disk footprint. Ideal for routers with 16MB/32MB flash. |

---

## OpenWrt Service Configuration

Create an init script at `/etc/init.d/miru`:

```sh
cat << 'EOF' > /etc/init.d/miru
#!/bin/sh /etc/rc.common

START=95
STOP=10
USE_PROCD=1

PROG=/usr/bin/miru

start_service() {
    procd_open_instance
    procd_set_param command "$PROG" \
        -config /etc/mihomo/config.yaml \
        -rules-dir /etc/mihomo/rules \
        -port 8080 \
        -mihomo-api http://127.0.0.1:9090
    procd_set_param respawn
    procd_set_param stdout 1
    procd_set_param stderr 1
    procd_close_instance
}
EOF

chmod +x /etc/init.d/miru
/etc/init.d/miru enable
/etc/init.d/miru start
```

Open your browser at `http://192.168.1.1:8080` to access the Web UI.

---

## Configuration & CLI Flags

| Flag | Default | Description |
|---|---|---|
| `-config` | `/etc/mihomo/config.yaml` | Path to the Mihomo configuration file |
| `-port` | `8080` | HTTP port for the Miru Web UI and REST API |
| `-rules-dir` | `/etc/mihomo/rules` | Directory where downloaded `.mrs` files are stored |
| `-mihomo-api` | `http://127.0.0.1:9090` | Mihomo external controller base URL |
| `-mihomo-secret` | `""` (auto-read) | Mihomo API secret token (automatically read from config if omitted) |
| `-github-token` | `""` | Optional GitHub Personal Access Token to avoid rate limits |
| `-catalog-repo` | `MetaCubeX/meta-rules-dat` | GitHub repository containing `.mrs` rulesets |
| `-catalog-ref` | `meta` | Git branch or tag in the catalog repository |
| `-catalog-ttl` | `12h0m0s` | Duration to cache the remote ruleset tree in memory |

---

## API Endpoints

All responses are formatted in JSON (`application/json`).

| Method & Path | Description |
|---|---|
| `GET /api/local` | Current state: parsed rules, rule-providers, proxy groups, and local disk files |
| `GET /api/catalog` | Available remote `.mrs` rulesets with applied status (`?action=refresh` to force reload) |
| `POST /api/rules` | Register rule-provider and append `RULE-SET` directive before `MATCH` |
| `DELETE /api/rules` | Delete rule and provider (`?name=<name>&delete_file=true` to purge disk file) |
| `POST /api/reload` | Manually trigger Mihomo hot-reload (`PUT /configs?force=true`) |
| `GET /` | Embedded single-page Web UI |

### Example: Adding a Rule via cURL

```sh
curl -X POST http://127.0.0.1:8080/api/rules \
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

Requires Go 1.22+.

```sh
# Clone repository
git clone https://github.com/paintingpromisesss/miru.git
cd miru

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
