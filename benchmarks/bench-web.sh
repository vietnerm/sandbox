#!/usr/bin/env bash
# bench-web.sh — one-shot measurements for the web milestone (Benchmarks phiên 3).
# 1. web-ready latency: `minict.sh web` -> HTTP 200 (includes private fs copy)
# 2. RSS of a running web instance (wrapper + sandboxed httpd)
# 3. disk per web instance
set -u
HERE="$(cd "$(dirname "$0")" && pwd)"
REPO="$(cd "$HERE/.." && pwd)"
MINICT="$REPO/sandbox/minict"
[[ -x "$MINICT" ]] || make -C "$REPO/sandbox" >/dev/null
[[ -d "$REPO/rootfs" ]] || bash "$REPO/sandbox/mkrootfs.sh" "$REPO/rootfs" >/dev/null

NAME="webbench-$$"
PORT=$(( 19000 + RANDOM % 2000 ))
N=5

echo "### 5. web instance (busybox httpd inside sandbox, N=$N)"

# ---------- 1. web-ready latency ----------
python3 - "$REPO" "$NAME" "$PORT" "$N" <<'PY'
import subprocess, time, sys, statistics
repo, name, port, n = sys.argv[1], sys.argv[2], int(sys.argv[3]), int(sys.argv[4])
ts = []
for i in range(n):
    t0 = time.monotonic_ns()
    subprocess.run(["bash", f"{repo}/sandbox/minict.sh", "web", name, str(port)],
                   stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, check=True)
    # serve check
    import urllib.request
    while True:
        try:
            urllib.request.urlopen(f"http://127.0.0.1:{port}/", timeout=1).read()
            break
        except Exception:
            time.sleep(0.02)
    ts.append((time.monotonic_ns() - t0) / 1e6)
    subprocess.run(["bash", f"{repo}/sandbox/minict.sh", "down", name],
                   stdout=subprocess.DEVNULL)
ts.sort()
print(f"| web up -> HTTP 200 (fs copy + spawn + listen) | "
      f"{ts[0]:.0f} | {statistics.median(ts):.0f} | {ts[-1]:.0f} |")
PY

# ---------- 2. RSS of web instance ----------
PORT=$(( PORT + 1 ))
bash "$REPO/sandbox/minict.sh" web "$NAME" "$PORT" >/dev/null 2>&1
sleep 0.4
WRAP=$(cat "$REPO/instances/$NAME/pid")
CHILD=$(pgrep -P "$WRAP" 2>/dev/null | head -1 || true)
# httpd exec'd inside the sandboxed sh — find it by port
HTTPD=$(pgrep -f "httpd -f -p 127.0.0.1:$PORT" | head -1 || true)
wrap_rss=$(awk '/VmRSS/{print $2}' "/proc/$WRAP/status" 2>/dev/null || echo 0)
child_rss=$(awk '/VmRSS/{print $2}' "/proc/$CHILD/status" 2>/dev/null || echo 0)
httpd_rss=$(awk '/VmRSS/{print $2}' "/proc/$HTTPD/status" 2>/dev/null || echo 0)
echo "| component | RSS (kB) |"
echo "|---|---|"
echo "| minict wrapper (host side) | $wrap_rss |"
echo "| sandboxed sh (parent of httpd) | $child_rss |"
echo "| busybox httpd (sandboxed, PID-space inside ns) | $httpd_rss |"
echo "| total | $(( wrap_rss + child_rss + httpd_rss )) |"

# ---------- 3. disk per web instance ----------
echo
echo "| artifact | size |"
echo "|---|---|"
echo "| web instance dir (private rootfs + docroot) | $(du -sh "$REPO/instances/$NAME" 2>/dev/null | cut -f1) |"
bash "$REPO/sandbox/minict.sh" down "$NAME" >/dev/null 2>&1
