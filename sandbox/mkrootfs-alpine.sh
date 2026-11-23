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

# STACK PACKAGES (GĐ1 mục 4, issue #85, D45): nginx + sqlite + fcgiwrap +
# spawn-fcgi pre-installed at BUILD TIME. Installation runs the rootfs's own
# apk via its musl loader on the HOST (apk is dynamic — cannot run natively
# outside the rootfs): "$T/lib/ld-musl-x86_64.so.1 --library-path ..." +
# `apk add --root $T` as the CALLING uid. Every installed file is owned by
# that uid, which IS root inside the sandbox (single-uid map, D20) —
# nginx/sqlite can then write their dirs. This deliberately avoids the AL7
# class (apk add INSIDE the instance would chown foreign uids -> EPERM).
# Signature verification stays intact: apk uses $T/etc/apk/keys (alpine-keys)
# against the CDN index — no --allow-untrusted, ever.
# Env-aware (AL5 pattern, audit PR #24): CDN/apk failure leaves the rootfs
# usable but WITHOUT stack packages; tests/test_stack.sh then reports
# SKIP(env-blocked) instead of painting CI red.
install_stack_pkgs() { # <target-dir> — best-effort, loud on failure
    local T="$1"
    local PKGS="${MINICT_STACK_PKGS:-nginx sqlite fcgiwrap spawn-fcgi}"
    [[ -n "$PKGS" ]] || return 0
    echo "==> pre-installing stack packages (D45): $PKGS"
    # apk exit code is NOT authoritative under non-root: the busybox trigger
    # (chroot) and directory-permission updates fail by design here (EPERM,
    # measured 2026-11-20) while every package file is still unpacked. The
    # authoritative check is the presence of ALL four stack binaries.
    # --no-scripts is MANDATORY (D45): with scripts, apk runs them via chroot
    # — EPERM on the host — and marks busybox/fcgiwrap/nginx BROKEN in the
    # DB, so every later `apk add` inside the sandbox fails with rc=3
    # ("APK unavailable, skipped" on apk fix; measured). --no-scripts leaves
    # no pending scripts, AL5 offline apk add stays rc=0. Triggers are not
    # needed for the stack (busybox applet symlinks ship in the minirootfs;
    # nginx runs `user root` — D45; no TLS cert rehash in the demo).
    "$T/lib/ld-musl-x86_64.so.1" --library-path "$T/lib:$T/usr/lib" \
        "$T/sbin/apk" add --root "$T" --no-cache --no-scripts $PKGS >/dev/null 2>&1 || true
    if [[ -x "$T/usr/sbin/nginx" ]] \
            && [[ -x "$T/usr/bin/sqlite3" ]] \
            && [[ -x "$T/usr/bin/fcgiwrap" ]] \
            && [[ -x "$T/usr/bin/spawn-fcgi" ]]; then
        echo "==> stack packages installed ($(du -sh "$T" | cut -f1))"
    else
        echo "WARNING: stack package install FAILED (CDN/network?) — rootfs is"
        echo "         still usable, but WITHOUT the web stack;"
        echo "         tests/test_stack.sh will report SKIP(env-blocked)."
    fi
}

# restore_pkg_dirs <target-dir> — recreate every directory the apk DB says
# the installed packages own (F: lines), with the mode from the M: line.
# Root cause of issue #55: rootfs-alpine/ is tracked in git (D44 keeps it so)
# but git does not store EMPTY directories. A fresh clone (CI checkout,
# pristine clone) has bin/busybox + etc/alpine-release, so the "already
# exists" path below skipped the build and the rootfs lacked home, tmp,
# media, root, run, srv, sys, proc, mnt, opt, dev/pts, var/cache, ... —
# exactly the "missing after build" set seen 2/4 runs (the 2 green runs were
# rebuilt from the tarball). The DB is part of the tracked tree, so this
# needs no network. Idempotent: existing dirs are left alone.
restore_pkg_dirs() {
    local T="$1" db="$1/lib/apk/db/installed" line d="" n=0
    [[ -f "$db" ]] || { echo "no apk DB at $db — cannot restore dirs"; return 1; }
    while IFS= read -r line; do
        case "$line" in
            F:*)
                d="${line#F:}"
                # the DB is ours, but never let a path leave the rootfs
                case "/$d/" in */../*|*/./*|//*) d=""; continue ;; esac
                if [[ ! -d "$T/$d" ]]; then
                    mkdir -p "$T/$d"; n=$((n + 1))
                    # remember the dir so a following M: line may set its mode
                else
                    d=""
                fi ;;
            M:*)
                # M:<uid>:<gid>:<octal mode> — uid/gid collapse to the calling
                # uid under the single-uid map (D20); only the mode matters
                [[ -n "$d" ]] && chmod "${line##*:}" "$T/$d" ;;
            P:*) d="" ;;
        esac
    done < "$db"
    [[ $n -gt 0 ]] && echo "==> restored $n empty package dir(s) missing from the tree (#55)"
    return 0
}

# MINICT_STACK_AUTOADD=1: rootfs already exists but lacks the stack — install
# only (used by sandbox.sh/test_stack.sh to upgrade an older build in place).
if [[ "${MINICT_STACK_AUTOADD:-0}" == "1" ]]; then
    if [[ -x "$TARGET/bin/busybox" && -f "$TARGET/etc/alpine-release" ]]; then
        restore_pkg_dirs "$TARGET"
        [[ -x "$TARGET/usr/sbin/nginx" ]] || install_stack_pkgs "$TARGET"
        exit 0
    fi
    echo "MINICT_STACK_AUTOADD: no existing alpine rootfs at '$TARGET' — build first"
    exit 1
fi

if [[ -x "$TARGET/bin/busybox" && -f "$TARGET/etc/alpine-release" ]]; then
    echo "alpine rootfs '$TARGET' already exists, skipping (rm -rf it to rebuild)"
    restore_pkg_dirs "$TARGET"
    mkdir -p "$TARGET/home/minict"
    [[ -x "$TARGET/usr/sbin/nginx" ]] || echo "note: stack packages NOT in this rootfs (MINICT_STACK_AUTOADD=1 re-runs install)"
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
# issue #55: a fresh tarball extract already has every package dir; this is
# a no-op there and keeps both build paths on the same code
restore_pkg_dirs "$TARGET"
mkdir -p "$TARGET/home/minict"

# stack packages for fresh builds (D45) — MINICT_STACK=0 opts out (plain rootfs)
if [[ "${MINICT_STACK:-1}" == "1" ]]; then
    install_stack_pkgs "$TARGET"
fi

SIZE=$(du -sh "$TARGET" | cut -f1)
echo "==> done: $TARGET ($SIZE, alpine $(cat "$TARGET/etc/alpine-release" 2>/dev/null || echo '?'))"
