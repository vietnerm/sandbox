# D63 — #154: Landlock fail-closed (spec.ReadOnly + no Landlock) + default policy all instances (Phiên 28, 2026-11-23)

## Ngữ cảnh

Phiên 26 (#149/D61) Landlock ro-root: spec.ReadOnly + ENOSYS → warn (fail-soft). Chỉ chạy
khi spec.ReadOnly. Cần: (1) fail-closed khi operator yêu cầu ro-root mà kernel không hỗ trợ;
(2) default Landlock policy cho mọi instance (defense-in-depth).

## Quyết định

1. **Fail-closed**: spec.ReadOnly + Landlock unavailable (ENOSYS/EOPNOTSUPP, kernel <5.13)
   → die (exit 126). Operator asked for ro-root, can't get it → refuse boot half-protected
   (consistent bind :ro + seccomp posture).
2. **Default policy**: `landlock_enforce()` chạy cho MỘI instance (not just spec.ReadOnly).
   Default: "/" rule LL_ACCESS_RW (all accesses) — payload restricted to rootfs subtree
   (host denied, defense-in-depth on pivot_root). ENOSYS → warn (fail-soft; default is
   defense-in-depth, not operator-requested).
3. spec.ReadOnly: "/" LL_ACCESS_RO + /tmp+/dev/shm writable (existing ro-root).
4. `root_access = g_spec_readonly ? LL_ACCESS_RO : LL_ACCESS_RW`.

## Phủ nhận / residual

- spec.ReadOnly on dev (<5.13) → die (can't boot). Operator must use kernel ≥5.13 for
  ro-root. LL2 verifies (dev die 126).
- Default on dev (<5.13) → warn (fail-soft) → boots without Landlock. LL3 verifies.
- Default Landlock (restrict to "/" RW) has no testable DENY (post-pivot, all paths under
  "/"; pivot_root+harden_payload_fds already prevent host access). LL3 verifies no
  false-deny (boots + /tmp OK). Defense-in-depth value = marginal (other layers cover),
  but always-on Landlock is a baseline.
- bind mounts writable (spec.ReadOnly): residual (D61) — not tracked per-bind.

## Tests

- LL1 (CI ≥5.13): spec.ReadOnly → ro-root (existing).
- LL2 (dev <5.13): spec.ReadOnly → die 126 (NEW, fail-closed verified dev).
- LL3 (any): default instance → boots + /tmp write OK (NEW, default no false-deny).

## Bài học

- Fail-closed posture: operator-requested feature (spec.ReadOnly) unavailable → die (refuse
  half-protected). Defense-in-depth (default) unavailable → warn (fail-soft). Distinction:
  operator-requested vs always-on.
- Default Landlock restrict-to-rootfs: marginal testable value (post-pivot no host paths),
  but always-on baseline + no false-deny (LL3).
