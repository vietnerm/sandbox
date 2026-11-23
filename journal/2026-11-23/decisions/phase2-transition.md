# Owner chốt chuyển GĐ2 (2026-11-23)

## Ngữ cảnh

- 2026-11-18 (D27): `minictd/` gộp sớm như GĐ2 seed; giai đoạn hiện tại vẫn là GĐ1,
  `minictd/` chỉ sửa bảo mật.
- 2026-11-19: owner chỉ thị "đẩy nhanh tiến độ để kịp qua giai đoạn 2".
- 2026-11-20 (phiên 12): ZAI ghi trong roadmap "GĐ2 HIỆN TẠI, D27 hết hiệu lực" và bắt
  đầu việc GĐ2 (#92, #95, #98, #128, #130), nhưng `idea/manifest.md` (nguồn đứng trên)
  vẫn ghi GĐ1 hiện tại, DoD GĐ1 3/6, D27 nguyên văn → manifest và roadmap mâu thuẫn.

## Quyết định

Owner (2026-11-23) chốt **GĐ2 là giai đoạn hiện tại**. Cập nhật:

1. Manifest §7: tick DoD GĐ1 6/6 với bằng chứng (#24/#54/#64/#88, #85, #59/#74/#79,
   #48/#54, #21, perf-guard); #5 và #31 đã có `verified`. Ghi tiến độ thực tế dưới các
   ô DoD GĐ2; tick "Template" (#128).
2. Manifest §8: thay đoạn ngoại lệ D27 bằng lịch sử chuyển giai đoạn.
3. PROMPT.MD: thay luật "minictd chỉ sửa bảo mật" bằng "giai đoạn hiện tại GĐ2; GĐ3
   phải hỏi owner trước".
4. Roadmap + README: tiêu đề giai đoạn khớp manifest.

## Không đổi

- Thứ tự mục GĐ2 trong roadmap.
- Luật dừng trước GĐ3.
- Residual GĐ1 (caddy, API production-grade, đo RAM idle web stack) không chặn GĐ2.
