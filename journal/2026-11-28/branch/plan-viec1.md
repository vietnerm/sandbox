# SBG Plan — 2026-11-28 (Việc 1: GĐ3 mục 4 UI security e2e)

## MODE=plan (Pha A)

### Sync
- main @ `5718148` (PR #176 console merge). Pull ff-only OK.
- 0 PR mở (sau khi prune). ~8 issue mở (p2/p3 from-audit + #8 CVE).
- GĐ3 mở (owner 2026-11-26). Phase:3 label cho mọi PR GĐ3.

### Kế hoạch nhánh hôm nay (Việc 1)
- Tạo: `test/gd3-ui-security-md55-60` (GĐ3 mục 4 — e2e UI security + XSS).
- Daemon test riêng (DPID6, UI_PORT, UI_STATE, UI_DIR) — pattern MD35/MD36.
- Fake UI dir trong $WORK: index.html (inline script cho CSP hash) + _next/static/app.js + .env (dotfile) + symlink escape → outside.txt.
- MD55-MD60 (6 case) + XSS grep evidence.
- KEEP: 0. DELETE_MERGED sau Pha C.

### Ràng buộc (owner)
- Không sửa .github/, PROMPT.MD, idea/manifest.md, prompts/.
- Không bind UI 0.0.0.0, không cookie/session auth, không Node runtime.
- Label phase:3.
