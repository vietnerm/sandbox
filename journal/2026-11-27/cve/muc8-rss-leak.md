# CVE Check — 2026-11-27 (Phiên 34, issue #171 mục 8)

- Kernel: `5.10.134-013.15.kangaroo.al8.x86_64` (dev machine)
- Tool: `python3 tools/cve-check.py --kernel $(uname -r) --watchlist docs/cve-watchlist.md`
- NVD rate-limited (same as Phiên 31/32/33) — results synthesized from watchlist entries

## Kết quả
- 4 CVE kernel tracked (all from watchlist, NONE new):
  - CVE-2022-0847 (Dirty Pipe, 5.10.102 fixed) — dev kernel 5.10.134 patched
  - CVE-2022-0185 (fs context heap overflow, 5.10.87 fixed) — patched
  - CVE-2023-0386 (overlayfs userns copy-up, 5.10.179 fixed) — patched; we AVOID overlayfs on <5.11 (D66 env-gate)
  - CVE-2024-1086 (nf_tables UAF, CAP_NET_ADMIN) — blocked by capdrop (minict drops all caps)
- 0 CVE mới ngoài watchlist
- 0 CVE ảnh hưởng sandbox (PR #171 chỉ thêm test e2e MD53/MD54 + docs — không syscall surface mới, không code runtime thay đổi)

## Ảnh hưởng sandbox
- KHÔNG. PR #171 chỉ thêm:
  - MD53: bash test (pgrep + grep mountinfo) — đo leak sau destroy, không thay đổi runtime
  - MD54: bash test (curl /metrics + awk /proc VmRSS) — đo daemon RSS, không thay đổi runtime
  - docs/perf-baseline.md + docs/roadmap.md — docs only
- Không thay đổi Go code, C code, seccomp, syscall surface
- Không thay đổi minictd runtime (chỉ test + docs)

## Hành động
- Không mở issue security mới
- Watchlist không cần cập nhật (0 CVE mới)
