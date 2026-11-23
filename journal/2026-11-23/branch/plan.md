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

---

# SBG — Branch Gate plan 2026-11-23 (MODE=plan, Pha A — Phiên 21)

Chu trình: Phiên 21 (SDDA, issue #127 / GĐ2 mục 2 — template).

## Trạng thái đầu chu trình

- Pull main `2d18aeb` (ff, ledger Phiên 20 qua PR #126 squash `2d18aeb`); working tree
  clean; local chỉ `main` (nhánh `sec/124-s6-webport-alloc` đã xoá cuối Phiên 20).
- PR mở: **0** (REST API rỗng) → Pha B/C đầu chu trình không có việc; PR mới đi Pha B→C ngay.
- Issue mở: 17 — không có `type:security priority:p0/p1` chen ngang. GĐ2 mục 1 S-series
  done (Phiên 20); còn systemd-real (env-gated) + #109 (chờ maintainer) — cả hai blocked.
  Move tới mục 2. Tạo #127 (template, type:feature p2) theo §8 "mục thấp nhất chưa xong".

## Kế hoạch DELETE_MERGED / KEEP

- Nhánh `feat/127-template` DELETE_MERGED sau squash. Không KEEP mới.

## Kế hoạch gộp trong ngày

- PR duy nhất: `feat/127-template` (issue #127) — Pha B→C (draft → CI → SCR → SIA →
  flip ready → squash `--match-head-commit`). Ledger qua PR `chore(branches): ledger
  2026-11-23 Phiên 21`.

---

# SBG — Branch Gate plan 2026-11-23 (MODE=plan, Pha A — Phiên 22)

Chu trình: Phiên 22 (SDDA, issue #130 / GĐ2 mục 3 sub — snapshot/backup/restore).

## Trạng thái đầu chu trình

- Pull main `2634286` (ff, ledger Phiên 21 qua PR #129 squash `2634286`); local chỉ `main`.
- PR mở: **0** (REST API rỗng) → Pha B/C đầu chu trình rỗng → vào thẳng Pha D.
- Issue mở: 17 — không `type:security priority:p0/p1`. GĐ2 mục 1 (S-series done + residuals
  blocked), mục 2 (done Phiên 21). Move tới mục 3. Tạo #130 (snapshot, type:feature p2) —
  mục 3 sub (snapshot/backup/restore); overlayfs (phần a) env-gated kernel 5.10, deferred.

## Kế hoạch DELETE_MERGED / KEEP

- `feat/130-snapshot` DELETE_MERGED sau squash. Không KEEP mới.

## Kế hoạch gộp trong ngày

- PR `feat/130-snapshot` (issue #130) — Pha B→C (draft → CI → SCR → SIA → flip ready →
  squash `--match-head-commit`). Ledger qua PR `chore(branches): ledger 2026-11-23 Phiên 22`.
