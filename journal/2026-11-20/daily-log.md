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

## Phiên 10 — Orchestrator §11 (ZAI: SBG plan → Pha D #85 web stack → SCR/SIA PR #86 → merge → ledger)

### Pha A — Đồng bộ + kế hoạch nhánh
- `git pull --ff-only` main: up to date (`66a4f09`); issues mở 14, **PR mở 0**;
  roadmap đọc; cve-watchlist đọc; daily-log Phiên 9 đọc.
- SBG MODE=plan: remote chỉ `main` (sau Phiên 9), CI main success @ `66a4f09`,
  điều kiện dừng §0.6 không có → `branch/plan.md` ghi "không nhánh nào cần xử
  lý". Pha B/C lượt 1: bỏ qua (0 PR).

### Pha D — Việc mới

#### Issue xử lý
- #85 (mới mở phiên này) — feat(web-stack): GĐ1 mục 4 — stack web thật
  (nginx + API + SQLite, ./sandbox up|down|bench) — status: done (merged).

#### Research
- Nguồn 1: https://nginx.org/en/docs/http/ngx_http_fastcgi_module.html —
  fastcgi_pass unix + fastcgi_param SCRIPT_FILENAME/PATH_INFO là 2 biến quyết
  định script cho fcgiwrap; CGI thuần không nói chuyện được với nginx.
- Nguồn 2: https://github.com/gnosek/fcgiwrap (README.rst, master) — wrapper
  ~84 KB/instance, launcher chuẩn spawn-fcgi (-s sock -M mode -P pidfile);
  mode (1) SCRIPT_FILENAME trực tiếp; CGI scripts nhận HTTP env từ FastCGI
  params. Thêm: sqlite3 CLI `-json` ≥ 3.33 (Alpine 3.49.2) cho API JSON.
- Ghi: `research.md` (trade-off: loại python3/node/caddy vì ngân sách disk).

#### Development
- Nhánh: `feat/85-web-stack` (commits: journal D45/research/plan → feature →
  evidence/CVE → SIA fixes → test scoping).
- Tóm tắt thay đổi:
  - `sandbox.sh` (mới, root repo): `up|down|ps|shell|bench` một lệnh —
    `./sandbox` không khả thi vì `sandbox/` là thư mục từ PoC (D45 §1);
    per-instance config (nginx.conf/index.html/api.cgi/DB seed) ghi vào
    PRIVATE rootfs copy (pristine D44); payload `spawn-fcgi + sqlite seed +
    exec nginx`; host bridge webproxy.py (S1-hardened, không đổi); down =
    kill_tree + D19 pidns teardown.
  - `mkrootfs-alpine.sh`: pre-install `nginx sqlite fcgiwrap spawn-fcgi`
    tại BUILD TIME — apk của chính rootfs qua musl loader + `apk add --root`
    như calling uid (fakeroot-by-single-uid D20); **`--no-scripts` bắt buộc**
    — không có nó apk chạy trigger qua chroot → EPERM trên host →
    busybox/fcgiwrap/nginx bị mark BROKEN trong DB → mọi `apk add` sau trong
    sandbox rc=3 (AL5 bắt được — đo được; đây là root cause thật, không phải
    sửa test); CDN fail → rootfs vẫn dùng được không stack → test
    SKIP(env-blocked) (AL5 pattern, không đỏ giả CI).
  - nginx `user root` trong ns: apk non-root không tạo được user "nginx"
    (scripts không chạy); setuid uid unmapped = EPERM sau capdrop → workers
    root-IN-NS (uid 0 = calling uid, D20) — mọi file stack viết thuộc uid đó.
  - api.cgi: CGI → `sqlite3 -json`; name allowlist `[a-zA-Z0-9 _.-]` trước
    SQL (class injection khai trong D45; production API = GĐ2); json_esc
    ash param-substitution cho PATH_INFO echo (SIA low #2).
  - `valid_name()` guard up/down (SIA low #1, class #61).
  - Seccomp blocklist KHÔNG đụng → minict-exec.c sync giữ nguyên (MD0 PASS).
- Perf: perf-guard busybox **1.78 ms / 1196–1228 kB / 2.04 MB** PASS (không
  đổi); rootfs-alpine **8.6 → 13.3 MB** (≤ 20 ngưỡng); **stack cold start
  median 124–127 ms, max 233** (N=5 ×2 + ST7 suite) — ngưỡng roadmap < 5000 ms
  (dư ~39×); bench N=50 (SCR): p50 1.8 ms, rss 1156 kB.

#### CVE Check
- `cve/web-stack.md|.json`: cve-check.py rc=0 (19 hàng NVD live), **cve-gate
  rc=0 → 0 CVE mới ngoài watchlist**. NVD keyword API cho component mới
  rate-limited (HTTP rỗng ×3) — khai minh bạch; **OSV.dev thay thế: 0 CVE mở**
  cho nginx 1.28.3-r8 / sqlite 3.49.2-r1 / fcgiwrap 1.1.0-r8 / spawn-fcgi.
- Watchlist thêm mục "Web stack packages" (bảng per-component + exposure note).

#### Validation "không bị nhiễm"
- `make test`: **8 suites, failed: none** (dev 121 case: isolation 31 +
  seccomp 21 + netns 7 + alpine 9 + escape-alpine 31 + cve-check 4 + pasta
  5 PASS/6 SKIP env-blocked + **stack 8/8**); `make test-web` 9/9;
  `make test-minictd` env-blocked dev (Go thiếu) — CI runner: `go test ok` +
  MD0–MD19 20/20.
- Mutants: M-STACK-1 (sanitize strips) → ST5 CAUGHT; M-STACK-2 (fake API) →
  ST4+ST5 CAUGHT; M-STACK-3 (name guard strips) → ST8 CAUGHT; SCR M0 (no
  start) → 24 FAIL CAUGHT; M2 (no-capdrop) → 7 FAIL CAUGHT (CAPD1 chủ lực;
  FS4/FS5 giữ PASS là đúng defense-in-depth — mount chặn độc lập bởi seccomp
  NR_mount).
- **Bài học negative control bắt lỗi của chính test**: bản đầu ST5/ST4 của
  suite bị vacuous (PORT không propagate từ subshell run_case + assertion
  không short-circuit) — M-STACK-1 đã KHÔNG bị bắt lần chạy đầu → sửa
  `|| return 1` từng assertion + PORT_FILE trước khi push. ST6 leak-probe
  global bị cross-contamination giữa rooms (mutant room để lại process) →
  scope theo instance rootfs path (pidns teardown = kernel SIGKILL).
- Host baseline: sha256 /etc/passwd, /bin/bash, /usr/bin/sudo **3/3 khớp
  journal 2026-11-19** (unchanged); listeners cùng tập nền infra; instances/
  rỗng sau teardown. Bằng chứng: `evidence/web-stack.txt`.

#### PR
- **#86 — MERGED** squash `75fa1a6` match-head-commit @ `11e3d3a` — đủ cổng:
  SCR PASS ×3 SHA (`33b6c36` ×2 runs, `ca7e0b8`, `11e3d3a`) + SIA PASS (label
  audit:pass @ ca7e0b8, 2 low → FIXED vòng này) + CI xanh ×3 SHA (build-test
  + perf + scan; stack 8/8 + pasta 11/11 0 SKIP + minictd 20/20 trên runner);
  ready flip sau audit; #85 auto-close; nhánh xoá sau merge; remote chỉ `main`.

#### SBG (Pha C + E)
- MODE=apply: PR #86 gộp squash; nhánh feat/85-web-stack DELETE_MERGED (PR
  MERGED + headRefOid = HEAD nhánh — squash nên không ancestor trực tiếp,
  §2 đối chiếu PR state); ledger dòng mới appended qua PR ledger này;
  remote sau dọn: chỉ `main`.

### Kế hoạch kế tiếp
- GĐ1 mục 4 residual: caddy thật (hiện nginx), API production-grade (GĐ2),
  SQLite WAL/concurrency — đã ghi roadmap.
- #75 (port-collision test_web) + #72 (WEB9 residual) khi đụng web suite;
  from-audit #77/#62/#61 (low); #55 flake instrumentation; #26/#27 docs CI.
- NVD keyword scan cho stack packages retry lần sau (rate-limited hôm nay).

DAY PASS — 2026-11-20 Phiên 10: issue #85 done (merge #86 `75fa1a6`, đủ cổng
SCR ×3 SHA + mutants ×5 caught + SIA PASS 2 low fixed), CVE 0 mới (gate rc=0 +
OSV 0), suite 8 suites none failed (121 case dev; runner 114/114 thật + stack
8/8 + minictd 20/20), host unchanged, main xanh `75fa1a6`, remote chỉ `main`.
