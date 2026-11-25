# D67 — port-forward qua pasta khai báo trong config instance (issue #165, GĐ2 mục 4)

Ngày: 2026-11-25. Nhánh: `feat/165-pasta-portforward-config`.

## Quyết định

Port-forward **theo instance** qua pasta được khai báo trong **RunSpec.PastaPorts**
(`pasta_ports`, omitempty — pattern D59 SeccompProfile); daemon attach pasta
out-of-band sau khi payload sống, tái dùng đúng cơ chế D37 của `minict.sh`:
`pasta --foreground --userns /proc/<pid>/ns/user --netns /proc/<pid>/ns/net
--config-net [--dns-forward GW] [-t h:n] [-u h:n]`. Shim C **không đổi**.

## Lý do chọn giải pháp

1. **Config-side = RunSpec, không phải env/flag**: mục tiêu của mục 4 là
   "khai báo trong config instance" (roadmap) — RunSpec là config duy nhất
   daemon hiểu, và nó ride theo spec.json nên `save-template` tự carry
   forwards sang `template:<name>` (D56 pattern, D59 SeccompProfile precedent).
   Không thêm flag `--pasta` như CLI vì daemon là long-lived: trạng thái
   forward thuộc về instance, không thuộc về daemon process.
2. **Grammar `[tcp|udp/]<host>:<ns>`, số đơn, cap 16** (passt.1 `-t
   first[-last][:toaddr]`): v1 không hỗ trợ range/address-prefix — đủ cho
   use-case preview/web service; validate fail-closed host-side (host port
   1024..65535 vì pasta rootless không bind được <1024; không trùng
   proto+host) → 4xx rõ ràng thay vì chết trong pasta argv. Range/address
   = residual, mở rộng sau nếu cần.
3. **Attach out-of-band, shim không biết**: giống webproxy.py — pasta chạy
   host-side làm con của daemon, join userns/netns của instance qua
   `/proc/<pid>/ns/*` (euid == owner → rootless, D37 đã chứng minh). Không
   directive MINICT_SPEC mới → **không đụng C, không đụng MD0 parity**
   (pasta không nằm trong filter payload), ít surface bảo mật nhất.
4. **Fail-closed toàn đường** (D34/D37 posture): `pasta_ports` khai báo mà
   thiếu binary (`--pasta-bin` → beside-binary → PATH) / thiếu
   `/dev/net/tun` / netns biến mất → Run 4xx/5xx + R4 wipe; Start → kill
   payload, instance STOPPED (`KilledBy=pasta-fail`). Không silent-skip —
   instance "có port" mà thực chất loopback-only là lie.
5. **Resolver D42 nướng TRƯỚC spawn** (khác minict.sh ghi SAU attach):
   legacy ghi `rootfs/etc/resolv.conf`, overlay ghi `upper/etc` — **không
   bao giờ `lower/`** vì lower zero-copy là nguồn SHARED (MD44 canh base
   không đổi). Ghi trước loại trừ race mount của D66; mọi failure của pasta
   fail cả Run/Start → wipe dir → không tồn tại resolver chết.
6. **Lifecycle kill ở MỌI đường kết thúc payload** — Stop, Remove, và
   alive() heal (payload chết ngoài query): pasta giữ **listening socket
   host-side** cho forwards; để sót pasta → Start sau crash bind-fail
   EADDRINUSE. `PastaPID` + `PastaTicks` (start_ticks) chống pid-reuse,
   cùng scheme PID/StartTicks.
7. **S6 carry-over**: cross-instance collision check cho host bind của
   forwards (`pastaPortsHeldBy`) detect up front → 409 rõ ràng. Third-party
   bind collision để pasta tự fail (spawn fail-closed + wipe) — probe trước
   bằng bind test không mô phỏng đúng semantics bind của pasta
   (mặc định bind mọi interface, passt.1).
8. **Giữ mặc định auto-forward của pasta** (passt.1: pasta mặc định
   `-t auto -u auto`): không truyền `-t none` — hành vi D37/NET12 hiện có
   không đổi; forwards khai báo chỉ ADD deterministic. Đặt `--dns-forward`
   = GW mặc định host (D42), skip fail-soft khi không có default route.

## Test

- Go unit `pasta_test.go`: parse valid/invalid (17 nhóm sai), argv pin
  chính xác (D37/D42 parity — reorder --userns sẽ được bắt), GW parse hex
  LE từ /proc/net/route fixture, resolver dir overlay/legacy, spawn
  fail-closed khi thiếu binary, note grammar.
- E2E MD47–49 (env-gated `/dev/net/tun`, D34; runner thật):
  - MD47: Run với `pasta_ports ["FPORT:8080"]` (FPORT≠8080 — auto-forward
    không thể giải thích) → host curl hit httpd trong netns; resolver có
    `nameserver`; spec.json carry; pasta-pid + log tồn tại.
  - MD48: stop → pasta-pid mất + host port bindable lại; start → attach
    mới + forward hoạt động.
  - MD49: kill -9 payload ngoài query → Inspect heal → pasta bị reap +
    port freed; start lại → forward trả lại; Run với `"80:80"` /
    `"tcp/abc:def"` → 4xx + dir wiped (R4).

## Residual (ghi nhận, không chặn)

- Range/address-prefix grammar, address-specific bind (passt.1 hỗ trợ
  `-t addr/port`) — chưa có use-case.
- `--pasta-bin` chỉ PATH/beside-binary không pin version — operator chọn;
  test suite luôn dùng `tools/bin/pasta` (pinned D37).
- CLI `sandbox run` chưa expose `pasta_ports` (CLI run hiện bare-bones
  Name+Image — MD20); API là entry-point chính.
- pasta default auto-forward giữ nguyên hành vi D37 (không `-t none`).
