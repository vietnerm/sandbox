# D82 — pasta fail-closed defaults: loopback bind, `-u/-T/-U none`, `--no-map-gw` (issue #220)

Nhánh `sec/220-pasta-failclosed` · 2026-11-27 (Phiên 43) · type:security p1

## Bằng chứng đo (bước a của issue — TRƯỚC khi sửa)

MD70 diagnostic qua **đường daemon thật** (pastaArgs), runner CI ubuntu-24.04
(commit `896c769`, run 45935460401, artifact `ci-evidence/md70-measure.txt`):

```
P1_declared_bind       : *:18024            ← hole 2: forward khai báo mở mọi interface
P2_udp_auto_count_5353 : 1                  ← hole 1: UDP auto — ns bind UDP 5353 → host mở 0.0.0.0:5353
P3_ns_to_host_API_out  : HTTP/1.0 401 Unauthorized  ← hole 3: sandbox tới được API minictd qua gateway mapping
```

Thêm 2 bằng chứng tĩnh của chính binary pin `pasta 2026_10_02.cba3570`:
`--help` khai `-t/-u/-T/-U default: auto`; source pin (fwd_rule.c, tarball
sha256-pinned D37) xác nhận `fwd_rule_parse` áp MỘT addr cho toàn bộ comma-list
(`parse_addrifname` trước spec), `auto` là một entry, `none` phải đứng một mình
("conflicts with previous options"), `-T/-U` từ chối listen-address.

## Quyết định

1. **`pastaArgs` fail-closed mặc định** (podman-parity — containers/common
   libnetwork/pasta_linux.go truyền `-t/-u/-T/-U none` + `--no-map-gw` cùng kiểu):
   - Forward khai báo bind **127.0.0.1** (trước đây: mọi interface). Opt-in
     `public/` mới bind mọi interface — grammar v2 `[tcp|udp/][public/]<host>:<ns>`;
     unknown scope → lỗi, không bỏ qua im lặng.
   - TCP `auto` (D42: port bind trong ns được forward về host cho preview/wake)
     **giữ nguyên chức năng nhưng thu về loopback** (`127.0.0.1/...,auto` trong
     cùng flag — prefix áp cho cả entry auto).
   - UDP auto là lỗ hổng đo được → explicit spec tự tắt scan; không khai báo UDP
     → `-u none`.
   - `-T none -U none` (ns→host) + `--no-map-gw` (gateway→host) luôn truyền —
     sandbox không còn đường nào tới dịch vụ host-loopback kể cả API minictd.
2. **UDP live-probe qua volume bind**: busybox nc không có `-u` (rootfs không có
   udpsvd/tftpd; ntpd/dnsd bind port <1024 phía host — pasta rootless không giữ
   được) → compile static `udpbind` (20 dòng C) vào `$WORK`, bind-mount qua
   Volumes vào instance. Không đổi rootfs, không thêm dependency.
3. **Không đổi**: đường legacy `minict.sh` (test_pasta.sh NET11 dựa gateway
   mapping) — fix chỉ đụng `pastaArgs` của daemon. Egress outbound +
   `--dns-forward` không bị ảnh hưởng (podman truyền cả hai cùng lúc) — MD70
   assert `wget example.com` vẫn chạy.
4. **Từ chối**: giữ 0.0.0.0 mặc định + opt-out (mặc định an toàn phải là không
   mở); field RunSpec riêng cho public (grammar prefix đủ, ít thay đổi contract);
   `-t auto` bỏ hẳn (G3 preview/wake phụ thuộc nó — thu loopback thay vì bỏ).

## Hai phép thử (manifest §3)

- Còn nhẹ không? Có — chỉ đổi argv builder + parser, ~60 dòng; không process
  mới, không dep mới.
- Bỏ đi core còn hữu dụng không? Có — đây là thu hẹp bề mặt mặc định; forward
  tính năng vẫn đủ (MD47-49, wake/preview loopback, egress, DNS).

## Bằng chứng sau fix

- Unit: TestPastaArgsFailClosed + TestPastaArgsPublicOptIn + grammar v2
  (public/ 3 dạng, unknown scope, empty scope) — package PASS.
- MD70 assert (runner, env-gated tun): P1' `127.0.0.1:$P2PORT` + không wildcard
  + forward sống; P2' count=0; P3' BLOCKED; P4' wget example.com ≥1 match;
  P5' `public/` bind wildcard + curl ok (quyền năng chuyển sang tường minh).
- MD47-49 không đổi hành vi khai báo (curl 127.0.0.1 vẫn qua được).
