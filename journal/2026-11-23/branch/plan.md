# SBG — Branch Gate plan 2026-11-23 (MODE=plan, Pha A — Phiên 20)

Chu trình: Phiên 20 (SDDA, issue #124 / GAPS S6 — webport collision detection, GĐ2 mục 1).

## Trạng thái đầu chu trình

- Pull main `cdb0e56` (ff, ledger Phiên 19 qua PR #123 squash `cdb0e56`); working
  tree clean; local chỉ `main` (nhánh `sec/121-s5-rlimits-exec` đã xoá cuối Phiên 19).
- PR mở: **0** (REST API `pulls?state=open` rỗng) → Pha B/C đầu chu trình không có
  việc; PR mới của Phiên 20 đi Pha B→C ngay trong lần chạy này.
- Issue mở: 17 — không có `type:security priority:p0/p1` nào chen ngang (#8 p2 track
  vendor backport). Tạo #124 (S6, type:security p1) theo GAPS thứ tự (S1→S6, S6 cuối
  cùng sau S5) + kế hoạch "ngày mai" Phiên 19.

## Kế hoạch DELETE_MERGED / KEEP

- Không nhánh local lẻ; nhánh làm việc `sec/124-s6-webport-alloc` sẽ DELETE_MERGED
  sau squash. Không nhánh KEEP mới.

## Kế hoạch gộp trong ngày

- PR duy nhất hôm nay: `sec/124-s6-webport-alloc` (issue #124) — đi Pha B→C ngay khi
  mở (draft → CI xanh → SCR → SIA → flip ready → squash `--match-head-commit`).
  Ledger cuối ngày qua PR `chore(branches): ledger 2026-11-23 Phiên 20`.
