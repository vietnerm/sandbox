#!/bin/sh
# 01-static-web: busybox httpd inside sandbox, wget loopback → marker.
# Smoke test: httpd boots, serves a file, wget retrieves it — all inside the
# netns loopback (no host bridge needed for this self-contained check).
set -e
HERE="$(cd "$(dirname "$0")" && pwd)"
REPO="$(cd "$HERE/../.." && pwd)"
MINICT="$REPO/sandbox/minict"
ROOTFS="${MINICT_ROOTFS:-$REPO/rootfs}"
[ -x "$MINICT" ] || { echo "missing $MINICT — run: make build"; exit 1; }
[ -d "$ROOTFS" ] || bash "$REPO/sandbox/mkrootfs.sh" "$ROOTFS" >/dev/null

OUT=$("$MINICT" "$ROOTFS" /bin/sh -c '
    mkdir -p /www && echo example-marker > /www/index.html
    httpd -f -p 127.0.0.1:8080 -h /www &
    sleep 1
    wget -q -O- http://127.0.0.1:8080/
' 2>/dev/null) || { echo "FAIL: minict/httpd/wget"; exit 1; }

echo "$OUT" | grep -q example-marker || { echo "FAIL: no marker: $OUT"; exit 1; }
echo "01-static-web: PASS"
