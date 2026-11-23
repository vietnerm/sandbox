#!/usr/bin/env bash
# debian allowlist workload — glibc (dash + coreutils). glibc 2.36+ tries
# clone3 for fork; the filter answers ENOSYS and glibc falls back to clone
# (the arg-checked path) — the probe records the fallback, which is what
# the allowlist must contain. Payload INLINE (private rootfs copy).
REPO="$(cd "$(dirname "$0")/../.." && pwd)"
PROBE="${PROBE:-$REPO/tools/syscall-probe}"
RAW="${RAW:-/tmp/probe-raw.txt}"
"$PROBE" --after-exec 2 --append "$RAW" -- sandbox/minict rootfs-debian /bin/sh -c '
echo hi; ls -la /; cat /etc/debian_version; uname -m; hostname; id
mkdir -p /tmp/d; touch /tmp/d/f; cp /etc/debian_version /tmp/d/g
mv /tmp/d/g /tmp/d/h; chmod 644 /tmp/d/h; rm -rf /tmp/d
date; sleep 0.2; sh -c "for i in 1 2 3; do echo \$i; done"; grep -c "" /etc/passwd
'
