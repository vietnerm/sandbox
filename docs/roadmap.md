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
- [x] 2026-11-17: **minict v0.1** — mini-container rootless (userns+mountns+pidns+utsns+ipcns,
  pivot_root, capdrop, NNP, PDEATHSIG, group-kill), busybox rootfs builder, lifecycle CLI
  (up/exec/shell/down/ps), 16 test isolation, benchmark đầy đủ. Cold start 1.2ms, RAM ~1.1MB, disk 2MB.

## Đang làm
- [ ] (nếu tiếp tục hôm nay) test song song nhiều instance + stress rollback

## Tiếp theo (theo thứ tự ưu tiên cho các ngày tới)
1. **seccomp-bpf tối thiểu cho sandbox** — filter sau capdrop: chặn open_by_handle_at,
   keyctl, bpf, setns ra ngoài... (backlog #3; kernel 5.10 OK; dùng prctl+ BPF tự viết,
   đo overhead syscall)
2. **netns + networking cho web stack** — slirp4netns/pasta không có trong env →
   tự làm: netns loopback-only + unix-socket proxy qua rootfs path, hoặc veth cần
   CAP_NET_ADMIN (đo xem userns có cho phép tạo veth trong ns riêng không)
3. **Full web stack trong minict** (Definition of Done tổng thể): nginx/caddy + API +
   SQLite bên trong 1 instance; `./sandbox up|down|bench` một lệnh; cold start stack < 5s
4. **chroot-escape test sâu hơn** — fd-based escape, /proc/self/root tricks, setuid
   binary trong rootfs (NNP phải chặn), core dump leak
5. **cgroups v1 delegation** (nếu host cho phép) hoặc đánh dấu parked vì v1 không rootless-friendly
6. Wasmtime/WASI cho backend — so sánh với minict về RAM/startup/isolation model
7. overlayfs rootless + idmapped mounts — parked cho tới khi kernel ≥5.15 (CVE-2023-0386)

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
- Tiến độ: **2/5** (rootless ✅, escape test cơ bản ✅; còn stack, netns, một-lệnh-stack)
