# D78 — #207: MD55/MD57 flake root cause — trùng dải port + healthz loop nuốt 404

- **Ngày:** 2026-11-26 (Phiên 41) · **Issue:** #207 (p3 chore) · **Nhánh:** `fix/207-port-flake`

## Root cause (từ log attempt 1 run 45788203542, không đoán)
1. **Trùng dải port**: `API_PORT = 17600+RANDOM%300` → 17600-17899 ∩ `UI_PORT =
   17800+RANDOM%200` → 17800-17999 (đè 17800-17899); `OVL_PORT = 17900+%90` →
   17900-17989 ∩ UI (đè 17900-17989). Khi trùng, UI daemon fail bind
   (EADDRINUSE) → exit ngay sau spawn.
2. **Wait-loop nuốt mọi HTTP response**: `curl -s ... healthz && break` — curl
   exit 0 cho MỌI HTTP code (404 từ daemon khác đang chiếm port) → loop
   "thành công" sai → MD55/MD57 curl vào mux không phải UI daemon → `GET /
   got 404` / `POST / got 404` (đúng signature run đỏ; rerun cùng SHA PASS vì
   random port không trùng).

## Fix
- Dải port tách rời hoàn toàn: API 17600-17799 (%200), UI 17800-17999 (giữ),
  OVL 18300-18399 (đổi từ 17900+90), PFPORT dời lên khối đầu; bảng đầy đủ
  (REG/S4/API/UI/S6/OVL/PFPORT/WPORT2/WEB) trong PORT0.
- Tất cả wait-loop healthz dùng `curl -sf` (chỉ 2xx mới break) — main daemon
  ×2, UI daemon, stub daemon MD66; UI daemon thêm assert sau loop: healthz
  200 + `kill -0 DPID6` (fail loud thay vì âm thầm).
- **PORT0** (case mới, chạy đầu suite): so 9 dải cố định pairwise — dải mới
  thêm vào suite mà đụng dải cũ → FAIL ngay. Mutant checker (API MAXS
  17799→17999) → PORT0 FAIL = caught ✓.
- daemon6.log (stub MD66) tách thành `daemon-stub.log` (không đè log UI
  daemon — dọn residual SIA PR #210 low #3).

## Sai lầm trong quá trình (bằng chứng quá trình)
- PORT0 v1 so khoảng theo giá trị random ĐÃ RÚT (`UI_PORT+199`) — tự bắt
  chính mình: `UI(17919-18118)×S6` overlap giả. Sửa thành so DÃỐ CỐ ĐỊNH
  (BASE..BASE+width-1). Bài học: khoảng random phải so theo base, không theo
  sample.
- OVL_PORT/PFPORT định nghĩa muộn → `unbound variable` khi PORT0 chạy sớm —
  dời về khối port đầu file (PORT0 cần mọi biến sớm).

## Hai phép thử (manifest §3)
- Còn nhẹ: có — chỉ test harness; 0 dòng production code.
- Bỏ đi core còn hữu dụng: có — suite chạy không cần PORT0 (nhưng mất canary).

## Bằng chứng
- `evidence/p207-portflake.txt`: make test-minictd 61 PASS + 8 SKIP + 0 FAIL
  (PORT0 PASS, MD55/MD57 PASS); mutant checker caught; host unchanged.
