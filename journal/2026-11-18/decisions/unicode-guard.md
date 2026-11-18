# Decisions — unicode-guard (PR #36, nhánh `ci/unicode-guard`, issue #37)

> File này gộp hồ sơ quyết định từ **hai phía cùng phiên 2026-11-18**: phía PR (tác giả
> PR push lên nhánh) và phía orchestrator (đã vào main qua PR #40). Trùng số decision
> khi B1 sync (main đã có `D28` canonical) → phía PR đổi số: D28→**D29**, D29→**D30**
> (commit sync này). Không có nội dung nào bị bỏ.


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

---

# Decisions - unicode-guard (nhánh `ci/unicode-guard`, PR #36, issue #37)

Yêu cầu của owner (Doan, 2026-11-18): quét và dọn Unicode ẩn / bidi mỗi ngày,
dựa trên một bản "chiến lược CI/CD 3 lớp" do AI khác viết. Bản đó được coi là
đầu vào chưa tin cậy: mọi tool nó nêu được kiểm lại trước khi dùng
(nguồn: `research.md` cùng ngày).

## D29 - scanner tự viết, stdlib-only, thay vì tool bên thứ ba

- `tools/unicode-guard.py`: Python stdlib, không mạng, không pip. Cổng bảo mật
  không được tự kéo code bên ngoài không ghim (`pip install heckler` trong bản
  gốc không ghim phiên bản = rủi ro chuỗi cung ứng đúng loại nó muốn chặn).
- Phân loại: **error** (fail + `--fix` xoá) = bidi U+202A-202E/2066-2069/200E/200F/061C,
  zero-width/invisible, tag U+E0000-E007F, variation selector ngoài emoji,
  VS supplement U+E0100-E01EF, C0/C1 control, U+2028/2029, BOM giữa file.
  **warn** (chỉ báo) = NBSP và khoảng trắng lạ, homoglyph Latin+Cyrillic/Greek, UTF-8 hỏng.
- Tiếng Việt (NFC lẫn NFD, dấu tổ hợp U+0300-U+0323) và emoji (VS16, chuỗi ZWJ)
  không phải finding: test UG4/UG5.
- `--fix` chỉ xoá byte của error; không chuẩn hoá NFC, không thay NBSP
  (đổi nghĩa văn bản phải do người quyết).
- Fixture test sinh lúc chạy từ escape `\uXXXX`; `tests/test_unicode.sh` thuần ASCII,
  nên UG13 "repo sạch" không phải tự miễn trừ.

## D30 - 3 lớp, bot không bao giờ ghi thẳng main

1. Local: `tools/hooks/pre-commit`, `make unicode-check`.
2. PR: `unicode-guard.yml` quét file thay đổi so với base (`--changed`), push main
   quét toàn cây. `permissions: contents: read`, `persist-credentials: false`.
   Quét file thay đổi thay vì toàn cây để một ký tự bẩn có sẵn trên main không
   chặn mọi PR không liên quan (lớp 3 dọn nó).
3. Hằng ngày 01:00 UTC (08:00 ICT): `unicode-clean-daily.yml` quét toàn cây, bẩn thì
   `--fix` -> nhánh mới `bot/unicode-clean-YYYY-MM-DD` -> PR. Mỗi lúc 1 PR (có PR mở
   thì chỉ comment), không force-push, không push main. Repo tắt quyền Actions mở PR
   thì mở issue thay thế.
- Action ghim SHA: checkout `11bd719` (v4.2.2), upload-artifact `ea165f8` (v4.6.2).
- PR do `GITHUB_TOKEN` mở không kích hoạt workflow khác: ghi rõ trong body PR bot.
- PR dọn của bot là `type:security`, G7 với ZAI: không tự gộp.

## Bài học khi gộp

- Merge main vào nhánh (86030d3) đã đổi TAB recipe trong Makefile thành 8 space ->
  `make build` "missing separator". Sửa ở 7685b3f. Sau mọi merge/giải xung đột
  Makefile: chạy `make -n build test`.
- Lúc viết test, công cụ ghi file đã biến escape `\uXXXX` thành ký tự thật; mô phỏng
  job hằng ngày bắt được trước khi push. Kiểm file test bằng `grep -P '[^\x00-\x7F]'`.
