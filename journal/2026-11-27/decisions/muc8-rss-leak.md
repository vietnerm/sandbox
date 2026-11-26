# Decision D69 — GĐ2 mục 8: daemon idle RSS ≤ 30 MB gate + destroy no-leak e2e

- **Phiên**: 34 (2026-11-27)
- **Issue**: #171 (GĐ2 mục 8 — mục cuối GĐ2)
- **Slug**: `muc8-rss-leak`

## Vấn đề
GĐ2 mục 8 (roadmap dòng 576-577, manifest DoD):
1. Control plane idle RSS ≤ 30 MB (tính cả observability mục 7).
2. destroy không rò rỉ process/mount/netns.

## Giải pháp chọn
**MD53 — destroy no-leak e2e** (test_minictd.sh):
- Snapshot baseline: `pgrep -f "$STATE"` count + `grep "$STATE" /proc/self/mountinfo` line count.
- Run instance `md-leak` (webport, busybox rootfs — overlay/pasta env-gated SKIP như MD44-49).
- Stop + Remove.
- Verify: `pgrep -f "$STATE/md-leak"` = 0 (no shim/payload/webproxy/pasta left).
- Verify: `grep "$STATE/md-leak" /proc/self/mountinfo` = 0 (no stale bind mounts).
- Verify: instance dir gone (MD19 đã check, MD53 re-assert).
- Netns: không test trực tiếp — process + mount leak = 0 → netns tự reclaim (kernel semantics, research nguồn 3).

**MD54 — daemon idle RSS ≤ 30 MB** (test_minictd.sh):
- Daemon đã chạy (DPID, main test_minictd.sh daemon).
- Scrape `/metrics` `minictd_process_resident_memory_bytes` (D68 đã expose).
- Cross-check: `awk '/VmRSS/{print $2}' /proc/$DPID/status` (KB) × 1024 → phải khớp /metrics (±64 KB rounding).
- Assert: RSS ≤ 30 MB = 31457280 bytes (30 * 1024 * 1024). FAIL → test-minictd FAIL → CI đỏ.
- Env-gated: không cần (daemon RSS đo được trên mọi host có /proc — dev + CI).

**Perf-baseline**: cập nhật `docs/perf-baseline.md` với daemon idle RSS số đo (dev 6372 KB + CI runner số đo sau SCR).

## Lý do chọn (trade-off)
- **MD54 trong test_minictd.sh thay vì perf-check.sh**: perf-check chạy BEFORE setup-go (minictd chưa build). test_minictd.sh chạy AFTER setup-go + có daemon đang chạy → đo trực tiếp. Gate hiệu lực tương đương (test-minictd FAIL → CI đỏ).
- **Scrape /metrics thay vì chỉ đọc /proc**: verify cả hai nguồn khớp (D68 metrics endpoint chính xác + /proc ground truth). Nếu /metrics sai → MD54 catch.
- **Netns không test trực tiếp**: kernel reclaim netns khi process cuối exit (research nguồn 3). Process leak = 0 → netns reclaim. Test trực tiếp netns leak phức tạp (cần enumerate `/proc/*/ns/net` distinct) và fragile trên dev 5.10. Process + mount check đủ bao phủ.
- **Threshold 30 MB**: roadmap chỉ thị. Baseline dev ~6.4 MB → biên độ dư ~24 MB. Regression guard — nếu observability hoặc feature mới tăng RSS > 24 MB → catch.

## Phạm vi phiên này (DONE)
- MD53 (destroy no-leak: process + mount) + MD54 (daemon RSS ≤ 30 MB + /metrics cross-check).
- docs/perf-baseline.md cập nhật.
- docs/roadmap.md mục 8 → xong.

## Phạm vi KHÔNG làm (residual)
- netns leak test trực tiếp (process + mount đủ bao phủ).
- daemon RSS perf-guard workflow riêng (gate qua test_minictd.sh đủ).
- overlay/pasta leak paths (env-gated SKIP trên dev, PASS thật trên CI runner — MD53 chỉ test busybox rootfs path trên dev).

## Sau mục 8
GĐ2 DoD đạt → **DỪNG + nhắc owner trước khi mở GĐ3** (§8 chỉ thị owner, roadmap dòng 579-583).
