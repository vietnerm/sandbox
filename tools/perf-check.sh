#!/usr/bin/env bash
# perf-check.sh — perf-guard gate used by `make perf-check` and perf-guard.yml.
#
# Measures the three PROMPT.MD budgets and fails when any exceeds its
# threshold (unless the PR carries the perf-regression-approved label,
# which is checked by the workflow, not here):
#   cold_start_ms <= 100    (p50 of N spawn->exit runs)
#   rss_kb        <= 10240  (wrapper + sandboxed PID 1, idle)
#   disk_base_mb  <= 20     (rootfs + minict binary)
#
# Methodology matches benchmarks/bench.sh so numbers are comparable.
# Exit code: 0 within budgets, 1 over budget.
set -u

HERE="$(cd "$(dirname "$0")" && pwd)"
REPO="$(cd "$HERE/.." && pwd)"
MINICT="$REPO/sandbox/minict"
ROOTFS="$REPO/rootfs"

[[ -x "$MINICT" ]] || make -C "$REPO/sandbox" >/dev/null
[[ -d "$ROOTFS" ]] || bash "$REPO/sandbox/mkrootfs.sh" "$ROOTFS" >/dev/null

N="${N:-20}"
TH_COLD_MS=100
TH_RSS_KB=10240
TH_DISK_MB=20

# ---------- 1. cold start (p50) ----------
cold_ms=$(python3 - "$N" "$MINICT" "$ROOTFS" <<'PY'
import subprocess, time, sys, statistics
n, minict, rootfs = int(sys.argv[1]), sys.argv[2], sys.argv[3]
ts = []
for _ in range(n):
    t0 = time.monotonic_ns()
    subprocess.run([minict, rootfs, "/bin/sh", "-c", "true"],
                   stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    ts.append((time.monotonic_ns() - t0) / 1e6)
print(f"{statistics.median(ts):.2f}")
PY
)

# ---------- 2. RSS (wrapper + sandboxed PID 1) ----------
"$MINICT" "$ROOTFS" /bin/sh -c 'sleep 3' & WRAP=$!
sleep 0.4
CHILD=$(pgrep -P "$WRAP" 2>/dev/null | head -1 || true)
wrap_rss=$(awk '/VmRSS/{print $2}' "/proc/$WRAP/status" 2>/dev/null || echo 0)
child_rss=$(awk '/VmRSS/{print $2}' "/proc/$CHILD/status" 2>/dev/null || echo 0)
rss_kb=$(( ${wrap_rss:-0} + ${child_rss:-0} ))
kill -9 "$WRAP" "$CHILD" 2>/dev/null || true
wait "$WRAP" 2>/dev/null || true

# ---------- 3. disk base (rootfs + binary) ----------
disk_kb=$(du -sk "$ROOTFS" "$MINICT" 2>/dev/null | awk '{s+=$1} END{print s}')
disk_mb=$(python3 -c "print(f'{$disk_kb/1024:.2f}')")

echo "== perf-check $(date '+%F %T') — kernel $(uname -r), N=$N =="
printf 'cold_start_ms  : %8s  (threshold %s)\n' "$cold_ms" "$TH_COLD_MS"
printf 'rss_kb         : %8s  (threshold %s)\n' "$rss_kb" "$TH_RSS_KB"
printf 'disk_base_mb   : %8s  (threshold %s)\n' "$disk_mb" "$TH_DISK_MB"

rc=0
awk -v c="$cold_ms" -v t="$TH_COLD_MS" 'BEGIN{exit !(c<=t)}' \
    || { echo "OVER BUDGET: cold_start_ms $cold_ms > $TH_COLD_MS"; rc=1; }
[ "$rss_kb" -le "$TH_RSS_KB" ] \
    || { echo "OVER BUDGET: rss_kb $rss_kb > $TH_RSS_KB"; rc=1; }
awk -v d="$disk_mb" -v t="$TH_DISK_MB" 'BEGIN{exit !(d<=t)}' \
    || { echo "OVER BUDGET: disk_base_mb $disk_mb > $TH_DISK_MB"; rc=1; }

[ $rc -eq 0 ] && echo "perf-check: PASS" || echo "perf-check: FAIL"
exit $rc
