# D42 — DNS cho instance qua pasta: `--dns-forward` + inject `resolv.conf`

Ngày: 2026-11-19 (Phiên 7). Issue: #74 (GĐ1 mục 1 residual — `apk add` online
chờ DNS). Bổ sung D37 (pasta egress + port-forward).

## Bối cảnh

NET11 chứng minh egress theo IP qua pasta chạy thật trên runner, nhưng mọi
workload resolve-by-name vẫn hỏng: rootfs busybox/alpine không có
`/etc/resolv.conf` trỏ tới resolver với tới được từ trong netns, và pasta
KHÔNG ghi file trong namespace (chỉ advertise qua DHCP — guest của minict
không chạy DHCP client).

Đo được trên source pin (passt 2026_10_02.cba3570, sha256 khớp D37):

1. `fwd.c fwd_nat_from_tap()`: flow DNS (UDP/TCP port 53/853) tới `dns_match`
   → rewrite sang `dns_host`, **ưu tiên hơn** `nat_outbound()` (map-gw) →
   bật DNS không phá NET11 (egress tới GW:port khác vẫn map về 127.0.0.1).
2. `--dns-forward` là cách duy nhất đặt `dns_match` (mặc định trống) →
   không truyền thì query GW:53 rơi vào map-gw → 127.0.0.1:53 → chỉ đúng
   khi host resolver nghe 127.0.0.1:53. Host của phiên này dùng resolver
   public (100.100.2.136/138) — sẽ hỏng → không dựa vào map-gw cho DNS.
3. `dns_host` mặc định = nameserver đầu tiên của host `/etc/resolv.conf`
   (pasta đọc host-side lúc start; nối từ host netns nên loopback resolver
   như 127.0.0.53 vẫn với tới được).

## Quyết định

`spawn_pasta()` trong `sandbox/minict.sh`:

1. GW = `ip -4 route show default` host-side (cùng nguồn `GW_IP` mà
   `tests/test_pasta.sh` đã dùng — pasta `--config-net` copy host gw làm
   guest_gw; NET10 đã assert default route có mặt trong guest).
2. GW rỗng → **skip** cả hai (không truyền `--dns-forward`, không ghi
   resolv.conf) + cảnh báo stderr — hành vi không đổi so với trước DNS
   (fail-soft cho môi trường không default route).
3. GW có → thêm `--dns-forward "$GW"` vào argv pasta; sau khi route-ready +
   aliveness PASS (các check hiện có), ghi `nameserver $GW` vào
   `$INST/$name/rootfs/etc/resolv.conf` (ghi đè deterministic — trong lúc
   instance sống, pasta là đường resolver duy nhất; down đã có cleanup,
   rootfs instance là bản private copy nên không rò sang instance khác).
4. Không đụng `minict.c`/seccomp (D25): DNS là UDP/TCP 53 tới một IP private —
   đã nằm trong surface AF_INET (SC19) của netns; blocklist không đổi.

## Phương án bị loại

- **Nhờ map-gw cho DNS** (nameserver GW không truyền --dns-forward): chỉ đúng
  khi host resolver nghe 127.0.0.1:53; host thật dùng resolver public →
  non-deterministic theo môi trường.
- **DHCP trong guest**: guest minict là busybox static — thêm DHCP client
  phình to, sai hướng "nhẹ".
- **Resolver built-in trong rootfs** (dnsmasq/tinydns): thêm binary vào disk
  budget 20 MB, không cần thiết — pasta đã forward sẵn.
- **Ghi resolv.conf vào template rootfs dùng chung**: rootfs template được
  copy cho instance KHÔNG dùng pasta — phải để nguyên; inject per-instance
  sau `up --pasta` mới đúng phạm vi.

## Test

`tests/test_pasta.sh` thêm 2 case env-gated (SKIP khi không tun, tiền lệ
NET10–12):

- **PT3C** (control): không pasta — payload `nslookup github.com` trong netns
  chỉ-lo KHÔNG ra được marker DNS (rootfs không có resolver dùng được) →
  chứng minh marker của PT3 đến từ pasta, không phải giải pháp khác.
- **PT3**: với `--pasta` — (a) rootfs `/etc/resolv.conf` của instance trỏ đúng
  `nameserver $GW_IP` (assert host-side, file vẫn còn trước `down`);
  (b) payload trong instance (`up --` — ns set gốc, có tap) chạy
  `busybox nslookup -timeout=3 github.com`, poll marker `DNS_RESOLVED_OK`
  host-side (anti-race pattern NET10/NET11).

Domain `github.com`: hạ tầng chính runner CI chạy trên đó — resolvable và
không rate-limit kiểu 8.8.8.8; không dùng /etc/hosts (tránh vacuous class
CHROOT1/NET11C).

## Residual

- `apk add` online end-to-end (DNS + HTTPS tới dl-cdn) không nằm trong PT3 —
  cần rootfs alpine + APKINDEX; làm ở phiên kế khi mục 1 cần (PT3 chứng minh
  lớp DNS; HTTPS là việc của apk-tools).
- IPv6 (dns-forward v6) chưa làm — pasta snapshot chạy v4 cho egress NET11;
  mở rộng khi GĐ1 cần v6.
