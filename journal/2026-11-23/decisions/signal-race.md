# D62 — #30: signal race fork(pidns)→g_child_pid closed (parent block + child UNBLOCK) (Phiên 27, 2026-11-23)

## Ngữ cảnh

from-audit #30 (SIA PR #21 low): shim inherits fwd_sig (g_child_pid=0) before pidns fork.
SIGTERM/SIGINT in window fork→sigprocmask(SIG_BLOCK) → fwd_sig no-op → signal swallowed.
Issue suggested: parent block fwd before fork; "con giữ nguyên luồng" — but child
restore-`old` preserves the parent's inherited block → child handlers never fire (broken).

## Quyết định

1. **Parent**: block SIGTERM/SIGINT before pidns fork (`sigprocmask SIG_BLOCK → g_parent_sig_old`).
   Child inherits blocked → window fork→child-BLOCK has SIGTERM QUEUED (not delivered to
   no-op fwd_sig). Child race closed.
2. **Parent**: unblock after `g_child_pid` set (`sigprocmask SIG_SETMASK g_parent_sig_old`) →
   pending SIGTERM fires fwd_sig with g_child_pid set → forwarded to shim. Parent race closed.
3. **Child shim restore**: `sigprocmask(SIG_SETMASK, &old)` → `SIG_UNBLOCK &fwd`. `old`
   captures the parent's inherited block; restoring it would keep SIGTERM/SIGINT blocked →
   shim's fwd_to_payload never fires. UNBLOCK fwd → clean mask → handlers receive queued
   signals.
4. **Child payload restore**: same UNBLOCK fwd → clean mask for payload.

Effective masks unchanged (only fwd set blocked; UNBLOCK = clean = old default). SIG1-SIG5 PASS.

## Phủ nhận / residual

- Child race (fork→child-BLOCK) closed (parent block inherits). Parent race (fork→g_child_pid)
  closed (parent block + unblock after g_child_pid).
- Stress test SIG_STRESS (20x SIGTERM during spawn → all 143): best-effort, µs window.
  SIG1-SIG5 are the deterministic regression. SIG_STRESS may not catch the race if timing
  misses (passes either way) — but with the fix, all forward correctly.
- `old` var now unused at restore (captured at the child BLOCK but restore uses UNBLOCK fwd).
  Kept for the sigprocmask BLOCK call signature.

## Tests

- SIG1-SIG5: PASS (regression — signal forwarding intact, exit 143/130/143).
- SIG_STRESS (new): 20x SIGTERM during spawn → exit 143 all 20. CI verifies (fast CPU).
- Mutant: revert parent block (no sigprocmask before fork) → race reopens → SIG_STRESS
  might catch (flaky) + SIG1-SIG5 still pass (race is µs, not deterministic).

## Bài học

- Issue's "con giữ nguyên luồng" incomplete: parent block propagates via fork; child
  restore-`old` preserves it → child handlers blocked. Correct fix: child UNBLOCK (not
  restore inherited `old`) → clean mask. Effective mask unchanged (fwd set only).
- fork + signal mask: any parent block before fork propagates to child. To close the parent
  race without breaking the child, the child must UNBLOCK (not restore inherited `old`).
