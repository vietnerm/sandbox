#!/usr/bin/env bash
# mkrootfs-alpine.sh — build an Alpine minirootfs rootfs (~10MB) WITHOUT root.
#
# Source: alpine-minirootfs tarball pinned to ALPINE_VERSION, downloaded from
# the official CDN and VERIFIED before extraction:
#   1. PINNED sha256 (supply-chain, SIA audit PR #24): the hash of the default
#      version is hardcoded below, cross-checked at pin time against THREE
#      independent sources (dl-cdn.alpinelinux.org, mirrors.edge.kernel.org,
#      mirror.leaseweb.com — all three publish the same digest for 3.22.8,
#      measured 2026-11-17). The CDN .sha256 alone only catches a corrupted
#      download — it comes from the same origin as the tarball and CANNOT
#      catch a tampered tarball (audit, medium).
#   2. CDN-published .sha256 (transport integrity) — kept as belt.
# An override ALPINE_VERSION=<other> has no pin — the script warns loudly and
# falls back to CDN self-consistency only (weaker, documented) so test
# automation can still target other versions knowingly.
#
# Ownership model (D20, manifest §5 single-uid mapping): GNU tar running as a
# non-root user defaults to --no-same-owner, so every extracted file is owned
# by the calling uid. With minict's map "0 -> real_uid" that uid IS root
# inside the sandbox — the natural fakeroot equivalent (Apptainer
# FAKEROOTDONTTRYCHOWN=1 pattern), no fakeroot/newuidmap needed.
#
# Usage: ./mkrootfs-alpine.sh [target-dir]   (default: rootfs-alpine)
set -euo pipefail

TARGET="${1:-rootfs-alpine}"
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"

ALPINE_VERSION="${ALPINE_VERSION:-3.22.8}"
ALPINE_BRANCH="${ALPINE_VERSION%.*}"   # 3.22.8 -> 3.22 (URL derived, audit low #4)
ALPINE_ARCH="${ALPINE_ARCH:-x86_64}"
MIRROR="${ALPINE_MIRROR:-https://dl-cdn.alpinelinux.org}"
TARBALL="alpine-minirootfs-${ALPINE_VERSION}-${ALPINE_ARCH}.tar.gz"
URL="${MIRROR}/alpine/v${ALPINE_BRANCH}/releases/${ALPINE_ARCH}/${TARBALL}"

# Pinned digests: <version>-<arch> -> sha256. Verified against three mirrors
# at pin time; extend this table (with the same 3-source check) per release.
pin_lookup() {
    case "$1" in
        alpine-minirootfs-3.22.8-x86_64)
            printf '%s' "2d7f46af9b7e86ad64ebeba3fe9b75b62dd5b259e708afb2ab21268f627696e5" ;;
        *) printf '%s' "" ;;
    esac
}
PIN_KEY="alpine-minirootfs-${ALPINE_VERSION}-${ALPINE_ARCH}"
PINNED="$(pin_lookup "$PIN_KEY")"

if [[ -x "$TARGET/bin/busybox" && -f "$TARGET/etc/alpine-release" ]]; then
    echo "alpine rootfs '$TARGET' already exists, skipping (rm -rf it to rebuild)"
    exit 0
fi

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

echo "==> downloading $TARBALL (+sha256) — pin $ALPINE_VERSION, verify mandatory"
curl -fsSL --retry 3 --retry-delay 2 -o "$WORK/$TARBALL" "$URL"
curl -fsSL --retry 3 --retry-delay 2 -o "$WORK/$TARBALL.sha256" "$URL.sha256"
(cd "$WORK" && sha256sum -c "$TARBALL.sha256" >/dev/null) \
    || { echo "sha256 MISMATCH for $TARBALL — aborting"; exit 1; }
MEASURED=$(cd "$WORK" && sha256sum "$TARBALL" | cut -d' ' -f1)
if [[ -n "$PINNED" ]]; then
    if [[ "$MEASURED" != "$PINNED" ]]; then
        echo "FATAL: sha256 does NOT match the PINNED digest (supply chain?)"
        echo "  measured: $MEASURED"
        echo "  pinned  : $PINNED"
        exit 1
    fi
    echo "==> sha256 OK (pinned)"
else
    echo "WARNING: no pinned digest for $PIN_KEY — verification is CDN"
    echo "WARNING: self-consistency only (weaker, SIA audit PR #24); pin it"
    echo "==> sha256 OK (unpinned)"
fi

echo "==> extracting (GNU tar non-root => --no-same-owner, D20)"
mkdir -p "$TARGET"
tar -xzf "$WORK/$TARBALL" -C "$TARGET" --no-same-owner

echo "==> assembling minict specifics"
# /dev placeholders: minict bind-mounts host nodes over these at runtime
# (mknod is EPERM inside userns — same trick as mkrootfs.sh)
touch "$TARGET"/dev/{null,zero,urandom,random}
chmod 0666 "$TARGET"/dev/{null,zero,urandom,random}

# MARKER identity: prove the sandbox sees OUR passwd, not the host's
# (mirror of the busybox rootfs convention used by the isolation suite).
# Alpine's real passwd is kept intact on top.
grep -q minict-sandbox "$TARGET/etc/passwd" || cat >> "$TARGET/etc/passwd" <<'EOF'
minict-sandbox:x:1000:1000:minict sandbox user:/home/minict:/bin/sh
EOF
grep -q "minict-sandbox" "$TARGET/etc/group" || cat >> "$TARGET/etc/group" <<'EOF'
minict-sandbox:x:1000:
EOF
echo "minict" > "$TARGET/etc/hostname"

cat > "$TARGET/etc/profile.d/minict.sh" <<'EOF'
export PS1='minict-alpine> '
EOF
mkdir -p "$TARGET/tmp"
chmod 1777 "$TARGET/tmp"

# sanity: minict requires /bin/busybox; minirootfs ships it (busybox static)
[[ -x "$TARGET/bin/busybox" ]] || { echo "no /bin/busybox in minirootfs?!"; exit 1; }
[[ -x "$TARGET/sbin/apk" ]] || { echo "no /sbin/apk in minirootfs?!"; exit 1; }
# D36 residual: alpine 3.22.8 tarball ships /tmp + /home, but empty dirs
# were observed MISSING after intermittent builds (2/4 runs 2026-11-19,
# issue #55) — create them unconditionally, fail loud on failure
mkdir -p "$TARGET/home/minict" "$TARGET/media" "$TARGET/root" "$TARGET/run" \
         "$TARGET/srv" "$TARGET/sys"

SIZE=$(du -sh "$TARGET" | cut -f1)
echo "==> done: $TARGET ($SIZE, alpine $(cat "$TARGET/etc/alpine-release" 2>/dev/null || echo '?'))"
