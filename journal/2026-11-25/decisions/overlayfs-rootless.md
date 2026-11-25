# D66 — overlayfs rootless env-gated (issue #162, GĐ2 mục 3 residual)

Ngày: 2026-11-25. Nhánh: `feat/162-overlayfs-rootless`.

## Quyết định
Thay private copy 2 MB/instance bằng overlayfs mount **trong userns của shim**,
gate bằng env `MINICT_OVERLAY=1` của daemon (không cờ CLI mới).

## Lý do chọn giải pháp
1. **Mount ở shim, không ở daemon**: daemon chạy uid thường, không có
   CAP_SYS_ADMIN trên fs host; chỉ trong userns riêng (sau unshare + map_ids)
   mới có cap để mount — và kernel ≥ 5.11 cho phép overlay mount trong userns
   với `userxattr`. Đây là điểm mount duy nhất kiến trúc hiện có cho phép
   (bind/tmpfs/dev binds đều ở shim).
2. **Env-gate thay vì mặc định on**: dev kernel 5.10 từ chối overlay mount
   trong userns (đo: EPERM qua `unshare -Ur`); bật mặc định sẽ làm mọi
   instance chết trên dev env. Gate cho phép CI (ubuntu-24.04, kernel 6.x)
   chạy e2e thật trong khi dev giữ đường copy cũ — cùng binary, cùng test
   suite, không fork hành vi âm thầm.
3. **Zero copy cho `base`/`dir:`/`template:`**: lower chỉ thẳng vào nguồn.
   Overlay lower là read-only theo thiết kế (copy-up ghi vào upper), nên
   chia sẻ lower giữa nhiều instance an toàn — đây chính là lợi ích "thay
   private copy" mà roadmap đòi. `tar:`/`oci:`/`backup:` vẫn phải unpack vào
   `<dir>/lower` (archive → dir là bắt buộc), đổi tên đích từ `rootfs` →
   `lower`.
4. **Fail-closed khi mount overlay lỗi**: không fallback về bind+copy —
   fallback ẩn biến gate thành lie (instance "overlay" thực chất full copy).
   Die() với message chỉ rõ nguyên nhân (kernel < 5.11 / fs không hỗ trợ).
5. **`userxattr` tường minh**: unprivileged không ghi `trusted.overlay.*`;
   copy-up cần lưu origin xattr → `user.overlay.*` là namespace duy nhất khả
   dụng (kernelnewbies 5.11 + kernel docs).
6. **Snapshot từ chối rõ ràng trên instance overlay**: host-side `<dir>/rootfs`
   là mountpoint rỗng (overlay mount chỉ tồn tại trong mountns của shim) →
   tarGzTree sẽ tạo archive rỗng NGHĨA LÀ MẤT DỮ LIỆU NGẦM. Refuse với lỗi rõ
   > archive rỗng. Merge whiteout-aware (lower+upper) để residual D57.
7. **readonly + overlay = lower-only mount**: không upper → mọi write EROFS,
   khớp semantics `--read-only` (Landlock ro-root vẫn áp như cũ — defense in
   depth độc lập).

## Những gì KHÔNG làm trong D66
- idmapped mounts (kernel ≥ 5.12, overlayfs idmap ≥ 5.19) — residual mục 3.
- zstd compression (D57 residual, cần quyết định dep).
- Overlay-aware snapshot/backup (merge lower+upper + whiteout) — residual.
- Port-forward pasta config-side (mục 4), observability (mục 7), systemd-real
  (mục 1 residual) — các phiên sau, owner đã liệt kê thứ tự.

## Mutant kế hoạch (SIA sẽ chạy)
- M-ovl-shim: shim bỏ mount overlay (bind rootfs rỗng) → payload chết /
  marker không thấy → MD44 bắt.
- M-ovl-copy: prepareRootfs vẫn copy full rootfs trong overlay mode → du
  assertion (thin-provision) MD44 bắt.
- M-ovl-render: renderSpec không phát directive overlay → shim chạy bind
  rootfs rỗng → payload chết → MD44 bắt.
