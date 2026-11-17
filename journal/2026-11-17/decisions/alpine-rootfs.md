# D20 — Alpine minirootfs làm rootfs distro thật (issue #23, GĐ1 mục 2)

Ngày: 2026-11-17 (phiên 8) · Nhánh: `feat/23-alpine-rootfs` · Tiếp nối D19 (sigterm, PR #21).

## Bối cảnh
Roadmap GĐ1 mục 2 yêu cầu rootfs distro thật (Alpine trước, Debian sau), đo
`/proc` trong pidns trên runner, init tối thiểu, và ghi rõ gói nào hỏng dưới
single-uid mapping. Ràng buộc bất biến: rootless tuyệt đối, **không newuidmap
setuid** (manifest §5), không Docker.

## Quyết định

1. **Pin `alpine-minirootfs-3.22.8-x86_64` + verify `.sha256` CDN trước extract**
   (D20-1). Không pin = test không tái lập; CDN publish sha256 riêng cho từng
   tarball — verify bắt buộc, không tin mù download (bài học D9 về false positive).
2. **Ownership: GNU tar non-root `--no-same-owner` = fakeroot tự nhiên** (D20-2).
   Toàn bộ tree thuộc host-uid-thật; với map `0 ↔ uid-thật` của minict, file hiện
   `root:root` bên trong sandbox. **Đo được** (AL6): `/etc/init.d/acpid` =
   `0:0` trong ns ↔ `1001:1001` host. Tránh cả fakeroot lẫn newuidmap (cấm).
   Trade-off: không phân biệt được user nào cài file — chấp nhận cho GĐ1
   (single-user sandbox), multi-user trong ns = không với tới với single-uid map.
3. **apk offline, signature-verified (không `--allow-untrusted`)** (D20-3).
   Local repo pre-seed 1 gói không-deps (`acpid-openrc 2.0.34-r6`, lấy version
   từ APKINDEX v3.22 tại build-time); apk verify bằng alpine-keys preinstall.
   **Đo được** (AL5): `apk add --no-network` rc=0 trong sandbox. `--allow-untrusted`
   bị loại vì làm mù lớp kiểm tra.
4. **/proc trong pidns: env-aware test** (D20-4). Kernel cho phép mount procfs
   khi userns sở hữu pidns (pid_namespaces(7)) — runner (6.x) assert
   `/proc/self/status` `Pid: 1` thật; máy dev host-seccomp chặn mount(2) → assert
   fail-soft (payload vẫn chạy, exit sạch). Không hardcode một môi trường.
5. **init: busybox init có sẵn; payload-as-PID1 + shim D19 đủ cho one-shot/web**
   (D20-5). Init thật (reap vòng đời dài, multi-service) = việc của GĐ2
   exec/daemon — không mở scope hôm nay.
6. **apk add online trong instance → chờ egress pasta (mục 4)** (D20-6).
   Netns loopback-only (D17), không đường ra; offline repo chỉ là kiểm chứng
   hành vi, không phải thay thế egress.

## Gói nào hỏng dưới single-uid mapping (yêu cầu roadmap) — đo được

- **Chown sang uid khác uid-thật**: EPERM post-capdrop — đo AL7
  (`chown 100:100 /etc/passwd` fail). Bất kỳ gói có post-install/runtime
  `chown` sang user service riêng (nginx → `nginx:nginx`, postgres → `postgres`)
  sẽ hỏng ở bước đó, vì trong ns chỉ có uid 0 được map; uid khác = overflow
  `65534` không quyền.
- **Script chạy dịch vụ qua openrc/multi-user**: hỏng cùng lý do (cần uid riêng).
- **Phương án (ghi rõ, không tự ý mở scope)**:
  (a) chạy service dưới root-in-ns — đơn giản nhưng mất user isolation, chỉ dùng
  cho rootfs dev/web tĩnh (busybox httpd đã làm vậy, D6);
  (b) pre-bake user + ownership vào rootfs từ mkrootfs stage (host side) — được,
  vì file vẫn root:root trong ns và service đọc được; đề xuất cho nginx ở mục 5;
  (c) idmapped mounts (kernel ≥ 5.19, runner đạt) để map range uid không cần
  newuidmap — research-only cho GĐ2.
- Gói **không** cần chown-foreign (acpid-openrc, busybox applet, file tĩnh)
  cài sạch — đo AL5/AL6.

## Số đo (uid 1001, N=20 cold / N=5 RSS)

| Metric | busybox baseline | alpine 3.22.8 | Ngưỡng |
|---|---|---|---|
| cold_start_ms p50 | 1.88 | 1.83 | ≤ 100 |
| rss_kb (wrapper+PID1 idle) | 1066 | 1087 | ≤ 10240 |
| disk_base_mb | 2.0 | 8.6 | ≤ 20 |

disk tăng +6.6MB là chi phí cố tình chấp nhận của "distro thật" (apk DB, musl,
busybox full applet); vẫn ≤ 20MB. Evidence: `journal/2026-11-17/evidence/alpine-perf.txt`.

## Test
Suite mới `tests/test_alpine.sh` (AL1–AL8), vào `run-suites.sh` → `make test`
chạy đủ 4 suite. Không case cũ bị sửa → 50-test baseline giữ nguyên (53 khi
PR #21 gộp do +3 SIG test của nó).
