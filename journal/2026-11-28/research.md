# Research — Terminal PTY cho exec trong userns rootless (GĐ3 mục 2, issue #181)

## Ngày: 2026-11-28 | Phiên: 35 (Việc 3)

## Bài toán
GĐ3 mục 2: terminal line-mode xong (mỗi dòng = `execv2` `/bin/sh -c`). Còn thiếu endpoint PTY cho chương trình tương tác (vi, top, REPL). Yêu cầu: rootless (uid ≥ 1000), daemon idle ≤ 30 MB RSS (MD54), Go stdlib only (không dependency ngoài).

## Nguồn 1 — PTY allocation: openpty + posix_openpt (kernel docs)
- URL: https://github.com/torvalds/linux/blob/master/Documentation/admin-guide/devices.rst + https://github.com/torvalds/linux/blob/master/Documentation/filesystems/devpts.rst
- Tóm tắt:
  - `openpty()` (libc) = `posix_openpt()` + `grantpt()` + `unlockpt()` + `ptsname()` → trả về master fd + slave fd.
  - Master fd (`/dev/ptmx`) nằm trong process gọi openpty. Slave fd (`/dev/pts/N`) nằm trong devpts mount.
  - devpts mount per-namespace: mỗi mount có pts indices độc lập. Trong userns, có thể mount devpts riêng (`mount -t devpts devpts /dev/pts`).
  - `kernel.pty.max = 4096` (global limit) — đủ cho single-user.
- Áp dụng minict: **openpty phía minict-exec SAU setns** (vào instance ns). Master fd trả về daemon qua pipe/stdout. Slave fd dup2 vào stdin/stdout/stderr của payload process. Payload thấy `/dev/pts/N` trong instance devpts.
  - **Không cần mount devpts riêng**: instance đã có devpts (minict.c mount `/dev/pts` trong prepare flow nếu có). Nếu không có → openpty fallback dùng host devpts (slave path `/dev/pts/N` nhìn thấy từ instance — leak nhẹ nhưng acceptable vì single-user).
  - **Rootless**: openpty không cần root — posix_openpt mở `/dev/ptmx` mode 0666. grantpt() trong glibc hiện đại là no-op (kernel tự set ownership).
  - **Threat**: master fd trên daemon — nếu payload viết escape sequence Control-Z → master interpret → không (master là raw byte pipe, không interpret). An toàn.

## Nguồn 2 — Go stdlib transport: net/http Hijack (Go stdlib)
- URL: https://github.com/golang/go/blob/master/src/net/http/server.go (Hijack method)
- Tóm tắt:
  - `http.ResponseWriter.Hijack()` → `(net.Conn, *bufio.ReadWriter, error)`. Lấy raw TCP connection, thoát khỏi HTTP handler. Cho phép đọc/ghi raw bytes two-way.
  - Go stdlib KHÔNG có WebSocket server (chỉ có `golang.org/x/net/websocket` — external). `nhooyr.io/websocket` hoặc `gorilla/websocket` — external dependency (roadmap cấm).
  - **HTTP/1.1 upgrade**: client gửi `Upgrade: websocket` (hoặc custom) → server Hijack → raw TCP. Nhưng client (browser) chỉ hỗptr WebSocket cho upgrade — không hỗtr custom raw upgrade.
  - **Server-Sent Events (SSE)**: `text/event-stream` — one-way (server → client). Go stdlib hỗtr (net/http + Flusher). nhưng stdin (client → server) cần kênh riêng.
- Áp dụng minict: **2 lựa chọn**:
  - **(A) HTTP/1.1 hijack + custom framing**: client (xterm addon `attach`) mở WebSocket tới daemon. Daemon KHÔNG có WebSocket stdlib → cần implement handshake + framing tay (~150 dòng, RFC 6455). Đáng tin cậy nhưng phức tạp.
  - **(B) 2 kênh**: SSE cho stdout/stderr (server → browser, `text/event-stream` + Flusher) + POST cho stdin (browser → server, chunked). Đơn giản hơn, Go stdlib thuần. **CHỌN (B)** — ít code, stdlib only, daemon RSS không tăng đáng kể.
  - **Trade-off**: (B) có latency stdin (~50ms per keystroke do HTTP POST overhead) — acceptable cho line-mode apps (vi/top), không cho real-time games. Console giữ line-mode fallback nếu (B) không đủ.

## Nguồn 3 — TIOCSWINSZ (terminal resize ioctl)
- URL: https://man7.org/linux/man-pages/man2/ioctl_tty.2.html (TIOCSWINSZ)
- Tóm tắt:
  - `ioctl(master_fd, TIOCSWINSZ, &winsize)` — set window size cho PTY. `winsize {rows, cols, xpixel, ypixel}`.
  - Kernel gửi `SIGWINCH` tới foreground process group của PTY slave → app (vi, top) nhận và redraw.
  - Go: `syscall.Syscall(syscall.SYS_IOCTL, masterFd, syscall.TIOCSWINSZ, uintptr(unsafe.Pointer(&ws)))`.
- Áp dụng minict: endpoint `POST /v1/sandboxes/{name}/resize` với body `{"rows":N,"cols":M}` → daemon `ioctl(master_fd, TIOCSWINSZ, &ws)`. minict-exec giữ master fd mở (persistent process). Console gửi resize khi xterm `onResize` fire.

## Nguồn 4 — Cleanup: process reaping trên disconnect (containerd precedent)
- URL: https://github.com/containerd/containerd/blob/main/pkg/process (reap pattern)
- Tóm tắt:
  - Khi client disconnect (HTTP connection close / SSE stream end), daemon phải:
    1. Detect disconnect (SSE writer error / context cancel).
    2. Gửi SIGHUP tới PTY master → payload nhận SIGHUP (default action: terminate) → cleanup.
    3. Nếu payload không thoát trong 5s → SIGKILL.
    4. Close master fd → kernel reclaim PTY pair.
    5. Reap zombie (`waitpid`).
  - Containerd pattern: goroutine `exec.Process.Wait()` + `Kill` trên context cancel.
- Áp dụng minict: minict-exec giữ session (persistent PTY). Daemon giám sát SSE writer error → cancel context → minict-exec nhận signal → kill payload → exit. `PastaPID`/`StartTicks` pattern (D67) tái dùng cho PTY session PID guard.

## Nguồn 5 — RSS budget: Go stdlib only, no WebSocket dep
- URL: https://pkg.go.dev/log/slog + https://pkg.go.dev/net/http (Go stdlib)
- Tóm tắt:
  - Go stdlib `net/http` + `bufio` + `syscall` — đã có trong daemon (D68 observability, D47 reconcile).
  - SSE = `text/event-stream` + `http.Flusher` — 0 dependency, ~50 dòng code.
  - POST stdin = chunked request body reader — 0 dependency, ~20 dòng.
  - PTY open = `syscall.Open("/dev/ptmx")` + ioctl — 0 dependency, ~40 dòng (trong minict-exec C hoặc Go).
  - **Ước lượng RSS**: SSE goroutine per session (~8 KB stack) + bufio buffer (4 KB) + PTY buffer (4 KB) = ~16 KB per active session. Daemon idle (0 session) = không tăng. 1 session = +16 KB — way under 30 MB.
- Áp dụng minict: **chọn (B) SSE + POST** — stdlib only, RSS delta < 1 MB per session, idle delta 0. Daemon idle vẫn ≤ 30 MB (MD54 giữ).

## Kết luận + kế hoạch code (sau research)
1. **Endpoint**: `POST /v1/sandboxes/{name}/terminal` (start PTY session, trả session id) + `GET /v1/sandboxes/{name}/terminal/{sid}/stream` (SSE stdout/stderr) + `POST /v1/sandboxes/{name}/terminal/{sid}/input` (stdin) + `POST /v1/sandboxes/{name}/terminal/{sid}/resize` (TIOCSWINSZ).
2. **minict-exec**: thêm mode `--pty` — openpty sau setns, exec payload với slave fd dup2. Master fd truyền về daemon qua pipe. Daemon giữ map `sid → master fd + process`.
3. **Cleanup**: SSE writer error → cancel context → SIGHUP master → SIGKILL sau 5s → close fd → reap.
4. **Console**: xterm `attach` thay `write()` line-mode. `onResize` → POST resize. Fallback line-mode nếu `/terminal` trả 404 (daemon cũ).
5. **Budget**: stdlib only, idle delta 0, per-session ~16 KB. MD54 (≤ 30 MB) giữ.
6. **Test**: MD62 (PTY boot + echo round-trip) + MD63 (resize → SIGWINCH → `stty size` thay đổi) + MD64 (disconnect → SIGHUP → cleanup). Mutant: bỏ SIGHUP → MD64 FAIL (payload zombie).

## Không làm (cần owner duyệt riêng)
- WebSocket (stdlib không có — cần external dep hoặc implement tay RFC 6455 ~150 dòng). Nếu owner muốn WS real-time → mở issue riêng.
- `make console` CI step (cần owner duyệt .github/ change).
