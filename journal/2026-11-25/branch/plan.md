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

---

# SBG Plan — 2026-11-25 (Phiên 32)

## Trạng thái nhánh
| branch | ahead/behind | PR | CI | retest | audit | quyết định | lý do |
|---|---|---|---|---|---|---|---|
| `main` (a7a05f2) | — | — | xanh (ci + unicode-guard run a7a05f2) | — | — | — | base |
| `feat/162-overlayfs-rootless` | đã xoá remote | #163 MERGED | xanh | PASS | pass | DELETE_MERGED (đã xoá từ Phiên 31) | squash `17f5a97` nằm trong main |
| `chore/ledger-2026-11-25` | đã xoá remote | #164 MERGED | xanh | — (ledger) | — | DELETE_MERGED (đã xoá) | ledger Phiên 31 |

- Thu thập §2: `git fetch --prune` → API `branches` chỉ trả `main`. 0 nhánh cần
  xử lý; 0 PR mở. Halt-check §0.6: CI main xanh; không có issue
  `type:security` `priority:p0` mở (chỉ #8 p2 tracking); host baseline không đổi.

## Kế hoạch
- Pha B/C: rỗng → Pha D.
- Pha D: issue mới GĐ2 mục 4 — **port-forward pasta config-side**
  (khai báo trong RunSpec, daemon attach pasta; owner chỉ định 2026-11-25;
  còn lại của chuỗi: observability mục 7, systemd-real — phiên sau).
- Nhánh dự kiến: `feat/<issue>-pasta-portforward-config`.
- Hết phiên: SBG apply (gộp PR + DELETE_MERGED), ledger qua PR `chore(branches)`.
## Kết quả thực thi (MODE=apply, cuối Phiên 32)
- MERGE: PR #166 squash `d7cb8a3` (--match-head-commit 04f51db) — G1–G8 PASS,
  SCR PASS + SIA PASS cùng SHA (run 45590685583), label audit:pass.
- DELETE_MERGED: `feat/165-pasta-portforward-config` (đã xoá remote).
- DROP: rỗng. KEEP: rỗng.
- Xác minh cuối: API branches → chỉ `main`; #165 auto-close.
