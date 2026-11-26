# D80 — #209: uptime tick realtime + heal note/finished_at cho stale state

- **Ngày:** 2026-11-26 (Phiên 41) · **Issue:** #209 (p2 bug) · **Nhánh:** `feat/209-uptime-stale-state`

## Quyết định
- **a (console uptime)**: `Sandbox.startedAt` (ISO created_at) trong store;
  `startUptimeTick()` — MỘT interval 1s dùng chung recompute `uptime` cho
  running sandboxes (guard: không tick khi không có running → idle 0
  re-render); app-shell mount; overview/list/detail tự tick (đều đọc
  `sandbox.uptime`). pollLive 5s giữ nguyên cho CPU/RAM.
- **b (daemon stale state)**: `alive()` heal bổ sung `FinishedAt` (RFC3339 UTC)
  + note `"process lost (payload pid dead — daemon/session restart, OOM hoặc
  reboot) @ <ts>"` (append Notes) + slog sự kiện `reconcile op=heal` + log
  lỗi persist fail (trước đây `_ =`). Inspect giữ "exited" (docker parity —
  console map "stopped" cho user).
- **c (README)**: minictd/README lifecycle: sống qua daemon restart thường
  (Setsid); KHÔNG sống qua kill session/cgroup hay reboot; `--autostart` cho
  reboot; console detail hiện note + gợi ý autostart.
- **d (test)**: Go unit `TestHealRecordsNoteAndFinishedAt` (PID 0 → heal
  deterministic; assert note + finished_at + persist) + **MD68** e2e
  (kill -9 shim → /state: stopped + note + finished_at; Inspect exited;
  daemon log có `"op":"heal"`); console tsc + next build sạch.

## Hai phép thử (manifest §3)
- Còn nhẹ: có — 2 field JSON additive + 1 interval console; không process nền.
- Bỏ đi core còn hữu dụng: có — xoá note/tick không ảnh hưởng lifecycle.

## Mutants
- M1 (heal không ghi note): `TestHealRecordsNoteAndFinishedAt` FAIL ✓ (assert
  note + finished_at + persist).
- M2 (MD68 không kill shim → state vẫn running): case FAIL (assert stopped) ✓
  — non-vacuous (during-check phụ thuộc kill thật).
