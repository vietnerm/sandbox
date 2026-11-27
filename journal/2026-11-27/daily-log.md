# Daily Log — 2026-11-27 (tiếp — Phiên 43)

## Phiên 43 — Orchestrator (Bắt đầu 03:03 UTC)

### Pha A — Đồng bộ + kế hoạch nhánh
- main `2aa9180` (ledger Phiên 42) — pull OK; open PR: 0.
- **Owner mở 4 issue p1 mới (#218–#221, phase:3)** trong lúc phiên 42 kết thúc:
  #218 (WebSocket PTY), #219 (files bytes-name), #220 (sec pasta — p1
  type:security), #221 (network none|egress). Theo Bước 3 §1: `type:security`
  + p1 chen trước Track M → **#220 là việc của phiên này**.
- SBG MODE=plan: không PR mở đầu phiên; nhánh dự kiến `sec/220-pasta-failclosed`.
- Go toolchain vẫn còn ở `/home/z/my-project/toolchain/go` (tái dùng).

### Pha B/C — PR đang mở
- Không có → rảnh, đi thẳng Pha D.

### Pha D — Issue #220 (p1, security): pasta fail-closed (D82)

## Issue xử lý
- #220 — sec(pasta): auto UDP inbound / bind 0.0.0.0 / ns→host loopback —
  status: **done** (PR #224 squash `c3d8b50` @ main, CI 6/6 xanh từng SHA,
  SCR PASS + SIA PASS, mutant 4/4 CAUGHT). Triage: comment + label
  status:in-progress (#218/#219/#221 để phiên sau, theo Bước 3: 1 issue/lần).

## Research
- Nguồn 1 (đo trực tiếp): `--help` + SOURCE của binary pin
  `passt-2026_10_02.cba3570` (tarball sha256-pinned D37, `tools/cache/`) —
  `fwd_rule_parse`/`parse_addrifname`/`fwd_rule_parse_ports`: một flag áp MỘT
  addr cho cả comma-list; FWD_SCAN là cờ per-spec; `none` phải đứng một mình;
  `-T/-U` từ chối listen-address; defaults `auto` cho cả 4 chiều (pasta mode).
- Nguồn 2 (podman parity): containers/common `libnetwork/pasta/pasta_linux.go`
  — podman truyền `-t/-u/-T/-U none` khi trống + `--no-map-gw` + `addr/range`
  cho published ports — cùng posture fail-closed.
- Chi tiết: comment triage trên #7208746030 + decision D82.

## Development
- Nhánh: `sec/220-pasta-failclosed` (5 commit) → PR #224 → squash `c3d8b50`
- **Đo trước khi sửa (bước a)** — MD70 diagnostic qua đường daemon thật, runner
  CI (commit `896c769`, run 45935460401, artifact md70-measure.txt):
  P1 `*:18024` (mọi interface) · P2 UDP auto count=1 (ns bind 5353 → host mở
  0.0.0.0:5353) · P3 sandbox → API minictd = `HTTP/1.0 401 Unauthorized`.
  Cả 3 lỗ hổng của issue xác nhận bằng đo live.
- **Fix (bước b)** — `pastaArgs` fail-closed podman-parity:
  forward khai báo bind `127.0.0.1/` (opt-in `public/` grammar v2
  `[tcp|udp/][public/]<host>:<ns>`, unknown scope → 4xx); `-t/-u none` khi
  trống; `-T none -U none --no-map-gw` luôn truyền; **auto forwarding (D42)
  bỏ hẳn** (UDP auto = lỗ hổng đo được; TCP auto range-scan conflict với
  forward tường minh cùng địa chỉ; wake/preview đi webproxy D16, không phụ
  thuộc auto).
- **2 vòng đo giữa chừng bắt 2 lỗi thiết kế của chính agent** (giá trị của
  "đo trước khi sửa" áp cho cả fix):
  (i) spec gộp `127.0.0.1/p,auto` → rule explicit kế thừa FWD_SCAN thành
  `(auto-scan)` rule điều kiện không bao giờ kích hoạt (md47-pasta.txt);
  (ii) `-t 127.0.0.1/auto` riêng → `Forwarding configuration conflict: TCP
  [127.0.0.1]:1-32767 (auto-scan) versus TCP [127.0.0.1]:18854 => 8080 —
  Unable to add rule` → pasta chết khi attach.
- Unit: TestPastaArgs (argv mới), TestPastaArgsFailClosed (`-t none -u none
  -T none -U none --no-map-gw`), TestPastaArgsPublicOptIn (public tách flag),
  grammar v2 valid/invalid (public 3 dạng + unknown/empty scope).
- e2e: MD70 chuyển thành assert 5 điểm (env-gated tun, chạy runner) + giữ
  measurement artifact; **UDP live-probe qua helper C tĩnh 20 dòng bind-mount
  Volumes** (busybox nc không có -u; không đổi rootfs).
- Perf: không đụng hot path (argv builder); perf-guard runner PASS.

## CVE Check
- Kernel: 5.10.134-013.15.kangaroo.al8.x86_64
- CVE mới: 0 ngoài watchlist (cve-check rc=0, mọi entry đều "from
  cve-watchlist.md"); kernel-discover lần này timeout NVD ở dev — phần live
  dựa workflow cve-daily CI (giống Phiên 41).
- Ảnh hưởng sandbox: không; files `journal/2026-11-27/cve/pasta-failclosed.{md,json}`

## Validation "không bị nhiễm"
- `make test`: 10 suites 0 FAIL (test_pasta 5 PASS + 6 SKIP env — đường legacy
  minict.sh KHÔNG đổi, NET11 gateway-mapping vẫn PASS)
- `make test-web`: 9/9 · `make test-minictd`: dev 64 PASS + 9 SKIP + 0 FAIL
  (MD70 SKIP env-gated; runner: MD47-49/64/70 PASS)
- Mutant (B3): **4/4 CAUGHT** — M1 bỏ -T/-U none; M2 bỏ loopback prefix;
  M3 bỏ -u none; M4 no-op check scope (bắt thêm lỗi coverage: 3 case invalid
  scope chưa từng có trong map vì patch thiếu assert — đã thêm thật rồi chạy lại)
- Host baseline: unchanged (passwd/bash/sudo sha256 khớp; listeners = infra)
- Bằng chứng: `journal/2026-11-27/evidence/pasta-failclosed.txt`

## PR
- #224 — sec(pasta) fail-closed (#220) — draft→ready→merged — CI 6/6 xanh
  @ `d29271e` — Merge: CÓ (squash `c3d8b50`; #220 auto-close)
- SCR: RETEST PASS · SIA: PASS — 3 finding low (xv2 không check rc trong MD70;
  auto-bỏ là behavior change đã ghi doc; `[public]` suffix trong notes).

### Sự kiện giữa phiên (owner)
- Owner/phiên khác merge PR #226 (GĐ4 kvm job tested on CI; manifest §10;
  roadmap platform layer) → main `c685056` trước khi #224 squash — không xung
  đột (file sets rời nhau); main @ `c3d8b50` CI xanh (build-test/kvm/scan/console).

### Pha E — Kết phiên (SBG apply)
- Nhánh agent đã xoá: `sec/220-pasta-failclosed` (remote+local, squash `c3d8b50`).
- Ledger qua PR `chore(branches): ledger 2026-11-27 Phien 43`.

## Kế hoạch kế tiếp
- 3 issue p1 còn lại của owner (thứ tự đề xuất): #219 (p1 bug files
  bytes-name — nhỏ, chấm dứt nhanh) → #218 (WebSocket PTY) → #221 (network
  none|egress + firewall — lớn nhất). Tất cả needs-repro → mỗi phiên bắt đầu
  bằng repro đo được.
- Residual: `make console-test` vào workflow ci (G7 — chờ owner); 3 finding
  low của SIA #224; triage #99/#8 khi còn quota.

=== SDDA REPORT — 2026-11-27 (Phiên 43) ===
Issue xử lý : #220 — sec(pasta) fail-closed defaults (D82) — security p1 chen trước Track M
PR mở      : 0 (đã gộp: #224 squash c3d8b50) — CI 6/6 xanh từng SHA
CVE check  : watchlist unchanged, 0 mới, 0 ảnh hưởng (kernel-discover dev timeout → CI cve-daily)
Escape test: PASS (10 suites 0 FAIL; test-minictd runner: MD47-49/64/70 PASS; mutant 4/4 CAUGHT)
Host       : unchanged
Perf       : không đụng hot path — perf-guard PASS từng SHA
Daily-log  : journal/2026-11-27/daily-log.md (mục Phiên 43)
Kế tiếp    : #219 (p1 bug) → #218 → #221; owner-review console-test workflow (G7)
Trạng thái : DAY PASS — security issue chốt trọn vòng đo→fix→assert→SCR/SIA→merge;
             2 lỗi thiết kế của chính agent bị chính harness đo bắt được trước khi land
============================================
