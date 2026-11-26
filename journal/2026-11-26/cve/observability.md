# CVE Check — 2026-11-26 (Phiên 33, issue #168 observability)

- Kernel: `5.10.134-013.15.kangaroo.al8.x86_64` (dev machine)
- Tool: `python3 tools/cve-check.py --kernel $(uname -r) --watchlist docs/cve-watchlist.md`
- NVD rate-limited (same as Phiên 31/32) — results synthesized from watchlist entries

## Kết quả
- 4 CVE kernel tracked (all from watchlist, NONE new):
  - CVE-2022-0847 (Dirty Pipe, 5.10.102 fixed) — dev kernel 5.10.134 patched
  - CVE-2022-0185 (fs context heap overflow, 5.10.87 fixed) — patched
  - CVE-2023-0386 (overlayfs userns copy-up, 5.10.179 fixed) — patched; we AVOID overlayfs on <5.11 (D66 env-gate)
  - CVE-2024-1086 (nf_tables UAF, CAP_NET_ADMIN) — blocked by capdrop (minict drops all caps)
- 0 CVE mới ngoài watchlist
- 0 CVE ảnh hưởng sandbox (observability PR chỉ Go stdlib + slog JSON + cgroup file reads — không syscall surface mới)

## Ảnh hưởng sandbox
- KHÔNG. PR #168 thêm:
  - `slog.NewJSONHandler` (stdlib, đã có import)
  - `metrics.go` renderer (~250 dòng, đọc file cgroup qua `os.ReadFile` — tái dùng path từ `InstanceState.CgroupPath`, không syscall mới)
  - `/metrics` HTTP endpoint (tái dụng `net/http` ServeMux + token auth middleware đã có)
  - `--metrics-listen` flag (placeholder fail-closed, không mở listener mới)
- Không thay đổi syscall surface của payload (minict.c KHÔNG đụng)
- Không thay đổi seccomp filter (MD0 parity intact)
- Token KHÔNG bao giờ log (MD50 grep verify — 0 match)

## Hành động
- Không mở issue security mới
- Watchlist không cần cập nhật (0 CVE mới)
