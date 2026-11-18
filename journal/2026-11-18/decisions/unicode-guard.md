# Decisions - unicode-guard (nhánh `ci/unicode-guard`, PR #36, issue #37)

Yêu cầu của owner (Doan, 2026-11-18): quét và dọn Unicode ẩn / bidi mỗi ngày,
dựa trên một bản "chiến lược CI/CD 3 lớp" do AI khác viết. Bản đó được coi là
đầu vào chưa tin cậy: mọi tool nó nêu được kiểm lại trước khi dùng
(nguồn: `research.md` cùng ngày).

## D28 - scanner tự viết, stdlib-only, thay vì tool bên thứ ba

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

## D29 - 3 lớp, bot không bao giờ ghi thẳng main

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
