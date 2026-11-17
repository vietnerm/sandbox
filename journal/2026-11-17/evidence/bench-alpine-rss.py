#!/usr/bin/env python3
# bench-alpine-rss.py — RSS đo theo đúng methodology của tools/perf-check.sh:
# tổng VmRSS của wrapper minict + sandboxed PID 1 (idle sleep).
import subprocess, time, sys, os

def rss_of(pid):
    try:
        with open(f"/proc/{pid}/status") as f:
            for line in f:
                if line.startswith("VmRSS:"):
                    return int(line.split()[1])
    except FileNotFoundError:
        pass
    return 0

def measure(rootfs, n=5):
    vals = []
    for _ in range(n):
        p = subprocess.Popen(["./sandbox/minict", rootfs, "/bin/sh", "-c", "sleep 3"],
                             stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        time.sleep(0.5)
        # children of the wrapper = pidns payload
        out = subprocess.run(["pgrep", "-P", str(p.pid)], capture_output=True, text=True)
        kids = [int(x) for x in out.stdout.split() if x.strip()]
        total = rss_of(p.pid) + sum(rss_of(k) for k in kids)
        vals.append(total)
        p.kill(); p.wait()
    return min(vals), max(vals), sum(vals)//len(vals)

lo, hi, med = measure("rootfs")
print(f"rss_kb busybox (baseline): min={lo} max={hi} median={med} (N=5)")
lo, hi, med = measure("rootfs-alpine")
print(f"rss_kb alpine 3.22.8     : min={lo} max={hi} median={med} (N=5)")
