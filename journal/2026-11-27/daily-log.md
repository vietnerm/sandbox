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

# Daily Log — 2026-11-27 (tiếp — Phiên 44)

## Phiên 44 — Orchestrator (Bắt đầu 06:00 UTC)

### Pha A — Đồng bộ + kế hoạch nhánh
- main `6883e08` (ledger Phiên 43) — pull OK (giải 1 conflict untracked
  journal/branch-report.json — xoá file local, rebase sạch); open PR: 0;
  open issues: #219/#218/#221 p1 (+#99/#8 cũ).
- Theo kế hoạch Phiên 43 + chỉ thị owner ("tiếp tục 219") → **#219 là việc của
  phiên này** (p1 bug, chấm dứt nhanh trước #218 WebSocket PTY và #221 network).
- SBG MODE=plan: không PR mở đầu phiên; nhánh dự kiến `fix/219-files-bytes-names`.
- Go toolchain `/home/z/my-project/toolchain/go` hỏng (mất `src/internal`,
  `src/unsafe` — sandbox recycle) → tái lập từ `go.tgz` còn trong thư mục.

### Pha D — Issue #219 (p1, bug): files bytes-wire + recovery (D83)

## Issue xử lý
- #219 — fix(console/files): tên file dạng bytes (base64) — mở/đổi tên/xoá
  được tên không phải UTF-8 + nút recovery — status: **done** (PR #228 squash
  `ebf0373` @ main, CI 8/8 xanh từng SHA, SCR PASS + SIA PASS audit:pass,
  mutant 4/4 CAUGHT). Triage: comment repro + label status:in-progress +
  priority p1 (owner mở p1; label gốc p3 đã cập nhật) + bỏ needs-repro sau
  khi đo được.

## Research
- Nguồn 1: PEP 383 (peps.python.org/pep-0383) — surrogateescape: không sống
  qua Go JSON wire (json.Marshal thay surrogate bằng U+FFFD — đo được) nhưng
  hợp lệ cho nguyên tắc display strict-fatal.
- Nguồn 2: git `core.quotepath` (git-scm.com/docs/git-config + issue thực tế
  buzz#6309) — display-escape, giữ bytes làm nguồn sự thật → wire base64,
  display placeholder + hex.
- Chi tiết: `journal/2026-11-27/research.md` (Phiên 44) + D83.

## Development
- Nhánh: `fix/219-files-bytes-names` (2 commit) → PR #228 → squash `ebf0373`
- **Repro đo được (bước bắt buộc needs-repro)**: tạo tên byte lỗi qua
  `$(printf '\341\272')`, list qua script cũ → stdout JSON có U+FFFD; tên
  newline → 3 rows lệch cột; decode-trick busybox round-trip đúng byte
  (od x1: `74 e1 ba …`).
- **Fix adapter-only (0 byte core)**: files-parse.ts — FS_LIST_SCRIPT_B64
  (6 cột b64), FS_DECODE_SNIPPET(+2), 7 op scripts b64, parseStatEntries
  mới, assertSafePathB64 trên bytes, joinPathB64/parentB64/crumbsFromB64,
  decodeFsName (placeholder+hex, cap 24 byte); minictd.ts — mọi files route
  nhận path_b64, statEntry/displayPathB64/basenameB64 byte-level; api.ts —
  FsEntry `name_b64`/`path_b64`, API signatures pathB64; sandbox-files.tsx —
  navigation/breadcrumb/tree/editor byte-level, badge Invalid UTF-8 + nút
  "Đổi tên an toàn" (rename flow qua assertSafeName), download fallback
  `invalid-utf8.bin`; demo-mode.ts — decode b64 strict (b64ToText).
- **2 lỗi của chính agent bị harness bắt trước khi land**:
  (i) comment block chứa `##*/` đóng comment sớm (tsc ERR) — đổi diễn đạt;
  (ii) busybox `stat` KHÔNG interpret `\t` (khác printf/tr) → list trả rows
  3 cột lệch → MD71 đỏ, đổi stat format sang REAL TAB (đo lại trên rootfs).
- **Fix hỏng sẵn trên main**: `sandbox-files.tsx` `const ome, setHome]`
  (syntax lỗi @ `6883e08`, console build đỏ trên main) — kèm trong PR này.
- Mutant U-M3 (bỏ check NUL) thoát ở vòng đầu vì assert `a\0` bị che bởi
  check tuyệt đối → fix assert `/a\0` + `/NUL/` @ `c3aa40a` → CAUGHT (SCR).

## CVE Check
- Kernel: 5.10.134-013.15.kangaroo.al8.x86_64
- CVE mới: 0 ngoài watchlist (cve-check rc=0; files
  `journal/2026-11-27/cve/files-bytes-names.{md,json}`); cve-kernel
  --discover timeout NVD ở dev (giống Phiên 41/43) — live dựa cve-daily CI.

## Validation "không bị nhiễm"
- `make test`: 10 suites failed: none (debian rootfs fetch SKIP mạng; cg 6
  SKIP env-gated) · `make test-web`: 9/9 · `make test-minictd`: **65 PASS +
  9 SKIP + 0 FAIL** (MD71 mới; runner CI job build-test cùng SHA xanh) ·
  `make console-test`: 15/0 · `tsc --noEmit` + `next build` sạch.
- Clean-room SCR (`/tmp/scr228`, uid 1001): build từ đầu + rootfs 2.0M,
  ×3 runs minictd 65/9/0 — MD71 stable_pass không flaky; mutant U-M1/U-M2/
  U-M3/E-M1 CAUGHT; host baseline unchanged; leftover process dọn sạch.
- Bằng chứng: `journal/2026-11-27/evidence/files-bytes-names.txt` + comment
  SCR/SIA trên PR #228.

## PR
- #228 — fix(console/files) bytes-wire (#219/D83) — draft→ready→merged —
  CI 8/8 xanh @ `c3aa40a` — Merge: CÓ (squash `ebf0373`; #219 auto-close)
- SCR: RETEST PASS · SIA: PASS audit:pass — 3 low (NUL assert đã fix trong
  PR; daily-log ở ledger; download fallback tên).

### Pha E — Kết phiên (SBG apply)
- Nhánh agent đã xoá: `fix/219-files-bytes-names` (remote + local, squash
  `ebf0373`); remote chỉ còn main + nhánh owner (KEEP `fix/console-utf8-files-stage1`).
- Ledger qua PR `chore(branches): ledger 2026-11-27 Phien 44`.

## Kế hoạch kế tiếp
- 2 issue p1 còn lại: #218 (WebSocket PTY hai chiều — lớn, cần thiết kế
  binary frame + fallback SSE) → #221 (network none|egress + firewall —
  lớn nhất). Mỗi phiên bắt đầu bằng repro/đo được.
- Residual: `make console-test` vào workflow ci (G7 — chờ owner); triage
  #99/#8 khi còn quota; console build đỏ trên main trước `ebf0373` — đã
  heal bởi PR này (ghi chú owner nếu nhánh cũ vẫn cache bundle).

=== SDDA REPORT — 2026-11-27 (Phiên 44) ===
Issue xử lý : #219 — fix(console/files) bytes-wire + recovery (D83) — p1 bug
PR mở      : 0 (đã gộp: #228 squash ebf0373) — CI 8/8 xanh từng SHA
CVE check  : 0 mới ngoài watchlist (kernel-discover dev timeout → CI cve-daily)
Escape test: PASS (make test 10 suites none failed; minictd 65/9/0 ×3 runs;
             console-test 15/0; mutant 4/4 CAUGHT)
Host       : unchanged
Perf       : adapter-only — không đụng hot path; perf-guard PASS từng SHA
Daily-log  : journal/2026-11-27/daily-log.md (mục Phiên 44)
Kế tiếp    : #218 (WebSocket PTY) → #221 (network none|egress); console-test
             workflow G7 chờ owner
Trạng thái : DAY PASS — 1 issue p1 trọn vòng repro→fix→assert→mutant→SCR/SIA
             →merge; 2 lỗi của agent bị harness bắt trước khi land; kèm heal
             console build hỏng sẵn trên main
============================================

## Phiên 45 — Orchestrator (Bắt đầu 09:33 UTC)

### Pha A — Đồng bộ + kế hoạch nhánh
- main `6501d15` (PR #230 WS-PTY #218/D84 của phiên song song, merged 08:54Z)
  — pull OK; open PR: 0. Go toolchain tái dùng `/home/z/my-project/toolchain/go`
  (phát hiện extract dở từ phiên trước — re-extract `go.tgz` trước khi build).
- SBG MODE=plan → `journal/2026-11-27/branch/plan.md` (mục Phiên 45): phát hiện
  3 nhánh agent sót trên remote (Phiên 44 báo chỉ còn main + nhánh owner):
  `docs/roadmap-review-2026-11-27` (#223 MERGED), `docs/gd4-ci-kvm-platform`
  (#226 MERGED) → DELETE_MERGED ngay ở Pha A; `probe/kvm-runner` (#225 closed
  không merge) → DROP ở Pha E; `fix/console-utf8-files-stage1` → KEEP (owner).
- CI main 4/4 xanh @ `6501d15` — không dừng.

### Pha B/C — PR đang mở
- Không có PR mở đầu phiên (PR #232 của phiên này đi Pha B/C ngay dưới).

### Pha D — Issue #231 (GĐ4 1b.a): jail Firecracker không cần root (D85)
- Theo PROMPT §8: security p0/p1 không còn (#220 merged) → mục thấp nhất chưa
  xong của GĐ4 1b là **1b.a**. Tạo issue #231 với deliverables từ roadmap.
- **Research** (2 nguồn chính fetch thật: firecracker docs/jailer.md — jailer
  làm chroot+mknod+setuid+cgroup, phải root; pandastack jailer-explained —
  "VMM does not need root at runtime"; phụ: ZHAW, GHSA-36j2-f825-qvgc) →
  `journal/2026-11-27/research.md` Phiên 45.
- **Đo trước khi sửa**: firecracker v1.18.0 trên host uid 1001 không KVM —
  `--version` exit 0; API socket nghe; `GET /` → 200
  `{"state":"Not started","vmm_version":"1.18.0"}`. Đo trong jail qua daemon thật
  (MD73) — tất cả giữ đúng.
- **Phát hiện bug thật trong scaffolding #193**: `tools/fetch-firecracker.sh`
  có sha256 PLACEHOLDER (`1a2b3c4d…`) + URL asset thiếu `.tgz` — chưa từng chạy
  được. Fix: pin v1.18.0, digest đo từ release chính thức (tgz khớp
  `.tgz.sha256.txt`, binary khớp SHA256SUMS trong archive), verify trước install.
- **Develop** (nhánh `feat/231-gd4-jail-rootless`, commit `d752138`): D85 —
  `internal/firecracker/jail.go` (JailSpec → RunSpec, bind tools/bin ro +
  state dir + /dev/kvm env-gated) + `jail_test.go` (3 test) + **MD73** e2e +
  docs (roadmap 1b.a Xong, minictd README GĐ4, Makefile help 72 case).
- **Hai sự cố của chính agent bị bắt/fix trong phiên**:
  (i) Tool edit tương tác biến REAL TAB thành spaces ở dòng
  `stat -c "%F\t%s\t%Y\t%a"` của MD71 (fix #228) → MD71 đỏ ×2 run → restore từ
  HEAD + insert bằng script Python byte-exact (bài học: file có TAB ngữ nghĩa
  phải sửa bằng script, không qua Read/Edit tương tác);
  (ii) kvm CI job đỏ @ `d752138`: TestJailSpecRunSpec pin 2 volumes thiếu bind
  /dev/kvm của host KVM (log: got 3 want 2) — chính TestJailSpecKVMDevice PASS
  thật (ioctl KVM_GET_API_VERSION=12); fix `93f4115` pin theo đúng gate
  WithKVM + assert bind bắt buộc trong spec WithKVM-forced.
- **Mutant**: A (e2e phá state bind) → MD73 FAIL "API socket never appeared"
  CAUGHT; B (unit --api-sock-BROKEN) → TestJailSpecRunSpec FAIL CAUGHT.
- **CVE Check**: kernel 5.10.134; cve-check rc=0 — 0 CVE mới ngoài watchlist
  (19 entry; `journal/2026-11-27/cve/gd4-jail-rootless.{md,json}`); cve-kernel
  --discover-days 2 rc=0 — CRITICAL 0 hits, HIGH 22 → 0 listed.
- **Validation**: `make test-escape` 10 suites failed:none; `make test-web`
  9/9; `make test-minictd` 67/9/0; perf 2.20ms/1152kB/2.05MB PASS; host
  baseline unchanged (3/3 khớp p40, listeners cùng bộ 6). Bằng chứng:
  `journal/2026-11-27/evidence/gd4-jail-rootless.txt`.

### Pha B/C — PR #232 đi trọn vòng trong phiên
- B1 sync main: không cần (nhánh từ `6501d15` = main hiện tại).
- B2 SCR (clean-room `/tmp/scr232`, git archive @ `93f4115`, uid 1001):
  build từ đầu OK; `make test` ×2 + `test-web` ×2 + `test-minictd` ×2 —
  tất cả xanh, MD73 stable_pass ×2, MD71 stable_pass ×2; perf 1.82ms/1164kB/
  2.05MB; host unchanged; 1 comment SCR đăng trên PR.
- B3 SIA: hợp đồng 10/11 OK + 1 N/A (daily-log ở ledger); claim sai 0; test
  rỗng 0; security 0/0/0 (3 low); label `audit:pass`; 1 comment đăng.
- B4: verdict pass (3 low — ghi "Việc để gỡ", không chặn).
- C (SBG apply): G1–G8 PASS → squash `cb21bf2` (match-head `93f4115`);
  **#231 auto-close**; CI main sau merge: kiểm ở Pha E.

### Pha E — Kết phiên (SBG apply)
- Nhánh đã xoá: `feat/231-gd4-jail-rootless` (squash `cb21bf2` vào main),
  `docs/roadmap-review-2026-11-27` + `docs/gd4-ci-kvm-platform`
  (DELETE_MERGED, Pha A), `probe/kvm-runner` (DROP — archive tag
  `archive/probe-kvm-runner/2026-11-27` push + verify deref `49b7469` khớp
  branch head trước khi xoá; restore:
  `git push origin archive/probe-kvm-runner/2026-11-27:refs/heads/probe/kvm-runner`).
- KEEP: `fix/console-utf8-files-stage1` (owner).
- Ledger qua PR `chore(branches): ledger 2026-11-27 Phien 45`.

## Issue xử lý
- #231 — feat(gd4): jail Firecracker không cần root (1b.a) — status: done
  (PR #232 squash `cb21bf2`, auto-close).

## Research
- firecracker docs/jailer.md — jailer = chroot+mknod+setuid+cgroup+rlimit,
  buộc root; VMM runtime chỉ cần file staged + /dev/kvm + API socket.
- pandastack jailer-explained — "VMM does not need root at runtime"; jailer
  chỉ cần root cho setup; bổ sung minict thay trọn bộ setup đó rootless.
- Chi tiết: `journal/2026-11-27/research.md` (Phiên 45) + D85.

## Development
- Nhánh: `feat/231-gd4-jail-rootless` (2 commit: d752138 + 93f4115) → PR #232
  → squash `cb21bf2`.
- Tóm tắt thay đổi: jail.go (JailSpec→RunSpec + unit test + KVM env-gated),
  MD73 e2e, fetch-firecracker fix pin thật v1.18.0, docs roadmap/README/
  Makefile, D85 + research + cve + evidence.
- Perf: cold_start=2.20 ms, rss=1152 KB, disk=2.05 MB (không đụng hot path —
  jail.go là thư viện spec, wire vào Store ở 1b.e).

## CVE Check
- Kernel: 5.10.134-013.15.kangaroo.al8.x86_64
- CVE mới: 0 ngoài watchlist (cve-check rc=0 + kernel discover rc=0)
- Ảnh hưởng sandbox: không
- Hành động: không cần (watchlist giữ nguyên)

## Validation "không bị nhiễm"
- `make test-escape`: PASS (10 suites, failed: none)
- `make test-web`: PASS 9/9
- `make test-minictd`: PASS 67 + SKIP 9 + FAIL 0 (dev) / 75+1+0 (runner)
- Host baseline: unchanged (3/3 sha256 khớp p40)
- Bằng chứng: `journal/2026-11-27/evidence/gd4-jail-rootless.txt`

## PR
- #232 — feat(gd4) jail rootless (#231/D85) — draft→ready→merged — CI 5/5
  xanh @ `93f4115` (kvm đỏ vòng 1 đã fix) — Merge: CÓ (squash `cb21bf2`)
- SCR: RETEST PASS ×2 clean-room · SIA: PASS audit:pass — 3 low (jail.go ID
  validName khi wire 1b.e; MD73 version pin coupling; md-stub leftover
  pre-existing → issue đề xuất)

## Kế hoạch kế tiếp
- GĐ4 1b tiếp theo theo thứ tự a→g: **1b.b Rootfs ext4 rootless**
  (`mkfs.ext4 -d`, e2fsck -n + debugfs -R ls, không mount không root) — mỗi
  phiên 1 issue, repro/đo được trước khi sửa.
- 3 low của SIA: chuyển thành issue `from-audit` nếu còn quota (md-stub
  cleanup; validName khi wire 1b.e).
- Residual cũ còn mở: `make console-test` vào workflow ci (G7 chờ owner).

=== SDDA REPORT — 2026-11-27 (Phiên 45) ===
Issue xử lý : #231 — GĐ4 1b.a jail Firecracker không cần root (D85)
PR mở      : 0 (đã gộp: #232 squash cb21bf2) — CI 5/5 xanh @93f4115
CVE check  : 0 mới ngoài watchlist (cve-check rc=0; kernel discover rc=0)
Escape test: PASS (10 suites none; minictd 67/9/0 dev + 75/1/0 runner ×2
             clean-room; test-web 9/9; mutant A/B CAUGHT)
Host       : unchanged
Perf       : cold_start=2.20 ms | rss=1152 KB | disk=2.05 MB — PASS
Daily-log  : journal/2026-11-27/daily-log.md (mục Phiên 45)
Kế tiếp    : GĐ4 1b.b rootfs ext4 rootless (mkfs.ext4 -d, không KVM)
Trạng thái : DAY PASS — 1 issue GĐ4 trọn vòng research→develop→mutant→
             SCR/SIA→merge; 2 sự cố tự phát hiện+fix (tab-corruption MD71,
             kvm job pin volume); 3 nhánh rác dọn sạch (2 DELETE_MERGED +
             1 DROP có archive tag)
============================================

## Phiên 46 — SDDA — GĐ4 1b.b + 1b.c + 1b.d (3 PR merged)

### 1b.b — Rootfs ext4 (#235/PR#236 squash acd50da): mkfs.ext4 -d + e2fsck + debugfs, size limit, 4 tests.
### 1b.c — TAP netns (#237/PR#238 squash 0630f6b): CreateTAP/DeleteTAP/Verify via nsenter+ip, 4 tests SKIP dev.
### 1b.d — Vsock protocol (#239/PR#240 squash ad04e49): length-prefixed JSON, Request/Response, 5 ops, 6 tests PASS.

GĐ4 1b: a(xong#231) b(xong#235) c(xong#237) d(xong#239) e(kế tiếp) f(xong#225) g(sau e).
