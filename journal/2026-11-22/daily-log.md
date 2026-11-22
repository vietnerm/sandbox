# Daily Log — 2026-11-22

## Phiên 17 — Orchestrator (ZAI, §11)

### Pha A — Sync + SBG plan
- Pull main `fc2e048` (ff 34 commit từ trạng thái đầu — nhánh chết local
  `chore/ledger-2026-11-19-p3` xác định DELETE_MERGED); remote chỉ `origin/main`.
- SBG MODE=plan → `branch/plan.md` (2026-11-22): 0 PR mở đầu chu trình → vào thẳng Pha D.

### Triage + chọn issue
- 0 issue mới trong 24h chưa label; 15–16 issue mở đều đã label.
- Chọn theo §8 ưu tiên 1: **#113** — `sec(minict): S3 bind :ro chỉ remount top
  mount, submount của recursive bind vẫn rw` (type:security, priority:p1,
  area:minictd) — residual thấp nhất còn mở của GĐ2 mục 1 (GAPS.md S3).
  Assign + in-progress; repro pre-fix đo được ngay trong phiên (xuống dưới),
  label `needs-repro` gỡ khi merge.

### Research (`research.md` — 4 nguồn, links verified 200/302)
- mount_setattr(2) (man7): một syscall `AT_RECURSIVE|MOUNT_ATTR_RDONLY` từ 5.12
  — kernel đi cả tree dưới VFS lock, không TOCTOU.
- util-linux #2247 + mount(8)/LWN 281157: `mount --rbind` rồi remount ro chỉ
  áp top mount — submount giữ rw; chính là lỗ S3, xác nhận hành vi kernel.
- mount_namespaces(7): rbind clone các mount dưới src; quyền remount per-mount
  trong userns con (mount clone thuộc userns đó).
- Kết luận: hai nhánh theo kernel (≥5.12 mount_setattr; <5.12 per-submount
  remount từ enumeration pre-unshare) + fail-closed khi không enum được.

### Development (`sec/113-s3-ro-bind-recursive`, D52 — `decisions/s3-ro-bind-recursive.md`)
- Repro pre-fix đo được (dev 5.10.134, uid 1001, minict thật): top EROFS nhưng
  `touch /data/sub/inner.txt` (tmpfs submount) THÀNH CÔNG — lỗ ghi thật.
- `sandbox/minict.c`: `mount_setattr(AT_FDCWD, dst, AT_RECURSIVE|AT_SYMLINK_NOFOLLOW,
  MOUNT_ATTR_RDONLY)` cho ≥5.12; ENOSYS → per-submount bind-remount từ
  `/proc/self/mountinfo` (pre-unshare — cửa sổ /proc duy nhất, D39), đếm
  src-relative, cap 16 submount → exit(125), mountinfo không đọc được (trên
  kernel không có mount_setattr) → die — fail-closed, không boot ro-một-phần.
- Tests: BIND1–BIND3 trong `tests/test_isolation.sh` (31→34) — plain tree,
  tree WITH submount (cùng assertion cả hai kernel, mechanism ghi trong tên
  case), rw control chống-vacuous.
- Không đụng seccomp/blocklist → không cần đồng bộ `minict-exec.c` (D25).
- Docs: GAPS.md S3 → FIXED; roadmap GĐ2 mục 1; journal ngày (research, D52,
  evidence, cve, branch/plan).
- Perf: cold 2.06 ms / rss 1244 kB / disk 2.05 MB (perf-check N=20) — PASS,
  lệch trong nhiễu đo so với baseline Phiên 16 (2.16/1212/2.04); gate không đổi.

### CVE Check
- Kernel: `5.10.134-013.15.kangaroo.al8.x86_64`
- `cve/s3-ro-bind-recursive.{md,json}`: cve-check rc=0 (6 rows watchlist) +
  cve-gate rc=0 → 0 CVE mới ngoài watchlist. **Caveat (AUDIT §F)**: script
  là fixed-list — không chứng minh "0 CVE mới tuyệt đối".
- Watchlist không thêm dòng (không component mới ship).

### Validation "không bị nhiễm"
- `make test`: 10 suites, failed: none (isolation 34/34 gồm BIND1–3,
  seccomp 21, netns 7, alpine 9, debian 8+1SKIP, escape-alpine 34,
  cve-check 4, pasta 5+6SKIP env, stack 8, cg 0+6SKIP env-gated);
  `make test-web` 9/9; `make test-minictd` 34/34 + 1 SKIP (MD33 opt-in).
- Cross-kernel: CI ubuntu-24.04 (kernel ≥ 6.8) chạy BIND1–3 qua nhánh
  **mount_setattr** — PASS 34/34 ×2 (run #45000674142 + push run) → cả hai
  cơ chế đều được kiểm chứng end-to-end.
- Mutant (negative control): M-A→BIND2 đỏ; M-B→BIND1+2 đỏ; M-C (mountinfo
  poisoned fail-closed)→BIND1+2 đỏ; BIND3 xanh xuyên suốt → 0 vacuous;
  M0 collapse 8 suites rc=2. Residual đúng D52: mutant nhánh mount_setattr
  (drop AT_RECURSIVE) không đo được trên dev 5.10 — env-blocked minh bạch.
- Host baseline: 3/3 sha256 khớp chính xác Phiên 14/15/16; listeners nền đã
  biết (81/12600/12700/19001/19005/19006); 0 process lạ; instances/ rỗng.
- Bằng chứng: `evidence/s3-ro-bind-recursive.txt`.

### Pha B — PR #114 (sec/* đầu hàng theo thứ tự §11)
- B1: nhánh con trực tiếp của main `fc2e048` → `merge origin/main` up-to-date.
- B2 — SCR clone trắng @ `e0dd1af`: **PASS** — suites ×2 rc=0 byte-stable
  (10 suites / web 9/9 / minictd 34/34+1SKIP); mutants 4/4 + M0 collapse;
  bench N=50: cold p50 2.1 / p95 2.7, rss 1244, disk ~2.05; host unchanged;
  CVE rc=0. Comment trên PR #114.
- B3 — SIA @ `e0dd1af`: **CONCERNS** — 10/11 OK; B10 VIOLATION (daily-log
  thiếu trong PR — precedent #106); claims 0 sai; test rỗng 0; bảo mật
  2 low (truncate im lặng suffix >255 byte — hướng fail-closed; cửa sổ
  enum→remount ngoài threat model). Label `audit:concerns`. Comment trên PR.
- B4 — SDDA sửa: (1) **[medium] commit này thêm `daily-log.md` vào nhánh**
  (đúng precedent #106); (2) [low] mở issue `from-audit` cho truncate >255;
  (3) [low] gỡ `needs-repro` khỏi #113 khi merge. Push → CI → SCR/SIA delta.

### Kế hoạch ngày mai
- Sau merge #114: S4 (unix-socket/TLS) hoặc S5 (spec rlimits exec) theo GAPS
  thứ tự; #109 chờ quyết định maintainer (không tự đổi posture); issue
  from-audit (truncate >255) nếu còn dư phiên.
- GĐ2 tiếp: mục 2 (template), mục 3 (storage overlayfs + snapshot), mục 6
  (Landlock), mục 7 (đo control-plane RSS); A2 residuals đã đóng.
- **GD3 (Web UI + console) CHƯA chạm** — per chỉ thị owner: mọi mục GĐ2 hoàn
  thành + GD3 sẵn sàng mở code → DỪNG + nhắc owner TRƯỚC dòng code UI đầu
  tiên. Trạng thái GĐ2: mục 1 còn S4–S6 + systemd-real + residual #109;
  mục 2–7 chưa mở.

### Pha C — Gộp (SBG MODE=apply)
- CI `957791d` xanh (build-test ×2 + perf + scan) → flip ready (GraphQL
  `markPullRequestReadyForReview` — REST PATCH không flip draft, đo được Phiên 16)
  → **squash `67c55ca`** REST PUT match-head-commit `957791d` → **#113 auto-close**
  08:21:02Z (precedent #104) + gỡ `needs-repro` khỏi #113 (SIA việc low #3 —
  repro pre-fix đã đo được và commit vào evidence).
- Kiểm sau merge: `git diff main 957791d` rỗng (squash tree khớp đúng head);
  nhánh xoá local + remote (DELETE_MERGED, nội dung trọn trong main — không cần
  tag lưu trữ); CI main `67c55ca` chạy (kết quả ghi ở Pha E).

### Pha E — Kết ngày (SBG)
- SBG MODE=apply: `sec/113-s3-ro-bind-recursive` DELETE_MERGED; residue local
  `chore/ledger-2026-11-19-p3` DELETE_MERGED (đã xoá đầu phiên theo plan.md);
  ledger qua PR này; remote sau Phiên 17: **chỉ `main`** @ `67c55ca`.
- Roadmap: GĐ2 mục 1 S3 → FIXED (GAPS.md); "Đã xong" thêm Phiên 17; GĐ2 mục 1
  còn mở: S4–S6, systemd-real, residual #109 (chờ maintainer); mục 2–7 chưa mở.
- Lưu ý minh bạch: trong phiên phát hiện commit chồng thời điểm `a9caee3`
  (fix(review) hardening mountinfo, author Z @ 08:12:01Z — phiên chạy song song
  trên cùng workspace). Xử lý: review từ đầu trong SIA delta @ `957791d`, SCR
  chạy lại ĐẦY ĐỦ trên SHA mới (code đổi — verdict không mang sang), mutants
  áp patch sạch 4/4. Không để nhánh dở Merge; mọi thay đổi qua PR.

DAY PASS — 2026-11-22 Phiên 17: issue #113 done (merge #114 `67c55ca` @
`957791d`; SCR PASS ×2 SHA — suites rc=0 ×2 byte-stable, web 9/9, minictd
34/34+1SKIP ×2, mutants 4/4 + M0 collapse ×2 SHA, bench N=50 2.1/1244/~2.05;
SIA CONCERNS → fix 2 commits → PASS delta 11/11; isolation 31→34; CVE gate
rc=0; host unchanged 3/3; main xanh `67c55ca`; remote chỉ `main`).

=== SBG REPORT — 2026-11-22 (apply) ===
Đã gộp     : 2  — #114 (sec #113, squash `67c55ca`), ledger PR này
Đã xoá     : 2  (đã gộp từ trước: `chore/ledger-2026-11-19-p3` residue local;
                  sau merge: `sec/113-s3-ro-bind-recursive`)
Giữ lại    : 0
Còn lại    : main
CI main    : xanh (67c55ca — build-test + scan + perf-guard, đo được qua API)
Dừng?      : không
=============================================
