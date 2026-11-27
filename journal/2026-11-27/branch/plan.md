# SBG plan — 2026-11-27 (Phiên 42, MODE=plan)

State đầu phiên: main @ `6fda589` (Phiên 41 ledger), pull --ff-only OK, working
tree sạch trừ 1 file untracked `journal/2026-11-25/branch/branch-report.json`
(artifact SBG Phiên 32 chưa commit — sẽ commit trong ledger PR cuối phiên, §10
"Mỗi ngày một thư mục"; không sửa nội dung).

## PR đang mở
- Không có. Remote chỉ còn `main` (kết quả cuối Phiên 41 giữ nguyên sau pull).

## Kế hoạch gộp/bỏ nhánh hôm nay
- Không có nhánh local/remote nào ngoài `main` → không có DELETE_MERGED/KEEP
  đầu phiên.
- Pha D (việc mới): Track M **M3 — Registry template tĩnh** (mục thấp nhất chưa
  xong; M1 xong #197, M2 trọn vẹn xong #199+#203). Nhánh dự kiến
  `feat/215-template-registry`, 1 issue → 1 PR, đi Pha B (SCR → SIA) → Pha C
  (squash) trong phiên này.
- G7 flag: KHÔNG tự đụng `.github/workflows/**` trong PR M3; việc "console-test
  vào workflow ci" vẫn giữ dạng residual chờ owner (đã ghi ledger Phiên 40/41).
- Cuối phiên: SBG MODE=apply — ledger `chore(branches): ledger 2026-11-27
  Phien 42`, xoá nhánh đã gộp, commit file untracked bên trên.

## Luật nhắc lại
- Không force-push, không push main, squash qua REST khi GraphQL lỗi
  (PUT /pulls/<n>/merge, merge_method=squash, sha = head SHA).
