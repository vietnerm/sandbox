# SBG Branch Plan — 2026-11-20 (MODE=plan, Phiên 9)

Input: MODE=plan, SCOPE=all, DATE=2026-11-20.

## Thu thập

- `git fetch --prune origin '+refs/heads/*:refs/remotes/origin/*' --tags`: OK.
- Nhánh remote: **chỉ `main`** (`2c7e839` — ledger Phiên 8 #82).
- PR mở: **0** (REST `GET /pulls?state=open` — `gh` CLI không có trên máy này,
  dùng REST API qua curl, token từ PAT owner — pattern Phiên 7).
- CI main: xanh — run #204 (ci) + #71 (unicode-guard) @ `2c7e839`.
- Nhánh local ≠ main: 0.
- Tags archive: không có mới (không DROP nào hôm nay).

## Kế hoạch nhánh

| branch | ahead/behind | PR | CI | retest | audit | quyết định | lý do |
|---|---|---|---|---|---|---|---|
| (không có nhánh nào ngoài main) | — | — | — | — | — | — | trạng thái mục tiêu §11 đã đạt từ cuối Phiên 8 |

- §0.6 dừng (halt): **không áp dụng** — CI main xanh; không có issue
  `type:security` `priority:p0` chưa triage (#8 p2 đã có watchlist track, 0
  CVE mới ngoài watchlist ở lần check gần nhất 2026-11-19; re-check Bước 6
  phiên này); host baseline sẽ đo lại ở Bước 7.

## Hành động (khi MODE=apply cuối ngày)

- Pha B/C đầu chu trình: rỗng (0 PR mở).
- PR mới của phiên này (`fix/80-alpine-hygiene`) đi qua B→C trong phiên;
  nếu gộp được → nhánh xoá sau merge (DELETE_MERGED), ledger append.
- Ledger `docs/branch-ledger.md` + `branch/branch-report.json` cập nhật qua
  PR ledger hoặc gộp chung theo §5 (tự nó qua CI).

---

# SBG MODE=apply — 2026-11-20 (Pha E, sau khi PR #83 gộp)

## Thực thi

| branch | decision | bằng chứng |
|---|---|---|
| fix/80-alpine-hygiene @ `689f171` | MERGE → DELETE_MERGED | PR #83: G1 ✓ (non-draft sau GraphQL flip), G2 ✓ (ci #206 + unicode-guard #72 + perf-guard #81 @ 689f171), G3 ✓ (SCR PASS ×3 clean-room + mutant M-CONTAM/M-FLOW caught), G4 ✓ (SIA PASS, label audit:pass), G5 ✓ (MERGEABLE, base = main base `2c7e839`), G6 ✓ (Closes #80 — type:bug, test mới AL9, D44, perf), G7 ✓ (chỉ tests/rootfs-alpine/docs/journal), G8 ✓ (perf-check PASS 1.75ms runner / 2.01ms dev) — squash `282234a` match-head; #80 auto-close |
| (không nhánh khác) | — | — |

- Xoá nhánh: remote `git push origin --delete` (lần 1 lỗi worktree — checkout
  main trước rồi xoá; lần 2 báo error nhưng `ls-remote` xác nhận đã mất) +
  nhánh local. Xác minh cuối: REST `/branches` → **chỉ `main`**.
- CI main sau merge: **xanh** (ci #207 + unicode-guard #73 @ `282234a`).
- §0.6 dừng: không áp dụng (CI xanh, 0 CVE mới ngoài watchlist, host unchanged).
- Ledger: 1 hàng append `fix/80-alpine-hygiene` (PR `chore(branches): ledger
  2026-11-20` gộp chung daily-log Phiên 9).
- branch-report.json cập nhật mode=apply sau merge.

