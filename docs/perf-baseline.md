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
| 2026-11-17 (alpine rootfs, phiên 8) | 1.83 (alpine, N=20; busybox cùng đo: 1.88) | 1087 (alpine, N=5; busybox: 1066) | 8.6 (alpine) | + rootfs Alpine minirootfs 3.22.8 (D20) — cold start/RSS không đổi về bản chất; disk +6.6MB là chi phí "distro thật" chấp nhận, vẫn ≤ 20; evidence `journal/2026-11-17/evidence/alpine-perf.txt` |

## Alpine rootfs (bổ sung phiên 8, issue #23)

| Metric | Giá trị | Ghi chú |
|---|---|---|
| disk rootfs-alpine | 8.6 MB (busybox 2.0) | minirootfs + apk DB; ngưỡng 20 MB |
| cold_start_ms alpine p50 | 1.83 (N=20) | ≈ busybox (1.88 cùng lúc đo) |
| rss_kb idle (wrapper+PID1) | 1087 (N=5, median) | ≈ busybox (1066) |
| perf-check busybox baseline | PASS 1.84 / 1140 / 2.02 (N=20) | `make perf-check` vẫn đo rootfs busybox (default), alpine đo riêng ở trên |

## Web instance (bổ sung cho area:web)

| Metric | Giá trị | Ghi chú |
|---|---|---|
| web-ready latency (spawn→HTTP 200 curl được) | xem `journal/2026-11-17/bench.md` phiên 3 | gồm private fs copy ~25–30ms |
| RSS instance web (wrapper + httpd) | xem cùng bảng | httpd busybox rất nhỏ |
| disk per web instance | ~2.0MB + docroot | private copy, không overlayfs |
