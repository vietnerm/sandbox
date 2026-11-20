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

---

## Phiên 11 — Orchestrator (2026-11-20)

### Pha A — Sync + SBG plan
- Sync main `bf409b8`; issues mở 14; PR mở **0**; CI main success; remote chỉ `main`.
- SBG MODE=plan (`branch/plan.md` mục Phiên 11): không nhánh ≠ main, 0 điều kiện dừng.
- **Env fact mới:** `gh` CLI không có sẵn trên máy ZAI phiên này → cài binary release
  `gh 2.63.2` vào `~/.local/bin` + PAT qua `GH_TOKEN` (REST như luật §11 khi GraphQL lỗi).

### Pha B — không có PR nào đang mở (bỏ qua).

### Pha C — không có PR để gộp trước việc mới (bỏ qua).

### Pha D — Issue #88 — Debian rootfs (GĐ1 mục 1 residual cuối, D46)

#### Triage + chọn issue
- #79 (apk online) đã xong từ phiên 7 → residual cuối của mục 1 = **Debian rootfs** (roadmap ghi "sau").
- Chưa có issue → tạo **#88** (area:rootfs, p1, type:feature), self-assign + `status:in-progress`.
- Blocker đo được trước khi viết code: `minict.c:1343` sanity hardcode `/bin/busybox` ⇒ debian bị chặn (`minict: no busybox`).

#### Research (journal/2026-11-20/research.md, Phiên 11)
- debuerreotype/docker-debian-artifacts dist-amd64 — rootfs chính thức `debian:bookworm-slim`,
  OCI layer digest pin được; tải về + sha256 **khớp** `2a7a0929…` (verified cùng phiên).
- deb.debian.org + rootfs slim đo trực tiếp: apt deb822 **HTTP**, keyring đầy đủ
  (signature verify mặc định), **không** ca-certificates; `_apt` uid 42.
- images.linuxcontainers.org — fallback nguồn (date-pinned + SHA256SUMS GPG), bỏ vì nặng hơn + không cần init system.

#### Development (nhánh `feat/88-debian-rootfs`, 6 commits)
- `tools/fetch-debian-rootfs.sh` (D37): pin URL + sha256 + size fail-closed, unpack
  `--no-same-owner` vào `rootfs-debian/` **gitignored** (83.3 MB đo được); dev placeholders
  `dev/{null,zero,random,urandom}` 0666 (mirror mkrootfs.sh — root cause DB8).
- `minict.c`: sanity `/bin/sh` executable + resolve symlink (D46); sau SIA: **cap 8 + lstat
  S_ISREG fail-closed** (finding medium). Seccomp blocklist KHÔNG đụng → MD0 PASS.
- `tests/test_debian.sh` **DB1–DB9** (9 case) vào `make test` (9 suites):
  DB1 boot anti-busybox · DB2 dpkg glibc · DB3 /proc env-aware · DB4 ownership
  (`/var/tmp` — `/tmp` tmpfs đo được) · DB5 chown EINVAL · DB6 control state-based ·
  DB7 apt online signature-verified (env-gated) · DB8 pristine manifest-hash ·
  DB9 self-symlink rc=125.
- Docs: roadmap (mục 1 ĐÓNG — residual chỉ HTTPS runtime khi cần), perf-baseline
  (debian supplementary 83.3 MB, gate busybox không đổi), cve-watchlist (apt 2.6.2
  fixed-at DSA-6607-1, dpkg 1.21.23 fixed-at, libc6 u14 0 mở, gpgv 0 mở — OSV
  per-version trên version THẬT; CVE-2011-3374 Debian-accepted residual tracked).

#### CI lessons (3 run đỏ → root-cause đủ, không guess)
1. Run 1: DB7 timeout — pasta build-first-cost trong cửa sổ chờ + DNS-timeout 1.1.1.1.
2. Run 2: diagnostics — DNS_READY chứng minh DNS OK; apt lỗi bị nuốt.
3. Run 3: **2 root cause thật**: (a) apt http method `setgroups(0)+setuid(42)` EPERM
   trong single-uid userns ⇒ "Method http has died" rc=112 → fix `-o APT::Sandbox::User=root`
   (methods = ns-root, no privilege change; đo được: pre-fix rc=100, post-fix tới fetch errors
   thật); (b) DB8 manifest flake = dev placeholders O_CREAT — CI diff **naming exactly
   dev/{null,zero,random,urandom}**. Cộng: `apt-get update` trả **rc=0-with-warnings**
   khi mọi source hỏng ⇒ DB6 đổi control state-based (install phải FAIL).

#### CVE Check
- `cve/debian-rootfs.md|.json`: cve-check.py rc=0 (19 rows), **cve-gate rc=0 → 0 CVE mới
  ngoài watchlist**. NVD keyword rate-limited (khai minh bạch, như phiên 10) — OSV.dev
  thay thế, đo trên version THẬT trong rootfs (apt 2.6.2 / dpkg 1.21.23 / libc6
  2.36-9+deb12u14 / gpgv 2.2.40): 0 CVE mở hành động được; 1 accepted-residual tracked.

#### Validation "không bị nhiễm"
- `make test-escape` 9 suites failed none (dev 124 PASS + 7 SKIP env-blocked:
  isolation 31 + seccomp 21 + netns 7 + alpine 9 + debian 8P/1S + escape 31 + cve 4 +
  pasta 5P/6S + stack 8); `make test-web` 9/9; test-minictd dev-blocked (Go thiếu) —
  CI runner: `go test ok` + MD0–MD19 20/20.
- Mutants (dev session): M-DEB-1 poisoned-pin refuse · M-DEB-2 busybox-sanity → DB1
  caught · M-DEB-3 control vacuous → DB6 caught · M-DEB-4 #80-contamination → payload
  APT_VACUOUS caught. **DB8 bắt contamination thật** do một-shot apt thủ công của
  chính SDDA khi debug (lists/auxfiles+lock) — rootfs restored pristine (diff vs tarball).
- **Bài học DB8**: pristine guard trên dir gitignored cần content-manifest materialized
  (digest-only diff không name file) — sửa trong vòng.
- Host baseline: 3/3 sha256 khớp 2026-11-19; listeners nền đã biết; bằng chứng
  `evidence/debian-rootfs.txt`.
- Perf: perf-guard busybox **2.19 ms / 1196 kB / 2.04 MB** PASS (gate không đổi);
  SCR bench N=50 clean-room 2.0/1128–1196/2.04 PASS; rootfs-debian 83.3 MB supplementary.

#### PR + Pha B (lại)
- **#89** draft tạo @ `3bb1c0e`. Pha B vòng 1: CI đỏ ×3 (root-causes trên) → fix
  `f90f6e6`/`b0e35db`/`2785467` → **CI xanh**; SCR clean-room ×3 @ `ff268a2` —
  **-Werror bắt 1 format-truncation thật** trong sanity mới → fix `ff268a2` → SCR PASS
  (108 case, 0 flaky, M0/M2/M-DEB-2/M-DEB-3 caught) → SIA **CONCERNS** (1 medium:
  symlink-loop no-cap + stale-path trong sanity mới; 2 low) → fix(review) `b75a825`
  (+ DB9 guard, DB4 20s) → CI xanh; SCR delta @ `b75a825` (M-CAP caught DB9 —
  mutant xóa cap → DB9 timeout-hang; M-DEB-2 caught) → SIA delta **PASS**
  (`audit:pass`). Số vòng Pha B: **3 vòng** (dưới limit 3/PR của §11 Pha B — verdict
  block chỉ vì code-fix cần CI, không phải security-block).

### Pha C + E — SBG apply
- PR #89: flip ready @ CI xanh + audit pass → **squash `7dc40af`** match-head-commit
  `b75a825` → #88 auto-close → nhánh DELETE_MERGED → remote chỉ `main`.
- Ledger: dòng Phiên 11 appended qua PR ledger này. G1–G8 PASS đủ cổng.

### Kế hoạch kế tiếp
- **GĐ1: cả 5 mục nền đã đóng** (rootfs distro xong cả alpine + debian; escape sâu
  xong; pasta xong; web stack xong). Residual ghi rõ: HTTPS runtime cho apt/apk khi cần
  (cài ca-certificates), caddy thật + SQLite WAL tuning (mục 4 polish).
- **GĐ2 mở kế tiếp** (control plane + storage) theo roadmap mục "Giai đoạn 2":
  1 daemon REST API (seed minictd — systemd `--user` unit, autostart, reconcile A3),
  CG1 cgroups v2 quota, G3 preview Traefik, template, storage overlayfs + snapshot,
  S1–S6 GAPS. **GĐ3 (Web UI + console) CHƯA chạm — per chỉ thị owner, khi roadmap
  tới GĐ3 sẽ dừng + nhắc owner trước khi mở code.**
- #26/#27 (docs p1) khi làm việc docs; #55 flake instrumentation; from-audit low backlog.

DAY PASS — 2026-11-20 Phiên 11: issue #88 done (merge #89 `7dc40af` @ `b75a825`,
SCR ×2 SHA + mutants ×6 caught total + SIA CONCERNS→PASS medium fixed), CVE gate rc=0
+ OSV per-version 0 actionable, suite 9 suites none failed (dev 124 PASS + 7 SKIP;
runner 124/124 thật gồm DB1–DB9 + pasta 11/11 + minictd 20/20), host unchanged,
main xanh `7dc40af`, remote chỉ `main`.

#### Process note (SBG Pha E — minh bạch vi phạm)
- **Vi phạm §11 luật "không push thẳng main"**: commit ledger `ac4347f` (chỉ
  `docs/branch-ledger.md` + `journal/2026-11-20/daily-log.md`) đã đi thẳng vào
  `main` qua `git push -u origin HEAD` chạy NHẦM khi HEAD vẫn là local `main`
  (đáng lẽ phải checkout nhánh trước khi commit). Không force-push; nội dung
  đúng như PR ledger dự kiến (docs-only); CI run trên main @ `ac4347f` được
  chờ xanh; nhánh `chore/ledger-11` thừa đã xoá. Phòng tránh: Pha E commits
  phải chạy trên nhánh `chore/ledger-<phiên>` từ đầu — thêm vào checklist
  phiên kế.

## Phiên 12 — Orchestrator (2026-11-20, ZAI — tiếp tục theo chỉ thị owner)

### Pha A — Sync + SBG plan
- Sync main `813afb1`; issues mở 13 → sau triage 14 (tạo #91); **PR mở 0**; CI
  main success; remote chỉ `main`. Host baseline đầu phiên **3/3 khớp**
  journal 2026-11-19 → UNCHANGED.
- SBG MODE=plan (`branch/plan.md` mục Phiên 12): remote chỉ `main`, 0 điều
  kiện dừng §0.6. Pha B/C lượt 1: bỏ qua (0 PR).
- Env: kernel 5.10.134 (container Debian trixie), uid 1001; `gh` CLI không
  có → REST API curl/urllib + PAT (env, không echo). **Go 1.27.4 cài mới**
  (`~/.local/go`, tarball sha256-verified từ go.dev) — máy dev thiếu Go
  trước đây; `make test-minictd` giờ chạy được local (trước: env-blocked).

### Pha B/C — không có PR nào đang mở (bỏ qua).

### Pha D — Issue #91 — GĐ2 mục 1: daemon service hoàn thiện (D47)

#### Chọn việc
- GĐ1 cả 5 mục nền đã đóng (Phiên 11) → luật D27 hết chặn GĐ2. Mục thấp
  nhất chưa xong của GĐ2 = mục 1: daemon REST API thiếu CLI `sandbox`,
  systemd `--user` unit, autostart, A3 reconcile (GAPS A3 "not yet built").
- Tạo issue **#91** (area:minictd — label mới, p1, type:feature, phase:2),
  self-assign + `status:in-progress`, gỡ `needs-repro`.

#### Research (journal/2026-11-20/research.md, Phiên 12 — 3 nguồn verified 200)
- systemd.unit (freedesktop): user unit search path `~/.config/systemd/user/*`,
  `WantedBy=default.target` cho user service.
- Podman quadlet: generate-then-install pattern; `Restart=on-failure` mặc định.
- loginctl: `enable-linger` = điều kiện tiên quyết boot-time user manager.

#### Development (nhánh `feat/91-daemon-service`, 8 commits, merge #92 `7803730`)
- `minictd/cmd/sandbox` (CLI mới): thin shell over `contract.HTTPClient`
  (list/up/start/stop/rm/exec/inspect/stats/logs/version); token resolve
  4-bước; `--cmd` comma/space split; không thêm logic (NO MAGIC DEFAULTS).
- `minictd/deploy/minictd.service.in` + `install.sh`: render fail-closed
  (placeholder còn → abort; sed-escape `&|\\` — SIA low), copy binary,
  token 0600 once, KHÔNG tự enable/linger (docs-only steps).
- `cmd/minictd`: `--autostart` (Start() mọi instance state=running lúc
  boot; operator-stopped vẫn stopped — `--restart=unless-stopped` semantics)
  + `--reconcile-interval N` (A3 heal-only ticker, ctx-cancelled).
- `internal/minict`: `ReconcileAll()` (SIA medium fix: **per-instance flock
  cho toàn bộ heal** — không còn TOCTOU đè state running của Start đang
  chạy) + `AutostartNames()`; Go unit tests ×4 (dead healed / live untouched /
  broken skipped / missing root).
- `tests/test_minictd.sh`: **MD20** CLI round-trip, **MD21** autostart
  wake-recreate (kill daemon → kill shim → daemon mới → PID mới ≠ cũ +
  fs persist → rm sạch), **MD22** deploy render + `systemd-analyze verify`
  (SKIP 125 khi thiếu tool). Suite 20 → **23 case**.
- **Fix bug đo được trong phiên**: runner pipe (grep/tail) thoát sớm →
  SIGPIPE → bash EXIT trap KHÔNG chạy → daemon `--autostart` (MD21) sống
  sót sau run — **6 daemon rò đo được (`ps aux`) từ 6 lần chạy suite**.
  Fix: cleanup catch-all `pkill -TERM/-KILL -f -- "$STATE"` (mktemp unique
  per run — không đụng run khác); post-fix pgrep = 0.
- Fix khác trong phiên: root Makefile recipe tabs bị Edit tool chuyển
  spaces (make "missing separator" ngay khi chạy suite — bắt thật, fix
  `f6c146c`, bài học 2026-11-17 lặp lại).
- Docs: README minictd (CLI + service), GAPS A3 (reconcile BUILT — heartbeat
  P4 còn), roadmap GĐ2 mục 1 cập nhật + **GĐ3 đánh dấu "DỪNG + nhắc owner
  trước khi mở code"** per chỉ thị owner.

#### Mutants (negative controls)
- 9 mutants caught, mỗi mutant bị đúng test mới: M-CLI-1→MD20 (exec nuốt
  stdout), M-AUTO-1→MD21 (daemon bỏ autostart), M-RECONCILE-1→go test
  (early-return; attempt 1 compile-fail = mutant không sạch, làm lại), 
  M-DEPLOY-1→MD22 (render bỏ BINDIR); SCR clean-room: M0 no-start→107 FAIL
  toàn suite, M2 no-capdrop→CAPD1+SETUID2 ×2 suites (đúng thiết kế #38),
  M6 env-leak→MD10, M7 bind-follow→MD17, M8 seccomp-drift→MD0. 0 vacuous.

#### CVE Check
- `cve/daemon-service.md|.json`: cve-check rc=0 (19 hàng NVD live), **cve-gate
  rc=0 → 0 CVE mới ngoài watchlist**. Watchlist không thêm dòng (không CVE mới,
  không component mới trong rootfs).

#### Validation "không bị nhiễm"
- `make test` ×3 (working tree) + ×2 clean-room (`/tmp/scr-room` @ 6018023,
  `/tmp/scr-room2` @ b3c44ac): **9 suites, failed: none** (124 PASS + 7 SKIP
  env-blocked tun); `make test-web` 9/9; **`make test-minictd` 23/23 +
  `go test ./...` ok** — lần đầu chạy đủ local (Go cài mới).
- **CI runner @ 6018023 + @ b3c44ac**: build-test + perf-guard + unicode-guard
  **success**; log job 136133793798: MD20/MD21/MD22 **PASS trên runner** +
  mọi suite xanh (31/21/7/9/9/31) — run 44690169206.
- Perf: perf-guard **2.00 ms / 1188 kB / 2.04 MB** PASS (gate không đổi —
  cold path runtime không đụng); SCR bench N=50 p50 2.0 / p95 3.3.
- Host baseline: 3/3 sha256 khớp journal 2026-11-19 (đo lại sau retest —
  unchanged); 0 process minict/busybox còn sống sau suite (pgrep); bằng chứng
  `evidence/daemon-service.txt`.

#### PR + Pha B (lại)
- **#92** draft tạo @ `6018023`. Pha B vòng 1: CI xanh + SCR clean-room ×3
  (mutants ×5 caught) → **SIA CONCERNS** (1 medium: `ReconcileAll` TOCTOU —
  heal không giữ instance flock, saveState(stopped) có thể đè state running
  vừa write bởi Start đang chạy; 3 low) → fix(review) `b3c44ac`
  (per-instance lock 300ms timeout + skip-warn; sed-escape render; token
  perm warn; low #4 → issue **#93** `from-audit` p3) → CI xanh @ b3c44ac →
  SCR delta PASS (9 suites + 23/23 clean-room) → **SIA delta PASS**
  (`audit:pass`, 0 medium còn lại). Số vòng Pha B: **1 vòng**.

#### Pha C + E — SBG apply
- PR #92 flip ready (GraphQL) @ CI xanh + audit pass → **squash `7803730`**
  match-head-commit `b3c44ac` → #91 auto-close → nhánh DELETE_MERGED (204) →
  remote chỉ `main` + nhánh ledger này. G1–G8 PASS đủ cổng.

### Kế hoạch kế tiếp
- **GĐ2 tiếp**: mục 1 còn thiếu (systemd unit chạy THẬT dưới user manager —
  env-gated), CG1 quota cgroups v2 (S2), G3 Traefik preview, A2 `oci:` test,
  template (mục 2), storage overlayfs + snapshot (mục 3), S3–S6.
- #26/#27 docs p1; #75 port-collision web suite; from-audit #93/#77/#62/#61.
- **GD3 (Web UI + console) CHƯA chạm** — per chỉ thị owner: khi mọi mục GĐ2
  trước GD3 hoàn thành và GD3 sẵn sàng mở code → **DỪNG + nhắc owner trước
  khi viết dòng code UI đầu tiên** (ghi rõ trong roadmap mục GD3).

DAY PASS — 2026-11-20 Phiên 12: issue #91 done (merge #92 `7803730` @
`b3c44ac`, 1 vòng Pha B; SCR clean-room ×3 + delta + mutants ×9 caught;
SIA CONCERNS→PASS medium fixed), CVE gate rc=0 0 mới, suite 9 suites none
failed (dev 124 PASS + 7 SKIP; minictd 23/23 lần đầu local + runner; host
unchanged), main xanh `7803730`, remote chỉ `main`. Bug mới ghi nhận + fix:
daemon-leak SIGPIPE-before-EXIT-trap (catch-all cleanup) + Makefile tabs
lesson lặp lại.

## Phiên 13 — Orchestrator (2026-11-20, ZAI — tiếp tục theo chỉ thị owner "pull PROMPT làm tiếp, khi tới GD3 phải dừng nhắc")

### Pha A — Sync + SBG plan
- Sync main `d3b0605`; issues mở 14 → sau triage giữ nguyên tập; **PR mở 0**; CI main
  success; remote chỉ `main`. Host baseline đầu phiên 3/3 khớp Phiên 12 → UNCHANGED.
- SBG MODE=plan (`branch/plan.md` mục Phiên 13): remote chỉ `main`; 2 nhánh local
  thừa (ledger-12, feat/91) = DELETE_MERGED (headRefOid khớp PR #92/#94 MERGED) —
  xoá ở Pha E. 0 điều kiện dừng §0.6.
- Env: `gh` 2.63.2 cài binary + PAT auth (khác Phiên 11: REST-only); kernel 5.10.134
  (cgroups **v1** dev); Go 1.27.4; PID1=tini (không systemd user manager).

### Pha B/C (lượt 1) — không có PR nào đang mở (bỏ qua).

### Pha D — Issue #4 — CG1/S2 cgroups v2 resource enforcement (D48, GĐ2 mục 5)

#### Triage + chọn việc
- §8: không security p0/p1 / web p1 / netns p1 / perf breach → mục thấp nhất chưa
  xong của GĐ2 = CG1 (S2 — gap số 1 theo GAPS "ordered by integration risk",
  issue #4 p2 feature). Triage: assign + `status:in-progress` + comment thiết kế.
- **GD3 WATCH**: GĐ2 còn nhiều mục (systemd-real env-gated, CG1→làm hôm nay,
  G3 Traefik, A2 test, template mục 2, storage mục 3, port-forward mục 4,
  Landlock mục 6, đo mục 7, S3–S6) → **GD3 CHƯA tới** — đúng chỉ thị owner
  "khi tới GD3 phải dừng nhắc": không có việc nào của phiên này đưa roadmap
  qua GD3; nhắc owner trạng thái ở báo cáo §9.

#### Research (journal/2026-11-20/research.md Phiên 13 — 3 nguồn verified 200)
- systemd.io/CGROUP_DELEGATION: delegation chỉ EXPOSE controllers; delegatee
  tự enable qua cgroup.subtree_control; islands model + no-processes-in-inner-nodes.
- podman rootless.md: rootless resource limits CHỈ có trên cgroups v2.
- kernel cgroup-v2.rst: no-internal-process constraint + delegation containment
  (common-ancestor write access) — căn cứ launcher migrate-before-exec.

#### Development (nhánh `feat/4-cgroups-v2`, 12 commits, merge #95 `a9e1898`)
- `minictd/internal/minict/cgroup.go` (mới): cgResolveBase (override
  `--cgroup-root`/$MINICTD_CGROUP_ROOT phải chứa cgroup v2 + daemon phải nằm
  TRONG base — containment) > /proc/self/cgroup `0::`; manager cache once;
  ensureApps (self-park `minictd-daemon/` + enable 2 CẤP `+memory +cpu +pids`
  trên base VÀ apps subtree_control — memory.max chỉ xuất hiện ở con khi parent
  enabled — đo được CI vòng 4); cgPrepareInto (mkdir + limits TRƯỚC spawn);
  cleanup Stop/Remove.
- **migrate-before-exec launcher** (SIA-informed): sh launcher tự migrate
  `$$` vào instance cgroup rồi `exec minict` — payload SINH TRONG cgroup
  (post-spawn migration thua fork của minict — đo được CI run
  #44702053556/#44703016076: limits apply cho cgroup rỗng, OOM không xảy ra).
- Map: Memory→memory.max · CPUs→cpu.max (period 100000) · CPUShares→cpu.weight
  (tuyến tính 1024→100, D48) · PidsLimit→pids.max; **rlimit nproc per-uid
  DROP khi cgAvail** (S2 measured: runner nproc=16 giết shim — instance log
  "can't fork" — chính là gap S2); rlimit giữ khi không-enforce.
- Stats: cgroup reads ưu tiên (memory.current/pids.current/cpu.stat 250ms
  window), fallback /proc; MemLimitBytes đọc memory.max thật; `CgroupPath`
  persist trong state.json (cleanup qua Stop/Remove).
- **alive() heal KHÔNG rmdir cgroup** (SIA medium → RE-ASSESSED trên evidence
  run 44711850479: heal-time rmdir XÓA post-mortem counters trước khi đọc —
  docker-parity giữ cgroup exited tới Remove; dir rỗng 0 process).
- renderSpec(spec, ports, cgAvail): notes posture thật (enforced vs fallback).
- Suite `tests/test_cg.sh` CG-BOOT/CG-MEM/CG-OOM/CG-PIDS/CG-STATS/CG-RM vào
  `make test` (9→10 suites); self-build binary Go khi thiếu (CI chạy make test
  trước step minictd); asserts kernfs-aware (**cgroupfs files stat size=0
  luôn — `-s` fail ảo; cat-content thay thế**) + teeth kernel-recorded:
  CG-OOM peak==max + max-hits (tmpfs shmem charge-fail INLINE — ENOMEM, KHÔNG
  oom_kill — kernel truth đo được); CG-PIDS pids.peak≤cap + shell-death
  (busybox ash exit ngay khi fork bị chặn — instance log trích).
- Docs: GAPS S2 FIXED, README minictd section cgroups, roadmap mục 5 GĐ2 +
  "Đã xong" Phiên 13 + số suite 129 case.

#### CI lessons (11 run đỏ → root-cause đủ, không guess — minh bạch theo §6)
1. Run 1: test_cg.sh thiếu binary Go (CI make test chạy trước make minictd) → self-build.
2. Run 2: CG-BOOT assert notes instance chưa tồn tại (ordering) → split asserts.
3. Run 3: python multi-line `-c` fragile → single-line; bootstrap subshell mất
   CGROOT (biến subshell) → bỏ subshell; **nproc=16 per-uid giết shim trên
   runner** → rlimit nproc drop khi cgAvail (S2 evidence).
4. Run 4: controllers phải enable 2 cấp (base + apps) — memory.max chỉ xuất
   hiện ở con khi parent subtree_control enabled (kernel doc + đo được).
5. Run 5-6: post-spawn migration race → launcher migrate-before-exec;
   kernfs `-s` luôn FAIL (size=0) → cat-based asserts; OOM teeth sai cơ chế
   (shmem charge-fail ≠ oom_kill) → peak==max + max-hits.
6. Run 7-8: cgmem pid-parse tr -d dính pids; busybox `dd bs=1M` không đáng tin
   trên runner → bs=1024k; cg-pids assert running-at-0.5s sai thời điểm
   (shell chết ~50ms khi fork bị chặn là hành vi ĐÚNG) → poll exited + peak teeth.
7. SIA delta (store.go alive()): medium fix 4406c25 → RE-ASSESSED revert 3ac46bc
   (post-mortem counters) — 2 vòng SIA minh bạch trong comment.

#### Mutants (negative controls)
- 9 mutants caught tổng: M0 (24 FAIL), M2 (CAPD1+SETUID2 — SCR clean-room),
  M-CG-1 (skip controller enable), M-CG-2 (skip memory.max), M-CG-4 (limits
  empty), M-CG-5 (cleanup no-op) — go unit fake FS ×2 (dev + clean-room);
  M-CG-3 (launcher skip pid write) unit-immune — phủ bởi CG-MEM trên runner
  (PASS thật); M-CG-4b/M-CG-2b bằng SCR delta. 0 vacuous.

#### CVE Check
- `cve/cgroups-v2.md|.json`: cve-check rc=0 (19 rows; NVD transient timeout
  CVE-2024-21626 — not-scored, minh bạch); **cve-gate rc=0 → 0 CVE mới ngoài
  watchlist**. Watchlist không thêm dòng (không component mới trong rootfs).
- Bổ sung: commit kèm 2 file cve/daemon-service.{md,json} còn untracked từ
  Phiên 12 (bổ sung minh bạch trong PR #95).

#### Validation "không bị nhiễm"
- `make test` ×3 (dev working tree + SCR clean-room ×2 @ 3e5613d + delta
  @ 3ac46bc): **10 suites, failed: none** — dev 124 PASS + 13 SKIP env-gated
  (127 case); `make test-web` 9/9 ×4; `make test-minictd` 23/23 ×3 + go test ok.
- **CI runner @ 3ac46bc (run #44712685802)**: build-test + perf-guard +
  unicode-guard success — **127/127 thật trên runner** (CG 6/6 thật v2 —
  bootstrap sudo + migrate-before-exec + peak-based teeth đều PASS thật);
  main sau merge @ a9e1898 xanh.
- Host baseline: 3/3 sha256 khớp Phiên 12 (unchanged); listeners nền đã biết;
  0 process minict/busybox còn sống sau suite (pgrep self-match loại trừ);
  bằng chứng `evidence/cgroups-v2.txt`.
- Perf: perf-guard **2.15 ms / 1128 kB / 2.04 MB** PASS (gate không đổi);
  SCR bench độc lập 2.01/1244/2.04 — lệch < 2× (nhiễu runner).

#### PR + Pha B (lại)
- **#95** draft tạo @ 08fab27. Pha B vòng 1-8: CI đỏ ×9 với root-cause đo được
  mỗi vòng (minh bạch trong commit messages + comment suite) → CI xanh @
  3e5613d → SCR clean-room ×2 (mutants ×7 caught) → **SIA CONCERNS** (1 medium:
  heal path không reclaim cgroup) → fix(review) `4406c25` → CI đỏ (heal-cleanup
  phá post-mortem reads) → **SIA RE-ASSESSED** → revert `3ac46bc` kèm rationale
  → CI xanh @ 3ac46bc → SCR delta PASS (store.go comment-only + behavior
  identical) → **SIA delta PASS** (`audit:pass` label, 3 low chấp nhận có lý
  do). Số vòng Pha B: CI-fix 8 vòng + SIA 2 vòng — ghi minh bạch; mỗi vòng đều
  root-cause đo được, không guess, không xoá/skip test.

#### Pha C + E — SBG apply
- PR #95 flip ready (GraphQL markPullRequestReadyForReview — REST PATCH draft
  read-only) @ CI xanh + audit pass → **squash `a9e1898`** match-head-commit
  `3ac46bc` (full-SHA) → #4 auto-CLOSED → nhánh DELETE_MERGED (local + remote
  tự xoá) → local dọn ledger-12 + feat/91 → remote chỉ `main`.
- Ledger: dòng Phiên 13 appended qua PR ledger này. G1–G8 PASS đủ cổng.

### Kế hoạch kế tiếp
- **GĐ2 tiếp**: G3 Traefik preview + wake probe (mục 4), A2 `oci:` test
  (đã code chưa test), systemd unit chạy THẬT dưới user manager (env-gated —
  cần host có systemd thật), template (mục 2), storage overlayfs + snapshot
  (mục 3), Landlock (mục 6), đo control plane ≤ 30MB RSS (mục 7), S3–S6.
- #26/#27 docs p1; #75 port-collision web suite; from-audit #93/#77/#62/#61.
- **GD3 (Web UI + console) CHƯA chạm** — per chỉ thị owner: khi mọi mục GĐ2
  trước GD3 hoàn thành và GD3 sẵn sàng mở code → **DỪNG + nhắc owner trước
  khi viết dòng code UI đầu tiên** (ghi rõ trong roadmap mục GD3). Trạng thái
  GĐ2 hiện tại: mục 1 (daemon) còn systemd-real env-gated + G3 + A2-test +
  S3–S6; mục 2–7 chưa mở — còn tối thiểu nhiều phiên nữa mới tới điều kiện
  mở GD3.

DAY PASS — 2026-11-20 Phiên 13: issue #4 done (merge #95 `a9e1898` @ 3ac46bc;
SCR clean-room ×2 + delta; mutants ×9 caught; SIA CONCERNS → RE-ASSESSED →
PASS; CI-fix 8 vòng root-cause đủ — minh bạch), CVE gate rc=0 0 mới, suite
10 suites none failed (dev 124 PASS + 13 SKIP; runner 127/127 thật — CG 6/6
v2 thật lần đầu), host unchanged, main xanh `a9e1898`, remote chỉ `main`.

---

## Phiên 14 — Orchestrator §11 (ZAI: Pha A sync → Pha D #97 G3 preview routing → Pha B SCR/SIA → merge)

### Pha A — Sync + SBG (MODE=plan)
- main `8291da8` (ledger Phiên 13 #96); CI main xanh (ci + unicode-guard).
- PR mở đầu chu trình: 0 → Pha B/C rỗng. Remote chỉ `main`.
- plan.md Phiên 14 + SBG: không dừng (CI xanh, không security p0, host unchanged).
- Môi trường: gh 2.105.0 cài mới (~/.local/bin), Go 1.27.4 cài mới (~/.go/bin —
  GOPATH=~/.go tránh đụng GOROOT), kernel 5.10.134 dev, uid 1001.

### Issue xử lý
- **#97** (mới tạo phiên này — type:feature p1 area:minictd phase:2): G3 preview
  routing — Traefik file provider + wake probe (GĐ2 mục 1 residual, gộp mục 4;
  kế hoạch kế tiếp của Phiên 13). Tự assign + status:in-progress. Status: done
  (PR #98 — merge ở Pha C, chờ CI xanh).
- Triage 24h: không có issue mới chưa gán label (#93 from-audit phiên 13 đã có
  labels đầy đủ; các #55–#77 đã triage phiên trước).

### Research (Phiên 14 — 2 nguồn verified 200)
- **Traefik file provider docs v3.5** (raw.githubusercontent.com .../docs/content/providers/file.md):
  file provider nhận dynamic config qua directory `.yml`/`.toml`; shape
  `http.routers.{rule,entryPoints,service}` + `loadBalancer.servers[].url`.
- **Traefik pkg/provider/file/file.go v3.5.0**: directory mode → fsnotify watch
  từng entry; chỉ load `.yml`/`.toml` → atomic temp+rename an toàn (temp suffix
  không `.yml` không bao giờ load).
- Áp dụng: D49 — renderer YAML deterministic + hooks Run/Start/Stop/Remove +
  SyncAllRoutes (boot + reconcile) + Wake endpoint.

### Development
- Nhánh: `feat/97-g3-preview-routing` (commit `5ef0e51` → amend `e77001b`).
- Thay đổi: `minictd/internal/minict/routes.go` + `routes_test.go` (mới);
  hooks runtime.go ×4; `SyncAllRoutes` side-effect-free (measured: heal trong
  boot-sync phá `--autostart` MD21 → probe `pidAlive` thay `alive()` +
  anti-regression unit assert); `POST .../wake` + `HTTPClient.Wake` (non-contract
  extra) + CLI `sandbox wake`; flags `--traefik-dir/--traefik-domain`; systemd
  unit + install.sh; docs GAPS/API/README; MD23–MD25 (suite 23→26).
- Mutant dev-loop: MD21 đỏ lần đầu (boot-sync heal) — root cause đo được, sửa
  trước khi push (ghi D49 §Quyết định).
- Perf: cold_start=2.01 ms, rss=1184 kB, disk=2.04 MB — PASS mọi ngưỡng (gate
  không đổi; cold path minict không đụng).

### CVE Check
- Kernel: `5.10.134-013.15.kangaroo.al8.x86_64`
- `cve/g3-preview-routing.{md,json}`: cve-check rc=0 (17 rows) + **cve-gate rc=0
  → 0 CVE mới ngoài watchlist**; SCR clean-room chạy độc lập rc=0/rc=0.
- Watchlist không thêm dòng (Traefik KHÔNG được ship trong repo — operator
  binary, residual ghi rõ trong GAPS.md §G3).

### Validation "không bị nhiễm"
- `make test`: **10 suites, failed: none** (dev env-gated cg SKIP như baseline);
  `make test-web` 9/9; `make test-minictd` **26/26 ×2 không flake**.
- SCR clean-room (e77001b): R1–R7 đầy đủ — **26/26 ×2 byte-identical; mutants
  5/5 caught, 0 vacuous** (M1 no-write, M2 no-remove, M3 wake-lying→unit,
  M4 heal-in-sync→MD21+unit, M5 disabled-ignored); bench p50 2.3 ms / 1204 kB /
  2.04 MB; host 3/3 sha256 unchanged; CVE độc lập rc=0/rc=0.
- Host baseline: 3/3 sha256 khớp Phiên 13 (unchanged); listeners nền đã biết
  (12600/12700/19001/19005/19006/81/3000-next-server dev tooling); 0 process
  minict/busybox còn sống; bằng chứng `evidence/g3-preview-routing.txt`.

### PR + Pha B (lại)
- **#98** draft @ `e77001b` (B1: nhánh từ main mới nhất — up-to-date).
- SCR comment `=== SCR RETEST — e77001b ... ===` Verdict PASS (comment #7037620644).
- SIA comment Verdict **CONCERNS** (comment #7037645873, label `audit:concerns`):
  1 medium (B10 daily-log thiếu Phiên 14 — chính mục này), 1 low (MD25 không có
  negative control wake-lying ở tầng e2e → issue `from-audit`).
- B4: daily-log bổ sung (commit này) + issue from-audit; push → SCR chạy lại +
  SIA delta (chỉ journal) → flip ready khi CI xanh.

### Kế hoạch kế tiếp
- **GĐ2 tiếp**: A2 `oci:` test (đã code chưa test), S3–S6, systemd-real
  (env-gated), template (mục 2), storage overlayfs + snapshot (mục 3),
  port-forward pasta khai báo trong config (mục 4 residual — G3 đã nối routing
  side, pasta config-side còn), Landlock (mục 6), đo control plane ≤30MB RSS
  (mục 7).
- **GD3 (Web UI + console) CHƯA chạm** — per chỉ thị owner: khi mọi mục GĐ2
  hoàn thành và GD3 sẵn sàng mở code → DỪNG + nhắc owner trước khi viết dòng
  code UI đầu tiên. Trạng thái GĐ2: mục 1 còn A2-test + S3–S6 + systemd-real;
  mục 2–7 chưa mở — còn nhiều phiên trước điều kiện mở GD3.

### Pha C + E — kết thúc Phiên 14
- #98 flip ready (GraphQL) khi CI xanh ×4 checks @ `e8ec847` → **squash `23cf249`**
  match-head-commit → #97 auto-close; nhánh xoá (local + remote).
- SCR chạy lại 2 (e8ec847): 10 suites rc=0 + web rc=0 + minictd 26/26 rc=0; SIA
  re-assessed **PASS** (11/11 — B10 fixed); ledger #100 squash `ad7cd7b`;
  roadmap bổ sung #101 squash `a708092` (docs-only, minh bạch trong entry này).
- Remote sau Phiên 14: **chỉ `main`** @ `a708092`; local chỉ `main`.
- CI main: xanh (unicode-guard + ci + issue-triage success).

DAY PASS — 2026-11-20 Phiên 14: issue #97 done (merge #98 `23cf249` @ e8ec847;
SCR clean-room ×2 SHA — 26/26 ×2 byte-identical, mutants 5/5 caught 0 vacuous;
SIA CONCERNS → fix B10 → PASS; test-minictd 23→26 cases), CVE gate rc=0 ×2,
make test 10 suites none failed, host unchanged 3/3, perf 2.01/1184/2.04 PASS,
main xanh `a708092`, remote chỉ `main`.
