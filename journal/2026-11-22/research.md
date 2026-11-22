# Research — 2026-11-22 (SDDA Phiên 17, issue #113 / GAPS S3)

Chủ đề: recursive read-only bind — `mount_setattr(AT_RECURSIVE)` vs per-submount
remount; quyền mount trong userns lồng nhau.

## Nguồn 1 — mount_setattr(2) man page
- Link: https://man7.org/linux/man-pages/man2/mount_setattr.2.html
- `AT_RECURSIVE` = "Change the mount properties of the entire mount tree";
  `MOUNT_ATTR_RDONLY` = tương đương MS_RDONLY per-mount. syscall mới ở
  **kernel 5.12** (glibc wrapper 2.36) → trên kernel cũ gọi raw syscall trả
  **ENOSYS** (đo được trên dev 5.10.134: errno 38). Áp dụng cho minict: nhánh
  chính cho `bind :ro` — một syscall set ro CẢ subtree, không TOCTOU.

## Nguồn 2 — util-linux issue #2247 (mount_setattr fallback reality)
- Link: https://github.com/util-linux/util-linux/issues/2247
- `mount(8)` thử mount_setattr kể cả khi kernel không hỗ trợ → fail trên
  5.10 LTS (mount_setattr thêm ở 5.12). Bài học: mọi consumer thật đều phải
  có đường fallback khi ENOSYS — chính là lý do minict cần nhánh
  per-submount cho 5.10 (máy dev) thay vì die() vô điều kiện.

## Nguồn 3 — mount(8) semantics cho --rbind ro (the documented fallback)
- Link: https://man7.org/linux/man-pages/man8/mount.8.html
- "This recursively binds filesystems from /foo to /bar, making /bar and all
  submounts read-only": mount(8) đạt hiệu ứng này bằng cách remount
  **từng mount con** (pre-5.12) / mount_setattr recursive (≥ 5.12). Lịch sử:
  ro-bind chỉ tồn tại từ kernel 2.6.26 (LWN "Read-only bind mounts",
  https://lwn.net/Articles/281157 — một mount() với MS_BIND|MS_RDONLY bị
  kernel bỏ qua MS_RDONLY, bắt buộc remount step thứ hai — đúng comment
  hiện có trong `apply_spec_binds`).

## Nguồn 4 — mount_namespaces(7): ownership/MNT_LOCKED
- Link: https://man7.org/linux/man-pages/man7/mount_namespaces.7.html
- "Mounts that come as a single unit from more privileged mount are locked
  together and may not be separated in a less privileged mount namespace"
  (MNT_LOCKED). Đo xác minh 2026-11-22 (probe rec3 + minict thật): mount do
  userns cha tạo ra **được rbind-clone bởi userns con chạy minict** (cửa sổ
  bị đoán sai trước đó: EPERM probe đầu là do rbind trong mountns CHIA SẺ,
  không phải ownership) — và **remount ro per-submount từ userns con THÀNH
  CÔNG** (rec3: "SUB bind-remount ro: OK" → sub EROFS). Đây là cơ sở của
  fallback < 5.12.

## Đo đạc quyết định thiết kế (dev kernel 5.10.134, uid 1001)
1. **Bug S3 tái hiện e2e (pre-fix)**: harness `unshare --user
   --map-root-user --mount` tạo tmpfs submount trong cây src → minict thật
   với `bind src:/data:ro`: boot OK, `touch /data/top.txt` → EROFS, nhưng
   `touch /data/sub/inner.txt` → **THÀNH CÔNG** (submount rw) — chính gap
   GAPS.md S3.
2. **mount_setattr ENOSYS trên 5.10** → nhánh chính chỉ chạy trên runner
   (kernel 6.x).
3. **Post-fix e2e trên 5.10**: cùng harness → submount giờ **EROFS**
   (per-submount remount từ enumeration `/proc/self/mountinfo` host-side
   pre-unshare — cửa sổ duy nhất còn /proc; trong mountns mới không thể
   mount proc: EPERM đo được, D39).
4. Mutant M-A/M-B/M-C đều bị bắt (chi tiết decision D52).

## Áp dụng cho minict (D52)
- `mount_setattr(AT_FDCWD, dst, AT_RECURSIVE|AT_SYMLINK_NOFOLLOW,
  {MOUNT_ATTR_RDONLY})` khi syscall tồn tại; ENOSYS → remount từng submount
  (từ mountinfo host-side) rồi top; mountinfo không đọc được → die()
  fail-closed. Không đụng seccomp → không cần đồng bộ `minict-exec.c` (D25).

---

# Phiên 18 — S4 unix-socket listener (issue #116)

**Vấn đề:** daemon chỉ serve HTTP trên TCP loopback; bearer token đi clear trong
header — user cùng host khác connect được nếu biết port, và transport không đủ
cho non-loopback (VPS cluster). GAPS S4 đề xuất "unix-socket listener or TLS".

**Giải pháp đã có trong các dự án tương tự:**
1. **unix(7) (man7.org/kernel.org):** pathname unix socket honor permissions
   của directory chứa nó; tạo socket cần write+search trên dir; socket file
   0600 + dir 0700 → chỉ uid owner connect được. Không cần PKI, không root.
   https://man7.org/linux/man-pages/man7/unix.7.html
2. **podman-system-service(1):** rootless REST API mặc định listen
   `unix:///run/user/<uid>/podman/podman.sock`; mô hình bảo mật "built upon
   access via a Unix socket with access restricted via standard file
   permissions" — precedent thiết kế trùng khớp mục tiêu minict (rootless,
   user-level control plane).
   https://docs.podman.io/en/latest/markdown/podman-system-service.1.html
3. **Docker daemon socket precedent:** `/var/run/docker.sock` root:docker
   0660 — control-plane socket perms CHÍNH là access control; 0666 = any
   local user full daemon access (class lỗi đã biết).
   https://docs.docker.com/engine/daemon/remote-access/#daemon-socket
4. **SO_PEERCRED (unix.stackexchange / Go recipe):** daemon xác thực peer uid
   không cần token — COULD dùng, nhưng minictd giữ token trên MỌI transport
   (R3): same-uid process vẫn connect được socket; auth = phòng thủ sâu, đồng
   thời giữ một code-path auth duy nhất (mảng MD2 chạy trên cả TCP lẫn unix).
   https://unix.stackexchange.com/questions/556068

**Trade-off:** unix socket chỉ bảo vệ LOCAL (không thay TLS cho multi-node
qua mạng) — đủ theo chữ GAPS ("before any non-loopback use" = chặn non-loopback
clear-text BẰNG không expose TCP public + cho local path an toàn); TLS là
residual khi cluster thật cần cross-host. Chọn unix socket trước vì rootless,
không PKI, precedent podman.

**Áp dụng cho minict:** `--unix-socket PATH` fail-closed: dir 0700 (chmod nếu
lỏng, die nếu chmod lỗi), socket 0600, refuse symlink + non-socket file tại
path (chống hijack), replace stale socket (boot lại sau crash), token auth
giữ nguyên; TCP listener trở thành opt-in (rỗng = tắt — unix-only posture).

---

# Research — 2026-11-22 (SDDA Phiên 19, issue #121 / GAPS S5)

Chủ đề: spec rlimits cho exec'd processes — setrlimit lifecycle qua setns +
execve; CAP_SYS_RESOURCE window; precedent runc/libcontainer exec.

## Nguồn 1 — setrlimit(2) man page
- Link: https://man7.org/linux/man-pages/man2/setrlimit.2.html
- setrlimit đặt soft/hard limit **per-process**; limit survives `execve(2)`
  (exec KHÔNG reset rlimits) → apply trong child của minict-exec trước
  `execvp` sẽ tác động lên cmd chạy. Raising hard limit above current cần
  `CAP_SYS_RESOURCE` (capabilities(7)); lowering không cần cap. Áp dụng:
  apply trong child SAU khi join userns U1 (full cap set) và TRƯỚC
  `drop_all_caps()` — D23 parity.

## Nguồn 2 — setrlimit(2) descriptions + limits(7)
- Link: https://man7.org/linux/man-pages/man7/limits.7.html
- `RLIMIT_NOFILE`: max fd+1 (soft = default open max, hard = ceiling).
  `RLIMIT_NPROC`: max processes per **real UID** — shared across processes
  cùng uid (S2 hazard: per-uid, không per-instance). "unlimited" =
  `RLIM_INFINITY` (`rlim_t` = ~0ULL). soft ≤ hard, hard ≤ RLIM_INFINITY.
  Áp dụng: parser hỗ trợ "unlimited" (parity `minict.c spec_parse_rlim`).

## Nguồn 3 — minict.c apply_spec_rlimits + D23 (repo nội bộ)
- Link: `sandbox/minict.c` dòng 1204-1220; decision D23.
- Payload path: `apply_spec_rlimits()` chạy BEFORE `drop_all_caps()` —
  raising hard limit cần CAP_SYS_RESOURCE held trong own userns đến capdrop.
  Sau capdrop chỉ lower được. **Cùng luật áp dụng cho exec path**: child của
  minict-exec có full cap U1 (join user ns) trước `drop_all_caps()` của chính
  nó → window raise hard limit tồn tại. Trước S5, exec path chỉ set
  RLIMIT_CORE=0 (`harden_exec_fds`, D33) và bỏ qua spec rlimits = lỗ.

## Nguồn 4 — runc/libcontainer exec precedent (setns + rlimit re-apply)
- Link: https://github.com/opencontainers/runc/blob/main/libcontainer/process/exec.go
- runc exec joins container ns qua setns (cùng model RootlessKit/podman mà
  minict-exec D25 dùng) và re-applies spec rlimits trong exec'd process trước
  exec. Precedent: exec KHÔNG inherit container payload's rlimits tự động
  (rlimit per-process, không cross process) → phải re-apply tường minh.

**Trade-off:** truyền rlimit qua env-file với prefix `MINICT_RLIMIT_` (thay
thêm flag `--rlimit` repeatable) — đơn giản cho daemon, nhưng loader phải tách
riêng để không leak vào env cmd (R2). PidsLimit NOT mapped cho exec: cgroups
v2 (D48) đã bound exec'd process qua instance cgroup `pids.max`; không cgroups
→ per-uid nproc mang hazard S2, để payload path alone.

**Áp dụng cho minict:** `minict-exec.c` — `record_rlimit()` vào mảng riêng +
`apply_spec_rlimits()` trong child SAU `harden_exec_fds` TRƯỚC
`drop_all_caps`; daemon `exec.go` load `spec.json`, append
`MINICT_RLIMIT_<NAME>=soft[:hard]` vào env-file. Hai resource NOFILE+NPROC
parity payload. Test MD37: `ulimit -n` trong exec == spec nofile.
