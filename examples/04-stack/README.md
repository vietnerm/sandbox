# 04 — Stack 2 service (Track M M2, `sandbox stack`)

Minicstack demo: `db` boot trước (ready `exec` exit 0), rồi `web` phụ thuộc
`db`, có `pasta_ports` forward host 8088 → 8080 trong netns (cần pasta +
`/dev/net/tun`), ready `tcp` đo qua forward host.

## Chạy

```sh
minictd &            # daemon (xem minictd/README.md)
sandbox stack up   --file examples/04-stack/stack.json
sandbox stack ps   --file examples/04-stack/stack.json   # JSON status
curl -s http://127.0.0.1:8088/   # qua pasta forward
sandbox stack down --file examples/04-stack/stack.json
```

Lưu ý:
- `image: base` = rootfs alpine của repo (`sandbox/mkrootfs.sh`); đổi sang
  `template:<name>` hoặc `oci:<ref>` nếu cần.
- ready `http`/`tcp` chỉ chạm được service khi có `pasta_ports` (netns
  loopback-only, D17) hoặc service nghe trên host qua webport label.
- `stack down` bỏ qua instance không tồn tại; lỗi thật → exit ≠ 0.
