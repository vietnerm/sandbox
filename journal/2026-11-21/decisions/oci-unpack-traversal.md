# D50 — secureUnpack: thay host tar trong đường unpack nội dung không tin cậy

**Ngày**: 2026-11-21 · **Issue**: #104 (type:security priority:p1) · **Nhánh**: `sec/104-oci-unpack-traversal`
**Phát hiện từ**: research cho #103 (A2 oci:) — Bước 4 PROMPT.MD (research phát hiện rủi ro → mở issue security ngay, pivot issue-hôm-nay theo §8)

## Bối cảnh đo được

`prepareRootfs` (minictd) đưa nội dung **không tin cậy** (registry `oci:<ref>`,
operator tar `tar:<path>`) vào `tar -x -C dst --no-same-owner` chạy host-side
uid operator, TRƯỚC pivot_root/capdrop. Ma trận đo GNU tar 1.35
(`journal/2026-11-21/research.md`):

| Vector | GNU tar 1.35 | busybox/GNU cũ | minictd cũ |
|---|---|---|---|
| member `../x` | refuse rc=2 | **extract ra ngoài** | fail-closed vô chủ ý |
| write-through symlink | chặn rc=2 | **ghi host-side** | fail-closed vô chủ ý |
| dotdot-symlink tồn tại trong rootfs | tạo rc=0 | như trên | rò rỉ host-side (residual) |

⇒ fail-closed là **hành vi incidental của host tar**, không phải của minictd.
Mục tiêu triển khai "Linux host nhẹ" không có invariant về tar.

## Quyết định

1. **Thay host tar bằng Go stdlib extractor** (`internal/minict/unpack.go`,
   `secureUnpack`) cho CẢ hai đường `oci:` và `tar:`. stdlib-only (không thêm
   dependency): `archive/tar` + `syscall.Openat/Mkdirat/Mknodat` + raw
   `SYS_SYMLINKAT/SYS_LINKAT` (stdlib không có 2 *at-variants này).
2. **D31 pattern** (webproxy #41): openat O_NOFOLLOW từng thành phần —
   TOCTOU-free, extractor không bao giờ resolve qua symlink do archive trồng.
3. **Reject, không sanitize**: member `..`/absolute, link target `..`/absolute
   → error (không im lặng sửa như tar).
4. **Duplicates refuse** (O_EXCL; chỉ dir bị nhắc lại được — docker images
   lặp `dir/` markers), **setuid/setgid/sticky strip** qua pure
   `stripSpecialBits` (unit-verifiable kể cả trên host LSM strip suid lúc
   create — measured dev env 2026-11-21), **FIFO qua mknodat S_IFIFO**
   (rootless-safe), **char/block refuse** (mknod EPERM rootless — §0).

## Bài học measured trong phiên (ghi để tái dùng)

- `openat(O_CREAT|O_DIRECTORY)` → **ENOTDIR trên kernel 5.10** — mkdirat +
  reopen mới tạo dir được.
- Mẹo "tạo FIFO bằng open O_CREAT|O_EXCL|O_NONBLOCK" → ra **regular file**
  trên Linux — phải mknodat.
- Dev sandbox (z.ai) LSM: **SYS_SYMLINK bị giết** ("Creating symbolic links
  is not allowed." — os.Symlink/python/ln), nhưng **SYS_SYMLINKAT được phép**
  (raw helper chạy được); suid bị strip lúc create; re-open-for-write bị
  EACCES. ⇒ test security phải assert **message/errno do code mình tạo** để
  mutant không bị che bởi env (mẫu: U6 assert "file exists" bắt M5 local).
- `go test` env của dev: `t.TempDir()` OK, tar stream test chạy ổn.

## Mutant matrix (negative control, dev-side)

| Mutant | Catch | Ghi chú |
|---|---|---|
| M1 accept `..` member | 4 tests đỏ | U1/U1b/U2/U4 |
| M2 accept mọi link target | 2 tests đỏ | U3/U4 |
| M3 drop O_NOFOLLOW walk | **0 local** | LSM dev chặn cùng vector (message khác); trên CI vanilla mutant gây missing-error → đỏ. Đặc tính bảo mật (planted file không tồn tại) vẫn assert trực tiếp |
| M4 giữ setuid | 1 đỏ | TestStripSpecialBits (pure) + integration |
| M5 drop O_EXCL | 1 đỏ | U6 assert "file exists" (provenance message) |
| M6 tolerate char devices | 1 đỏ | U11 |

## Residual

- dotdot-symlink entry vẫn được GNU tar cũ tạo — giờ không thể xảy ra qua
  đường minictd (reject). Trên rootfs đặt bằng `dir:<path>` (operator tự
  chuẩn bị) vẫn có thể có symlink lạ — tin cậy operator, ghi GAPS.
- e2e `oci:` với crane thật (#103) — phiên kế.
