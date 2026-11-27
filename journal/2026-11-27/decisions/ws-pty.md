# D84 — WebSocket PTY hai chiều, binary frames (#218, phiên 45, 2026-11-27)

## Bối cảnh
Terminal GĐ3 (#187/#210, D73/D77) dùng SSE cho output + **mỗi chunk bàn phím
một POST /terminal/{sid}/input** (hàng đợi tuần tự #217). IME/tiếng Việt/emoji/
paste dài vẫn phụ thuộc nhiều HTTP round-trip + nhiều lần string→JSON→string.
Issue #218 chốt: một WebSocket hai chiều, frame binary, RFC 6455 tự viết
stdlib-only (go.mod zero-dep — manifest §3 "mặc định tối thiểu").

## Quyết định
1. **RFC 6455 minimal (`server/ws.go`, ~300 dòng)** — chỉ text/binary/close/
   ping/pong; reject extension (RSV bits → 1002), client frames MUST mask
   (RFC §5.3 — cache-poisoning defence) → không mask = 1002; payload ≤ 64 KB
   → 1009; handshake Sec-WebSocket-Accept = sha1(key+GUID) base64; không
   fragment (browsers không fragment ≤ 64 KB — continuation = 1002, ghi rõ
   limitation). Server frames không mask, FIN luôn set.
2. **Endpoint** `GET /v1/sandboxes/{name}/terminal/ws`:
   - **Origin gate 403 TRƯỚC upgrade** (CSWSH — RFC §1.6 origin model):
     Origin host phải == Host của listener; `--ws-origin host:port` mở thêm
     cho deployment qua proxy (mặc định rỗng = strict). Client không phải
     browser (không Origin) vẫn qua — token gate phía sau.
   - **Auth frame**: text ĐẦU TIÊN `{"type":"auth","token":…}` trong 5 s,
     constant-time compare; sai/late/sai-kinds → close 4401 **trước khi spawn
     PTY** (auth hỏng = 0 session, 0 env-file). Không nhận token qua query
     string (lộ log/history). Browsers không set được Authorization header
     trên WS handshake → middleware auth bỏ qua đúng path này (isWSHandshakePath)
     — frame auth là gate duy nhất và không thể skip bằng cách skip header check.
   - **Protocol**: binary = stdin/stdout PTY thô (không JSON — UTF-8 an toàn
     qua chunk boundary); text = resize/close (client) + ready/exit/error
     (server, `exit.code` từ reaper — terminalSession.done mới).
   - **Liveness**: server ping 30 s; > 60 s không có inbound frame nào → close
     (browser tự auto-pong ở tầng protocol — MDN).
   - **Backpressure**: hàng đợi output 64 frames; client ngừng đọc > 30 s →
     drop session (SIGHUP + close) — RSS daemon không thể tăng vô hạn.
   - **Đóng**: {"type":"close"} hoặc ngắt → Stop() (SIGHUP → reap); reaper
     giờ **xoá env-file** (pattern cleanup execv2; trước #218 PTY path leak
     một env-file nhỏ mỗi session — heal luôn cả SSE path vì cùng reaper).
3. **Fallback giữ nguyên** (#187/#210): console thử WS trước (open ≤ 2 s +
   auth + ready; fail trước ready → fallback SSE+POST với thông báo hiển thị,
   rồi line-mode). **Không cần sequence number**: mỗi POST /input đợi response
   rồi mới gửi tiếp (hàng đợi tuần tự #217) → thứ tự giữ được ở fallback;
   WS giữ thứ tự tự nhiên của TCP.
4. **statusRecorder.Hijack()** — middleware requestLog chặn http.Hijacker
   (found while testing: mọi handshake 500 trước fix).
5. **Dead code removal** — check cap pre-decode trong readFrame là dead
   (7-bit ≤ 127 < 64KB) — mutant M-ws3 phát hiện (suite vẫn xanh khi bỏ nó)
   → xoá, giữ duy nhất check post-decode phủ mọi length encoding (mutant
   M-ws3b bỏ nó → test đỏ).

## Hai phép thử manifest §3
- 0 dependency: `ws.go`/`terminal_ws.go` stdlib only (sha1/base64/hijacker);
  go.mod vẫn zero-dep.
- Tắt khi không có session: mọi goroutine (pump/reader/ping) sinh theo
  connection và kết thúc với connection — không goroutine nền khi rảnh.

## Bằng chứng
- **13 Go unit** (`terminal_ws_test.go`, mini RFC 6455 client tự viết): accept
  vector RFC §1.3, plain GET 400, Origin lạ 403 / cùng host 101 / allowlist,
  wrong-token 4401, missing-frame 4401, binary-first 4401, unmasked 1002,
  oversize 1009, auth-timeout 4401, echo binary byte-exact (tiếng Việt+emoji),
  close → reap + env-file removed.
- **MD72 e2e** (daemon thật): evil-origin 403 · wrong-token 4401 · auth+ready ·
  unicode echo byte-exact (tiếng Việt 😀 é) · paste 10 039 bytes unicode
  (head -c + wc -c khớp) · cat output byte-exact (ONLCR-aware) · RSS/session
  steady 0 KB (< 1024) · 2 session song song không lẫn output · close → reap
  → env-file 2 → 0.
- Mutant **4/4 CAUGHT**: M-ws1 (bỏ mask check), M-ws2 (bỏ Origin), M-ws3b
  (bỏ cap payload), M-ws4 (auth nhận mọi token) + M-ws3 dead-code finding.
- `make test-minictd` 66 PASS + 9 SKIP + 0 FAIL; console tsc/build sạch.
