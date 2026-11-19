# Decisions — procfs trong pidns (nhánh `fix/31-proc-pidns`, issue #31)

## D39 — mount procfs TRƯỚC khi detach old root; tháo old root trong shim pidns-1

**Root cause (đo được, xác nhận giả thuyết SIA PR #24):** kernel chỉ cho một userns
mount procfs khi mount ns **đang có sẵn một procfs nhìn thấy đầy đủ**
(`fs/namespace.c`: `mount_too_revealing()` → `mnt_already_visible()`). minict
`pivot_root` rồi `umount2(old, MNT_DETACH)` ngay trong launcher, **sau đó** shim mới
mount `/proc` → ns không còn procfs nào → EPERM trên mọi kernel (máy dev 5.10, runner
azure 7.0, container 6.18 — A/B trên 6.18: main EPERM, bản sửa mount được).

**Sửa (`sandbox/minict.c`):**
- launcher vẫn `pivot_root`, nhưng **không** detach old root nữa;
- shim pidns-1 (đã ở trong pidns mới — procfs phản ánh pidns của tiến trình mount)
  `mkdir /proc` nếu thiếu, `mount("proc", "/proc", MS_NOSUID|MS_NODEV|MS_NOEXEC)`,
  rồi mới `umount2(old, MNT_DETACH)` — **fail-closed** (`die`) như cũ, và `rmdir`.
- Payload chỉ được fork **sau** bước detach, nên không bao giờ thấy host root. Giữa
  pivot và fork, launcher không chạm filesystem.
- Mount proc vẫn **warn-only**: host có seccomp chặn mount (máy dev) hoặc procfs bị che
  (overmount = không fully visible) thì sandbox chạy không có `/proc` — suy giảm, không
  mất an toàn.

**Bề mặt mới khi có /proc và vì sao chấp nhận:** payload chỉ thấy tiến trình trong
pidns của nó (PID2/PROC1), `/proc/<pid>/root` của mọi tiến trình thấy được là root
sandbox (PROC2), procfs nosuid/nodev/noexec (PROC3), payload không mount thêm procfs
được (PROC4 — seccomp `mount` + capdrop). `/proc/sys`, `kcore`, `sysrq-trigger` cần
caps ở init userns — payload đã drop toàn bộ caps. Lộ thông tin nhỏ còn lại:
`/proc/self/mountinfo` cho thấy đường dẫn host của rootfs (giống docker/podman rootless).

**Test (env-aware, không vacuous):** probe host độc lập với minict
(`unshare -r -p -f -m --mount-proc true`). Host cho phép → `/proc` phải có và đúng
pidns (PID2 mới, PROC1–PROC3, AL4); host chặn → giữ hợp đồng cũ "/proc vắng mặt"
(xfail/pass), không bao giờ PASS im lặng. PID2 hết vacuous với mutant M4 (bài học #38).
Negative control: M-old (thứ tự cũ) → 4 đỏ; M4 no-pidns → 7 đỏ; M-noflags → PROC3;
M-noumount → FS3 + PROC2 (`evidence/proc-pidns.txt`).

**Còn mở:** xác nhận trên runner CI (kernel 7.0) — nếu runner che procfs, bộ test tự
chuyển sang chế độ "absent" và ghi `NOTE`; khi đó #31 chưa đóng được.
