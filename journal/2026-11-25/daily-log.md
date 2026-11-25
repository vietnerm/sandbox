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

## Phiên 32 — Orchestrator (ZAI, §11) — issue #165

### Pha A — Sync + SBG plan
- Pull main `a7a05f2` (ledger Phiên 31 #164). Remote chỉ `main` (2 nhánh
  Phiên 31 đã DELETE_MERGED), 0 PR mở → Pha D. Halt-check: CI main xanh,
  không p0 security, host baseline unchanged. Plan: journal branch/plan.md.

### Triage + chọn issue
- 9 issue mở (đã label; #8 CVE p2 tracking; không p0/p1 chen ngang).
- Tạo **#165** `feat(minictd): port-forward qua pasta khai báo trong config
  instance (GĐ2 mục 4)` (p1, area:minictd+netns, type:feature) — owner-
  directed (chuỗi tính năng lớn còn: mục 4 → mục 7 → systemd-real).

### Research (journal/2026-11-25/research.md mục Phiên 32)
- passt.1: `-t first[-last][:toaddr]`, pasta bind forwards host-side mọi
  interface mặc định, default `-t auto -u auto` (same-port ns→host).
- Podman/Oracle pasta docs: option-set per container, `ss` thấy pasta
  LISTEN 0.0.0.0:port → pasta giữ host listening socket → lifecycle phải
  kill attach khi instance chết.

### Development (`feat/165-pasta-portforward-config`, D67)
- contract: `RunSpec.PastaPorts` (`pasta_ports` omitempty — D59 pattern).
- Go daemon `internal/minict/pasta.go` (mới): parse fail-closed
  (`[tcp|udp/]<host>:<ns>`, 1024..65535 host, không trùng, cap 16),
  `pastaArgs` (D37/D42 argv pin), `hostDefaultGW` (hex LE /proc/net/route,
  stdlib-only), `bakeResolver` (TRƯỚC spawn: legacy `rootfs/etc`, overlay
  `upper/etc` — không đụng lower zero-copy), `spawnPasta` (tun check +
  netns check + readiness default-route + alive-check, fail-closed),
  `stopPasta` (pid+ticks guard), `pastaPortsHeldBy` (S6 carry-over).
- runtime: Run (validate → 4xx; R4 rollback kill pasta; attach sau payload;
  state PastaPID/PastaTicks) · Start (re-validate + collision; pasta-fail →
  kill payload + STOPPED `KilledBy=pasta-fail`) · Stop/Remove/alive-heal
  (kill attach — host listening socket phải được giải phóng).
- main.go: flag `--pasta-bin` (beside-binary → PATH, resolution lazy).
- Shim C KHÔNG đổi (pasta out-of-band như webproxy — MD0 parity intact).
- Tests: 8 Go unit (argv pin chính xác, 17 nhóm sai, GW hex LE, resolver
  split, spawn fail-closed) + MD47–49 env-gated `/dev/net/tun`.

### CVE Check
- Kernel: `5.10.134-013.15.kangaroo.al8.x86_64`
- 19 entries — 0 mới ngoài watchlist, 0 CVE passt/pasta (giống Phiên 31)
- Ảnh hưởng sandbox: không — pasta host-side, payload vẫn D11 blocklist
- File: journal/2026-11-25/cve/pasta-portforward.md + .json

### Validation "không bị nhiễm"
- `make test` (10 suites): 0 FAIL (dev: 43 PASS + 7 SKIP — 4 overlay D66 +
  3 pasta D67 env-blocked)
- `make test-minictd`: PASS=43 SKIP=7 FAIL=0; `make test-web`: 9/9 PASS
- Host baseline: unchanged (diff rỗng trước/sau)
- Perf: cold_start=2.04 ms | rss=1196 kB | disk=2.05 MB — PASS, đường cũ
  không đổi (pasta chỉ spawn khi config declare; attach latency ghi note)
- Bằng chứng: journal/2026-11-25/evidence/pasta-portforward.txt

### Pha B/C — PR #166 (squash `d7cb8a3`)
- CI lần 1 đỏ (run 45582995043): MD48/49 port probe sai ngữ nghĩa — TIME_WAIT
  của curl trước đó giữ EADDRINUSE sau khi listener đóng → probe đổi sang
  listener-gone `ss -tln` (commit f48bb08; passt đặt SO_REUSEADDR — đo từ
  source pin nên re-attach trong cửa sổ TIME_WAIT hợp lệ).
- CI lần 2 đỏ push/đỏ xanh PR (run 45587261541 vs 45587273793) — **PRODUCT
  BUG thật, MD49 flaky**: pasta `setns` vào INIT userns → EINVAL vì shim
  chưa unshare lúc pasta spawn (stat ns/net chỉ chứng minh process sống).
  Fix `waitNsDistinct` (so link-target ns của shim với /proc/self) +
  TestWaitNsDistinct + readiness giờ đọc đúng route ns instance (artifact
  "attach 0ms" của MD47 cùng gốc — commit 04f51db).
- B2 — SCR @ 04f51db: dev 43 PASS + 7 SKIP; runner CI cùng SHA **49 PASS +
  1 SKIP + 0 FAIL** — MD47/48/49 PASS thật (run 45590685583); mutant 6/6
  caught (M-fwd-parse/-2, M-args, M-gw, M-resolver, M-stop-pasta), control
  PASS; perf dev 1.96 ms / 1200 kB / 2.05 MB PASS; host unchanged.
- B3 — SIA @ 04f51db: PASS — 11/11 contract OK, claim sai 0, test rỗng 0,
  bảo mật 0 critical/high/medium, 2 low (→ residual D67, không chặn). Label
  `audit:pass`.
- B4/C — SBG apply: squash `--match-head-commit 04f51db` → **`d7cb8a3` trên
  main; #165 auto-close**; nhánh DELETE_MERGED.

### Pha E — Kết ngày (SBG)
- `feat/165-pasta-portforward-config` DELETE_MERGED (không cần archive —
  đã gộp trọn). **#165 XONG.**
- Ledger qua PR `chore(branches): ledger 2026-11-25 Phiên 32`.
- Remote kỳ vọng: chỉ `main`.

## PR
- #166 — ready — CI: xanh (run 45590685583 + unicode-guard + perf-guard) — merged squash `d7cb8a3`
- Merge: CÓ (#165 auto-close)
