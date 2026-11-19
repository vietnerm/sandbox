# Daily Log — 2026-11-19

## Phiên 1 — Orchestrator §11 (ZAI): issue #5 — deep escape PoC (D33)

> Chu trình: pull (main 187bfd2, chỉ mode-change rác — restore) → Pha A SBG
> plan (0 PR mở, repo chỉ main) → Pha B/C rỗng → Pha D issue #5 → PR #47
> (draft, PAT không flip được) → PR #48 non-draft → B→C, merge `70921c2` →
> Pha E SBG apply + ledger PR.

### Issue xử lý
- **#5** — sec(escape): deep escape PoC — fd-based chroot escape, core_dump
  leak, setuid binary vs NNP — status: done (đã merge; issue mở lại theo §6
  chờ owner `verified`).
- Triage khác: #44 gỡ `needs-repro` (repro đã xác nhận live Phiên 4) +
  `status:blocked`; #34 giữ mở theo §6 (đã triage Phiên 2).

### Research (5 nguồn — `research.md`)
- CVE-2018-15664 (TOCTOU qua fd); close_range(2) man7 (kernel 5.9, syscall
  436); PR_SET_NO_NEW_PRIVS man7 + kernel.org; core(5) man7 (WebFetch trực
  tiếp: "RLIMIT_CORE will be ignored if the system is configured to pipe core
  dumps to a program"); Elastic rule core_pattern container escape.

### Development (`sec/5-deep-escape`, squash `70921c2`, D33)
- Phát hiện trước vá: minict.c **0** `close_range/FD_CLOEXEC` (payload kế thừa
  mọi fd không-CLOEXEC của caller → bypass pivot_root) và **0** `RLIMIT_CORE`
  (crash → core file trong rootfs; máy dev `core_pattern` = `core` file).
- `harden_payload_fds()` (minict.c payload, trước capdrop/seccomp):
  `setrlimit(RLIMIT_CORE,{0,0})` die-on-fail + `close_range(3,~0U,0)` +
  fallback loop trần 65536. `harden_exec_fds()` (minict-exec child) — cùng
  lớp, **filter seccomp không đổi** (MD0 PASS trên CI).
- Test mới: isolation 23→**29** — FD1 (2 giai đoạn: control kế thừa qua
  /proc/<minict>/fd/9 → không vacuous; payload `cat <&9` phải EBADF + không
  lộ marker), PROC2, CHROOT1 (expect=fail), SETUID2 (exec busybox 4755,
  host /proc NoNewPrivs=1 + CapEff=0 kiểu D32), CORE1 (harness tự nâng limit
  trước để không kế thừa 0 — vacuous-guard), CORE2; seccomp 20→**21** — SC20
  (blocklist hit sống sót rc=1 EPERM, không SIGSYS death).
- Negative control clean-room 6/6 bắt đúng: fd→FD1+CORE1; **core→CORE1 duy
  nhất với FD1 PASS** (chứng minh mỗi test đo đúng lớp); capdrop→CAPD1+
  CHROOT1+SETUID2; nnp→collapse (seccomp tự die); trap→SC20; M0→collapse.
- Sự cố + bài học: (1) mutant script `git checkout` cả file chưa commit →
  mất WIP minict.c + 2 suite giữa phiên (khôi phục bằng vá lại; NC1 lần đó
  vẫn hợp lệ) — bài học vào `docs/skills.md`; (2) mutant binary đặt tên
  `minict_fd` làm comm-matching đỏ giả toàn suite (quirk #38) — rebuild tên
  `minict` per-dir; (3) SC20 viết bằng `ic` không tồn tại trong seccomp suite
  → FAIL giả lần chạy đầu — đổi sang helper riêng; (4) tưởng seccomp kết thúc
  SC19, thực tế có **SC0** — lesson đếm id bằng grep/sort -V/uniq -c vào
  `docs/skills.md`; (5) **PR #47 draft stuck**: PAT fine-grained không flip
  được draft (PATCH 200 no-op + GraphQL FORBIDDEN → merge 405 "still a
  draft") — đóng có lý do + tạo lại non-draft cùng head SHA, SCR/SIA hợp lệ
  (evidence gắn SHA không gắn PR).

### CVE Check
- Kernel `5.10.134-013.15.kangaroo.al8.x86_64` (máy dev == máy SCR).
- `cve/deep-escape.md|.json` — rc=0, 19 hàng watchlist, **0 CVE mới ngoài
  watchlist**; critical đã biết đều host-side (WEB2 loopback đo được — Phiên 2).
- NVD live OK lần này (không tái hiện crash #44 — issue vẫn hợp lệ cho lỗi
  transient, giữ `status:blocked`).

### Validation "không bị nhiễm"
- `make test` ×3 clean-room + ×1 máy dev: isolation 29/29 + seccomp 21/21 +
  netns 7/7 + alpine 8/8 (4 suites failed: none); `make test-web` 9/9 ×3.
- `make test-minictd`: máy dev không Go (env-blocked) — nguồn CI job
  135286325326 @ đúng SHA 32dd9aa: **PASS=20 FAIL=0**, MD0 PASS.
- Perf (make perf-check, N=20 máy dev + bench N=50 SCR): cold_start 1.86 ms
  (SCR p50 1.9 / p95 2.5), rss 1160 kB, disk 2.04 MB — **PASS** mọi ngưỡng.
- Host baseline: /etc/passwd, /bin/bash, /usr/bin/sudo sha256 **khớp chính
  xác** 2026-11-18 → UNCHANGED; listeners cùng tập nền (6 dòng).
- Bằng chứng: `evidence/deep-escape.txt`.

### PR
- #47 — draft (PAT-limited) — closed có lý do.
- #48 — ready, CI xanh (scan/build-test×2/perf @ 32dd9aa) — **merged squash
  `70921c2`**, match-head-commit; CI main xanh sau merge.

### Kế hoạch ngày mai
- Owner review #5 (gắn `verified` → đóng) và #41 (từ Phiên 3, cùng trạng
  thái `status:needs-review`).
- GĐ1 mục thấp nhất kế tiếp: mục 2 còn phần "chạy lại cả suite trên rootfs
  distro thật" (Alpine đã có — extension: chạy isolation suite với
  MINICT_ROOTFS=alpine nếu làm được, hoặc mục 3 pasta/slirp4netns connectivity).
- Ứng viên từ-audit: issue CHROOT1 assert-started (low), PROC2/CORE2
  follow-up (low), cve-daily chỉ mở issue khi CVE ngoài watchlist (chore CI,
  kế hoạch từ Phiên 2/3), #44 cve-check.py KeyError (p2).
