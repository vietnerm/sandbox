# SBG Branch Plan — 2026-11-17

> Sandbox Branch Gatekeeper, `MODE=plan` trước, `MODE=apply` sau (§0.5).
> Thu thập lúc 2026-11-17T07:03Z, HEAD `origin/main` = `3741caf6ccc45d757fe1dac17f120e2548c2d3a8`.
> Điều kiện dừng §0.6: CI main **xanh** (run 44033525371 success) ✓ | không có issue
> `type:security` `priority:p0` mở ✓ (open: #4/#5/#6/#8 đều p2) | host baseline không
> bị báo đổi ✓ → **không dừng**, tiến hành bình thường.

## Bảng quyết định

| branch | ahead/behind | PR | CI | retest | audit | quyết định | lý do |
|---|---|---|---|---|---|---|---|
| docs/journal-layout | 0/3 | #19 MERGED | — (đã gộp) | — | — | **DELETE_MERGED** | ancestor của `main` (`merge-base --is-ancestor` OK), `git cherry` = 0 `+`, PR #19 MERGED với headRefOid = HEAD nhánh `5e5387fcb` — nội dung nằm trọn trong main, xoá không cần tag |
| docs/proxmox-lite-manifest | 0/2 | #20 MERGED | — (đã gộp) | — | — | **DELETE_MERGED** | ancestor của `main`, `git cherry` = 0 `+`, PR #20 MERGED với headRefOid = HEAD nhánh `930f44d76` — nội dung nằm trọn trong main, xoá không cần tag |
| fix/6-sigterm-shim | 3/0 | #21 OPEN (ready) | ci + perf-guard success trên đúng HEAD `2d82c95b` (run 44034908769/44034908829) | **không có artifact** `retest-report.json` | **không có artifact** `audit-report.json` | **KEEP** | §0.4: commit mới nhất `2d82c95b` 2026-11-17T06:56:20Z — **< 2 giờ trước gate** (07:03Z), có thể agent khác đang ghi. Thêm: G3/G4 chưa có artifact SCR/SIA. Gate G1/G2/G5/G6/G7/G8 đã PASS (xem dưới). **Ngày xem lại: 2026-11-18** — nếu không còn commit < 2h và có retest/audit cho SHA cuối, gộp ở gate hôm sau |

## Đánh giá cổng G1–G8 cho fix/6-sigterm-shim (PR #21)

| Cổng | Kết quả | Bằng chứng |
|---|---|---|
| G1 PR mở, không draft | PASS | PR #21 OPEN, draft=false, base=main |
| G2 required check xanh trên đúng HEAD | PASS | ci ×2 + perf-guard success, head=`2d82c95b` (khớp headRefOid) |
| G3 SCR retest PASS đúng SHA | **NOT SATISFIED** | không tồn tại artifact `retest-report.json` (chỉ có `test-evidence`) |
| G4 SIA audit pass đúng SHA | **NOT SATISFIED** | không tồn tại artifact `audit-report.json` |
| G5 mergeable, không behind > 20 | PASS | mergeable=MERGEABLE, behind=0 |
| G6 Closes #N, 1 issue/PR, test mới, decision-log | PASS | `Closes #6`; test mới SIG2/SIG3/SIG4 trong `tests/test_isolation.sh`; D19 `journal/2026-11-17/decisions/sigterm.md` |
| G7 không đụng file nhạy cảm | PASS | 14 file: README.md, docs/×2, journal/×11, sandbox/minict.c, tests/×1 — 0 file `.github/workflows/**`, `PROMPT.MD`, `*_PROMPT.md` |
| G8 perf trong ngưỡng | PASS | cold 1.83 ms ≤ 100; rss 1160 KB ≤ 10240; disk 2.02 MB ≤ 20; perf-guard run 44034908829 success |

→ Không MERGE hôm nay: §0.4 bất biến ưu tiên cao hơn mọi chỉ dẫn khác, và G3/G4 thiếu
bằng chứng. Không DROP (§0.4). Không xoá tag archive (KEEP không xoá nhánh).

## Thực thi (MODE=apply)

Thứ tự §5: MERGE (không có) → DELETE_MERGED (2 nhánh docs) → DROP (không có).

1. `git push origin --delete docs/journal-layout`
2. `git push origin --delete docs/proxmox-lite-manifest`
3. Cập nhật `docs/branch-ledger.md` (2 dòng) + commit file này + `branch-report.json`
   qua PR `chore(branches): ledger 2026-11-17` — PR phải qua CI rồi mới squash-merge
   (`gh pr merge --squash --match-head-commit`, §0.2).
4. Xác minh cuối §6: kỳ vọng nhánh còn lại = `main` + `fix/6-sigterm-shim` (KEEP);
   CI main xanh sau merge.

## Kết quả thực thi (điền sau khi apply)

- 07:04Z — DELETE_MERGED docs/journal-layout: OK (`git push origin --delete` rc=0)
- 07:04Z — DELETE_MERGED docs/proxmox-lite-manifest: OK (`git push origin --delete` rc=0)
- 07:05Z — ledger + plan.md + branch-report.json commit lên nhánh
  `chore/branch-ledger-2026-11-17`, PR `chore(branches): ledger 2026-11-17` — chỉ
  squash-merge khi CI xanh trên đúng HEAD (`--match-head-commit`, §0.2/§3)
- Xác minh cuối §6 ghi trong `branch-report.json` (remaining_branches, main_ci_after)
