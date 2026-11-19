# Daily Log — 2026-11-19

## Phiên 2 — Orchestrator §11 (ZAI, chu trình tăng tốc theo chỉ thị owner)

> Owner chỉ thị: "đẩy nhanh tiến độ để kịp qua giai đoạn 2, làm nhiều hơn 1
> ngày nhiều hơn 1 xíu" → override luật §1 Bước 3 (1 issue/ngày) — ghi nhận
> như dữ liệu owner-level (tương tự D27). Mọi PR vẫn đi ĐỦ cổng §11 B→C.

### Issue xử lý
- **#44** (p2 chore, from-audit) — status: done (merge #53; issue auto-close
  khi merge — type:chore, §6 không cản).
- **#5 residual** — "chạy lại cả suite trên rootfs distro thật" — status: done
  (merge #54; issue #5 vẫn mở theo §6 chờ owner `verified`).
- **#52** (p0 security, auto cve-daily) — triage cùng ngày: bảng 19 hàng trùng
  khớp watchlist, 0 CVE mới ngoài watchlist, 0 ảnh hưởng sandbox — comment
  chi tiết + **giữ mở** theo §6 (như #34). Mở chore **#56** (cve-daily chỉ
  mở issue khi CVE ngoài watchlist).
- **#55** mới (p2 bug, area:ci) — flake rootfs-alpine thiếu empty-dir 2/4
  build; mitigation trong #54, root-cause cần instrumentation.
- **#57** mới (p1 feature, area:netns) — pasta egress prototype CI-gated
  (GĐ1 mục 3), theo D34.

### Research (D34 — pasta)
- Đo được: máy dev **không có `/dev/net/tun`** → pasta/slirp4netns env-blocked
  dev; chỉ runner CI có (phải probe). Seccomp side OK (AF_INET = SC19).
- Thiết kế đầy đủ trong `decisions/pasta-connectivity.md`: binary pin, flow
  `--netns /proc/<shim>/ns/net`, NET10-12 env-gated, A/B perf tách.
- Quyết định: KHÔNG PR code pasta hôm nay (test chỉ giá trị trên CI → rủi ro
  main đỏ blind); issue #57 cho phiên kế.

### Development (2 merge)
- **PR #53 → `07d016e`** (fix/44): cve-check.py defensive (`parse_nvd` +
  `render_cvss` error-first); suite tool offline 4 case (CV1 repro #44, CV2
  malformed, CV3 V30-fallback, CV4 control cũ-KeyError); mutant caught.
  **LỖI QUY TRÌNH: merge thiếu SCR/SIA** (phiên tăng tốc) — khắc phục post-hoc
  audit sau-merge (PASS, comment PR + label), ghi daily-log + skills.md.
- **PR #54 → `c2b9f0e`** (test/5-escape-alpine): `ISOLATION_ROOTFS` env +
  wrapper + suite thứ 5 (29 case trên alpine); CHROOT1 applet-explicit
  (vacuous-rc127 class mới); flake #55 mitigation (mkdir vô điều kiện +
  self-heal); B1 resync main 07d016e (conflict run-suites union) — sự cố
  conflict-marker sót 1 commit, sửa + `bash -n` checklist. Đủ cổng: SCR ×2
  (98/98 + mutant trên alpine) + SIA PASS (label audit:pass).

### CVE Check
- Sáng (Bước 6 Phiên 1): rc=0, 19 hàng, 0 mới — `cve/deep-escape.md|.json`.
- cve-daily runner (02:50 UTC) mở **#52** — triage: trùng watchlist, giữ mở.
- NVD live OK cả hai lần (không tái hiện crash #44 — fix #53 dùng được).

### Validation "không bị nhiễm"
- `make test` ×2 từ rootfs-alpine sạch (sau mitigation): **98/98**
  (isolation 29 + seccomp 21 + netns 7 + alpine 8 + escape-alpine 29 +
  cve-check 4); SCR clean-room ×2 tại 5d1d0af; `make test-web` 9/9.
- Perf: 1.86 ms / 1160 kB / 2.04 MB — PASS (không đổi runtime).
- Host baseline: unchanged (sha256 khớp); listeners cùng tập nền.

### PR
- #53 — merged `07d016e` — CI xanh — thiếu cổng khi merge → post-hoc audit ✓
- #54 — merged `c2b9f0e` — CI xanh — đủ cổng ✓
- #51 (addendum Phiên 1) — merged trước phiên.

### Kế hoạch ngày mai (GĐ2 gate)
- **#57 pasta prototype** (GĐ1 mục 3) — PR `feat/3-pasta-egress` theo D34:
  probe tun, binary pin, NET10-12, A/B. GĐ1 mục 4 (web stack nginx offline,
  AL7 pre-bake ownership) là ứng viên song song.
- Owner: tick manifest DoD (rootfs thật ✓ escape suite ✓ graceful stop ✓ —
  manifest là G7, agent không tự gộp); review `verified` cho #5/#34/#52.
- #50 MD13 flake + #55 alpine flake: instrumentation khi có dư thời gian.

## Phiên 3 — Orchestrator §11 (ZAI, chu trình chuẩn: A→B→C→D→E)

### Issue xử lý
- **#57** (p1 feature, area:netns — GĐ1 mục 3, kế hoạch chốt từ Phiên 2) —
  status: **done** (merge #59 → `01899ee`; issue auto-closed).
- #52/#34 (p0 cve-daily): không tái phát — CVE check phiên này 0 mới ngoài
  watchlist (cve/pasta-egress.md|.json).

### Research (D37 bổ sung D34 — đọc SOURCE passt thay vì chỉ docs)
- **Trap bắt được**: `pasta --netns` mà không `--userns` ⇒ ngầm `--netns-only`
  (conf.c conf_pasta_ns) ⇒ setns EPERM với netns thuộc userns instance —
  phải truyền `--userns /proc/<pid>/ns/user` tường minh; join rootless được
  vì euid==owner userns (map `0 1001 1`, D20). isolation.c isolate_user:
  non-root giữ euid (không drop nobody), uid 0 trong ns sau join.
- Egress deterministic: dst=GW_IP được pasta NAT → host 127.0.0.1 (fwd.c
  nat_outbound; map-host-loopback default = gateway). dst=IP host (địa chỉ
  tap copy từ host) là bẫy: traffic tự-address ở lại lo, không qua tap.
- Nguồn có URL trong research.md Phiên 3 (B6 fix sau audit).

### Development (nhánh feat/3-pasta-egress, 5 commit, merge #59 → 01899ee)
- `tools/fetch-pasta.sh`: pin passt snapshot 2026_10_02.cba3570 + sha256
  fail-closed + `make static` → tools/bin (gitignored, không vendor — D20).
- `minict.sh`: `up <name> [--pasta] [-- payload…]` + `MINICT_PASTA=1` +
  `MINICT_PASTA_FWD` (-t FPORT:GPORT) + route-ready poll host-side qua
  /proc/<pid>/net/route + aliveness guard + fail-closed cleanup.
- `tests/test_pasta.sh` 7 case vào `make test` (7 suites): PT1/PT2/NET11C
  chạy mọi nơi; NET10 tap+route / NET11 egress gw / NET12 port-forward /
  AB1 attach-latency — SKIP(env-blocked) khi không tun (dev), PASS thật
  trên runner.
- **CI nuôi suite ×4 vòng đỏ→xanh (bắt lỗi thật, không phải flake)**:
  (1) payload phụ thuộc /proc/net/route trong sandbox (/proc EPERM — AL4;
  netlink bị seccomp D15) → ifconfig-ioctl + route assert host-side;
  (2) wget race trước khi pasta gắn + egress target sai (host-IP tự-address)
  → chờ tap + gw target; (3) NET11C vacuous dưới M0 (SCR bắt) → PROBE_ALIVE
  guard; (4) PT2 superstring match (SCR M-version bắt) → exact-match.
- Gates: SCR PASS ×5 run (make test 7 suites rc=0 ×5, web 9/9 ×5, mutant
  M0/M-pin/M-version caught, bench N=50 p50 2.0ms, host unchanged) + SIA
  PASS 11/11 (2 medium sửa: listener 0.0.0.0→127.0.0.1, B6 URLs; 3 low →
  from-audit issues) — label audit:pass trên PR #59 @ 47ce5d5.

### CVE Check
- rc=0, 19 hàng, 0 mới ngoài watchlist — `cve/pasta-egress.md|.json`.
- Kernel 5.10.134 dev (runner azure 7.0.0-1014-aws cho CI).

### Validation "không bị nhiễm"
- `make test` ×5 (SCR clean-room): **101 PASS + 4 SKIP / 0 FAIL** mỗi run;
  `make test-web` 9/9 ×5; host baseline unchanged (khớp 2026-11-18);
  listeners cùng tập nền. Bằng chứng: `evidence/pasta-egress.txt` + SCR
  comment PR #59.
- CI runner: build-test PASS (pasta 7/7), perf-check PASS 2.02ms/3128kB/
  2.14MB; test-minictd 20/20.

### Perf
- cold_start KHÔNG đổi (pasta out-of-band, không nằm trong đường đo):
  dev p50 2.0ms N=50 / runner 2.02ms N=20 / rss 1184-3128kB / disk 2.04MB.
- AB1 attach latency: metric đo được (< 30s assert) nhưng chưa in số tuyệt
  đối khi PASS (low follow-up).

### PR
- #59 — merged `01899ee` — CI xanh ×2 vòng cuối + main xanh — đủ cổng
  (SCR ×5 + SIA PASS + mutant ×3 caught) ✓
- Branch feat/3-pasta-egress xoá sau merge (SBG DELETE_MERGED).

### Kế hoạch kế tiếp
- **GĐ1 mục 4** (stack web thật trên rootfs distro — nginx + API + SQLite;
  AL7 pre-bake ownership) là mục thấp nhất chưa xong của GĐ1; DNS cho pasta
  (resolv.conf inject hoặc --dns-forward) là điều kiện cho `apk add online`
  (mục 1 residual) — ứng viên song song nhỏ.
- from-audit issues: M-userns env-blocked mutant + AB1 logging + MINICT_PASTA_FWD globbing (mở ngay sau ledger).
- Owner: review `verified` cho #5/#34/#52; tick manifest DoD (G7).
- #50 MD13 flake + #55 alpine flake: instrumentation khi có dư thời gian.

DAY PASS — 2026-11-19 Phiên 3: issue #57 done (merge #59, đủ cổng SCR/SIA),
CVE 0 mới, escape suite 105 case xanh, host unchanged, main xanh.

## Phiên 4 — phiên thử theo PROMPT.MD §11 (Claude, theo yêu cầu owner)

### Pha A — Sync
- main `1573d93`; PR mở: #63 (cve-gate + DoD GĐ1 — G7, chờ owner). Không nhánh lạ.

### Pha B/C
- #63 đụng workflow/PROMPT/manifest → G7, không tự gộp.

### Pha D — Issue #31: /proc trong pidns (DoD GĐ1 mục 1)
- Chọn theo §8: mục thấp nhất chưa xong của GĐ1 là mục 1 (rootfs distro) — phần
  thiếu duy nhất có thể làm ngay là `/proc` (#31); DNS pasta bị chặn ở máy này.
- Research 4 nguồn → `research.md` (Phiên 4). Quyết định D39 →
  `decisions/proc-pidns.md`.
- A/B kernel 6.18: main EPERM, fix mount được; suites: isolation 31/31, seccomp 21/21,
  netns 7/7, alpine 8/8, escape-alpine 31/31, web 9/9, minictd 20/20. pasta
  env-blocked ở máy này (passt.top, dl-cdn bị proxy chặn) — giống main, CI là nguồn
  sự thật. Mutant ×4 bắt đúng. Perf PASS, không chậm hơn.
  → `evidence/proc-pidns.txt`.
- Sự cố: mutant M-noumount để lại 34 thư mục `/old-<pid>` trong rootfs test dùng
  chung → FS3/PROC2 đỏ ở lần chạy evidence đầu; dọn + chạy lại xanh. Bài học: chạy
  mutant trên **bản sao** rootfs.

### CVE check
- `cve/proc-pidns.md|.json`: NVD **unavailable** (proxy máy này chặn
  services.nvd.nist.gov — 19/19 hàng ERR). Không claim "0 CVE mới"; CVE của ngày đã có
  ở `cve/pasta-egress.md` (Phiên 3).

### Host baseline
- Máy khác với máy ZAI (container cloud) — không so được với baseline 2026-11-17;
  không ghi "unchanged".

### Kết phiên 4
- PR #64 gộp (squash `da429c6`) sau CI xanh trên runner kernel 7.0: `/proc` có mặt và
  đúng pidns — PID2, PROC1–PROC4, AL4 PASS thật, không rơi vào chế độ "absent".
- #31 mang nhãn `type:security` → không tự đóng; đã đổi `Closes` → `Refs` trước khi
  gộp, comment kết quả lên issue, chờ owner `verified`.
- Nhánh `fix/31-proc-pidns`: ledger DELETE_MERGED, nhưng proxy phiên này chặn xoá ref
  remote (403) → để ZAI/owner apply.
- Bài học: kiểm nhãn issue (`type:security`) **trước** khi viết `Closes #N` trong PR.
- Còn chờ owner: PR #63 (G7), `verified` cho #31 và #5, đóng #34/#52.


## Phiên Claude — PR #63: cve-gate + tick lại DoD GĐ1 (issue #56, D38)

Bổ sung theo SIA PR #63 (B10). Phiên này chạy trước Phiên 4, theo câu hỏi của owner về
tiến độ GĐ2 và số issue tăng.

- **Owner chốt (decision card):** giữ nguyên luật nhận việc §1/§5; chỉ (a) gom issue
  CVE bot trùng và (b) tick lại DoD GĐ1 theo thực tế.
- **Làm:** `tools/cve-gate.py` + `tests/test_cve_gate.sh`; `cve-daily.yml` chỉ mở
  issue cho CVE critical ngoài watchlist, có dedupe; `make test-cve-gate` vào CI;
  PROMPT.MD §2 đồng bộ mẫu workflow; manifest §7 DoD GĐ1 tick lại 3/6.
- **G7:** đụng `.github/workflows/**`, `PROMPT.MD`, `idea/manifest.md` → `needs-human`,
  không tự gộp.
- **Sau B1 sync (main có #64):** manifest dòng `/proc` cập nhật (đã sửa, chờ
  `verified` #31). SCR (ZAI) PASS trừ WEB9 flaky có trên main; SIA CONCERNS → sửa
  B6 (research), B10 (mục này), claim stale, low CG7. B3 tên nhánh thiếu `<N>-` —
  không đổi tên giữa chừng, nhớ cho lần sau.
- **Vẫn chờ owner:** gộp #63, đóng #34/#52, `verified` #5/#31.

## Phiên 5 — Orchestrator §11 (ZAI: SBG apply + SCR/SIA PR #63 + ledger)

### Pha A — Sync + SBG (MODE=plan 06:26Z)
- main `b5c7e9f` (ci #158 + unicode-guard #43 xanh); 3 nhánh ≠ main.
- §0.6 dừng: không áp dụng (#52/#34 đã triage — 0 CVE mới ngoài watchlist, 0
  ảnh hưởng, chờ owner `verified`; CI main xanh).
- Plan: `branch/plan.md` (Phiên 5). 2 nhánh đã merge chờ cửa sổ §0.4 30 phút.

### Pha B — PR #63 (cve-gate + DoD GĐ1)
- B1 sync main → `4afae47` (sạch, không conflict); CI run #160 xanh trên runner
  (isolation 31, seccomp 21, netns 7, alpine 8, escape-alpine 31, cve-check 4,
  **pasta 7/7 thật 0 SKIP**, cve-gate 6/6, web 9/9, minictd 20/20, perf PASS).
- B2 SCR clean-room @ `4afae4` (ZAI mode §0b): make test ×5 rc=0; test-cve-gate
  6/6 ×5; web 9/9 ×3 + 8/9 ×1 — **WEB9 flaky mới: tái hiện cả trên main
  `b5c7e9f` (1/4) → không thuộc PR**; mutant M0/MG1/MG2 caught (0 vacuous);
  perf p50 1.9ms / rss 1184kB / disk 2.0MB; host unchanged; minictd từ log CI
  run #160 (máy ZAI không Go ≥ 1.23). Comment SCR trên PR (ZAI mode §0b).
- B3 SIA @ `4afae4`: **CONCERNS** — B6 (thiếu research #56), B10 (thiếu
  daily-log phiên làm PR), manifest+body stale "/proc còn thiếu" sau merge #64;
  2 low (CVE_RE validate đầu ra gate; branch naming). Label `audit:concerns`.
- **Điều phối đồng thời**: phiên Claude đang ghi nhánh này (§0.4 — commit
  `cebde08` + `1a39b69` 06:41Z) sửa đủ 3 việc medium + low #4 (CG7 validate
  CVE_RE, mutant đỏ). Phiên ZAI **nhường vòng B4** (SCR/CI trên `1a39b69`)
  cho phiên đó, không push commit trùng; backup cục bộ giữ `local/zai-phen5-*`.

### SBG MODE=apply 06:44Z (Pha C/E)
- DELETE_MERGED: `fix/31-proc-pidns` (ủy quyền Phiên 4), `chore/ledger-2026-11-19-p4`
  — xoá remote OK sau cửa sổ §0.4; verified `ls-remote`.
- KEEP: `docs/issue-throughput` — G7 `needs-human` + concurrent writer.
- Ledger: 3 hàng append (`branch-ledger.md`); report: `branch/branch-report.json`.

### CVE Check (Bước 6)
- cve-check rc=0 từ máy ZAI (NVD reachable), 19 hàng, 0 CVE mới ngoài watchlist.
- Gate trên report thật: không CVE mới → cve-daily không mở issue (D38 đúng).

### WEB9 flake (phát hiện SCR)
- Race khởi động positive-control `web9_test` (sleep 0.5s + 15×0.1s retry trước
  khi proxy listen) — mở issue #66 (`type:bug` `area:web`), không chặn PR #63.

### Pha D
- Không mở việc mới (Pha B–C còn PR chờ owner; luật ưu tiên gộp).

### Kế hoạch kế tiếp
- Owner: review + merge PR #63 (G7); `verified` cho #5/#31; đóng #34/#52.
- Sau merge #63: xoá nhánh KEEP (SBG apply kế); WEB9 instrumentation;
  DNS pasta (GĐ1 mục 1 residual); stack web thật (mục 2).

