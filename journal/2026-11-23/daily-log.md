# Daily Log — 2026-11-23

## Phiên 20 — Orchestrator (ZAI, §11)

### Pha A — Sync + SBG plan
- Pull main `cdb0e56` (ff, ledger Phiên 19 qua PR #123); remote chỉ `origin/main`.
- SBG MODE=plan → `branch/plan.md` (Phiên 20): 0 PR mở đầu chu trình → vào thẳng Pha D.

### Triage + chọn issue
- 17 issue mở đều đã label; không có `type:security priority:p0/p1` nào chen ngang.
- Tạo **#124** (S6 — GAPS thứ tự cuối cùng sau S5 + kế hoạch "ngày mai" Phiên 19;
  type:security p1, assign + in-progress, label `phase:2`). S1-S6 series hoàn thành sau phiên này.

### Research (`research.md` Phiên 20 — 4 nguồn)
- docker published-ports: fail sớm `bind: address already in use` (precedent detect up front).
- net.Listen TCP probe: TOCTOU, không chọn.
- minict state machine (A3/D47): alive() + Ports field → portsHeldBy state-based.
- RFC 9110 §15.5.10: 409 Conflict = state conflict, parity ErrNotRunning.

### Development (`sec/124-s6-webport-alloc`, D55 — `decisions/s6-webport-alloc.md`)
- `store.go`: `portsHeldBy(except)` — scan `ListByNamePrefix`, filter `alive().State == running`,
  collect `port -> name` (trừ except). Sentinel `ErrPortInUse`.
- `runtime.go Run`: sau `webportsOf`, check declared ports vs `portsHeldBy(spec.Name)` →
  collision `ErrPortInUse` (fmt `%w`) BEFORE spawn.
- `runtime.go Start`: re-check trước `spawnProxies` (port có thể bị chiếm khi stopped).
- `server.go mapErr`: `ErrPortInUse` → 409 Conflict (parity `ErrNotRunning`).
- Tests: MD38 trong `tests/test_minictd.sh` (37→38) — run A port WP_S6 → OK; run B cùng port
  → 409; stop A → run B → OK; cleanup list md-s6a/md-s6b + header MD38 + WP_S6 var.
- Không đụng C (minict/minict-exec unchanged) → MD0-MD37 parity giữ nguyên.
- Docs: GAPS.md S6 → FIXED; roadmap GĐ2 mục 1 "S1-S6 đều FIXED" + "Đã xong" Phiên 20; journal ngày.

### CVE Check
- Kernel: `5.10.134-013.15.kangaroo.al8.x86_64`
- `cve/s6-webport-alloc.{md,json}`: 5 watchlist rows (partial — NVD timeout cut); cve-gate rc=0
  → 0 mới ngoài watchlist.
- **Caveat**: cve-check.py NVD query transient rate-limited (timeout 45s, .md chỉ 5/7 rows);
  JSON synthesized từ .md; cve-gate rc=0. Caveat AUDIT §F fixed-list.
- Không component mới ship (Go stdlib errors/fmt/context đã có) → watchlist không cần dòng mới.

### Validation "không bị nhiễm"
- `make test`: 9/10 suites PASS (isolation 34, seccomp 21, netns 7, alpine 9, debian 8+1SKIP,
  escape-alpine 34, cve-check 4, pasta 5+6SKIP, stack 8); `test_cg.sh` FAIL = env (make minictd
  cần Go, không có trên dev — KHÔNG regression S6, S6 không đụng cgroups).
- `make test-web`: 9/9 PASS (WEB1-9).
- `make test-minictd`: **SKIP env** (không Go build minictd local) — CI ubuntu-24.04 chạy
  MD0-MD38; minict/minict-exec build C clean (-Wall -Wextra), selftest seccomp OK.
- Host baseline: passwd `21d5de2d…` / bash `223fe856…` / sudo `a59f393b…` — 3/3 khớp Phiên 14-19.
- Bằng chứng: `evidence/s6-webport-alloc.txt`.

### Pha B — PR (sec/* theo thứ tự §11)
- B1: nhánh con trực tiếp của main `cdb0e56` → merge origin/main up-to-date.
- B2 — SCR: ... (sau CI).
- B3 — SIA: ... (sau CI).
- B4 — verdict: ... (sau SIA).

### Kế hoạch ngày mai
- Sau merge #124: GĐ2 mục 1 S-series (S1-S6) hoàn thành; còn systemd-real unit + residual #109
  (chờ maintainer). GĐ2 tiếp: mục 2 (template), mục 3 (storage overlayfs + snapshot), mục 6
  (Landlock), mục 7 (đo control-plane RSS); from-audit #117/#119 nếu dư phiên.
- **GD3 (Web UI) CHƯA chạm** — per chỉ thị owner: DỪNG + nhắc owner TRƯỚC dòng code UI đầu
  tiên khi mọi mục GĐ2 hoàn thành. GĐ2 mục 1 gần xong (S-series done, còn systemd-real + #109);
  mục 2-7 chưa mở.

### Pha C/E — cập nhật sau merge (SBG apply + ledger sẽ ghi ở phần cuối phiên)
