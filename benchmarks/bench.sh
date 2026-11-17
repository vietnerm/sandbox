#!/usr/bin/env bash
# bench.sh — measure cold start, RAM overhead, disk footprint of minict.
# Output: markdown-friendly tables.
set -u

HERE="$(cd "$(dirname "$0")" && pwd)"
REPO="$(cd "$HERE/.." && pwd)"
MINICT="$HERE/../sandbox/minict"
ROOTFS="$REPO/rootfs"

[[ -x "$MINICT" ]] || make -C "$HERE/../sandbox" >/dev/null
[[ -d "$ROOTFS" ]] || bash "$REPO/sandbox/mkrootfs.sh" "$ROOTFS" >/dev/null

N="${N:-20}"

echo "## benchmark $(date '+%F %T') — kernel $(uname -r), N=$N"
echo

# ---------- 1. cold start ----------
echo "### 1. cold start (spawn -> exit)"
echo
python3 - "$N" "$MINICT" "$ROOTFS" <<'PY'
import subprocess, time, sys, statistics
n, minict, rootfs = int(sys.argv[1]), sys.argv[2], sys.argv[3]

def bench(cmd, label):
    ts = []
    for _ in range(n):
        t0 = time.monotonic_ns()
        subprocess.run(cmd, stdout=subprocess.DEVNULL,
                       stderr=subprocess.DEVNULL)
        ts.append((time.monotonic_ns() - t0) / 1e6)
    ts.sort()
    p50 = statistics.median(ts)
    p95 = ts[min(len(ts) - 1, int(0.95 * len(ts)))]
    print(f"| {label} | {ts[0]:.1f} | {p50:.1f} | {p95:.1f} | {ts[-1]:.1f} |")

print("| variant | min (ms) | p50 (ms) | p95 (ms) | max (ms) |")
print("|---|---|---|---|---|")
bench(["/bin/sh", "-c", "true"],
      "baseline: bare /bin/sh (no isolation)")
bench([minict, rootfs, "/bin/sh", "-c", "true"],
      "minict: 5 namespaces + pivot_root + capdrop")
PY
echo

# ---------- 2. RAM overhead ----------
echo "### 2. RAM overhead (RSS, idle instance)"
echo
"$MINICT" "$ROOTFS" /bin/sh -c 'sleep 3' &  # wrapper pid $!; child = sandboxed sh
WRAP=$!
sleep 0.3
# child = newest process whose ppid is the wrapper (pidns child)
CHILD=$(pgrep -P "$WRAP" 2>/dev/null | head -1 || true)
wrap_rss=$(awk '/VmRSS/{print $2}' "/proc/$WRAP/status" 2>/dev/null || echo 0)
child_rss=$(awk '/VmRSS/{print $2}' "/proc/$CHILD/status" 2>/dev/null || echo 0)
echo "| component | RSS (kB) |"
echo "|---|---|"
echo "| minict wrapper (host side) | ${wrap_rss:-0} |"
echo "| sandboxed busybox sh (PID 1 in ns) | ${child_rss:-0} |"
echo "| total | $(( ${wrap_rss:-0} + ${child_rss:-0} )) |"
wait "$WRAP" 2>/dev/null
echo

# ---------- 3. disk footprint ----------
echo "### 3. disk footprint"
echo
echo "| artifact | size |"
echo "|---|---|"
echo "| busybox binary | $(du -h "$ROOTFS/bin/busybox" | cut -f1) |"
echo "| rootfs (full, with symlinks) | $(du -sh "$ROOTFS" | cut -f1) |"
echo "| minict binary | $(du -h "$MINICT" | cut -f1) |"
TRACKED_COUNT=$(git -C "$REPO" ls-files 2>/dev/null | wc -l)
if [[ "$TRACKED_COUNT" -gt 0 ]]; then
    echo "| repo code (git-tracked only) | $(git -C "$REPO" ls-files -z | xargs -0 du -cb | tail -1 | awk '{printf "%.0f kB", $1/1024}') |"
else
    echo "| repo code (git-tracked only) | (not committed yet) |"
fi
echo

# ---------- 4. per-instance fs copy cost ----------
echo "### 4. instance fs copy (private rootfs per 'up')"
echo
T0=$(python3 -c 'import time;print(time.monotonic_ns())')
rm -rf /tmp/mk-bench-copy && cp -a "$ROOTFS" /tmp/mk-bench-copy
T1=$(python3 -c 'import time;print(time.monotonic_ns())')
rm -rf /tmp/mk-bench-copy
python3 -c "print(f'| cp -a rootfs (~2MB) | {($T1-$T0)/1e6:.1f} ms |')"
