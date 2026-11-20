# SBG Branch Plan — 2026-11-20 (MODE=plan, Phiên 9)

Input: MODE=plan, SCOPE=all, DATE=2026-11-20.

## Thu thập

- `git fetch --prune origin '+refs/heads/*:refs/remotes/origin/*' --tags`: OK.
- Nhánh remote: **chỉ `main`** (`2c7e839` — ledger Phiên 8 #82).
- PR mở: **0** (REST `GET /pulls?state=open` — `gh` CLI không có trên máy này,
  dùng REST API qua curl, token từ PAT owner — pattern Phiên 7).
- CI main: xanh — run #204 (ci) + #71 (unicode-guard) @ `2c7e839`.
- Nhánh local ≠ main: 0.
- Tags archive: không có mới (không DROP nào hôm nay).

## Kế hoạch nhánh

| branch | ahead/behind | PR | CI | retest | audit | quyết định | lý do |
|---|---|---|---|---|---|---|---|
| (không có nhánh nào ngoài main) | — | — | — | — | — | — | trạng thái mục tiêu §11 đã đạt từ cuối Phiên 8 |

- §0.6 dừng (halt): **không áp dụng** — CI main xanh; không có issue
  `type:security` `priority:p0` chưa triage (#8 p2 đã có watchlist track, 0
  CVE mới ngoài watchlist ở lần check gần nhất 2026-11-19; re-check Bước 6
  phiên này); host baseline sẽ đo lại ở Bước 7.

## Hành động (khi MODE=apply cuối ngày)

- Pha B/C đầu chu trình: rỗng (0 PR mở).
- PR mới của phiên này (`fix/80-alpine-hygiene`) đi qua B→C trong phiên;
  nếu gộp được → nhánh xoá sau merge (DELETE_MERGED), ledger append.
- Ledger `docs/branch-ledger.md` + `branch/branch-report.json` cập nhật qua
  PR ledger hoặc gộp chung theo §5 (tự nó qua CI).
