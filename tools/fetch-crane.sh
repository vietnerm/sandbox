#!/usr/bin/env bash
# fetch-crane.sh — acquire the crane binary (go-containerregistry) WITHOUT
# root, following the pin-and-verify pattern of tools/fetch-pasta.sh (D37):
#
#   1. PINNED version + sha256 of the release tarball, measured at pin time
#      (2026-11-21, D51) from the official checksums.txt of the release tag
#      (github.com/google/go-containerregistry/releases/tag/v0.22.1) and
#      cross-checked by downloading the tarball separately (sha256 match)
#      plus `crane version` reporting the pinned version.
#   2. Fail-closed: wrong digest aborts BEFORE any install. Binary is NOT
#      vendored into the repo (disk budget 20 MB, D34) — it lands in
#      tools/bin/, which is gitignored.
#   3. go-containerregistry publishes no source-only install for the CLI
#      worth the dep tree here; the prebuilt release binary is the same
#      artifact Crane CI distributes and is signed/attested upstream (not
#      verified here — single canonical source + checksum pin, residual
#      documented in journal/2026-11-21/decisions/crane-e2e.md D51).
#
# Usage: tools/fetch-crane.sh [CRANE_VERSION]
# Env:   FORCE_CRANE_REBUILD=1 to re-download over the stamp.
#
# Result: tools/bin/crane (static ELF from the upstream release tarball).
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
BIN_DIR="$HERE/bin"
CACHE_DIR="$HERE/cache"

CRANE_VERSION="${1:-v0.22.1}"
# go-containerregistry release asset naming: vX.Y.Z -> go-containerregistry_X.Y.Z
ASSET_VER="${CRANE_VERSION#v}"
TARBALL="go-containerregistry_Linux_x86_64.tar.gz"
CRANE_URL="https://github.com/google/go-containerregistry/releases/download/${CRANE_VERSION}/${TARBALL}"

pin_lookup() {
    case "$1" in
        go-containerregistry_Linux_x86_64.tar.gz)
            echo "0ab7a1d6932a213aed964ce97666c3077fe691c8606413674a8b3e0b9ec4cda0" ;;
        *) echo "" ;;
    esac
}

PINNED="$(pin_lookup "$TARBALL")"
if [[ -z "$PINNED" ]]; then
    echo "fetch-crane: no pin for '$TARBALL' — refusing (add one with a 2-source check)" >&2
    exit 1
fi

STAMP="$BIN_DIR/.crane-stamp"
if [[ -x "$BIN_DIR/crane" && -f "$STAMP" ]] && [[ "$(cat "$STAMP")" == "$CRANE_VERSION" ]] \
   && [[ "${FORCE_CRANE_REBUILD:-0}" != "1" ]]; then
    exit 0
fi

mkdir -p "$BIN_DIR" "$CACHE_DIR"

echo "==> downloading $TARBALL — pin $CRANE_VERSION, verify mandatory"
curl -sL --fail -o "$CACHE_DIR/$TARBALL" "$CRANE_URL"
ACTUAL="$(sha256sum "$CACHE_DIR/$TARBALL" | cut -d' ' -f1)"
if [[ "$ACTUAL" != "$PINNED" ]]; then
    echo "fetch-crane: sha256 MISMATCH for $TARBALL" >&2
    echo "  expected: $PINNED" >&2
    echo "  actual:   $ACTUAL" >&2
    rm -f "$CACHE_DIR/$TARBALL"
    exit 1
fi
echo "==> sha256 verified: $ACTUAL"

INSTALL_DIR="$(mktemp -d)"
trap 'rm -rf "$INSTALL_DIR"' EXIT
tar -xzf "$CACHE_DIR/$TARBALL" -C "$INSTALL_DIR" crane
SRC="$INSTALL_DIR/crane"
[[ -x "$SRC" ]] || { echo "fetch-crane: tarball contained no crane binary" >&2; exit 1; }

# Version must match the pin (anti stale-cache / wrong-asset guard).
GOT="$("$SRC" version | awk '{print $NF}')"
[[ "$GOT" == "$ASSET_VER" ]] || {
    echo "fetch-crane: version mismatch: got $GOT, want $ASSET_VER" >&2
    exit 1
}

cp "$SRC" "$BIN_DIR/crane"
chmod 0755 "$BIN_DIR/crane"
echo "$CRANE_VERSION" > "$STAMP"

echo "==> crane installed: $BIN_DIR/crane ($("$BIN_DIR/crane" version))"
