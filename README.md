# Argus

[![GitHub Release](https://img.shields.io/github/v/release/ulm0/argus?sort=semver&display_name=tag&style=flat-square)](https://github.com/ulm0/argus/releases/latest) [![GitHub Actions Workflow Status](https://img.shields.io/github/actions/workflow/status/ulm0/argus/release.yml?style=flat-square)](https://github.com/ulm0/argus/actions/workflows/release.yml) [![License](https://img.shields.io/github/license/ulm0/argus?style=flat-square)](https://github.com/ulm0/argus/blob/main/LICENSE) ![GitHub Downloads (all assets, all releases)](https://img.shields.io/github/downloads/ulm0/argus/total?style=flat-square) 

Argus is an edge-optimized Tesla Dashcam and Sentry Mode manager implemented as a **single Go binary** with an embedded web UI. It is designed for unattended, in-car operation with low CPU and RAM footprint.

## What Argus does

Argus turns a Raspberry Pi into a Tesla-compatible multi-LUN USB storage device while also exposing a local web UI for management:

- Presents TeslaCam/LightShow/Music storage over USB gadget mode
- Supports mode switching between:
  - **Present mode** (Tesla-facing USB gadget)
  - **Edit mode** (RW mounts + Samba/file management)
- Provides a web interface for:
  - videos/events browsing
  - chimes/lightshows/wraps/music management
  - cleanup and analytics
  - runtime settings
- Includes unattended reliability features:
  - startup boot pipeline
  - optional fsck checks
  - optional cleanup
  - optional random chime selection
  - hardware watchdog integration
  - AP fallback and Wi-Fi monitoring
  - Telegram alerting for Sentry events (when configured)

## Architecture

- **Backend:** Go (USB gadget, mounts, loops, Samba orchestration, AP/Wi-Fi, Telegram, scheduler)
- **Frontend:** Next.js static export embedded in the Go binary
- **Delivery model:** single self-contained executable
- **Configuration:** single `config.yaml` source of truth

## Requirements

### Recommended hardware

- Raspberry Pi Zero 2 W (primary target)
- microSD with enough space for OS + image files
- Reliable data USB cable (not charge-only)

### OS support

Argus is intended to run on **Raspberry Pi OS Lite** for unattended deployments.  

### Runtime requirements

- Linux kernel with USB OTG gadget support (`dwc2`, `libcomposite`, `configfs`)
- systemd
- root privileges for setup/system operations

## Project structure

```text
cmd/argus/        Go entrypoint + CLI commands
internal/         API, services, system integrations
web/              Next.js frontend source
config.yaml       Reference configuration (see ~/.argus/config.yaml on device)
scripts/          install-argus.sh (one-command install from GitHub)
Makefile          Build/test orchestration
```

## CLI commands

```bash
argus run [--config path]
argus generate [--output path] [--force]
argus setup [--dir path] [--show-size 10G] [--music-size 32G]
argus upgrade [--yes]
argus remove [--yes] [--keep-images]
argus version
```

## Installation

Releases ship a **raw Linux binary** per architecture (no archive). Names match GoReleaser, e.g. `argus_1.2.3_linux_arm64`.

### Raspberry Pi (recommended)

```bash
curl -fsSL https://raw.githubusercontent.com/ulm0/argus/main/scripts/install-argus.sh | sudo bash
argus generate
sudo argus setup
```

This installs the **latest release** binary for your CPU (`arm64` / `armv7` / `armv6`). If you prefer not to pipe from the network, clone the repo and run `sudo bash scripts/install-argus.sh`, or open the [releases page](https://github.com/ulm0/argus/releases/latest) and `sudo install -m 0755` the `argus_*_linux_*` file that matches your Pi.

After setup, upgrades on the device are: `sudo argus upgrade` (same raw binary layout).

### Setup options

| Flag | Default | Description |
|---|---|---|
| `--dir` | `~/.argus/` | Data directory |
| `--config` | `<dir>/config.yaml` | Config file path |
| `--show-size` | `10G` (`ARGUS_SHOW_SIZE`) | LightShow image size |
| `--music-size` | `32G` (`ARGUS_MUSIC_SIZE`) | Music image size |

## Build and development

```bash
make deps
make all
make test
```

Frontend dev:

```bash
cd web
pnpm dev
```

Backend dev:

```bash
go run ./cmd/argus run config.yaml
```

## Configuration

All runtime settings are in `~/.argus/config.yaml`.

### Main sections

| Section | Purpose |
|---|---|
| `installation` | startup behavior, target user, mount dir |
| `disk_images` | image names, partition toggles, fsck boot option |
| `network` | web port, Samba password |
| `offline_ap` | AP fallback behavior |
| `system` | watchdog + sysctl startup behavior |
| `telegram` | alerting configuration |
| `update` | update strategy |

### Unattended defaults (new setups)

`argus setup` generates unattended-friendly defaults:

- `installation.boot_present_on_start: true`
- `installation.boot_block_until_ready: true`
- `installation.boot_cleanup_on_start: true`
- `installation.boot_random_chime_on_start: false`
- `disk_images.boot_fsck_enabled: true`
- `system.watchdog_enabled: true`
- `system.watchdog_timeout_sec: 60` (Debian `watchdog` daemon, TeslaUSB-style)
- `system.reapply_sysctl_on_start: true`

### Web-configurable startup and reliability

The Settings page includes a **Startup & Reliability** section:

- Present on startup
- Block startup until boot pipeline completes
- Boot cleanup
- Random chime on startup
- Boot fsck checks
- Watchdog via Debian `watchdog.service` (`/etc/watchdog.conf`) + timeout
- Sysctl profile re-apply on startup

Some changes take effect on next service restart or reboot.

## Feature overview

### Storage and USB gadget

- Multi-LUN presentation for TeslaCam + optional partitions
- RO local mounts in Present mode for safe read access
- RW mounts in Edit mode for management operations
- Safe mode transition sequencing (unmount/loop/gadget orchestration)

### Media and management

- Video browsing and event handling
- Chime management and scheduling
- Lightshow and wraps management
- Music file management (optional partition)
- Cleanup policies and analytics

### Networking and resiliency

- AP fallback on connectivity loss
- Wi-Fi status monitoring
- Telegram event queue and delivery
- Optional hardware watchdog (Debian watchdog daemon, like TeslaUSB)

## Acknowledgements

Argus is **heavily based on [TeslaUSB](https://github.com/mphacker/TeslaUSB)** concepts and workflow.  
Huge thanks to the TeslaUSB project and contributors.

## License

See repository files for license information.
