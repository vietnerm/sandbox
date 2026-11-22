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
