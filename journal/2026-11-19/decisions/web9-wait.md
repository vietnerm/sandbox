# Decisions — WEB9 positive-control race fix (nhánh `fix/66-web9-wait-listen`, issue #66)

## D41 — chờ bind + chờ listen trước khi curl trong `web9_test`

- Vấn đề đo được (issue #66, phát hiện SCR Phiên 5): WEB9 FAIL flaky 3/9 run trên
  nhánh `4afae4` và 1/4 run trên `main` `b5c7e9f` — phần (a) positive control
  báo "positive control failed". Nửa (b) hardened-proxy đã có bước chờ `ss -tln`
  trước khi assert, nửa (a) thì chỉ `sleep 0.5` → bất đối xứng.
- Root cause: 3 tiến trình python (2 socket server + webproxy) khởi động async;
  khi runner load, import python > 2s → proxy chưa listen / upstream socket chưa
  bind lúc curl chạy → 15×(curl+0.1s) không đủ → FAIL.
- Sửa: chờ `-S` trên **cả hai** upstream socket (legit + attacker, tối đa 5s)
  trước khi start proxy; sau đó chờ `ss -tln` thấy `127.0.0.1:$p9` listen
  (tối đa 5s) trước vòng curl. Vẫn giữ vòng curl retry như lớp bảo vệ thứ ba.
- Negative control: mutant trên bản sao test — proxy gắn sai port → WEB9 FAIL
  ngay tại check "positive-control proxy never listened" (rc=1, 8/1 FAIL).
- Validation: `tests/test_web.sh` ×8 run sau fix = 8/8 PASS (9/9 case mỗi run),
  trên cùng host/mốc thời gian có 3/9 FAIL trước fix.
- Không đổi proxy/production code (`tools/webproxy.py` untouched) — thuần test
  harness; 9 case web giữ nguyên ngữ nghĩa (WEB7/WEB8 không đổi).
