# test_web: chọn port trống thay vì random mù (issue #75, 2026-11-23)

## Ngữ cảnh
`tests/test_web.sh` chọn `PORT=$((18000 + RANDOM % 2000))`. Trên máy dev ZAI có
service infra listen 19005/19006 → 19006 bị chọn → WEB1/2/4/5 đỏ giả, WEB3 xanh
giả (expect-absent). `grep "127.0.0.1:$PORT"` còn khớp tiền tố (1900 khớp 19006).

## Quyết định
1. `port_listening <p>`: so khớp nguyên trường port của cột local address `ss -Htln`
   (mọi địa chỉ), không khớp tiền tố.
2. `free_port <base> <span>`: random tối đa 64 lần, bỏ port đang listen và port suite
   đã cấp (USED_PORTS) → PORT, PORT2, PORT3, p9, p9b không bao giờ trùng nhau hay
   trùng listener host. Hết lượt → suite fail rõ ràng, không đoán.
3. WEB2 so khớp chính xác `127.0.0.1:$PORT` và không có `0.0.0.0|*|[::]:$PORT`.
4. WEB3 kiểm marker `minict-web` trước; port không do httpd của mình phục vụ → FAIL
   setup thay vì PASS rỗng.

## Kiểm
Negative control cục bộ: mở listener 0.0.0.0:19000–19009, `free_port 19000 12`
chỉ cấp 19010/19011, lần thứ 3 báo hết port; `port_listening 1900` = false khi 19006
đang listen. Số case và ngữ nghĩa WEB1–WEB9 không đổi.
