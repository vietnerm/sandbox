#!/usr/bin/env bash
# A/B cold-start overhead of netns (issue #3, session 5) — interleaved N=40.
# ON  = default (CLONE_NEWNET + lo up)   OFF = MINICT_NO_NETNS=1
set -euo pipefail
cd "$(dirname "$0")/.."

N=40
on=(); off=()
for i in $(seq 1 $N); do
    if (( i % 2 )); then
        on+=( "$(python3 -c 'import subprocess,time; t=time.monotonic_ns(); subprocess.run(["./sandbox/minict","./rootfs","/bin/sh","-c","true"],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL); print((time.monotonic_ns()-t)/1e6)')" )
    else
        off+=( "$(MINICT_NO_NETNS=1 python3 -c 'import subprocess,time; t=time.monotonic_ns(); subprocess.run(["./sandbox/minict","./rootfs","/bin/sh","-c","true"],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL); print((time.monotonic_ns()-t)/1e6)')" )
    fi
done
stat() { python3 - "$@" <<'PY'
import sys
v = sorted(float(x) for x in sys.argv[1:])
n = len(v)
p50 = v[int(n*0.5)- (0 if n%2 else 1)]
p95 = v[min(n-1, int(n*0.95))]
print(f"n={n} p50={p50:.2f}ms p95={p95:.2f}ms min={v[0]:.2f} max={v[-1]:.2f}")
PY
}
echo "netns ON  (default): $(stat "${on[@]}")"
echo "netns OFF (control): $(stat "${off[@]}")"
