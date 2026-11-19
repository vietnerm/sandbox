# Decisions — execstream mất frame cuối (nhánh `fix/50-execstream-drain`, issue #50)

## D40 — muxStreams xả hết channel sau khi cả hai pump báo xong

**Root cause:** mỗi pump trong `handleExecStream` đẩy chunk cuối vào channel `frames`
(buffer 256), gặp EOF, rồi gửi `done`. Vòng lặp chính `select` trên `frames` và
`done`; khi cả hai sẵn sàng, Go chọn **ngẫu nhiên**. Nếu hai lần `done` thắng trước
khi chunk được đọc, vòng lặp thoát ở `completed == 2` và chunk còn trong channel bị bỏ.
Đúng triệu chứng MD13: stdout frame `b''` với lệnh rất ngắn (`echo out1`). Lỗi gặp
3 lần ngày 19/11: main `a339401`, PR #63 `1a39b6`, PR #63 `e98a379`.

**Sửa (`minictd/internal/server/server.go`):** tách phần mux thành
`muxStreams(ctx, stdout, stderr, emit)`. Sau khi cả hai pump báo xong (không pump nào
gửi nữa), xả hết `frames` bằng `select … default` rồi mới trả về. Giữ nguyên R6 (copy
chunk, mọi send đều nhìn ctx) và hành vi khi client ngắt (reap `wait()` trong
goroutine, không gửi exit frame).

**Test:** `internal/server/stream_test.go`. `TestMuxStreamsKeepsLastChunk` chạy 5000
vòng với stream ngắn EOF ngay; bản cũ (mutant bỏ đoạn xả) FAIL ngay lần chạy đầu, bản
sửa PASS. `TestMuxStreamsCtxCancel` kiểm ctx huỷ thì trả false. `make test-minictd`
giờ chạy `go test ./...` trước e2e, nên CI chạy unit test này (không cần sửa
workflow).
