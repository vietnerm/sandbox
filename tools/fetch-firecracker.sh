#!/usr/bin/env bash
# fetch-firecracker.sh — acquire the Firecracker binary WITHOUT root, following
# the pin-and-verify pattern of tools/fetch-crane.sh (D37/D51):
#
#   1. PINNED version + sha256, measured from the official release assets
#      (github.com/firecracker-microvm/firecracker/releases):
#        - the .tgz archive digest published as .tgz.sha256.txt
#        - the inner binary digest published as SHA256SUMS inside the archive
#      Both are verified BEFORE anything is installed.
#   2. Fail-closed: any digest mismatch aborts BEFORE any install. Binaries
#      land in tools/bin/ (gitignored, disk budget 20 MB — D34).
#   3. Firecracker publishes statically-linked release binaries (musl,
#      static-pie) — no runtime deps.
#
# NOTE (D85, issue #231): only the `firecracker` VMM binary is installed.
# The upstream `jailer` binary is NOT used by minictd — it requires root
# (chroot, mknod, setuid, cgroup), violating the §0 rootless invariant; the
# jail role is played by a minict instance (internal/firecracker/jail.go).
#
# Usage: tools/fetch-firecracker.sh [FIRECRACKER_VERSION]
# Env:   FORCE_FC_REBUILD=1 to re-download over the stamp.
#
# Result: tools/bin/firecracker (static ELF x86_64).
# GĐ4 mục 1 (issue #193, precondition #191: /dev/kvm required at RUNTIME,
# not at FETCH time — the binary itself fetches fine without KVM).
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
BIN_DIR="$HERE/bin"
STAMP="$BIN_DIR/.firecracker-stamp"

FC_VERSION="${1:-v1.18.0}"
ARCH="$(uname -m)"
case "$ARCH" in
    x86_64)  FC_ARCH="x86_64" ;;
    aarch64) FC_ARCH="aarch64" ;;
    *) echo "fetch-firecracker: unsupported arch $ARCH"; exit 1 ;;
esac

# sha256 digests, measured from the official release page (2026-11-27, Phiên 45).
# These MUST be re-measured when FC_VERSION changes.
#   firecracker-v1.18.0-x86_64.tgz            (published .tgz.sha256.txt)
#   firecracker-v1.18.0-x86_64 (inner binary, published SHA256SUMS)
case "${FC_VERSION}-${FC_ARCH}" in
    v1.18.0-x86_64)
        TGZ_SHA256="1ac8eec866dd5eea14efecf6323e22b23eea24054545338a5b34bc5bca6623cd"
        FC_SHA256="b33f46cd6d88834cf6a4f8d184f08f219809c636236bffd286d233a60602a2ba"
        ;;
    *) echo "fetch-firecracker: no pinned digest for ${FC_VERSION}-${FC_ARCH} — add one first" >&2; exit 1 ;;
esac

BASE_URL="https://github.com/firecracker-microvm/firecracker/releases/download/${FC_VERSION}"

mkdir -p "$BIN_DIR"

if [[ -f "$STAMP" && -x "$BIN_DIR/firecracker" \
      && -z "${FORCE_FC_REBUILD:-}" ]]; then
    echo "fetch-firecracker: cached (stamp=$STAMP)"
    exit 0
fi

echo "fetch-firecracker: downloading ${FC_VERSION} (${FC_ARCH})..."

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

# Release archive + its officially published digest (cross-check first).
curl -fsSL -o "$TMP/fc.tgz" "${BASE_URL}/firecracker-${FC_VERSION}-${FC_ARCH}.tgz"
ACTUAL_TGZ=$(sha256sum "$TMP/fc.tgz" | awk '{print $1}')
if [[ "$ACTUAL_TGZ" != "$TGZ_SHA256" ]]; then
    echo "fetch-firecracker: sha256 mismatch (archive)" >&2
    echo "  expected: $TGZ_SHA256" >&2
    echo "  got:      $ACTUAL_TGZ" >&2
    exit 1
fi

tar xzf "$TMP/fc.tgz" -C "$TMP"
INNER="$TMP/release-${FC_VERSION}-${FC_ARCH}/firecracker-${FC_VERSION}-${FC_ARCH}"
ACTUAL_FC=$(sha256sum "$INNER" | awk '{print $1}')
if [[ "$ACTUAL_FC" != "$FC_SHA256" ]]; then
    echo "fetch-firecracker: sha256 mismatch (firecracker binary)" >&2
    echo "  expected: $FC_SHA256" >&2
    echo "  got:      $ACTUAL_FC" >&2
    exit 1
fi

chmod +x "$INNER"
mv "$INNER" "$BIN_DIR/firecracker"
echo "$FC_VERSION" > "$STAMP"
echo "fetch-firecracker: OK ${FC_VERSION} → tools/bin/firecracker"
