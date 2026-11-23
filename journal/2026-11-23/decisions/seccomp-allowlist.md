# D59 — seccomp allowlist theo template (GĐ2 mục 3a, issue #139)

## Ngữ cảnh

Blocklist D11 (mặc định ALLOW + deny-list 32 syscall + arg-check clone/socket) là
mức duy nhất hiện có. Mọi syscall mới của kernel (io_uring, userfaultfd class —
hai cái đã phải vá tay vào blocklist) tự động nằm trong bề mặt cho tới khi ai đó
vá blocklist. Owner chốt 2026-11-23 (manifest §3): bảo mật ưu tiên số 1, seccomp
allowlist theo template làm trước GĐ2 mục 4–8.

## Quyết định

1. **Format profile**: file text `minictd/internal/minict/profiles/<name>.allow`,
   mỗi dòng `<nr> <tên>`, `#` là comment. Số x86_64 trực tiếp (manifest §5 neo
   x86_64; không libseccomp — giữ stdlib-only + không dep ngoài). Tên chỉ là
   documentation cho người review. Thư mục nằm dưới `minictd/` vì `go:embed`
   yêu cầu module-relative; nhánh issue nói `seccomp/profiles/` — vị trí thật
   được ghi ở đây và ở README của thư mục.
2. **Nguồn profile = đo thật**: `tools/syscall-probe.c` (ptrace PTRACE_SYSCALL,
   TRACESYSGOOD|FORK|VFORK|CLONE|EXEC) đếm tập syscall của đúng cây con của
   `minict` (payload). Workload đo nằm trong `tools/seccomp-workloads/<name>.sh`
   — không chép Docker default (#29). `tools/seccomp-profile-gen.sh` chạy probe
   trên workload, union nhiều lần chạy, annotate tên từ
   `/usr/include/.../unistd_64.h`, ghi `.allow`.
3. **Data-flow**: `RunSpec.SeccompProfile` (contract) → `renderSpec` nhúng số
   syscall thành `seccomp_allow <nr>[,...]` trong MINICT_SPEC (payload, minict.c
   parse host-side trước unshare) VÀ `MINICT_SECCOMP_ALLOW=<nr>[,...]` vào
   env-file exec setns (minict-exec parse giống RLIMIT pattern — không export
   vào env của cmd). Template carry profile qua spec.json (save-template sao
   spec.json — template:<name> tự mang allowlist của instance gốc).
4. **Filter (cả hai file, copy verbatim — MD0)**: arch-check → load nr → SÀN
   blocklist D11 (EPERM) → arg-check clone-NS-mask + socket AF_ALG/AF_NETLINK →
   clone3 ENOSYS (fallback clone có arg-check) → JEQ allowlist→ALLOW → default
   EPERM. Allowlist không bao giờ nới lỏng blocklist: entry blocklist bị deny
   kể cả khi có trong profile.
5. **Opt-in rõ ràng**: profile áp khi RunSpec.SeccompProfile đặt tên hợp lệ
   (API/CLI). Không auto-apply theo image trong PR này — auto-magic làm break
   workload chưa đo (apk/apt online, DNS…); tên profile đi vào spec.json nên
   template đã mang nó qua các lần Run sau. Blocklist D11 vẫn mặc định khi
   không đặt profile (đúng #29).
6. **Profile phát hành trong PR**: `busybox` (đo workload suite-isolation trên
   máy dev), `alpine` (sh workload), `alpine-nginx` (boot stack nginx+fcgiwrap+
   CGI+sqlite thật + curl), `debian` (sh workload). Đo trên kernel dev 5.10;
   runner (7.0) validate qua test e2e (residual: đo lại trên runner nếu surface
   khác).

## Phương án bị loại

- **libseccomp** — dep ngoài, vi phạm stdlib-only; số syscall cũng phải neo arch.
- **Auto-apply theo image name** — một profile sai một syscall = workload chết
  dở run; để người vận hành chọn (profile gắn template qua spec.json là đủ).
- **JSON profile** — C phải có parser JSON hoặc Go phải map tên→số (bảng cần
  maintain thêm một nơi); text `nr name` rẻ hơn và MD0-parity không đụng.
- **default KILL_THREAD cho syscall lạ** — EPERM là mặc định OCI; errno cho phép
  libc vẫn shutdown sạch, KILL làm flaky test khó chẩn đoán.

## Hệ quả

- MD0 vẫn là rào parity: vùng so (enum + g_blocklist + install_seccomp) phải
  byte-identical giữa `sandbox/minict.c` và `minictd/cmd/minict-exec/minict-exec.c`;
  phần parse directive (khác format) nằm ngoài vùng so.
- Test mới: suite seccomp thêm case allowlist (workload chạy / syscall ngoài
  danh sách EPERM / control), MD e2e exec-parity, Go unit cho load/render.
- Mutant bắt buộc: bỏ install allowlist (SE đỏ), bỏ render (SE đỏ), bỏ emission
  exec (MD đỏ).
- Residual ghi roadmap: auto-apply, arm64 số syscall, đo lại trên runner.
