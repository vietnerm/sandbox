# Decision D68 — Observability: log JSON + Prometheus metrics + /metrics endpoint

- **Phiên**: 33 (2026-11-26)
- **Issue**: #168 (GĐ2 mục 7)
- **Slug**: `observability`

## Vấn đề
GĐ2 mục 7 (manifest DoD) yêu cầu observability:
1. Log JSON có cấu trúc (thay TextHandler ad-hoc hiện tại) với schema cố định (time/level/msg/instance/op/result/duration_ms/error/request_id), KHÔNG bao giờ leak token/env.
2. Metrics instance đọc cgroup v2 (cpu.stat/memory/io.stat/pids) xuất dạng Prometheus text exposition — viết tay, KHÔNG prometheus/client_golang (ràng buộc manifest: chỉ Go stdlib).
3. Endpoint `GET /metrics` trên listener API sẵn có, yêu cầu cùng token như `/v1/*`, không listener mới, không goroutine nền.

## Giải pháp chọn
- **Log**: `--log-format text|json` flag (mặc định `json`). Dùng `slog.NewJSONHandler` (Go 1.21+ stdlib). Logger daemon mang `request_id` qua `slog.With` từ HTTP middleware (UUID đọc từ header `X-Request-Id` hoặc sinh). Event logger trong runtime dùng `.With("instance", name, "op", X)` rồi gọi `.Info("create", "result", "ok", "duration_ms", ms)` — JSON output tự có đủ trường. `ReplaceAttr` chuẩn hóa `level` lowercase + `time` RFC3339Nano. KHÔNG log header Authorization/env-file content (chỉ log request_id, op, instance, result, duration, error message).
- **Metrics**: file mới `minictd/internal/minict/metrics.go` — function `renderPrometheus(snap MetricsSnapshot) []byte` thuần (test trực tiếp không cần HTTP). Snapshot thu thập scrape-time: iterate instances qua store, đọc cgroup files (tái dùng `cgReadInt`, `cgCPUStatUsage`, thêm `cgIOStat` parser sum rbytes/wbytes). Render: `# HELP`/`# TYPE` cho mỗi metric + sample lines sort theo (name, labels) → deterministic. 8 metric instance (cpu_usage_seconds_total counter, memory_bytes gauge, memory_limit_bytes gauge, io_read_bytes_total counter, io_write_bytes_total counter, pids gauge, pids_limit gauge, up{state} gauge) + 3 daemon (instances{state} gauge, process_resident_memory_bytes gauge đọc `/proc/self/status` VmRSS, build_info gauge với version/commit).
- **Endpoint**: `GET /metrics` trên cùng ServeMux với `/v1/*`, cùng token middleware. Content-Type `text/plain; version=0.0.4; charset=utf-8`. KHÔNG `--metrics-listen` flag (roadmap: nếu sau này có, fail-closed exit ≠ 0 cho non-loopback).
- **Fail-soft**: instance không có cgroup (Stats đã trả ok=false) → chỉ xuất `minict_instance_up{instance,state} 1` (hoặc 0 nếu stopped), không bịa 0 cho memory/cpu/io/pids.

## Lý do chọn (trade-off)
- **Viết tay renderer thay vì prometheus/client_golang**: manifest cấm, + giảm RSS (client_golang ~2MB). Renderer ~100 dòng, deterministic, test trực tiếp.
- **scrape-time read thay vì goroutine nền**: single-user, N instance nhỏ; không leak goroutine; không giữ lịch sử RAM (roadmap mục 7c). Trade-off: scrape có thể miss counter giữa 2 lần — chấp nhận cho observability, không phải accounting.
- **slog JSONHandler thay vì custom JSON encoder**: stdlib, đủ field, ReplaceAttr cho chuẩn hóa. Logger con `.With()` mang context → không lặp code.
- **Token auth /metrics**: tái dụng middleware hiện có (server.go). Không listener mới → không attack surface mới. Test MD43 verify 401 không token + reject non-loopback (qua test cờ --metrics-listen nếu thêm — phiên này không thêm nên chỉ test 401).

## Phạm vi phiên này (DONE)
- 7a (log JSON) + 7b (metrics renderer) + 7c (/metrics endpoint + auth) + 7d (tests MD41/MD42/MD43 + Go unit) + 7e (perf baseline RSS).
- Env-gated MD42 (cgroups v2) như CG0–CG5 — dev 5.10 SKIP, runner CI kernel 7.x PASS.

## Phạm vi KHÔNG làm (residual)
- Biểu đồ CPU/RAM (GĐ3 mục 3, roadmap dòng 536).
- OTLP exporter / collector ngoài (hướng cluster nhiều VPS, "Hướng xa").
- `--metrics-listen` flag (chỉ thêm khi cần, fail-closed non-loopback).
- Histogram/summary metrics (chỉ counter/gauge đủ cho GĐ2 DoD).
