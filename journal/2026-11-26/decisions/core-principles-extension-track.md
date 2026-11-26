# Decision D74 — Nguyên tắc "core nhỏ" + Track Mở rộng (stack.yaml, registry tĩnh, cluster)

- **Ngày**: 2026-11-26
- **Nguồn**: góp ý khách quan từ một agent bên ngoài, owner duyệt đưa vào ("nên đưa vào, đó cũng là điều anh muốn làm").
- **Slug**: `core-principles-extension-track`

## Vấn đề
Bản góp ý chỉ ra rủi ro lớn nhất là "mất bản chất vì cố làm quá nhiều", và các khoảng
trống: chưa có định dạng nhiều instance, chưa có kho template chia sẻ, chưa có đường
lên nhiều VPS, tài liệu chưa nói rõ cái gì sẵn sàng. Manifest §4 lại ghi "không cluster",
trái với mục tiêu owner (2026-11-17) gom nhiều VPS quản lý chung như Proxmox.

## Quyết định
1. Manifest §3 thêm 4 nguyên tắc: mặc định tối thiểu (≥ 1 MB RSS mặc định → xem lại),
   core nhỏ + hai phép thử, không phát minh lại, nhãn trạng thái sẵn sàng/thử nghiệm/kế hoạch.
2. Manifest §5: chỉ cần kernel ≥ 6.1 LTS của distro phổ biến, không patch/module ngoài cây.
3. Manifest §4/§6: "không cluster" → "không cluster **trong core**"; RBAC chỉ ở lớp cluster, tuỳ chọn.
4. Manifest §9 + roadmap "Track M": M1 nhãn trạng thái, M2 `stack.yaml`, M3 registry
   template tĩnh, M4 lớp cluster (thiết kế trước, chờ owner duyệt), M5 ví dụ Ansible/GitHub Action.
5. PROMPT.MD §8: giai đoạn hiện tại là GĐ4 (research-only vì thiếu KVM) → việc kế tiếp lấy từ Track M.

## Không lấy từ bản góp ý
- Lộ trình giai đoạn của bản góp ý: thông tin cũ (GĐ2 đã xong, GĐ3 đạt DoD, GĐ4 đã mở).
- "minict không nên làm cụm đa máy": giữ đúng tinh thần (core không biết cluster) nhưng
  vẫn làm cluster như lớp ngoài, vì đó là mục tiêu owner.
- CRI shim cho K3s/K8s: ngược non-goal §4, để ở "Hướng xa, chưa duyệt".
- microVM: đã là GĐ4, không phải "dài hạn tuỳ chọn".
