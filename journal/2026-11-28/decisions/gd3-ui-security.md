# Decision D71 — GĐ3 mục 4: UI security e2e (MD55-MD60) + XSS check

- **Phiên**: 35 (2026-11-28, Việc 1)
- **Issue**: #177
- **Slug**: `gd3-ui-security`

## Vấn đề
GĐ3 mục 4 (roadmap dòng 622-626): Go unit `ui_test.go` đã có (traversal/symlink/dotfile/405/auth/CSP). Còn thiếu e2e MD trên daemon thật + kiểm XSS cho log/exec output.

## Giải pháp
**6 e2e cases (MD55-MD60) trong test_minictd.sh** — daemon riêng (DPID6) với `--ui-dir` trỏ fake UI dir:
- **MD55**: GET / → 200 HTML + CSP `script-src 'self' 'sha256-…'` (no `unsafe-inline`) + X-Frame-Options: DENY.
- **MD56**: traversal (`/../`, `%2e%2e`, `..%2f`) + symlink escape + dotfile (`/.env`) → 404, no content leak.
- **MD57**: POST / → 405.
- **MD58**: có --ui-dir: `/v1/sandboxes`, `/v1/templates`, `/metrics`, `/healthz` không token → 401.
- **MD59**: không --ui-dir (daemon chính DPID): GET / không token → 401 (legacy).
- **MD60**: --ui-dir dir không có index.html → daemon exit ≠ 0.

**XSS check**: grep `console/src/` cho `dangerouslySetInnerHTML` + `innerHTML` → 0 match (xterm `term.write()` safe — renders to canvas, không innerHTML). Evidence vào journal.

**Fake UI dir**: `$WORK/ui/` với index.html (inline `<script>console.log(1)</script>` cho CSP hash) + `_next/static/app.js` + `.env` (SECRET=leaked) + `escape.txt` (symlink → `$WORK/outside.txt`).

## Lý do
- E2e trên daemon thật verify: header thực tế gửi đi (CSP/X-Frame-Options), routing withUI vs auth middleware, daemon exit code cho --ui-dir bad.
- XSS grep verify console source không có unsafe HTML injection — xterm write() là safe (escape sequence parser → DOM text, không innerHTML).
- Không cần build Next.js — fake UI dir đủ cho e2e (HTML + static asset).

## Không làm
- Không sửa .github/, PROMPT.MD, idea/manifest.md, prompts/.
- Không thêm CI step `make console` (cần owner duyệt riêng).
- Không bind UI 0.0.0.0, không cookie/session.
