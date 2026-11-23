# D60 — #144: B6 retroactive research for PR #142 + B3 note (Phiên 25, 2026-11-23)

## Ngữ cảnh

from-audit #144 (SIA PR #142): PR #142 (owner, `sec(cve)` CVE kernel evaluation) thiếu mục
research ≥2 nguồn trong `journal/2026-11-23/research.md` (B6 violation). B3: nhánh
`sec/cve-kernel-ranges` thiếu `<N>-<slug>` (PROMPT §1 B5).

## Quyết định

1. **B6 fix**: append mục research cho PR #142 vào `journal/2026-11-23/research.md` — 2 nguồn
   (NVD API v2.0 rate-limit/CPE, kernel.org CNA flow/lore) + context #142 (`cve-kernel.py`,
   CPE `linux_kernel`, LTS ranges 6.1/6.6/6.12, control query, CVE-2026-98432/53266).
2. **B3 note**: không rename nhánh owner; ghi nhận precedent — các nhánh sec/ sau dùng
   `sec/<N>-<slug>` (Phiên 17-23: sec/113, sec/121, sec/124, fix/117 — đúng quy ước).

## Phủ nhận / residual

- Research retroactive (PR #142 đã merge): research ghi sau-the-fact dựa trên #142 body +
  NVD/kernel.org docs. Không re-run #142; chỉ restore B6 compliance cho journal ngày.
- B3: nhánh `sec/cve-kernel-ranges` (owner) giữ tên; chỉ documentation.
- Docs-only PR (parity ledger PRs): không test mới (§1 B5 — docs/journal exempt, precedent
  Phiên 17-23 ledger PRs).

## Tests

- Docs-only (journal research.md + decision). CI: `make test` unaffected (journal only).

## Bài học

- B6 retroactive: SDDA có thể bổ sung research cho PR khác (owner) khi from-audit yêu cầu,
  dựa trên PR body + nguồn public. Restore compliance cho journal ngày.
- GĐ2 mục 4-7 env-gated (overlayfs/Landlock kernel ≥5.11, port-forward pasta, systemd-real):
  dev 5.10 không test được local. from-audit p3 còn lại hard-to-test. #144 là clean docs fix
  trong ràng buộc env.
