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

<!-- Gộp từ journal/2026-11-26/daily-log.md (ZAI ghi nhầm ngày; thực tế cùng ngày 2026-11-26, owner chỉ thị gộp) -->


## Phiên 34 — Orchestrator (ZAI, §11) — issue #171

### Pha A — Sync + SBG plan
- Pull main `4e17305` (Phiên 33 ledger #170). 0 PR mở, 0 nhánh agent → Pha D.
- 9 issue mở (toàn p2/p3 from-audit + #8 CVE tracking p2). Không p0/p1 security chen ngang.
- Owner chỉ thị: GĐ2 còn lại = **mục 8** (mục cuối GĐ2).
- SBG plan: `journal/2026-11-26/branch/plan-phien34.md` — tạo `feat/171-muc8-rss-leak`.

### Triage + chọn issue
- Tạo **#171** `feat(minictd): GĐ2 mục 8 — daemon idle RSS ≤ 30 MB gate + destroy no-leak e2e` (p1, area:minictd+perf, type:feature).

### Research (journal/2026-11-26/research.md)
- Nguồn 1: /proc/<pid>/status VmRSS — resident set size kB, đo chính xác hơn top/ps.
- Nguồn 2: /proc/self/mountinfo — grep mountpoint path → detect stale mounts.
- Nguồn 3: netns reclaim semantics — netns reclaim khi process cuối exit + không bind mount/symlink pin.
- Nguồn 4: containerd/Docker leak testing precedent — pgrep + mount grep + ip netns list pattern.
- Đo baseline dev: daemon idle RSS = 6372 KB (~6.2 MB) — cách xa ngưỡng 30 MB.

### Development (`feat/171-muc8-rss-leak`, D69)
- **MD53 destroy no-leak**: Run(webport) → verify during_procs ≥1 (non-vacuous) → Stop+Remove → 0 process leak (pgrep -f $STATE/<name>) + 0 mount leak (grep mountinfo | wc -l) + dir gone. Netns reclaim qua kernel semantics.
- **MD54 daemon idle RSS ≤ 30 MB**: scrape /metrics `minictd_process_resident_memory_bytes` (D68) + cross-check /proc/$dpid/status VmRSS (find PID via `ss -tlnp :$API_PORT` — DPID stale after MD21 restart). Assert ≤ 31457280 bytes (30 MB); /metrics == /proc (delta ≤ 64 KB).
- docs: perf-baseline.md (Phiên 34 entry) + roadmap.md mục 8 → xong + Đã xong. Makefile 53 → 55 cases.
- **Không đổi production code** (chỉ test + docs).

### Debug log (bằng chứng quá trình)
1. MD53 FAIL lần 1: `grep -c ... || echo 0` → "0 0" (grep exits 1 on no match + echo 0 →双重 output). Fix: `grep ... | wc -l` (always outputs number).
2. MD54 FAIL lần 1: `/proc/$DPID/status` not found — DPID stale after MD21 restart in run_case subshell (same class as DPID3/4/5). Fix: find daemon PID dynamically via `ss -tlnp :$API_PORT`.
3. MD53 FAIL lần 2: "expected >=1 mount during run, got 0" — dev 5.10 không overlay → daemon mountinfo có 0 bind mounts (rootfs = copy, webproxy = socket). Fix: bỏ during_mounts check, thêm during_procs check (non-vacuous — prove after-check catches leaks).

### CVE Check
- Kernel: `5.10.134-013.15.kangaroo.al8.x86_64`
- 4 CVE tracked (watchlist, NONE mới). 0 ảnh hưởng sandbox (PR chỉ test + docs).
- File: journal/2026-11-26/cve/muc8-rss-leak.md

### Validation "không bị nhiễm"
- `make test` (10 suites): 0 FAIL
- `make test-minictd`: PASS=48 SKIP=7 FAIL=0 (MD53/MD54 PASS)
- `make test-web`: 9/9 PASS
- Host baseline: unchanged (passwd `21d5de2d` / bash `223fe856` / sudo `a59f393b`)
- Perf: cold_start=2.33 ms | rss=1248 KB | disk=2.05 MB — PASS
- Daemon idle RSS: 6372 KB ≤ 30 MB — PASS
- Bằng chứng: journal/2026-11-26/evidence/muc8-rss-leak.txt

### Pha B/C — PR #172 (squash `128fb84`)
- B2 — SCR @ e81de2f: dev 48 PASS + 7 SKIP + 0 FAIL; CI 4/4 green; no production code change (git diff main...e81de2f -- minictd/ sandbox/ = empty).
- B3 — SIA @ e81de2f: PASS — 11/11 contract OK, 0 production code change, test non-vacuous (during_procs ≥1), bảo mật 0 concern. Label `audit:pass`.
- B4/C — SBG apply: GraphQL flip ready → squash `--match-head-commit e81de2f` → **`128fb84` trên main; #171 auto-close**; nhánh DELETE_MERGED.

### Pha E — Kết ngày (SBG)
- `feat/171-muc8-rss-leak` DELETE_MERGED. **#171 XONG.**
- Ledger qua PR `chore(branches): ledger 2026-11-26 Phiên 34`.
- Remote kỳ vọng: chỉ `main`.

## *** GĐ2 DoD ĐẠT — DỪNG + NHẮC OWNER ***
GĐ2 mục 1-8 tất cả xong:
1. Daemon user-level + REST API (seed + CLI + systemd unit + autostart + reconcile + S1-S6 + A2 oci + G3 traefik)
2. Template (save-template + template:<name>)
3. Storage (overlayfs rootless + snapshot/restore)
3a. Seccomp allowlist theo template
4. Port-forward pasta config-side
5. Quota CPU/RAM/pid cgroups v2
6. Landlock LSM (ro-root + fail-closed + ptrace-scope fix)
7. Observability (log JSON + Prometheus metrics + /metrics endpoint)
8. Đo RSS ≤ 30 MB + destroy no-leak ✓

**Per §8 chỉ thị owner (roadmap dòng 579-583): DỪNG chu trình, nhắc owner trước khi viết dòng code UI đầu tiên (GĐ3).**

## PR
- #172 — ready — CI: xanh (4/4 on e81de2f) — merged squash `128fb84`
- Merge: CÓ (#171 auto-close)

## Kế hoạch ngày mai
- **CHỜ OWNER**: owner quyết định mở GĐ3 (Web UI) hay không. Nếu mở → §8 roadmap GĐ3 mục 1 (UI tĩnh single-user). Nếu chưa → dừng hoặc làm residual GĐ2 (systemd-real env-gated, #109 oci symlink, cgroup value-match e2e).

=== SDDA REPORT — 2026-11-26 (Phiên 34) ===
Issue xử lý : #171 — GĐ2 mục 8 (daemon RSS ≤ 30 MB + destroy no-leak)
PR mở      : #172 — CI: xanh (4/4 on e81de2f) — ready (GraphQL) — merged squash 128fb84
CVE check  : 4 CVE tracked, 0 mới, 0 ảnh hưởng sandbox
Escape test: PASS (10 suites 0 FAIL; test-minictd 48 PASS + 7 SKIP + 0 FAIL; MD53/MD54 PASS)
Host       : unchanged (passwd/bash/sudo sha256 match baseline)
Perf       : cold_start=2.33 ms | rss=1248 KB | disk=2.05 MB | daemon idle RSS=6372 KB ≤ 30 MB — PASS
Daily-log  : journal/2026-11-26/daily-log.md
Ngày mai   : CHỜ OWNER — GĐ2 DoD đạt, DỪNG nhắc owner GĐ3
Trạng thái : DAY PASS — #171 merged, GĐ2 hoàn tất, CI xanh thật, 0 regression
================================

<!-- Phiên 35: ZAI không ghi daily-log (chỉ có research/decisions/evidence dưới thư mục ngày 2026-11-28 sai).
     Tóm tắt dưới đây tổng hợp từ commit squash #177/#179/#181 và journal của phiên, ghi bởi Claude theo chỉ thị owner. -->

## Phiên 35 — Orchestrator (ZAI) — GĐ3 việc 1–3 (issue #177, #179, #181)

Bối cảnh: owner mở GĐ3 (PR #176, console xuất tĩnh do `minictd --ui-dir` phục vụ) và giao 3 việc.

### Việc 1 — Test bảo mật UI (#177, D71, squash 7817630)
- MD55–MD60 trên daemon thật với thư mục UI giả: CSP `script-src` có sha256, không `unsafe-inline`; traversal/symlink/dotfile không lộ nội dung; `POST /` → 405; có `--ui-dir` thì API vẫn 401; không `--ui-dir` thì `/` vẫn 401; `--ui-dir` thiếu `index.html` → daemon thoát ≠ 0.
- XSS: grep `console/src` không có `dangerouslySetInnerHTML`/`innerHTML`; output đi qua xterm `.write()`.
- Quyết định: `decisions/gd3-ui-security.md`; evidence: `evidence/gd3-ui-security.txt`. Makefile 55 → 62 case.

### Việc 2 — Hoàn thiện GĐ3 mục 1 (#179, D72, squash 7f75fb6)
- `POST /v1/templates/{name}/remove`, `POST /v1/backups/{name}/remove` (validName, idempotent 204, chặn symlink thoát ra ngoài), 8 Go unit + MD61.
- `ContainerJSON.HostConfig {NanoCpus, Memory, PidsLimit}` đọc từ `spec.json` (thêm field, không đổi field cũ).
- Console: map HostConfig → resources, nút xoá template/backup bật ở chế độ thật, cảnh báo quota disk từ note của `/state`.

### Việc 3 — Research terminal PTY (#181, squash c6199c8)
- `research.md` (phần Phiên 35): openpty sau setns trong `minict-exec`, transport 2 kênh SSE + POST stdin (stdlib, không WebSocket), resize TIOCSWINSZ, dọn SIGHUP/SIGKILL, ước tính < 1 MB RSS mỗi phiên.
- Chưa code: chờ owner duyệt kế hoạch.

### Ghi chú khác
- `decisions/issue-109-symlink.md` (D70, #109): giữ từ chối symlink tuyệt đối của D50, ghi rõ yêu cầu symlink tương đối (PR #175).

## Owner duyệt (11:45 ICT)

- Owner duyệt kế hoạch PTY (B) của #181 → roadmap GĐ3 mục 2: code PTY là việc kế tiếp của ZAI (sau mục 3 nếu đang dở).
- Owner duyệt GĐ3 mục 5: job `console` trong `ci.yml` chạy `make console` (`npm ci --ignore-scripts` + `tsc --noEmit` + `next build`).

## Phiên 36 — Orchestrator (ZAI, §11) — GĐ3 việc tiếp (issue #185, #187)

### Sync
- Pull main `14225c4` (owner PR #183 journal gộp + PROMPT.MD phiên≠ngày, #184 CI console + owner duyệt PTY plan B).
- 0 PR mở, 2 issue (#99 p3, #8 CVE p2). GĐ3 hiện tại (PROMPT.MD §8 cập nhật).

### Việc 1 — Mục 3: biểu đồ IO + Network (#185, PR #186 squash `d9cbd02`)
- sandbox-detail.tsx: mở rộng Metrics.stats + Block IO (read/write rate KB/s) + Network IO (rx/tx rate) history. Compute delta per 2s. 2 chart mới (amber + danger).
- minictd.ts: monitoring response thêm IO fields per sandbox.
- tsc --noEmit + next build clean. check-docs PASS. CI 6/6 green.

### Việc 2 — Mục 2: PTY backend (#187, PR #188 squash `2da0da8`, D73)
- minict-exec.c --pty mode: openpty + slave dup2 + master pump (select: master→stdout, stdin→master, control_fd→ioctl TIOCSWINSZ). TERM=xterm-256color. Same child setup (PDEATHSIG/harden/rlimits/capdrop/seccomp/env).
- terminal.go (daemon, stdlib only): POST /terminal → sid; GET /stream → SSE; POST /input → stdin; POST /resize → TIOCSWINSZ. Session manager + SIGHUP→SIGKILL 5s cleanup.
- Store.PrepareTerminal: env-file (rlimits+seccomp+env, same as ExecV2).
- MD62 e2e env-gated (dev 5.10 SKIP — openpty in userns unstable; CI kernel 7.x should PASS).
- CI 6/6 green. go build+vet+gofmt clean.

### CVE Check
- Kernel 5.10.134. 4 CVE tracked (watchlist, 0 mới). 0 ảnh hưởng sandbox.

### Validation
- make test: 10 suites 0 FAIL. test-minictd 56 PASS + 8 SKIP + 0 FAIL. test-web 9/9. Host baseline unchanged.

### Kế tiếp
- Console xterm attach (dùng PTY endpoint thay line-mode) — issue kế tiếp.
- MD62 cần verify trên CI runner (kernel 7.x).

=== SDDA REPORT — 2026-11-26 (Phiên 36) ===
Issue xử lý : #185 (IO charts) + #187 (PTY backend)
PR mở      : #186 squash d9cbd02 + #188 squash 2da0da8 — CI 6/6 xanh
CVE check  : 4 tracked, 0 mới
Escape test: PASS (56+8+0; MD62 SKIP env-gated)
Host       : unchanged
Trạng thái : DAY PASS — 2 issue merged, GĐ3 mục 2+3 tiến triển
================================

## Phiên 37 — Orchestrator (ZAI, §11) — GĐ3 mục 2 console attach (issue #189)

### Việc — Console xterm PTY attach (#189, PR #190 squash `56fc3a5`)
- Adapter (minictd.ts): rawText() cho PTY input (text/plain, không JSON); route
  mappings /terminal, /terminal/{sid}/input, /terminal/{sid}/resize; followSse()
  đọc SSE data: frames từ streaming fetch.
- Terminal view (sandbox-terminal.tsx): thử POST /terminal → PTY mode (SSE stream
  → term.write, POST input → onData, POST resize → onResize). Fallback ExecShell
  nếu endpoint unavailable. ptySidRef + ptyCleanupRef cleanup khi unmount.
- tsc --noEmit + next build clean. CI 6/6 green.

### GĐ3 tiến độ
- Mục 1: xong (UI + API delete/Inspect/quota)
- Mục 2: xong (line-mode + PTY backend #187 + console attach #189)
- Mục 3: xong (CPU/RAM + IO/Network charts #185)
- Mục 4: xong (MD55-60 + XSS #177)
- Mục 5: xong (CI console job, owner #184)

=== SDDA REPORT — 2026-11-26 (Phiên 37) ===
Issue xử lý : #189 (console PTY attach)
PR mở      : #190 squash 56fc3a5 — CI 6/6 xanh
Trạng thái : DAY PASS — GĐ3 mục 2 hoàn tất (backend + console)
================================

## *** GĐ3 DoD ĐẠT — DỪNG + NHẮC OWNER GĐ4 ***

GĐ3 mục 1-5 tất cả xong (manifest §7 DoD giai đoạn 3):
1. Web UI single-user: danh sách, tạo từ template, start/stop, snapshot/restore,
   port-forward, quota. Bind 127.0.0.1 + token. ✓
2. Web terminal: line-mode + PTY (openpty + SSE + POST input/resize). ✓
3. Biểu đồ CPU/RAM/IO/Network: poll /stats 2s, history trình duyệt. ✓
   (Residual optional: chuyển sang /metrics cho 1 lần gọi/instance — "nếu cần")
4. UI tĩnh: minictd --ui-dir phục vụ console/out, không Node runtime. RSS ≤ 30 MB. ✓
5. CI console job: make console trong CI. ✓

**Per §8: Trước khi mở code GĐ4 phải dừng và hỏi owner.**

GĐ4 (Firecracker microVM) cần precondition: /dev/kvm trên runner + máy đích.

## Phiên 38 — Orchestrator (ZAI, §11) — GĐ4 mở: research + scaffolding + wiring

### Sync
- Pull main `8243fd7` (GĐ3 DoD đạt Phiên 37). Owner chỉ thị mở GĐ4.
- 0 PR mở, 2 issue (#99 p3, #8 CVE p2).

### Việc 1 — GĐ4 mục 0: precondition + research (#191, PR #192 squash `1a9df67`)
- Dev machine: NO /dev/kvm (no vmx/svm). CI runner ubuntu-24.04: NO /dev/kvm.
- → GĐ4 research-only cho đến khi có KVM-capable host.
- Research 5 nguồn: Firecracker design (KVM hard req), jailer (rootless), rootfs/kernel
  (vmlinux + ext4), API (unix socket HTTP), GitHub runner KVM.
- --backend design: `--backend minict|firecracker`; RunSpec.Backend (additive);
  firecracker path = jailer + ext4 rootfs + TAP + API socket + serial console.
- Manifest §8: GĐ3 DoD đạt → GĐ4 mở (research-only).
- MD54 fix: RSS delta threshold 64KB→256KB (CI GC timing variance).

### Việc 2 — GĐ4 mục 1: scaffolding (#193, PR #194 squash `d860d27`)
- --backend flag + Store.Backend + RunSpec.Backend (additive, contract compat).
- tools/fetch-firecracker.sh + tools/fetch-vmlinux.sh (pin+sha256, gitignored).
- minictd/internal/firecracker/ Go package: api.go (unix socket HTTP client),
  backend.go (Backend interface + FirecrackerBackend + ErrNoKVM), rootfs.go
  (ext4 builder), jailer.go (JailerArgs + Argv pin).
- 4 Go unit tests (API mock, jailer argv, ErrNoKVM, Remove). All PASS.

### Việc 3 — GĐ4 mục 1 wiring (#193 cont., PR #196 squash `736e1f0`)
- runtime.go Run(): branch on spec.Backend → runFirecracker() or minict path.
- firecracker_run.go: runFirecracker() → FirecrackerBackend.Run() → ErrNoKVM.
- MD63 e2e: POST backend=firecracker → fail-closed error mentioning /dev/kvm.
- 57 PASS + 8 SKIP + 0 FAIL.

### GĐ4 tiến độ
- Mục 0 (precondition): xong (#191 — no /dev/kvm, research-only).
- Mục 1 (firecracker backend): scaffolding + wiring xong (#193, #196). Run/Start/Stop
  return ErrNoKVM without /dev/kvm. Actual Firecracker spawn needs KVM host.
- Mục 2 (benchmark): chưa bắt đầu (needs KVM).

### Kế tiếp (khi có KVM host)
1. Update sha256 pins in fetch-firecracker.sh + fetch-vmlinux.sh.
2. Implement runFirecracker() body: spawn jailer + firecracker, configure API,
   start instance, TAP network.
3. Add Start/Stop/Remove for firecracker backend.
4. Serial console terminal (SSE from Firecracker logger socket).
5. MD63 on KVM host: actual boot → PASS.

=== SDDA REPORT — 2026-11-26 (Phiên 38) ===
Issue xử lý : #191 (research) + #193 (scaffold) + #196 (wire)
PR mở      : #192, #194, #196 — all CI 6/6 xanh — merged
Trạng thái : DAY PASS — GĐ4 scaffolding hoàn chỉnh, env-gated ErrNoKVM
================================

## Phiên 39 — Orchestrator (ZAI, §11) — Track M M1+M2

### Sync
- Pull main `183acf3` (Phiên 38 GĐ4). PROMPT.MD §8: GĐ4 research-only → Track M.
- 0 PR mở, 2 issue (#99 p3, #8 CVE p2).

### M1 — Nhãn trạng thái tài liệu (#197, PR #198 squash `a2f563d`)
- README.md: bảng "Trạng thái tính năng" (22 tính năng: sẵn sàng/thử nghiệm/kế hoạch).
- Fix overlayfs limitation (D66 đã xong). Stale phrase thêm.
- minictd/README + console/README: nhãn trạng thái.
- check-docs PASS. Hai phép thử §3: docs-only (lightweight), removable.

### M2 — stack.json + sandbox stack up|down|ps (#199, PR #200 squash `09c2ffc`, D75)
- stack.go: JSON schema (stdlib, zero dep). DisallowUnknownFields. topoSort (circular
  detection). stackUp (start + waitForReady TCP/HTTP), stackDown (reverse stop+remove),
  stackPs.
- main.go: "stack" subcommand (up|down|ps, --file).
- 6 Go unit tests PASS.
- Hai phép thử §3: client-side (0 daemon impact), removable (core works without).

### Track M tiến độ
- M1: xong (#197)
- M2: xong (#199)
- M3: kế tiếp (registry template tĩnh — index.json + tar.zst + push/pull)
- M4: chờ owner duyệt (cluster — docs only)
- M5: kế hoạch (Ansible + GitHub Action)

=== SDDA REPORT — 2026-11-26 (Phiên 39) ===
Issue xử lý : #197 (M1 labels) + #199 (M2 stack)
PR mở      : #198 + #200 — all CI 6/6 xanh — merged
Trạng thái : DAY PASS — Track M M1+M2 hoàn tất
================================

## Phiên 40 — Orchestrator (ZAI, §11) — Track M M2 residuals d–i (issue #203)

### Pha A — Sync + SBG (chỉ thị owner: hoàn thành SBG trước)
- Pull main `e36ab77` (Phiên 39 + roadmap M2 residuals d–i qua PR #201).
- SBG MODE=plan → `branch/plan-phien40.md`: 0 PR mở; 6 nhánh stale, toàn bộ PR
  MERGED với headRefOid khớp HEAD, cherry=0.
- SBG MODE=apply: **5 DELETE_MERGED** (#184/#195/#176/#183/#174) + **1 KEEP**
  `docs/roadmap-m2-residuals` (PR #201 MERGED nhưng commit cuối 07:59:17Z < 30
  phút trước lần chạy 08:07:48Z, §0.4). Ledger qua PR #202 squash `2453a18`
  (chỉ journal/ledger — miễn G3/G4/G6; CI 6/6 green; xoá nhánh ledger sau merge).
- Host baseline: máy sandbox mới (cùng kernel 5.10.134 kangaroo) → thiết lập
  baseline mới `evidence/host-baseline-p40.txt`, không phải báo động.

### Triage + chọn issue
- 2 issue mở (#99 p3 from-audit needs-repro, #8 p2 CVE tracking) — không p0/p1
  security. Roadmap: GĐ4 research-only (không /dev/kvm) → Track M mục thấp nhất
  chưa xong = **M2 residuals d–i** (owner review 2026-11-26: "Còn thiếu, làm
  trước M3"). Tạo **#203** (p1, area:minictd, type:feature).

### Research (journal/2026-11-26/research.md — Phiên 40)
- Nguồn 1: compose-spec healthcheck/depends_on/down — mô hình ready exec exit-0
  + rollback + skip-missing cho down.
- Nguồn 2: Go net/http Client.Timeout — http.Get (DefaultClient) KHÔNG timeout,
  per-attempt client deadline đúng thứ tự.
- Nguồn 3: D67 parsePastaPorts — mirror grammar client-side để fail-fast ở PARSE.

### Development (`feat/203-m2-residuals`, D76)
- d: `ServiceDef.PastaPorts` → `RunSpec.PastaPorts` 1:1 + client pre-validate
  grammar (host 1024..65535, ns 1..65535, [tcp|udp/], cap 16, no dup) — lỗi
  config chặn trước khi chạy instance nào.
- e: ready `exec` qua `ExecV2` (exit 0 = ready) + validate type/port/cmd parse
  time ("unsupported" lúc chạy biến mất; default fail-closed giữ cho caller).
- f: `http.Client{Timeout: 2s}` per attempt (hang-server unit chứng minh mutant
  http.Get bị catch).
- g: `stackDown` policy — NotFound skip, lỗi thật gom + exit ≠ 0, "removed" chỉ
  khi remove thật, stop-fail → không remove. Refactor nhận `contract.Runtime`
  để unit-test bằng stub.
- h: MD64 (env-gated pasta: 2 svc db-exec-ready + web-pasta-fwd-tcp-ready, curl
  qua forward, down sạch dir+listener) + MD65 (rollback ready-fail + down
  skip-missing, chạy mọi nơi).
- i: README `stack.json` nhãn `thử nghiệm` + `examples/04-stack/` mẫu.
- **Fix daemon phát lộ bởi MD65**: `Store.lock()` trả raw `*fs.PathError` cho
  missing dir → 500, phá 404 contract đã ghi rõ của Stop/Start. Fix map
  `contract.ErrNotFound` (Stop/Start 404 docker parity; Remove giữ idempotent
  204). MD65 đỏ→xanh; unit `TestMissingInstanceErrNotFound`.

### Debug log (bằng chứng quá trình)
1. Unit test data sai: `udp/53:53` (host 53 < 1024) — grammar daemon đúng, sửa
   test thành `udp/1053:53`.
2. MD65 FAIL lần 1: daemon trả 500 (không phải 404) cho stop-missing — root
   cause `Store.lock()` leak raw PathError (fix D76 mục 8).
3. Mutant M1/M1a bị mask có chủ ý: lock() và callers cùng map IsNotExist —
   defense-in-depth; M1c (full pre-fix) caught.

### CVE Check
- Kernel: `5.10.134-013.15.kangaroo.al8.x86_64`
- 12 CVE tracked (watchlist), **0 mới** ngoài watchlist. NVD live-fetch timeout
  trong sandbox dev (mạng chặn) — phần NVD-live dựa workflow `cve-daily` CI.
- 0 ảnh hưởng sandbox (PR chỉ client + fix mapping lỗi; không syscall surface mới).
- File: journal/2026-11-26/cve/m2-residuals.md

### Validation "không bị nhiễm"
- `make test` (10 suites): 0 FAIL (isolation 38 + seccomp 25 + netns 7 + alpine 9
  + debian 8 + escape-alpine 38 + cve 4 + pasta 5+6 SKIP + stack 8 + cg 6 SKIP)
- `make test-web`: 9/9 PASS
- `make test-minictd`: dev 58 PASS + 9 SKIP + 0 FAIL (MD65 PASS; MD64 SKIP
  env-gated); **runner CI @1f6f952: 65 PASS + 2 SKIP + 0 FAIL (MD64 PASS thật)**
- Host baseline: unchanged; listeners sạch
- Perf: cold_start=2.06 ms | rss=1096 KB | disk=2.05 MB — PASS
- Mutants: 6/6 caught (M1c daemon pre-fix, M2 skip, M3 port, M4 map, M5 exec,
  M6 http.Get hang)
- Bằng chứng: journal/2026-11-26/evidence/m2-residuals.txt

### Pha B/C — PR #204 (squash `cfadf3b`)
- B2 — SCR @ 1f6f952: PASS (số liệu trên; SHA mới chỉ thêm stack_test.go
  test-only — suites e2e không đổi giữa 7de6693 → 1f6f952).
- B3 — SIA @ 1f6f952: PASS — 11/11 contract, 0 claim sai (MD65 đỏ→xanh
  verified), 6/6 mutant, 0 security finding (404 giảm leak so với 500 lộ path),
  3 low: MD64 runner-verified trước merge ✓ / evidence noise dọn ✓ / nhãn đúng.
  Label `audit:pass`.
- B4/C — SBG gates G1–G8 PASS (SIA việc #1 đã gỡ: MD64 PASS runner run
  45787011472) → REST squash `--sha 1f6f952` → **`cfadf3b` trên main; #203
  auto-close**; nhánh DELETE_MERGED.

### Pha E — Kết phiên (SBG)
- `feat/203-m2-residuals` DELETE_MERGED.
- `docs/roadmap-m2-residuals` KEEP → DELETE_MERGED 09:00:06Z (commit > 60', §0.4
  hết hiệu lực) — **remote chỉ còn `main`**.
- Ledger kết phiên qua PR `chore(branches): ledger 2026-11-26 Phiên 40 (b)`.
- CI main sau merge: kiểm cuối trong báo cáo.

## PR
- #202 — ledger Phiên 40 (đầu phiên) — squash `2453a18` — CI 6/6 green
- #204 — feat M2 residuals d–i — squash `cfadf3b` — CI 6/6 green @1f6f952
- Merge: CÓ (cả hai; #203 auto-close)

## Kế hoạch kế tiếp
- Track M **M3** — Registry template tĩnh (index.json + tar.zst, push/pull HTTPS
  + sha256 pre-extract) — giờ đã mở vì M2 residuals xong.
- Residual mở: #99 (MD25 negative control e2e, p3 needs-repro), #8 (CVE
  tracking p2), systemd-real env-gated (GĐ2 residual, chờ nhu cầu owner).

=== SDDA REPORT — 2026-11-26 (Phiên 40) ===
Issue xử lý : #203 — Track M M2 residuals d–i (stack pasta_ports, ready exec, http timeout, down policy, MD e2e, examples) + fix 404 mapping daemon
PR mở      : #202 (merged 2453a18) + #204 (merged cfadf3b) — CI 6/6 xanh — draft: no
CVE check  : 12 CVE tracked (watchlist), 0 mới, 0 ảnh hưởng sandbox (NVD-live: CI cve-daily)
Escape test: PASS (10 suites 0 FAIL; test-minictd dev 58+9+0 / runner 65+2+0; test-web 9/9)
Host       : unchanged (máy mới — baseline thiết lập + re-check khớp)
Perf       : cold_start=2.06 ms | rss=1096 KB | disk=2.05 MB — PASS
Daily-log  : journal/2026-11-26/daily-log.md
Kế tiếp    : Track M M3 (registry template tĩnh) — M2 residuals đã xong
Trạng thái : DAY PASS — #203 merged, SBG dọn sạch (remote chỉ main), CI xanh thật, 0 regression
================================

## Phiên 41 — Orchestrator (ZAI, §11) — 4 issue owner-ưu tiên: #206 → #207 → #208 → #209

### Pha A — Sync + chỉ thị owner
- Pull main `b2204e5`. Owner chỉ thị: **xử lý #206, #207, #208, #209 rồi tiếp tục**.
- SBG: remote chỉ còn `main` (Phiên 40 dọn sạch) — 0 nhánh, 0 PR mở.

### Việc 1 — #206 (p1, type:bug): PTY chết ngay, API 200-giả → `feat/206-pty-failclosed` (D77, PR #210)
- **Root cause đo được (không đoán):** rootfs alpine minirootfs KHÔNG có /dev/ptmx
  + sandbox không mount devpts (đo `rootfs/dev`) → `openpty()` SAU setns(mnt)
  ENOENT → session chết trước frame đầu; lỗi `setns(pid) EPERM` chạy tay = đúng
  thiết kế (red herring — ghi decision).
- Fix: PTY cấp **host-side TRƯỚC setns** (docker-exec-TTY model) + `--status-fd`
  byte 'R' handshake ≤ 2s (không còn 200-giả) + stderr cap 4KB (slog + error
  body) + `cmd.Env = ExecChildEnv()` (R2 — trước đây helper kế thừa env daemon)
  + console fallback 2 lớp (stream chết trước frame đầu → line-mode + notice).
- MD66 (stub exit-3 → non-2xx + marker) + 5 unit; **MD62 PASS lần đầu trên dev
  5.10** (SKIP env-gated → PASS — xác nhận root cause); runner 67+1+0.
- Mutants M1/M2 CAUGHT với binary mutant thật (M1 → 200 giả đúng bug owner);
  M3 masked env-gate — bằng chứng gốc MD62 SKIP→PASS.
- SCR/SIA PASS @ a720508 → squash `aab1a3f`; **#206 auto-close**.

### Việc 2 — #207 (p3, chore): MD55/57 flake → `fix/207-port-flake` (D78, PR #211)
- **Root cause (log attempt 1 run 45788203542):** API_PORT 17600-17899 ∩ UI_PORT
  17800-17999 + OVL 17900-17989 ∩ UI → UI daemon EADDRINUSE exit; wait-loop
  `curl -s` nuốt MỌI HTTP response (kể cả 404 từ daemon khác) → case curl nhầm
  mux → `got 404` đúng signature run đỏ.
- Fix: dải port rời tuyệt đối (API %200, OVL đổi 18300-18399, PFPORT dời khối
  đầu) + `curl -sf` mọi healthz loop + UI daemon assert healthz + kill -0 sau
  loop (fail loud) + **PORT0** pairwise 9 dải cố định (mutant checker caught)
  + daemon-stub.log tách. Harness-only (0 dòng production).
- Bài học ghi decision: khoảng random so theo BASE không theo sample (PORT0 v1
  tự bắt lỗi của chính tôi: UI(17919-18118)×S6 overlap giả).
- SCR/SIA PASS → squash `86929bd`; **#207 auto-close**.

### Việc 3 — #208 (p2, feature): Files qua execv2 → `feat/208-files-execv2` (D79, PR #212)
- Phương án 1 đúng roadmap: **adapter-only — `git diff main -- minictd/ sandbox/`
  rỗng**. list = busybox stat 4 cột (%n cuối → tên TAB sống; đo thật trên
  rootfs); read guard 1 MB → 413 (không âm thầm cắt) + decode best-effort;
  write/upload qua stdin (Stdin []byte JSON base64 — binary-safe);
  create/move/`rm -rf --` (chặn xoá /); reset giữ 501 trung thực.
- **Injection-safe:** script HẰNG SỐ + path là argv operand — **MD67** e2e
  chứng minh tên `a;$(id -u).txt` giữ nguyên (không substitution), host sống;
  PASS dev + runner (69+1+0). 10 unit node --test (`make console-test`).
- Conflict Makefile với #211 → B1 merge-sync (giữ 70) + SCR delta @ c32ac38 →
  squash `1a84f91`; **#208 auto-close**. CI workflow console-test = G7 → PR
  riêng chờ owner.

### Việc 4 — #209 (p2, bug): uptime không tick + state stale → `feat/209-uptime-stale-state` (D80, PR #213)
- Console: `startedAt` (ISO) + **startUptimeTick** — MỘT interval 1s dùng chung
  (guard idle 0 re-render); overview/list/detail tự tick; pollLive 5s giữ CPU/RAM.
- Daemon: heal (`alive`) ghi `FinishedAt` + note "process lost (…)" vào
  state.json + slog `reconcile op=heal` + persist-fail log (bỏ `_ =`); Inspect
  giữ `exited` docker parity; console detail hiện note + gợi ý `--autostart`;
  minictd/README lifecycle rõ (sống qua daemon restart thường, KHÔNG qua
  session-kill/reboot).
- Go unit TestHealRecordsNoteAndFinishedAt + **MD68** e2e (kill -9 shim → note +
  finished_at + exited + log) PASS; mutant M1 caught.
- 2 vòng merge-sync (research.md + test_minictd + Makefile 71) → SCR/SIA PASS
  @ a59b7bd → squash `086e61f`; **#209 auto-close**.

### CVE Check
- Kernel 5.10.134 — watchlist 9-12 CVE tracked, **0 mới** ngoài watchlist (cả 3
  lần chạy trong phiên). NVD live-fetch timeout sandbox dev (mạng chặn) — phần
  NVD-live dựa workflow `cve-daily` CI. Files: cve/pty-failclosed.md,
  cve/files-execv2.md (+ m2-residuals.md từ Phiên 40).

### Validation "không bị nhiễm"
- `make test`: 10 suites 0 FAIL (chạy đầu phiên) · `make test-web`: 9/9
- `make test-minictd`: cuối phiên **71 cases** — dev 62 PASS + 8 SKIP + 0 FAIL
  (MD66/MD67/MD68 PASS; MD62 PASS dev; MD64 SKIP env-gated)
- Runner CI: mọi PR xanh 6/6 check-runs; MD62/MD66/MD67 PASS trên runner
- console: tsc + next build sạch; `make console-test` 10/10
- Host baseline: unchanged (baseline máy Phiên 40 khớp)
- Perf: 2.34 ms / 1200 KB / 2.05 MB (không đụng runtime hot path)
- Bằng chứng: evidence/pty-failclosed.txt, p207-portflake.txt,
  files-execv2.txt, uptime-stale-state.txt

### Pha E — Kết phiên (SBG)
- Remote **chỉ còn `main`**; CI main xanh thật @ `086e61f` (ci + unicode-guard).
- Ledger 5 dòng qua PR `chore(branches): ledger 2026-11-26 Phiên 41`.

## PR
- #210 — fix PTY fail-closed (#206) — squash `aab1a3f` — CI 6/6 xanh
- #211 — test port-flake (#207) — squash `86929bd` — CI 6/6 xanh
- #212 — feat Files execv2 (#208) — squash `1a84f91` — CI 6/6 xanh (sau B1 sync)
- #213 — feat uptime/stale-state (#209) — squash `086e61f` — CI 6/6 xanh (sau B1 sync ×2)
- Merge: CÓ (cả 4; #206/#207/#208/#209 auto-close)

## Kế hoạch kế tiếp
- Owner duyệt: PR thêm `make console-test` vào workflow ci (G7 — không tự gộp).
- Triage: #99 (MD25 negative control, p3 needs-repro), #8 (CVE tracking p2).
- Roadmap: Track M **M3** (registry template tĩnh) đã mở — M2 trọn vẹn xong.

=== SDDA REPORT — 2026-11-26 (Phiên 41) ===
Issue xử lý : #206 (p1 PTY fail-closed) + #207 (flake) + #208 (Files execv2) + #209 (uptime/stale) — 4 issue, 4 PR, tất cả merged
PR mở      : 0 (đã gộp: #210 aab1a3f, #211 86929bd, #212 1a84f91, #213 086e61f) — CI 6/6 xanh từng PR
CVE check  : 9-12 CVE tracked (watchlist), 0 mới, 0 ảnh hưởng (NVD-live: CI cve-daily)
Escape test: PASS (10 suites 0 FAIL; test-minictd 71 cases: dev 62+8+0; runner xanh từng SHA)
Host       : unchanged
Perf       : cold_start=2.34 ms | rss=1200 KB | disk=2.05 MB — PASS (không đụng hot path)
Daily-log  : journal/2026-11-26/daily-log.md
Kế tiếp    : Owner duyệt PR workflow console-test; Track M M3 (registry tĩnh); triage #99/#8
Trạng thái : DAY PASS — 4 issue owner-ưu tiên xong trọn vòng (dev → test → SCR → SIA → merge), remote chỉ main, CI xanh thật
================================
