# Daily Log — 2026-11-27

## Phiên 34 — Orchestrator (ZAI, §11) — issue #171

### Pha A — Sync + SBG plan
- Pull main `4e17305` (Phiên 33 ledger #170). 0 PR mở, 0 nhánh agent → Pha D.
- 9 issue mở (toàn p2/p3 from-audit + #8 CVE tracking p2). Không p0/p1 security chen ngang.
- Owner chỉ thị: GĐ2 còn lại = **mục 8** (mục cuối GĐ2).
- SBG plan: `journal/2026-11-27/branch/plan.md` — tạo `feat/171-muc8-rss-leak`.

### Triage + chọn issue
- Tạo **#171** `feat(minictd): GĐ2 mục 8 — daemon idle RSS ≤ 30 MB gate + destroy no-leak e2e` (p1, area:minictd+perf, type:feature).

### Research (journal/2026-11-27/research.md)
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
- File: journal/2026-11-27/cve/muc8-rss-leak.md

### Validation "không bị nhiễm"
- `make test` (10 suites): 0 FAIL
- `make test-minictd`: PASS=48 SKIP=7 FAIL=0 (MD53/MD54 PASS)
- `make test-web`: 9/9 PASS
- Host baseline: unchanged (passwd `21d5de2d` / bash `223fe856` / sudo `a59f393b`)
- Perf: cold_start=2.33 ms | rss=1248 KB | disk=2.05 MB — PASS
- Daemon idle RSS: 6372 KB ≤ 30 MB — PASS
- Bằng chứng: journal/2026-11-27/evidence/muc8-rss-leak.txt

### Pha B/C — PR #172 (squash `128fb84`)
- B2 — SCR @ e81de2f: dev 48 PASS + 7 SKIP + 0 FAIL; CI 4/4 green; no production code change (git diff main...e81de2f -- minictd/ sandbox/ = empty).
- B3 — SIA @ e81de2f: PASS — 11/11 contract OK, 0 production code change, test non-vacuous (during_procs ≥1), bảo mật 0 concern. Label `audit:pass`.
- B4/C — SBG apply: GraphQL flip ready → squash `--match-head-commit e81de2f` → **`128fb84` trên main; #171 auto-close**; nhánh DELETE_MERGED.

### Pha E — Kết ngày (SBG)
- `feat/171-muc8-rss-leak` DELETE_MERGED. **#171 XONG.**
- Ledger qua PR `chore(branches): ledger 2026-11-27 Phiên 34`.
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

=== SDDA REPORT — 2026-11-27 (Phiên 34) ===
Issue xử lý : #171 — GĐ2 mục 8 (daemon RSS ≤ 30 MB + destroy no-leak)
PR mở      : #172 — CI: xanh (4/4 on e81de2f) — ready (GraphQL) — merged squash 128fb84
CVE check  : 4 CVE tracked, 0 mới, 0 ảnh hưởng sandbox
Escape test: PASS (10 suites 0 FAIL; test-minictd 48 PASS + 7 SKIP + 0 FAIL; MD53/MD54 PASS)
Host       : unchanged (passwd/bash/sudo sha256 match baseline)
Perf       : cold_start=2.33 ms | rss=1248 KB | disk=2.05 MB | daemon idle RSS=6372 KB ≤ 30 MB — PASS
Daily-log  : journal/2026-11-27/daily-log.md
Ngày mai   : CHỜ OWNER — GĐ2 DoD đạt, DỪNG nhắc owner GĐ3
Trạng thái : DAY PASS — #171 merged, GĐ2 hoàn tất, CI xanh thật, 0 regression
================================
