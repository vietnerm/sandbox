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
| 2026-11-20 (debian rootfs, phiên 11, #88) | (busybox baseline giữ nguyên) | 1196 | 2.04 (busybox) | + debian rootfs boots (D46): gate busybox KHÔNG đổi; rootfs-debian 83.3 MB gitignored đo supplementary (fetch pinned+sha256 D37); suite debian 8 case vào make test (9 suites) |
| 2026-11-26 (mục 8 RSS+leak, phiên 34, #171) | (busybox baseline giữ nguyên) | daemon idle 6372 KB (dev) | 2.04 (busybox) | + GĐ2 mục 8 (D69): MD53 destroy no-leak (process+mount+dir gone) + MD54 daemon idle RSS ≤ 30 MB gate (scrape /metrics + cross-check /proc VmRSS ±64 KB). Daemon idle RSS đo trên dev 5.10 = 6372 KB (~6.2 MB) — cách xa ngưỡng 30 MB, biên độ dư ~24 MB (regression guard). /metrics `minictd_process_resident_memory_bytes` khớp /proc VmRSS (cùng nguồn). CI runner số đo ghi trong evidence. |
| 2026-11-26 (observability, phiên 33, #168) | (busybox baseline giữ nguyên — perf-check đo minict wrapper, KHÔNG đo minictd) | ~1196 + Δ minictd RSS | 2.04 (busybox, KHÔNG đổi) | + GĐ2 mục 7 observability (D68): `--log-format json` mặc định (slog.NewJSONHandler + ReplaceAttr); `metrics.go` renderer Prometheus text viết tay stdlib-only (~250 dòng, KHÔNG prometheus/client_golang); `/metrics` endpoint trên listener sẵn có + `--metrics-listen` placeholder fail-closed. perf-guard gate đo **minict wrapper** (cold_start/rss/disk busybox) — KHÔNG đổi. minictd RSS delta: slog JSON handler stdlib (có sẵn) + renderer ~250 dòng Go + requestLog middleware — tăng dự kiến **< 2 MB** (no new deps, no goroutine, scrape-time read). Số đo CI chính xác ghi vào journal/2026-11-26/bench.md sau SCR. Ngưỡng roadmap mục 8 (idle ≤ 30 MB RSS tính cả observability) — PASS dự kiến (minictd idle trước ~1.2 MB wrapper / daemon RSS đo riêng). |

## Alpine rootfs (bổ sung phiên 8, issue #23; cập nhật phiên 10)

| Metric | Giá trị | Ghi chú |
|---|---|---|
| disk rootfs-alpine | **13.3 MB** (busybox 2.0) | phiên 8: 8.6 MB; phiên 10: + nginx/sqlite/fcgiwrap/spawn-fcgi pre-install (D45, `--no-scripts`); ngưỡng 20 MB |
| cold_start_ms alpine p50 | 1.83 (N=20, phiên 8) | đo trước khi thêm stack; stack trong instance không nằm trong cold_start busybox |
| rss_kb idle (wrapper+PID1) | 1087 (N=5, median, phiên 8) | như trên |
| **stack cold start** (spawn → index + API health) | **median 127 ms, min 123, max 232 (N=5)** | gồm rootfs copy ~14 MB + spawn-fcgi + nginx ready + 2 curl qua bridge; ngưỡng roadmap < 5000 ms — PASS dư ~39× |
| perf-check busybox baseline | PASS 1.84 / 1140 / 2.02 (N=20) | `make perf-check` vẫn đo rootfs busybox (default), alpine đo riêng ở trên |

## Debian rootfs (bổ sung phiên 11, issue #88, D46)

| Metric | Giá trị | Ghi chú |
|---|---|---|
| disk rootfs-debian | **83.3 MB** (unpacked, du -sk đo 2026-11-20) | bookworm-slim 12.15 amd64, debuerreotype epoch 1794787200; **gitignored** (fetch pinned+sha256 mỗi lần thiếu — 28.2 MB compressed), KHÔNG nằm trong disk_base_mb gate (gate đo rootfs busybox default 2.04 MB) — cùng cách đo supplementary như rootfs-alpine |
| cold_start_ms debian p50 | chưa đo riêng (one-shot boot đo được trong DB suite: < 1 s kể cả cp -a 83 MB per instance) | rootfs copy ~83 MB per instance là phần lớn chi phí; không gate |
| rss_kb idle (wrapper+PID1) | tương đương busybox baseline (~1.2 MB) | dash/sleep PID1 glibc, đo trên dev |
| apt online (DB7) | env-gated tun — đo thật trên runner CI | HTTP deb.debian.org + DNS D42; signature-verified mặc định |

## Web instance (bổ sung cho area:web)

| Metric | Giá trị | Ghi chú |
|---|---|---|
| web-ready latency (spawn→HTTP 200 curl được) | xem `journal/2026-11-17/bench.md` phiên 3 | gồm private fs copy ~25–30ms |
| RSS instance web (wrapper + httpd) | xem cùng bảng | httpd busybox rất nhỏ |
| disk per web instance | ~2.0MB + docroot | private copy, không overlayfs |
| 2026-11-19 (pasta prototype, phiên 3) | 1.87 (dev perf-check N=20) / 2.02 (runner CI N=20) | 1184 (dev) / 3128 (runner) | 2.04 (dev) / 2.14 (runner) | + pasta connectivity (issue #57, D37) — pasta spawn OUT-OF-BAND sau khi `up` trả về, KHÔNG nằm trong cold_start; A/B attach latency (AB1) đo trên runner, ngưỡng < 30 s, cold_start ngưỡng ≤ 100 ms giữ nguyên PASS; tools/bin/pasta gitignored (disk repo không tăng) |
