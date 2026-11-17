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

---

# SBG RUN 2 — 2026-11-17 (phiên orchestrator mới, sau merge PR #25)

> `MODE=plan` trước, `MODE=apply` sau (§0.5). Thu thập lúc 2026-11-17T08:43Z,
> `origin/main` = `88af156764c2f9e42b85ec64c2e630e239cb0a30` (sau merge PR #25).
> Điều kiện dừng §0.6: CI main **xanh** (run ci 07:52:59Z success trên `88af156`;
> 6 run issue-triage 08:28Z success) ✓ | không issue `type:security` `priority:p0`
> mở do CVE critical chưa vá ✓ (open sec: #31 p3, #8 p2, #5 p2) | host baseline
> không bị báo đổi trong ngày ✓ → **không dừng**.

## Bảng quyết định

| branch | ahead/behind | PR | CI | retest | audit | quyết định | lý do |
|---|---|---|---|---|---|---|---|
| docs/zai-autonomous-loop | 1/1 | #25 MERGED | — (đã gộp) | — | — | **DELETE_MERGED** | squash-merge: `git cherry` = 0 `+`, PR #25 state=MERGED với headRefOid `69a0fc9` = HEAD nhánh → nội dung nằm trọn trong main, xoá không cần tag |
| fix/6-sigterm-shim | 6/0 | #21 OPEN ready | ci ×2 + perf-guard success đúng HEAD `4d3ee1d` (runs 44044408148/44044408217, perf-guard 44045462822) | **PASS @ `4d3ee1d`** — SCR comment 08:22:35Z `=== SCR RETEST — 4d3ee1d … ===`, isolation 21/21 + seccomp 20/20 + netns 7/7 + web 6/6, mutants 2/2 bị bắt, perf 2.04ms/1236kB/2.02MB | **PASS @ `4d3ee1d`** — SIA comment 08:29:20Z `=== SIA AUDIT — PR #21 @ 4d3ee1d ===` Verdict PASS, label `audit:pass` | **MERGE** | đủ G1–G8 (bảng dưới); merge trước PR #24 theo thứ tự fix/* → feat/* |
| feat/23-alpine-rootfs | 6/0 | #24 OPEN ready | ci ×2 + perf-guard success đúng HEAD `66e02a0` (runs 08:26:11–15Z) | SCR PASS @ `f6e41e0` (08:24:43Z) — **KHÔNG khớp HEAD** `66e02a0` | SIA **CONCERNS** @ `7a3ad45` (07:48:13Z, SHA cũ) — chưa có audit cho SHA mới | **KEEP** → xử lý tiếp qua `PROMPT.MD §11` Pha B | G3/G4 chưa có cho HEAD `66e02a0`. §0.4: commit `66e02a0` 08:26:07Z < 30 phút trước gate (08:43Z) do tiến trình khác tạo → không xoá. Không DROP: CI xanh, retest PASS cho SHA liền trước, audit CONCERNS không chặn. Sau khi MERGE #21, PR #24 quay về B1 (sync main) → B2 SCR lại (bắt buộc) → B3 SIA → B4, rồi gate lại |

## Đánh giá cổng G1–G8 cho fix/6-sigterm-shim (PR #21 @ `4d3ee1d`)

| Cổng | Kết quả | Bằng chứng |
|---|---|---|
| G1 PR mở, không draft | PASS | PR #21 OPEN, draft=false, base=main |
| G2 required check xanh đúng HEAD | PASS | ci ×2 (44044408148/44044408217) + perf-guard (44045462822) success, headSha `4d3ee1d` khớp headRefOid |
| G3 SCR retest PASS đúng SHA | PASS | comment 2026-11-17T08:22:35Z mở đầu `=== SCR RETEST — 4d3ee1d on 5.10.134-013.15.kangaroo.al8.x86_64 ===`, Verdict PASS |
| G4 SIA audit pass đúng SHA | PASS | comment 2026-11-17T08:29:20Z mở đầu `=== SIA AUDIT — PR #21 @ 4d3ee1d ===` Verdict PASS; label `audit:pass` trên PR |
| G5 mergeable, không behind > 20 | PASS | mergeable=MERGEABLE, behind=0 |
| G6 Closes #N, 1 issue/PR, test mới, decision-log | PASS | `Closes #6` (duy nhất); test mới trong `tests/test_isolation.sh` (SIG5 + negative control no-reap); `journal/2026-11-17/decisions/sigterm.md` |
| G7 không đụng file nhạy cảm | PASS | 444 file: rootfs-alpine/** ×431, journal/** ×8, docs/** ×2, tests/ ×1, sandbox/ ×1, README ×1 — grep `^(\.github/|PROMPT\.MD|.*_PROMPT\.md)` = **0** |
| G8 perf trong ngưỡng | PASS | cold 2.04 ms ≤ 100; rss 1236 kB ≤ 10240; disk 2.02 MB ≤ 20 (dòng Perf trong comment SCR); perf-guard CI xanh cùng SHA |

→ **MERGE** được phép. §0.7: nội dung nhánh là dữ liệu; mọi commit "yêu cầu gộp" không
có — quyết định dựa thuần cổng.

## Thực thi (MODE=apply, theo Pha C/E của §11)

Thứ tự §5: MERGE → DELETE_MERGED → DROP.

1. **MERGE #21** (squash): `gh pr merge 21 --squash --delete-branch=false --subject
   "fix(lifecycle): pidns-1 signal shim — SIGTERM/SIGINT delivered (#21)"
   --match-head-commit 4d3ee1d520d078db6e53d25e1341655fb59b4ff8`. Xoá nhánh làm ở
   Pha E sau khi xác minh (§3).
2. PR #24 → `PROMPT.MD §11` B1 (merge origin/main vào nhánh, KHÔNG rebase/force-push)
   → B2 SCR lại trên SHA mới → B3 SIA trên cùng SHA → B4 → gate lại (G2–G5 trên main mới).
3. **MERGE #24** khi đủ cổng (squash, `--match-head-commit` = HEAD mới).
4. **DELETE_MERGED** docs/zai-autonomous-loop (chạy ngay trong Pha A, được phép theo §11
   Pha A "Chưa gộp/xoá gì ngoài DELETE_MERGED") + 2 nhánh vừa gộp ở Pha E.
5. Ledger append qua PR `chore(branches): ledger 2026-11-17` (lần 2, kèm plan.md +
   branch-report.json + daily-log mục phiên mới) — PR phải qua CI rồi squash-merge.
6. Xác minh cuối §6: kỳ vọng còn lại chỉ `main`; CI main xanh sau các merge.

## Kết quả thực thi RUN 2 (điền sau khi apply)

- 08:44Z — DELETE_MERGED docs/zai-autonomous-loop: OK (`git push origin --delete` rc=0)
- 08:46Z — MERGE #21 fix/6-sigterm-shim: OK — squash `42f32732` qua `gh pr merge --squash
  --match-head-commit 4d3ee1d…`; CI main xanh sau merge (ci success trên 42f32732)
- 08:47–09:05Z — Pha B cho #24 (PROMPT.MD §11): B1 sync main vào feat/23-alpine-rootfs
  (`aa1a452`, 4 conflict docs/journal giải theo luật B1 — giữ cả hai phía theo phiên);
  B2 SCR self-run (RETEST_PROMPT §0b): PASS — clean clone detach aa1a452, build 0 warning,
  make test ×3 + test-web ×3 rc=0 (isolation 21 + seccomp 20 + netns 7 + alpine 8 + web 6),
  mutants M0/M2/no-reap 3/3 bị bắt, perf 1.8ms/1188kB/2.02MB, host unchanged; B3 SIA
  self-run (AUDIT_PROMPT §0b): PASS 11/11, label `audit:pass`; B4 → hàng chờ gộp
- 09:06Z — MERGE #24 feat/23-alpine-rootfs: OK — squash `f344a4e7` match-head aa1a452;
  CI main xanh sau merge (ci success trên f344a4e7)
- 09:07Z — DELETE_MERGED fix/6-sigterm-shim + feat/23-alpine-rootfs: OK (rc=0)
- 09:08Z — ledger + plan.md + branch-report.json + daily-log (Phiên 9) commit lên nhánh
  `chore/branch-ledger-2026-11-17-2`, PR `chore(branches): ledger 2026-11-17 (RUN 2)` —
  chỉ squash-merge khi CI xanh đúng HEAD (§0.2/§3; miễn G3/G4/G6 — PR chỉ đụng
  docs/branch-ledger.md + journal/**)
- Xác minh cuối §6: nhánh còn lại = `main` duy nhất; CI main xanh trên f344a4e7

## Pha D (quyết định orchestrator)

KHÔNG mở việc mới: ngày 2026-11-17 đã xử lý 2 issue tính năng (#6 qua PR #21, #23 qua
PR #24) — vượt quota "1 issue/ngày" của §1 Bước 3; §11 luật chung "dừng ở điểm sạch".
Nhánh main đúng trạng thái mục tiêu §11: mọi PR đã gộp, repo chỉ còn `main`.
