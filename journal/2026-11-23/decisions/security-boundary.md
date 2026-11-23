# Ranh giới bảo mật trước GĐ4 + seccomp allowlist lên sớm (2026-11-23)

## Ngữ cảnh
Owner hỏi về rủi ro thoát qua lỗ hổng kernel (instance chia chung kernel với host).
Đánh giá: escape suite chứng minh cơ chế cách ly đang bật, không chứng minh kernel
không có lỗi; watchlist CVE chỉ đánh giá cho kernel dev 5.10. Owner chốt "bảo mật
vẫn là điều ưu tiên".

## Quyết định
1. Manifest §3 thêm nguyên tắc bất biến: bảo mật ưu tiên số 1; tới khi GĐ4 xong,
   minict/minictd chỉ chạy workload tin cậy; việc bảo mật làm trước tính năng cùng
   giai đoạn. README Known Limitations ghi rõ.
2. Seccomp allowlist theo template (#29) bỏ khỏi "Parked thật", thành GĐ2 mục 3a,
   làm trước mục 4–8; hiện thực ở issue #139. Landlock (mục 6) làm ngay sau 3a.
3. Manifest DoD GĐ2 thêm ô allowlist.
