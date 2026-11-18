# Research — 2026-11-18 (phiên orchestrator 2, chủ đề: Trojan Source / Unicode guard)

> Liên quan PR #36 (ci/unicode-guard). Mục đích: đủ cơ sở đánh giá scanner tự viết
> của PR so với state of the art. Nguồn đã mở/verify link thật.

## Nguồn 1 — NVD CVE-2021-42574 (Trojan Source, bidi)
- Link: https://nvd.nist.gov/vuln/detail/CVE-2021-42574 (đã verify qua search NIST)
- Tóm tắt: lỗ hổng theo dõi công khai cho tấn công tái sắp xếp hiển thị qua thuật toán
  bidirectional của Unicode ≤ 14.0 — code hiển thị với reviewer khác với thứ tự parser
  đọc. Miễn phí giấy phép special-exception, điển hình: chèn RLO/PDI vào comment/string
  để ẩn nhánh `if (admin)`. Ảnh hưởng compiler/language đa số; mitigation chính là
  tooling quét ở review/CI, không phải vá compiler.
- Áp dụng cho minict: repo c code + shell + workflow YAML — một ký tự RLO trong
  `sandbox/minict.c` đủ đổi ngữ nghĩa lệnh mount/unshare cho người review. Lớp PR-gate
  của PR #36 chặn đúng lớp này trước khi vào main.

## Nguồn 2 — Paper gốc: "Trojan Source: Invisible Vulnerabilities" (Boucher & Anderson)
- Link: https://arxiv.org/abs/2111.00169 (verify qua search), mã nguồn PoC:
  https://github.com/nickboucher/trojan-source
- Tóm tắt: hệ thống hoá các attack class: Bidi_override, early_return (RLO trong
  string), homoglyph_attack (đổi identifier bằng ký tự giống mắt — scanner trả về
  warn, UG8), Combining-character, bad_indent với mixed LTR/RTL. Đề xuất compiler
  warn + linter reject bidi/invisible.
- Áp dụng: bảng codepoint của unicode-guard.py (bidi U+202A–2069, invisible, tag
  U+E0000–E007F, VS supplement U+E0100–E01EF) phủ đúng các class trong paper; điểm
  paper không xử lý mà PR xử lý thêm: allowlist ngữ cảnh emoji (ZWJ/VS16) để không
  phá text thực — trade-off chấp nhận được vì emoji không xuất hiện trong code path.

## Nguồn 3 — ASCII smuggling qua Unicode Tags (vector mới 2024–2026)
- Link: https://embracethered.com/blog/posts/2024/hiding-and-finding-text-with-unicode-tags
  (verify) ; Microsoft Security Blog 2026-09-03:
  https://www.microsoft.com/en-us/security/blog/2026/09/03/ascii-smuggling-crosses-over-from-ai-prompt-injection-to-phishing-evasion (verify)
- Tóm tắt: block Tags U+E0000–E007F ánh xạ ASCII (U+E0041='A') render vô hình — dùng
  tráo instruction cho LLM/agent, hoặc giấu payload qua email/phishing (MS 2026).
  Không nằm trong paper 2021 → scanner chỉ check bidi thuần sẽ bỏ sót.
- Áp dụng: repo này có agent (ZAI/SCR/SIA/SBG) đọc prompt/journal — một ký tự tag
  nhét vào PROMPT.MD là vector prompt-injection. unicode-guard.py đánh dấu error
  (U+E0000–E007F + VS supplement) — quyết định D28 đúng hướng; UG3/UG10 test phủ.

## Trade-off tổng (ghi decision D28)
- Tự viết stdlib vs pip tool: mất vào-độ-mới, đổi lấy zero-supply-chain + audit
  được trong 263 dòng. Chấp nhận.
- warn không fail (NBSP/homoglyph): giảm false positive tiếng Việt; --strict cho
  ai muốn chặt. Chấp nhận.
