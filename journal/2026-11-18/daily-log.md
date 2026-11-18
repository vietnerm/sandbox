# Daily Log — 2026-11-18

## Phiên 1 — tích hợp control-plane `minictd` (owner bàn giao)

> Không phải chu trình SDDA thường: owner (Doan) giao gói `minictd-handoff.zip` do
> một agent khác viết và yêu cầu đưa vào repo + cập nhật prompt/tài liệu cho đúng
> tiến độ. Gói được xử lý như **code chưa tin cậy**: review tay, build, chạy thật,
> A/B với bản gốc.

### Issue xử lý
- Không có issue GitHub riêng (việc do owner giao trực tiếp). Nhánh
  `feat/minictd-control-plane`, nhãn đề xuất `phase:2` + `needs-human` (đụng
  `PROMPT.MD`, `prompts/**`, `.github/workflows/**`, `idea/manifest.md` — G7).

### Quyết định
- D23 `MINICT_SPEC`, D24 multi-webport, D25 exec setns (gate GĐ2 đo được),
  D26 review 7 lỗi R1–R7, D27 **GĐ1 vẫn là giai đoạn hiện tại** (owner chọn trên
  decision card) → `journal/2026-11-18/decisions/minictd.md`.
- Manifest §8 ghi ngoại lệ "GĐ2 seed gộp sớm" **trước** khi gộp; roadmap GĐ2 có luật
  D27 để SDDA không tự mở việc GĐ2.

### Development
- `sandbox/minict.c`: gộp patch MINICT_SPEC + sửa R7 (bind target không đi theo
  symlink; tạo file hay dir theo loại src — gói gốc luôn tạo dir nên bind file hỏng).
- `minictd/`: daemon Go (stdlib), `cmd/minict-exec` (C); sửa R1–R6 (xem D26).
- `tests/test_minictd.sh` 20 case, `make minictd`, `make test-minictd`, job CI.
- Lỗi của gói gốc được ghi nhận khi review: pump execstream alias buffer; Run lỗi để
  lại thư mục "ma"; daemon không auth mặc định; `os.Environ()` lọt vào sandbox (đo
  được cả env phiên làm việc lọt vào `execv2 env`).

### CVE Check
- Không chạy `tools/cve-check.py` trong phiên này (phiên tích hợp, không phải chu
  trình ngày) — **chu trình SDDA kế tiếp phải chạy bù**.
- Bề mặt mới: Go stdlib (`net/http`, `os/exec`). `go.mod` ghi tối thiểu 1.23 (đã hết
  hỗ trợ upstream) → CI dùng `setup-go` `go-version: stable` để luôn có stdlib vá mới
  nhất. Chưa chạy `govulncheck` (follow-up).

### Validation "không bị nhiễm"
- Máy: container kernel 6.18.44, chạy test dưới uid 1001 (không root).
- `make test`: isolation 21/21 + seccomp 20/20 + netns 7/7 + alpine 8/8 (sau khi cài
  `iproute2` — thiếu `ss` làm NS5 đỏ, lỗi môi trường); `make test-web` 6/6;
  `make perf-check` PASS (4.01 ms / 3104 kB / 2.15 MB trên máy này).
- Cold start A/B có/không patch: +0.10 ms median (nhiễu) → `evidence/minictd-regress.txt`.
- `make test-minictd` 20/20 → `evidence/minictd-e2e.txt`; cùng suite trên gói gốc
  14/20 (6 đỏ đúng các case R-fix) → `evidence/minictd-ab.txt`.
- Lưu ý SIA: evidence này do chính phiên tích hợp sinh ra — chỉ log CI/SCR mới tính.

### PR
- PR `feat(minictd): integrate control-plane daemon + setns exec (GĐ2 seed)` — để
  owner review (G7).

### Kế hoạch ngày mai
- SDDA quay lại GĐ1 (D27): mục thấp nhất chưa xong của roadmap GĐ1.
- Ứng viên `type:security` được phép chen ngang: **S1** (webproxy theo symlink
  socket trong rootfs, có từ D16).
- Chạy bù CVE check; xem kết quả job CI `test-minictd` trên runner (xác nhận D25 lần 2).

## Phiên 2 — Orchestrator §11 (ZAI): PR #36 loop + SBG + CVE bù

> Chu trình: pull (ff 5dc367b→81b5dce, 14 commit remote vào main; local chỉ có
> mode-change rác — bỏ) → Pha A SBG plan → Pha B loop PR #36 → Pha C (không gộp
> — G7) → Pha D triage #34 + CVE bù → Pha E SBG apply + ledger PR.

### Pha A — Sync + SBG plan
- Issue mở: #34 (p0 CVE auto, bot), #31/#30/#29/#28/#27/#26/#8/#5/#4. PR mở: #36.
- SBG `journal/2026-11-18/branch/plan.md` + `branch-report.json` (cập nhật apply).

### Pha B — PR #36 (ci/unicode-guard, Trojan Source gate)
- B1 sync main vào nhánh (86030d3): Makefile conflict (union .PHONY) — nhưng commit
  sync làm vỡ TAB recipe → CI đỏ `missing separator`; fix 7685b3f (tác giả PR push,
  byte-identical với fix cục bộ của runner; bài học → `docs/skills.md`).
- B2 SCR @ 7685b3f, clean-room uid 1001, kernel 5.10.134: `make test` 56/56 +
  `test-web` 6/6 + `test-unicode` (suite MỚI) 13/13, ×3 runs 0 flaky; minictd 20/20
  từ CI build-test 134589032778 (máy SCR không Go — env-blocked, nguồn ghi rõ);
  perf p50 1.9 ms / rss 1204 kB / disk 2.04 MB; host unchanged. Negative control:
  M0 bắt toàn bộ; unicode-mutant (U+202E inject) bắt được; **M2 (no-capdrop)
  KHÔNG bị bắt** — FS4/FS5 masked bởi seccomp mount-block (pre-existing) → issue
  #38; M4 chỉ bắt PID1 (PID2/SIG1 masked — đã biết AUDIT §D). Comment PR.
- B3 SIA @ 7685b3f: CONCERNS (B1/B5/B6 thiếu — đã sửa: issue #39 tracking + D28
  `decisions/unicode-guard.md` + `research.md` 3 nguồn verify; 0 claim sai; 0
  security finding mới; 1 medium pre-existing → #38). Labels `audit:concerns`,
  `needs-human`. Comment PR.
- B4 verdict: **G7 → không tự gộp** (diff đụng workflows + PROMPT.MD; body
  "Requested by Doan" là dữ liệu theo SBG §0.7, không phải approval). PR ở lại
  KEEP, ngày xem lại: sau owner review.

### Pha C — Gộp
- Không PR nào đủ cổng gộp tự động (#36 G7). `main` giữ nguyên 81b5dce.

### Pha D — Triage + CVE bù
- **#34** (p0 security, auto cve-daily): đối chiếu watchlist — 0 CVE mới ảnh hưởng
  sandbox; 9.8 CVE đều host-side (WEB2 loopback đo được); đã comment triage chi
  tiết + label `area:cve`; **giữ mở** (§6 không tự đóng security). Đề xuất: cve-daily
  chỉ mở issue khi có CVE ngoài watchlist (việc mới, qua PR sau).
- CVE check bù (nợ từ Phiên 1): `cve/unicode-guard.md` — bảng 19 CVE watchlist;
  NVD live cross-check timeout (mạng sandbox) → ghi `unavailable`, không claim
  "0 CVE mới từ NVD live" (nhẹ hơn SCR R7).
- Host baseline: /etc/passwd, /bin/bash, /usr/bin/sudo — **khớp chính xác** ghi
  2026-11-17 (`evidence/host-baseline.txt`) → UNCHANGED, không dấu hiệu nhiễm.
- Việc mới GĐ1 (S1 webproxy-symlink): **chuyển ngày mai** — hôm nay đã dùng lượt
  "1 issue/ngày" cho loop PR #36 (§1 B3).

### Pha E — SBG apply + đóng ngày
- DELETE_MERGED: `feat/minictd-control-plane` (remote, squash-match #35), 3 nhánh
  local đã gộp. KEEP: `ci/unicode-guard` (G7). Repo còn `main` + `ci/unicode-guard`.
- Ledger 2 dòng mới (`docs/branch-ledger.md`); bài học Makefile-tab + PR-body-is-data
  vào `docs/skills.md`; ledger qua PR `chore(branches): ledger 2026-11-18 + daily-log`
  (docs/journal-only — miễn G3/G4/G6, đủ G1/G2/G5/G7).
- Kết quả CI test-minictd trên runner (xác nhận D25 lần 2 theo kế hoạch Phiên 1):
  **20/20 PASS** (job 134589032778, kernel azure 7.0) — không cần lặp lại.

### Kế hoạch tiếp theo (ngày mai hoặc phiên kế)
- Owner review PR #36 (G7) → gộp → `make test-unicode` vào `make test` aggregator
  nếu owner muốn.
- GĐ1: S1 webproxy-symlink (security, được chen ngang) + issue #38 (suite hardening).
- cve-daily improvement: chỉ mở issue khi CVE ngoài watchlist (chore CI).
