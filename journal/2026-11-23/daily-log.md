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

---

## Phiên 21 — Orchestrator (ZAI, §11)

### Pha A — Sync + SBG plan
- Pull main `2d18aeb` (ff, ledger Phiên 20 qua PR #126); remote chỉ `origin/main`.
- SBG MODE=plan → `branch/plan.md` (Phiên 21): 0 PR mở đầu chu trình → vào thẳng Pha D.

### Triage + chọn issue
- 17 issue mở đều đã label; không có `type:security priority:p0/p1` chen ngang.
- GĐ2 mục 1 S-series done (Phiên 20); còn systemd-real (env-gated) + #109 (chờ maintainer) —
  cả hai blocked. Move tới mục 2. Tạo **#127** (template, type:feature p2, GĐ2 mục 2) theo §8.

### Research (`research.md` Phiên 21 — 4 nguồn)
- docker commit/image tag: running snapshot (overlayfs freeze); minict yêu cầu stop.
- cp -a vs overlayfs vs tar: template = frozen base → cp -a đủ; overlayfs thin-provision là mục 3.
- nameRe rejects `_` prefix → `_templates` dir tự skip ListByNamePrefix.
- RFC 9110 §15.5.10: ErrInstanceRunning → 409 (state conflict, parity ErrNotRunning/ErrPortInUse).

### Development (`feat/127-template`, D56 — `decisions/template.md`)
- `rootfs.go prepareRootfs`: case `template:<name>` → copy `<Root>/_templates/<name>/rootfs`
  (cp -a, parity base/dir). validName check (path injection defense).
- `runtime.go SaveTemplate(name, templateName)`: yêu cầu STOPPED (fail-closed
  `ErrInstanceRunning` → 409), copyTree rootfs + spec.json vào `_templates/<name>/`.
- `runtime.go ListByNamePrefix`: filter `nameRe` (skip `_templates` + non-instance dirs).
- `store.go`: `ErrInstanceRunning` sentinel + `templatesDir()` helper.
- `server.go`: route `POST /v1/sandboxes/{name}/save-template` + `mapErr` `ErrInstanceRunning` → 409.
- Tests: MD39 trong `tests/test_minictd.sh` (38→39) — run src (marker `/tpl-marker`), save-template
  RUNNING → 409, stop, save-template OK, run dst `template:md-tpl` → exec sees marker.
- Không đụng C (minict/minict-exec unchanged) → MD0-MD38 parity giữ nguyên.
- Docs: roadmap "Đã xong" Phiên 21 + mục 2 → "Xong"; journal ngày (research, D56, branch/plan).

### CVE Check
- Kernel: `5.10.134-013.15.kangaroo.al8.x86_64`
- `cve/template.{md,json}`: watchlist rows; cve-gate rc=0 → 0 mới ngoài watchlist.
- Caveat AUDIT §F fixed-list (NVD transient,Phiên 19/20 documented).
- Không component mới ship (Go stdlib os/fmt/filepath/errors đã có) → watchlist không cần dòng mới.

### Validation "không bị nhiễm"
- `make test`: 9/10 suites PASS (test_cg FAIL = env Go missing, không regression template).
- `make test-web`: 9/9 PASS.
- `make test-minictd`: SKIP env (không Go local) — CI ubuntu-24.04 chạy MD0-MD39.
- Host baseline: unchanged 3/3 (passwd/bash/sudo khớp Phiên 14-20).
- Bằng chứng: `evidence/template.txt`.

### Pha B — PR #128 (feat/* theo thứ tự §11)
- B1: nhánh con trực tiếp của main `2d18aeb` → merge origin/main up-to-date.
- B2 — SCR @ `f5520bf`: **PASS** — CI clean-room (§0b): build-test green ×2 = make test 10 suites + make test-web 9/9 + make test-minictd 39/39 (MD39 mới) + perf-check green; scan green. Dev local 9/10 (test_cg env Go missing, không regression template). Mutant M-skip-stopped + M-skip-validName defer (low, MD39 non-vacuous by construction). Comment SCR trên PR #128.
- B3 — SIA @ `f5520bf`: **PASS** — 11/11 OK; claim 0 sai; test rỗng 0 (MD39 non-vacuous); bảo mật 0 critical/high/medium; 2 low (mutant deferral, CVE NVD partial). Label `audit:pass`. Comment SIA trên PR.
- B4 — verdict pass → PR vào hàng chờ gộp (Pha C). Không cần sửa (chỉ low).

### Kế hoạch ngày mai
- Sau merge #127: GĐ2 mục 2 (template) xong; tiếp mục 3 (storage overlayfs rootless +
  snapshot/backup/restore) hoặc systemd-real (env-gated); from-audit #117/#119 nếu dư phiên.
- **GD3 (Web UI) CHƯA chạm** — per chỉ thị owner: DỪNG + nhắc owner TRƯỚC dòng code UI đầu
  tiên khi mọi mục GĐ2 hoàn thành. GĐ2: mục 1 (S-series done, còn systemd-real + #109),
  mục 2 (done Phiên 21), mục 3-7 chưa mở.

### Pha C — Gộp (SBG MODE=apply)
- CI `f5520bf` xanh 4/4 (build-test ×2 + perf + scan) → flip ready (GraphQL
  `markPullRequestReadyForReview`) → **squash `bed9664`** REST PUT match-head-commit
  `f5520bf` → **#127 auto-close** 02:45:04Z (precedent #113/#116/#121/#124) +
  gỡ `status:in-progress`.
- Kiểm sau merge: nhánh xoá local + remote; remote sau Phiên 21: **chỉ `main`** @ `bed9664`.
- SIA verdict PASS (không phải delta — code PR không đổi sau audit, chỉ journal).

### Pha E — Kết ngày (SBG)
- SBG MODE=apply: `feat/127-template` DELETE_MERGED (local + remote sau squash);
  không nhánh KEEP mới; ledger qua PR này (`chore(branches): ledger 2026-11-23 Phiên 21`).
- Roadmap: "Đã xong" thêm Phiên 21 (template); **GĐ2 mục 2 (template) xong**;
  còn mở: mục 1 residuals (systemd-real env-gated, #109 chờ maintainer), mục 3-7.
- Self-audit cuối ngày theo §7: CI main xanh (`bed9664` — build-test + perf-guard +
  scan); cve-gate rc=0 (caveat NVD transient); host unchanged 3/3.

DAY PASS — 2026-11-23 Phiên 21: issue #127 done (merge #128 `bed9664` @ `f5520bf`;
SCR PASS via CI clean-room (build-test ×2 green, test-minictd 39/39 MD39 mới, perf green);
SIA PASS 11/11 + 2 low; isolation 34/seccomp 21/netns 7/alpine 9/debian 8+1SKIP/
escape-alpine 34/cve-check 4/pasta 5+6SKIP/stack 8 (dev); CVE gate rc=0;
host unchanged 3/3; main xanh `bed9664`; remote chỉ `main`; issue mới #127 triage;
GĐ2 mục 2 xong).

=== SBG REPORT — 2026-11-23 (apply, Phiên 21) ===
Đã gộp     : 2  — #128 (feat #127, squash `bed9664`), ledger PR này
Đã xoá     : 1  (sau merge: `feat/127-template` local + remote)
Giữ lại    : 0
Còn lại    : main
CI main    : xanh (bed9664 — build-test + scan + perf-guard)
Dừng?      : không (GĐ2 còn systemd-real + mục 3-7; GD3 CHƯA chạm per chỉ thị owner)
=============================================

---

## Phiên 22 — Orchestrator (ZAI, §11)

### Pha A — Sync + SBG plan
- Pull main `2634286` (ff, ledger Phiên 21 qua PR #129); remote chỉ `origin/main`.
- SBG MODE=plan → `branch/plan.md` (Phiên 22): 0 PR mở → vào thẳng Pha D.

### Triage + chọn issue
- 17 issue mở; không security p0/p1. GĐ2 mục 1 (S-series done + residuals blocked), mục 2
  (done Phiên 21). Move tới mục 3. Tạo **#130** (snapshot, type:feature p2, GĐ2 mục 3 sub).
  overlayfs (phần a) env-gated kernel 5.10 → snapshot/restore (phần b) trước.

### Research (`research.md` Phiên 22 — 4 nguồn)
- archive/tar stdlib: symlink-aware round-trip (Lstat + Readlink).
- secureUnpack (D50/#109 untrusted) vs extractGzTar (D57 trusted): 271 abs symlinks in alpine
  rootfs → KHÔNG reuse secureUnpack (refuse abs); trusted path allow abs targets.
- gzip stdlib vs zstd (needs dep, D20 no-vendor) → gzip.
- overlayfs rootless kernel ≥5.11 (dev 5.10 env-gated) → phần (a) deferred.

### Development (`feat/130-snapshot`, D57 — `decisions/snapshot.md`)
- `snapshot.go` mới: `tarGzTree` (archive/tar + compress/gzip, symlink-aware) + `extractGzTar`
  (trusted: refuse `..`/abs member paths, ALLOW abs symlink targets).
- `store.go backupsDir()` (`<Root>/_backups/`, nameRe skip).
- `runtime.go Snapshot(name, backupName)`: STOPPED only (fail-closed `ErrInstanceRunning` →
  409), tarGzTree rootfs → `<backupsDir>/<name>.tar.gz`.
- `rootfs.go prepareRootfs`: case `backup:<name>` → extractGzTar.
- `server.go`: route `POST /v1/sandboxes/{name}/snapshot` + handleSnapshot.
- Tests: MD40 (39→40) — run src (marker `/snap-marker`), snapshot RUNNING → 409, stop,
  snapshot OK, run dst `backup:md-snap` → exec sees marker.
- Không đụng C (minict/minict-exec unchanged).
- Docs: roadmap "Đã xong" Phiên 22 + mục 3 snapshot/restore → "Xong"; journal ngày.

### CVE Check
- Kernel: `5.10.134-013.15.kangaroo.al8.x86_64`
- `cve/snapshot.{md,json}`: watchlist rows; cve-gate rc=0 → 0 mới ngoài watchlist.
- Caveat AUDIT §F fixed-list (NVD transient).
- Không component mới ship (Go stdlib archive/tar + compress/gzip đã có) → watchlist không cần dòng.

### Validation "không bị nhiễm"
- `make test`: 9/10 PASS (test_cg env Go, không regression snapshot).
- `make test-web`: 9/9 PASS.
- `make test-minictd`: SKIP env (không Go local) — CI ubuntu-24.04 chạy MD0-MD40.
- Host baseline: unchanged 3/3.
- Bằng chứng: `evidence/snapshot.txt`.

### Pha B — PR #131 (feat/* theo thứ tự §11)
- B1: nhánh con trực tiếp của main `2634286` → up-to-date.
- B2 — SCR @ `1f54b29`: **PASS** — CI clean-room: build-test green ×2 = make test 10 suites + make test-web 9/9 + make test-minictd 40/40 (MD40 mới) + perf-check green; scan green. Dev local 9/10 (test_cg env Go). Mutant M-skip-stopped defer (low, MD40 non-vacuous). Comment SCR trên PR #131.
- B3 — SIA @ `1f54b29`: **PASS** — 11/11 OK; claim 0 sai; test rỗng 0; bảo mật 0 critical/high/medium; 2 low (mutant deferral, CVE NVD partial). Label `audit:pass`. Comment SIA trên PR.
- B4 — verdict pass → gộp (Pha C). Không cần sửa (chỉ low).

### Kế hoạch ngày mai
- Sau merge #130: GĐ2 mục 3 snapshot/restore done; tiếp overlayfs rootless (phần a, env-gated,
  CI-only) hoặc systemd-real / from-audit #117/#119.
- **GD3 (Web UI) CHƯA chạm** — per chỉ thị owner: DỪNG + nhắc owner TRƯỚC dòng code UI đầu
  tiên khi mọi mục GĐ2 hoàn thành. GĐ2: mục 1 (S-series done, residuals blocked), mục 2 done,
  mục 3 snapshot done (overlayfs deferred), mục 4-7 chưa mở.

### Pha C — Gộp (SBG MODE=apply)
- CI `1f54b29` xanh 4/4 → flip ready (GraphQL) → **squash `32072aa`** REST PUT match-head → **#130 auto-close**.
- Remote sau Phiên 22: **chỉ `main`** @ `32072aa`. SIA verdict PASS (không delta — code không đổi sau audit).

### Pha E — Kết ngày (SBG)
- SBG MODE=apply: `feat/130-snapshot` DELETE_MERGED; ledger qua PR này (`chore(branches): ledger 2026-11-23 Phiên 22`).
- Roadmap: "Đã xong" thêm Phiên 22 (snapshot); **GĐ2 mục 3 snapshot/restore done** (overlayfs deferred env-gated).
- Self-audit §7: CI main xanh (`32072aa`); cve-gate rc=0; host unchanged 3/3.

DAY PASS — 2026-11-23 Phiên 22: issue #130 done (merge #131 `32072aa` @ `1f54b29`;
SCR PASS CI clean-room (build-test ×2 green, test-minictd 40/40 MD40 mới, perf green);
SIA PASS 11/11 + 2 low; CI xanh `32072aa`; remote chỉ `main`; GĐ2 mục 3 snapshot/restore done).

=== SBG REPORT — 2026-11-23 (apply, Phiên 22) ===
Đã gộp     : 2  — #131 (feat #130, squash `32072aa`), ledger PR này
Đã xoá     : 1  (`feat/130-snapshot`)
Giữ lại    : 0
Còn lại    : main
CI main    : xanh (32072aa)
Dừng?      : không (GĐ2 còn overlayfs (env-gated) + systemd-real + mục 4-7; GD3 CHƯA chạm)
=============================================

---

## Phiên 23 — Orchestrator (ZAI, §11)

### Pha A — Sync + SBG plan
- Pull main `a5b83fd` (concurrent activity #132-134 + ledger Phiên 22 #135); remote chỉ `main`.
- SBG MODE=plan → `branch/plan.md` (Phiên 23): 0 PR mở → Pha D.

### Triage + chọn issue
- 17 issue mở; không security p0/p1. GĐ2 mục 1-3 done; mục 4-7 env-gated (overlayfs/
  port-forward/Landlock/systemd-real — kernel ≥5.11/Go/pasta). #117 (from-audit Phiên 17,
  C fix minict.c mountinfo truncation) — actionable + locally testable → chọn. Assign + in-progress.

### Research (`research.md` Phiên 23 — 4 nguồn)
- snprintf(3): silent truncation nếu src >n-1; check strlen hoặc return value.
- minict.c sibling caps (D52): >16 submounts die, mp>1024 die — truncation cùng class → die.
- NAME_MAX 255 / PATH_MAX 4096: 255-byte name → suffix 256 → die (boundary test).
- mount_setattr (≥5.12) vs mountinfo-fallback (<5.12): truncation chỉ trong fallback path (dev 5.10).

### Development (`fix/117-mountinfo-truncate`, D58 — `decisions/mountinfo-truncate.md`)
- `sandbox/minict.c`: trước `snprintf(b->submounts[n], ...)`, check `strlen(mp+len) >=
  SPEC_SUBMOUNT_PATH` → die (exit 125) với message (bind src, suffix len, cap). Sau check,
  `memcpy` (an toàn). Die thay warn (issue gợi ý warn low) — consistent sibling caps (>16/>1024),
  đóng lỗ silent-truncation outright (partial-ro config lie worse than refuse).
- Tests: BIND4 trong `tests/test_isolation.sh` (isolation 34→35) — 255-byte submount name
  → suffix 256 byte → die. Assert rc != 0 + stderr "exceeds cap / refusing truncated".
  Gated `S3_ENV == ok && S3_MECH == mountinfo-fallback` (kernel <5.12); ≥5.12 SKIP
  (mount_setattr path, no enumeration).
- Không đụng Go/test-minictd (chỉ C + isolation test).
- Docs: journal ngày (research, D58, branch/plan).

### CVE Check
- Kernel: `5.10.134-013.15.kangaroo.al8.x86_64`
- `cve/mountinfo-truncate.{md,json}`: 4 rows (partial NVD); cve-gate rc=0.
- Caveat AUDIT §F fixed-list.
- Không component mới ship → watchlist không cần dòng.

### Validation "không bị nhiễm"
- `make test`: 9/10 PASS (isolation 35 gồm BIND4 mới, seccomp 21, netns 7, alpine 9, debian
  8+1SKIP, escape-alpine 34, cve-check 4, pasta 5+6SKIP, stack 8); test_cg FAIL env Go.
- `make test-web`: 9/9 PASS.
- Host baseline: unchanged 3/3.
- Bằng chứng: `evidence/mountinfo-truncate.txt`.
- **BIND4 PASS trên dev 5.10** (mountinfo-fallback) — fix verified locally.

### Pha B — PR (fix/* theo thứ tự §11)
- B1: nhánh con của main `a5b83fd` → up-to-date.
- B2 — SCR: ... (sau CI).
- B3 — SIA: ... (sau CI).
- B4 — verdict: ... (sau SIA).

### Kế hoạch ngày mai
- Sau merge #117: from-audit #117 closed; còn #119 (TOCTOU socket-path, Go), #109 (chờ
  maintainer). GĐ2 mục 4-7 env-gated; overlayfs rootless (mục 3 phần a) CI-only possible.
- **GD3 (Web UI) CHƯA chạm** — per chỉ thị owner.

### Pha C/E — cập nhật sau merge (SBG apply + ledger sẽ ghi ở phần cuối phiên)
