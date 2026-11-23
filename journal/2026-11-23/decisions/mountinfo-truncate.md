# D58 — #117: mountinfo enumeration truncation → fail-closed die (issue #117, Phiên 23, 2026-11-23)

## Ngữ cảnh

from-audit SIA PR #114 (Phiên 17, S3 #113/D52) mức low: `sandbox/minict.c` đường
ENOSYS (kernel <5.12 mountinfo-fallback) per-submount enumeration dùng
`snprintf(b->submounts[n], sizeof(...), "%s", mp+len)` — src-relative suffix >255 byte
bị cắt im lặng → sai path remounted → partial-ro config lie (lỗ trong posture fail-closed).

## Quyết định

Chọn **die fail-closed** thay vì warn (issue gợi ý warn, nhưng die đúng hơn):

1. Trước snprintf, check `strlen(mp+len) >= SPEC_SUBMOUNT_PATH` → die (exit 125) với
   message rõ ràng (bind src, suffix len, cap). Sau check, `memcpy` (an toàn — đã verify len < cap).
2. Consistent với sibling caps cùng function: >16 submounts die (line 983-988),
   mp>1024 die (line 960-963). Truncation là cùng class (config lie → partial ro) → die.
3. Issue (#117, low) gợi ý "warn + evidence note" (minimal fix). Die là improvement:
   đóng lỗ silent-truncation outright, không để partial-ro possible. SDDA cải thiện issue
   suggestion với reasoning (D58).

## Phủ nhận / residual

- **kernel ≥5.12 không affected**: mount_setattr(AT_RECURSIVE) path không enumerate
  mountinfo → truncation code không reach. BIND4 test SKIP trên ≥5.12 (gated S3_MECH).
  Dev 5.10 (mountinfo-fallback) → BIND4 PASS, verify fix.
- **NAME_MAX 255**: test dùng 255-byte single component → suffix "/<255>" = 256 byte
  >= 256 → die. Path >4096 (PATH_MAX) không test (cần nested dirs, complex).
- **warn vs die**: die có thể break một workload có submount path >255 legitimate (rare).
  Nhưng partial-ro silent là worse (config lie). Workload nên dùng shorter paths hoặc
  mount_setattr kernel (≥5.12).

## Tests

- **BIND4** (`tests/test_isolation.sh`): 255-byte submount name under bind src → suffix
  256 byte → die. Assert rc != 0 + stderr "exceeds cap / refusing truncated". Gated
  `S3_ENV == ok && S3_MECH == mountinfo-fallback` (kernel <5.12). ≥5.12 SKIP.
- Mutant: revert to snprintf (no check) → BIND4 đỏ (minict succeeds với truncated path,
  không die). C mutant buildable local — SCR runs.

## Bài học đo được

- **snprintf silent truncate**: `snprintf(dst, n, "%s", src)` cắt im lặng nếu src >n-1.
  Luôn check strlen trước hoặc dùng return value. Lỗ class "config lie" — silently
  half-protected worse than refuse.
- **mountinfo-fallback là dev-only path**: kernel ≥5.12 dùng mount_setattr, không
  enumerate. Fix chỉ reach trên <5.12. Test BIND4 gated accordingly (SKIP trên CI 6.x).
- **from-audit low → die improvement**: issue gợi ý warn (low severity, minimal). SDDA
  đánh giá die đúng hơn (consistent sibling caps) + cải thiện với reasoning (D58).
