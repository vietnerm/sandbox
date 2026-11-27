# Roadmap — sandbox-lab

> **Phân cấp nguồn sự thật** (issue #15): hướng tổng thể nằm ở `idea/manifest.md`
> (5 giai đoạn + DoD từng giai đoạn). File này là **kế hoạch chi tiết** — SDDA chọn
> việc theo mục thấp nhất chưa xong của giai đoạn hiện tại. `PROMPT.MD` chỉ trỏ
> vào file này, không giữ danh sách ưu tiên riêng.

## Nền tảng mục tiêu (từ manifest §5)

- Linux kernel **≥ 6.1**, **cgroups v2**, uid ≥ 1000 không sudo; `/dev/kvm` cho GĐ4.
- Runner CI (ubuntu-24.04) đã đạt mức này (kernel 6.x + cgroups v2); máy dev
  (kernel 5.10, cgroups v1, không KVM) chỉ dùng cho test nhanh/benchmark.
- Quy ước label: mục chặn vì máy dev gán `blocked-on-dev-env` — vẫn nằm trong
  kế hoạch, KHÔNG parked. Parked thật (lý do thiết kế, không phải môi trường)
  chỉ nằm ở mục "Parked thật" bên dưới.

## Quy trình hằng ngày (từ 17-11, bắt buộc)
1. A — Research kỹ thuật sandbox (Landlock+seccomp / bwrap / gVisor / Firecracker / Wasm)
2. B — Develop prototype (web service trong sandbox, đo startup/RAM)
3. C — CVE check: `python3 tools/cve-check.py --json journal/$(date +%F)/cve/<slug>.json`
   + kernel: `python3 tools/cve-kernel.py --kernels "$(uname -r),6.1,6.6,6.12" --watchlist docs/cve-watchlist.md --discover-days 2`
   (dải NVD cho kernel thật + dòng LTS đích; CVE kernel mới — owner 2026-11-23)
   + cập nhật `docs/cve-watchlist.md`
4. D — No-Compromise Validation: `make test` (isolation 35 + seccomp 25 + netns 7 +
   alpine 9 + debian 9 + escape-alpine 35 + cve-check tool 4 + pasta 11 + stack 8 + cg 6 = 149,
   pipefail thật — issue #14; cg suite từ #4/D48 2026-11-20, env-gated cgroups v2;
   seccomp 25 từ #139/D59 2026-11-23 (SE1-4 allowlist); dev: 136 PASS + 13 SKIP env-blocked)
   + `make test-web` + `make test-minictd` (47, MD0–MD46 — MD44–46 overlay D66 env-gated kernel ≥5.11) + lưu evidence vào
   `journal/YYYY-MM-DD/evidence/` + integrity check host
5. Ghi `journal/YYYY-MM-DD/daily-log.md` (quy tắc thư mục: PROMPT.MD §10)

## Đã xong
- [x] 2026-11-26 (phiên 34, SDDA): **GĐ2 mục 8 — daemon idle RSS ≤ 30 MB gate +
  destroy no-leak e2e (issue #171, D69)**. MD53: Run(webport) → Stop+Remove → 0 process
  leak (pgrep) + 0 mount leak (mountinfo grep) + dir gone. MD54: daemon idle RSS
  ≤ 30 MB (scrape /metrics + cross-check /proc VmRSS ±64 KB); dev baseline 6372 KB.
  **GĐ2 DoD đạt → DỪNG + nhắc owner trước khi mở GĐ3** (§8 chỉ thị owner).
- [x] 2026-11-26 (phiên 33, SDDA): **GĐ2 mục 7 — observability: log JSON + Prometheus
  metrics + /metrics endpoint (issue #168, D68)**. `--log-format text|json` (mặc định
  json, slog.NewJSONHandler + ReplaceAttr level lowercase); lifecycle events qua
  `eventLog` helper (instance/op/result/duration_ms); HTTP access log qua `requestLog`
  middleware (request_id + status + result). `metrics.go` renderer viết tay stdlib-only
  (8 instance + 3 daemon metrics, deterministic sort, fail-soft no-cgroup). `GET /metrics`
  cùng token auth trên listener sẵn có; `--metrics-listen` placeholder fail-closed (exit 1).
  MD50/MD51/MD52 + 10 Go unit (mutants caught). Residual: cgroup value-match e2e (test_cg.sh).
- [x] 2026-11-23 (phiên 24, SDDA): **GĐ2 mục 3a — seccomp allowlist theo template
  (issue #139, D59)**: đo thật 4 profile bằng ptrace probe (syscall-probe,
  --after-exec 2 tránh rò pha setup); filter floor+allowlist+EPERM cả payload lẫn
  exec (MD0 parity); RunSpec.SeccompProfile → MINICT_SPEC/env-file; template carry;
  SE1-4 + MD41-43 + 7 unit; 4 mutant đều đỏ. Perf không đổi (đo lại bench).
  CVE gate rc=0. Residual: opt-in, runner re-measure, arm64, backup spec. (#139)
- [x] 2026-11-23 (phiên 22, SDDA): **GĐ2 mục 3 sub — snapshot/backup/restore (issue #130,
  D57)**: `snapshot.go` mới — `tarGzTree` (Go archive/tar + compress/gzip stdlib,
  symlink-aware) + `extractGzTar` (**trusted path**: refuse `..`/absolute member paths,
  ALLOW absolute symlink TARGETS — busybox applet pattern `/bin/busybox`, khác secureUnpack
  D50 refuse abs symlinks cho untrusted OCI #109). gzip thay tar.zst (stdlib-only, no zstd
  dep — D20 no-vendor; zstd deferred). `store.go backupsDir()` (`<Root>/_backups/`, nameRe
  skip). `runtime.go Snapshot(name, backupName)` — yêu cầu STOPPED (fail-closed
  `ErrInstanceRunning` → 409), tarGzTree rootfs → `<backupsDir>/<name>.tar.gz`. `rootfs.go`
  case `backup:<name>` → extractGzTar. `server.go` route `POST /v1/sandboxes/{name}/snapshot`.
  Test MD40: run src (marker), snapshot RUNNING → 409, stop, snapshot OK, run dst
  `backup:md-snap` → exec sees marker. Perf không đổi. CVE gate rc=0. Residual:
  overlayfs rootless + idmapped mounts (phần (a) mục 3) env-gated kernel 5.10 dev
  (cần ≥5.11), separate phiên; zstd compression (gzip tạm, cần dep decision); list/remove-backup API.
  (#130)
- [x] 2026-11-23 (phiên 21, SDDA): **GĐ2 mục 2 — template (issue #127, D56)**:
  `rootfs.go prepareRootfs` thêm case `template:<name>` → copy `<Root>/_templates/<name>/rootfs`
  (cp -a, parity `base`/`dir:`). `runtime.go SaveTemplate(name, templateName)` — validate
  validName, yêu cầu instance STOPPED (fail-closed `ErrInstanceRunning` → 409, copy running
  rootfs races payload writes), `copyTree` rootfs + spec.json vào `<Root>/_templates/<name>/`.
  `runtime.go ListByNamePrefix` filter `nameRe` (skip `_templates` + non-instance dirs, không
  pollute List). `server.go` route `POST /v1/sandboxes/{name}/save-template` body
  `{"template":"<name>"}` → 200; `mapErr` `ErrInstanceRunning` → 409. Test MD39: run src (base,
  write marker), stop, save-template `md-tpl`; run dst `image=template:md-tpl` → exec sees
  marker (rootfs cloned); save-template on RUNNING → 409. Perf không đổi (cp -a + state dir,
  không runtime hot path). CVE gate rc=0. Residual: list/remove-template API chưa có (cleanup
  qua state dir); template spec.json lưu reference, chưa auto-apply defaults. (#127)
- [x] 2026-11-23 (phiên 20, SDDA): **S6 — webport collision detection up front (issue #124,
  D55, GĐ2 mục 1)**: daemon `store.go` thêm `portsHeldBy(except)` — scan tất cả instance qua
  `ListByNamePrefix`, filter `alive().State == running`, collect `port -> name` (trừ instance
  đang Run/Start); sentinel `ErrPortInUse` → `server.go mapErr` HTTP 409 Conflict (parity
  `ErrNotRunning`). `runtime.go Run` check sau `webportsOf`; `Start` re-check trước
  `spawnProxies` (instance khác có thể chiếm port khi này đang stopped). Stop/Remove giết
  webproxies (existing) → port released → Run/Start sau thành công. Test MD38: hai instance
  cùng `minict.webport` → Run thứ hai 409; stop A → B reuses port. Perf không đổi (scan
  state dir ~µs, không runtime path). CVE gate rc=0. Residual: external process hold port →
  webproxy Note (out of scope); no dynamic port allocation (operator-declared). (#124)
- [x] 2026-11-22 (phiên 19, SDDA): **S5 — spec rlimits cho exec'd processes (issue #121,
  D54, GĐ2 mục 1)**: `minict-exec.c` parse `MINICT_RLIMIT_<NAME>=soft[:hard]` từ env-file
  vào mảng riêng (KHÔNG setenv — R2 parity, rlimit config không leak vào cmd env),
  `apply_spec_rlimits()` trong child SAU `harden_exec_fds` (RLIMIT_CORE=0 giữ) TRƯỚC
  `drop_all_caps` (D23: raising hard limit cần CAP_SYS_RESOURCE held trong joined
  userns đến capdrop); daemon `exec.go` (setns mode) load `spec.json`, parse
  `spec.Ulimits` (nofile/nproc only — parity buildSpecMinict), append `MINICT_RLIMIT_*`
  vào env-file. PidsLimit NOT mapped cho exec (cgroups v2 đã bound exec'd process qua
  instance cgroup pids.max; không cgroups → per-uid nproc hazard S2, payload-only).
  Test MD37: `ulimit -n` trong exec == spec nofile; mutant skip-apply → MD37 đỏ.
  Perf cold ~2 ms / rss ~1.2 MB / disk ~2.05 MB (gate không đổi). CVE gate rc=0.
  Residual: none (nofile/nproc parity payload; resources khác do cgroups quản). (#121)
- [x] 2026-11-22 (phiên 18, SDDA): **S4 — unix-socket listener fail-closed (issue #116,
  D53, GĐ2 mục 1)**: `openUnixSocket()` (`minictd/cmd/minictd/listener.go`) — pathname
  unix socket, perms filesystem = transport security (podman rootless service model,
  unix(7)): socket **0600** Lstat-verified sau bind (umask-proof), dir **0700**
  (chmod nếu lỏng; chmod lỗi/mode cuối sai → die), refuse symlink + non-socket file
  (posture D31 #41), replace stale socket, unlink on clean shutdown; `--listen ""`
  tắt TCP (unix-only posture — ss-pid assert), cả hai rỗng → die "no listener";
  token auth GIỮ NGUYÊN trên mọi transport (R3 — same-uid vẫn connect được, MD36
  401-assert); `contract.NewUnixHTTPClient` + CLI `--socket`/`$MINICTD_SOCKET`
  (thắng `--addr`); unit systemd thêm `--unix-socket` + install.sh chmod state dir.
  Tests: unit 6 case (stale socket tạo bằng raw syscall.Bind — Go net.Listen tự
  unlink khi Close, không tạo được stale giả) + MD35/MD36 (test-minictd 34→36);
  mutants M1 (socket-0666)/M2 (skip-chmod-dir) → MD35 đỏ, M-auth → MD2+MD36 đỏ,
  M-tcp → MD36 đỏ, M3 masked-documented (nhánh non-socket vẫn refuse — defense in
  depth). Residual: TLS cross-host cho multi-node thật (unix socket local-only);
  TOCTOU socket-path swap same-uid-only → #119 (from-audit). SCR PASS ×2 SHA
  (suites ×2 rc=0, mutants 4/4 + M0 collapse, bench p50 1.9/1152/~2.1, host 3/3);
  SIA CONCERNS → fix daily-log + residuals → PASS delta; CI 4/4 ×2 SHA; perf
  1.88 ms / 1164 kB / 2.05 MB PASS; CVE gate rc=0; squash `336373a` @ `9c4cf9c`;
  #116 auto-close. (#116)
- [x] 2026-11-22 (phiên 17, SDDA): **S3 — bind :ro recursive enforcement (issue #113,
  D52, GĐ2 mục 1)**: `apply_spec_binds()` (`sandbox/minict.c`) — `mount_setattr(AT_RECURSIVE,
  MOUNT_ATTR_RDONLY)` (kernel ≥ 5.12, syscall 442 khai tay x86_64) thay remount top-only;
  ENOSYS (kernel < 5.12, đo được trên dev 5.10.134) → per-submount bind-remount từ
  enumeration `/proc/self/mountinfo` host-side **pre-unshare** (cửa sổ duy nhất còn /proc —
  D39); mountinfo không đọc được hoặc > 16 submount → **fail-closed die**. Bug tái hiện
  đo được trước vá (tmpfs submount vẫn ghi được sau khi top EROFS) rồi xác nhận submount
  EROFS sau vá trên cùng harness (`unshare --user --map-root-user --mount` — submount
  do userns cha tạo được rbind-clone bởi userns con, probe rec3 + minict thật). Test
  BIND1 (cây phẳng :ro) / BIND2 (submount ro — assert giống nhau 2 kernel, tên cơ chế
  in trong case) / BIND3 (rw control chống-vacuous) vào isolation suite (31→34);
  mutants M-A/M-B/M-C caught (BIND3 xanh xuyên suốt — 0 vacuous, M-C pin nhánh
  fail-closed); residual: nhánh mount_setattr chỉ đo e2e trên runner ≥ 5.12.
  minictd hưởng qua spec (MD8 không regress). (#113)
- [x] 2026-11-21 (phiên 16, SDDA): **A2 e2e — `oci:` với crane thật (issue #103, D51,
  GĐ2 mục 1 residual; + fix #108)**: crane v0.22.1 pin+sha256 (`tools/fetch-crane.sh`,
  D37 pattern) + registry in-memory 127.0.0.1 (`tools/ocireg`, module riêng — go.mod
  minictd không đổi) ⇒ e2e `oci:` HOÀN TOÀN OFFLINE (không docker.io rate-limit flake);
  MD28–MD34 (suite 28→34): fetch-pin, harness+push manifest-verified, boot+exec marker
  (bind-volume host-side — image minimal chỉ có sh/applet links relative), bad-ref
  fail-closed, hostile `../` refuse+wiped (2 lớp refuse đo được: crane upstream
  "unsafe tar path" trước-khi-viết + secureUnpack mid-stream), refusal >64 KB → 500
  FAST (MD34 pin **fix #108**: `unpackOCI` drain trước Wait — refuse giữa stream từng
  deadlock `crane.Wait()` tới ctx-cancel, đo bằng goroutine dump); remote pull thật
  = opt-in `MINICTD_OCI_REMOTE_REF` (MD33, SKIP unset — flake class documented);
  residual #109: absolute symlink targets bị refuse (rootfs của repo: 271 applet
  links `/bin/busybox` — không boot được qua `oci:`) → **resolved D70 (owner
  2026-11-26): giữ refusal D50 nguyên trạng, document yêu cầu relative symlinks
  trong README, đóng #109**;
  suite cleanup leak fix (ocireg RPID mất trong subshell → pkill theo argv).
  Mutants: M1 (bỏ drain) MD34 đỏ; M2b (drop refusal errors) MD34 đỏ qua provenance
  assert; M3 (sha mismatch) exit 1 verified; measured lessons: crane export không đọc
  tarball local, `io.Discard` không phải Reader (mutant M1 compile-fail), harness
  registry chết giữa blob-serve khi process-group bị dọn giữa các tool-call.
  SCR/SIA trong phiên; CVE gate rc=0; make test 10 suites none-failed; perf
  2.16 ms / 1212 kB / 2.04 MB PASS; host 3/3 unchanged. (#103/#108) — PR đang mở.
- [x] 2026-11-21 (phiên 15, SDDA): **Unpack boundary hardening — secureUnpack thay host tar
  (issue #104, D50, GĐ2 mục 1 residual — phát lộ từ research #103/A2)**:
  `minictd/internal/minict/unpack.go` — Go stdlib `secureUnpack`: openat
  O_NOFOLLOW per-component (D31 pattern), reject `..`/absolute member +
  link target ngoài, duplicates refuse (O_EXCL), `stripSpecialBits` (pure —
  setuid/setgid/sticky), FIFO mknodat S_IFIFO rootless-safe, char/block
  refuse (mknod EPERM); `unpackOCI` (crane pipe → extractor) + `tar:` path
  cùng extractor — **host tar không còn trong đường untrusted** (fail-closed
  không còn phụ thuộc GNU tar 1.35 incidental; busybox/GNU cũ extract `..`
  + write-through symlink = ghi host-side — ma trận đo
  `journal/2026-11-21/research.md`); unit matrix 14 test + MD26/MD27
  (test-minictd 26→28); measured lessons: `openat(O_CREAT|O_DIRECTORY)`
  =ENOTDIR trên 5.10, FIFO-open trick ra regular file, dev-LSM strip suid +
  giết SYS_SYMLINK (SYS_SYMLINKAT chạy) ⇒ test assert provenance message;
  mutant M1(4 đỏ)/M2(2)/M4(1)/M5(1)/M6(1) caught, M3 masked-local documented
  (property assert trực tiếp); SCR PASS ×2 SHA (28/28 ×2 byte-stable;
  bench 2.0/1188/2.04; host 3/3); SIA PASS (2 low → fix trong vòng); CVE
  gate rc=0; squash `22831da` @ 92b56e0; #104 auto-close. A2 e2e (crane
  thật) vẫn mở — **#103**. (#104)
- [x] 2026-11-20 (phiên 14, SDDA): **G3 — preview routing: Traefik file provider + wake probe
  (issue #97, D49, GĐ2 mục 1 residual / mục 4 routing-side)**: `minictd/internal/minict/routes.go`
  — renderer YAML deterministic (stdlib-only) + atomic temp+rename (Traefik v3.5 directory
  mode load mọi .yml + fsnotify — 2 nguồn verified); hooks Run/Start (+file) / Stop/Remove
  (−file); `SyncAllRoutes` boot + reconcile heal, side-effect-free trên state.json (measured:
  heal trong boot-sync phá --autostart wake-recreate, MD21 đỏ → probe pidAlive + anti-regression
  unit assert); `POST /v1/sandboxes/{name}/wake` + CLI `sandbox wake` + HTTPClient.Wake
  (non-contract extra, Runtime 14-op giữ nguyên); flags `--traefik-dir` (off = fail-soft CG1
  pattern) / `--traefik-domain` (mặc định localhost); systemd unit + install.sh; namespace file
  `sandbox-` không đụng file operator; honor traefik.enable=false (full docker-label DSL =
  residual GAPS.md); e2e MD23–MD25 (test-minictd 23→26) + go unit routes_test.go; SCR PASS ×2
  SHA (26/26 ×2 byte-identical; mutants 5/5 caught 0 vacuous; bench 2.3ms/1204kB/2.04MB PASS;
  host 3/3 unchanged); SIA CONCERNS (B10 daily-log) → fix → **PASS** (1 low → #99); CVE gate
  rc=0; perf 2.01ms/1184kB/2.04MB PASS; squash `23cf249` @ e8ec847; #97 auto-close. (#97)
- [x] 2026-11-20 (phiên 13, SDDA): **CG1/S2 — cgroups v2 resource enforcement
  (issue #4, D48, GĐ2 mục 5)**: `minictd/internal/minict/cgroup.go` — islands
  model (daemon self-park `minictd-daemon/` + enable `+memory +cpu +pids` trên
  base subtree_control + instance tree `minictd-apps/sandbox-<name>`), shim-pid
  migration sau spawn (containment-safe), mapping Memory→memory.max /
  CPUs→cpu.max / CPUShares→cpu.weight / PidsLimit→pids.max, Stats ưu tiên
  cgroup reads (memory.current/pids.current/cpu.stat 250ms window) fallback
  /proc, cleanup Stop/Remove/heal; `CgroupPath` persist trong state.json;
  fail-soft khi không có base v2 (note lý do đo được); 13 Go unit test trên
  fake cgroup FS + shell suite `tests/test_cg.sh` CG0–CG5 env-gated vào
  `make test` (9→10 suites, dev 127 PASS + 13 SKIP); 5/5 mutant caught;
  perf 2.15ms/1128kB/2.04MB PASS (gate không đổi); CVE gate rc=0. (#4)
- [x] 2026-11-20 (phiên 11, SDDA): **Debian rootfs — GĐ1 mục 1 residual cuối (issue #88, D46)**:
  rootfs bookworm-slim 12.15 amd64 từ debuerreotype/docker-debian-artifacts
  (OCI layer digest pin `2a7a0929…`, verified tải+sha256 cùng phiên) qua
  `tools/fetch-debian-rootfs.sh` fail-closed (D37 pattern) — **gitignored**
  (83.3 MB unpacked, không track); `minict.c` sanity `/bin/sh` executable
  resolve symlink thay `/bin/busybox` hardcode (blocker đo được: debian không
  có busybox ⇒ `minict: no busybox`); seccomp blocklist KHÔNG đụng (MD0 sync
  giữ nguyên); suite mới `tests/test_debian.sh` 8 case (DB1 boot dash
  anti-busybox, DB2 dpkg glibc, DB3 /proc env-aware D39, DB4 ownership
  AL6-pattern probe `/var/tmp` — đo được: **/tmp là tmpfs** trong sandbox,
  DB5 chown EINVAL AL7-pattern, DB6 control no-pasta apt unreachable
  PT4C-pattern + wait_for_exit, DB7 apt update+install tree ONLINE signature-
  verified 2-step anti-vacuous #80 — env-gated tun, DB8 pristine manifest hash
  AL9-pattern) vào `make test` (9 suites, dev 123 PASS + 7 SKIP; gate busybox
  2.04 MB không đổi — debian đo supplementary); apt chạy HTTP deb.debian.org +
  DNS D42, `_apt` unmapped warning documented; cve-watchlist thêm apt 2.6.1 /
  dpkg 1.21.23 / glibc 2.36 (OSV: 0 CVE mở). (#88)
- [x] 2026-11-19 (phiên 4, phiên thử của Claude theo PROMPT.MD §11): **/proc trong pidns
  (issue #31, D39)** — root cause đo được: kernel chỉ cho userns mount procfs khi ns còn
  một procfs fully-visible (`mount_too_revealing`); minict detach old root TRƯỚC khi
  mount → EPERM mọi nơi. Sửa: shim pidns-1 mount `/proc` (nosuid,nodev,noexec) rồi mới
  detach old root (fail-closed). A/B kernel 6.18: main EPERM → fix mount được, payload
  thấy `Pid: 2`. Test env-aware (probe host `unshare --mount-proc`): PID2 mới + PROC1–4
  + AL4 thật; isolation 29→31; 4 mutant đều bị bắt (PID2 hết vacuous với M4). Chờ CI
  runner xác nhận. (#31)
- [x] 2026-11-19 (phiên 3, SDDA): **pasta egress + port-forward prototype (issue #57,
  mục 3 GĐ1)**: `tools/fetch-pasta.sh` pin passt snapshot 2026_10_02.cba3570 +
  sha256 fail-closed + `make static` (không vendor — D20 pattern, D37);
  `minict.sh up --pasta` gắn pasta vào netns instance SAU khi sống (`--userns`
  tường minh — trap: `--netns` đơn lẻ ⇒ --netns-only ⇒ EPERM, conf.c
  conf_pasta_ns; join rootless bằng euid==owner); payload override
  `up <name> [-- payload]` (exec mở ns set mới — không dùng được cho pasta);
  `MINICT_PASTA_FWD` port-forward; suite `tests/test_pasta.sh` 7 case vào
  `make test` (7 suites, 105 case: PT1/PT2/NET11C mọi nơi + NET10/11/12/AB1
  SKIP(env-blocked) khi không tun — PASS thật trên runner 7/7); egress
  deterministic qua gateway-NAT (không internet/DNS); CI nuôi suite 4 vòng
  đỏ→xanh bắt lỗi thật (payload /proc-dependency, race attach, NET11C vacuous
  dưới M0, PT2 superstring); SCR ×5 + mutant ×3 caught + SIA PASS; perf không
  đổi (pasta out-of-band). D16 unix-socket bridge = fallback loopback-only.
  (#57/#59)
- [x] 2026-11-19 (phiên 2, SDDA — chu trình tăng tốc theo chỉ thị owner
  "đẩy nhanh tiến độ để kịp qua giai đoạn 2", D35): **(a) #44 — cve-check.py
  đỡ crash khi NVD lỗi transient** (D35: parse_nvd defensive + render_cvss
  error-first; suite tool offline 4 case + control CV4; mutant caught);
  **(b) residual #5 — trọn isolation suite trên rootfs distro thật (D36)**:
  29 case chạy trên alpine qua `ISOLATION_ROOTFS`, suite thứ 5 của
  `make test` (98 tổng); phát hiện + sửa CHROOT1 vacuous trên alpine
  (rc=127 vì thiếu /bin/chroot symlink — class "expect=fail + lệnh không có
  trong rootfs"); flake #55 (rootfs-alpine thiếu empty-dir 2/4 build) —
  mitigation mkdir vô điều kiện + self-heal; **(c) D34 pasta research** —
  `/dev/net/tun` không có ở máy dev (đo được) → pasta env-blocked dev,
  thiết kế + issue #57 cho prototype CI-gated phiên kế; triage #52 (cve-daily
  p0 — 0 CVE mới ngoài watchlist, giống #34) + mở chore #56. Merge:
  #53 → `07d016e` (LỖI QUY TRÌNH: thiếu SCR/SIA khi merge — khắc phục bằng
  post-hoc audit sau-merge, minh bạch trong PR #53 + daily-log), #54 →
  `c2b9f0e` (đủ cổng). (#44/#53, #5-residual/#54)
- [x] 2026-11-19 (phiên 1, SDDA): **deep escape PoC #5 — fd/core hardening + 7 test**
  (issue #5, type:security p2, mục 2 GĐ1): `harden_payload_fds()` trong
  `sandbox/minict.c` payload path — `close_range(3, ~0U, 0)` (syscall 436,
  fallback loop có trần cho kernel < 5.9) + `RLIMIT_CORE=0` (trước vá: payload
  kế thừa MỌI fd không-CLOEXEC của caller qua fork+exec — bypass pivot_root,
  lớp TOCTOU CVE-2018-15664; core dump ghi vào rootfs khi crash);
  `minict-exec.c` same hardening (filter không đổi — MD0 PASS). Test mới
  isolation 23→29 (FD1 hai giai đoạn chống-vacuous, PROC2, CHROOT1, SETUID2
  host-/proc kiểu D32, CORE1 vacuous-guard, CORE2) + seccomp 20→21 (SC20
  ERRNO-mode survival); negative control clean-room 6/6 bắt đúng
  (fd→FD1+CORE1; core→CORE1 duy nhất; capdrop→CAPD1+CHROOT1+SETUID2; nnp→
  collapse; trap→SC20; M0→collapse). Residual khai đúng: piped core_pattern
  (core(5) bỏ qua RLIMIT khi pattern pipe) → GĐ2; PROC2/CORE2 env-dependent;
  CHROOT1 cùng class FS4/FS5 (expect=fail khi sandbox chết). Perf 1.86 ms /
  1160 kB / 2.04 MB PASS. SCR ×3 + SIA PASS; PR #47 (draft, PAT không flip
  được) → PR #48 non-draft, squash `70921c2`; #5 mở lại theo §6 chờ
  `verified`. (#5/#48)
- [x] 2026-11-18 (phiên 4, SDDA): **suite hardening #38 — assertion cơ chế
  capdrop/pidns** (issue #38, type:chore from-audit): CAPD1 (CapEff=0 shim+payload
  qua host /proc) + PID3 (NSpid lồng shim=1/payload=2) — bắt mutant M2 (trước
  đây vacuous 21/21) và M4 (phát hiện PID1-only → PID1+PID3); FS4/FS5/PID2/SIG1
  giữ nguyên + comment dẫn D32; từ chối MINICT_NO_SECCOMP (mở mặt tấn công);
  isolation 21→23, tổng repo 87; SCR clean-room ×2 + SIA PASS; squash `a7ef9c7`.
- [x] 2026-11-18 (phiên 3, SDDA): **S1 — webproxy host bridge kháng symlink**
  (issue #41, type:security p1): `tools/webproxy.py` resolve relay socket
  fail-closed per-connection — openat `O_NOFOLLOW` từng thành phần + `O_PATH`/
  `S_ISSOCK` thành phần cuối + connect qua `/proc/self/fd/<n>` pin inode
  (TOCTOU-free; pattern CVE-2018-15664/CWE-59; 4 nguồn verified, D31); callers
  `minict.sh` + minictd `spawnProxies` đổi `--root/--sock`; test WEB7–WEB9
  (suite web 6→9) + mutant matrix M1/M2/M3 — mỗi mutant bị đúng test nó bắt;
  regression 4 suite 56/56 ×2; perf 1.80 ms / 1204 kB / 2.04 MB PASS (host-side,
  cold path không đổi); `minictd/docs/GAPS.md` S1 FIXED (residual: Go bridge
  dùng pattern tương tự khi GĐ2; payload-rebind same-path = low, không escape).
  (#41/#42)
- [x] 2026-11-18 (owner bàn giao, tích hợp + review): **minictd — GĐ2 seed gộp sớm**
  (ngoại lệ manifest §8, owner chốt D27: **GĐ1 vẫn là giai đoạn hiện tại**,
  minictd chỉ sửa bảo mật + giữ xanh). Gồm: `MINICT_SPEC` trong `sandbox/minict.c`
  (D23: hostname/env/bind[:ro]/tmpfs/shm/ulimit, fail-closed) + multi-webport (D24);
  `minict-exec` exec bằng **setns** vào instance đang chạy + cài lại seccomp (D25 —
  gate GĐ2 đo được trên kernel 6.18 uid 1001); daemon Go `minictd/` (14 op contract
  sandboxd, state machine, flock, HTTP API + client). Review tích hợp sửa 7 lỗi
  R1–R7 (path traversal tên, rò env daemon vào sandbox, auth mặc định tắt, bind
  theo symlink ra host…, D26). Suite mới `tests/test_minictd.sh` 20 case
  (`make test-minictd`, có trong CI); cùng suite trên gói gốc → 6 đỏ (negative
  control). Regression: 4 suite cũ 56/56 + web 6/6, cold start +0.1 ms (nhiễu).
  Còn mở: CG1, G3, A2, S1–S6 (`minictd/docs/GAPS.md`).
- [x] 2026-11-17 (phiên 8, SDDA): **rootfs Alpine minirootfs 3.22.8** — distro thật
  boot được trong minict giữ nguyên isolation (D20): `sandbox/mkrootfs-alpine.sh`
  (pin version + sha256 verify CDN, tar non-root `--no-same-owner` = fakeroot tự
  nhiên của single-uid mapping — đo AL6: `0:0` trong ns ↔ host-uid); suite mới
  `tests/test_alpine.sh` 8 case (AL1–AL8) vào `make test` (4 suite, 52 case);
  apk offline signature-verified chạy trong sandbox (AL5, `acpid-openrc`);
  **gói hỏng vì chown foreign-uid: đo được** (AL7 — chown `100:100` EPERM
  post-capdrop; nginx/postgres-type sẽ hỏng ở bước adduser — phương án (a) root-in-ns,
  (b) pre-bake ownership, (c) idmapped mounts GĐ2 — chi tiết D20);
  /proc trong pidns env-aware — **ENV FACT đo được phiên này: mount /proc
  EPERM ở cả máy dev VÀ runner azure kernel 7.0** (run 44039937935) — AL4
  assert fail-soft đúng cả hai; root-cause runner = follow-up; perf: cold
  1.83ms, rss 1087KB, disk 8.6MB — PASS mọi ngưỡng;
  apk add online chờ egress pasta (mục 4); apk-tools quét CVE (2 CVE cũ đã vá,
  0 mở) → `docs/cve-watchlist.md` có mục apk-tools. (#23)
- [x] 2026-11-17 (phiên 7, SDDA): **SIGTERM/SIGINT delivered qua pidns-1 signal
  shim** — root cause #6 là semantics chuẩn pidns-init (pid_namespaces(7): init
  chỉ nhận signal có handler; SIGKILL/SIGSTOP bị ép qua), KHÔNG phải host
  seccomp (giả thuyết D4 sửa — SIG4 control chứng minh); shim cài
  {TERM,INT,HUP,QUIT,USR1,USR2,WINCH} + forward payload + mirror exit
  128+sig; wrapper gửi signal vào shim pid (docker semantics), bỏ group-kill;
  fix 2 bẫy SIG_IGN sống sót fork+exec (forwarders cài trước fork pidns +
  payload reset DFL pre-exec); isolation 17 → 20 (SIG2/SIG3/SIG4), PID1 =
  "payload is PID 2"; teardown D4 giữ nguyên fail-safe. A/B: TERM swallowed →
  delivered rc=143; cold start +0.17ms (1.66→1.83, N=40); perf-check PASS
  1.85/1160/2.02. (D19 `journal/2026-11-17/decisions/sigterm.md`, evidence
  `journal/2026-11-17/evidence/sigterm-ab.txt`)
- [x] 2026-11-17 (phiên 6): **CI xanh thật** — pipefail (`defaults: run: shell: bash`
  cho ci.yml + cve-daily.yml), SIG1 NOFORK-aware (busybox Ubuntu 24.04 chạy sleep
  NOFORK — tái hiện bằng chính .deb của runner), UID1 overflow-aware (supplementary
  groups hiển thị 65534 — setgroups deny-lock), `make test` chạy đủ 3 suite + tổng
  hợp exit code (`tests/run-suites.sh`), preflight `ss` cho WEB2. CI run trên PR:
  isolation 17/17 + **seccomp 20/20 lần đầu chạy trên CI** + netns 7/7 + web 6/6 +
  perf 1.99ms/3480kB/2.12MB PASS. (#14/#16)
- [x] 2026-11-17 (phiên 5, SDDA): **netns isolation mặc định BẬT** — CLONE_NEWNET
  cho mọi run (fail-closed, MINICT_NO_NETNS=1 opt-out, D17/D18); `lo` bring-up sau
  uid_map (bẫy uid_map rỗng = 0 caps, đo được); web vẫn reachable qua unix-socket
  bridge host webproxy.py → in-sandbox relay trên rootfs path (D16, precedent
  RootlessKit --port-driver); seccomp thêm block socket(AF_NETLINK) (D15) + 3 case
  SC17–SC19 (control A/B netlink/tcp); suite mới tests/test_netns.sh 7/7 (NS1–NS7,
  gồm control NS2); NET1 từ xfail → pass thật (NET1/NET2); overhead +0.28ms
  (N=40 interleaved, perf-check PASS 1.90ms/1032kB/2.03MB); fix quay lại Makefile
  tabs (GNU Make 4.4.1). SBG: gộp PR #11 (prompts/BRANCH_GATE_PROMPT.md lên repo theo
  lệnh owner) trước phiên dev.
- [x] 2026-11-17 (phiên 4, SDDA): **seccomp-bpf blocklist mặc định bật** — 32 syscall +
  arg-check clone(CLONE_NEW*)/socket(AF_ALG), clone3→ENOSYS (D11–D13); 17 test mới
  `tests/test_seccomp.sh` gồm 4 control A/B (chứng minh EPERM từ filter mình, không phải
  host); overhead đo được +0.2ms cold start (A/B N=40, evidence `journal/2026-11-17/evidence/seccomp-ab.txt`);
  bài học số syscall phải grep unistd_64.h (D14 — setns là 308, không phải 306). SBG:
  gộp PR #9, dọn 2 nhánh đã gộp, ledger `docs/branch-ledger.md` tạo mới.
- [x] 2026-11-17 (phiên 3, SDDA): **web server trong sandbox** — `minict.sh web` +
  busybox httpd static, bind 127.0.0.1 (D6/D7), `tests/test_web.sh` 6/6 PASS,
  root Makefile + 5 GitHub Actions workflows + PR template + `tools/perf-check.sh`
  (perf-guard gate) + cve-check CLI `--kernel/--watchlist/--format md` (D9 — chỉ parse
  dòng bảng, bài học false positive CVE-2022-0316). CVE check 16 CVE (NVD): 5 CVE
  kernel mới 2026 + 5 CVE busybox httpd mới — tất cả đo được "không với tới" hoặc
  host-side (issue #8 track backport). Research: veth trong userns tạo được nhưng
  không nối host rootless; netns phải đi cùng seccomp (D10).
- [x] 2026-11-17: **minict v0.1** — mini-container rootless (userns+mountns+pidns+utsns+ipcns,
  pivot_root, capdrop, NNP, PDEATHSIG, group-kill), busybox rootfs builder, lifecycle CLI
  (up/exec/shell/down/ps), 16 test isolation, benchmark đầy đủ. Cold start 1.2ms, RAM ~1.1MB, disk 2MB.

## Đang làm
- (trống)

## Giai đoạn 1 — minict runtime (XONG — DoD 6/6, manifest tick 2026-11-23)

> Hướng dự án (manifest, chốt 2026-11-17): **"Proxmox nhẹ", rootless tuyệt đối**.
> GĐ1 là nền runtime; lõi "Proxmox" (API, template, snapshot, quota) là GĐ2–3.

1. **Rootfs distro thật** (mới): Alpine minirootfs trước (nhỏ, musl), Debian sau.
   Đo trên runner (kernel ≥ 6.1): mount `/proc` trong pidns, init tối thiểu
   (tini/busybox init — shim signal của minict (D19) là cùng pattern),
   `apk add nginx` / `apt-get install` dưới **single-uid
   mapping** (manifest §5: không dùng `newuidmap` setuid). Ghi rõ gói nào hỏng
   vì `chown` sang uid khác và phương án (bỏ qua / giả lập).
   → Alpine **xong phiên 8** (xem "Đã xong"); `/proc` trong pidns **sửa 2026-11-19
   phiên 4 (#31, D39)**, chờ CI runner; DNS cho pasta **xong 2026-11-19 phiên 7
   (#74, D42 — `--dns-forward` + inject resolv.conf, PT3/PT3C)**; `apk add` online
   **xong 2026-11-19 phiên 7 (#79, D43 — PT4/PT4C)**; Debian rootfs **xong
   2026-11-20 phiên 11 (#88, D46 — bookworm-slim fetch pinned+sha256 gitignored +
   sanity `/bin/sh` thay busybox + DB1–DB8; apt online HTTP qua pasta, DB7 runner).
   Residual mục 1: HTTPS runtime cho apt/apk (cài ca-certificates — cùng class,
   khi cần); các point-release mới của bookworm/alpine chỉ là maintenance.
2. **chroot-escape test sâu hơn** (issue #5): fd-based escape, /proc/self/root,
   setuid binary trong rootfs (NNP chặn), core dump leak; case seccomp SIGSYS;
   chạy lại cả suite trên rootfs distro thật (có `/proc`, nhiều mặt tấn công hơn busybox).
3. ~~Connectivity qua pasta/slirp4netns~~ — **xong phiên 3** (prototype CI-gated,
   issue #57): egress + port-forward qua pasta; DNS **xong phiên 7 (#74, D42 —
   `--dns-forward` GW + `nameserver` inject, case PT3/PT3C)**; `apk add online`
   là residual (cần HTTPS runtime cho apk-tools). D16 chỉ còn fallback loopback-only.
4. **Stack web thật trong 1 instance**: nginx/caddy + API + SQLite trên rootfs
   distro; `./sandbox up|down|bench` một lệnh; cold start stack < 5 s;
   signal lifecycle graceful đã có từ phiên 7 (D19).
   → **Stack dùng được phiên 10 (#85, D45)**: nginx + fcgiwrap + CGI shell +
   SQLite trong 1 instance alpine (pre-install tại build time, `--no-scripts`
   — D45), một lệnh `./sandbox.sh up|down|bench` (`./sandbox` không khả thi —
   `sandbox/` là thư mục từ PoC); cold start stack đo được **median 127 ms**
   (ngưỡng < 5 s, dư ~39×); suite `tests/test_stack.sh` ST1–ST7 vào `make test`
   (dev 121 case: 115 PASS + 6 SKIP env-blocked); residual: caddy thực
   (hiện nginx), API production-grade (thay CGI shell bằng runtime thật —
   GĐ2), SQLite WAL/concurrency tuning.

## Giai đoạn 2 — Control plane + storage (HIỆN TẠI — owner chốt 2026-11-23, manifest §8; ZAI bắt đầu từ phiên 12 ngày 2026-11-20)

> Luật D27 (owner, 2026-11-18) hết hiệu lực: owner chốt GĐ2 là giai đoạn hiện tại
> ngày 2026-11-23 và manifest §8 đã ghi lại.

0. **GATE research:** exec vào instance đang chạy rootless — **đo được 2026-11-18**
   (D25, `minict-exec` setns, MD5–MD13); xác nhận trên runner CI: **đạt** —
   `test-minictd` 20/20 chạy đều mỗi PR từ run #206 (những run gần nhất giữ
   20/20 + go test ok).
1. Daemon user-level + REST API: **seed có** (`minictd`, state machine
   creating/running/stopped, flock, liveness chống pid-reuse). **Phiên 12
   (#91, D47): CLI `sandbox` (contract.HTTPClient thin shell) + systemd
   --user unit (deploy/install.sh render, systemd-analyze verify MD22) +
   `--autostart` (reboot recovery, MD21) + A3 reconcile `--reconcile-interval`
   (heal proactive, Go unit + e2e).** Suite MD0–MD22 (23 case).
   Còn thiếu cho DoD mục 1:
   - systemd unit chạy THẬT dưới user manager (lingering) — env-gated,
     chỉ verify cú pháp được (MD22); cần host có systemd thật.
   - **CG1** quota qua cgroups v2 (Memory/CPUs/pids — hiện chỉ Notes + rlimit nproc
     theo uid, S2) → gộp với mục 5.
   - **G3 preview: XONG phiên 14 (#97, D49)** — Traefik file provider + wake endpoint;
     còn residual mục 4 là pasta config-side (khai báo port-forward trong config instance).
   - **A2** image `oci:` (crane): **e2e xong 2026-11-21 phiên 16 (#103, D51 + fix
     #108)** — crane pin v0.22.1 + registry in-memory offline; MD28–MD34; residual:
     #109 absolute-symlink refusal (**resolved D70**: giữ refusal, document, đóng), remote pull opt-in
     MD33, rootfs cache khi images churn. (**unpack boundary hardening xong
     2026-11-21 (#104, D50)**: `tar -x` host tar trong `oci:`/`tar:` thay bằng Go
     stdlib `secureUnpack` openat O_NOFOLLOW fail-closed — MD26/MD27 + unit matrix).
   - **S1–S6 đều FIXED** (S1 FIXED #41; **S3 FIXED #113 2026-11-22**; **S4 FIXED #116
     2026-11-22 — `--unix-socket` fail-closed, token giữ nguyên trên mọi transport;
     residual TLS cross-host**; **S5 FIXED #121 2026-11-22 — exec'd processes nhận
     spec rlimits (nofile/nproc) qua `MINICT_RLIMIT_*` directive trong env-file,
     apply BEFORE capdrop (D54)**; **S6 FIXED #124 2026-11-23 — webport collision
     detection up front (`ErrPortInUse` → 409, D55)**; A3 reconcile proactive đã có — heartbeat P4 mở).
2. Template: instance từ template; lưu instance thành template.
   → **Xong 2026-11-23 Phiên 21 (#127, D56)**: `save-template` API + `template:<name>` image
   form; MD39 e2e (save stopped → run from template → marker seen; running → 409). Residual:
   list/remove-template API (cleanup qua state dir).
3. Storage: overlayfs rootless + idmapped mounts (thay private copy 2 MB/instance);
   snapshot/backup/restore (overlay layer + tar.zst).
   → **Snapshot/restore xong 2026-11-23 Phiên 22 (#130, D57)**: `snapshot` API + `backup:<name>`
   image form; tar.gz trusted (gzip stdlib, zstd deferred); MD40 e2e.
   → **Overlayfs rootless xong 2026-11-25 Phiên 31 (#162, D66 qua PR #163 squash
   `17f5a97`)**: env-gated `MINICT_OVERLAY=1`; zero-copy cho base/dir/template (lower
   chỉ thẳng nguồn), tar/oci/backup unpack `<dir>/lower`; mount overlay trong userns
   shim (`userxattr`, kernel ≥ 5.11) fail-closed; twin validation Go/C; MD44–46 e2e
   PASS trên runner kernel 7.0; Go unit 9 + mutant M-ovl-copy/render/shim bị bắt.
   Residual: idmapped mounts (kernel ≥ 5.12, overlayfs idmap ≥ 5.19); overlay-aware
   snapshot/SaveTemplate (merge whiteout-aware lower+upper — hiện refuse D66);
   zstd compression (gzip tạm).
3a. **Seccomp allowlist theo template — XONG 2026-11-23 Phiên 24 (#139, D59)**:
   - `minictd/internal/minict/profiles/<name>.allow` (go:embed): 4 profile ĐO THẬT
     bằng `tools/syscall-probe.c` (ptrace, `--after-exec 2` loại pha setup sandbox)
     trên workload thật (`tools/seccomp-workloads/`): busybox 61, alpine 45,
     alpine-nginx 75 (nginx+fcgiwrap+sqlite boot + traffic in-instance), debian 59
     (glibc clone3→ENOSYS fallback được đo luôn) — không chép Docker default (#29).
   - Filter (copy verbatim MD0 cả hai file): D11 blocklist floor + arg-check
     clone-NS/socket-AF + clone3 ENOSYS + JEQ allowlist + default EPERM — floor
     luôn thắng, allowlist không tái-được-nhận entry blocklist.
   - Data-flow: `RunSpec.SeccompProfile` → renderSpec nhúng `seccomp_allow` vào
     MINICT_SPEC (payload) + `MINICT_SECCOMP_ALLOW=` vào env-file exec setns
     (parity); template:<name> kế thừa profile từ spec.json đã lưu.
   - Tests: SE1-SE4 (workload chạy / syscall ngoài danh sách EPERM / control /
     floor thắng) + MD41-43 (payload+exec parity / control / template carry) +
     7 Go unit; mutants M-allow/M-render/M-exec/M-inherit đều bị bắt.
   - Residual: opt-in (chưa auto-apply theo image); đo lại trên runner nếu surface
     khác (đã đo dev 5.10); arm64 số syscall; backup:<name> không carry spec
     (snapshot chỉ tar rootfs — D57 residual).
4. Port-forward theo instance qua pasta, khai báo trong config instance.
   → **Xong phần config-side 2026-11-25 Phiên 32 (#165, D67)**:
   `contract.RunSpec.PastaPorts` (`pasta_ports`, omitempty — D59 pattern;
   grammar `[tcp|udp/]<host>:<ns>`, số đơn cap 16, validate fail-closed
   host-side 1024..65535 + không trùng proto/host-bind); daemon attach
   pasta out-of-band sau payload sống (`internal/minict/pasta.go` —
   `--foreground --userns /proc/<pid>/ns/user --netns /proc/<pid>/ns/net
   --config-net [--dns-forward GW] -t/-u`, readiness default route trong
   `/proc/<shim>/net/route` + alive-check, D37/D42 parity); shim C KHÔNG
   đổi (pasta host-side như webproxy — không đụng MD0); resolver D42
   nướng TRƯỚC spawn (legacy `rootfs/etc`, overlay `upper/etc` — không bao
   giờ lower zero-copy); fail-closed toàn đường (thiếu bin/tun → Run 4xx +
   R4 wipe, Start → instance STOPPED `KilledBy=pasta-fail`); lifecycle kill
   ở Stop/Remove/alive-heal (`PastaPID`+`PastaTicks` chống pid-reuse —
   pasta giữ listening socket host-side); S6 collision check cho host bind;
   flag `--pasta-bin` (beside-binary → PATH). Tests: 8 Go unit (argv pin +
   17 nhóm sai + GW hex LE) + MD47–49 env-gated `/dev/net/tun` (forward
   FPORT≠8080 tách auto-default, stop/start lifecycle, crash-heal reap +
   fail-closed). Residual: range/address-prefix grammar, CLI sandbox run
   chưa expose pasta_ports, --pasta-bin PATH không pin (suite luôn pinned).
5. Quota CPU/RAM/pid qua cgroups v2 delegation; disk quota dạng cảnh báo.
   → **Xong phần core 2026-11-20 phiên 13 (#4, D48)**: cgroups v2 islands
   enforcement (memory.max/cpu.max/cpu.weight/pids.max + shim migration +
   Stats cgroup reads, fail-soft khi không có base); suite `test_cg.sh`
   CG0–CG5 env-gated vào `make test` (129 case); CG0–CG5 PASS thật trên
   runner nếu sudo bootstrap được. Residual: memory.swap.max/io.max mapping,
   cpu.weight mapping tuyến tính (D48).
6. Landlock LSM cho fs-policing (kernel ≥ 5.13, runner đạt).
   → **Xong 2026-11-23 Phiên 26+28+#157** (#149/D61 ro-root + #154/D63 fail-closed + #157/D64
   ptrace-scope fix + default policy re-enabled). Landlock áp dụng ở SHIM (sau pivot_root,
   trước fork payload) → shim+payload cùng domain (fix PROC2 ptrace). spec.ReadOnly =
   child domain RO layer (fail-closed die). LL_FS_REFER (ABI ≥2) cho rename. ABI probe
   (LANDLOCK_CREATE_RULESET_VERSION). LL1 (CI ro-root) + LL2 (dev fail-closed die) + LL3
   (default boots).
7. **Observability** (mới 2026-11-23, owner duyệt; manifest DoD GĐ2). Thứ tự:
   sau mục 4 (port-forward), **trước mục 8** vì mục 8 đo RSS phải tính cả phần này;
   mục 6 (Landlock) độc lập, làm trước hay sau đều được. Chỉ Go stdlib (`log/slog`, `net/http`,
   đọc file cgroup); **không** OpenTelemetry SDK, **không** `prometheus/client_golang`.
   → **Xong 2026-11-26 Phiên 33 (#168, D68 qua PR squash)**:
     - **7a log JSON**: `--log-format text|json` (mặc định json); `slog.NewJSONHandler`
       + `ReplaceAttr` level lowercase. Lifecycle events (create/start/stop/destroy/
       snapshot/save-template) qua `eventLog` helper: `instance`/`op`/`result`/`duration_ms`.
       HTTP access log qua `requestLog` middleware: `request_id` (X-Request-Id hoặc random
       hex 16-char), `method`/`path`/`instance`/`op`/`status`/`result`/`duration_ms`. Token
       KHÔNG bao giờ log (MD50 grep verify).
     - **7b metrics**: `minictd/internal/minict/metrics.go` — `CollectMetrics` +
       `renderPrometheus` (viết tay ~250 dòng, stdlib only). 8 instance metrics
       (cpu_usage_seconds_total counter, memory_bytes/memory_limit_bytes/io_read/
       io_write/pids/pids_limit gauges, up{state} gauge) + 3 daemon (instances{state},
       process_resident_memory_bytes, build_info{version,go_version,runtime}). Tái dùng
       `cgReadInt`/`cgCPUStatUsage` (#4/D48) + thêm `cgIOStat` (sum rbytes/wbytes).
       Fail-soft: instance không cgroup → chỉ `up`/`state`, không bịa 0. Deterministic
       sort (name, labels) cho scrape ổn định.
     - **7c /metrics endpoint**: `GET /metrics` trên ServeMux sẵn có, cùng token auth
       (middleware `auth` wrap toàn mux). Content-Type `text/plain; version=0.0.4`.
       Scrape-time (no goroutine, no RAM history). `--metrics-listen` flag là placeholder
       fail-closed — mọi giá trị non-empty → exit 1 (roadmap: non-loopback always rejected;
       loopback-only có thể relax sau).
     - **7d tests**: MD50 (log schema + no token leak) + MD51 (/metrics format + fail-soft +
       instance up) + MD52 (401 no-token + --metrics-listen fail-closed + 200 with token);
       10 Go unit (renderer escape/order/fail-soft, io.stat sum/empty/missing, daemon RSS,
       build_info labels). Mutants M-render-escape/-order/-no-cgroup-skip/-io-stat-sum bị bắt.
       Residual: cgroup value-match e2e (so sánh /metrics với file cgroup trực tiếp) cần
       sudo bootstrap — Go unit test renderer đủ bao phủ cho D68; full e2e để test_cg.sh.
     - **7e ngân sách**: RSS idle tăng < 2 MB (renderer ~250 dòng + slog JSON handler stdlib,
       không prometheus/client_golang → không thêm dependency nặng); số đo CI ghi vào
       `docs/perf-baseline.md` bảng Phiên 33.
   a. **Log JSON có cấu trúc**: `minictd` đổi `slog.NewTextHandler` → `slog.NewJSONHandler`
      (giữ cờ `--log-format text|json`, mặc định `json`). Mỗi sự kiện vòng đời ghi
      đúng một dòng: `create`, `start`, `stop`, `destroy`, `exec`, `snapshot`,
      `save-template`, `reconcile`/`heal`, `autostart`, và mọi lỗi trả về client
      (HTTP ≥ 400) hoặc lỗi nền. Trường bắt buộc: `time`, `level`, `msg` (= tên sự
      kiện), `instance`, `op`, `result` (`ok`/`error`), `duration_ms`, `error` (khi
      lỗi), `request_id` (nếu từ API). **Không bao giờ** ghi token, nội dung env-file,
      hay giá trị biến môi trường của instance.
   b. **Metrics từng instance** đọc từ cgroups v2 (tái dùng phần đọc của `Stats`
      đã có từ #4/D48, không viết đường đọc thứ hai): `cpu.stat` (usage_usec),
      `memory.current`, `memory.max`, `io.stat` (rbytes/wbytes), `pids.current`,
      `pids.max`. Xuất dạng Prometheus text exposition (viết tay, ~100 dòng):
      `minict_instance_cpu_usage_seconds_total`, `minict_instance_memory_bytes`,
      `minict_instance_memory_limit_bytes`, `minict_instance_io_read_bytes_total`,
      `minict_instance_io_write_bytes_total`, `minict_instance_pids`,
      `minict_instance_pids_limit`, `minict_instance_up` (label `instance`,
      `state`); thêm của daemon: `minictd_instances{state}`,
      `minictd_process_resident_memory_bytes`, `minictd_build_info`.
      Instance không có cgroup (fail-soft, không có base) → chỉ xuất `up`/`state`,
      không bịa số 0.
   c. **Endpoint `GET /metrics`** trên listener API sẵn có (TCP 127.0.0.1 và/hoặc
      `--unix-socket`), **yêu cầu cùng token** như `/v1/*`. Không mở listener mới;
      nếu sau này cần `--metrics-listen` thì phải từ chối (fail-closed, exit ≠ 0)
      mọi địa chỉ không phải loopback. Đọc theo yêu cầu (scrape-time), không có
      goroutine thu thập nền, không giữ lịch sử trong RAM.
   d. **Test** (vào `make test-minictd` + Go unit):
      - MD41 log schema: chạy create/start/stop/destroy + một lỗi (vd. 409), parse
        từng dòng stderr bằng JSON, kiểm đủ trường bắt buộc; grep không thấy token.
      - MD42 metrics khớp cgroup: instance chạy `sleep`, đọc `/metrics` và đọc trực
        tiếp file cgroup của instance, `memory_bytes`/`pids` khớp (pids chính xác,
        memory sai lệch ≤ 5 % hoặc ≤ 256 KiB), counter CPU không giảm giữa 2 lần scrape.
        Env-gated như CG0–CG5 (SKIP khi không có cgroups v2 delegation).
      - MD43 bind/auth: `/metrics` không token → 401; daemon chạy `--listen
        127.0.0.1:<port>` thì kết nối tới IP không phải loopback của máy bị từ chối;
        `--metrics-listen 0.0.0.0:…` (nếu có cờ) → daemon thoát lỗi.
      - Go unit: renderer Prometheus text (escape label, `# HELP`/`# TYPE`, thứ tự ổn
        định), parser `cpu.stat`/`io.stat`, instance không cgroup.
   e. **Ngân sách**: ghi RSS idle của `minictd` trước/sau vào `docs/perf-baseline.md`;
      tăng thêm > 2 MB phải giải trình trong PR (perf-guard manifest §3).
   Ngoài phạm vi GĐ2 (ghi để không làm sớm): biểu đồ là GĐ3 mục 3 (dùng lại
   `/metrics`); OTLP exporter / collector ngoài là hướng cluster nhiều VPS (xem
   "Hướng xa" cuối file).
8. Đo: control plane idle ≤ 30 MB RSS (**tính cả observability mục 7**); destroy không
   rò rỉ process/mount/netns.
   → **Xong 2026-11-26 Phiên 34 (#171, D69 qua PR squash)**:
     - **MD53 destroy no-leak**: Run(webport) → Stop + Remove → verify 0 process leak
       (`pgrep -f $STATE/<name>` rỗng — shim/payload/webproxy/pasta đều gone) + 0 mount
       leak (`grep $STATE/<name> /proc/self/mountinfo` rỗng — bind mounts unmounted) +
       instance dir gone. Netns reclaim qua kernel semantics (process + mount = 0 →
       netns tự reclaim, research nguồn 3).
     - **MD54 daemon idle RSS ≤ 30 MB**: scrape `/metrics` `minictd_process_resident_memory_bytes`
       (D68) + cross-check `/proc/$DPID/status` VmRSS (match ±64 KB) → assert ≤ 31457280
       bytes (30 MB). Dev baseline 6372 KB (~6.2 MB) — biên độ dư ~24 MB (regression
       guard, không tight constraint).
     - Residual: netns leak test trực tiếp (process + mount đủ bao phủ); overlay/pasta
       leak paths env-gated SKIP trên dev (PASS thật trên CI runner kernel 7.x).

## Giai đoạn 3 — Web UI + console (HIỆN TẠI — owner mở 2026-11-26)

> 2026-11-26: owner mở GĐ3 và giao template Next.js (`console/`). Owner chốt
> (thẻ quyết định 2026-11-26) **"xuất tĩnh, minictd phục vụ"**: `next build`
> với `output: "export"` → `console/out`, `minictd --ui-dir` phục vụ trên cùng
> listener 127.0.0.1 + token; không tiến trình Node lúc chạy. React/Next chỉ
> dùng lúc build (manifest §7 GĐ3 đã sửa theo quyết định này).

1. UI tĩnh single-user, bind 127.0.0.1 + token: danh sách,
   tạo từ template, start/stop, snapshot/restore, port-forward, quota.
   → **Khung xong 2026-11-26** (PR console): `--ui-dir` + CSP hash inline script,
   `GET /v1/templates` + `GET /v1/backups`, adapter `console/src/console/minictd.ts`
   (đăng nhập token, list/inspect/stats, run với cpus/memory/pasta_ports, start/stop/
   restart/remove, save-template/snapshot, tạo lại từ `template:`/`backup:`).
   → **Xong 2026-11-26 Phiên 35 (#179, D72)**: API `POST /v1/templates|backups/{name}/remove`
     (validName + idempotent 204 + symlink-safe); `HostConfig` trong `ContainerJSON`
     (NanoCpus/Memory/PidsLimit từ spec.json); console map resources + bật nút xoá
     Templates (real mode) + cảnh báo quota disk từ state notes. MD61 e2e + 8 Go unit.
2. Web terminal vào instance qua `exec` của GĐ2.
   → **Line-mode xong 2026-11-26** (mỗi dòng = `execv2` `/bin/sh -c`, giữ cwd).
   Còn thiếu: endpoint PTY (WebSocket hoặc stream hai chiều) cho vi/top/REPL.
   → **Research xong 2026-11-26 Phiên 35 (#181)**: chọn (B) openpty sau setns trong
     `minict-exec` + 2 kênh SSE (stdout) + POST (stdin/resize), stdlib only, < 1 MB RSS/phiên.
     **Owner duyệt kế hoạch (B) 2026-11-26** → việc kế tiếp: code theo #181 (PTY trong
     `minict-exec`, endpoint SSE + POST stdin/resize trong `minictd`, console dùng xterm
     thật thay line-mode khi endpoint có), MD e2e (vi/top chạy được, đóng phiên dọn PTY),
     giữ ngân sách RSS. Làm sau mục 3 nếu mục 3 đang dở.
   → **PTY backend xong 2026-11-26 Phiên 36 (#187, D73)**: minict-exec `--pty` (openpty
     + slave dup2 + master pump select + control_fd TIOCSWINSZ) + daemon endpoints
     (terminal.go: POST /terminal → sid, GET /stream SSE, POST /input, POST /resize,
     SIGHUP cleanup) + Store.PrepareTerminal (env-file reuse ExecV2). MD62 e2e env-gated.
   → **Console xterm attach xong 2026-11-26 Phiên 37 (#189)**: adapter rawText() cho
     PTY input (text/plain), followSse() đọc SSE frames, route mappings /terminal/*.
     Terminal view: thử POST /terminal → PTY mode (SSE+input+resize), fallback ExecShell.
     tsc + next build clean.
3. Biểu đồ CPU/RAM/IO theo instance: UI scrape `/metrics` của GĐ2 mục 7 (cùng token,
   qua 127.0.0.1), giữ lịch sử ngắn phía trình duyệt; không thêm collector trong daemon.
   → 2026-11-26: biểu đồ CPU/RAM dùng `GET /{name}/stats` (cùng nguồn cgroup với
   `/metrics`), poll 2 s, lịch sử trong trình duyệt. Còn thiếu: IO + chuyển sang
   parse `/metrics` nếu cần một lần gọi cho mọi instance.
   → Việc kế tiếp: thêm biểu đồ IO (block read/write, net rx/tx — đã có trong `/stats`)
     vào trang chi tiết + Monitoring; MD e2e hoặc Go unit cho dữ liệu, `npx tsc --noEmit`
     + `npx next build` sạch.
   → **Xong 2026-11-26 Phiên 36 (#185)**: biểu đồ Block IO (read/write rate KB/s) +
     Network IO (rx/tx rate) trong sandbox-detail; monitoring response thêm IO fields;
     tsc + next build sạch.
4. Test bảo mật UI: CSRF/token, không bind 0.0.0.0 mặc định, path traversal API.
   → 2026-11-26: Go unit `internal/server/ui_test.go` (traversal/symlink/dotfile,
   405 cho ghi, API vẫn 401 khi có UI, CSP không `unsafe-inline` cho script,
   không `--ui-dir` thì `/` vẫn sau auth). Token là header Bearer, không cookie.
   → **E2e xong 2026-11-26 Phiên 35 (#177, D71)**: MD55-MD60 (GET / 200 + CSP
     sha256 no-unsafe-inline + X-Frame DENY; traversal/symlink/dotfile no-leak;
     POST 405; API 401 no-token UI-on; GET / 401 no-token UI-off legacy;
     --ui-dir no index.html → exit ≠ 0) + XSS grep (0 dangerouslySetInnerHTML/
     innerHTML, xterm write() safe).
5. CI build console: thêm bước `make console` (tsc + next build) vào CI để PR làm hỏng
   console bị chặn.
   → **Xong 2026-11-26** (owner duyệt): job `console` trong `ci.yml` chạy `make console`
     (`npm ci --ignore-scripts` + `tsc --noEmit` + `next build` + kiểm `out/index.html`).

## Giai đoạn 4 — Backend microVM Firecracker (HIỆN TẠI — owner mở 2026-11-26)

> GĐ3 DoD đạt (Phiên 37). Owner chỉ thị mở GĐ4 2026-11-26.
> Mục 1–2 cần host có KVM; khi chưa có, SDDA làm Track M (bên dưới).

0. **KIỂM TRA TRƯỚC:** `/dev/kvm` trên runner GitHub (workflow probe `ls -l /dev/kvm`)
   và máy đích. Có → mở kế hoạch chi tiết; không có → research-only, ghi rõ.
   → **Xong 2026-11-26 Phiên 38 (#191, research-only)**:
     - Dev machine: NO `/dev/kvm` (no vmx/svm CPU flags — cloud container).
     - GitHub runner ubuntu-24.04 standard: NO `/dev/kvm` (no nested virt — runs inside Hyper-V).
     - → GĐ4 ở mức **research + design** cho đến khi có KVM-capable host (self-hosted runner
       hoặc bare metal).
     - Research 5 nguồn: Firecracker design (KVM hard req), jailer (rootless uid/gid/chroot),
       rootfs/kernel (vmlinux + ext4 image), API (unix socket HTTP PUT), GitHub runner KVM
       (standard = no, larger runner = yes).
     - --backend design: `--backend minict|firecracker` flag; RunSpec.Backend field (additive);
       firecracker path = jailer + ext4 rootfs + TAP/pasta + API socket + serial console.
       Same API/UI, only runtime changes.
     - File: `journal/2026-11-26/research-gd4.md`.
1. **Firecracker backend** (code khi có KVM):
   a. `tools/fetch-firecracker.sh` — pin+sha256 Firecracker binary + jailer (pattern D37).
   b. `tools/fetch-vmlinux.sh` — pin+sha256 uncompressed kernel (AWS S3 CI artifacts).
   c. `minictd/internal/firecracker/` — Go package: API socket client (unix HTTP PUT),
      rootfs ext4 builder (mkfs.ext4 + copyTree), TAP setup (in netns), jailer spawn.
   d. `Store.Backend` field + `--backend` flag; Run/Start/Stop/Remove branch on backend.
   e. Serial console terminal: GET /v1/sandboxes/{name}/serial/stream (SSE from Firecracker
      logger socket); POST /serial/{sid}/input (write to serial).
   f. Tests: env-gated SKIP khi no /dev/kvm (pattern overlay D66 / pasta D67); PASS thật
      trên KVM-capable host.
   g. Budget: daemon RSS +~5 MB (management code); per-microVM Firecracker ~10-20 MB.
1b. **Làm được khi CHƯA có KVM** (owner chốt 2026-11-27 GĐ4 ưu tiên số 1; SDDA làm theo
   thứ tự, mỗi mục có test chạy không cần `/dev/kvm`, phần cần KVM thì env-gated SKIP):
   a. **Jail không cần root**: jailer của Firecracker cần root (chroot, đổi uid, cgroup).
      Nghiên cứu + decision file: chạy `firecracker` **bên trong một instance minict**
      (userns + mountns + seccomp + pivot_root của minict làm jail; chỉ bind `/dev/kvm` +
      socket API vào). Đo được phần không cần KVM: binary khởi động trong jail, `--version`,
      socket API trả lời `GET /`. Không dùng jailer upstream nếu nó buộc root.
   b. **Rootfs ext4 rootless**: builder từ template/rootfs hiện có bằng `mkfs.ext4 -d`
      (không mount, không root); kiểm image bằng `e2fsck -n` + `debugfs -R ls`; giới hạn
      kích thước; test MD không cần KVM.
   c. **Mạng microVM rootless**: TAP tạo trong netns do userns của daemon sở hữu (có
      CAP_NET_ADMIN trong netns đó), pasta gắn vào netns ấy như instance minict (áp mặc định
      fail-closed của #220). Test tạo/xoá TAP trong userns netns, không cần KVM.
   d. **Guest agent cho exec/terminal/files**: microVM không `setns` được như container.
      Thiết kế agent tối giản trong guest (init PID 1 tĩnh, nói qua **vsock**: exec, PTY,
      đọc/ghi file theo bytes) để API/console hiện có dùng chung cho cả hai backend. Tài
      liệu thiết kế + agent build tĩnh + test giao thức bằng socket giả (không cần KVM).
   e. **API client + backend**: hoàn thiện `internal/firecracker` (boot-source, drives,
      machine-config, network-interfaces, vsock, InstanceStart, shutdown) với test bằng unix
      socket giả lập; `--backend firecracker` fail-closed rõ ràng khi thiếu KVM (MD63 giữ).
   f. **Host có KVM cho CI**: chờ owner chọn (VPS có `/dev/kvm`, self-hosted runner, hoặc
      GitHub larger runner). Workflow chạm `.github` → owner duyệt.
   g. Console: chọn backend khi tạo + badge "microVM"; API không đổi.
2. **Benchmark** microVM vs container:
   - cold_start_ms: minict ~2 ms vs Firecracker ~125 ms (Firecracker measured upstream).
   - RSS: minict ~1.2 MB wrapper vs Firecracker ~10-20 MB VMM + guest memory.
   - Isolation: Firecracker = hardware VM (stronger than container userns+seccomp).
   - Workload web nhỏ: HTTP 200 ready time (spawn → first request).
   - Ghi vào `docs/perf-baseline.md` bảng GĐ4.

## Giai đoạn 5 — Appliance OS (xa hơn)

1. Image tối giản (NixOS/distro nhẹ) ≤ 300 MB, boot vào user thường đã có điều
   kiện tiên quyết (pasta, cgroup delegation, quyền /dev/kvm); control plane chạy dưới user đó.
2. Health + update tối thiểu; tài liệu từ máy trắng. (Chi tiết khi GĐ4 xong.)

## GĐ3 — gia cố sau DoD (owner dùng thật, 2026-11-26/27)

> Lỗi và nhu cầu owner gặp khi dùng console thật. **Owner chốt 2026-11-27: GĐ4 microVM
> làm trước.** Trong mục này chỉ **#220 (security)** còn được làm ngay; #218, #219, #221
> **hoãn tới sau GĐ4** (đã hạ `priority:p3`). Lý do: GUI, firewall, terminal WS đều cần
> chạy được workload không tin cậy một cách an toàn, mà manifest §3 chỉ cho phép điều đó
> sau GĐ4.

- Xong: #206 PTY fail-closed (PR #210, D77) · #208 Files qua execv2 (PR #212, D79) ·
  #209 uptime + heal stale state (PR #213, D80) · bản sửa của owner, gồm TAB thật trong
  `stat -c`, bật trang Files, input UTF-8 gửi tuần tự, locale PTY (PR #217).
1. **#220 (security)**: pasta mặc định `auto` (UDP inbound, bind 0.0.0.0, sandbox gọi vào
   127.0.0.1 của host). Đo rồi mới sửa; mặc định fail-closed (`-u none -T none -U none`,
   forward bind 127.0.0.1, công khai phải opt-in). Tới khi xong, manifest §6 "chỉ mở port
   khai báo" chưa được coi là đúng.
2. **#218**: WebSocket PTY binary (stdlib RFC 6455, auth bằng frame đầu, kiểm `Origin`,
   64 KB/frame); SSE+POST giữ làm fallback.
3. **#219**: Files dùng tên/path base64 hai chiều (adapter-only) + recovery tên không
   phải UTF-8.
4. **#221 phần 1**: `network: none|egress` + công tắc "Cho phép Internet" trên console.
   **Phần 2** (tường lửa vào/ra + tab Network giám sát): **chỉ tài liệu thiết kế, chờ owner
   duyệt**. Nếu được duyệt thì cần sửa manifest §4/§6 trước khi code.

## Track M — Mở rộng ngoài core (manifest §9, D74, từ 2026-11-26)

> **Tạm dừng tới sau GĐ4** (owner chốt 2026-11-27). M1–M3 đã xong; M3e, M4, M5 chờ.

> Không phải giai đoạn mới. SDDA lấy mục thấp nhất chưa xong ở đây **chỉ khi** mọi
> mục còn lại của giai đoạn hiện tại bị chặn (môi trường hoặc chờ owner); hiện GĐ4
> đang research-only vì thiếu `/dev/kvm`, nên Track M là việc kế tiếp. Mọi mục ở đây
> là lớp client/công cụ/file tĩnh: **không** thêm phụ thuộc, tiến trình nền hay
> parse YAML vào `minict`/`minictd`. Mỗi PR ghi câu trả lời hai phép thử manifest §3.

1. **M1 — Nhãn trạng thái tài liệu.** Gắn nhãn `sẵn sàng` / `thử nghiệm` / `kế hoạch`
   (định nghĩa manifest §3) cho từng tính năng trong `README.md`, `minictd/README.md`
   và tài liệu console. `sẵn sàng` chỉ khi có e2e xanh trên CI; env-gated SKIP trên CI
   hoặc còn residual → `thử nghiệm`. Thêm câu cũ mơ hồ vào `tools/docs-stale-phrases.txt`
   nếu cần (`make check-docs`).
   → **Xong 2026-11-26 Phiên 39 (#197, PR #198)**: bảng 22 tính năng trong README.
2. **M2 — `stack.yaml` + `sandbox stack up|down|ps`.**
   a. Thiết kế trước (decision file): schema tối giản chỉ gồm trường `minictd` đã hỗ
      trợ (image/`template:`, cmd, env, `pasta_ports`, cpus/memory/pids, mount thư mục,
      `depends_on` + `ready` kiểu HTTP/TCP/lệnh exec, timeout). Trường lạ → lỗi, không
      bỏ qua im lặng. Không tương thích docker-compose (manifest §4).
   b. Parser + CLI ở lớp client (Go, tránh thư viện YAML nặng; cân nhắc tập con YAML
      hoặc JSON nếu thư viện làm phình binary, ghi số đo). Daemon không đổi.
   c. Test: MD e2e một stack 2 instance (web + db) có phụ thuộc + chờ sẵn sàng; down
      dọn sạch (không rò process/mount/netns).
   → **Một phần 2026-11-26 Phiên 39 (#199, PR #200, D75)**: `stack.json` (JSON thay
     YAML để giữ zero-dep; YAML qua `yq`), `DisallowUnknownFields`, topoSort + phát hiện
     vòng, `sandbox stack up|down|ps`, 6 Go unit test. **Còn thiếu, làm trước M3**
     (owner review 2026-11-26):
     d. Thêm trường `pasta_ports` (map thẳng `RunSpec.PastaPorts`). Hiện stack không
        khai báo được port, nên ready `http`/`tcp` trên 127.0.0.1 không trỏ tới service.
     e. Ready `exec` đang được nhận trong schema nhưng trả lỗi "unsupported": hiện
        thực qua `execv2` (exit 0 = sẵn sàng) hoặc bỏ khỏi schema; validate `type` lúc
        parse, không đợi tới lúc chạy.
     f. Ready `http` dùng `http.Client` có `Timeout` (hiện `http.Get` không timeout, có
        thể treo quá `timeout_s`); validate `port` 1–65535 khi type là http/tcp.
     g. `stack down` không nuốt lỗi: instance không tồn tại → bỏ qua, lỗi khác → in ra
        và exit ≠ 0; chỉ in "removed" khi xoá thật.
     h. MD e2e theo mục c (2 instance có `depends_on` + ready, `down` dọn sạch, lỗi ready
        → rollback các instance đã chạy). Env-gated như MD47–49 nếu cần pasta.
     i. Cập nhật README dòng `stack.yaml` → `stack.json`, nhãn `thử nghiệm` (M1), thêm
        `examples/` một stack.json mẫu.
   → **d–i xong 2026-11-26 Phiên 40 (#203, PR #204, D76)**: `pasta_ports`, ready `exec`
     qua execv2, http timeout + validate port, down error policy, MD e2e, `examples/04-stack`.
3. **M3 — Registry template tĩnh.**
   a. Định dạng: `index.json` (name, version, sha256, size, created) + `<name>-<ver>.tar.zst`.
   b. `sandbox template push <name> --out DIR`: sinh file tĩnh từ template local.
      `sandbox template pull URL/name[:ver]`: chỉ HTTPS (HTTP thường chỉ khi
      `--insecure` cho localhost), kiểm sha256 **trước** khi giải nén, giải nén an toàn
      (không path traversal/symlink ra ngoài, dùng lại kiểm tra của backup restore).
   c. Test: pull từ web server tĩnh local; sha256 sai → fail-closed, không để lại file;
      tar độc (`../`, symlink) bị chặn.
   d. Ghi chú: kéo image OCI chuẩn đã có ở lớp client qua `crane` (D51), giữ nguyên
      hướng đó, không đưa logic OCI vào runtime.
   → **Xong 2026-11-27 Phiên 42 (#215, PR #216, D81)**: client-only `template
     push|pull` (daemon không đổi, zero-dep). Blob `.tar.gz` (deviation ghi rõ —
     stdlib không có zstd, tiền lệ snapshot.go D57). sha256+size kiểm trước khi
     giải nén (tamper → không để lại file); giải nén tái dùng `secureUnpack`
     qua export `minict.SecureUnpack` (không copy — bài học drift D25); HTTPS
     bắt buộc, HTTP chỉ `--insecure` + loopback; refuse overwrite `--out`;
     push từ chối symlink tuyệt đối (D70). 14 unit test (httptest) + **MD69**
     e2e (static server, tamper fail-closed, evil tar `../` bị chặn dù index
     xác nhận sha).
   e. **Còn thiếu (owner review 2026-11-27)**: sha256 trong `index.json` chỉ chứng minh
      blob khớp index **trên cùng server**. Server bị chiếm thì kẻ tấn công sửa được cả
      hai. Cần thêm: (1) pin khi pull `name:ver@sha256:<digest>` (digest lấy ngoài băng,
      vd. ghi trong `stack.json`/Git); (2) tuỳ chọn ký `index.json` bằng ed25519 (Go
      stdlib, zero-dep): `push --sign-key`, `pull --trust-key`; có `--trust-key` thì thiếu
      chữ ký hoặc chữ ký sai đều fail-closed; (3) giới hạn tổng dung lượng/số entry khi giải
      nén (chống tar/gzip bomb làm đầy disk); (4) README nhắc lại manifest §3: template
      kéo về vẫn chỉ dành cho workload **tin cậy** cho tới khi GĐ4 xong.
4. **M4 — Lớp cluster nhiều VPS** (mục tiêu owner 2026-11-17).
   a. **Chỉ tài liệu thiết kế, chờ owner duyệt trước khi code**: công cụ riêng (tên
      đề xuất), danh sách node + token, kết nối node qua SSH tunnel/WireGuard (`minictd`
      vẫn bind 127.0.0.1), API tổng hợp danh sách/stats nhiều node, deploy `stack.yaml`
      lên node chỉ định. Không etcd/Raft/DB, không HA, không migration.
   b. Observability nhiều node: collector ngoài (Prometheus/VictoriaMetrics) scrape
      `/metrics` từng node qua đường hầm có xác thực; hoặc OTLP exporter tuỳ chọn trong
      `minictd` (tắt mặc định, không tính vào ngân sách 30 MB khi tắt).
   c. Tuỳ chọn sau khi a được duyệt: phân quyền 3 vai owner/editor/viewer + audit log
      ở lớp cluster, không chạm core.
5. **M5 — Ví dụ triển khai VPS** trong `examples/`: Ansible playbook cài `minictd`
   (systemd `--user` + lingering) và GitHub Action gọi API qua SSH tunnel để deploy
   `stack.yaml`. Không viết nền tảng deploy riêng.

## Hướng xa (chưa thuộc giai đoạn nào, chưa duyệt)

- **Desktop GUI trong sandbox + noVNC trên console** (owner hỏi 2026-11-27): template
  "desktop" (Xvnc + WM nhẹ, không GPU), tunnel cổng VNC qua WebSocket của #218 (không mở
  port host), tab Desktop dùng noVNC đóng gói tĩnh. Làm **sau GĐ4** (cần microVM để chạy
  trình duyệt/nội dung lạ an toàn) và sau #218.

- **arm64**: port `minict` (seccomp filter theo arch, syscall số khác) — cần runner arm64.
- **Runtime cho K3s/K8s (CRI shim)**: ngược non-goal manifest §4 "không Kubernetes API";
  chỉ cân nhắc nếu owner sửa manifest.

## Parked thật (lý do thiết kế, KHÔNG phải môi trường)
- ~~seccomp allowlist mode~~ — **bỏ khỏi parked 2026-11-23**: chuyển thành GĐ2 mục 3a (#29).
- gVisor runsc — overhead cao cho use-case dev/CI; model isolation của nó trùng
  nhu cầu với microVM GĐ4.

## Mốc web stack (DoD tổng thể cũ, nay là một phần của GĐ1; đích dự án xem manifest §1, §7)
- [ ] nginx/caddy + API (Node/Python/Go) + SQLite/Postgres + Redis trong sandbox
- [x] không Docker, không root (bất biến từ PoC — mọi test chạy uid 1001)
- [ ] cold start toàn stack < 5s; RAM idle < 300MB
- [x] escape test cơ bản pass (isolation 29/29 + seccomp 21/21 + netns 7/7 + alpine 8/8,
  pipefail thật; signal lifecycle graceful từ phiên 7; đếm lại 2026-11-18)
- [ ] `./sandbox up|down|bench` một lệnh
- **Tiến độ: 2/5** (mục 2 và 4).
- Lưu ý sửa sai đếm cũ ("5/5 mục phụ đã có nền"): web tĩnh busybox + seccomp +
  netns KHÔNG nằm trong 5 mục này — chúng là điều kiện nền của GĐ1, không được
  đếm vào DoD tổng thể. Cột mốc thật của chúng xem ở "Đã xong" và manifest §7 GĐ1.
