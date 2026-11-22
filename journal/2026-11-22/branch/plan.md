# SBG — Branch Gate plan 2026-11-22 (MODE=plan, Pha A)

Chu trình: Phiên 17 (SDDA, issue #113 / GAPS S3).

## Trạng thái đầu chu trình

- `git fetch --prune`: remote chỉ còn `origin/main` (đúng trạng thái cuối
  Phiên 16); local có nhánh chết `chore/ledger-2026-11-19-p3` (origin gone,
  nội dung đã gộp từ lâu qua PR #60) — đã checkout main, pull ff 34 commit
  tới `fc2e048`.
- PR mở: **0** (gh/API `pulls?state=open` rỗng) → Pha B/C không có việc;
  không cần SCR/SIA trên PR nào.
- Issue mở: 15 (triage Bước 2: không có issue mới trong 24h chưa gán label;
  #109 đã có đủ label từ Phiên 16).

## Kế hoạch DELETE_MERGED / KEEP

- `chore/ledger-2026-11-19-p3` (local-only, origin gone): **DELETE_MERGED**
  (đã gộp qua PR #60 `1573d93`; chỉ còn bản local) — xoá local trong Pha E.
- Không nhánh KEEP mới.

## Kế hoạch gộp trong ngày

- PR duy nhất hôm nay: `sec/113-s3-ro-bind-recursive` (issue #113) — đi
  Pha B→C ngay khi mở (draft → CI xanh → SCR → SIA → flip ready → squash).
- Ledger cuối ngày qua PR `chore(branches): ledger 2026-11-22` (Phiên 17).

---

# SBG — Branch Gate plan 2026-11-22 (MODE=plan, Pha A — Phiên 18)

Chu trình: Phiên 18 (SDDA, issue S4-GAPS — unix-socket listener, GĐ2 mục 1).

## Trạng thái đầu chu trình

- Pull main `69ecedb` (ff, ledger Phiên 17 đã vào main); working tree clean;
  local chỉ `main` (nhánh `sec/113-s3-ro-bind-recursive` đã xoá cuối Phiên 17).
- PR mở: **0** (API `pulls?state=open` rỗng) → Pha B/C đầu chu trình không có
  việc; PR mới của Phiên 18 đi Pha B→C ngay trong lần chạy này.
- Issue mở: 15 — không có issue mới trong 24h chưa label. 2 issue sẽ tạo
  trong phiên: (1) S4-GAPS unix-socket (type:security, theo GAPS thứ tự +
  kế hoạch "ngày mai" của Phiên 17); (2) from-audit truncate>255 của PR #114
  (low, SIA Phiên 17 cam kết mở — kiểm tra lại thấy CHƯA tồn tại, tạo bù).

## Kế hoạch DELETE_MERGED / KEEP

- Không nhánh local lẻ; nhánh làm việc `sec/<num>-s4-unix-socket` sẽ tạo trong
  Pha D và dự kiến DELETE_MERGED sau squash. Không nhánh KEEP mới.

---

# SBG — Branch Gate plan 2026-11-22 (MODE=plan, Pha A — Phiên 19)

Chu trình: Phiên 19 (SDDA, issue #121 / GAPS S5 — spec rlimits cho exec'd
processes, GĐ2 mục 1).

## Trạng thái đầu chu trình

- Pull main `336373a` (ff, ledger Phiên 18 đã vào main qua PR #120 squash
  `e65d0e2`); working tree clean; local chỉ `main` (nhánh
  `sec/116-s4-unix-socket` đã xoá cuối Phiên 18).
- PR mở: **0** (REST API `pulls?state=open` rỗng) → Pha B/C đầu chu trình
  không có việc; PR mới của Phiên 19 đi Pha B→C ngay trong lần chạy này.
- Issue mở: 17 — không có `type:security priority:p0/p1` nào chen ngang
  (#8 p2 track vendor backport CVE-2026-31431, không chặn). Tạo #121 (S5,
  type:security p1) theo GAPS thứ tự (S1→S6, S5 tiếp theo sau S4) + kế hoạch
  "ngày mai" Phiên 18.

## Kế hoạch DELETE_MERGED / KEEP

- Không nhánh local lẻ; nhánh làm việc `sec/121-s5-rlimits-exec` sẽ
  DELETE_MERGED sau squash. Không nhánh KEEP mới.

## Kế hoạch gộp trong ngày

- PR duy nhất hôm nay: `sec/121-s5-rlimits-exec` (issue #121) — đi Pha B→C
  ngay khi mở (draft → CI xanh → SCR → SIA → flip ready → squash
  `--match-head-commit`). Ledger cuối ngày qua PR `chore(branches): ledger
  2026-11-22 Phiên 19` (hoặc gộp vào PR chính theo precedent Phiên 17/18).
