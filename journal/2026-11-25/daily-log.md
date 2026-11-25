# Daily Log — 2026-11-25

## Phiên 31 — Orchestrator (ZAI, §11) — issue #162

### Pha A — Sync + SBG plan
- Pull main `b447a8c` (Phiên 30 #161). 0 PR mở, 0 nhánh agent → Pha D.
- Owner chỉ định (2026-11-25): các tính năng lớn còn lại của GĐ2 — overlayfs
  rootless (mục 3), port-forward pasta (mục 4), observability/RSS (mục 7),
  systemd-real (mục 1 residual). Thứ tự: mục thấp nhất chưa xong → mục 3.

### Triage + chọn issue
- 9 issue mở (tất cả đã label; #8 CVE p2 tracking). Không p0/p1 security chen ngang.
- Tạo **#162** `feat(minictd): overlayfs rootless env-gated` (p1, area:minictd,
  type:feature) — owner-directed. Assign + in-progress.

### Research (journal/2026-11-25/research.md)
- kernelnewbies Linux 5.11: unprivileged overlay mounts + `userxattr` (user.overlay.*).
- kernel.org overlayfs docs: mount syntax, workdir cùng fs với upper, lower-only ro.
- Đo dev 5.10.134: `unshare -Ur` + mount overlay → EPERM → xác nhận cần env-gate.

### Development (`feat/162-overlayfs-rootless`, D66)
- Go daemon: `overlay.go` (gate `MINICT_OVERLAY=1` + `overlayFor` theo image form —
  base/dir/template zero-copy, tar/oci/backup unpack `<dir>/lower`; `validateOverlay`
  twin-gate) · `prepareRootfs(ovl)` (rootfs mountpoint rỗng + upper/work 0700) ·
  `renderSpec` directive `overlay lower=… upper=… work=…` · `InstanceState.Overlay` ·
  Snapshot/SaveTemplate refuse (mountpoint rỗng → archive rỗng = mất dữ liệu ngầm) ·
  spawnProxies resolves relay socket ở upper.
- C shim: directive `overlay` parse fail-closed; `spec_die` varargs; load_spec_file
  dời trước check /bin/sh (overlay check lower/bin/sh); mount overlay tại rootfs
  (userxattr) thay bind-onto-self; FAIL-CLOSED — mount lỗi → exit 125 + hint kernel.
- Go unit 9 test (gate/zero-copy/validation/refusal/legacy-regression).
- MD44–46 e2e env-gated: probe mount userns (`unshare -Urm`) → SKIP rc=125 khi
  kernel < 5.11; home-fs fallback; bisect diagnostics BI1–5.

### Debug log (bằng chứng quá trình)
1. CI run 45571512420 chạy MD44–46 SKIP cả runner → probe thiếu `--mount` namespace
   (`unshare -Ur` không tạo mount ns → MỌI mount EPERM; BI1 tmpfs cũng fail — không
   phải gate overlay). Fix `unshare -Urm` → MD44–46 chạy thật.
2. MD44 FAIL đầu tiên: `du` đọc inner `work/work` (kernel-owned 0700) + grep
   `"overlay":true` thiếu space (MarshalIndent). Fix 2>/dev/null + `[[:space:]]*`.
3. CI đỏ 1d23d43: gofmt drift trong overlay_test.go → `make minictd` (vet) fail trong
   test_cg.sh. Fix gofmt → 7e85b67 xanh.
4. SCR finding: unit zero-copy là RỖNG với mutant M-ovl-copy (copy vào lower/ không
   bị assert). Strengthen test → mutant bị bắt. Commit riêng.

### Pha B/C — PR #163 (squash `17f5a9709`)
- B2 — SCR: dev 43 PASS + 4 SKIP + 0 FAIL; runner CI cùng SHA **46 PASS + 1 SKIP +
  0 FAIL** (MD44/45/46 PASS kernel 7.0.0-1014-aws); perf dev 2.13 ms / 1240 kB /
  2.05 MB PASS; host unchanged.
- B3 — SIA: PASS — 10/11 contract OK (B10 daily-log = PR ledger này), claim sai 0,
  test rỗng 0 (3/3 mutant bị bắt), bảo mật 0 critical/high/medium, 2 low (đã ghi,
  không chặn). Label `audit:pass`.
- B4 — verdict pass → gộp. GraphQL `markPullRequestReadyForReview` (REST PATCH draft
  no-op với fine-grained PAT — ghi nhận).

### Pha E — Kết ngày (SBG)
- `feat/162-overlayfs-rootless` DELETE_MERGED. **#162 XONG** (auto-close).
- Remote chỉ `main` (`17f5a97`). CI main: squash commit CI run mới (giám sát bên dưới).

## Research
- https://kernelnewbies.org/Linux_5.11 — unprivileged overlay mounts 5.11 + userxattr.
- https://docs.kernel.org/filesystems/overlayfs.html — syntax, workdir cùng fs upper,
  userxattr namespace user.overlay.*.
- Elixir/raw kernel source (fs/overlayfs/super.c, params.c): FS_USERNS_MOUNT set;
  xattr-fail fallback warn; EPERM bisect chỉ ra thiếu mount ns (BI1) — không phải gate.

## CVE Check
- Kernel: `5.10.134-013.15.kangaroo.al8.x86_64`
- CVE mới: 0 ngoài watchlist (cve-check + cve-kernel discovery)
- Ảnh hưởng sandbox: **CVE-2023-0386 (overlayfs userns copy-up setuid) re-evaluated** —
  overlayfs GIỜ ĐƯỢC DÙNG (D66): exploit cần attacker tự mount; payload bị chặn
  NR_mount/NR_umount2 (D11 blocklist, MD0 parity); chỉ shim (tin tưởng) mount TRƯỚC
  payload; lower = rootfs tin tưởng. Watchlist cập nhật posture mới (2 dòng).
- Hành động: giữ D11 floor; nếu remote image không tin tưởng bật → re-evaluate.

## Validation "không bị nhiễm"
- `make test` (10 suites): 0 FAIL (isolation 38 + seccomp 25 + netns 7 + alpine 9 +
  debian SKIP-net + escape-alpine 38 + cve-tool 4 + pasta 5+6 SKIP + stack 8 + cg
  0+6 SKIP)
- `make test-web`: 9/9 PASS
- `make test-minictd`: dev 43 PASS + 4 SKIP + 0 FAIL; runner CI 46 PASS + 1 SKIP
- Host baseline: unchanged (sha256 /etc/passwd /bin/bash /usr/bin/sudo)
- Bằng chứng: `journal/2026-11-25/evidence/overlayfs-rootless.txt` + CI run
  45571949327 + comment SCR/SIA trên PR #163

## PR
- #163 — ready — CI: xanh (4/4 checks, run 45571949327) — merged squash `17f5a9709`
- Merge: CÓ (#162 auto-close)

## Kế hoạch ngày mai
- Issue ưu tiên: GĐ2 mục 4 — port-forward pasta config-side (khai báo trong config
  instance; routing-side G3 đã có #97) — owner đã liệt kê trong chuỗi tính năng lớn.
- Sau đó: mục 7 observability (log JSON + /metrics Prometheus text, stdlib-only),
  mục 1 residual systemd-real (lingering), residual D66 (idmapped mounts,
  overlay-aware snapshot, zstd).
