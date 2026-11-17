# Decision Log — 2026-11-17

## D1: PoC mini-container tự viết thay vì dùng bwrap/nsjail — CHẤP NHẬN

**Bối cảnh:** không có bwrap/nsjail/podman trong env; có unshare+gcc; userns rootless hoạt động.

**Quyết định:** viết `minict.c` (~260 dòng C): userns+mountns+pidns+utsns+ipcns,
self uid_map, MS_PRIVATE, bind rootfs, pivot_root + umount old root (fail-closed),
bind /dev bits, tmpfs /tmp, capdrop, no_new_privs, PDEATHSIG, process-group kill.

**Số liệu quyết định:** cold start p50=1.2ms, RAM ~1.1MB, disk rootfs 2.0MB —
thắng mọi target (2s / 30MB / 50MB). bwrap phải cài thêm (không có sẵn), nsjail
thường cần build từ source. Trade-off: mình lo phần edge case (đã gặp 4 ngay ngày 1).

## D2: KHÔNG dùng overlayfs trong userns trên kernel 5.10 — CHẤP NHẬN (parked)

**Lý do:** CVE-2023-0386 (đã verify NVD) là lỗ hổng escalation đúng trong scenario
overlayfs+userns; kernel 5.10.134 này không rõ đã backport đủ hay chưa → rủi ro vô ích.

**Thay thế:** private copy rootfs cho mỗi instance (`cp -a`, đo được 25ms cho 2MB).
Với rootfs 2MB, chi phí chấp nhận được; overlayfs chỉ có lợi khi rootfs lớn.

**Revisit khi:** kernel ≥5.15 hoặc có chính sách bảo mật cho phép + cần rootfs >50MB.

## D3: Fallback /dev bằng bind-mount thay vì mknod — CHẤP NHẬN

**Đo được:** mknod device node EPERM trong userns trên kernel này (devices cgroup v1
hoặc LSM). Bind-mount /dev/null,zero,urandom,random từ host vào rootfs **trước khi
pivot_root** hoạt động và chỉ lộ 4 node vô hại (không phải cả /dev như docker --privileged).
Đã thử mount tmpfs vào /dev rồi mknod: mount OK nhưng mknod fail → bind là đường duy nhất.

## D4: Cleanup dùng SIGKILL + PDEATHSIG + pidns teardown, không dựa SIGTERM — CHẤP NHẬN

**Phát hiện đo được (ngày 1, quan trọng nhất):** `kill -TERM` từ host vào process
trong userns → rc=0 nhưng không delivery (A/B/C matrix: host-sleep chết, minict-process
sống; SigPnd=0; SIGKILL được delivery). Giả thuyết: host seccomp/LSM chặn cross-userns
signal fake-success.

**Thiết kế do đó:**
- wrapper: handler SIGTERM/SIGINT → kill(-pgid) (đúng cho kernel thường, vô hại ở đây)
- child: PR_SET_PDEATHSIG=SIGKILL → parent chết thì tree chết
- PID 1 của pidns chết → kernel dọn hết process trong ns (teardown tự động)
- `minict.sh down`: TERM (best-effort) → KILL sau 0.5s (đảm bảo)

## D5: `exec` KHÔNG setns vào instance đang chạy — CHẤP NHẬN (hạn chế đã ghi)

**Lý do:** rootless — ta không có CAP_SYS_ADMIN trong userns của instance để setns vào.
Options: (a) exec = ns set mới trên cùng rootfs (chọn — dùng được ngay),
(b) supervisor nhận lệnh qua unix socket (làm khi cần), (c) bỏ `up` daemon.
Trade-off: các process "exec" không thấy process của nhau (pidns riêng) — đủ cho
dev/CI use-case hôm nay; ghi rõ trong README để không ai nhầm là docker exec.

## Kẹt >30 phút? Không. Tổng thời gian vòng lặp: ~2h (research 40m, code+debug 50m, docs 30m).
