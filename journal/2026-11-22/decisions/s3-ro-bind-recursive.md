# D52 — S3: recursive read-only bind (issue #113, GAPS.md S3)

Ngày: 2026-11-22 · Phiên 17 · Nhánh: `sec/113-s3-ro-bind-recursive`

## Bối cảnh

`apply_spec_binds()` thực hiện bind bằng `MS_BIND|MS_REC` rồi remount ro
bằng MỘT lệnh `MS_BIND|MS_REC|MS_REMOUNT|MS_RDONLY`. Kernel chỉ áp
MS_RDONLY cho **top mount**; submount clone theo rbind **giữ rw** — operator
yêu cầu `:ro` nhưng còn lỗ ghi. Đây là gap S3 trong `minictd/docs/GAPS.md`
(có từ handoff, comment trong code thừa nhận). GĐ2 mục 1 còn S3–S6; kế hoạch
Phiên 16 chọn S3 theo thứ tự GAPS.

Đo được trước khi sửa (dev 5.10.134, uid 1001, minict thật): submount tmpfs
trong cây bind vẫn ghi được sau khi top đã EROFS (evidence
`journal/2026-11-22/evidence/s3-ro-bind-recursive.txt`, phần pre-fix).

## Quyết định

**Enforce ro trên CẢ cây, hai nhánh theo kernel:**

1. **Nhánh chính — `mount_setattr(AT_RECURSIVE, MOUNT_ATTR_RDONLY)`**
   (kernel ≥ 5.12): một syscall, kernel đi cả tree dưới VFS lock — không
   TOCTOU, không phụ thuộc enumeration. syscall 442 khai tay cho x86_64
   (D14: số syscall lấy từ unistd_64.h); arch khác lấy từ
   `<sys/syscall.h>` nếu header đủ mới, không có → rơi thẳng fallback.

2. **Fallback — per-submount remount (kernel < 5.12, ENOSYS đo được trên
   dev):** enumerate mount points **ngiêm ngặt bên dưới** src host-side
   **pre-unshare** từ `/proc/self/mountinfo` (cửa sổ duy nhất còn /proc —
   trong mountns mới không mount được proc: EPERM đo được, D39), lưu
   src-relative (`/sub`); sau rbind remount từng submount
   (`MS_BIND|MS_REMOUNT|MS_RDONLY` — quyền đo được: mount clone thuộc
   userns con, remount per-submount OK trên 5.10, probe rec3) rồi remount
   top như cũ.

3. **Fail-closed:** mountinfo không đọc được trên kernel không có
   mount_setattr → `die()` (không boot container ro-không-đầy-đủ); > 16
   submount/bind → `exit(125)` (cap, D17-style); lỗi remount bất kỳ →
   `die(why)` — cùng posture với bind không thiết lập được ("a bind that
   fails to establish means the container is silently NOT what the operator
   asked for").

## Phương án đã loại

- **Die() khi ENOSYS + có submount** (không fallback): đơn giản hơn nhưng
  mất tính năng `:ro` trên kernel LTS 5.10 cho case mà fallback xử lý đúng
  được (đo được) — từ chối.
- **Chỉ warn (giữ hành vi cũ)**: giữ nguyên lỗ ghi im lặng — против trực
  tiếp giá trị fail-closed của repo — từ chối.
- **Walk st_dev post-bind thay mountinfo**: không phát hiện bind
  cùng-superblock bên trong cây (st_dev trùng); mountinfo chính xác hơn và
  vẫn chạy host-side trước unshare — chọn mountinfo.

## Residual (khai minh bạch)

- Submount được tạo TRONG mountns khác (non-ancestor) không thể vào cây bind
  (MNT_LOCKED/ownership — probe đo được EPERM) → không phát sinh case ro
  một-phần từ class này; case thật (admin mount trên host) nằm trong
  enumeration.
- Nhánh `mount_setattr` (runner ≥ 5.12) chỉ đo e2e trên CI; dev 5.10 luôn
  đi fallback. Mutant cho nhánh này (drop AT_RECURSIVE) phải đo trên runner.
- File bind (`:ro` trên file) không có submount — fallback top-only là đủ
  (đúng).
- `MINICT_SPEC` spec của minictd (`volumes: src:dst:ro`) tự hưởng fix —
  không đổi Go code; MD8 không regress (cây phẳng, top remount giữ nguyên).

## Test + mutant (tests/test_isolation.sh, section BIND)

- **BIND1** control cây phẳng `:ro`: nội dung đọc được + top EROFS.
- **BIND2** cây CÓ tmpfs submount `:ro`: top EROFS **và submount EROFS**
  (assert giống nhau cả hai kernel; tên cơ chế in trong mô tả case —
  dev: mountinfo-fallback, runner: mount_setattr).
- **BIND3** control chống-vacuous: cùng cây KHÔNG `:ro` — top+submount ghi
  được thật.
- Mutant (scripts/mutant_s3.sh, dev 5.10): M-A (submount remount mất
  MS_RDONLY) → **BIND2 đỏ**; M-B (top remount mất MS_RDONLY) → **BIND1+BIND2
  đỏ**; M-C (mountinfo poisoned → fail-closed die) → **BIND1+BIND2 đỏ**
  (boot refused). BIND3 xanh xuyên suốt → 0 vacuous. Mutant nhánh
  mount_setattr: CI-only (residual).

## Bằng chứng

- `journal/2026-11-22/evidence/s3-ro-bind-recursive.txt` (pre-fix repro +
  post-fix PASS + mutant matrix + perf + baseline)
- `journal/2026-11-22/cve/s3-ro-bind-recursive.{md,json}` (rc=0, 0 CVE mới
  ngoài watchlist)
