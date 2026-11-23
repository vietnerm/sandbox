# check-docs: chặn README lỗi thời trong CI (issues #26, #27, 2026-11-23)

## Ngữ cảnh
README chậm hơn code: mục "Giới hạn đã biết" vẫn ghi "Không /proc trong sandbox
(host seccomp chặn mount proc)" dù D39 đã sửa, và không có mục Platform Support.

## Quyết định
1. `tools/check-docs.sh` (bash, không dependency) — D1 mỗi dòng `## Known Limitations`
   có link `journal/` hoặc `docs/`; D2 link tương đối trong README + `docs/*.md` tồn tại;
   D3 README không chứa câu trong `tools/docs-stale-phrases.txt`; D4 bảng
   `## Platform Support` khớp manifest §5 ở kernel (phiên bản) và cgroups (v1/v2).
   Issue ghi `scripts/`; đặt ở `tools/` theo quy ước sẵn có của repo.
2. `tests/test_check_docs.sh` DC0–DC5: baseline PASS + 5 negative control, mỗi case
   đỏ đúng lý do (`DC_VERBOSE=1` in output).
3. CI `ci.yml` thêm bước `make check-docs && make test-check-docs`; PR template thêm
   dòng nhắc cập nhật Known Limitations + câu stale.
4. README: thay "Giới hạn đã biết" bằng `## Known Limitations` (9 dòng, mỗi dòng một
   link decision/roadmap, bỏ các giới hạn đã sửa); thêm `## Platform Support` sau
   Quick Start, chỉ ghi điều đã đo (kernel 5.10 / 6.18 / 7.0, x86_64).

## Ghi chú
PR đụng `.github/**` → cần owner duyệt trước khi gộp.
