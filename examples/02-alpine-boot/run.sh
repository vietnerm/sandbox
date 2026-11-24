#!/bin/sh
# 02-alpine-boot: rootfs boots, /bin/sh runs, marker printed.
# Simplified from apk-offline (#28 residual: apk add offline cần local repo
# pre-seed — chờ pasta egress mục 4 cho apk online). Smoke test: boot + sh.
set -e
HERE="$(cd "$(dirname "$0")" && pwd)"
REPO="$(cd "$HERE/../.." && pwd)"
MINICT="$REPO/sandbox/minict"
ROOTFS="${MINICT_ROOTFS:-$REPO/rootfs}"
[ -x "$MINICT" ] || { echo "missing $MINICT — run: make build"; exit 1; }
[ -d "$ROOTFS" ] || bash "$REPO/sandbox/mkrootfs.sh" "$ROOTFS" >/dev/null

OUT=$("$MINICT" "$ROOTFS" /bin/sh -c 'echo alpine-boot-ok; uname -a' 2>/dev/null) \
    || { echo "FAIL: minict boot"; exit 1; }

echo "$OUT" | grep -q alpine-boot-ok || { echo "FAIL: no marker: $OUT"; exit 1; }
echo "02-alpine-boot: PASS"
