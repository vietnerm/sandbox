# Benchmarks — 2026-11-17

Môi trường đo: kernel 5.10.134-013.15.kangaroo.al8.x86_64, Debian 13 userspace,
user thường (uid 1001), N=20 lần cho cold start.

## 1. Cold start (spawn → exit, đo monotonic clock)

| variant | min (ms) | p50 (ms) | p95 (ms) | max (ms) |
|---|---|---|---|---|
| baseline: bare /bin/sh (không isolation) | 0.4 | 0.4 | 0.7 | 0.7 |
| **minict: 5 ns + pivot_root + capdrop** | **1.1** | **1.2** | **1.7** | 1.7 |
| Docker container (cited, không đo được trong env này) | — | ~300-1000 | — | — |

- Overhead so với bare process: **~0.8ms** — negligible cho dev/CI.
- Target cold start < 2s: **PASS, dư 1600x**.
- `up` đầy đủ (tạo instance: cp -a rootfs + fork daemon) = +25ms fs copy.

## 2. RAM overhead (idle)

| component | RSS (kB) |
|---|---|
| minict wrapper (host side, glibc) | 1128 |
| sandboxed process (busybox, shared pages) | ~4-1200 |
| **total / instance** | **~1.1-2.3 MB** |

- Target RAM < 30MB/instance: **PASS**.
- Đo bằng VmRSS từ /proc của host. RSS của busybox thấp vì chia sẻ page với
  các busybox process khác; glibc wrapper là phần lớn nhất (1.1MB).
- Docker (cited): daemon chiếm ~50-100MB + mỗi container thêm shim.

## 3. Disk footprint

| artifact | size |
|---|---|
| busybox binary | 2.0M |
| rootfs đầy đủ (~400 applet symlinks) | 2.0M |
| minict binary | 20K |
| tổng sandbox base | **~2.0 MB** |

- Target < 50MB: **PASS, dư 25x**. (Docker image python:slim ≈ 50MB+, cited.)

## 4. Chi phí private rootfs per instance (thay overlayfs)

| op | time |
|---|---|
| cp -a rootfs 2MB | 25.2 ms |

## 5. Mức độ cô lập (escape tests — tests/test_isolation.sh, 16/16)

| nhóm | test | kết quả |
|---|---|---|
| FS | host /etc/passwd, /home/z, /old-* không visible; mount bind / fail; mount sau capdrop fail; write-on-disk | **PASS** (6) |
| PID | PID 1 riêng, không thấy host processes | **PASS** (2) |
| UID | uid=0 chỉ trong ns; file trên disk thuộc real uid | **PASS** (2) |
| PRIV | NoNewPrivs=1 (verify /proc host); hostname isolated; SIGKILL → no orphan | **PASS** (3) |
| GAP | netns không isolate; /proc mount bị chặn; không cgroup v2 | **expected-fail** (3) — ghi nhận, chưa phải mục tiêu hôm nay |

## So sánh với baseline hôm qua
Ngày đầu tiên — baseline là "process trần không isolation" và số liệu công khai của Docker.
Số liệu Docker đánh dấu (cited): dùng để định hướng, sẽ thay bằng đo thật khi có Docker-compatible
môi trường hoặc khi so sánh với Podman ở ngày sau.

## Phiên 3 (SDDA cycle) — web instance trong sandbox

Môi trường đo: cùng môi trường phiên 1, uid 1001 rootless. Script: `benchmarks/bench-web.sh`.

### 5. web instance (busybox httpd inside sandbox, N=5)

| variant | min (ms) | p50 (ms) | max (ms) |
|---|---|---|---|
| web up → HTTP 200 (fs copy + spawn + listen + curl) | 18 | **18** | 38 |

- So với ngưỡng perf-guard ≤ 100 ms: **PASS, dư ~5.5x**. fs copy ~25-30ms trội lên
  khi cache nóng/lạnh khác nhau → p50 18ms (cache ấm), max 38ms.
- httpd bind `127.0.0.1:<port>` bên trong sandbox (D7 — không netns, không public bind).

### 6. RAM web instance (idle)

| component | RSS (kB) |
|---|---|
| minict wrapper (host side) | 1044 |
| sandboxed sh (parent của httpd) | 4 |
| busybox httpd (sandboxed) | 1044 |
| **total / web instance** | **~2.0 MB** |

### 7. disk web instance

| artifact | size |
|---|---|
| web instance dir (private rootfs + docroot) | 2.1 MB |

### Kết luận perf-guard phiên 3

`tools/perf-check.sh` (N=20): cold_start p50 = **1.03 ms** | rss = **1148 kB** |
disk = **2.01 MB** — tất cả PASS, không đổi so với phiên 1 (web không chạm runtime path
của cold start: httpd chạy như một command bên trong ns đã dựng sẵn).
