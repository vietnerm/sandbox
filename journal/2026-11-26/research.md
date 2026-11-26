# Research — 2026-11-26 (Phiên 33, issue #168 observability)

## Nguồn 1 — Prometheus text exposition format (official docs)
- URL: https://github.com/prometheus/docs/blob/main/docs/instrumenting/exposition_formats.md
- Tóm tắt (5 dòng):
  - Content-Type `text/plain; version=0.0.4; charset=utf-8`, UTF-8, `\n` line endings, last line phải kết thúc `\n`.
  - Comment `#` được bỏ qua trừ `# HELP <name> <docstring>` và `# TYPE <name> counter|gauge|...`. TYPE phải xuất hiện trước sample đầu tiên của metric đó.
  - Sample: `metric_name{label="value",...} <float> [timestamp_ms]`. Label value escape: `\` → `\\`, `"` → `\"`, LF → `\n`.
  - Mọi line của cùng metric phải grouped (HELP/TYPE trước). Sorting reproducible preferred nhưng không required — ta sẽ sort để scrape ổn định (test MD42 yêu cầu counter không giảm).
  - Metric name regex `[a-zA-Z_:][a-zA-Z0-9_:]*`, label name `[a-zA-Z_][a-zA-Z0-9_]*`.
- Áp dụng minict: viết tay renderer ~100 dòng (KHÔNG prometheus/client_golang), emit `# HELP`/`# TYPE` cho 8 metric instance + 3 metric daemon. Sort theo (name, label set) để deterministic.

## Nguồn 2 — cgroups v2 interface files (kernel docs)
- URL: https://github.com/torvalds/linux/blob/master/Documentation/admin-guide/cgroup-v2.rst
- Tóm tắt:
  - `cpu.stat` flat-keyed: `usage_usec <n>`, `user_usec`, `system_usec` (+5 CFS fields khi controller enabled). usage_usec là counter microseconds (monotonic, không giảm trừ reset cgroup).
  - `memory.current` single int bytes; `memory.max` int bytes hoặc `"max"`.
  - `io.stat` nested-keyed: mỗi dòng `$MAJ:$MIN rbytes=<n> wbytes=<n> rios=… wios=… dbytes=… dios=…`. Cần sum rbytes/wbytes qua tất cả device để ra total.
  - `pids.current` single int; `pids.max` int hoặc `"max"`.
- Áp dụng minict: tái dùng `cgReadInt` (cgroup.go:406) + `cgCPUStatUsage` (cgroup.go:419) đã có từ #4/D48. Thêm parser `io.stat` mới (sum rbytes/wbytes). KHÔNG viết đường đọc thứ hai — runtime Stats đã đọc memory.current/pids.current/cpu.stat.

## Nguồn 3 — gVisor metric package (precedent)
- URL: https://github.com/google/gvisor/blob/master/pkg/metric/metric.go
- Tóm tắt: gVisor dùng protobuf + registered metric framework (heavy, export qua /metrics JSON hoặc Prometheus). Quá nặng cho minict (roadmap cấm prometheus/client_golang, chỉ Go stdlib).
- Áp dụng minict: học cách tách renderer (gVisor có `metric.Export` cho nhiều format) — ta tách `metrics.go` renderer thuần function `renderPrometheus(snapshot) []byte`, test trực tiếp không cần HTTP. Đảm bảo deterministic output.

## Nguồn 4 — Go `log/slog` stdlib (JSONHandler)
- URL: https://pkg.go.dev/log/slog (Go 1.21+ stdlib, minictd đã dùng TextHandler)
- Tóm tắt:
  - `slog.NewJSONHandler(w, &slog.HandlerOptions{Level, ReplaceAttr})` — mỗi record 1 dòng JSON, field tự động: `time`, `level`, `msg`, + attrs theo thứ tự add.
  - `ReplaceAttr` cho phép rename/normalize field (vd. chuẩn hóa `time` RFC3339, `level` lowercase).
  - Grouping: `slog.With("instance", name, "op", "create")` trả logger con mang context attrs — mỗi log call kế thừa, không lặp.
- Áp dụng minict: đổi `slog.NewTextHandler` → chọn theo `--log-format` flag. Logger daemon mang `request_id` qua `slog.With` từ middleware HTTP. Event logger dùng `.With("instance", name, "op", X)` rồi `Info("create", "result", "ok", "duration_ms", ms)`. JSON output tự có đủ trường roadmap yêu cầu.

## Trade-off / risk
- **Slog field order**: JSONHandler giữ thứ tự attrs add → ổn định nhưng khác TextHandler (KV). Test MD41 parse JSON stderr, không phụ thuộc format text cũ.
- **Metrics scrape-time cost**: đọc 6 file cgroup × N instance mỗi scrape. N nhỏ (single-user), không goroutine nền → OK. Fail-soft: instance không có cgroup → chỉ `up{state}` không bịa 0.
- **Token leak vào log**: phải cẩn thận — request_id OK (UUID/giá trị client gửi, không phải token); KHÔNG log header Authorization. Test MD41 grep token đảm bảo.
- **--metrics-listen fail-closed**: nếu sau này có cờ, phải reject `0.0.0.0`. Phiên này KHÔNG thêm cờ (endpoint /metrics trên listener sẵn có) → không có risk mới.
