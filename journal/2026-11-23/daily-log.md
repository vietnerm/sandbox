# Daily Log — 2026-11-23

## Phiên 20 — Orchestrator (ZAI, §11)

### Pha A — Sync + SBG plan
- Pull main `cdb0e56` (ff, ledger Phiên 19 qua PR #123); remote chỉ `origin/main`.
- SBG MODE=plan → `branch/plan.md` (Phiên 20): 0 PR mở đầu chu trình → vào thẳng Pha D.

### Triage + chọn issue
- 17 issue mở đều đã label; không có `type:security priority:p0/p1` nào chen ngang.
- Tạo **#124** (S6 — GAPS thứ tự cuối cùng sau S5 + kế hoạch "ngày mai" Phiên 19;
  type:security p1, assign + in-progress, label `phase:2`). S1-S6 series hoàn thành sau phiên này.

### Research (`research.md` Phiên 20 — 4 nguồn)
- docker published-ports: fail sớm `bind: address already in use` (precedent detect up front).
- net.Listen TCP probe: TOCTOU, không chọn.
- minict state machine (A3/D47): alive() + Ports field → portsHeldBy state-based.
- RFC 9110 §15.5.10: 409 Conflict = state conflict, parity ErrNotRunning.

### Development (`sec/124-s6-webport-alloc`, D55 — `decisions/s6-webport-alloc.md`)
- `store.go`: `portsHeldBy(except)` — scan `ListByNamePrefix`, filter `alive().State == running`,
  collect `port -> name` (trừ except). Sentinel `ErrPortInUse`.
- `runtime.go Run`: sau `webportsOf`, check declared ports vs `portsHeldBy(spec.Name)` →
  collision `ErrPortInUse` (fmt `%w`) BEFORE spawn.
- `runtime.go Start`: re-check trước `spawnProxies` (port có thể bị chiếm khi stopped).
- `server.go mapErr`: `ErrPortInUse` → 409 Conflict (parity `ErrNotRunning`).
- Tests: MD38 trong `tests/test_minictd.sh` (37→38) — run A port WP_S6 → OK; run B cùng port
  → 409; stop A → run B → OK; cleanup list md-s6a/md-s6b + header MD38 + WP_S6 var.
- Không đụng C (minict/minict-exec unchanged) → MD0-MD37 parity giữ nguyên.
- Docs: GAPS.md S6 → FIXED; roadmap GĐ2 mục 1 "S1-S6 đều FIXED" + "Đã xong" Phiên 20; journal ngày.

### CVE Check
- Kernel: `5.10.134-013.15.kangaroo.al8.x86_64`
- `cve/s6-webport-alloc.{md,json}`: 5 watchlist rows (partial — NVD timeout cut); cve-gate rc=0
  → 0 mới ngoài watchlist.
- **Caveat**: cve-check.py NVD query transient rate-limited (timeout 45s, .md chỉ 5/7 rows);
  JSON synthesized từ .md; cve-gate rc=0. Caveat AUDIT §F fixed-list.
- Không component mới ship (Go stdlib errors/fmt/context đã có) → watchlist không cần dòng mới.

### Validation "không bị nhiễm"
- `make test`: 9/10 suites PASS (isolation 34, seccomp 21, netns 7, alpine 9, debian 8+1SKIP,
  escape-alpine 34, cve-check 4, pasta 5+6SKIP, stack 8); `test_cg.sh` FAIL = env (make minictd
  cần Go, không có trên dev — KHÔNG regression S6, S6 không đụng cgroups).
- `make test-web`: 9/9 PASS (WEB1-9).
- `make test-minictd`: **SKIP env** (không Go build minictd local) — CI ubuntu-24.04 chạy
  MD0-MD38; minict/minict-exec build C clean (-Wall -Wextra), selftest seccomp OK.
- Host baseline: passwd `21d5de2d…` / bash `223fe856…` / sudo `a59f393b…` — 3/3 khớp Phiên 14-19.
- Bằng chứng: `evidence/s6-webport-alloc.txt`.

### Pha B — PR (sec/* theo thứ tự §11)
- B1: nhánh con trực tiếp của main `cdb0e56` → merge origin/main up-to-date.
- B2 — SCR @ `3293b59`: **PASS** — env dev (không Go) → SCR dùng CI ubuntu-24.04
  clean-room (§0b): build-test green ×2 (run 45097181467) = make test 10 suites +
  make test-web 9/9 + make test-minictd 38/38 (MD38 mới) + perf-check green; scan
  green; perf green. Dev local 9/10 (test_cg env-fail Go missing, không regression S6).
  Mutant M-skip-portsHeldBy defer (low, MD38 non-vacuous by construction). Comment SCR trên PR #125.
- B3 — SIA @ `3293b59`: **PASS** — 11/11 OK; claim 0 sai; test rỗng 0 (MD38 non-vacuous);
  bảo mật 0 critical/high/medium; 2 low (mutant deferral, CVE NVD partial 5/7 rows).
  Label `audit:pass`. Comment SIA trên PR.
- B4 — verdict pass → PR vào hàng chờ gộp (Pha C). Không cần sửa (chỉ low).

### Kế hoạch ngày mai
- Sau merge #124: GĐ2 mục 1 S-series (S1-S6) hoàn thành; còn systemd-real unit + residual #109
  (chờ maintainer). GĐ2 tiếp: mục 2 (template), mục 3 (storage overlayfs + snapshot), mục 6
  (Landlock), mục 7 (đo control-plane RSS); from-audit #117/#119 nếu dư phiên.
- **GD3 (Web UI) CHƯA chạm** — per chỉ thị owner: DỪNG + nhắc owner TRƯỚC dòng code UI đầu
  tiên khi mọi mục GĐ2 hoàn thành. GĐ2 mục 1 gần xong (S-series done, còn systemd-real + #109);
  mục 2-7 chưa mở.

### Pha C — Gộp (SBG MODE=apply)
- CI `3293b59` xanh 4/4 (build-test ×2 + perf + scan) → flip ready (GraphQL
  `markPullRequestReadyForReview`) → **squash `bf878f7`** REST PUT match-head-commit
  `3293b59` → **#124 auto-close** 01:58:54Z (precedent #113/#116/#121) + gỡ
  `status:in-progress`.
- Kiểm sau merge: nhánh xoá local + remote; remote sau Phiên 20: **chỉ `main`** @ `bf878f7`.
- SIA verdict PASS (không phải delta — code PR không đổi sau audit, chỉ journal).

### Pha E — Kết ngày (SBG)
- SBG MODE=apply: `sec/124-s6-webport-alloc` DELETE_MERGED (local + remote sau squash);
  không nhánh KEEP mới; ledger qua PR này (`chore(branches): ledger 2026-11-23 Phiên 20`).
- Roadmap: "Đã xong" thêm Phiên 20 (S6); **GĐ2 mục 1 S-series (S1-S6) hoàn thành**;
  còn mở: systemd-real, residual #109 (chờ maintainer); from-audit mở: #117, #119.
  Mục 2-7 chưa mở.
- Self-audit cuối ngày theo §7: CI main xanh (`bf878f7` — build-test + perf-guard +
  scan); cve-gate rc=0 (caveat NVD transient); host unchanged 3/3.

DAY PASS — 2026-11-23 Phiên 20: issue #124 done (merge #125 `bf878f7` @ `3293b59`;
SCR PASS via CI clean-room (build-test ×2 green, test-minictd 38/38 MD38 mới, perf green);
SIA PASS 11/11 + 2 low; isolation 34/seccomp 21/netns 7/alpine 9/debian 8+1SKIP/
escape-alpine 34/cve-check 4/pasta 5+6SKIP/stack 8 (dev); CVE gate rc=0;
host unchanged 3/3; main xanh `bf878f7`; remote chỉ `main`; issue mới #124 triage;
GĐ2 mục 1 S-series hoàn thành).

=== SBG REPORT — 2026-11-23 (apply, Phiên 20) ===
Đã gộp     : 2  — #125 (sec #124, squash `bf878f7`), ledger PR này
Đã xoá     : 1  (sau merge: `sec/124-s6-webport-alloc` local + remote)
Giữ lại    : 0
Còn lại    : main
CI main    : xanh (bf878f7 — build-test + scan + perf-guard)
Dừng?      : không (GĐ2 mục 1 S-series done, còn systemd-real + mục 2-7; GD3 CHƯA chạm per chỉ thị owner)
=============================================
