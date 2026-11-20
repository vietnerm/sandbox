# Perf Baseline — minict (perf-guard thresholds)

> Nguồn số liệu: `tools/perf-check.sh` (methodology trùng `benchmarks/bench.sh`).
> perf-guard.yml fail khi vượt ngưỡng trừ khi PR có label `perf-regression-approved`.

| Metric | Threshold (PROMPT.MD) | Baseline đo 2026-11-17 | Target dài hạn (roadmap) |
|---|---|---|---|
| cold_start_ms (p50, N=20) | ≤ 100 | 1.0–1.2 | ≤ 5 sau netns+seccomp |
| rss_kb (wrapper + PID 1) | ≤ 10240 | ~1100–2300 | ≤ 2048 sau netns+seccomp |
| disk_base_mb (rootfs + binary) | ≤ 20 | ~2.0 | giữ ≤ 5 với web stack |

## Lịch sử đo

| Ngày | cold_start p50 (ms) | rss_kb | disk_base_mb | Ghi chú |
|---|---|---|---|---|
| 2026-11-17 (PoC) | 1.2 (p95 1.4) | ~1128 wrapper, ~1.1–2.3 total | 2.0 | minict v0.1, N=20 |
| 2026-11-17 (web, phiên 3) | 1.03 | 1148 | 2.01 | + busybox httpd in-sandbox (`minict.sh web`), perf-check PASS |
| 2026-11-17 (seccomp, phiên 4) | 1.15 (perf-check N=20) | 1060 | 2.02 | + seccomp-bpf blocklist mặc định BẬT; A/B N=40: ON 1.38 vs OFF 1.18 → **+0.2ms** (chi tiết D11/D14, evidence `journal/2026-11-17/evidence/seccomp-ab.txt`) |
| 2026-11-17 (netns, phiên 5) | 1.90 (perf-check N=20) | 1032 | 2.03 | + CLONE_NEWNET mặc định + lo up (D17/D18) + unix-socket bridge web (D16); A/B N=40 interleaved: ON 1.80 vs OFF 1.52 → **+0.28ms** netns (bench `bench-netns-ab.sh`, evidence `journal/2026-11-17/evidence/netns.txt`); perf-check PASS tất cả ngưỡng |
| 2026-11-17 (sigterm shim, phiên 7) | 1.85 (perf-check N=40) | 1160 | 2.02 | + pidns-1 signal shim (D19); A/B N=40: trước (không shim) 1.66 vs sau 1.83 → **+0.17ms** (1 fork + waitpid; evidence `journal/2026-11-17/evidence/sigterm-ab.txt`); perf-check PASS tất cả ngưỡng |
| 2026-11-20 (web stack, phiên 10, #85) | 1.93 (perf-check N=20, busybox) | 1196 | 2.04 (busybox) | + web stack nginx+fcgiwrap+CGI+SQLite trong 1 instance alpine (D45): perf-guard đo rootfs busybox default — KHÔNG đổi; rootfs-alpine 8.6 → **13.3 MB** (stack pre-install, vẫn ≤ 20); **stack cold start** (spawn → HTTP 200 + API 200) **median 127 ms, max 232 ms** (N=5, ngưỡng roadmap < 5000 ms — PASS); evidence `journal/2026-11-20/evidence/web-stack.txt` |

## Alpine rootfs (bổ sung phiên 8, issue #23; cập nhật phiên 10)

| Metric | Giá trị | Ghi chú |
|---|---|---|
| disk rootfs-alpine | **13.3 MB** (busybox 2.0) | phiên 8: 8.6 MB; phiên 10: + nginx/sqlite/fcgiwrap/spawn-fcgi pre-install (D45, `--no-scripts`); ngưỡng 20 MB |
| cold_start_ms alpine p50 | 1.83 (N=20, phiên 8) | đo trước khi thêm stack; stack trong instance không nằm trong cold_start busybox |
| rss_kb idle (wrapper+PID1) | 1087 (N=5, median, phiên 8) | như trên |
| **stack cold start** (spawn → index + API health) | **median 127 ms, min 123, max 232 (N=5)** | gồm rootfs copy ~14 MB + spawn-fcgi + nginx ready + 2 curl qua bridge; ngưỡng roadmap < 5000 ms — PASS dư ~39× |
| perf-check busybox baseline | PASS 1.84 / 1140 / 2.02 (N=20) | `make perf-check` vẫn đo rootfs busybox (default), alpine đo riêng ở trên |

## Web instance (bổ sung cho area:web)

| Metric | Giá trị | Ghi chú |
|---|---|---|
| web-ready latency (spawn→HTTP 200 curl được) | xem `journal/2026-11-17/bench.md` phiên 3 | gồm private fs copy ~25–30ms |
| RSS instance web (wrapper + httpd) | xem cùng bảng | httpd busybox rất nhỏ |
| disk per web instance | ~2.0MB + docroot | private copy, không overlayfs |
| 2026-11-19 (pasta prototype, phiên 3) | 1.87 (dev perf-check N=20) / 2.02 (runner CI N=20) | 1184 (dev) / 3128 (runner) | 2.04 (dev) / 2.14 (runner) | + pasta connectivity (issue #57, D37) — pasta spawn OUT-OF-BAND sau khi `up` trả về, KHÔNG nằm trong cold_start; A/B attach latency (AB1) đo trên runner, ngưỡng < 30 s, cold_start ngưỡng ≤ 100 ms giữ nguyên PASS; tools/bin/pasta gitignored (disk repo không tăng) |
