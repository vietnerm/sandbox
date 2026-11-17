# Decision Log — 2026-11-17 (sigterm): D19 — pidns-1 signal shim cho SIGTERM/SIGINT

> Bối cảnh: GĐ1 roadmap mục 1 — "SIGTERM swallowed (issue #6) trước khi thêm
> lifecycle mới". Phiên 7, tiếp nối D4 (phiên 1).

## D19: payload KHÔNG còn là pidns-1 — một shim supervisor nhỏ giữ chỗ PID 1,
cài handler rồi forward signal cho payload — CHẤP NHẬN

### Nguyên nhân gốc (sửa giả thuyết của D4)

D4 đã ghi: "kill -TERM từ host vào process trong userns → rc=0, không delivery;
giả thuyết host seccomp/LSM chặn cross-userns signal fake-success."

**Giả thuyết này SAI.** Nguyên nhân thật là semantics chuẩn của kernel, ghi rõ
trong pid_namespaces(7):

> "a process in an ancestor namespace can — subject to the usual permission
> checks — send signals to the 'init' process of a child PID namespace **only
> if the 'init' process has established a handler for that signal** ...
> SIGKILL or SIGSTOP are treated exceptionally: these signals are **forcibly
> delivered** when sent from an ancestor PID namespace."

Minict exec payload TRỰC TIẾP làm PID 1 của pidns mới → SIGTERM (disposition
mặc định, không handler) bị kernel bỏ qua ở bước prepare — khớp 100% matrix
A/B/C đã đo (rc=0, SigPnd=0 mãi; SIGKILL được ép qua). **Không phải chặn của
môi trường.** Bằng chứng bổ sung trong phiên này:

- SIG4 (control test mới): SIGTERM thẳng vào payload — **non-init** từ khi có
  shim — được delivery bình thường. Nếu host seccomp có chặn cross-userns
  signal thì control này cũng fail.
- BEFORE-line trong `journal/2026-11-17/evidence/sigterm-ab.txt`: SIGINT gửi
  cho payload có `trap` (handler đã cài) được delivery cả TRƯỚC fix — đúng
  luật "handler-installed → delivered".

### Thiết kế chọn (mô hình tini/runc/docker --init)

```
minict (host pidns, giữ forwarders INT/TERM)
  └─ shim  = pidns-1, KHÔNG exec: sigaction{TERM,INT,HUP,QUIT,USR1,USR2,WINCH}
             → fwd_to_payload() → waitpid(payload)
       └─ payload = fork+exec của shim: dispositions ĐƯỢC RESET SIG_DFL
                    trước exec, caps drop + seccomp như cũ
```

- Shim cài handler ⇒ kernel DELIVER (handler ≠ SIG_DFL) ⇒ shim forward cho
  payload, waitpid, mirror exit status: chết bởi signal → `exit(128+sig)`
  (bash convention) — parity quan sát được mà không phải dựa self-re-raise
  xuyên semantics UNKILLABLE của pidns-1.
- Wrapper gửi signal **vào shim pid** (docker semantics: signal PID 1, PID 1
  tự lo tree của mình), bỏ group-kill `-pgid` cũ — group-kill sẽ trúng payload
  2 lần (một lần qua group, một lần qua shim forward). Thành viên khác của
  tree (httpd workers…) vẫn được dọn bởi pidns teardown khi shim exit.
- Teardown D4 **giữ nguyên làm fail-safe**: shim giữ PDEATHSIG(SIGKILL) và vẫn
  là pidns-1 ⇒ chết ⇒ kernel SIGKILL phần còn lại của namespace. SIGKILL/SIGSTOP
  không cài được handler — đi đúng đường ép của kernel như cũ (SIG1 không đổi).

### Hai bẫy SIG_IGN phát hiện trong phiên (đã fix cùng lúc)

1. **Thứ tự cài handler:** `signal(SIGINT/SIGTERM, fwd_sig)` cũ đứng SAU fork
   pidns ⇒ shim/payload kế thừa disposition lúc spawn. Job background khởi động
   từ shell non-interactive bị POSIX đặt SIGINT/SIGQUIT = SIG_IGN ("asynchronous
   list"), và **SIG_IGN sống sót qua fork+exec** ⇒ payload không bao giờ nhận
   SIGINT dù shim forward (SIG3 đỏ). Fix: cài forwarders TRƯỚC fork pidns.
2. **Blocked-mask + dispositions sống sót qua exec:** payload phải
   `sigprocmask(SIG_SETMASK, old)` (mask bị chặn qua exec otherwise) VÀ reset
   {HUP,INT,QUIT,TERM,USR1,USR2,WINCH} về SIG_DFL trước exec — payload phải
   nhìn thấy đúng signal-world của một lệnh trần.

ENV FACT kiểm chứng bằng Popen (không qua shell): native `busybox sleep` có
SigIgn=0 (mọi số SigIgn=0x6 đo trước đó đều do quy tắc async của bash launcher,
không phải busybox tự ignore). Cảnh báo test: `sh -c 'sleep'` non-interactive
nuốt SIGINT thẳng vào shell khi đang chờ foreground child — payload test INT
phải dùng `trap INT` + bg `sleep & wait` (wait builtin là wait có thể ngắt).

### Hệ quả / trade-off

- cold start **+0.17 ms** p50 (1 fork + waitpid): A/B N=40 trước 1.66 ms vs
  sau 1.83 ms — `journal/2026-11-17/evidence/sigterm-ab.txt`; perf-check N=40
  PASS (1.85/1160/2.02). Còn rất xa ngưỡng 100 ms.
- PID1 test đổi语义: payload = PID 2 (shim = PID 1) — như `docker run --init`.
- `minict.sh down` giờ graceful thật: TERM → forwarded → payload chết → shim
  mirror → minict exit 143; đường kill -9 sau 0.5s chỉ còn là backstop.
- Nền cho control plane GĐ2: stop có graceful đã có ở tầng runtime; exec-into-
  instance (gate research setns) vẫn chờ đo trên runner.
- Suite isolation 17 → 20 (SIG2 TERM-wrapper 143, SIG3 INT-wrapper 130,
  SIG4 control TERM-payload 143); tổng baseline 47 + web 6 = 53.

### Bị từ chối

- **Tự re-raise signal trong shim** (`kill(getpid(), sig)` sau khi restore
  SIG_DFL, kiểu runc): không cần — `_exit(128+sig)` cho quan sát parity như
  nhau từ caller mà không phụ thuộc hành vi kernel với self-signal của pidns-1.
- **Group-kill từ wrapper** (giữ -pgid): double-signal payload; từ chối.
- **Shim có seccomp filter:** shim không exec, capdrop, attack surface = byte
  pump signal-forwarder; cùng posture với relay D16 (không filter). unshare/
  setns đã EPERM sau capdrop.

## D21: shim_reap — SIGCHLD handler reaps MỌI con (pattern tini) [review-fix
theo SIA audit PR #21, 2026-11-17, mức medium]

### Vấn đề (audit)
`shim_wait_loop` cũ chỉ `waitpid(payload)`. Grandchild mồ côi trong pidns được
re-parent về pidns-1 (= shim) và thành zombie vĩnh viễn trong lifetime của
payload — tái hiện: `sh -c 'for i in 1..5; do (sleep 0.1 &); done; sleep 2'`
→ 5 tiến trình `Z` dưới shim. Vô hại với lệnh ngắn, nhưng instance chạy lâu
(nginx, GĐ2) rò bảng pid.

### Giải pháp
- `shim_reap` (SIGCHLD handler, AS-safe): vòng `waitpid(-1, …, WNOHANG)` reap
  mọi con chết (orphan lẫn payload); chỉ payload set flag `g_payload_done` +
  lưu status vào `g_payload_status` (sig_atomic_t) để main loop mirror đúng
  một lần. errno save/restore để không clobber errno của `sigsuspend`.
- `shim_wait_loop` mới: vòng `while (!g_payload_done) sigsuspend(&empty)`.
- **SIGCHLD vào tập block quanh payload fork** (lost-wakeup): nếu payload chết
  giữa `fork()` và lúc cài handler, SIGCHLD mặc định là ignore → treo vô hạn.
  Block từ trước fork, unblock sau khi cài handler + set `g_payload_pid` →
  SIGCHLD pending phát đúng lúc (cơ chế "pending signals fire NOW" sẵn có).
- `SA_NOCLDSTOP`: SIGCHLD do STOP không phải chết — handler chỉ quan tâm exit.
- Không đợi grandchild SỐNG sau khi payload chết: WNOHANG reaper thoát ngay,
  process còn sống chết theo teardown pidns (kernel SIGKILL toàn namespace khi
  pidns-1 thoát, pid_namespaces(7)) — hành vi docker-init.

### Bị từ chối
- `waitpid(-1)` blocking thay `waitpid(payload)` blocking: payload kiểu daemon
  (`cmd &`) sẽ treo minict đến khi grandchild tự chết — regression cho use-case
  service. WNOHANG + teardown pidns đúng hơn.

### Kiểm chứng
- Suite isolation 20 → **21** (SIG5 "orphan grandchildren reaped by shim, no
  zombies"): host-view đếm `ps -eo ppid=,stat=` PPID=shim stat Z — zombie thấy
  từ host bất kể pidns. Tổng baseline 48 + web 6 = 54.
- Negative control (bắt buộc audit): mutant đổi `waitpid(-1,…)` →
  `waitpid(payload,…)` (không reap orphan) → SIG5 đo được **5 zombie → FAIL**;
  bản thật → 0 zombie → PASS. Test không rỗng.
- Env local (uid 1001, kernel 5.10.134): isolation 21/21 + seccomp 20/20 +
  netns 7/7 + web 6/6 + perf-check PASS (1.93/1204/2.02).
- Cold start không đổi đo được ngoài nhiễu (handler chỉ chạy khi SIGCHLD).
