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

---

# SBG Branch Plan + APPLY — 2026-11-19 (Phiên 3, Orchestrator §11)

MODE=plan ở Pha A → MODE=apply ở Pha C/E (ghi hợp nhất tại đây).

## Thu thập cuối phiên
- Nhánh ≠ main: **feat/3-pasta-egress** @ `47ce5d5` (PR #59, merged squash
  `01899ee` qua API `PUT /pulls/59/merge` match-head-corrected).
- PR mở sau merge: **0**; issue #57 auto-closed bởi merge.
- CI main sau merge @ `01899ee`: **success** (ci + unicode-guard).

## Quyết định theo nhánh
| Nhánh | Ahead/Behind | PR | CI | retest | audit | Quyết định | Lý do |
|---|---|---|---|---|---|---|---|
| feat/3-pasta-egress | 5/0 | #59 | xanh ×5 vòng CI (d3e6e43 đỏ→sửa, 2571734, a80a3a7, 676332b, 47ce5d5 xanh) | SCR PASS ×5 run + mutant M0/M-pin/M-version caught (comment PR) | SIA PASS (label audit:pass @ 47ce5d5) | MERGE → DELETE_MERGED | G1–G8 PASS; không G7; squash `01899ee`; CI main xanh sau merge |

- DELETE_MERGED: feat/3-pasta-egress (đã xoá remote + local sau khi PR MERGED
  khớp head `47ce5d5a`).
- MERGE: (đã thực hiện — #59)
- KEEP: (rỗng) — DROP: (rỗng)
- §0.6 dừng: không áp dụng (CI main xanh; #52/#34 đã triage 0 CVE ảnh hưởng;
  host baseline unchanged — evidence pasta-egress.txt).

## Kết luận
- Repo về trạng thái mục tiêu: chỉ **`main`**.
- Bài học mới ghi skills.md: flip draft→ready bằng REST PATCH no-op với PAT
  fine-grained → dùng GraphQL `markPullRequestReadyForReview` (đối chiếu
  precedent #47→#48 của Phiên 1).
