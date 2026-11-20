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
