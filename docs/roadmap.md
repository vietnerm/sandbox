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
4. D — No-Compromise Validation: `make test` (isolation 20 + seccomp 20 + netns 7,
   pipefail thật — issue #14) + `make test-web` + lưu evidence vào `journal/YYYY-MM-DD/evidence/`
   + integrity check host
5. Ghi `journal/YYYY-MM-DD/daily-log.md` (quy tắc thư mục: PROMPT.MD §10)

## Đã xong
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
   → Alpine **xong phiên 8** (xem "Đã xong"); phần còn mở: `apk add` online
   trong instance **chờ egress pasta (mục 4)** + Debian rootfs (sau).
2. **chroot-escape test sâu hơn** (issue #5): fd-based escape, /proc/self/root,
   setuid binary trong rootfs (NNP chặn), core dump leak; case seccomp SIGSYS;
   chạy lại cả suite trên rootfs distro thật (có `/proc`, nhiều mặt tấn công hơn busybox).
3. **Connectivity qua pasta/slirp4netns** (thay proxy tự viết): A/B với bridge D16;
   D16 chỉ còn fallback loopback-only. Điều kiện cho egress (cài gói trong instance)
   và cho port-forward theo instance ở GĐ2.
4. **Stack web thật trong 1 instance**: nginx/caddy + API + SQLite trên rootfs
   distro; `./sandbox up|down|bench` một lệnh; cold start stack < 5 s;
   signal lifecycle graceful đã có từ phiên 7 (D19).

## Giai đoạn 2 — Control plane + storage (KẾ TIẾP, chưa mở code)

0. **GATE research:** exec vào instance đang chạy rootless (daemon giữ caps trong
   userns nó tạo → setns từ phía daemon, mô hình RootlessKit/podman exec). Đo
   trên runner rồi mới mở GĐ2 (issue mở khi GĐ1 còn ≤ 1 mục).
1. Daemon user-level (systemd `--user`) + REST API + CLI `sandbox`:
   create/start/stop/destroy/list/exec; state machine; instance tồn tại qua reboot.
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
- [x] escape test cơ bản pass (isolation 20/20 + seccomp 20/20 + netns 7/7, pipefail thật;
  signal lifecycle graceful từ phiên 7)
- [ ] `./sandbox up|down|bench` một lệnh
- **Tiến độ: 2/5** (mục 2 và 4).
- Lưu ý sửa sai đếm cũ ("5/5 mục phụ đã có nền"): web tĩnh busybox + seccomp +
  netns KHÔNG nằm trong 5 mục này — chúng là điều kiện nền của GĐ1, không được
  đếm vào DoD tổng thể. Cột mốc thật của chúng xem ở "Đã xong" và manifest §7 GĐ1.
