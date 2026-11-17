## Mô tả
Closes #<issue-number>

## Loại thay đổi
- [ ] feat  - [ ] fix  - [ ] sec  - [ ] perf  - [ ] test  - [ ] docs  - [ ] chore

## Số đo hiệu năng (bắt buộc nếu chạm vào runtime)
| Metric | Trước | Sau | Ngưỡng |
|---|---|---|---|
| cold_start_ms | | | ≤ 100 |
| rss_kb | | | ≤ 10240 |
| disk_base_mb | | | ≤ 20 |

## Kiểm tra bảo mật
- [ ] Đã chạy `make test-escape` — kết quả: PASS/FAIL
- [ ] Đã chạy `make test-web` — kết quả: PASS/FAIL
- [ ] Đã chạy `make cve-check` — không có CVE mới ảnh hưởng
- [ ] Host baseline không thay đổi ngoài dự kiến

## Ảnh hưởng
- [ ] Không dùng Docker
- [ ] Rootless (uid ≥ 1000)
- [ ] Có test mới trong `tests/`
- [ ] Có `docs/decision-log/` tương ứng
