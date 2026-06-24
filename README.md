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
  - Video/event browsing with playback, telemetry overlay, and downloads
  - Chimes/lightshows/wraps/music management
  - Cleanup policies and analytics
  - WiFi, Bluetooth, AP, Samba, Telegram, and webhook configuration
  - Runtime settings
  - Session-based login (default `admin` / `argus`, changeable)
- Includes unattended reliability features:
  - Startup boot pipeline
  - Optional fsck checks on boot
  - Optional cleanup on boot
  - Optional random chime selection on boot
  - Hardware watchdog integration
  - AP fallback with WiFi reconnect monitoring
  - Telegram and webhook alerting for Sentry events

## Architecture

- **Backend:** Go (USB gadget, mounts, loops, Samba orchestration, AP/Wi-Fi, Bluetooth, Telegram, webhook, scheduler)
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
argus refresh-service
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

### Accessing the web UI

Browse to the device on the configured `web_port` (default `80`) — over your WiFi
or the offline AP. The UI is protected by a session login:

- **Default credentials: `admin` / `argus`.**
- **Change them on first use** — from **Settings** in the UI, or by editing
  `auth_username` / `auth_password` under `web:` in `config.yaml`. The login
  screen nags while the defaults are still in place.
- Set `auth_enabled: false` under `web:` to disable the login entirely (only do
  this on a fully trusted, isolated network).

Sessions are stateless cookies signed with `web.secret_key` (auto-generated on
first run), `HttpOnly` + `SameSite=Strict`, and marked `Secure` automatically
when Argus is reached over HTTPS (e.g. behind a TLS reverse proxy).

The API also rejects requests whose `Host` header isn't a local identity (an IP,
`localhost`, a bare name, or `*.local` / `*.lan`) as a DNS-rebinding defense. If
you front Argus with a reverse proxy on a real domain, add that hostname to
`web.allowed_hosts`.

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
| `installation` | startup behavior, target user, mount dir, archive path |
| `disk_images` | image names, partition toggles, fsck boot option |
| `network` | web port, Samba password |
| `web` | UI login (`auth_enabled`, `auth_username`, `auth_password`), `allowed_hosts`, upload limits, chime/lightshow folders, signing `secret_key` |
| `offline_ap` | AP fallback behavior, force mode, internet sharing |
| `system` | watchdog + sysctl startup behavior |
| `telegram` | alerting configuration, video quality, offline queueing |
| `webhook` | HTTP callback URL and HMAC-SHA256 signing secret |
| `update` | update strategy and channel |
| `viewer_prefs` | speed unit, HUD scale, map scale |

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
- USB gadget state recovery for hung gadget conditions

### Video management

- Browse SavedClips, SentryClips, and RecentClips by event
- Play videos with multi-camera switching and HTTP range support
- Encrypted clip detection (Tesla-locked files displayed distinctly)
- SEI telemetry extraction — GPS, speed, battery, gear, and more as overlay or JSON
- HUD overlay and interactive map overlay with configurable scale
- Download individual video files or full events as ZIP (all cameras + metadata)
- Per-event thumbnails auto-generated from the first available camera
- Session grouping for RecentClips
- Optional secondary archive path for historical TeslaCam data

### Chime management

- Upload, rename, delete, and preview lock chimes
- Set the active lock chime
- **Chime scheduling** — weekly, specific date, holiday, or recurring schedules
- **Chime groups** — organize chimes for random selection
- **Random mode** — randomly pick from a configured group on each event or at boot
- Duration constraints enforced for Tesla compatibility

### Lightshow and wraps

- Upload, download, and delete light shows (FSEQ + audio pairs)
- Upload, thumbnail-preview, download, and delete wrap images

### Music management

- Hierarchical directory browsing
- Upload (single or chunked for large files), stream, move, rename, delete
- Create and delete directories

### Analytics and health

- Storage health scoring with alerts and recommendations
- Partition usage stats per LUN
- Video statistics and recording-time estimates by folder
- System metrics: CPU usage, temperature, clock speed, RAM, and power consumption

### Cleanup

- Per-folder retention policies (keep last N events for SavedClips, SentryClips, RecentClips)
- Dry-run preview before executing
- Optional boot-time cleanup

### Networking and connectivity

- **Access Point** — offline AP fallback with configurable SSID, passphrase, channel, and DHCP; force-on/auto/force-off modes; RSSI-based activation threshold; reconnect grace period and retry cycles
- **AP internet sharing** — NAT routing for AP clients through the Pi's upstream (WiFi or Bluetooth)
- **WiFi client management** — scan networks, connect, view saved networks, forget, set autoconnect priority
- **WiFi reconnect reliability** — signal strength monitoring, grace periods, and retry cycles before AP activates
- **Bluetooth tethering (PAN)** — manage the Bluetooth adapter (power, discoverable mode), scan, pair, connect, and disconnect devices for phone-tethered internet upstream
- **Captive portal detection** — responds to standard probes from Apple, Android, and Windows clients

### Notifications

- **Telegram** — send Sentry event clips via bot; configurable HD/SD quality; offline queueing with configurable max queue size
- **Webhook** — HTTP POST callbacks with optional HMAC-SHA256 request signing

### Samba file sharing

- Enable/disable Samba service
- Set Samba password and regenerate config via web UI
- Restart service from Settings page

### System power

- Reboot and power off from the web UI

### Updates

- Check for updates at startup
- Optional automatic binary updates
- Update channels: stable, beta, dev

### Real-time features

- Server-Sent Events (SSE) for live log streaming and Sentry event notifications
- System metrics dashboard with periodic polling

### Display preferences (server-persisted)

- Speed unit (kph / mph)
- HUD overlay scale
- Map overlay scale

## Acknowledgements

Argus is **heavily based on [TeslaUSB](https://github.com/mphacker/TeslaUSB)** concepts and workflow.  
Huge thanks to the TeslaUSB project and contributors.

## License

See repository files for license information.
