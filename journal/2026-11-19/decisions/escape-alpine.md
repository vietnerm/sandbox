# D36 — Chạy trọn escape suite trên rootfs distro thật (alpine) — residual #5

- **Ngày**: 2026-11-19 (Phiên 2 — chu trình tăng tốc theo chỉ thị owner,
  xem D35)
- **Nhánh**: `test/5-escape-alpine` — đóng residual cuối của roadmap GĐ1
  mục 2: "chạy lại cả suite trên rootfs distro thật (có /proc…, nhiều mặt
  tấn công hơn busybox)". Issue #5 đã merge PR #48; PR này hoàn tất phần
  còn lại (residual), không mở lại issue (ghi trong PR body link #5).

## Quyết định
1. **`tests/test_isolation.sh` nhận `ISOLATION_ROOTFS` env** (mặc định giữ
   nguyên `$REPO/rootfs`) — một suite, hai rootfs. Suite đọc rootfs từ env,
   mọi case khác giữ nguyên.
2. **`tests/test_escape_alpine.sh`** — wrapper: bảo đảm rootfs-alpine tồn tại
   (tái sử dụng rootfs đã build bởi test_alpine.sh khi chạy qua run-suites;
   tự build khi chạy standalone) rồi chạy isolation với
   `ISOLATION_ROOTFS=rootfs-alpine`.
3. **run-suites**: thêm suite thứ 5 (sau test_alpine.sh để tận dụng rootfs
   đã build). `make test` tổng mới: 29 + 21 + 7 + 8 + 29 = 94. Makefile:
   target `test-escape-alpine` (build rootfs-alpine + chạy suite) — CI chạy
   tự động qua `make test` mà không sửa workflow (tránh G7).
4. **Sửa rootfs thật (phát hiện khi chạy)**: `mkrootfs-alpine.sh` hiện tại
   đã tạo `/home/minict` + chmod /tmp, nhưng rootfs-alpine CŨ trong máy dev
   được build trước khi các dòng đó có → thiếu `/home`, `/tmp` ("mount /tmp
   (child): No such file or directory", "can't create /home/iso"). Rebuild
   từ đầu sửa hết — **không sửa script**, chỉ invalidate cache cục bộ. (Bài
   học: suite chạm mount/tmpfs phải rebuild rootfs khi script đổi.)
5. **CHROOT1 gọi applet tường minh `/bin/busybox chroot`**: đo được alpine
   có applet chroot trong busybox NHƯNG không có symlink `/bin/chroot` →
   `chroot` bare chết rc=127 → expect=fail PASS GIẢ (vacuous trên alpine).
   Applet-explicit chạy đúng trên CẢ HAI rootfs (busybox .deb có cả hai
   hình thức). Đây là class bug mới cho SIA: "expect=fail + lệnh không tồn
   tại trong rootfs = vacuous".

## Kết quả đo (máy dev, kernel 5.10.134, uid 1001)
- busybox rootfs: 29/29; **alpine rootfs: 29/29** (mọi contract giữ nguyên
  trên musl + /etc thật + passwd marker của alpine).
- FD1/CORE1/SETUID2 hardening binary-agnostic (minict.c, không phụ thuộc
  rootfs) — PASS cả hai.
- Mutant chờ SCR xác nhận trên alpine: fd-mutant → FD1+CORE1 đỏ (binary
  đổi tên `minict` đúng comm).

## Residual
- Suite alpine KHÔNG bật on CI với mutation matrix riêng — negative control
  cho lớp alpine dùng cùng mutant binary (fix nằm ở minict.c, rootfs không
  ảnh hưởng hành vi close_range/RLIMIT_CORE). Đủ theo D32/SIA rule.
- PROC1/CG1 vẫn xfail (known gap, không đổi).
- Debian rootfs: "sau" theo roadmap — không mở việc hôm nay.
