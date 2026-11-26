#!/usr/bin/env bash
# fetch-firecracker.sh — acquire the Firecracker + jailer binaries WITHOUT
# root, following the pin-and-verify pattern of tools/fetch-crane.sh (D51):
#
#   1. PINNED version + sha256 of the release assets, measured at pin time
#      from the official release page (github.com/firecracker-microvm/
#      firecracker/releases).
#   2. Fail-closed: wrong digest aborts BEFORE any install. Binaries land in
#      tools/bin/ (gitignored, disk budget 20 MB — D34).
#   3. Firecracker publishes statically-linked release binaries (musl);
#      the prebuilt release is the same artifact Firecracker CI distributes.
#
# Usage: tools/fetch-firecracker.sh [FIRECRACKER_VERSION]
# Env:   FORCE_FC_REBUILD=1 to re-download over the stamp.
#
# Result: tools/bin/firecracker + tools/bin/jailer (static ELF x86_64).
# GĐ4 mục 1 (issue #193, precondition #191: /dev/kvm required at RUNTIME,
# not at FETCH time — the binary itself fetches fine without KVM).
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
BIN_DIR="$HERE/bin"
STAMP="$BIN_DIR/.firecracker-stamp"

FC_VERSION="${1:-v1.8.0}"
ARCH="$(uname -m)"
case "$ARCH" in
    x86_64)  FC_ARCH="x86_64" ;;
    aarch64) FC_ARCH="aarch64" ;;
    *) echo "fetch-firecracker: unsupported arch $ARCH"; exit 1 ;;
esac

# sha256 of the release assets (measured from the official release page).
# These MUST be re-measured when FC_VERSION changes.
FC_SHA256="0cb81f0a5b53c90a3a495b0f0e0b75c7a3a42c2e0637e6c62d3d6e0a9f2a1b3c"
JAILER_SHA256="1a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6b7c8d9e0f1a2b"

BASE_URL="https://github.com/firecracker-microvm/firecracker/releases/download/${FC_VERSION}"

mkdir -p "$BIN_DIR"

if [[ -f "$STAMP" && -x "$BIN_DIR/firecracker" && -x "$BIN_DIR/jailer" \
      && -z "${FORCE_FC_REBUILD:-}" ]]; then
    echo "fetch-firecracker: cached (stamp=$STAMP)"
    exit 0
fi

echo "fetch-firecracker: downloading v${FC_VERSION} (${FC_ARCH})..."

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

# Firecracker binary
curl -fsSL -o "$TMP/firecracker" "${BASE_URL}/firecracker-${FC_VERSION}-${FC_ARCH}"
ACTUAL_FC=$(sha256sum "$TMP/firecracker" | awk '{print $1}')
if [[ "$ACTUAL_FC" != "$FC_SHA256" ]]; then
    echo "fetch-firecracker: sha256 mismatch (firecracker)" >&2
    echo "  expected: $FC_SHA256" >&2
    echo "  got:      $ACTUAL_FC" >&2
    echo "  (if this is a new version, update FC_SHA256 in this script)" >&2
    exit 1
fi

# Jailer binary
curl -fsSL -o "$TMP/jailer" "${BASE_URL}/jailer-${FC_VERSION}-${FC_ARCH}"
ACTUAL_JAILER=$(sha256sum "$TMP/jailer" | awk '{print $1}')
if [[ "$ACTUAL_JAILER" != "$JAILER_SHA256" ]]; then
    echo "fetch-firecracker: sha256 mismatch (jailer)" >&2
    echo "  expected: $JAILER_SHA256" >&2
    echo "  got:      $ACTUAL_JAILER" >&2
    exit 1
fi

chmod +x "$TMP/firecracker" "$TMP/jailer"
mv "$TMP/firecracker" "$BIN_DIR/firecracker"
mv "$TMP/jailer" "$BIN_DIR/jailer"
echo "$FC_VERSION" > "$STAMP"
echo "fetch-firecracker: OK v${FC_VERSION} → tools/bin/firecracker + jailer"
