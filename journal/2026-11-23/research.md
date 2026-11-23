# Research — 2026-11-23 (SDDA Phiên 20, issue #124 / GAPS S6)

Chủ đề: webport collision detection — state-based vs TCP-probe vs dynamic allocation;
docker port allocation precedent; HTTP 409 Conflict semantics.

## Nguồn 1 — docker published-ports (docs.docker.com)
- Link: https://docs.docker.com/config/containers/container-networking/#published-ports
- `docker run -p <host>:<container>` — nếu host port đã chiếm, docker fail sớm với
  `bind: address already in use` (exit 125, không silent). Precedent: detect collision
  up front, fail closed. minict S6 = same posture (409 thay vì exit 125 vì API model).
- Docker dynamic allocation: `-p :<container>` (không host port) → docker chọn port
  ngẫu nhiên free. minict KHÔNG làm (operator-declared, host==container contract).

## Nguồn 2 — net.Listen TCP probe (Go stdlib)
- Link: https://pkg.go.dev/net#Listen
- `net.Listen("tcp", "127.0.0.1:<port>")` bind test: nếu OK, close ngay → port free.
  TOCTOU: giữa close và webproxy bind, process khác chiếm. minict KHÔNG chọn (state-based
  an toàn hơn, không race với external process).

## Nguồn 3 — minict state machine (repo, A3/D47)
- Link: `minictd/internal/minict/store.go` alive(); decision D47 (`journal/2026-11-20/`).
- State machine creating/running/stopped, alive() heals dead-pid running→stopped. Ports
  field trong state.json. Stop/Remove giết ProxyPIDs (webproxies) → port released.
  portsHeldBy reuse alive() + Ports: chính xác cho minict instances.

## Nguồn 4 — HTTP 409 Conflict semantics (RFC 9110 §15.5.10)
- Link: https://www.rfc-editor.org/rfc/rfc9110#section-15.5.10
- 409 Conflict: "request conflicts with the current state of the target resource".
  ErrNotRunning → 409 (instance state conflict). ErrPortInUse cùng class (port held
  by another running instance = state conflict). Parity mapping.

**Trade-off:** state-based detection (scan state.json) thay vì TCP probe — không
TOCTOU với external process, không false-positive trên operator's nginx, cheap.
Trade-off: false positive trên stale state sau daemon crash (reconcile heal).

**Áp dụng cho minict:** `portsHeldBy(except)` state-based + `ErrPortInUse` → 409;
Run/Start check trước spawn; Stop/Remove release (existing). No dynamic allocation
(operator-declared, host==container contract D16/D24).
