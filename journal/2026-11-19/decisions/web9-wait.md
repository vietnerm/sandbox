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

## Phụ lục D41 (sau vòng đo thêm trên host ZAI)

- Kết quả post-fix trên host ZAI (cùng host, các mốc liên tiếp): **25/26 suite
  run PASS** (8/8 + 4/5 non-instrumented; 13/13 instrumented) so với pre-fix
  ~6/17 FAIL (~35%). Cải thiện 35%→4% fail-rate.
- Loại trừ nguyên nhân (đo được): (a) `/proc/self/fd` magic-link connect
  stress 200/200 OK; (b) upstream socket trả marker **0.00 s** mọi lần probe
  trực tiếp; (c) proxy instrumented: **0 refusal** qua 26 run (log stderr).
- Residual (1/26): burst `rc=52` empty-reply của proxy asyncio trên host
  container này, dài tới >5 s trong context suite — **không tái hiện trên
  runner CI** (web 9/9 xanh mọi run trong ngày, kernel 7.0). Chua ra root
  cause tầng scheduler/container; budget 5 s đã bao phủ mọi burst ≤1.4 s đo
  được standalone. Theo dõi: nếu xuất hiện trên CI → mở issue sâu hơn.
- Mutant port-sai vẫn caught (check "never listened" sống).
