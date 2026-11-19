# SBG Branch Plan — 2026-11-19 (Phiên 1)

MODE=plan (Pha A). APPLY sẽ chạy ở Pha C và Pha E.

## Thu thập (nguồn sự thật tại thời điểm plan)
- `git fetch --prune`: remote chỉ còn **`main`** (187bfd2) — không nhánh nào khác.
- PR mở: **0** (REST `GET /pulls?state=open` → `[]`).
- CI `main`: run ci #44273346039 @ 187bfd2 = **success**; unicode-guard = success.
- Issue mở: #44 (chore/area:ci, p2, from-audit — cve-check.py KeyError 'cvss'),
  #34 (security p0, đã triage Phiên 2 2026-11-18 — giữ mở theo §6, 0 CVE ảnh
  hưởng sandbox), #31/#30/#29 (p3), #28/#27/#26 (docs/ci p1–p2), #8/#5 (security
  p2), #4 (feature p2, GĐ2 — luật D27 không chọn).
- Host baseline: sẽ đo ở Bước 7 (chưa có chỉ báo bất thường nào từ journal
  2026-11-18).

## Kế hoạch cho từng nhánh ≠ main
- (không có nhánh nào khác `main` ở local lẫn remote)

| Nhánh | Ahead/Behind | Quyết định plan-mode |
|---|---|---|
| — | — | — |

- DELETE_MERGED: (rỗng)
- MERGE: (rỗng)
- KEEP: (rỗng)
- DROP: (rỗng)

## Kết luận
- Repo đã ở trạng thái mục tiêu §11: chỉ `main`. Không có pha B/C để xử lý ở đầu
  chu trình này.
- §0.6 dừng: không áp dụng (CI main xanh, #34 đã triage, chưa có dấu hiệu host
  baseline đổi).
- Pha D hôm nay: issue **#5** (type:security p2, area:escape-test — mục 2 GĐ1
  trong roadmap: "chroot-escape test sâu hơn"). Nhánh dự kiến
  `sec/5-deep-escape`; PR mới sẽ đi lại Pha B → C trong lần chạy này.
