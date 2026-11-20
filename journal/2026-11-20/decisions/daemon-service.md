# D47 — daemon service hoàn thiện: CLI sandbox + systemd --user unit + autostart + A3 reconcile (#91, GĐ2 mục 1)

Ngày: 2026-11-20 (Phiên 12). Nhánh: `feat/91-daemon-service`.

## Vấn đề

Roadmap GĐ2 mục 1 (daemon user-level + REST API) còn thiếu 4 mảnh cho DoD:
CLI `sandbox` (chỉ có curl thủ công trong test), systemd `--user` unit,
instance tồn tại qua reboot (autostart), reconcile loop định kỳ (A3 —
GAPS.md A3: "Not yet built: periodic full reconcile loop").

## Quyết định (từng mảnh, lý do)

1. **CLI = thin shell over `contract.HTTPClient`** — HTTPClient đã implement
   đủ `contract.Runtime` (14 op) qua HTTP; CLI chỉ map argv → op + in JSON.
   Không thêm logic/default nào (quy tắc NO MAGIC DEFAULTS của contract).
   Tránh viết client thứ hai lặp transport logic. `--cmd` chấp nhận comma
   hoặc space-split (không shell-quote — người dùng gọi trực tiếp, test
   suite ghi rõ format).
2. **Unit = template + render tại install** (`deploy/minictd.service.in` +
   `deploy/install.sh`) — pattern Podman quadlet: systemd user unit cần
   path tuyệt đối; `%h` render muộn gây verify-fail ngoài user context.
   install.sh: render sed fail-closed (còn placeholder → abort), copy
   binary, tạo token 0600 một-lần, KHÔNG tự enable/linger (chỉ hướng dẫn
   — enable-linger có thể cần manager prompt; không tự chạy lệnh có tác
   dụng hệ thống).
3. **Autostart policy: chỉ recreate `state=running`** — instance
   operator-stopped (`state=stopped`, `killed_by=stop`) vẫn stopped. Đây là
   semantics `--restart=unless-stopped` của docker nhìn từ daemon. `Start()`
   đã idempotent cho instance sống (MD16 flow) nên daemon-restart vô hại;
   wakeup thật chỉ phục vụ reboot/daemon-crash. Heal trước khi spawn dùng
   chính `alive()` (pid-reuse-safe từ D25-era code).
4. **A3 = `--reconcile-interval N` (giây, 0=off) + `Store.ReconcileAll()`** —
   goroutine ticker trong daemon; heal-only (KHÔNG spawn từ reconcile —
   Start/autostart là quyết định tường minh của daemon). State file hỏng
   được skip + warn, không crash daemon (Inspect sẽ surface sau).
5. **Hardening unit tối giản** — chỉ các option không dùng mount namespace
   và không filter syscall cho cả cgroup (payload dùng unshare/mount trong
   cùng cgroup): `NoNewPrivileges`, `MemoryDenyWriteExecute`,
   `LockPersonality`, `UMask 0077`. Cố ý KHÔNG có `ProtectSystem`/
   `ProtectHome` (mount ns cho user service), KHÔNG `RestrictSUIDSGID`
   (cgroup-wide, có thể chặn syscalls của payload) — seccomp blocklist của
   daemon/payload (D13/D25) đã enforce posture riêng.

## Test (mới, vào `make test-minictd` — 20 → 23 case)

- **MD20** CLI round-trip: list/up/inspect/exec (stdout thật trong ns)/
  stop/inspect exited/rm — chứng minh CLI nói được với daemon trên toàn
  lifecycle.
- **MD21** autostart: kill daemon (payload Setsid sống) → kill shim (reboot
  semantics: state=running, PID chết) → daemon mới `--autostart` → heal +
  wake-recreate từ spec.json → PID MỚI ≠ PID cũ → exec đọc marker từ rootfs
  cũ (filesystem persist) → rm sạch.
- **MD22** deploy: install.sh render vào staging HOME → file unit + binary
  tồn tại + ExecStart đúng path + không leak placeholder + `systemd-analyze
  verify` exit 0. Env-gated SKIP (rc 125) khi không có systemd-analyze.
- Go unit tests (`internal/minict/reconcile_test.go`): dead-running healed /
  live-pid untouched / broken-state skipped / AutostartNames đúng tập /
  missing-root rỗng — không spawn payload thật (pidAlive là /proc-derived:
  PID 999_999 chết; `os.Getpid()` + ticks=0 = sống).

## Residual (khai minh bạch)

- Lingering KHÔNG được test e2e (runner CI không có systemd user instance
  đầy đủ trong job container) — chỉ verify cú pháp unit + các cờ daemon;
  enable-linger là bước thủ công có docs.
- CLI chưa có `logs --follow` (v1: copy một lần — Logs(follow=false)).
- Reconcile ticker không tự restart payload (heal-only) — có chủ đích (xem
  quyết định 4); restart = autostart path.
- `--reconcile-interval` và `--autostart` không tác động MD1–MD19 (mặc định
  off) — daemon flow cũ giữ nguyên.
