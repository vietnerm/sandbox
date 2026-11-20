# D48 — CG1/S2: cgroups v2 resource enforcement cho instance (#4, GĐ2 mục 5)

## Bối cảnh
GAPS CG1/S2 là gap số 1 trong danh sách "ordered by integration risk": Memory/
CPUs/CPUShares/PidsLimit của RunSpec chỉ là Notes + rlimit nproc (per-uid
approximation). Issue #4 (p2, feature) mở từ đầu dự án với dev cgroups v1.

## Quyết định
1. **cgroups v2 islands model** (systemd.io/CGROUP_DELEGATION + kernel
   cgroup-v2.rst, 3 nguồn verified trong `journal/2026-11-20/research.md`
   Phiên 13):
   - daemon tự migrate vào `<base>/minictd-daemon/` → base không còn PID ở
     leaf → enable `+memory +cpu +pids` trên `<base>/cgroup.subtree_control`
     (delegation chỉ EXPOSE controllers, delegatee phải tự enable — systemd.io);
     instance cgroups nằm dưới `<base>/minictd-apps/sandbox-<name>`
     (no-internal-process rule tuân thủ).
   - shim PID migrate vào instance cgroup SAU spawn (child kế thừa cgroup
     daemon lúc fork → sibling move; Delegation Containment: common ancestor
     = base — owner user → OK).
2. **Fail-soft, không fail-closed**: không có base v2 writable (v1 dev, không
   systemd delegation) → giữ hành vi pre-CG1 (Notes + rlimit nproc) + note
   lý do đo được. Enforcement là additive — baseline 124 case phải xanh trên
   mọi host. `renderSpec(spec, ports, cgAvail)` đổi note theo posture thật.
3. **Base resolution** (`cgResolveBase`): override `--cgroup-root` /
   `$MINICTD_CGROUP_ROOT` (phải chứa cgroup.controllers v2 + daemon phải đã
   nằm TRONG base — containment check) > `/proc/self/cgroup` `0::` path của
   daemon (root "/" bị từ chối — runner bootstrap bằng sudo + move daemon
   một lần trong suite). Manager cache 1 lần per Store (self-move đổi
   /proc/self/cgroup → re-resolve sẽ lệch base).
4. **Mapping** (docker semantics): Memory→`memory.max` bytes; CPUs "1.5"→
   `cpu.max "150000 100000"` (period 100000); CPUShares→`cpu.weight` tuyến
   tính shares/1024*100 clamp [1,10000] (1024≡100 default); PidsLimit→
   `pids.max`. MemorySwap KHÔNG map (v2 memory.swap.max là knob riêng —
   note minh bạch, không silently ignore). rlimit nproc GIỮ nguyên khi
   PidsLimit>0 (defense-in-depth).
5. **Stats ưu tiên cgroup**: `memory.current` + `pids.current` + `cpu.stat`
   usage_usec (window 250ms) khi instance có CgroupPath; MemLimitBytes đọc
   `memory.max` thật; fallback /proc RSS tree khi đọc fail. Field mới
   `CgroupPath` trong state.json (persist — cleanup qua Stop/Remove/heal
   kể cả sau daemon restart).
6. **Cleanup**: Stop/Remove/heal → rmdir instance cgroup (best-effort,
   idempotent). Kernel tự thu hồi cgroup rỗng.
7. **Tests**: Go unit trên FAKE cgroup FS (12 test: resolve×4, ensureApps×3,
   limits×2, apply×2, stats, cleanup) + shell e2e `tests/test_cg.sh`
   CG0–CG6 env-gated (probe-based SKIP dev v1; PASS thật trên runner —
   bootstrap sudo: mkdir+chown base + enable controllers ROOT + move daemon
   1 lần). Suite vào `make test` (10 suites).

## Residual (ghi rõ, không giấu)
- Runner verify (memory.max thật + OOM + pids burst) nằm ở CI của PR — dev
  không thể đo (v1). Nếu runner không cho sudo bootstrap → suite SKIP và
  cần follow-up (mở issue).
- `cpu.weight` mapping tuyến tính ≠ công thức docker chuẩn (1+(s-2)*9999/
  262142) — khác biệt nhỏ, ghi ở đây; đổi sau nếu workload thật đòi hỏi.
- `memory.swap.max`, `io.max`, `cpu.pressure` chưa map (không cần cho DoD).
