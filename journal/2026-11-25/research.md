# Research — 2026-11-25 (overlayfs rootless, issue #162)

## Vấn đề
`prepareRootfs` copy nguyên cây rootfs (~2 MB busybox/alpine) cho mỗi instance
(`cp -a`). Với N instance từ cùng image: N × 2 MB disk + N lần copy I/O ở
cold start. GĐ2 mục 3 residual yêu cầu overlayfs rootless thay private copy.

## Nguồn 1 — kernelnewbies Linux 5.11 (overlayfs unprivileged mounts)
https://kernelnewbies.org/Linux_5.11
- 5.11 cho phép non-privileged users mount overlayfs sau khi audit code đảm
  bảo mọi thao tác trên các layer chạy với quyền của mounting task.
- Option `-o userxattr` buộc overlayfs dùng xattr namespace `user.overlay.`
  thay vì `trusted.overlay.` — "useful for unprivileged mounting of overlayfs"
  (unprivileged không có quyền ghi `trusted.*`).
- Đo trên dev (kernel 5.10.134): `unshare -Ur` + mount overlay → **EPERM** —
  xác nhận dev env cần gate; CI ubuntu-24.04 (kernel 6.x) đạt.

## Nguồn 2 — kernel.org overlayfs docs (mount syntax + ràng buộc layer)
https://docs.kernel.org/filesystems/overlayfs.html
- Syntax: `mount -t overlay overlay -o lowerdir=L,upperdir=U,workdir=W /m`.
- **workdir phải cùng filesystem với upperdir** (hard requirement — daemon
  tạo cả hai trong cùng instance dir nên tự thỏa).
- `userxattr` chọn namespace `user.overlay.*`; lower-only mount (không upper)
  = read-only overlay hợp lệ.
- Podman rootless dùng cùng pattern (overlay driver + userxattr trên 5.11+);
  idmapping của overlayfs chỉ có từ 5.19 → idmapped mounts để residual
  (đúng kế hoạch "phần a").

## Trade-off đã cân nhắc
- **Mount ở shim (trong userns) vs ở daemon (ngoài userns)**: daemon không có
  CAP_SYS_ADMIN trên fs host → bắt buộc mount trong userns của shim (sau
  unshare, có cap trong ns riêng). Khớp kiến trúc hiện có (bind/tmpfs cũng
  mount ở shim).
- **Fallback âm thầm khi mount overlay lỗi → KHÔNG**: fallback ẩn phá vỡ cam
  kết "không private copy" và biến gate thành lie. Fail-closed: die() với
  message chỉ rõ kernel < 5.11 hoặc fs không hỗ trợ.
- **copy-up xattr trên tmpfs/overlay-of-overlay**: lower là dir thường trên
  fs host (ext4/overlay runner) — không phải overlay-on-overlay.
- **userxattr vs không**: unprivileged không ghi được `trusted.*` khi copy-up
  cần lưu origin xattr → phải truyền `userxattr` tường minh.

## Áp dụng cho minict
1. Go daemon: env `MINICT_OVERLAY=1` → `base`/`dir:`/`template:` lower chỉ
   THẲNG vào nguồn (zero copy), `tar:`/`oci:`/`backup:` unpack vào
   `<dir>/lower`; tạo `<dir>/rootfs` (mountpoint rỗng) + `upper` + `work`
   (0700); renderSpec phát `overlay lower=… upper=… work=…` (validate
   fail-closed: path tuyệt đối, cap 512B, lower không là ancestor của
   upper/work, upper ≠ work).
2. C shim: directive `overlay` (parse fail-closed như các directive khác);
   chuyển `load_spec_file()` lên trước check `/bin/sh` (check `lower/bin/sh`
   khi overlay); sau unshare+MS_PRIVATE: mount overlay tại rootfs thay
   bind-onto-self; readonly+overlay → lower-only (không upper). pivot_root
   không đổi (overlay mount là mount point hợp lệ; old-<pid> ghi vào upper).
3. Snapshot trên instance overlay: từ chối rõ ràng (host-side rootfs là
   mountpoint rỗng — tar sẽ ra archive rỗng); merge whiteout-aware residual.

## Phiên 32 — pasta port-forward config-side (issue #165, D67)

- **passt.1 (passt.top/builds/latest/web/passt.1.html)**: `-t/-u spec`
  grammar `none | [addr[%if]/]ports` — entry `first[-last][:toaddr]`;
  pasta **bind forwards host-side trên mọi interface mặc định** (addr
  prefix = tùy chọn); mặc định pasta là `-t auto -u auto` (forward các port
  đang bind trong ns về cùng host port, poll /proc/net/tcp* mỗi giây);
  `--config-net` copy addr/route từ host vào tap; `--dns-forward ADDR` map
  DNS flows (UDP/TCP :53/:853) từ ns về resolver host; `--map-gw` mặc định
  dịch gw → host loopback.
  → Áp dụng: v1 grammar `[tcp|udp/]<host>:<ns>` đủ dùng; FPORT≠8080 trong
  test để tách biệt với auto-forward; --dns-forward = GW (D42).
- **Podman pasta networking (docs.podman.io podman-pod.unit.5 +
  docs.oracle.com/en/learn/ol-podman-pasta-networking)**: Podman truyền
  `pasta:--map-gw` / `pasta:-t,auto,-u,auto,-T,auto,-U,auto` hoặc explicit
  `-t 8080-8080:80-80 --dns-forward 169.254.0.1 -u none -T none -U none
  --no-map-gw`; `ss -tulpn` thấy pasta LISTEN 0.0.0.0:8080 host-side →
  mỗi container một process pasta riêng, host port là listening socket
  thật → lifecycle phải kill pasta khi instance chết (khác webproxy chỉ
  relay). Trade-off Podman: hằng số option set; minict chọn declare-only
  (auto giữ mặc định).
- **Đo dev (đã có từ D34/D37)**: `/dev/net/tun` không có trên dev container
  (5.10.134) → MD47–49 env-gated SKIP trên dev, runner thật ở CI.
