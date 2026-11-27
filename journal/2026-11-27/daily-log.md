# Daily Log — 2026-11-27

## Phiên 42 — Orchestrator (Bắt đầu 02:04 UTC)

### Pha A — Đồng bộ + kế hoạch nhánh
- main `6fda589` (Phiên 41 ledger) — pull --ff-only OK; open PR: 0; open issue:
  #99 (p3 needs-repro), #8 (p2 CVE tracking) — không p0/p1 mới.
- SBG MODE=plan → `journal/2026-11-27/branch/plan.md`: không nhánh để dọn đầu
  phiên; Pha D = Track M M3 (#215).
- Lưu ý môi trường: sandbox mới (recycle) — Go toolchain phải cài lại
  (go1.27.4, `/home/z/my-project/toolchain/go`, download từ dl.google.com;
  `gh` CLI không có — mọi tương tác GitHub qua REST `scripts/ghapi.sh` +
  GraphQL chỉ cho mark-ready).

### Pha B/C — PR đang mở
- Không có PR nào mở đầu phiên → Pha B rảnh, Pha C không có gì gộp.

### Pha D — Việc mới: Track M M3 (issue #215, D81)

## Issue xử lý
- #215 — feat(track-m): M3 registry template tĩnh — status: **done**
  (PR #216 squash `6dbddb0` @ main, CI 6/6 xanh từng SHA, SCR PASS + SIA PASS)

## Research
- Nguồn 1: Snyk Zip Slip (security.snyk.io/research/zip-slip-vulnerability) —
  traversal khi giải nén archive; reject tuyệt đối thay vì sanitize; repo đã có
  lời giải mạnh hơn (secureUnpack #104/D50) → tái dùng qua import, không copy
  (bài học drift D25).
- Nguồn 2: OCI image/distribution spec (opencontainers.org 1.1 blog + Quarkslab
  2022) — index (descriptor: digest/size) tách khỏi blob; sha256-trước-giải-nén
  là model chuẩn; threat model 2 lớp (registry bị kiểm soát hoàn toàn →
  extractor là tuyến cuối).
- Nguồn 3 (nội bộ): snapshot.go D57 deviation gzip↔zstd; unpack.go đo được
  GNU tar vs busybox tar.
- Chi tiết: `journal/2026-11-27/research.md`

## Development
- Nhánh: `feat/215-template-registry` → PR #216 → squash `6dbddb0` trên main
- Tóm tắt thay đổi: `sandbox template push|pull` (client-only, zero-dep,
  daemon không đổi logic — chỉ +8 dòng wrapper export `minict.SecureUnpack`):
  index.json (name/version/sha256/size/created, strict decode) + blob
  `<name>-<ver>.tar.gz` (deviation .tar.zst→.tar.gz ghi rõ D81/roadmap);
  pull: HTTPS bắt buộc (HTTP chỉ `--insecure`+loopback), redirect hạ cấp bị
  chặn, sha256+size kiểm TRƯỚC giải nén (sai → không để lại file), extract
  vào temp dir → rename khi thành công, refuse overwrite `--out`; push chặn
  symlink tuyệt đối (D70); root URL chặn segment `.`/`..` (kể cả hậu tố).
- Perf: cold_start=1.80–2.35 ms, rss=1096–1212 KB, disk=2.05 MB (client-layer,
  không đụng hot path; perf-guard runner PASS)

## CVE Check
- Kernel: 5.10.134-013.15.kangaroo.al8.x86_64
- CVE mới: 0 ngoài watchlist (kernel-discover: 21 HIGH NVD cửa sổ 2 ngày →
  0 listed; CRITICAL 0; NVD-live đầy đủ nhờ tools/cve-kernel.py chạy trực tiếp)
- Ảnh hưởng sandbox: không (watchlist 12 CVE tracked, không đổi)
- Hành động: không cần; files `journal/2026-11-27/cve/template-registry.{md,json}`,
  `cve/kernel-discover.{md,json}`

## Validation "không bị nhiễm"
- `make test`: 10 suites 0 FAIL (isolation 9 + seccomp 5 + netns 7 + alpine 0skip1
  + debian 4 + escape-alpine 38 + pasta 8 + stack 5 + cve-tool 4 + cg 6 SKIP env)
- `make test-web`: 9/9 PASS
- `make test-minictd`: **64 PASS + 8 SKIP + 0 FAIL** — **MD69 PASS** (push →
  python static server → pull roundtrip + gate + sha fail-closed + evil tar);
  PORT0 mở rộng TPL 18500–18699 disjoint — PASS
- Unit: `go test ./cmd/sandbox/` 33/33 (14 test mới)
- Mutant (B3 bắt buộc): M1b sha-check disabled / M2 gate disabled / M3b cleanup
  removed → **3/3 CAUGHT**
- Host baseline: **unchanged** (passwd/bash/sudo sha256 khớp Phiên 40; ss -tlnp
  identical infra 12600/19001/12700/19005/19006/81)
- Bằng chứng: `journal/2026-11-27/evidence/template-registry.txt`

## PR
- #216 — feat(track-m) M3 (#215) — draft→ready→merged — CI: 6/6 xanh
  (build-test + perf + scan + console + perf-guard + unicode-guard) — Merge: CÓ
  (squash `6dbddb0`; #215 auto-close)
- SCR: RETEST PASS (clean-room = runner CI job 140278708536 success + dev
  bổ trợ; mutant 3/3 CAUGHT)
- SIA: PASS — 3 finding low (TOCTOU rename empty-dir; tie-break version chuỗi;
  nhãn `thử nghiệm` chờ ổn định) — không chặn gộp, ghi residual ledger

### Sự kiện giữa phiên (owner)
- Owner push PR #217 (`fix(console): Files works on minictd + UTF-8 terminal
  input — owner local fixes, stage 1`) lên main → `dd4aa5d` lúc 02:47 UTC,
  giữa chu trình của agent. main @ `dd4aa5d` CI xanh (build-test/console/scan
  success). Không xung đột với M3 (file sets rời nhau). Nhánh nguồn
  `fix/console-utf8-files-stage1` trên origin: KEEP — tài sản của owner, đã
  merge squash, KHÔNG xoá trong chu kỳ SBG tự động (agent chỉ dọn nhánh của
  agent); owner có thể xoá tay.

### Pha E — Kết phiên (SBG apply)
- Nhánh agent đã xoá: `feat/215-template-registry` (remote+local, squash
  `6dbddb0`); 2 nhánh agent local stale từ Phiên 32:
  `feat/165-pasta-portforward-config`, `chore/ledger-2026-11-25-p32`
  (nội dung đã squash `d7cb8a3`/`77a2f35` vào main từ lâu).
- `journal/2026-11-25/branch/branch-report.json` (artifact SBG Phiên 32 chưa
  commit) được commit trong ledger PR hôm nay — không sửa nội dung (§10).
- Ledger qua PR `chore(branches): ledger 2026-11-27 Phien 42`.

## Kế hoạch kế tiếp
- Track M: **M4** chỉ docs-thiết-kế chờ owner duyệt (không code) → việc kế tiếp
  thực tế: M5 (examples VPS deploy) hoặc triage #99 (MD25 negative control,
  p3 needs-repro) + #8 (CVE tracking p2) nếu owner ưu tiên.
- Residual chờ owner: `make console-test` vào workflow ci (G7, từ Phiên 40);
  3 finding low của SIA #216 (renameat2 NOREPLACE / semver tie-break / nâng
  nhãn `sẵn sàng` khi MD69 ổn định).

=== SDDA REPORT — 2026-11-27 (Phiên 42) ===
Issue xử lý : #215 — feat(track-m) M3 registry template tĩnh (D81)
PR mở      : 0 (đã gộp: #216 squash 6dbddb0) — CI 6/6 xanh từng SHA
CVE check  : 12 CVE tracked (watchlist), 0 mới, 0 ảnh hưởng
Escape test: PASS (10 suites 0 FAIL; test-minictd 64+8+0 với MD69 mới PASS dev+runner)
Host       : unchanged
Perf       : cold_start=1.80–2.35 ms | rss=1096–1212 KB | disk=2.05 MB — PASS
Daily-log  : journal/2026-11-27/daily-log.md
Kế tiếp    : Track M M5 hoặc triage #99/#8; owner-review console-test workflow (G7)
Trạng thái : DAY PASS — 1 issue → 1 PR → SCR/SIA → squash merge; remote chỉ còn
             `main` + nhánh owner `fix/console-utf8-files-stage1` (KEEP có lý do)
================================
