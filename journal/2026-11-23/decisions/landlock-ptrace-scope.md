# D64 — #157: Landlock ptrace-scope fix (shim domain) + fail-closed complete + default re-enabled (Phiên 29, 2026-11-23)

## Ngữ cảnh

#157: Phiên 28 (#154) có 2 lỗi:
1. **Fail-closed chưa trọn**: `open("/")`, `landlock_add_rule`, `landlock_restrict_self` errors
   → warn+return (ReadOnly instance boot ghi được — config lie).
2. **PROC2 chẩn đoán sai** (magic-link) → default policy revert oan. Thực: **ptrace scope** —
   Landlock domain process chỉ ptrace-access (`readlink /proc/<pid>/root` cần
   `PTRACE_MODE_READ`) same/descendant domain. `landlock_enforce` chạy ở **payload** (sau fork
   từ shim) → shim ngoài domain → PROC2 đọc `/proc/1/root` (shim) EACCES.

## Quyết định

1. **Áp default policy ở SHIM** (sau pivot_root, trước fork payload):
   `landlock_enforce_default()` — "/" RW. Shim+payload (no spec.ReadOnly) **cùng domain** →
   PROC2 ptrace OK. **Default re-enabled**.
2. **Payload RO layer** (child domain): `landlock_enforce_readonly()` — chỉ nếu spec.ReadOnly.
   "/" RO + /tmp+/dev/shm RW. Child domain (more restrictive). **ALL errors die** (fail-closed
   #157 mục 1 — closes warn+boot holes).
3. **ABI probe**: `landlock_abi()` = `create_ruleset(NULL,0,VERSION)`. ABI 1 (5.13-5.18) →
   no REFER; ABI ≥2 (5.19+) → REFER. `ll_handled_for(abi)`.
4. **Fail-closed complete**: readonly function — create_ruleset/open/add_rule/restrict_self
   errors ALL die. Default function — errors warn (defense-in-depth) except ENOSYS+
   spec.ReadOnly → die.

## Phủ nhận / residual

- spec.ReadOnly payload (child domain) can't ptrace shim (parent) — but PROC2 runs in
  default instances (same domain), unaffected.
- bind mounts writable (spec.ReadOnly): residual D61 — not tracked per-bind.
- ABI 1 (5.13-5.18): no REFER → rename/magic-link might fail for spec.ReadOnly. CI 6.x
  (ABI ≥2) OK.

## Tests

- LL1 (CI): spec.ReadOnly → ro-root. LL2 (dev): spec.ReadOnly → die 126. LL3 (any):
  default → boots + /tmp OK.
- **PROC2 (CI)**: now passes (shim+payload same domain, ptrace allowed) — the key #157
  verification.

## Bài học

- **ptrace scope**: Landlock domain process ptrace same/descendant only. Applying Landlock
  at shim (not payload) keeps shim+payload same domain → `/proc/<pid>/root` readlink works.
- **Fail-closed**: ALL error branches must die for operator-requested features (spec.ReadOnly).
  warn+boot = config lie (half-protected).
- **ABI probe**: `create_ruleset(NULL,0,VERSION)` returns ABI version. REFER needs ≥2. Probe
  → conditional handled set (avoid EINVAL on ABI 1).
