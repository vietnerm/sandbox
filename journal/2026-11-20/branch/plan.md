# SBG Branch Plan — 2026-11-20 (Phiên 10, MODE=plan)

Đầu vào: MODE=plan, SCOPE=all, DATE=2026-11-20.

## Thu thập

- `git fetch --prune origin` — xong.
- Nhánh remote: **chỉ `main`** (`gh api .../branches`: main).
- PR mở: **0** (`gh pr list --state open`: rỗng).
- PR đã merge gần nhất: #83 (squash `282234a`), nhánh đã xoá (DELETE_MERGED, Phiên 9).
- CI `main`: **success** — run `ci` + `unicode-guard` @ `66a4f09` (2026-11-20T02:29Z).

## Bảng quyết định

| branch | ahead/behind | PR | CI | retest | audit | quyết định | lý do |
|---|---|---|---|---|---|---|---|
| (không có nhánh ≠ main) | — | — | — | — | — | — | remote chỉ còn `main` sau Phiên 9 |

## Điều kiện dừng (§0.6)

- CI `main` đỏ: **không** (success).
- Issue `type:security` `priority:p0` CVE critical chưa vá: **không** (không có issue p0; #8 là p2 track vendor backport).
- Host baseline báo thay đổi trong ngày: **không** (Phiên 9 ghi unchanged; sẽ kiểm lại ở Pha D Bước 7).

→ KHÔNG dừng. Chế độ plan: không gộp/xoá gì. Không có nhánh nào cần xử lý.

---

## Phiên 11 — SBG MODE=plan (Pha A, 2026-11-20)

- `git fetch --prune origin` — xong.
- Nhánh remote: **chỉ `main`**.
- PR mở: **0** (`gh pr list --state open`: rỗng) → Pha B/C không có việc.
- CI `main`: **success** @ `bf409b8` (ledger Phiên 10, run 2026-11-20T03:52Z).

## Bảng quyết định

| branch | ahead/behind | PR | quyết định | lý do |
|---|---|---|---|---|
| (không có nhánh ≠ main) | — | — | — | remote chỉ còn `main` sau Phiên 10 |

## Điều kiện dừng (§0.6)

- CI `main` đỏ: **không** (success).
- Issue `type:security` `priority:p0` mở: **không** (#8 là p2 track backport).
- Host baseline thay đổi: sẽ kiểm lại ở Pha D Bước 7.

→ KHÔNG dừng. Sang Pha D (việc mới). SBG MODE=apply chạy lại ở Pha E.
