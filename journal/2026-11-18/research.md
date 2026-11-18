# Research 2026-11-18 - Unicode ẩn / bidi (Trojan Source) trong CI

## Phạm vi
Kiểm chứng các tool trong bản "chiến lược CI/CD 3 lớp" owner dán vào (do AI khác
viết) trước khi dùng cho PR #36. Kết luận dùng ở `decisions/unicode-guard.md` (D28-D29).

## Mối đe doạ
- **Trojan Source, CVE-2021-42574** (Boucher & Anderson, 2021, trojansource.codes):
  ký tự điều khiển bidi (RLO U+202E, LRI/RLI/FSI/PDI U+2066-2069...) làm thứ tự
  hiển thị khác thứ tự compiler đọc. CVE-2021-42694: homoglyph trong định danh.
- **ASCII smuggling** qua khối tag U+E0000-E007F và variation selector
  (U+FE00-FE0F, U+E0100-E01EF): payload vô hình trong văn bản, nhắm cả LLM/agent đọc repo.
- Unicode TR#9 (Bidirectional Algorithm) và TR#36/39 (Security Considerations /
  Mechanisms): danh mục control và confusable dùng để chọn tập ký tự error/warn.

## Kiểm chứng tool được nêu (2026-11-18)

| Tool | Nêu trong bản gốc | Kiểm được | Dùng? |
|---|---|---|---|
| GlyphGuard (`NicPWNs/GlyphGuard`) | GitHub Action "chính thức" | repo có thật, tag v0.2.0, v0.3.0 | Không: dự án nhỏ 1 người, v0.x |
| `heckler` | Python CLI `--ci` | PyPI 2.0.0, repo `kholcomb/heckler` | Không: bản gốc `pip install` không ghim |
| `anti-trojan-source` | "từ ortoo" | npm 1.13.1 nhưng của **lirantal**, không phải ortoo | Không: cần Node, nguồn bị nêu sai |
| `out-of-character` | CLI dọn ký tự | npm 2.3.0 (spencermountain); **không** có trên PyPI | Không: cần Node |
| `eslint-plugin-anti-trojan-source` | IDE | chỉ JS/TS; repo này C/Go/Python/shell | Không áp dụng |
| `cleaner-code` (CodeSafer) | dọn ký tự | không kiểm chứng được | Không |

=> Tự viết scanner stdlib (~260 dòng). Lợi: không phụ thuộc, không mạng, đọc hết
được trong 1 lần review; hiểu tiếng Việt (không coi dấu là lỗi).

## Ghi chú môi trường
- `rootfs-alpine/` có ~335 symlink busybox: scanner bỏ qua symlink, không đi theo
  ra ngoài cây.
- Repo tại 81b5dce: 204 file text, 0 error, 0 warning.
