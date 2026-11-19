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
[[ -d "$AROOTFS/bin" ]] || bash "$REPO/sandbox/mkrootfs-alpine.sh" "$AROOTFS" >/dev/null
[[ -x "$AROOTFS/bin/busybox" ]] || { echo "alpine rootfs build failed"; exit 1; }

ISOLATION_ROOTFS="$AROOTFS" bash "$HERE/test_isolation.sh"
