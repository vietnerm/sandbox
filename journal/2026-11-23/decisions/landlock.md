# D61 — GĐ2 mục 6: Landlock ro-root enforcement (issue #149, Phiên 26, 2026-11-23)

## Ngữ cảnh

Manifest GĐ2 mục 6 DoD: Landlock LSM cho fs-policing (kernel ≥5.13, runner đạt).
spec.ReadOnly hiện noted-only (D2: no overlayfs/ro-root). Landlock (kernel ≥5.13) cho
phép enforce ro-root rootless: payload đọc rootfs, write chỉ /tmp + /dev/shm (tmpfs).

## Quyết định

Chọn **Landlock enforce spec.ReadOnly ro-root** (CI-only, kernel ≥5.13):

1. `sandbox/minict.c landlock_enforce()`: nếu `g_spec_readonly` + Landlock available
   (create_ruleset OK): ruleset handled all core fs accesses; rule "/" read-only
   (READ_FILE|READ_DIR|EXECUTE); rule "/tmp"+"/dev/shm" read-write (+WRITE|REMOVE|MAKE);
   `landlock_restrict_self`. ENOSYS (<5.13) → warn (fail-soft; pivot_root+capdrop+seccomp
   remain primary isolation).
2. `readonly` spec directive: minict.c parse → `g_spec_readonly`; daemon `store.go`
   renderSpec emit `readonly` khi `spec.ReadOnly` (thay note-only D2).
3. Landlock rules: dir rule grants subtree (union of ancestor rules); handled_access_fs =
   core set → Landlock restricts everything, allow per-path.
4. Defense-in-depth: on top of pivot_root. Even a fd-leak escape resolves to a
   Landlock-denied open (host path not under the ruleset).

## Phủ nhận / residual

- **kernel <5.13**: ENOSYS → warn, spec.ReadOnly NOT enforced (dev 5.10). LL1 SKIP.
  CI ubuntu-24.04 (kernel 6.x) → Landlock enforce, LL1 PASS.
- **bind mounts writable**: not tracked per-bind; writable binds inherit "/" read-only
  → write denied. Known limitation. Residual: per-bind Landlock write rule.
- **/dev devices**: Landlock may deny device writes (e.g. /dev/null). Not tested
  (payload doesn't touch /dev). Residual: /dev rule if needed.
- **ro-root breaks write-to-/ workloads**: payload must be designed for ro-root + tmpfs
  writable (Docker --read-only parity). spec.ReadOnly opt-in.

## Tests

- **LL1** (CI, ≥5.13): spec.ReadOnly → write `/etc/ll-deny` EACCES (Landlock denies),
  write `/tmp/ll-ok` OK (tmpfs writable). Assert TMP_OK + ETC_DENIED. Dev <5.13 SKIP
  (landlock probe: create_ruleset syscall ENOSYS).
- Mutant (CI): skip `landlock_restrict_self` → write /etc succeeds → LL1 ETC_DENIED
  assertion red. CI-only mutant (buildable local but runs on CI only).

## Bài học

- **Landlock subtree semantics**: dir rule grants subtree (union of ancestor rules).
  "/" read-only + "/tmp" write → /tmp writable, /etc read-only. Verified by LL1 (CI).
- **CI-only feature**: dev 5.10 ENOSYS → can't test local. LL1 gated by landlock probe
  (create_ruleset syscall). CI ubuntu-24.04 (kernel 6.x) verifies.
- **spec.ReadOnly enforcement**: D2 noted-only → Landlock makes it real (ro-root).
  Docker --read-only parity rootless.
