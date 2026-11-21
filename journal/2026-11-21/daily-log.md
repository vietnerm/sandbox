# Daily Log — 2026-11-21

## Phiên 15 — Orchestrator (ZAI, §11)

### Pha A — Sync + SBG plan
- Pull main `ffa3d3a` (up-to-date); remote chỉ `main`; CI main xanh ×3 (research-log/cve-daily/unicode-clean @ `ffa3d3a`).
- SBG MODE=plan → `branch/plan.md`: local `feat/97-g3-preview-routing` = DELETE_MERGED (PR #98 squash `23cf249`, head `e8ec847` khớp); 0 PR mở → Pha B/C trống.

### Triage + chọn issue
- 0 issue mới từ hôm qua; 14 issue mở đều đã label (p2/p3).
- Chọn theo §8/roadmap: GĐ2 mục 1 residual thấp nhất = **A2 oci: test** → tạo **#103**.
- **Research (#103) phát lộ security finding đo được** → mở **#104** (type:security p1) + pivot issue-hôm-nay sang #104 theo §8 ưu tiên 1. (Lỗi quy trình nhỏ ghi minh bạch: POST issue đầu tạo #104 nhưng stdout bị nuốt khi traceback → tạo trùng #105 → đóng #105 duplicate có comment link.)
- **#103 giữ mở** (e2e crane thật — phiên kế).

### Research (`research.md`)
- Ma trận đo GNU tar 1.35: `..` member refuse rc=2; write-through symlink chặn rc=2; **dotdot-target symlink được tạo rc=0 và đọc xuyên ra ngoài rootfs đo được** (ESCAPED-READ).
- Kết luận: fail-closed của đường `oci:` là **hành vi incidental của host tar** — busybox/GNU cũ không có các protection đo trên ⇒ ghi file host-side = escape ranh giới unpack.
- Nguồn: đo cục bộ + crane `export.go` + release v0.22.1 (links verified qua GitHub API); moby chrootarchive flagged unverified-fetch.

### Development (`sec/104-oci-unpack-traversal`, D50 — `decisions/oci-unpack-traversal.md`)
- `minictd/internal/minict/unpack.go` (mới): `secureUnpack` stdlib-only — openat O_NOFOLLOW per-component (D31 pattern), reject `..`/absolute, link target validate, duplicates refuse (O_EXCL), `stripSpecialBits` (pure), FIFO mknodat S_IFIFO, char/block refuse.
- `rootfs.go`: `unpackOCI` (crane pipe → secureUnpack, no host tar) + `tar:` path cùng extractor.
- Unit matrix 14 test (`unpack_test.go`) + e2e **MD26/MD27** (suite 26→28).
- Measured lessons (ghi D50): `openat(O_CREAT|O_DIRECTORY)`=ENOTDIR trên 5.10; FIFO-open trick ra regular file; dev-sandbox LSM strip suid lúc create + giết SYS_SYMLINK (SYS_SYMLINKAT vẫn chạy) ⇒ test assert provenance message để mutant không bị env che.
- Mutant matrix: M1(4 đỏ)/M2(2)/M4(1 pure)/M5(1 provenance)/M6(1) caught; **M3 masked local bởi LSM** (property assert trực tiếp — planted file không tồn tại; vanilla CI bắt bằng missing-error) — ghi minh bạch trong PR.
- Docs: GAPS.md (A2 + unpack hardening), roadmap GĐ2 mục 1, Makefile help count (23→28 stale).

### CVE Check
- Kernel: `5.10.134-013.15.kangaroo.al8.x86_64`
- `cve/oci-unpack-traversal.{md,json}`: cve-check rc=0 + **cve-gate rc=0 → 0 CVE mới ngoài watchlist**.
- Watchlist không thêm dòng (không có component mới ship trong repo).

### Validation "không bị nhiễm"
- `make test`: **10 suites, failed: none** (cg 0+6 SKIP env-gated như baseline); `make test-web` **9/9**; `make test-minictd` **28/28**.
- Perf: **cold 2.30 ms / rss 1204 kB / disk 2.04 MB — PASS** (gate không đổi; cold path minict không đụng).
- Host baseline: **3/3 sha256 khớp Phiên 14** (unchanged); listeners nền đã biết (12600/12700/19001/19005/19006/81/3000); 0 minict/busybox sống; instances/ rỗng.
- Bằng chứng: `evidence/oci-unpack-traversal.txt`.

### Pha B/C — PR #106
- **#106** draft @ `6441428` (nhánh từ main mới nhất — B1 up-to-date).
- SCR comment `=== SCR RETEST — 6441428 ... ===` Verdict **PASS**: suites ×2 rc=0 byte-stable (10 suites / web 9/9 / minictd 28/28); mutants 5/5 + M0 collapse 8 suites; bench p50 2.0ms / 1188kB / 2.04MB; host unchanged; CVE rc=0.
- SIA: (kết quả ở mục dưới sau khi audit)
- B4: CI xanh → flip ready → squash `--match-head-commit` → #104 auto-close.

### Kế hoạch ngày mai
- **#103**: e2e `oci:` với crane thật (fetch pin v0.22.1 + sha256 theo D37; happy-path boot + negative control ref lỗi; ttl.sh/local registry cho hostile-image e2e nếu cần).
- GĐ2 tiếp: S3 (ro-bind submounts), S4 (unix-socket/TLS), S5 (spec rlimits cho exec), S6 (webport allocation), template (mục 2), storage overlayfs + snapshot (mục 3), Landlock (mục 6), control-plane ≤30MB RSS (mục 7).
- **GD3 (Web UI + console) CHƯA chạm** — per chỉ thị owner: mọi mục GĐ2 hoàn thành + GD3 sẵn sàng mở code → DỪNG + nhắc owner trước dòng code UI đầu tiên. Trạng thái GĐ2: mục 1 còn A2-e2e + S3–S6 + systemd-real; mục 2–7 chưa mở.

### Pha C + E — kết thúc Phiên 15
- SIA PASS @ fd9db7d (audit:pass, 2 low) → B4: fix(review) `5ddff7f` (GAPS residual
  mode-000) + `92b56e0` (MD27 bỏ no-op env -u) → SCR lại @ 92b56e0 (×2 rc=0: 10
  suites / web 9/9 / minictd 28/28 byte-stable) → SIA delta **PASS** (0 việc còn).
- CI @ 92b56e0: ci + unicode-guard + perf-guard **xanh** → flip ready (GraphQL
  `markPullRequestReadyForReview` — REST PATCH không flip được draft, đo được)
  → **squash `22831da`** match-head-commit 92b56e0 → **#104 auto-close**; nhánh
  xoá (local + remote).
- SBG MODE=apply: `sec/104-oci-unpack-traversal` DELETE_MERGED + local residue
  `feat/97-g3-preview-routing` DELETE_MERGED; ledger qua PR này; remote sau Phiên
  15: **chỉ `main`** @ `22831da`.
- Roadmap: mục "Đã xong" thêm #104 (phiên 15); GĐ2 mục 1 còn: A2-e2e (#103),
  S3–S6, systemd-real; mục 2–7 chưa mở.

DAY PASS — 2026-11-21 Phiên 15: issue #104 done (merge #106 `22831da` @ 92b56e0;
SCR clean-room ×2 SHA — suites rc=0 ×2 byte-stable, web 9/9, minictd 28/28 ×2,
mutants 5/5 + M0 collapse ×8 suite, bench 2.0/1188/2.04; SIA PASS 2 low → fix →
0; test-minictd 26→28), CVE gate rc=0, make test 10 suites none-failed, host
unchanged 3/3, perf 2.30/1204/2.04 PASS, main xanh `22831da`, remote chỉ `main`.
