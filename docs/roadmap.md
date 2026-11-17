# Roadmap — sandbox-lab

> **Phân cấp nguồn sự thật** (issue #15): hướng tổng thể nằm ở `idea/manifest.md`
> (4 giai đoạn + DoD từng giai đoạn). File này là **kế hoạch chi tiết** — SDDA chọn
> việc theo mục thấp nhất chưa xong của giai đoạn hiện tại. `PROMPT.MD` chỉ trỏ
> vào file này, không giữ danh sách ưu tiên riêng.

## Nền tảng mục tiêu (từ manifest §5)

- Linux kernel **≥ 6.1**, **cgroups v2**, uid ≥ 1000 không sudo; `/dev/kvm` cho GĐ3.
- Runner CI (ubuntu-24.04) đã đạt mức này (kernel 6.x + cgroups v2); máy dev
  (kernel 5.10, cgroups v1, không KVM) chỉ dùng cho test nhanh/benchmark.
- Quy ước label: mục chặn vì máy dev gán `blocked-on-dev-env` — vẫn nằm trong
  kế hoạch, KHÔNG parked. Parked thật (lý do thiết kế, không phải môi trường)
  chỉ nằm ở mục "Parked thật" bên dưới.

## Quy trình hằng ngày (từ 17-11, bắt buộc)
1. A — Research kỹ thuật sandbox (Landlock+seccomp / bwrap / gVisor / Firecracker / Wasm)
2. B — Develop prototype (web service trong sandbox, đo startup/RAM)
3. C — CVE check: `python3 tools/cve-check.py --json evidence/cve-$(date +%F).json`
   + cập nhật `docs/cve-watchlist.md`
4. D — No-Compromise Validation: `make test` (isolation 20 + seccomp 20 + netns 7,
   pipefail thật — issue #14) + `make test-web` + lưu evidence vào `tests/evidence/`
   + integrity check host
5. Ghi `docs/daily-log-YYYY-MM-DD.md` (format 5 mục)

## Đã xong
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
  1.85/1160/2.02. (D19, evidence `2026-11-17-sigterm-ab.txt`)
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
  tabs (GNU Make 4.4.1). SBG: gộp PR #11 (BRANCH_GATE_PROMPT.md lên repo theo
  lệnh owner) trước phiên dev.
- [x] 2026-11-17 (phiên 4, SDDA): **seccomp-bpf blocklist mặc định bật** — 32 syscall +
  arg-check clone(CLONE_NEW*)/socket(AF_ALG), clone3→ENOSYS (D11–D13); 17 test mới
  `tests/test_seccomp.sh` gồm 4 control A/B (chứng minh EPERM từ filter mình, không phải
  host); overhead đo được +0.2ms cold start (A/B N=40, evidence `2026-11-17-seccomp-ab.txt`);
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

1. **chroot-escape test sâu hơn** (issue #5) — fd-based escape, /proc/self/root
   tricks, setuid binary trong rootfs (NNP phải chặn), core dump leak; thêm case
   cho seccomp (SIGSYS mode) + case escape cho mặt netns mới (netlink đã chặn —
   PoC socket(AF_NETLINK) trong sandbox phải EPERM, đã có SC17).
2. **Stack web thật trong 1 instance** (DoD tổng thể): nginx/caddy + API + SQLite
   bên trong 1 instance; `./sandbox up|down|bench` một lệnh; cold start stack < 5s
   — bridge web D16 là phương án loopback-only cho từng service/port; signal
   lifecycle graceful đã có từ phiên 7 (D19).
3. **Connectivity qua pasta/slirp4netns thay vì tự làm proxy** (chốt theo owner
   2026-11-17): `pasta`/`slirp4netns` cài được cả trên runner CI lẫn máy đích
   (`apt-get install pasta` / `slirp4netns`, không cần root) — đánh giá A/B với
   bridge D16 hiện có; D16 chỉ còn fallback loopback-only. Đây là điều kiện cho
   stack web đa service và cho egress (cài package bên trong instance).

## Giai đoạn 2 — Control plane tối giản (KẾ TIẾP — chưa mở code)

0. **GATE research (bắt buộc trước khi viết code):** exec vào instance đang chạy
   rootless — daemon tạo instance giữ full caps trong userns nó tạo → setns được
   từ phía daemon (mô hình RootlessKit/podman exec). Đo được trên runner rồi mới
   mở GĐ2 (issue sẽ mở khi GĐ1 còn ≤ 1 mục).
1. Daemon + API: up/down/ps/exec nhiều instance đồng thời; state machine
   created/running/stopped; cleanup không rò rỉ process/mount/netns.
2. cgroups v2 delegation (kernel ≥ 6.1 trên runner — mục này KHÔNG parked nữa;
   máy dev cgroups v1 chỉ cảnh báo khi đo).
3. Landlock LSM cho chroot/fs-policing (kernel ≥ 5.13 — runner đạt; KHÔNG parked nữa).
4. overlayfs rootless + idmapped mounts (kernel ≥ 5.15 + CVE-2023-0386 vá — runner
   đạt; thay private copy 2MB/instance, giảm disk/instance).

## Giai đoạn 3 — Đa backend microVM (chưa mở code)

0. **KIỂM TRA TRƯỚC:** `/dev/kvm` trên runner GitHub (`ls /dev/kvm` trong 1 workflow
   probe) và trên máy đích. Có → mở kế hoạch chi tiết Firecracker; không có →
   research-only và ghi rõ.
1. Firecracker backend cùng API với minict (`--backend minict|firecracker`).
2. Benchmark microVM vs container cho workload web nhỏ (startup < 1s target).

## Giai đoạn 4 — Appliance OS (xa hơn)

1. Image HĐH tối giản (NixOS/distro nhẹ) đóng gói runtime + control plane.
2. Boot máy sạch + health + update tối thiểu. (Chi tiết khi GĐ3 xong.)

## Parked thật (lý do thiết kế, KHÔNG phải môi trường)
- seccomp **allowlist mode** — chỉ làm khi có matrix workload httpd được audit đầy
  đủ; blocklist + capdrop + NNP đủ cho DoD hiện tại (D11).
- gVisor runsc — overhead cao cho use-case dev/CI; model isolation của nó trùng
  nhu cầu với microVM GĐ3.

## Definition of Done tổng thể (đích của dự án — đếm ĐÚNG theo danh sách)
- [ ] nginx/caddy + API (Node/Python/Go) + SQLite/Postgres + Redis trong sandbox
- [x] không Docker, không root (bất biến từ PoC — mọi test chạy uid 1001)
- [ ] cold start toàn stack < 5s; RAM idle < 300MB
- [x] escape test cơ bản pass (isolation 20/20 + seccomp 20/20 + netns 7/7, pipefail thật;
  signal lifecycle graceful từ phiên 7)
- [ ] `./sandbox up|down|bench` một lệnh
- **Tiến độ: 2/5** (mục 2 và 4).
- Lưu ý sửa sai đếm cũ ("5/5 mục phụ đã có nền"): web tĩnh busybox + seccomp +
  netns KHÔNG nằm trong 5 mục này — chúng là điều kiện nền của GĐ1, không được
  đếm vào DoD tổng thể. Cột mốc thật của chúng xem ở "Đã xong" và manifest §6 GĐ1.
