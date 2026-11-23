# D55 — S6: webport collision detection up front (issue #124, Phiên 20, 2026-11-23)

## Ngữ cảnh

GAPS S6: khi hai instance khai báo cùng `minict.webport` label port, `spawnProxies`
của instance thứ hai bind `127.0.0.1:<port>` fail EADDRINUSE → webproxy exit, chỉ
ghi Note `preview port X not TCP-ready`. Instance vẫn "running" nhưng preview port
hỏng silently — operator không nhận lỗi rõ ràng, tưởng OK.

## Quyết định

Chọn **pre-flight collision detection trong daemon** (state-based), không dynamic
port allocation:

1. **`portsHeldBy(except)`** (`store.go`): scan tất cả instance qua
   `ListByNamePrefix(ctx, "")`, filter `alive(st).State == running`, collect
   `port -> name` cho mọi instance trừ `except`. Stopped/dead instances không
   hold port (Stop/Remove giết webproxies; alive() heals dead-pid running→stopped).
2. **Sentinel `ErrPortInUse`** → `server.go mapErr` HTTP 409 Conflict (parity
   `ErrNotRunning` — cùng semantics "state conflict").
3. **Run**: sau `webportsOf`, check mỗi declared port vs `portsHeldBy(spec.Name)` →
   collision return `fmt.Errorf("%w: port %d held by instance %s", ErrPortInUse, ...)`
   BEFORE spawnPayload/spawnProxies. Operator nhận 409 rõ ràng.
4. **Start**: re-check trước `spawnProxies` — instance khác có thể chiếm port khi
   này đang stopped. Same fail-closed posture.
5. **Không dynamic port allocation**: operator-declared ports (label), daemon không
   assign ports. Lý do: matching host==container port là contract hiện tại
   (D16/D24 unix-socket bridge); dynamic allocation would break Traefik routing
   (G3) và operator expectation. S6 scope = detect collision, không redesign allocation.

## Phủ nhận / residual

- **External process hold port**: nếu non-minict process chiếm `127.0.0.1:<port>`,
  `portsHeldBy` không biết (chỉ scan minict instances) → spawnProxies vẫn EADDRINUSE
  + Note. Out of S6 scope (ops/host issue, không daemon-allocatable). Operator phải
  chọn port free.
- **TOCTOU**: cửa sổ giữa `portsHeldBy` check và `spawnProxies` bind — cùng daemon,
  single-threaded per-instance (flock lock trên Run/Start), không concurrent Run
  cùng port. Nhưng hai daemon khác nhau trên cùng host sẽ race (out of scope —
  single-daemon design, manifest §2 "một người vận hành một máy").
- **Daemon restart**: running instances' webproxies chết khi daemon chết. State
  vẫn running. `portsHeldBy` báo port held (state running) nhưng thực tế port free.
  → false positive: Run mới bị 409 dù port free. Khắc phục: `--autostart`/reconcile
  heal stopped instances; running instances được wake-recreate (restart webproxies).
  Trước khi reconcile chạy, false positive có thể. Acceptable (fail-closed: safer
  refuse than silent collision). Residual: đo time window reconcile boot.

## Tests

- **MD38** (`tests/test_minictd.sh`): run A `minict.webport=WP_S6` → running; run B
  cùng port → 409; stop A → run B cùng port → running. Negative (mutant, SCR):
  skip `portsHeldBy` check → B "running" với broken preview (not 409) → MD38 đỏ.
- **MD0-MD37**: không đụng (S6 chỉ Go daemon + test, không C, không seccomp).

## Bài học đo được

- **State-based vs TCP-probe**: chọn state-based (scan state.json) thay vì TCP probe
  (bind test) vì (1) không TOCTOU với external process, (2) không false-positive trên
  port held bởi non-minict process (operator's nginx không blocks minict Run), (3)
  cheap (state dir scan ~µs). Trade-off: false positive trên stale state sau daemon
  crash (residual, reconcile heal).
- **409 Conflict parity**: `ErrPortInUse` map cùng `ErrNotRunning` → 409 — cùng
  class "state conflict" (resource held by another instance). Không 503/500.
