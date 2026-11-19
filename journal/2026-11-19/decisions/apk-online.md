# D43 — apk add online trong instance alpine (issue #79, GĐ1 mục 1 residual)

## Ngữ cảnh
Roadmap GĐ1 mục 1 còn residual duy nhất làm được ngay: `apk add` ONLINE
(HTTPS, signature-verified) trong instance. Các lớp trước đã có: pasta egress
(#59, D37), DNS qua `--dns-forward` + inject resolv.conf (#76, D42), alpine
rootfs tracked (#54). Thiếu: (1) không boot được alpine rootfs làm instance —
`up` hardcode `$REPO/rootfs`; (2) không có test chứng minh apk online trong
sandbox.

## Quyết định
1. **`minict.sh up [--rootfs <dir>]`** — tham số tùy chọn, mặc định giữ nguyên
   `rootfs/` (busybox) — zero thay đổi hành vi cho các caller cũ. Fail-closed:
   dir không tồn tại/thiếu `/bin/sh` → lỗi, không rơi về busybox im lặng.
   Private copy per-instance (`cp -a`) giữ nguyên — ownership D20 không đổi.
2. **Test mới trong `tests/test_pasta.sh`** (pasta suite 9 → 11):
   - **PT4** (env-gated tun — SKIP ở dev, PASS thật trên runner, tiền lệ
     NET10-12/PT3): alpine instance `--pasta` → payload chờ tap → retry
     `apk update` (APKINDEX HTTPS qua dl-cdn) → `apk add --no-cache
     acpid-openrc` → marker `APK_ONLINE_OK` + assert `apk info -e` signature-
     verified (không `--allow-untrusted`); host-side assert thêm `apk info`
     của instance rootfs trước `down` (anti-vacuous: log có marker + DB apk
     ghi được vào instance rootfs).
   - **PT4C control** (chạy mọi nơi): KHÔNG pasta trên alpine rootfs →
     `apk update` phải FAIL (netns lo-only, không resolver) — guard
     `PROBE_ALIVE` chống vacuous kiểu M0/CHROOT1 (#54): instance chết ≠ pass.
     Môi trường không alpine rootfs + CDN fail → rc 125 (env-blocked skip),
     không fake-PASS (pattern AL5, SIA PR #24).
3. **Rootfs alpine dùng cho instance**: `$REPO/rootfs-alpine` (tracked, pin
   3.22.8); test build nếu thiếu qua `mkrootfs-alpine.sh` (CDN fail → skip).
4. **Không đụng seccomp/blocklist** — soát g_blocklist: không syscall nào apk
   cần bị chặn (connect/sendto AF_INET = SC19 control đã có; mmap/futex/openat
   allow-by-default). Không đụng pasta flow — D42 giữ nguyên.

## Bị loại
- (b) mirror HTTP cho apk: dl-cdn redirect HTTPS; mất giá trị chứng minh.
- (c) chroot host-side `apk --root`: không chứng minh isolation stack — không
  đưa vào suite, chỉ sanity local.
- (d) bake `--rootfs` vào MINICT_SPEC của minict.c: scope phình (GĐ2), not now.

## Số đo kỳ vọng
- cold_start/rss/disk: KHÔNG đổi (bench đo binary + busybox rootfs, không qua
  minict.sh; rootfs-alpine đã tracked từ #54 — disk_base không đổi).
- Suite: pasta 9 → 11; `make test` 111 → 113 (dev: 108 PASS + 5 SKIP).

## Anti-vacuous checklist (bài học SCR/SIA trước)
- PT4C assert PROBE_ALIVE TRƯỚC khi tin vào thất bại của apk (M0 class).
- Mọi grep expect-fail chạy TRƯỚC `down` (down xoá dir — luật suite).
- PT4 assert CẢ marker trong log LẪN trạng thái DB apk host-side — một marker
  echo không đủ (bài học PT2 superstring, M-version).
