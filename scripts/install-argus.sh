#!/usr/bin/env bash
# Install the latest Argus release binary to /usr/local/bin/argus (Linux / Raspberry Pi).
# Usage: curl -fsSL https://raw.githubusercontent.com/ulm0/argus/main/scripts/install-argus.sh | sudo bash
set -euo pipefail

case "$(uname -m)" in
aarch64) ARGUS_ARCH=arm64 ;;
armv7l) ARGUS_ARCH=armv7 ;;
*) ARGUS_ARCH=armv6 ;;
esac

TAG="$(curl -fsSL https://api.github.com/repos/ulm0/argus/releases/latest | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -1)"
VERSION="${TAG#v}"
URL="https://github.com/ulm0/argus/releases/download/${TAG}/argus_${VERSION}_linux_${ARGUS_ARCH}"

TMP="$(mktemp)"
trap 'rm -f "$TMP"' EXIT

curl -fsSL "$URL" -o "$TMP"
install -m 0755 "$TMP" /usr/local/bin/argus
echo "Installed /usr/local/bin/argus (${TAG} linux_${ARGUS_ARCH})"
echo "Next: argus generate && sudo argus setup"
