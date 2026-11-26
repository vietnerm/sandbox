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

<!-- Gộp từ journal/2026-11-26/research.md (Phiên 34) -->

# Research — 2026-11-26 (Phiên 34, issue #171 mục 8)

## Nguồn 1 — Linux /proc/<pid>/status VmRSS (process RSS measurement)
- URL: https://github.com/torvalds/linux/blob/master/Documentation/filesystems/proc.rst (section /proc/<pid>/status)
- Tóm tắt: `VmRSS` là resident set size (pages in memory) tính bằng kB. Đo chính xác hơn `top`/`ps` vì đọc trực tiếp từ kernel. minictd là Go process — VmRSS bao gồm Go runtime heap + stacks + mmap'd binary. Idle (không instance) = baseline.
- Áp dụng minict: test_minictd.sh đã có daemon chạy (DPID). Đọc `awk '/VmRSS/{print $2}' /proc/$DPID/status` → KB. Hoặc scrape `/metrics` `minictd_process_resident_memory_bytes` (D68 đã expose, cùng nguồn /proc/self/status). Cả hai phải khớp (verify trong MD54).

## Nguồn 2 — /proc/self/mountinfo (mount leak detection)
- URL: https://github.com/torvalds/linux/blob/master/Documentation/filesystems/proc.rst (section /proc/<pid>/mountinfo)
- Tóm tắt: mountinfo liệt kê tất cả mount thuộc process ns. Mỗi dòng: `mount ID parentID major:minor root mountpoint ...`. Grep mountpoint path → tìm stale mounts. Sau Remove, không nên còn mount nào reference instance dir (bind mounts từ webproxy, overlay mount nếu D66).
- Áp dụng minict: `grep "$STATE/<name>" /proc/self/mountinfo` → nếu rỗng sau Remove = no mount leak. Lưu ý: overlay mount nằm trong SHIM's mount ns, không trong daemon's mountinfo. Nhưng bind mounts (webproxy socket) nằm trong daemon ns → detect được.

## Nguồn 3 — netns reclaim semantics (kernel)
- URL: https://github.com/torvalds/linux/blob/master/Documentation/networking/netns.rst
- Tóm tắt: netns được giữ sống bởi reference count (process + bind mounts + /proc/<pid>/ns/net symlinks). Khi process cuối exit và không còn bind mount/symlink → netns được reclaim tự động. minict shim exit → netns reclaim (không cần explicit cleanup). Leak chỉ xảy ra nếu process hoặc bind mount pin netns.
- Áp dụng minict: không cần test netns trực tiếp — nếu process leak = 0 (pgrep) và mount leak = 0 (mountinfo grep) → netns tự reclaim. MD53 verify process + mount = đủ bao phủ netns.

## Nguồn 4 — container destroy leak testing precedent (Docker/containerd)
- URL: https://github.com/containerd/containerd/blob/main/pkg/process (reap pattern) + Docker `docker rm` leak tests
- Tóm tắt: containerd reaper goroutine `exec.Process.Wait()` + `Kill` trên SIGCHLD. Leak detection trong Docker integration tests: `pgrep -f <container-id>` + `mount | grep <id>` + `ip netns list | grep <id>`. Pattern: snapshot baseline → create → destroy → verify delta = 0.
- Áp dụng minict: cùng pattern — pgrep `-f "$STATE/<name>"` (shim cmdline carries rootfs path) + mountinfo grep. Không cần `ip netns list` (minict không dùng named netns — anonymous via CLONE_NEWNET).

## Đo baseline dev 5.10
- Daemon idle RSS: 6372 KB (~6.2 MB) — cách xa ngưỡng 30 MB. /metrics `minictd_process_resident_memory_bytes` = 6524928 bytes = 6372 KB (khớp /proc VmRSS). VmRSS bao gồm Go runtime (~2 MB heap + stacks + mmap binary). Observatory mục 7 (slog JSON + metrics.go ~250 dòng) tăng < 1 MB so với trước (TextHandler).
- Threshold: 30 MB = 30720 KB = 31457280 bytes. Gate có biên độ dư ~24 MB → regression guard, không phải tight constraint.

## Trade-off / risk
- **RSS variance**: Go GC có thể làm RSS tăng tạm thời (heap before sweep). Đo idle sau `sleep 2` (daemon settled) + scrape 3 lần lấy max → ổn định. Nếu runtime.GC trước đo → thấp hơn nhưng không phản ánh steady-state. Chọn đo steady-state (sau sleep, không force GC).
- **Mount ns scope**: daemon mountinfo chỉ thấy bind mounts trong daemon ns. Overlay mount (D66) nằm trong shim ns → không detect qua daemon mountinfo. Nhưng overlay mount được unmount khi shim exit (pivot_root + exit). Process leak check (pgrep shim) bao phủ trường hợp này.
- **netns leak trên dev 5.10**: không có userns → netns tạo được nhưng /proc/<pid>/ns/net symlink tồn tại. Nếu shim exit đúng → symlink gone → netns reclaim. Test gián tiếp qua process check.

<!-- Gộp từ journal/2026-11-26/research.md (Phiên 35) -->

# Research — Terminal PTY cho exec trong userns rootless (GĐ3 mục 2, issue #181)

## Ngày: 2026-11-26 | Phiên: 35 (Việc 3)

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

---

# Phiên 40 — Research (issue #203, Track M M2 residuals d–i)

## Nguồn 1 — Docker Compose spec: depends_on + healthcheck + down semantics
- URL: https://github.com/compose-spec/compose-spec/blob/master/spec.md (05-services.md healthcheck section)
- Tóm tắt:
  - Compose `depends_on` + `healthcheck`: service chỉ coi "healthy" khi healthcheck
    pass liên tiếp; `condition: service_healthy` chờ dependency healthy trước khi start
    service kế. Healthcheck test = lệnh trong container, exit 0 = healthy.
  - `compose down`: dừng + xoá theo thứ tự ngược; service không tồn tại không phải lỗi.
  - Failure khi `up` (healthcheck fail sau `start_period`): các container đã start
    bị dừng/xoá (rollback) — hành vi chuẩn của docker compose up --wait.
- Áp dụng minict: `stack up` đã làm topoSort + rollback (D75); residual e hiện thực
  ready `exec` theo đúng mô hình healthcheck (exit 0 = ready, poll tới deadline);
  residual g cho `stack down` bám semantics compose: thiếu = bỏ qua, lỗi thật = báo.

## Nguồn 2 — Go net/http Client.Timeout (residual f: http.Get không timeout)
- URL: https://pkg.go.dev/net/http#Client.Timeout + https://go.dev/src/net/http/client.go
- Tóm tắt:
  - `http.Get` dùng `DefaultClient` — **không có Timeout** (documented zero-value
    hazard): request có thể treo vô hạn → `stack up` vượt `timeout_s` của ready.
  - `http.Client{Timeout: d}` là per-request deadline bọc toàn bộ: connect + gửi
    request + đọc headers + đọc body. Đúng thứ tự: 1 attempt chậm bị cắt ở 2s,
    vòng poll ngoài vẫn giữ deadline tổng `timeout_s`.
  - Pattern chuẩn: client cấp per-call (không dùng lại global mutable), per-attempt
    timeout < tổng deadline.
- Áp dụng minict: `waitForReady` case "http" dùng `&http.Client{Timeout: 2 * time.Second}`
  tạo trong hàm; bỏ `http.Get`. TCP dial đã có `DialTimeout` — giữ nguyên.

## Nguồn 3 — passt/pasta port-forward grammar (mirror client-side, residual d)
- URL: passt.1 man page (đã trích D67) + minictd/internal/minict/pasta.go parsePastaPorts
- Tóm tắt:
  - Daemon đã validate fail-closed (D67): `[tcp|udp/]<host>:<ns>`, host 1024..65535
    (rootless bind floor), ns 1..65535, không range/space, ≤ 16 entries, không dup
    host bind theo proto.
  - Stack (client) nếu KHÔNG validate trước: lỗi config chỉ lộ sau khi daemon Run
    fail → rollback đã chạy oan (instance trước đó bị dừng vì 1 dòng config sai).
  - Client-side mirror cùng luật (không import internal package — layer client
    không được đụng daemon internals) → parse lỗi TRƯỚC khi chạy instance nào.
- Áp dụng minict: `validatePastaPortsStack()` trong stack.go (sai → parseStack lỗi),
  daemon vẫn là nguồn sự thật (fail-closed lần hai ở Run).

## Kết luận
1. d: `ServiceDef.PastaPorts []string` → `spec.PastaPorts`; client pre-validate
   grammar D67; ready tcp/http chỉ đo được qua pasta forward (loopback-only netns
   D17 khi không có pasta → ready tcp/http KHÔNG có pasta chỉ hợp khi test giả lập
   ở unit; e2e thật dùng pasta env-gated MD47-49 gate).
2. e: `ReadyDef.Cmd []string`; case "exec" → `rt.ExecV2(ctx, name, {Cmd, TimeoutS:5})`
   ExitCode 0 = ready; poll tới deadline giống http/tcp (service có thể chưa boot);
   validate type ∈ {http,tcp,exec,none} + port + cmd LUÔN PARSE TIME.
3. f: `http.Client{Timeout: 2s}` per attempt; port 1..65535 parse time.
4. g: `stackDown` phân loại: `errors.Is(err, contract.ErrNotFound)` → bỏ qua;
   lỗi khác → gom, in, exit ≠ 0; "removed" chỉ khi Remove thật sự OK; sau stop lỗi
   (không phải NotFound) → KHÔNG remove (instance vẫn chạy, remove sẽ phá sai).
5. h: MD64 (env-gated pasta): stack 2 service db(exec ready)+web(pasta fwd, tcp ready)
   → up/ps/curl/down sạch; MD65 (không pasta): ready exec fail → rollback; down stack
   rỗng → exit 0 không in "removed"; down 1 có 1 thiếu → chỉ cái có bị removed.
6. Refactor nhỏ: stackUp/Down/Ps/waitForReady nhận `contract.Runtime` (interface) —
   unit test được bằng stub runtime, không đổi wire.

---

# Phiên 41 — Research (issue #206, PTY fail-closed + root cause)

## Nguồn 1 — docker exec TTY model: PTY cấp ở HOST, fd truyền vào container
- URL: moby/moby daemon/exec, `libcontainer` setsid + host /dev/ptmx; man 2 setns
  ("namespace fds are open file descriptions")
- Tóm tắt:
  - `docker exec -t` KHÔNG cấp PTY bên trong container: runtime cấp PTY trên
    devpts của HOST rồi dup2 slave vào 0/1/2 của tiến trình exec. Container
    không cần mount devpts để có terminal.
  - fd đã mở là open file description — hợp lệ qua mọi setns() (user/mnt/pid);
    ioctl(TIOCSWINSZ) trên master hoạt động độc lập mount namespace.
  - /dev/ptmx host là 0666 → rootless-safe.
- Áp dụng minict: **root cause #206 xác nhận** — rootfs alpine minirootfs
  KHÔNG có /dev/ptmx và sandbox không mount devpts (đo 2026-11-26: rootfs/dev
  chỉ null/random/urandom/zero) → openpty() SAU setns(mnt) ENOENT → die →
  session chết trước frame đầu → `/stream` "not found". Fix: cấp PTY host-side
  TRƯỚC enter_instance_ns() (D77) — MD62 từ SKIP (5.10) thành PASS.

## Nguồn 2 — readiness handshake pattern cho subprocess child
- URL: Go os/exec `Cmd.Wait` docs + passt/pasta `--fd` handshake precedent
  (daemonic setup rồi báo ready qua fd trước khi serve)
- Tóm tắt:
  - Báo thành công giả xảy ra khi API trả id ngay sau Start() mà chưa biết
    child sống: lỗi nào chết trong setup (setns/openpty/fork) chỉ lộ qua
    stderr/stdout. Pattern chuẩn: child ghi 1 byte trạng thái vào fd riêng
    khi setup xong (passt dùng --fd trước khi sẵn sàng serve).
  - Daemon-side: chờ byte với deadline ngắn (2s); EOF = child chết (ghép
    stderr tail); deadline = kill + báo lỗi. Không bao giờ 200-giả.
- Áp dụng minict: `--status-fd 4` ghi 'R' sau fork trong run_pty; daemon
  `waitPtyReady` chờ 2s; stderr cap 4 KB (head+tail) vào slog + error body.

## Nguồn 3 — client fallback hai lớp (stream chết trước frame đầu)
- URL: EventSource/web reconnect semantics (whatwg), SSE error semantics
- Tóm tắt: SSE error/close trước frame data đầu = "chưa từng có dữ liệu" —
  phải phân biệt với "stream kết thúc bình thường": cờ `gotFirstFrame`.
- Áp dụng minict console: POST 200 + stream chết trước frame đầu → dọn sid →
  in notice → chuyển line-mode ExecShell (không để terminal trống).

---

# Phiên 41 — Research bổ sung (issue #209, uptime + stale state)

## Nguồn 1 — docker inspect State + restart policy semantics
- URL: docker inspect (State.Status running/exited) + `--restart=unless-stopped`
- Tóm tắt: docker báo container chết = `exited` (không phải "stopped" — API
  mapping của console mới là chỗ dịch cho user); restart policy `unless-stopped`
  phục vụ reboot/daemon-restart, KHÔNG phủ case kill cả cgroup/session (khi
  nhóm tiến trình chết cùng lúc, daemon cũng chết → không ai restart được).
- Áp dụng minict: heal (alive) ghi `note` + `finished_at` vào state.json —
  user thấy NGUYÊN NHÂN; console detail hiện note + gợi ý `--autostart`;
  Inspect giữ "exited" (docker parity), console map thành "stopped".

## Nguồn 2 — realtime UI: một timer dùng chung vs per-row timers
- URL: React docs (render cost) + zustand selector re-render semantics
- Tóm tắt: N row × 1s timers = N intervals + N re-render độc lập → jitter;
  một shared tick trong store (zustand set một lần) → một re-render/lần tick
  cho toàn bộ view subscribe sandboxes.
- Áp dụng console: `startUptimeTick` (1s) recompute `uptime` từ `startedAt`
  (ISO created_at) cho running sandboxes; overview/list/detail đều đọc
  `sandbox.uptime` → tự tick; dừng tick khi không có instance running (guard
  `some(...)`) để idle không re-render.
