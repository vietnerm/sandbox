# Research — 2026-11-27 (Phiên 34, issue #171 mục 8)

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
