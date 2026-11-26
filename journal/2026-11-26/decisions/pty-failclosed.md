# D77 — #206: PTY fail-closed — host-side openpty + ready handshake + stderr capture + console fallback

- **Ngày:** 2026-11-26 (Phiên 41)
- **Issue:** #206 (owner báo, p1 type:bug) · **Nhánh:** `feat/206-pty-failclosed`
- **Tiền đề:** D73 (#187 PTY backend), #189 (console attach), MD62 env-gated.

## Root cause (đo được, không đoán)
1. **openpty() chạy SAU setns(mnt) ENOENT**: rootfs alpine minirootfs không có
   `/dev/ptmx` và sandbox không mount devpts (đo: `rootfs/dev` chỉ
   null/random/urandom/zero). minict-exec `--pty` die ngay → session bị reaper
   xoá → `/stream` "not found". Lỗi `setns(pid) EPERM` khi owner chạy tay là
   đúng thiết kế (host shell không thuộc userns tổ tiên) — red herring.
2. Lỗi bị nuốt: `cmd.Stderr = nil` → daemon log sạch vô giao.
3. Thành công giả: Start trả `session_id` ngay sau `cmd.Start()`.
4. Console chỉ fallback khi POST lỗi; POST 200 + stream chết → terminal trống.

## Fix (đúng thứ tự issue a–d)
- **a (root cause)**: minict-exec cấp PTY **host-side trước setns chain** —
  mô hình docker-exec-TTY (Nguồn 1): `/dev/ptmx` host 0666 rootless-safe; cả
  master+slave là open file description, hợp lệ qua mọi setns; TIOCSWINSZ qua
  fd không phụ thuộc mount ns. Seccomp/capdrop không đổi (install trong child
  sau fork — MD0 parity giữ nguyên).
- **b**: `ptyStderrBuffer` cap 4 KB (head 2 KB + tail 2 KB, marker
  `…[truncated]…`), `cmd.Stderr` bắt vào buffer, slog `terminal: pty start
  failed` + tail vào error body. Env: `cmd.Env = minict.ExecChildEnv()`
  (export wrapper childEnv — R2: helper daemon-spawned không kế thừa env
  daemon; trước đây terminal path quên đặt → inherited, khác execv2).
- **c**: `--status-fd 4` → minict-exec ghi byte `'R'` sau setns+chdir+NNP+
  openpty+fork (trước pump loop); daemon `waitPtyReady` chờ ≤ 2s: 'R' →
  register session + trả sid; EOF → kill+reap+5xx kèm stderr tail; deadline →
  kill+reap+5xx. **Không còn 200-giả.**
- **d**: console two-layer fallback — cờ `gotFirstFrame`; stream chết trước
  frame đầu → dọn sid, in notice vàng, chuyển line-mode ExecShell.
- **e**: MD66 (stub minict-exec exit-3 → POST /terminal non-2xx + marker
  stderr trong body + `stderr_tail` trong daemon log — chạy mọi nơi, không
  cần PTY); 5 Go unit (waitPtyReady OK/EOF/timeout + buffer cap/verbatim);
  console `tsc --noEmit` + `next build` sạch. **MD62 PASS trên dev 5.10 lần
  đầu** (trước đây SKIP env-gated — xác nhận root cause fix đúng).
- **f**: README PTY giữ `thử nghiệm` (điều kiện nâng `sẵn sàng`: MD62 xanh
  ổn định trên CI; lần này PASS dev — flip khi CI xác nhận).

## Hai phép thử (manifest §3)
- **Còn nhẹ không?** Có — không dep mới, không process nền mới (PTY session
  là tính năng GĐ3 có sẵn); C chỉ dời openpty lên trước setns + thêm 1 byte
  status; Go thêm handshake/stderr cap stdlib.
- **Bỏ đi core còn hữu dụng không?** Có — PTY là lớp trải nghiệm; core
  execv2/line-mode chạy không cần nó.

## Mutants (negative control — chạy với binary mutant thật)
- M1 bỏ ready handshake (trả sid ngay sau Start): MD66-core — POST /terminal
  trả **200 (giả)** = đúng bug owner → **CAUGHT** (binary mutant, 09:44 UTC) ✓
- M2 stderr về lại nil (+ gỡ tail): MD66-core — body **không còn**
  `stub-pty-fail-marker` → **CAUGHT** ✓
- M3 openpty về lại trong ns: MD62 quay về SKIP trên dev 5.10 (env-gate nuốt
  mutant trên kernel ≥ 6.8) — **không caught tự động**, bù bằng chứng thật:
  MD62 dev SKIP→PASS trước/sau fix (evidence). Ghi rõ trong SIA.

## Bằng chứng
- `evidence/pty-failclosed.txt`: make test-minictd 60 PASS + 8 SKIP + 0 FAIL
  (MD62 PASS, MD66 PASS); go unit server/minict/sandbox ok; console tsc+build
  sạch; perf + host baseline.
