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
   + cập nhật `docs/cve-watchlist.md`
4. D — No-Compromise Validation: `make test` (isolation 31 + seccomp 21 + netns 7 +
   alpine 8 + escape-alpine 31 + cve-check tool 4 + pasta 11 = 113, pipefail thật —
   issue #14; đếm lại 2026-11-19 phiên 8; dev: 107 PASS + 6 SKIP env-blocked) + `make test-web` +
   `make test-minictd` (20, từ 2026-11-18) + lưu evidence vào
   `journal/YYYY-MM-DD/evidence/` + integrity check host
5. Ghi `journal/YYYY-MM-DD/daily-log.md` (quy tắc thư mục: PROMPT.MD §10)

## Đã xong
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

## Giai đoạn 1 — minict runtime (HIỆN TẠI, theo thứ tự ưu tiên)

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
   (#74, D42 — `--dns-forward` + inject resolv.conf, PT3/PT3C)**; phần còn mở:
   `apk add` online trong instance (lớp DNS đã có — còn HTTPS/apk-tools runtime)
   + Debian rootfs (sau).
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

## Giai đoạn 2 — Control plane + storage (KẾ TIẾP; có "seed" minictd, chưa mở việc mới)

> **Luật D27 (owner, 2026-11-18):** SDDA **không chọn** mục GĐ2 nào dưới đây khi GĐ1
> còn mục mở. Với `minictd/` chỉ được: sửa lỗi bảo mật (S1–S6, issue `type:security`),
> giữ `make test-minictd` xanh, và cập nhật khi thay đổi GĐ1 làm vỡ nó.

0. **GATE research:** exec vào instance đang chạy rootless — **đo được 2026-11-18**
   (D25, `minict-exec` setns, MD5–MD13; container kernel 6.18). Còn: xác nhận trên
   runner CI (job `test-minictd`).
1. Daemon user-level + REST API: **seed có** (`minictd`, state machine
   creating/running/stopped, flock, liveness chống pid-reuse). Còn thiếu cho DoD:
   systemd `--user` unit, CLI `sandbox`, instance tồn tại qua reboot (autostart),
   reconcile loop định kỳ (A3). API hiện theo contract sandboxd (docker-shaped).
   - **CG1** quota qua cgroups v2 (Memory/CPUs/pids — hiện chỉ Notes + rlimit nproc
     theo uid, S2) → gộp với mục 5.
   - **G3** preview: Traefik file provider `s-<id>.<domain>` → `127.0.0.1:<port>` +
     wake probe theo port host (BridgeIP luôn rỗng) → gộp với mục 4.
   - **A2** image `oci:` (crane) đã code, **chưa test**.
   - **S1** webproxy đi theo socket bị thay bằng symlink trong rootfs (có từ D16) —
     sửa được ngay cả trong GĐ1 vì là bảo mật.
2. Template: instance từ template; lưu instance thành template.
3. Storage: overlayfs rootless + idmapped mounts (thay private copy 2 MB/instance);
   snapshot/backup/restore (overlay layer + tar.zst).
4. Port-forward theo instance qua pasta, khai báo trong config instance.
5. Quota CPU/RAM/pid qua cgroups v2 delegation; disk quota dạng cảnh báo.
6. Landlock LSM cho fs-policing (kernel ≥ 5.13, runner đạt).
7. Đo: control plane idle ≤ 30 MB RSS; destroy không rò rỉ process/mount/netns.

## Giai đoạn 3 — Web UI + console (chưa mở code)

1. UI tĩnh single-user (không framework nặng), bind 127.0.0.1 + token: danh sách,
   tạo từ template, start/stop, snapshot/restore, port-forward, quota.
2. Web terminal vào instance qua `exec` của GĐ2.
3. Biểu đồ CPU/RAM/IO theo instance từ thống kê cgroup.
4. Test bảo mật UI: CSRF/token, không bind 0.0.0.0 mặc định, path traversal API.

## Giai đoạn 4 — Backend microVM Firecracker (chưa mở code)

0. **KIỂM TRA TRƯỚC:** `/dev/kvm` trên runner GitHub (workflow probe `ls -l /dev/kvm`)
   và máy đích. Có → mở kế hoạch chi tiết; không có → research-only, ghi rõ.
1. Firecracker dưới user thường, cùng API/UI (`--backend minict|firecracker`).
2. Benchmark microVM vs container cho workload web nhỏ (startup < 1 s).

## Giai đoạn 5 — Appliance OS (xa hơn)

1. Image tối giản (NixOS/distro nhẹ) ≤ 300 MB, boot vào user thường đã có điều
   kiện tiên quyết (pasta, cgroup delegation, quyền /dev/kvm); control plane chạy dưới user đó.
2. Health + update tối thiểu; tài liệu từ máy trắng. (Chi tiết khi GĐ4 xong.)

## Parked thật (lý do thiết kế, KHÔNG phải môi trường)
- seccomp **allowlist mode** — chỉ làm khi có matrix workload httpd được audit đầy
  đủ; blocklist + capdrop + NNP đủ cho DoD hiện tại (D11).
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
