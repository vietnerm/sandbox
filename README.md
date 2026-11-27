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

## Trạng thái tính năng

Nhãn theo Track M M1 (manifest §9, D74): `sẵn sàng` = e2e xanh trên CI; `thử nghiệm` = env-gated SKIP/residual; `kế hoạch` = chưa code.

| Tính năng | Trạng thái | Ghi chú |
|---|---|---|
| minict runtime (userns+pidns+mountns+netns+seccomp+capdrop) | sẵn sàng | isolation 37 + seccomp 25 + netns 7 PASS CI |
| Signal lifecycle (SIGTERM/SIGINT via pidns shim) | sẵn sàng | D19, SIG_STRESS PASS CI |
| Web trong sandbox (busybox httpd via unix-socket bridge) | sẵn sàng | WEB1-9 PASS CI |
| minictd daemon + REST API + token auth | sẵn sàng | MD0-MD54 PASS CI |
| CLI `sandbox` (run/start/stop/remove/inspect/logs/stats) | sẵn sàng | MD20 PASS CI |
| exec vào instance (minict-exec setns) | sẵn sàng | MD5-MD13 PASS CI |
| Template (save-template + `template:` image) | sẵn sàng | MD39 PASS CI |
| Snapshot/restore (`backup:` image, tar.gz trusted) | sẵn sàng | MD40 PASS CI |
| Seccomp allowlist theo template | sẵn sàng | MD41-43 PASS CI (4 profile đo sẵn) |
| Overlayfs rootless (zero-copy rootfs) | sẵn sàng | MD44-46 PASS CI kernel 7.x |
| Landlock LSM (ro-root + fail-closed) | sẵn sàng | LL1-3 PASS CI kernel 7.x |
| Port-forward pasta config-side | thử nghiệm | MD47-49 + MD70 env-gated /dev/net/tun; mặc định fail-closed: bind 127.0.0.1, auto forwarding bỏ (chỉ `pasta_ports`), ns→host + gateway mapping chặn (`public/` opt-in) (#220) |
| cgroups v2 quota (memory/cpu/pids) | thử nghiệm | CG0-5 cần sudo bootstrap CI |
| PTY terminal (vi/top/REPL) | thử nghiệm | host-side PTY + ready handshake (#206 D77); MD62 PASS dev 5.10 + runner; giữ `thử nghiệm` tới khi MD62 xanh ổn định trên CI |
| systemd `--user` unit | thử nghiệm | MD22 chỉ verify cú pháp |
| Observability (/metrics + log JSON) | sẵn sàng | MD50-52 PASS CI |
| Web console (Next.js xuất tĩnh) | sẵn sàng | MD55-60+XSS PASS CI |
| Files view qua execv2 (#208) | thử nghiệm | MD67 PASS; 10 unit parser; giới hạn 1 MB; reset 501 |
| API xoá template/backup + Inspect HostConfig | sẵn sàng | MD61 PASS CI |
| Firecracker microVM (GĐ4) | kế hoạch | scaffolding only, cần /dev/kvm |
| `stack.json` multi-instance (Track M M2) | thử nghiệm | client-side `stack up|down|ps`, ready http/tcp/exec, pasta_ports (#203) |
| Registry template tĩnh (Track M M3) | thử nghiệm | client-side `template push|pull`: index.json + blob sha256-verified trước khi giải nén, HTTPS-only (HTTP loopback + `--insecure`), extractor dùng chung `secureUnpack` (#215) |
| Cluster nhiều VPS (Track M M4) | kế hoạch | chờ owner duyệt |

## Control plane `minictd` (GĐ2)

Daemon rootless (Go stdlib) quản lý nhiều instance qua HTTP API + token:
run/start/stop/remove/inspect/logs/stats và **exec vào instance đang chạy bằng
setns** (`minict-exec`, seccomp cài lại). Cấu hình instance qua `MINICT_SPEC`
(hostname, env, bind[:ro], tmpfs, shm, ulimit, nhiều webport), CLI `sandbox`,
systemd `--user` unit, quota cgroups v2, template. GĐ1 xong 2026-11-20; GĐ2 là giai
đoạn hiện tại từ 2026-11-23. Xem `minictd/README.md`; test: `make test-minictd`.

## Web console (GĐ3 — giai đoạn hiện tại từ 2026-11-26)

UI web single-user dựng từ template Next.js, **xuất tĩnh** (`make console` →
`console/out`) và do chính `minictd --ui-dir console/out` phục vụ trên
127.0.0.1 + token; không có tiến trình Node lúc chạy. Xem `console/README.md`.

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

## Platform Support

Chỉ ghi điều đã đo được. Mức mục tiêu lấy từ `idea/manifest.md` §5 (CI so khớp bằng `tools/check-docs.sh`).

| Thành phần | Yêu cầu | Ghi chú |
|---|---|---|
| Kernel | Linux ≥ 6.1 (mục tiêu), patch LTS mới: tối thiểu 6.1.190 / 6.6.159 / 6.12.113 (2026-11-23) | đã chạy thực tế trên 5.10 (máy dev), 6.18 (container), 7.0 (runner CI); bảng dải CVE theo kernel: [cve-watchlist](docs/cve-watchlist.md) |
| User namespace | unprivileged userns bật | Ubuntu ≥ 23.10: cần `kernel.apparmor_restrict_unprivileged_userns=0` (CI làm bước này) |
| cgroups | v2 cho giới hạn tài nguyên (GĐ2) | minict chạy được không cần cgroups; máy dev ZAI chỉ có v1, đó là giới hạn môi trường, không phải của engine |
| Arch | x86_64 (đã kiểm) | arm64 chưa kiểm: `mkrootfs-alpine.sh` mặc định `x86_64`, CI chỉ chạy x86_64, số syscall seccomp là của x86_64 |
| Mạng | `pasta` (passt) | `tools/fetch-pasta.sh` tải bản pin; thiếu pasta thì chỉ còn bridge loopback (D16) |
| Quyền | uid thường, không sudo, không setuid | |

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
tools/check-docs.sh     chặn README lỗi thời trong CI (link, câu stale, Platform Support vs manifest §5)
tools/unicode-guard.py  quét/dọn Unicode ẩn + bidi (Trojan Source); hook local: tools/hooks/pre-commit
docs/cve-watchlist.md   bảng CVE kernel/engine + trạng thái + mitigation
docs/perf-baseline.md   ngưỡng + lịch sử perf
docs/branch-ledger.md   sổ gộp/xoá nhánh (append-only)
```

## Known Limitations


Mỗi dòng là một câu và một link tới bằng chứng; chi tiết nằm ở link, không chép lại
ở đây. `tools/check-docs.sh` (CI) chặn dòng không có link và các câu đã lỗi thời.

- **Chỉ cho workload tin cậy tới khi có microVM (GĐ4):** instance chia chung kernel với host, một lỗ hổng kernel có thể thoát ra ngoài; namespace/capdrop/seccomp chỉ thu hẹp rủi ro ([cve-watchlist](docs/cve-watchlist.md), [roadmap](docs/roadmap.md)).
- Single-uid mapping: mọi file trong instance thuộc "root", gói cần `chown` sang uid khác phải cài lúc build hoặc bỏ script ([alpine-rootfs](journal/2026-11-17/decisions/alpine-rootfs.md), [web-stack](journal/2026-11-20/decisions/web-stack.md)).
- Seccomp allowlist theo template là **opt-in** (4 profile đo sẵn: busybox/alpine/alpine-nginx/debian; instance không đặt `seccomp_profile` → blocklist D11 vẫn mặc định); workload mới phải đo lại profile bằng `tools/seccomp-profile-gen.sh` trước ([seccomp-allowlist](journal/2026-11-23/decisions/seccomp-allowlist.md), [seccomp](journal/2026-11-17/decisions/seccomp.md)).
- `/proc` trong pidns chỉ mount được khi host không chặn mount; nếu bị chặn, sandbox chạy tiếp không có `/proc` (warn-only) ([proc-pidns](journal/2026-11-19/decisions/proc-pidns.md)).
- Overlayfs rootless env-gated (kernel ≥5.11; máy dev 5.10 SKIP, CI kernel 7.x PASS): mỗi instance mặc định copy rootfs; `MINICT_OVERLAY=1` bật zero-copy ([roadmap GĐ2 mục 3](docs/roadmap.md), [overlay D66](journal/2026-11-26/decisions/observability.md)).
- `minict.sh exec` tạo bộ namespace mới trên cùng rootfs; exec vào instance đang chạy (setns) chỉ có qua `minictd` ([minictd](journal/2026-11-18/decisions/minictd.md)).
- Quota cgroups v2 cần cgroup delegation cho user; không có thì `minictd` fail-soft về rlimit ([cgroups-v2](journal/2026-11-20/decisions/cgroups-v2.md)).
- systemd `--user` unit mới được kiểm cú pháp, chưa chạy thật dưới user manager có lingering ([daemon-service](journal/2026-11-20/decisions/daemon-service.md)).
- Image `oci:`/`tar:` có symlink target tuyệt đối (vd. `/bin/busybox`) bị từ chối fail-closed (D50 `secureUnpack` — reject, không sanitize). Image builder phải dùng **symlink tương đối** (vd. `ln -s busybox` thay vì `ln -s /bin/busybox`) hoặc copies (`busybox --install` không `-s`). Rootfs `base` nội bộ không qua đường này (copy, không `secureUnpack`) nên `mkrootfs.sh --install -s` vẫn dùng được ([D70](journal/2026-11-26/decisions/issue-109-symlink.md), [crane-e2e](journal/2026-11-21/decisions/crane-e2e.md)).
- Chỉ kiểm trên x86_64: bảng syscall seccomp và `mkrootfs-alpine.sh` là của x86_64 ([alpine-rootfs](journal/2026-11-17/decisions/alpine-rootfs.md)).
- Port-forward pasta mặc định **fail-closed**: forward khai báo bind `127.0.0.1` (trước đây mở mọi interface — đo được `*:18024`), auto forwarding (D42) bỏ hẳn — chỉ `pasta_ports` tường minh là đường inbound (UDP auto đo được mở `0.0.0.0:5353` từ ns bind; TCP auto range-scan conflict với forward tường minh), ns→host và gateway→host chặn (đo được sandbox chạm API minictd qua gateway). Muốn mở công khai phải khai `public/<host>:<ns>` tường minh ([#220 D82](journal/2026-11-27/decisions/pasta-failclosed.md)); port preview/wake vẫn qua webproxy bridge D16, không đổi.

## Quy trình hằng ngày
A Research → B Develop → C CVE check → D No-Compromise Validation → daily-log.
Chi tiết: docs/roadmap.md (mục "Quy trình hằng ngày").

## Đang đọc
- journal/2026-11-17/research.md — cơ chế userns/mountns/pivot_root + kết quả đo thực nghiệm
- docs/roadmap.md — kế hoạch seccomp-bpf → netns → full web stack
