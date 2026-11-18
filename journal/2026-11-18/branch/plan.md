# SBG Branch Plan — 2026-11-18 (RUN: orchestrator session 2)

> MODE=plan lúc ghi; cùng lần chạy này chuyển MODE=apply sau Pha B (§11).
> Thu thập lúc 2026-11-18T04:55Z, main = 81b5dce2 (run #77 ci: success — không halt §0.6).

## Nhánh remote

| branch | ahead/behind | PR | CI @ HEAD | retest | audit | quyết định | lý do |
|---|---|---|---|---|---|---|---|
| ci/unicode-guard | 1 / 1 | #36 (open, ready) | build-test+perf+scan success @ 0023b97 | chờ B2 (sau B1 sync) | chờ B3 | **KEEP → B1** | mergeable=dirty (xung đột với main) → §11 B1: merge main vào nhánh, giải xung đột, push, chờ CI → B2 SCR → B3 SIA. PR đụng `.github/workflows/**` + `PROMPT.MD` → **G7: không tự gộp**, gắn `needs-human` chờ owner review |
| feat/minictd-control-plane | 2 / 1 | #35 **MERGED** | n/a | n/a | n/a | **DELETE_MERGED** | PR #35 state=closed+merged=true, headRefOid `8287246` == HEAD nhánh (SBG §2 squash-match) → nội dung nằm trọn trong main (81b5dce). Xoá không cần tag |

## Nhánh local (không có trên remote — housekeeping)

| branch | trạng thái | quyết định |
|---|---|---|
| chore/branch-gate-prompt | đã gộp (ledger 2026-11-17, squash a512824) | DELETE_MERGED (xóa local) |
| chore/daily-log-session5 | đã gộp (ledger 2026-11-17) | DELETE_MERGED (xóa local) |
| feat/3-netns | đã gộp (ledger 2026-11-17, squash 18cb41e) | DELETE_MERGED (xóa local) |

## Thứ tự thực thi (MODE=apply, sau Pha B)

1. Pha B trên PR #36: B1 sync main (giải xung đột PROMPT.MD/Makefile/README theo luật §11 B1) → push → chờ CI → B2 SCR trên SHA mới → B3 SIA audit → B4 verdict.
2. Pha C: PR #36 G7 → **không gộp tự động**; label `needs-human`.
3. DELETE_MERGED: `feat/minictd-control-plane` (remote), 3 nhánh local đã gộp.
4. Ledger: `docs/branch-ledger.md` append qua PR `chore(branches): ledger 2026-11-18` (gộp chung PR daily-log theo §5).

## Ghi chú

- `main` CI xanh (run #77 success) → không dừng §0.6. Issue #34 (CVE p0, auto) đã được
  đối chiếu với watchlist — mọi CVE trong báo cáo đều có dòng "không với tới"/host-side
  đã ghi trong `docs/cve-watchlist.md`; xử lý trong Pha D (triage + CVE check bù).
- File `prompts/BRANCH_GATE_PROMPT.md` đã có trên `main` (88af156) theo lệnh owner
  phiên trước — không cần hành động thêm.
