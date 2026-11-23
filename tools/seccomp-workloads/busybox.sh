#!/usr/bin/env bash
# busybox allowlist workload — union of what the suites + normal sh use
# (default rootfs, D11 blocklist ACTIVE during measurement — D59).
# NOTE: payload is INLINE (-c): the instance rootfs is a private copy, so
# host-side temp files are invisible inside.
REPO="$(cd "$(dirname "$0")/../.." && pwd)"
PROBE="${PROBE:-$REPO/tools/syscall-probe}"
RAW="${RAW:-/tmp/probe-raw.txt}"
"$PROBE" --after-exec 2 --append "$RAW" -- sandbox/minict rootfs /bin/sh -c '
echo hi; printf "%s\n" abc
ls -la /; ls /etc >/dev/null
cat /etc/passwd >/dev/null
head -c 16 /dev/zero | wc -c
dd if=/dev/zero of=/tmp/x bs=1k count=2 2>/dev/null
cat /tmp/x >/dev/null
mktemp -d
mkdir -p /tmp/d/e; touch /tmp/d/f; cp /tmp/x /tmp/d/g
mv /tmp/d/g /tmp/d/h; chmod 644 /tmp/d/h; ln -sf /tmp/d/h /tmp/d/i
rm -rf /tmp/d
uname -m; hostname; id; date
ps | head -3; pgrep -f sh | head -1
grep root /etc/passwd; sed -n "1p" /etc/passwd; cut -d: -f1 /etc/passwd
find /etc -name passwd; seq 1 3 >/dev/null
sort /etc/passwd | uniq | head -2
md5sum /etc/passwd
tar czf /tmp/t.tgz /etc/passwd 2>/dev/null; tar tzf /tmp/t.tgz >/dev/null
sleep 0.2; test -f /etc/passwd && echo ok
'
# in-instance httpd + wget (pasta/web suites need server+client syscalls)
"$PROBE" --after-exec 2 --append "$RAW" -- sandbox/minict rootfs /bin/sh -c 'httpd -f -p 18081 >/dev/null 2>&1 & s=$!; sleep 0.3; wget -qO- http://127.0.0.1:18081/ >/dev/null 2>&1; wget -qO- http://127.0.0.1:18081/nope >/dev/null 2>&1; kill $s 2>/dev/null; echo done'
