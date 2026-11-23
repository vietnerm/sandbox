# rootfs-alpine thiếu thư mục rỗng sau git checkout (issue #55, 2026-11-23)

## Root cause (đo được)
`rootfs-alpine/` nằm trong `.gitignore` nhưng 497 file vẫn được track (từ `42f3273`,
D44 giữ nguyên). Git không lưu thư mục rỗng. Một bản clone mới (CI checkout, pristine
clone `/tmp/repro54`) có `bin/busybox` + `etc/alpine-release` nên `mkrootfs-alpine.sh`
đi nhánh "already exists, skipping" và rootfs thiếu đúng tập thư mục rỗng.
Đo trên `git archive HEAD rootfs-alpine`: chỉ có `bin dev etc lib sbin usr var`;
62 thư mục mà apk DB ghi là của gói bị thiếu (home, tmp, media, root, run, srv, sys,
proc, mnt, opt, dev/pts, dev/shm, var/cache, usr/local, ...). 2 lần xanh trong issue
là 2 lần build lại từ tarball, 2 lần đỏ là cây vừa checkout.

## Quyết định
1. `restore_pkg_dirs`: đọc dòng `F:` (thư mục) + `M:` (mode) trong
   `lib/apk/db/installed` của chính cây, `mkdir -p` thư mục thiếu, đặt mode
   (tmp 1777, root 700). Không cần mạng, idempotent, chặn path `..`/tuyệt đối.
2. Gọi ở cả 3 nhánh của script (skip, AUTOADD, build mới); bỏ danh sách mkdir cứng.
3. Các suite alpine/stack/pasta/escape-alpine luôn gọi script thay vì chỉ gọi khi
   thiếu `bin/busybox`; `test_escape_alpine.sh` bỏ `rm -rf` cây tracked.

## Kiểm
`git archive HEAD rootfs-alpine` → script: "restored 62", đủ 17 thư mục gốc,
tmp=1777, root=700, 0 thư mục DB còn thiếu; chạy lần 2 im lặng; `git status
--porcelain rootfs-alpine` sạch (AL9 churn probe không đổi).
