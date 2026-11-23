#!/usr/bin/env bash
# alpine allowlist workload — busybox sh + apk local-db queries (musl),
# no network (D59: apk online goes through pasta on the runner; the syscall
# set for it is validated by the pasta suites, not baked in here).
# Payload INLINE: the instance rootfs is a private copy — host-side temp
# files are invisible inside.
REPO="$(cd "$(dirname "$0")/../.." && pwd)"
PROBE="${PROBE:-$REPO/tools/syscall-probe}"
RAW="${RAW:-/tmp/probe-raw.txt}"
"$PROBE" --after-exec 2 --append "$RAW" -- sandbox/minict rootfs-alpine /bin/sh -c '
echo hi; ls -la /; cat /etc/alpine-release; uname -m; hostname; id
mkdir -p /tmp/d; touch /tmp/d/f; cp /etc/alpine-release /tmp/d/g; rm -rf /tmp/d
apk --version; apk info 2>/dev/null | head -3; apk search -v busybox 2>/dev/null | head -2
ps | head -3; grep -c "" /etc/passwd; date; sleep 0.2; test -f /etc/passwd && echo ok
'
