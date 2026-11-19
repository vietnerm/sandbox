#!/usr/bin/env bash
# fetch-pasta.sh — acquire the pasta binary (passt project) WITHOUT root,
# following the pin-and-verify pattern of mkrootfs-alpine.sh (D20):
#
#   1. PINNED version + sha256 of the source tarball, measured at pin time
#      (2026-11-19, D37) directly from the canonical cgit at passt.top.
#      passt publishes no official prebuilt static binaries for download,
#      so we build from the pinned snapshot with `make static` (gcc only,
#      no extra deps — passt bundles its own seccomp/BPF generator).
#   2. Fail-closed: wrong digest aborts BEFORE any build. Re-download uses
#      the same digest. Binary is NOT vendored into the repo (disk budget
#      20 MB, D34) — it lands in tools/bin/, which is gitignored.
#
# Usage: tools/fetch-pasta.sh [PASST_VERSION]
# Env:   FORCE_PASTA_REBUILD=1 to rebuild from the cached tarball.
#
# Result: tools/bin/pasta (static ELF, self-contained; the `passt` binary
# with `pasta` semantics — same binary, different argv[0]).
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
BIN_DIR="$HERE/bin"
CACHE_DIR="$HERE/cache"

PASST_VERSION="${1:-2026_10_02.cba3570}"
PASST_URL="https://passt.top/passt/snapshot/passt-${PASST_VERSION}.tar.gz"

# Pinned digest (measured 2026-11-19 from passt.top cgit, cross-checked by
# re-download in the same session; single canonical source exists — residual
# risk documented in journal/2026-11-19/decisions/pasta-egress.md D37).
pin_lookup() {
    case "$1" in
        passt-2026_10_02.cba3570.tar.gz)
            echo "7b81892b1070521ed217de672cdce43d7a37436e6f850cd04f2b63919ac484b6" ;;
        *) echo "" ;;
    esac
}

TARBALL="passt-${PASST_VERSION}.tar.gz"
PINNED="$(pin_lookup "$TARBALL")"
if [[ -z "$PINNED" ]]; then
    echo "fetch-pasta: no pin for '$TARBALL' — refusing (add one with a 2-source check)" >&2
    exit 1
fi

STAMP="$BIN_DIR/.pasta-stamp"
if [[ -x "$BIN_DIR/pasta" && -f "$STAMP" ]] && [[ "$(cat "$STAMP")" == "$PASST_VERSION" ]] \
   && [[ "${FORCE_PASTA_REBUILD:-0}" != "1" ]]; then
    exit 0
fi

mkdir -p "$BIN_DIR" "$CACHE_DIR"

echo "==> downloading $TARBALL — pin $PASST_VERSION, verify mandatory"
curl -sL --fail -o "$CACHE_DIR/$TARBALL" "$PASST_URL"
ACTUAL="$(sha256sum "$CACHE_DIR/$TARBALL" | cut -d' ' -f1)"
if [[ "$ACTUAL" != "$PINNED" ]]; then
    echo "fetch-pasta: sha256 MISMATCH for $TARBALL" >&2
    echo "  expected: $PINNED" >&2
    echo "  actual:   $ACTUAL" >&2
    rm -f "$CACHE_DIR/$TARBALL"
    exit 1
fi
echo "==> sha256 verified: $ACTUAL"

echo "==> building static pasta (make static — no root, gcc only)"
BUILD_DIR="$(mktemp -d)"
trap 'rm -rf "$BUILD_DIR"' EXIT
tar -xzf "$CACHE_DIR/$TARBALL" -C "$BUILD_DIR"
make -C "$BUILD_DIR/passt-${PASST_VERSION}" static "VERSION=${PASST_VERSION}" >/dev/null

SRC="$BUILD_DIR/passt-${PASST_VERSION}/passt"
[[ -x "$SRC" ]] || { echo "fetch-pasta: build produced no binary" >&2; exit 1; }
cp "$SRC" "$BIN_DIR/pasta"
chmod 0755 "$BIN_DIR/pasta"
# passt's non-AVX2 entry point re-execs the sibling '<name>.avx2' when present
# and warns loudly on stderr otherwise — install the AVX2 variant alongside to
# keep pasta.log clean (fallback to the plain binary is automatic).
if [[ -x "$BUILD_DIR/passt-${PASST_VERSION}/passt.avx2" ]]; then
    cp "$BUILD_DIR/passt-${PASST_VERSION}/passt.avx2" "$BIN_DIR/pasta.avx2"
    chmod 0755 "$BIN_DIR/pasta.avx2"
fi
echo "$PASST_VERSION" > "$STAMP"

echo "==> pasta installed: $BIN_DIR/pasta ($("$BIN_DIR/pasta" --version | head -1))"
