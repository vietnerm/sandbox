# SBG Branch Plan — 2026-11-18 (RUN: orchestrator session 2)

> MODE=plan ghi lúc 04:55Z; MODE=apply 05:17–05:20Z cùng lần chạy (§11).
> main = 81b5dce2 (run #77 ci: success — không halt §0.6).

## Nhánh remote (plan)

| branch | ahead/behind | PR | CI @ HEAD | retest | audit | quyết định | lý do |
|---|---|---|---|---|---|---|---|
| ci/unicode-guard | 1 / 1 | #36 (open, ready) | scan+build-test×2+perf success @ 0023b97 | SCR @ 7685b3f sau B1 (63/63 ×3) | SIA CONCERNS @ 7685b3f | **KEEP** | G7: diff đụng `.github/workflows/**` + `PROMPT.MD` → không tự gộp; label `needs-human`; B1 sync đã làm (86030d3, fix tab 7685b3f); SCR/SIA comment trên PR |
| feat/minictd-control-plane | 2 / 1 | #35 **MERGED** | n/a | n/a | n/a | **DELETE_MERGED** | squash-match: PR state=closed+merged=true, headRefOid `8287246` == HEAD nhánh (SBG §2) |

## Nhánh local (plan — housekeeping)

| branch | trạng thái | quyết định |
|---|---|---|
| chore/branch-gate-prompt | đã gộp (squash a512824) | DELETE_MERGED (local) |
| chore/daily-log-session5 | đã gộp | DELETE_MERGED (local) |
| feat/3-netns | đã gộp (squash 18cb41e) | DELETE_MERGED (local) |

## Apply (MODE=apply, 05:17–05:20Z)

| branch | quyết định cuối | thực thi | kết quả |
|---|---|---|---|
| feat/minictd-control-plane | DELETE_MERGED | `git push origin --delete feat/minictd-control-plane` | xoá thành công 05:17Z (PR #35 squash-match; commit cuối 04:35:06Z — ngoài cửa sổ 30' §0.4) |
| ci/unicode-guard | KEEP (G7 needs-human) | không gộp, không xoá | PR #36 mở, labels `audit:concerns` + `needs-human`; việc còn lại: owner review |
| chore/branch-gate-prompt, chore/daily-log-session5, feat/3-netns (local) | DELETE_MERGED | `git branch -d` | xoá local thành công |

## Ghi chú

- PR #36 gặp sự cố trong B1: commit sync 86030d3 chuẩn hoá TAB Makefile → CI đỏ
  "missing separator"; tác giả PR fix 7685b3f (byte-identical với fix cục bộ của
  SCR-runner; fix cục bộ chưa push đã bỏ). Bài học vào `docs/skills.md`.
- File `prompts/BRANCH_GATE_PROMPT.md` đã có trên `main` (88af156) theo lệnh owner
  phiên trước — đã xác minh, không cần hành động.
