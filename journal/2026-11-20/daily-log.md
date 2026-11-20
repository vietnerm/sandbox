# Daily Log — 2026-11-20

## Phiên 9 — Orchestrator §11 (ZAI: SBG plan → Pha D #80 alpine hygiene → SCR/SIA PR #83 → merge → ledger)

### Pha A — Sync + SBG
- main `2c7e839` (ledger Phiên 8 #82); CI main xanh (ci #204 + unicode-guard #71).
- PR mở đầu chu trình: 0 → Pha B/C rỗng. Remote chỉ `main` (mục tiêu §11 giữ
  nguyên từ cuối Phiên 8); local chỉ `main`.
- plan.md Phiên 9 + branch-report.json (mode=plan, halted=false).
- `gh` CLI không có trên máy này → REST API qua curl/urllib (token PAT owner,
  không echo vào log); flip draft→ready phải dùng GraphQL
  `markPullRequestReadyForReview` (REST PATCH `draft` là read-only — học mới).
- Môi trường: kernel 5.10.134, uid 1001, **không có Go** (minictd lấy từ CI),
  NVD + dl-cdn reachable (khác máy Phiên 4).

### Issue xử lý
- **#80** (p2 bug area:ci, opened 2026-11-19 — alpine rootfs hygiene): triage
  trong 24h ✓ — gỡ `needs-repro` (repro đầy đủ trong issue), gán assignee +
  `status:in-progress`. Status: **done** (PR #83 squash `282234a`, #80
  auto-close; `type:bug` — được phép `Closes` theo §6).
- Chọn việc theo §8: không còn security p0/p1 / web p1 / netns p1 / perf breach;
  kế hoạch Phiên 8 chốt rõ "#80 alpine-hygiene TRƯỚC khi mở rộng suite alpine"
  (điều kiện dữ liệu sạch cho GĐ1 mục 4 — stack web thật sẽ dùng rootfs alpine).

### Research (D44 — 3 nguồn verified 200)
- **apk-world(5)** (apk-tools official repo): `apk add <file>.apk` LUÔN sinh
  constraint `><identity-hash` vào world — đúng dạng dòng corrupt
  `acpid-openrc><Q1W3l08syrWte8…` đo được. Root cause khép kín.
- **kernel.org overlayfs**: pattern copy-on-write template→upper — căn cứ cho
  `cp -a` per-suite; GĐ2 mục 3 (overlayfs rootless) landed thì chuyển được.
- **man7 cp(1)**: `cp -a` giữa cùng uid không đụng chown(2) — an toàn cho
  single-uid mapping (D20); precedent wrapper `minict.sh up --rootfs` (D43).

### Development (nhánh fix/80-alpine-hygiene, 2 commit, merge #83)
- **Rebuild pristine** rootfs-alpine từ tarball pin 3.22.8 (sha256 `2d7f46af…`
  khớp): diff git = chính xác 6 mục (M world/db/scripts.tar, D init.d|conf.d/
  acpid, D opt/local-repo/*.apk).
- **tests/test_alpine.sh**: AL5/AL6 cài trên bản sao `cp -a` (shared rootfs
  0 write) + AL5 anti-vacuous 2 bước (`apk info -e` FALSE trước install, TRUE
  sau); AL6 env-aware (sửa latent: CDN fail không còn đỏ giả); **AL9 mới**
  pristine guard (DB/world/artifacts + churn probe `git status`). Suite
  alpine 8→9, `make test` 113→114; roadmap cập nhật.
- **Sửa giả thuyết issue #80 (evidence-based)**: `usr/bin/tree` có SẴN trong
  tarball pristine (symlink applet busybox — verify `tar -tzf`) → GIỮ nguyên;
  `R:acpid` trong DB = logrotate file của busybox — không phải nhiễm.
- Quyết định D44 → `decisions/alpine-hygiene.md`.

### CVE Check
- `cve/alpine-hygiene.md|.json`: rc=0, 19 hàng NVD live, 0 lỗi NVD; cve-gate
  rc=0 → **0 CVE mới ngoài watchlist**. Kernel dev 5.10.134 (runner azure cho CI).
- Watchlist không cần thêm dòng (không CVE mới; apk-tools CVE-2021-30139/36159
  đã patched trong 2.14.13 như Phiên 8 ghi).

### Validation "không bị nhiễm"
- `make test` ×1 dev tree + ×2 clean-room: **114 case = 108 PASS + 6 SKIP
  (NET10/11/12/PT3/PT4/AB1 pasta env-blocked) + 0 FAIL**; `make test-web` 9/9
  ×2; AL5 install THẬT (acpid-openrc 2.0.34-r6, CDN + signature).
- **Churn = 0**: `git status --porcelain rootfs-alpine` sau 2 run = chỉ tập
  cleanup đã commit — diff EXACT khớp tập dự kiến.
- **Negative controls ×3 (clean-room, cùng SHA `689f171`)**: M-CONTAM (tái hiện
  nhiễm 42f3273) → AL5+AL9 FAIL — CAUGHT; **M-FLOW** (case giả định test-tương-
  lai ghi shared rootfs) → AL9 FAIL qua churn probe — CAUGHT; M-TEETH (old flow
  trên rootfs nhiễm) → rc=0 vacuous PASS — chứng minh class bug bị loại bỏ.
- **CI runner @ `689f171` (run #206)**: make test **114/114 thật 0 SKIP**
  (pasta 11/11, PT3/PT4 PASS thật), web 9/9, minictd MD0–MD19 **20/20** +
  `go test ./...` ok, perf 1.75ms PASS.
- Host baseline: sha256 3/3 khớp journal 2026-11-19 (unchanged); listeners cùng
  tập nền infra. Bằng chứng: `evidence/alpine-hygiene.txt`.

### PR
- **#83 — MERGED** squash `282234a` match-head @ `689f171` — đủ cổng: SCR PASS
  (×3 clean-room + mutant M-CONTAM/M-FLOW) + SIA **PASS** (label audit:pass,
  4 low không chặn) + CI xanh (run #206 @ 689f171 + main #207 @ 282234a);
  draft flip qua GraphQL (REST read-only); nhánh xoá sau merge (DELETE_MERGED);
  #80 auto-close; remote lại chỉ `main`.

### Bài học quy trình (ghi cho skills)
- REST PATCH `draft` trên PR là read-only — phải GraphQL
  `markPullRequestReadyForReview` (gh thiếu thì dùng GraphQL trực tiếp).
- Mutant timing-based KHÔNG dùng được cho suite sub-second (~0.47s): mutation
  sleep 2.0/6.0s rơi ngoài cửa sổ probe (~0.2–0.45s), pgrep poll 20ms bỏ lỡ
  process sống ~5ms; `cmd1 && cmd2 &` bind `&` cho CẢ chain (bẫy shell).
  Chọn flow-injection mutant (chèn case ghi shared rootfs) — deterministic.
- `git add <dir>/` fail khi dir nằm trong .gitignore dù files tracked — dùng
  `git add -u <dir>/`.

### Kế hoạch kế tiếp
- **GĐ1 mục 4: stack web thật** (nginx + API + SQLite trên rootfs distro;
  cold start stack < 5s; `./sandbox up|down|bench` một lệnh) — mục thấp nhất
  chưa xong của GĐ1; ownership pre-bake (AL7) hoặc apk pre-install vào copy.
- #75 (port-collision test_web) + #72 (WEB9 residual) khi đụng web suite;
  from-audit #77/#62/#61 (low); #55 flake instrumentation khi có dư thời gian.
- GĐ2 gate research vẫn mở: xác nhận minictd trên runner CI mỗi PR (đã chạy
  đều trong run #206).

DAY PASS — 2026-11-20 Phiên 9: issue #80 done (merge #83 `282234a`, đủ cổng
SCR ×3 + mutant ×2 caught + SIA PASS), CVE 0 mới (gate rc=0), suite 114 case
(108 PASS + 6 SKIP dev; 114/114 thật trên runner), host unchanged, main xanh
`282234a`, remote chỉ `main`.
