#!/usr/bin/env bash
# fetch-vmlinux.sh — acquire an uncompressed Linux kernel (vmlinux) for
# Firecracker microVMs, following the pin-and-verify pattern (D37/D51).
#
# Source: AWS S3 CI artifacts (spec.ccfc.min) — the same images Firecracker
# CI uses. Pinned to a specific date prefix.
#
# Usage: tools/fetch-vmlinux.sh
# Env:   FORCE_VMLINUX_REBUILD=1 to re-download.
#
# Result: tools/bin/vmlinux (uncompressed ELF kernel, x86_64).
# GĐ4 mục 1 (issue #193).
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
BIN_DIR="$HERE/bin"
STAMP="$BIN_DIR/.vmlinux-stamp"

ARCH="$(uname -m)"
case "$ARCH" in
    x86_64)  VM_ARCH="x86_64" ;;
    aarch64) VM_ARCH="aarch64" ;;
    *) echo "fetch-vmlinux: unsupported arch $ARCH"; exit 1 ;;
esac

# Pinned kernel artifact from Firecracker CI S3.
# sha256 MUST be re-measured when the prefix changes.
VMLINUX_SHA256="0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
S3_PREFIX="https://s3.amazonaws.com/spec.ccfc.min/firecracker-ci/2026-01-01"

mkdir -p "$BIN_DIR"

if [[ -f "$STAMP" && -f "$BIN_DIR/vmlinux" \
      && -z "${FORCE_VMLINUX_REBUILD:-}" ]]; then
    echo "fetch-vmlinux: cached (stamp=$STAMP)"
    exit 0
fi

echo "fetch-vmlinux: downloading (${VM_ARCH})..."

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

URL="${S3_PREFIX}/kernels/vmlinux-${VM_ARCH}.bin"
curl -fsSL -o "$TMP/vmlinux" "$URL"

ACTUAL=$(sha256sum "$TMP/vmlinux" | awk '{print $1}')
if [[ "$ACTUAL" != "$VMLINUX_SHA256" ]]; then
    echo "fetch-vmlinux: sha256 mismatch" >&2
    echo "  expected: $VMLINUX_SHA256" >&2
    echo "  got:      $ACTUAL" >&2
    exit 1
fi

mv "$TMP/vmlinux" "$BIN_DIR/vmlinux"
echo "pinned" > "$STAMP"
echo "fetch-vmlinux: OK → tools/bin/vmlinux"
