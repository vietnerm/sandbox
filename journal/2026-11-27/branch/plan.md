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

---

# SBG plan — 2026-11-27 (Phiên 45, MODE=plan)

State đầu phiên: main @ `6501d15` (PR #230 WS-PTY #218/D84, merged 08:54Z),
pull --ff-only OK, working tree sạch. CI main 4/4 xanh @ `6501d15`
(build-test, console, scan, kvm). KHÔNG dừng: không issue security p0 mở,
host baseline không có báo cáo thay đổi hôm nay.

## Nhánh ≠ main trên remote (5 → sau xử lý)

| branch | ahead/behind | head | PR | quyết định | lý do |
|---|---|---|---|---|---|
| docs/roadmap-review-2026-11-27 | 2/7 | `683a1e3` | #223 MERGED @03:31Z, headRefOid khớp HEAD | DELETE_MERGED | squash `e6b67d6` đã trong main |
| docs/gd4-ci-kvm-platform | 1/6 | `5756f13` | #226 MERGED @04:05Z, headRefOid khớp HEAD | DELETE_MERGED | squash đã trong main; job kvm xanh @6501d15 |
| probe/kvm-runner | 1/6 | `49b7469` | #225 CLOSED (không merge, "[KHÔNG MERGE] probe") | DROP (thực thi ở Pha E) | PR đóng không gộp; findings đã ghi roadmap 1b.f + journal; DROP theo §0.3: archive tag trước khi xoá |
| fix/console-utf8-files-stage1 | 1/10 | `98230bf` | #217 MERGED (owner) | KEEP | nhánh owner — precedent ledger Phiên 43/44 ("agent KHÔNG xoá tài sản owner"); review lại 2026-11-28 |

Ghi chú: Phiên 44 (07:29Z) báo "remote chỉ còn main + nhánh owner" nhưng 3 nhánh
agent (commit 03:25–03:50Z) vẫn còn trên remote — phiên gộp #223/#226 có thể chạy
song song và chưa kịp dọn. Commit cuối trên các nhánh này 03:50Z, > 5 giờ → §0.4
không chặn xoá.

## PR đang mở
- Không có (0 open). Pha B/C rỗng.

## Kế hoạch Pha D (việc mới)
- Theo PROMPT §8: security p0/p1 — KHÔNG còn (#220 merged). Chuyển GĐ4 mục 1b
  theo thứ tự a→g: **1b.a Jail không cần root** (chạy `firecracker` binary bên
  trong instance minict làm jail: userns+mountns+seccomp+pivot_root, bind
  /dev/kvm + socket API; đo binary khởi động trong jail, `--version`, socket
  API `GET /`; không dùng jailer upstream vì buộc root). Mở issue mới trước
  khi develop (PR cần `Closes #N`).
- Nhánh dự kiến: `feat/gd4-jail-rootless`.

## Luật nhắc lại
- Không force-push, không push main; squash qua REST
  (PUT /pulls/<n>/merge, merge_method=squash, sha=head SHA).
