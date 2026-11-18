## benchmark 2026-11-18 05:04:04 — kernel 5.10.134-013.15.kangaroo.al8.x86_64, N=50

### 1. cold start (spawn -> exit)

| variant | min (ms) | p50 (ms) | p95 (ms) | max (ms) |
|---|---|---|---|---|
| baseline: bare /bin/sh (no isolation) | 0.3 | 0.4 | 0.5 | 0.6 |
| minict: 5 namespaces + pivot_root + capdrop | 1.7 | 1.9 | 2.2 | 2.8 |

### 2. RAM overhead (RSS, idle instance)

minict: warn: mount /proc (blocked in this environment): Operation not permitted
| component | RSS (kB) |
|---|---|
| minict wrapper (host side) | 1100 |
| sandboxed busybox sh (PID 1 in ns) | 104 |
| total | 1204 |

### 3. disk footprint

| artifact | size |
|---|---|
| busybox binary | 2.0M |
| rootfs (full, with symlinks) | 2.0M |
| minict binary | 36K |
| repo code (git-tracked only) | 8780 kB |

### 4. instance fs copy (private rootfs per 'up')

| cp -a rootfs (~2MB) | 32.9 ms |
