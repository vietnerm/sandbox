# sandbox-lab

Nghiên cứu + hiện thực hóa sandbox nhẹ rootless cho full web service stack —
không Docker, không root, chỉ cơ chế kernel Linux có sẵn.

## Kết quả hiện tại (2026-11-17)

**minict v0.2** — mini-container rootless (C, 0 dependency ngoài libc):

| metric | đo được | target |
|---|---|---|
| cold start | 1.85 ms (p50, gồm netns+seccomp+signal shim) | < 2s |
| RAM overhead | ~1.1 MB | < 30 MB |
| disk base | 2.02 MB | < 50 MB |
| rootless | ✅ (uid 1001) | bắt buộc |
| escape tests | 20 PASS (incl. netns isolation thật + signal shim) + 6 web + 20 seccomp + 7 netns | — |
| signal lifecycle | SIGTERM/SIGINT delivered qua pidns-1 shim (tini/runc pattern), exit 128+sig; teardown SIGKILL+PDEATHSIG giữ làm fail-safe | D19 |
| web trong sandbox | busybox httpd TRONG netns riêng (loopback-only), host truy cập qua unix-socket bridge, E2E 200 | DoD mục 1 |
| syscall filter | seccomp-bpf blocklist mặc định bật (incl. AF_NETLINK), overhead +0.2ms | D11/D15 |
| network | netns riêng mặc định BẬT — không thấy iface host, không với tới host loopback | D17 |

Isolation: user ns + mount ns + pid ns + uts ns + ipc ns + **net ns**, pivot_root (host fs
tàng hình), capability drop, no_new_privs, seccomp-bpf blocklist (32 syscall
guy hiểm + clone ns-flags + AF_ALG + AF_NETLINK; `MINICT_NO_SECCOMP=1` để tắt cho
do A/B), PDEATHSIG + pidns teardown.
Isolated dirs: tmpfs /tmp; /dev = bind 4 node (null/zero/urandom/random).

## Control plane `minictd` (GĐ2 seed, 2026-11-18)

Daemon rootless (Go stdlib) quản lý nhiều instance qua HTTP API + token:
run/start/stop/remove/inspect/logs/stats và **exec vào instance đang chạy bằng
setns** (`minict-exec`, seccomp cài lại). Cấu hình instance qua `MINICT_SPEC`
(hostname, env, bind[:ro], tmpfs, shm, ulimit, nhiều webport). GĐ1 vẫn là giai
đoạn hiện tại — minictd chỉ giữ xanh + sửa bảo mật (D27). Xem `minictd/README.md`;
test: `make test-minictd` (20 case).

## Quick start

```bash
make -C sandbox              # build minict
bash sandbox/mkrootfs.sh     # build busybox rootfs (~2MB, rootless)
bash tests/test_isolation.sh # 20 isolation tests (netns isolation thật + signal shim)
bash tests/test_seccomp.sh   # 20 seccomp blocklist tests (có control A/B)
bash tests/test_netns.sh     # 7 netns + bridge tests

# lifecycle
./sandbox/minict.sh up demo            # instance với private rootfs copy (netns riêng)
./sandbox/minict.sh web demo 8080      # web: httpd trong netns riêng; host cầu unix-socket
curl -s http://127.0.0.1:8080/         # HTTP 200 — host proxy → bridge → httpd trong ns
./sandbox/minict.sh exec demo -- /bin/sh -c 'echo hi && id && hostname'
./sandbox/minict.sh shell demo         # shell vào instance fs
./sandbox/minict.sh ps
./sandbox/minict.sh down demo
./sandbox/minict.sh bench              # benchmark
```

## Cấu trúc

```
sandbox/minict.c        core (C, 1 file, 0 dependency ngoài libc)
sandbox/mkrootfs.sh     rootfs builder (apt-get download + dpkg-deb -x, rootless)
sandbox/minict.sh       lifecycle CLI (web = netns + unix-socket bridge)
tools/webproxy.py       host-side TCP->unix relay cho web bridge (D16)
minictd/                control-plane daemon (Go) + cmd/minict-exec (setns exec, C) — GĐ2 seed
tests/test_minictd.sh   e2e control-plane (20 cases: auth, exec setns, env, bind symlink, seccomp parity)
tests/test_isolation.sh escape/isolation tests (20 cases)
tests/test_seccomp.sh   seccomp blocklist tests (20 cases, có control A/B)
tests/test_netns.sh     netns + bridge tests (7 cases, có control NS2)
tests/seccomp_probe.c   probe syscall dùng bởi test_seccomp.sh (build static)
Makefile                entry points: build/test/test-escape/test-seccomp/test-netns/test-web/perf-check/cve-check/unicode-check
.github/workflows/      ci, cve-daily, perf-guard, issue-triage, research-log, unicode-guard (PR gate), unicode-clean-daily
benchmarks/             script benchmark (kết quả theo ngày nằm ở journal/)
journal/YYYY-MM-DD/     MỌI tài liệu theo ngày: daily-log, research, bench, decisions/, evidence/, cve/ (quy tắc: journal/README.md)
prompts/                prompt cho agent audit (SIA), retest (SCR), dọn nhánh (SBG); PROMPT.MD (SDDA) ở gốc repo
idea/manifest.md        hướng dự án (đứng trên roadmap)
docs/skills.md          pitfalls + lệnh hữu ích (cập nhật hàng ngày)
docs/roadmap.md         đã xong / đang làm / tiếp theo
tools/cve-check.py      CVE check hằng ngày (NVD API, watchlist trong file)
tools/unicode-guard.py  quét/dọn Unicode ẩn + bidi (Trojan Source); hook local: tools/hooks/pre-commit
docs/cve-watchlist.md   bảng CVE kernel/engine + trạng thái + mitigation
docs/perf-baseline.md   ngưỡng + lịch sử perf
docs/branch-ledger.md   sổ gộp/xoá nhánh (append-only)
```

## Giới hạn đã biết (ngày 1, cập nhật phiên 5)

- Web bind loopback-only trong netns riêng; host truy cập qua unix-socket bridge
  (D16); không httpd.conf/auth/proxy/CGI (CVE-2026-124739/88829 re-check trước khi bật).
- Sandbox không thấy iface host, không với tới host loopback (netns riêng, D17).
  Bridge D16 chỉ serve port-known per instance — multi-service stack cần mở rộng.
- Không /proc trong sandbox (host seccomp chặn mount proc), không cgroup limits
  (env chỉ có cgroups v1), không overlayfs (private copy 2MB/instance thay thế,
  25ms), `exec` tạo ns set mới trên cùng rootfs chứ không setns vào daemon
  (rootless không đủ caps để setns) — mỗi exec còn có netns RIÊNG (không thấy
  httpd của instance web qua loopback).
- SIGTERM từ host vào process trong userns: ĐÃ SỬA (phiên 7, D19) — nguyên nhân
  là semantics pidns-init của kernel (pid_namespaces(7)), không phải môi trường;
  shim pidns-1 cài handler + forward, `minict.sh down` giờ graceful (exit 128+sig).
  Teardown SIGKILL + PDEATHSIG + pidns teardown vẫn giữ làm fail-safe.
  Chi tiết: journal/2026-11-17/decisions/sigterm.md.
- Seccomp là blocklist (default ALLOW) chứ không allowlist — syscall không trong
  blocklist vẫn chạy; allowlist mode parked (D11). Rootfs test probe phải build
  `-static` (rootfs chỉ có busybox, không ld.so/libc).

## Quy trình hằng ngày
A Research → B Develop → C CVE check → D No-Compromise Validation → daily-log.
Chi tiết: docs/roadmap.md (mục "Quy trình hằng ngày").

## Đang đọc
- journal/2026-11-17/research.md — cơ chế userns/mountns/pivot_root + kết quả đo thực nghiệm
- docs/roadmap.md — kế hoạch seccomp-bpf → netns → full web stack
