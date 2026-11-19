# D34 — Pasta/slirp4netns connectivity (GĐ1 mục 3): research + thiết kế + env-blocked máy dev

- **Ngày**: 2026-11-19 (Phiên 2 — chu trình tăng tốc, xem D35)
- **Trạng thái**: RESEARCH-ONLY phiên này — KHÔNG mở PR code. Prototype hẹn
  phiên kế theo thiết kế bên dưới (issue #57).

## Sự thật đo được hôm nay
1. **`/dev/net/tun` KHÔNG tồn tại trên máy dev** (`ls -l /dev/net/tun` → ENOENT,
   container al8, kernel 5.10.134). pasta lẫn slirp4netns đều cần tạo tap
   (pasta mở /dev/net/tun từ mount-ns host rồi join netns đích) → **cả hai
   env-blocked trên máy dev**. Không thể dev-test pasta ở đây.
2. CI runner (ubuntu-24.04, kernel azure 7.0) — GitHub runners có
   `/dev/net/tun` (kiểm lại bằng probe `ip tuntap` trong workflow trước khi
   implement — ghi vào gate).
3. Seccomp side: payload dùng AF_INET cho egress — **SC19 đã allow**
   (socket(AF_INET) httpd case); tap fd sống trong process pasta (host-side,
   ngoài filter) — payload không cần syscall mới ngoài AF_INET/AF_UNIX.
4. NET1 assertion "only lo inside" sẽ SAI khi pasta bật (tap + route xuất
   hiện) → suite phải env-aware theo MINICT_PASTA (pattern AL5/AL4).

## Thiết kế đề xuất (ghi lại để phiên kế implement thẳng)
- **Acquisition**: tải binary pasta pinned từ passt.dev (URL + sha256 pin
  trong `tools/` hoặc script riêng — pattern mkrootfs-alpine D20). Không
  vendor binary vào repo (disk budget 20 MB).
- **Flow**: `minict.sh up --pasta` (hoặc MINICT_PASTA=1): launcher sinh
  instance như thường → HOST-side spawn
  `pasta --netns /proc/<shim-pid>/ns/net --config-net` (pasta join
  userns+netns của instance — đúng model rootless của pasta; rootless host
  uid 1001 đủ vì caps lấy từ userns đích). Gateway/DNS config-net copy từ
  host; port-forward host↔instance qua `--tcp-port-forward` (điều kiện
  trước cho GĐ2 mục 4 port-forward theo instance).
- **Suite**: `MINICT_PASTA=1` bật class case mới (NET10: tap hiện diện +
  route default; NET11: egress busybox wget ra ngoài qua pasta; NET12:
  host→instance port-forward). Không có tun → SKIP(env-blocked) ghi rõ
  (không PASS giả).
- **A/B phải có**: cold start có/không pasta (pasta spawn thêm process
  host-side — đo riêng, không cộng vào cold_start của minict).

## Vì sao không PR code hôm nay
- Mọi case pasta trên máy dev sẽ SKIP → test chỉ có giá trị trên runner CI →
  lỗi chỉ phát hiện được sau merge (vi phạm tinh thần "dev-verifiable").
- Phiên đã có 2 merge + 1 gate-miss cần khắc phục — giảm risk main đỏ.
- Kế hoạch phiên kế: PR `feat/3-pasta-egress` gồm probe tun + binary pin +
  3 case NET10-12 + A/B perf; chạy CI trước khi merge, SCR lấy log CI cho
  phần runner-only.

## Nguồn (đối chiếu 2026-11-19)
- passt/pasta docs (passt.dev — design "pasta: connect namespaces to the
  internet, rootless"); slirp4netns README (tap-fd model, so sánh pasta).
- man7 netns/userns caps: setns vào netns cần CAP_SYS_ADMIN trong userns
  sở hữu netns đó — pasta join userns đích trước (rootless pattern,
  RootlessKit --port-driver cùng họ — precedent D16).
