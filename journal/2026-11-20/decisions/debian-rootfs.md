# D46 — Debian rootfs: boot trong minict + apt online qua pasta (issue #88)

**Ngày:** 2026-11-20 (Phiên 11) · **Nhánh:** `feat/88-debian-rootfs` · **Tiếp nối:** D37 (pin+sha256), D42 (DNS pasta), D43 (`--rootfs` + PT4/PT4C), D44 (pristine guard AL9)

## Vấn đề

GĐ1 mục 1 còn đúng một residual: **Debian rootfs** ("Alpine trước, Debian sau" — roadmap).
Blocker đo được: `minict` binary sanity-check rootfs hardcode `/bin/busybox`
(`minict.c:1343`) — Debian không có busybox ⇒ mọi Debian rootfs bị chặn trước
khi mount (`minict: no busybox in ...`, đo Phiên 11 trên rootfs đã tải về).

## Quyết định

1. **Sanity check `/bin/sh` thay `/bin/busybox`** (fail-closed giữ nguyên):
   `/bin/sh` là thứ wrapper luôn exec (payload mặc định `/bin/sh -c`), busybox
   và alpine đều có nên vẫn pass; Debian `/bin/sh -> dash` (symlink TƯƠNG ĐỐI,
   khác alpine dùng symlink TUYỆT ĐỐI `/bin/sh -> /bin/busybox` chỉ có nghĩa
   trong rootfs) — resolve vòng lặp readlink: target tuyệt đối ghép `rootfs`,
   tương đối ghép `rootfs/bin/`. Cùng luật fail-closed với wrapper (D43).
   Seccomp blocklist KHÔNG đụng ⇒ `minict-exec.c` giữ nguyên (MD0 sync D25).
2. **Nguồn rootfs = debuerreotype/docker-debian-artifacts** (branch
   `dist-amd64`, `bookworm/slim/oci/blobs/rootfs.tar.gz`) — chính là rootfs
   của image `debian:bookworm-slim` Docker chính thức, build bởi debuerreotype
   0.17 từ snapshot.debian.org (epoch `1794787200` = 2026-11-16, Debian 12.15).
   Pin: URL + sha256 `2a7a0929687462067917a1ed0236948de0b43ef9a546c9a930540f6218d7dfd0`
   + size `28247934` — **verified bằng tải về + sha256sum cùng phiên** (khớp
   layer digest trong `image-manifest.json` của chính repo nguồn). Fail-closed
   trước khi unpack (D37 pattern).
3. **rootfs-debian gitignored** (~83 MB unpacked, đo được): không track git
   (trái với rootfs-alpine 13.3 MB tracked) — fetch lại khi thiếu (same class
   rootfs-alpine D36 / pasta binary D34-D37). CI tải ~28 MB mỗi run (~10-20 s).
   `make rootfs-debian` + fetch tự động trong suite (rc 125 = SKIP env-blocked,
   không fake-PASS).
4. **Suite mới `tests/test_debian.sh` (8 case DB1–DB8)** mirror đúng các class
   đã có: DB3 = AL4/PROC (env-aware D39), DB4 = AL6 ownership (probe
   `/var/tmp` — **/tmp là tmpfs trong sandbox**, file không bao giờ thấy
   host-side — đo Phiên 11), DB5 = AL7 chown EINVAL (GNU coreutils message
   cùng class "Invalid argument"), DB6 = PT4C control no-pasta (anti-vacuous
   PROBE_ALIVE + **wait_for_exit** — apt trên lo-only có thể chạy tới 60 s,
   không tin "không có marker" trước khi payload EXIT), DB7 = PT4 online
   (2-step anti-vacuous #80: `dpkg -s tree` phải FAIL trước install;
   signature verify là mặc định bắt buộc của apt/gpgv — không tồn tại
   `--allow-untrusted`), DB8 = AL9 pristine trên dir gitignored (content
   manifest hash thay git-churn probe).

## Các lựa chọn bị loại

- **images.linuxcontainers.org (lxc/distrobuilder):** date-pinned + SHA256SUMS
  GPG-signed tốt, nhưng rootfs ~100 MB+ và kèm systemd/openrc — nặng hơn nhu
  cầu (payload của minict là dash/direct exec, không cần init system). Ghi làm
  nguồn fallback trong research.md.
- **debootstrap tự build:** cần cài công cụ + mirrors mutable, không pin
  digest được như OCI manifest.
- **Track rootfs-debian trong git:** 83 MB × mọi clone — quá nặng cho repo
  với disk budget 20 MB (D34); fetch pin đã đủ reproducibility.
- **Giữ nguyên check busybox + thêm busybox vào debian rootfs:** "nâng" busybox
  vào debian chỉ để qua check = lười hoá gốc rễ; check sai về mặt ngữ nghĩa
  (rootfs cần shell, không cần busybox).

## Phạm vi + residual

- **Không đụng:** seccomp blocklist, capdrop, NNP, netns mặc định, wrapper
  web/stack paths. Regression: 9 suites `make test` dev = failed none
  (isolation 31 + seccomp 21 + netns 7 + alpine 9 + debian 7P/1S + escape 31 +
  cve 4 + pasta 5P/6S + stack 8).
- **Residual (ghi roadmap):** apt chạy qua **HTTP** `deb.debian.org` (HTTPS
  runtime chưa có trong rootfs-slim — cùng class alpine trước #74; cài
  `ca-certificates` sẽ mở, phần việc sau nếu cần); `_apt` uid 42 unmapped ⇒
  apt warning "Download is performed unsandboxed as root" (documented, rc=0);
  DB7 (online) env-gated tun — kết quả thật trên runner CI; disk_base_mb gate
  vẫn đo rootfs busybox (2.04 MB) — debian đo supplementary.
