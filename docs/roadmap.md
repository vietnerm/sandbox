# Roadmap — sandbox-lab

## Quy trình hằng ngày (từ 17-11, bắt buộc)
1. A — Research kỹ thuật sandbox (Landlock+seccomp / bwrap / gVisor / Firecracker / Wasm)
2. B — Develop prototype (web service trong sandbox, đo startup/RAM)
3. C — CVE check: `python3 tools/cve-check.py --json evidence/cve-$(date +%F).json`
   + cập nhật `docs/cve-watchlist.md`
4. D — No-Compromise Validation: `bash tests/test_isolation.sh` + lưu evidence
   vào `tests/evidence/` + integrity check host
5. Ghi `docs/daily-log-YYYY-MM-DD.md` (format 5 mục)

## Đã xong
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

## Tiếp theo (theo thứ tự ưu tiên cho các ngày tới)
1. **Full web stack trong minict** (Definition of Done tổng thể): nginx/caddy + API +
   SQLite bên trong 1 instance; `./sandbox up|down|bench` một lệnh; cold start stack < 5s
   — bridge web hiện có (D16) là phương án pending cho từng service/port.
2. **chroot-escape test sâu hơn** (issue #5) — fd-based escape, /proc/self/root tricks, setuid
   binary trong rootfs (NNP phải chặn), core dump leak; thêm case cho seccomp (SIGSYS mode)
   + case escape cho mặt netns mới (netlink đã chặn — PoC socket(AF_NETLINK) trong sandbox
   phải EPERM, đã có SC17).
3. **cgroups v1 delegation** (nếu host cho phép) hoặc đánh dấu parked vì v1 không rootless-friendly
4. Wasmtime/WASI cho backend — so sánh với minict về RAM/startup/isolation model
5. overlayfs rootless + idmapped mounts — parked cho tới khi kernel ≥5.15 (CVE-2023-0386)
6. seccomp allowlist mode — parked: chỉ làm khi có matrix workload httpd được audit
   đầy đủ; blocklist hiện tại + capdrop + NNP đủ cho DoD hiện tại
7. SIGTERM swallowed (issue #6) — môi trường nuốt SIGTERM cross-userns; parked vì
   cleanup SIGKILL+PDEATHSIG+pidns đã đủ cho lifecycle hiện tại

## Parked ideas
- Firecracker/Cloud Hypervisor: env không có KVM — chỉ có thể research lý thuyết
- gVisor runsc: chưa cài được; overhead cao cho use-case dev/CI hiện tại
- Landlock: đợi kernel ≥5.13

## Definition of Done tổng thể (đích của dự án)
- [ ] nginx/caddy + API (Node/Python/Go) + SQLite/Postgres + Redis trong sandbox
- [ ] không Docker, không root
- [ ] cold start toàn stack < 5s; RAM idle < 300MB
- [ ] escape test cơ bản pass
- [ ] `./sandbox up|down|bench` một lệnh
- Tiến độ: **5/5 mục phụ đã có nền** (rootless ✅, escape test cơ bản ✅, web server tĩnh
  trong sandbox ✅, syscall filter seccomp ✅, **netns loopback-only + bridge web ✅**;
  còn: stack đầy đủ nginx/API/DB, một-lệnh-stack)
