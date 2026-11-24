#!/bin/sh
# 03-isolation-check: in ra isolation properties + assert uid=0.
# Asserts: uid=0 (root-in-ns via userns). Prints: pid, hostname, nnp, seccomp,
# netns (informational — /proc mount is env-gated on dev 5.10, CI 6.x has it).
set -e
HERE="$(cd "$(dirname "$0")" && pwd)"
REPO="$(cd "$HERE/../.." && pwd)"
MINICT="$REPO/sandbox/minict"
ROOTFS="${MINICT_ROOTFS:-$REPO/rootfs}"
[ -x "$MINICT" ] || { echo "missing $MINICT — run: make build"; exit 1; }
[ -d "$ROOTFS" ] || bash "$REPO/sandbox/mkrootfs.sh" "$ROOTFS" >/dev/null

OUT=$("$MINICT" "$ROOTFS" /bin/sh -c '
    echo "uid=$(id -u)"
    echo "pid=$$"
    echo "host=$(hostname)"
    if [ -f /proc/self/status ]; then
        grep -q "NoNewPrivs.*1" /proc/self/status && echo "nnp=yes" || echo "nnp=no"
        grep -q "Seccomp.*2" /proc/self/status && echo "seccomp=strict" || echo "seccomp=off"
    else
        echo "nnp=(no /proc)"
        echo "seccomp=(no /proc)"
    fi
    ifconfig -a 2>/dev/null | grep -q "^lo" && echo "netns=lo-only" || echo "netns=none"
' 2>/dev/null) || { echo "FAIL: minict"; exit 1; }

echo "$OUT" | grep -q "uid=0" || { echo "FAIL: expected uid=0: $OUT"; exit 1; }
echo "03-isolation-check: PASS"
echo "$OUT"
