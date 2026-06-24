#!/usr/bin/env bash
# Install the latest Argus release binary to /usr/local/bin/argus (Linux / Raspberry Pi).
# Usage: curl -fsSL https://raw.githubusercontent.com/ulm0/argus/main/scripts/install-argus.sh | sudo bash
set -euo pipefail

case "$(uname -m)" in
aarch64) ARGUS_ARCH=arm64 ;;
armv7l) ARGUS_ARCH=armv7 ;;
armv6l) ARGUS_ARCH=armv6 ;;
*) echo "Unsupported architecture: $(uname -m) (Argus ships arm64/armv7/armv6 Linux builds)" >&2; exit 1 ;;
esac

TAG="$(curl -fsSL https://api.github.com/repos/ulm0/argus/releases/latest | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -1)"
if [ -z "${TAG:-}" ]; then
  echo "Could not determine the latest Argus release tag" >&2
  exit 1
fi
VERSION="${TAG#v}"
ASSET="argus_${VERSION}_linux_${ARGUS_ARCH}"
BASE="https://github.com/ulm0/argus/releases/download/${TAG}"

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

curl -fsSL "${BASE}/${ASSET}" -o "$TMP/argus"
curl -fsSL "${BASE}/checksums.txt" -o "$TMP/checksums.txt"

# Verify the downloaded binary against the release's published sha256 checksum
# before installing it as a root-run binary. This catches a corrupted or
# tampered download (the checksum file itself is served over GitHub HTTPS).
EXPECTED="$(sed -n "s/^\([0-9a-f]\{64\}\)[[:space:]]\{1,\}${ASSET}\$/\1/p" "$TMP/checksums.txt" | head -1)"
if [ -z "$EXPECTED" ]; then
  echo "No checksum found for ${ASSET} in checksums.txt" >&2
  exit 1
fi
ACTUAL="$(sha256sum "$TMP/argus" | awk '{print $1}')"
if [ "$EXPECTED" != "$ACTUAL" ]; then
  echo "Checksum mismatch for ${ASSET}: expected ${EXPECTED}, got ${ACTUAL}" >&2
  exit 1
fi

install -m 0755 "$TMP/argus" /usr/local/bin/argus
echo "Installed /usr/local/bin/argus (${TAG} linux_${ARGUS_ARCH}, sha256 verified)"
echo "Next: argus generate && sudo argus setup"
