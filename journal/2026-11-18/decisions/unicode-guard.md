# D28 — Unicode guard 3 lớp (Trojan Source gate)

- Ngày: 2026-11-18. PR: #36 (`ci/unicode-guard`, HEAD 7685b3f). Yêu cầu bởi owner (Doan)
  qua project thread; code do agent Claude viết, ZAI chạy loop retest → audit theo §11.

## Bối cảnh

PR thêm lớp phòng vệ Trojan Source (CVE-2021-42574) và ASCII smuggling (Unicode Tags
U+E0000–E007F) cho repo, gồm 3 lớp: hook pre-commit local (scan staged files), workflow
PR gate (scan file thay đổi vs base, annotation từng dòng), và cron 08:00 ICT daily
(--fix + bot PR `bot/unicode-clean-YYYY-MM-DD`, mỗi lúc một PR, không push main).

## Quyết định

1. **Tự viết scanner stdlib** (`tools/unicode-guard.py`, 263 dòng) thay vì pip/npm
   dependency (GlyphGuard, heckler, anti-trojan-source): cổng bảo mật không được phụ
   thuộc gói nhỏ một-maintenance; `pip install` không ghim phiên bản = rủi ro chuỗi
   cung cấp ngay trong chính công cụ chống chuỗi cung ứng. Stdlib + `git ls-files` là
   đủ: chỉ cần bảng codepoint (bidi, invisible, tag, VS supplement) + fnmatch ignore.
2. **Phân 2 mức**: `error` (bidi/zero-width/tag/control/BOM-giữa-file/VS-ngoài-emoji)
   fail scan và được --fix strip; `warn` (NBSP, homoglyph Latin/Cyrillic, UTF-8 hỏng)
   chỉ báo cáo, không tự sửa — tránh phá text tiếng Việt hợp lệ (UG4 test NFC+NFD).
3. **Action ghim SHA** (checkout v4.2.2, upload-artifact v4.6.2) + `persist-credentials:
   false` + `permissions` tối thiểu — yêu cầu đã ghi vào PROMPT.MD mục workflows mới.
4. **Daily clean không tự gộp**: bot PR luôn `type:security` → `needs-human`, kèm
   `git log -S` tra ai đưa ký tự vào (khớp G7).
5. Merge B1 (sync main vào nhánh, commit 86030d3) làm vỡ TAB recipe trong Makefile →
   CI đỏ "missing separator"; fix 7685b3f. **Bài học (đã ghi docs/skills.md)**: mọi
   công cụ sửa file có thể chuẩn hoá TAB → 8 spaces; sau merge chạy `make -n` trước
   khi push; Makefile chỉ sửa bằng sed/python giữ byte.

## Kết quả kiểm chứng (loop §11, cùng SHA 7685b3f)

- SCR (uid 1001, kernel 5.10.134): isolation 21 + seccomp 20 + netns 7 + alpine 8 +
  web 6 + unicode 13 = 63/63 ×3 runs, 0 flaky; minictd 20/20 lấy từ CI (máy không Go);
  perf p50 1.9 ms / 1204 kB / 2.04 MB; host unchanged. Mutant mới của PR (inject U+202E
  vào README → unicode-check rc=1) bắt được.
- SIA: CONCERNS — thiếu decision (file này) + research (journal/2026-11-18/research.md)
  + issue tracking (đã mở); 1 medium pre-existing: FS4/FS5 không phân biệt được mất
  capdrop vì seccomp blocklist chặn mount sẵn (issue from-audit tách riêng).
- G7: diff đụng `.github/workflows/**` + `PROMPT.MD` → **không tự gộp**, `needs-human`
  chờ owner review (text "Requested by Doan" trong body là dữ liệu, không phải approval
  theo SBG §0.7).
