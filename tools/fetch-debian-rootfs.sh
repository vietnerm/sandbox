#!/usr/bin/env bash
# fetch-debian-rootfs.sh — acquire the Debian rootfs (bookworm-slim, amd64)
# WITHOUT root, following the pin-and-verify pattern of fetch-pasta.sh (D37):
#
#   1. PINNED tarball + sha256 + size, measured at pin time (2026-11-20, D46)
#      from the canonical debuerreotype/docker-debian-artifacts repo (branch
#      dist-amd64) — the same rootfs the official `debian:bookworm-slim`
#      Docker image ships, built by debuerreotype 0.17 from
#      snapshot.debian.org (epoch 1794787200 = 2026-11-16).
#      Integrity: the OCI layer digest published in the repo's
#      image-manifest.json — verified by download + sha256sum in the same
#      session (evidence: journal/2026-11-20/decisions/debian-rootfs.md D46).
#   2. Fail-closed: wrong digest or size aborts BEFORE any unpack.
#      Re-download uses the same pin.
#   3. NOT tracked in git (~83 MB unpacked vs 20 MB disk budget — measured,
#      D46): lands in rootfs-debian/, which is gitignored — same class as
#      rootfs-alpine/ (rebuilt when missing, D36) and tools/bin/pasta (D37).
#
# Why debuerreotype and not images.linuxcontainers.org: smaller (28 MB
# compressed), digest-pinnable via the OCI manifest, and it IS the official
# Debian Docker rootfs (single provenance chain, debuerreotype-epoch pinned).
# lxc/distrobuilder images are documented as the fallback source (research.md).
#
# Usage: tools/fetch-debian-rootfs.sh [DEST_DIR]
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
REPO="$(cd "$HERE/.." && pwd)"
DEST="${1:-$REPO/rootfs-debian}"

DEB_SUITE="bookworm"
DEB_VARIANT="slim"
DEB_ARCH="amd64"
DEB_VERSION="12.15"
DEB_EPOCH="1794787200"

# dist-amd64 branch, bookworm/slim OCI blob; digest = the layer digest from
# image-manifest.json @ that branch (measured + verified 2026-11-20, D46).
DEB_URL="https://raw.githubusercontent.com/debuerreotype/docker-debian-artifacts/dist-amd64/bookworm/slim/oci/blobs/rootfs.tar.gz"
DEB_SHA256="2a7a0929687462067917a1ed0236948de0b43ef9a546c9a930540f6218d7dfd0"
DEB_SIZE="28247934"

STAMP="$DEST/.debian-stamp"
if [[ -f "$STAMP" && -e "$DEST/bin/sh" ]] \
   && [[ "$(cat "$STAMP")" == "$DEB_SHA256" ]]; then
    # still ensure device placeholders exist (older fetches predating the
    # placeholder step — idempotent, mirrors mkrootfs.sh)
    touch "$DEST"/dev/{null,zero,urandom,random}
    chmod 0666 "$DEST"/dev/{null,zero,urandom,random} 2>/dev/null || true
    exit 0  # already fetched + verified
fi

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

TARBALL="$WORK/rootfs.tar.gz"
echo "fetch-debian-rootfs: downloading pinned ${DEB_SUITE}-${DEB_VARIANT}/${DEB_ARCH} (${DEB_VERSION}, epoch ${DEB_EPOCH})..."
curl -fSL --retry 3 --retry-delay 2 --max-time 300 \
    -o "$TARBALL" "$DEB_URL"

GOT_SHA="$(sha256sum "$TARBALL" | awk '{print $1}')"
if [[ "$GOT_SHA" != "$DEB_SHA256" ]]; then
    echo "fetch-debian-rootfs: sha256 MISMATCH (got $GOT_SHA, want $DEB_SHA256) — refusing (fail-closed, D37)" >&2
    exit 1
fi
GOT_SIZE=$(stat -c %s "$TARBALL")
if [[ "$GOT_SIZE" != "$DEB_SIZE" ]]; then
    echo "fetch-debian-rootfs: size MISMATCH (got $GOT_SIZE, want $DEB_SIZE) — refusing (fail-closed)" >&2
    exit 1
fi

# deterministic unpack: replace the whole dir, never merge into a stale one
rm -rf "$DEST"
mkdir -p "$DEST"
# --no-same-owner: single-uid mapping (D20) — foreign-uid entries cannot be
# preserved for a non-root extractor; the kernel maps ns-root to our uid.
tar -xzf "$TARBALL" -C "$DEST" --no-same-owner

# fail-closed sanity: exactly what minict (D46) and the wrapper (D43) require
_sh="$DEST/bin/sh"
if [[ -L "$_sh" ]]; then
    _ltgt="$(readlink "$_sh")"
    case "$_ltgt" in
        /*) _sh="$DEST$_ltgt" ;;
        *)  _sh="$(dirname "$_sh")/$_ltgt" ;;
    esac
fi
if [[ ! -e "$_sh" || ! -x "$_sh" ]]; then
    echo "fetch-debian-rootfs: unpacked rootfs has no executable /bin/sh (after symlink resolve: $_sh) — refusing" >&2
    exit 1
fi
if [[ ! -x "$DEST/usr/bin/dpkg" ]]; then
    echo "fetch-debian-rootfs: /usr/bin/dpkg missing — not a usable Debian rootfs" >&2
    exit 1
fi

# device placeholder files — mirror mkrootfs.sh (busybox): mknod is EPERM in
# userns, so minict bind-mounts host /dev/{null,zero,random,urandom} over
# empty placeholders (O_CREAT fallback measured 2026-11-20: without them the
# FIRST one-shot run creates them in a shared rootfs and DB8 pristine fails
# — CI run 3 diff named exactly these four files)
touch "$DEST"/dev/{null,zero,urandom,random}
chmod 0666 "$DEST"/dev/{null,zero,urandom,random} 2>/dev/null || true

printf '%s\n' "$DEB_SHA256" > "$STAMP"
echo "fetch-debian-rootfs: OK — $DEST ($(du -sk "$DEST" | awk '{print $1}') KB, sha256 verified)"
