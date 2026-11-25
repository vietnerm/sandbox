# SBG Plan — 2026-11-25 (Phiên 31)

## Trạng thái nhánh
- Remote chỉ có `main` (b447a8c); 0 nhánh agent còn lại. 0 PR mở.
- DELETE_MERGED: rỗng (không có nhánh đã gộp chưa xoá).
- KEEP: rỗng.

## Kế hoạch
- Pha B/C: không có PR nào để gộp → đi thẳng Pha D.
- Pha D: issue mới GĐ2 mục 3 residual — **overlayfs rootless (env-gated)**
  (owner chỉ định 2026-11-25; phần còn lại của GĐ2: port-forward pasta mục 4,
  observability mục 7, systemd-real — làm theo lượt, 1 issue/phiên).
- Nhánh dự kiến: `feat/<issue>-overlayfs-rootless`.
- Hết phiên: SBG apply dọn nhánh, ledger qua PR `chore(branches)`.
