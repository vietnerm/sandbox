#!/usr/bin/env bash
# test_escape_alpine.sh — run the FULL isolation/escape suite against the
# alpine distro rootfs instead of the busybox rootfs.
#
# Roadmap GĐ1 mục 2 (issue #5 residual): "chạy lại cả suite trên rootfs
# distro thật (có /proc…, nhiều mặt tấn công hơn busybox)". Alpine brings
# a real musl libc, real /etc with users/groups, a bigger busybox config
# and its own passwd — every isolation contract must hold unchanged.
#
# Mechanism note (measured 2026-11-19): alpine's busybox has the chroot
# APPLET but no /bin/chroot symlink, so test_isolation.sh invokes applets
# explicitly (/bin/busybox chroot) — otherwise the expect=fail case dies
# rc=127 and passes spuriously.
set -u

HERE="$(cd "$(dirname "$0")" && pwd)"
REPO="$(cd "$HERE/.." && pwd)"
AROOTFS="$REPO/rootfs-alpine"

# reuse the rootfs built by test_alpine.sh when run-suites.sh got here;
# build standalone otherwise (pinned tarball + sha256, rootless)
# self-heal: intermittent builds were observed shipping WITHOUT the tarball's
# empty dirs (home/tmp/media/root/run/srv/sys — issue #55); FS6/UID2 then fail
# with a confusing "can't create /home/iso". Rebuild whenever dirs are missing.
if ! [[ -d "$AROOTFS/bin" && -d "$AROOTFS/home" && -d "$AROOTFS/tmp" \
        && -d "$AROOTFS/media" && -d "$AROOTFS/root" && -d "$AROOTFS/run" \
        && -d "$AROOTFS/srv" && -d "$AROOTFS/sys" ]]; then
    rm -rf "$AROOTFS"
    bash "$REPO/sandbox/mkrootfs-alpine.sh" "$AROOTFS" >/dev/null || exit 1
fi
[[ -x "$AROOTFS/bin/busybox" ]] || { echo "alpine rootfs build failed"; exit 1; }
[[ -d "$AROOTFS/home" && -d "$AROOTFS/tmp" ]] \
    || { echo "alpine rootfs STILL missing /home//tmp after rebuild — issue #55"; exit 1; }

ISOLATION_ROOTFS="$AROOTFS" bash "$HERE/test_isolation.sh"
