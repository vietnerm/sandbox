# Daily Log — 2026-11-26

## Phiên 33 — Orchestrator (ZAI, §11) — issue #168

### Pha A — Sync + SBG plan
- Pull main `77a2f35` (Phiên 32 ledger #167). 0 PR mở, 0 nhánh agent → Pha D.
- 9 issue mở (toàn p2/p3 from-audit + #8 CVE tracking p2). Không p0/p1 security chen ngang.
- Owner chỉ thị (2026-11-25): GĐ2 còn lại = mục thấp nhất chưa xong → **mục 7 Observability**.
- SBG plan: `journal/2026-11-26/branch/plan.md` — tạo `feat/168-observability`, KEEP 0, DELETE_MERGED sau Pha C.

### Triage + chọn issue
- Tạo **#168** `feat(minictd): observability — log JSON + Prometheus metrics + /metrics endpoint` (p1, area:minictd, type:feature). Assign + in-progress.

### Research (journal/2026-11-26/research.md)
- Nguồn 1: Prometheus text exposition format (official docs) — Content-Type v0.0.4, `# HELP`/`# TYPE` trước sample, escape `\\`/`\"`/`\n`, sort reproducible preferred.
- Nguồn 2: cgroups v2 interface files (kernel docs) — cpu.stat flat-keyed usage_usec, memory.current/max single value, io.stat nested-keyed `$MAJ:$MIN rbytes=… wbytes=…`, pids.current/max.
- Nguồn 3: gVisor metric package (precedent) — protobuf framework too heavy; học cách tách renderer thuần function.
- Nguồn 4: Go `log/slog` stdlib (JSONHandler) — stdlib Go 1.21+, ReplaceAttr cho normalize level, `.With()` mang context attrs.

### Development (`feat/168-observability`, D68)
- **7a log JSON**: `--log-format text|json` flag (default json); `slog.NewJSONHandler` + `ReplaceAttr` (level lowercase). `eventLog`/`eventLogErr` helpers trong runtime.go: lifecycle events (create/start/stop/destroy/snapshot/save-template) emit `instance`/`op`/`result`/`duration_ms`. `requestLog` middleware trong server.go: HTTP access log với `request_id` (X-Request-Id hoặc random hex 16-char) + `method`/`path`/`instance`/`op`/`status`/`result`/`duration_ms`. Token KHÔNG bao giờ log.
- **7b metrics**: `minictd/internal/minict/metrics.go` (~250 dòng stdlib-only) — `CollectMetrics` iterate instances đọc cgroup files (tái dùng `cgReadInt`/`cgCPUStatUsage` #4/D48 + thêm `cgIOStat` sum rbytes/wbytes); `renderPrometheus` 8 instance metrics (cpu_usage_seconds_total counter, memory_bytes/memory_limit_bytes/io_read/io_write/pids/pids_limit gauges, up{state} gauge) + 3 daemon (instances{state}, process_resident_memory_bytes, build_info{version,go_version,runtime}). Fail-soft: instance không cgroup → chỉ up/state. Deterministic sort (name, labels).
- **7c /metrics endpoint**: `GET /metrics` trên ServeMux sẵn có, same token auth (middleware `auth` wrap toàn mux). Content-Type `text/plain; version=0.0.4`. Scrape-time (no goroutine, no RAM history). `--metrics-listen` placeholder fail-closed — mọi giá trị non-empty → exit 1.
- **7d tests**: MD50 (log schema + no token leak) + MD51 (/metrics format + fail-soft + instance up) + MD52 (401 no-token + --metrics-listen fail-closed + 200 with token) + 10 Go unit (metrics_test.go: renderer escape/order/fail-soft, io.stat sum/empty/missing, daemon RSS, build_info labels, empty snapshot, instances states sort). Mutants M-render-escape/-order/-no-cgroup-skip/-io-stat-sum caught.
- **7e perf**: slog JSON handler stdlib (đã có import) + renderer ~250 dòng + requestLog middleware — RSS delta < 2 MB (no new deps, no prometheus/client_golang).

### Debug log (bằng chứng quá trình)
1. CI lần 1 đỏ (d6151bf): gofmt drift `cmd/minictd/main.go` — flag block alignment sai (showVer/logFormat/metricsLn column 11 vs 13). Fix `unexpand` + manual align → 14818ee.
2. CI lần 2 đỏ (14818ee): gofmt drift `internal/minict/metrics.go` — struct field alignment + map literal alignment + single-field comment spacing. Tải Go binary portable (`go.dev/dl/go1.23.4.linux-amd64.tar.gz`), chạy `gofmt -w` auto-fix → de927f9.
3. CI lần 3 đỏ (de927f9): TestRenderPrometheusInstanceWithCgroup FAIL — `FormatFloat('g')` produce scientific notation `1.048576e+06` cho 1048576, test expect `1048576`. Fix: `'g'` → `'f'` format → 32bc94d xanh.
4. Journal commit (7647d11) docs-only — CI re-green, SIA verdict carry over.

### CVE Check
- Kernel: `5.10.134-013.15.kangaroo.al8.x86_64`
- 4 CVE kernel tracked (watchlist, NONE mới): CVE-2022-0847/0185, CVE-2023-0386, CVE-2024-1086 — all patched/blocked.
- 0 CVE mới ngoài watchlist; 0 CVE ảnh hưởng sandbox (PR chỉ Go stdlib + cgroup file reads, không syscall surface mới).
- File: journal/2026-11-26/cve/observability.md

### Validation "không bị nhiễm"
- `make test` (10 suites): 0 FAIL (isolation 37 + seccomp 25 + netns 7 + alpine 9 + escape-alpine 38 + cve 4 + pasta 5+6 SKIP + stack 8 + cg 6 SKIP env-blocked)
- `make test-minictd`: PASS=46 SKIP=7 FAIL=0 (MD50/MD51/MD52 PASS; MD44-46 SKIP overlay env-gated kernel <5.11; MD47-49 SKIP pasta env-gated no /dev/net/tun)
- `make test-web`: 9/9 PASS
- Host baseline: unchanged (passwd `21d5de2d` / bash `223fe856` / sudo `a59f393b` — diff rỗng trước/sau)
- Perf: cold_start=2.50 ms | rss=1204 KB | disk=2.05 MB — PASS, đường cũ không đổi (perf-check đo minict wrapper, minictd KHÔNG đụng)
- Bằng chứng: journal/2026-11-26/evidence/observability.txt

### Pha B/C — PR #169 (squash `ccc0780`)
- B2 — SCR @ 7647d11: dev 46 PASS + 7 SKIP + 0 FAIL; runner CI cùng SHA **4/4 check-runs GREEN** (build-test ×2 push+PR + perf + scan); mutant 4/4 caught (M-render-escape/-order/-no-cgroup-skip/-io-stat-sum); control PASS; perf dev 2.50 ms / 1204 kB / 2.05 MB PASS; host unchanged.
- B3 — SIA @ 7647d11: PASS — 11/11 contract OK, claim sai 0, test rỗng 0 (4/4 mutant caught), bảo mật 0 critical/high/medium, 2 low (residual D68: cgroup value-match e2e + biểu đồ GĐ3, không chặn). Label `audit:pass`.
- B4/C — SBG apply: GraphQL `markPullRequestReadyForReview` (isDraft: false) → squash `--match-head-commit 7647d11` → **`ccc0780` trên main; #168 auto-close**; nhánh DELETE_MERGED.

### Pha E — Kết ngày (SBG)
- `feat/168-observability` DELETE_MERGED (remote + local pruned). **#168 XONG.**
- Ledger qua PR `chore(branches): ledger 2026-11-26 Phiên 33`.
- Remote kỳ vọng: chỉ `main`.

## Research
- Xem mục Research phía trên (4 nguồn: Prometheus exposition, cgroups v2, gVisor, slog).

## PR
- #169 — ready — CI: xanh (4/4 check-runs green on 7647d11) — merged squash `ccc0780`
- Merge: CÓ (#168 auto-close)

## Kế hoạch ngày mai
- GĐ2 còn lại: mục 8 (Đo: control plane idle ≤ 30 MB RSS tính cả observability; destroy không rò rỉ process/mount/netns) — đo trên CI runner (dev 5.10 không có cgroups v2 delegation). Sau mục 8 → GĐ2 DoD đạt → **DỪNG + nhắc owner trước khi mở GĐ3** (Web UI).
- Residual GĐ2: systemd-real (mục 1 residual, env-gated), #109 oci absolute-symlink (chờ maintainer), cgroup value-match e2e (test_cg.sh).

=== SDDA REPORT — 2026-11-26 ===
Issue xử lý : #168 — observability — log JSON + Prometheus metrics + /metrics endpoint
PR mở      : #169 — CI: xanh (4/4 on 7647d11) — draft: no (flipped via GraphQL) — merged squash ccc0780
CVE check  : 4 CVE tracked (watchlist), 0 mới, 0 ảnh hưởng sandbox
Escape test: PASS (10 suites 0 FAIL; test-minictd 46 PASS + 7 SKIP + 0 FAIL)
Host       : unchanged (passwd/bash/sudo sha256 match baseline)
Perf       : cold_start=2.50 ms | rss=1204 KB | disk=2.05 MB — PASS
Daily-log  : journal/2026-11-26/daily-log.md
Ngày mai   : GĐ2 mục 8 (Đo RSS ≤ 30 MB + destroy no-leak) → sau đó DỪNG nhắc owner GĐ3
Trạng thái : DAY PASS — #168 merged, CI xanh thật, 0 regression
================================
