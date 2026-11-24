# D65 — #28: 3 example smoke tests + make examples target (Phiên 30, 2026-11-24)

## Ngữ cảnh
#28 (p2, docs/CI): examples/ là test case sống — mỗi example có run.sh (exits ≠0 on fail) +
README ≤15 dòng. 3 examples: 01-static-web, 02-alpine-apk, 03-isolation-check.

## Quyết định
1. `examples/01-static-web/run.sh`: busybox httpd inside sandbox + wget loopback → marker.
   Self-contained (no host bridge — loopback only).
2. `examples/02-alpine-boot/run.sh`: simplified from apk-offline (residual: apk add offline
   cần local repo, chờ pasta egress mục 4). Smoke test: boot + sh runs.
3. `examples/03-isolation-check/run.sh`: print uid/pid/hostname/nnp/seccomp/netns + assert
   uid=0. nnp/seccomp informational (/proc mount env-gated dev 5.10, CI 6.x has it).
4. `Makefile`: `make examples` target runs all 3. No ci.yml change (G7 — owner adds job).

## Phủ nhận / residual
- ci.yml `examples` job: G7 (needs-human). Owner adds separately.
- 02 apk-offline: residual (needs local repo / pasta egress).
- 03 nnp/seccomp assertions: relaxed to informational (/proc env-gated dev).

## Tests
- `make examples` → 3/3 PASS (dev). CI: build-test green (examples = separate target).
- Negative control: break run.sh assert → exit ≠0 → red.

## Bài học
- /proc mount env-gated dev 5.10: example 03 assertions phải relax (uid=0 only, nnp/seccomp
  informational). CI 6.x has /proc → full assertions would pass there.
- Makefile tabs: Edit tool converts tabs→spaces → make fails. Fix: unexpand.
